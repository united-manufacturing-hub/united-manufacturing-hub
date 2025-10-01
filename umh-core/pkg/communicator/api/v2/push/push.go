// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package push

import (
	"context"
	http2 "net/http"
	"sync/atomic"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/backend_api_structs"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/error_handler"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/http"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

type DeadLetter struct {
	Messages      []models.UMHMessage
	retryAttempts int
}

func DefaultDeadLetterChanBuffer() chan DeadLetter {
	return make(chan DeadLetter, 1000)
}

func DefaultBackoffPolicy() *tools.Backoff {
	// 5 seconds is half the time until the frontend will consider the backend as dead
	return tools.NewBackoff(1*time.Millisecond, 2*time.Millisecond, 5*time.Second, tools.BackoffPolicyExponential)
}

type Pusher struct {
	dog                    watchdog.Iface
	jwt                    atomic.Value
	outboundMessageChannel chan *models.UMHMessage
	deadletterCh           chan DeadLetter
	backoff                *tools.Backoff
	logger                 *zap.SugaredLogger
	apiURL                 string
	instanceUUID           uuid.UUID
	watcherUUID            uuid.UUID
	insecureTLS            bool
}

func NewPusher(instanceUUID uuid.UUID, jwt string, dog watchdog.Iface, outboundChannel chan *models.UMHMessage, deadletterCh chan DeadLetter, backoff *tools.Backoff, insecureTLS bool, apiURL string, logger *zap.SugaredLogger) *Pusher {
	p := Pusher{
		instanceUUID:           instanceUUID,
		outboundMessageChannel: outboundChannel,
		deadletterCh:           deadletterCh,
		jwt:                    atomic.Value{},
		dog:                    dog,
		backoff:                backoff,
		insecureTLS:            insecureTLS,
		apiURL:                 apiURL,
		logger:                 logger,
	}
	p.jwt.Store(jwt)

	return &p
}

func (p *Pusher) UpdateJWT(jwt string) {
	p.jwt.Store(jwt)
}

func (p *Pusher) Start() {
	go p.push()
}

func (p *Pusher) Push(message models.UMHMessage) {
	umhMessage := &models.UMHMessage{
		InstanceUUID: p.instanceUUID,
		Content:      message.Content,
		Email:        message.Email,
	}

	defer func() {
		if r := recover(); r != nil {
			p.logger.Errorf("Panic in Push: %v", r)
			p.dog.ReportHeartbeatStatus(p.watcherUUID, watchdog.HEARTBEAT_STATUS_WARNING)
		}
	}()

	select {
	case p.outboundMessageChannel <- umhMessage:
		return
	default:
		p.logger.Warnf("Outbound message channel is full, draining to deadletter channel.")

		if p.watcherUUID != uuid.Nil {
			p.dog.ReportHeartbeatStatus(p.watcherUUID, watchdog.HEARTBEAT_STATUS_WARNING)
		}

		messages := p.outBoundMessages()
		messages = append(messages, *umhMessage)
		p.enqueueAndDropOldestDeadLetterCh(messages, 0)
	}
}

func (p *Pusher) enqueueAndDropOldestDeadLetterCh(messages []models.UMHMessage, retryAttempt int) {
	dl := DeadLetter{
		Messages:      messages,
		retryAttempts: retryAttempt,
	}

	select {
	case p.deadletterCh <- dl:
		return
	default:
		p.logger.Warnf("Deadletter channel is full, dropping oldest message.")
	}

	select {
	case <-p.deadletterCh:
		p.logger.Debugf("Dropped oldest deadletter message to not have a blocking channel")
	default:
	}

	select {
	case p.deadletterCh <- dl:
		return
	default:
	}
}

func (p *Pusher) push() {
	boPostRequest := p.backoff
	p.watcherUUID = p.dog.RegisterHeartbeat("push", 10, 600, false)

	ticker := time.NewTicker(10 * time.Millisecond)

	for {
		select {
		case <-ticker.C:
			p.dog.ReportHeartbeatStatus(p.watcherUUID, watchdog.HEARTBEAT_STATUS_OK)

			messages := p.outBoundMessages()
			if len(messages) == 0 {
				continue
			}

			cookies := map[string]string{
				"token": p.jwt.Load().(string),
			}

			payload := backend_api_structs.PushPayload{
				UMHMessages: messages,
			}

			_, status, err := http.PostRequest[any, backend_api_structs.PushPayload](context.Background(), http.PushEndpoint, &payload, nil, &cookies, p.insecureTLS, p.apiURL, p.logger)
			if err != nil {
				error_handler.ReportHTTPErrors(err, status, string(http.PushEndpoint), "POST", &payload, nil)
				p.dog.ReportHeartbeatStatus(p.watcherUUID, watchdog.HEARTBEAT_STATUS_WARNING)

				if status == http2.StatusBadRequest {
					boPostRequest.IncrementAndSleep()

					continue
				}

				p.enqueueAndDropOldestDeadLetterCh(messages, 0)

				boPostRequest.IncrementAndSleep()

				continue
			}

			error_handler.ResetErrorCounter()
			boPostRequest.Reset()

		case d, ok := <-p.deadletterCh:
			if !ok {
				continue
			}

			if len(d.Messages) == 0 {
				continue
			}

			p.dog.ReportHeartbeatStatus(p.watcherUUID, watchdog.HEARTBEAT_STATUS_OK)

			if d.retryAttempts > 2 {
				continue
			}

			d.retryAttempts++

			cookies := map[string]string{
				"token": p.jwt.Load().(string),
			}

			_, _, err := http.PostRequest[any, backend_api_structs.PushPayload](context.Background(), http.PushEndpoint, &backend_api_structs.PushPayload{UMHMessages: d.Messages}, nil, &cookies, p.insecureTLS, p.apiURL, p.logger)
			if err != nil {
				p.dog.ReportHeartbeatStatus(p.watcherUUID, watchdog.HEARTBEAT_STATUS_WARNING)
				boPostRequest.IncrementAndSleep()

				p.enqueueAndDropOldestDeadLetterCh(d.Messages, d.retryAttempts)
			}

			boPostRequest.Reset()
		}
	}
}

func (p *Pusher) outBoundMessages() []models.UMHMessage {
	messages := make([]models.UMHMessage, 0, len(p.outboundMessageChannel))
	if len(p.outboundMessageChannel) == 0 {
		return messages
	}

	for {
		select {
		case msgX := <-p.outboundMessageChannel:
			if msgX == nil {
				continue
			}

			messages = append(messages, *msgX)
		default:
			return messages
		}
	}
}
