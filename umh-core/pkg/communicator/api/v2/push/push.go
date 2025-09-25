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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/error_handler"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/http"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

type DeadLetter struct {
	cookies       map[string]string
	messages      []models.UMHMessage
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
	if len(p.outboundMessageChannel) == cap(p.outboundMessageChannel) {
		p.logger.Warnf("Outbound message channel is full !")

		if p.watcherUUID != uuid.Nil {
			p.dog.ReportHeartbeatStatus(p.watcherUUID, watchdog.HEARTBEAT_STATUS_WARNING)
		}
	}

	// Recover from panic
	// This is primarily for tests, where the outboundMessageChannel is closed.
	defer func() {
		if r := recover(); r != nil {
			zap.S().Errorf("Panic in Push: %v", r)
			p.dog.ReportHeartbeatStatus(p.watcherUUID, watchdog.HEARTBEAT_STATUS_WARNING)
		}
	}()

	p.outboundMessageChannel <- &models.UMHMessage{
		InstanceUUID: p.instanceUUID,
		Content:      message.Content,
		Email:        message.Email,
	}
}

func (p *Pusher) push() {
	boPostRequest := p.backoff
	p.watcherUUID = p.dog.RegisterHeartbeat("push", 10, 600, false)

	var ticker = time.NewTicker(10 * time.Millisecond)

	for {
		select {
		case <-ticker.C:
			p.dog.ReportHeartbeatStatus(p.watcherUUID, watchdog.HEARTBEAT_STATUS_OK)

			messages := p.outBoundMessages()
			if len(messages) == 0 {
				continue
			}

			var cookies = map[string]string{
				"token": p.jwt.Load().(string),
			}

			payload := backend_api_structs.PushPayload{
				UMHMessages: messages,
			}

			_, status, err := http.PostRequest[any](context.Background(), http.PushEndpoint, &payload, nil, &cookies, p.insecureTLS, p.apiURL, p.logger)
			if err != nil {
				error_handler.ReportHTTPErrors(err, status, string(http.PushEndpoint), "POST", &payload, nil)
				p.dog.ReportHeartbeatStatus(p.watcherUUID, watchdog.HEARTBEAT_STATUS_WARNING)

				if status == http2.StatusBadRequest {
					// Its bit fuzzy here to determine the error code since the PostRequest does not return the error code.
					// Todo: Need to refactor the PostRequest to return the error code.
					// If the error is 400, drop the message, then the message is invalid.
					// Hence do not reenqueue the message to the deadletter channel.
					boPostRequest.IncrementAndSleep()

					continue
				}
				// In case of an error, push the message back to the deadletter channel.
				go enqueueToDeadLetterChannel(p.deadletterCh, messages, cookies, 0, p.logger)

				boPostRequest.IncrementAndSleep()

				continue
			}

			error_handler.ResetErrorCounter()
			boPostRequest.Reset()

		case d, ok := <-p.deadletterCh:
			if !ok {
				continue
			}

			if len(d.messages) == 0 {
				continue
			}

			p.dog.ReportHeartbeatStatus(p.watcherUUID, watchdog.HEARTBEAT_STATUS_OK)
			// Retry the messages in deadletter channel only thrice. If it fails after 3 retryAttempts, log the message and drop.
			if d.retryAttempts > 2 {
				continue
			}

			d.retryAttempts++

			_, _, err := http.PostRequest[any](context.Background(), http.PushEndpoint, &backend_api_structs.PushPayload{UMHMessages: d.messages}, nil, &d.cookies, p.insecureTLS, p.apiURL, p.logger)
			if err != nil {
				p.dog.ReportHeartbeatStatus(p.watcherUUID, watchdog.HEARTBEAT_STATUS_WARNING)
				boPostRequest.IncrementAndSleep()
				// In case of an error, push the message back to the deadletter channel.
				go enqueueToDeadLetterChannel(p.deadletterCh, d.messages, d.cookies, d.retryAttempts, p.logger)
			}

			boPostRequest.Reset()
		}
	}
}

func enqueueToDeadLetterChannel(deadLetterCh chan DeadLetter, messages []models.UMHMessage, cookies map[string]string, retryAttempt int, logger *zap.SugaredLogger) {
	logger.Debugf("Enqueueing to deadletter channel to push messages: %v with retry attempts: %d", messages, retryAttempt)

	select {
	case _, ok := <-deadLetterCh:
		if !ok {
			// Channel is closed
			sentry.ReportIssuef(sentry.IssueTypeError, logger, "[enqueueToDeadLetterChannel] Deadletter channel is closed, cannot enqueue messages!")

			return
		}
	case deadLetterCh <- DeadLetter{
		messages:      messages,
		cookies:       cookies,
		retryAttempts: retryAttempt,
	}:
		// Message successfully enqueued to deadletter channel. Do nothing.
	default:
		sentry.ReportIssuef(sentry.IssueTypeError, logger, "[enqueueToDeadLetterChannel] Deadletter channel is not open or ready to receive the re-enqueued messages from the Pusher!")
	}
}

func (p *Pusher) outBoundMessages() []models.UMHMessage {
	messages := make([]models.UMHMessage, 0, len(p.outboundMessageChannel))
	if len(p.outboundMessageChannel) == 0 {
		return messages
	}

	for len(p.outboundMessageChannel) > 0 {
		msgX := <-p.outboundMessageChannel
		if msgX == nil {
			continue
		}

		messages = append(messages, *msgX)
	}

	return messages
}
