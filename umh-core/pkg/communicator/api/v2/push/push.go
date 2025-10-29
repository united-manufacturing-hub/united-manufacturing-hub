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
	nethttp "net/http"
	"sync"
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

// connectionCloser is an interface for HTTP transports that support closing idle connections
type connectionCloser interface {
	CloseIdleConnections()
}

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
	coordinatedRestartFunc func() error
	dog                    watchdog.Iface
	jwt                    atomic.Value
	outboundMessageChannel chan *models.UMHMessage
	deadletterCh           chan DeadLetter
	backoff                *tools.Backoff
	logger                 *zap.SugaredLogger
	stopChan               chan struct{}
	doneChan               chan struct{}
	apiURL                 string
	watcherMutex           sync.RWMutex
	stopOnce               sync.Once
	stopMutex              sync.Mutex
	isRestarting           atomic.Bool
	instanceUUID           uuid.UUID
	watcherUUID            uuid.UUID
	insecureTLS            bool
}

func NewPusher(instanceUUID uuid.UUID, jwt string, dog watchdog.Iface, outboundChannel chan *models.UMHMessage, deadletterCh chan DeadLetter, backoff *tools.Backoff, insecureTLS bool, apiURL string, logger *zap.SugaredLogger) *Pusher {
	return NewPusherWithRestartFunc(instanceUUID, jwt, dog, outboundChannel, deadletterCh, backoff, insecureTLS, apiURL, logger, nil)
}

func NewPusherWithRestartFunc(instanceUUID uuid.UUID, jwt string, dog watchdog.Iface, outboundChannel chan *models.UMHMessage, deadletterCh chan DeadLetter, backoff *tools.Backoff, insecureTLS bool, apiURL string, logger *zap.SugaredLogger, coordinatedRestartFunc func() error) *Pusher {
	p := Pusher{
		coordinatedRestartFunc: coordinatedRestartFunc,
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
	p.stopMutex.Lock()
	p.stopChan = make(chan struct{})
	p.doneChan = make(chan struct{})
	p.stopOnce = sync.Once{}
	p.stopMutex.Unlock()

	go p.push()
}

// Stop stops the pusher and returns a channel that will be closed when the goroutine finishes.
func (p *Pusher) Stop() <-chan struct{} {
	p.stopMutex.Lock()
	stopChan := p.stopChan
	doneChan := p.doneChan
	p.stopMutex.Unlock()

	if stopChan != nil {
		p.stopOnce.Do(func() {
			p.logger.Info("[PUSH] Stopping")
			close(stopChan)
		})
	}

	if doneChan == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}

	return doneChan
}

func (p *Pusher) Push(message models.UMHMessage) {
	if len(p.outboundMessageChannel) == cap(p.outboundMessageChannel) {
		p.logger.Warnf("Outbound message channel is full !")

		p.watcherMutex.RLock()
		watcherUUID := p.watcherUUID
		p.watcherMutex.RUnlock()

		if watcherUUID != uuid.Nil {
			p.dog.ReportHeartbeatStatus(watcherUUID, watchdog.HEARTBEAT_STATUS_WARNING)
		}
	}

	// Recover from panic
	// This is primarily for tests, where the outboundMessageChannel is closed.
	defer func() {
		if r := recover(); r != nil {
			zap.S().Errorf("Panic in Push: %v", r)

			p.watcherMutex.RLock()
			watcherUUID := p.watcherUUID
			p.watcherMutex.RUnlock()

			p.dog.ReportHeartbeatStatus(watcherUUID, watchdog.HEARTBEAT_STATUS_WARNING)
		}
	}()

	p.outboundMessageChannel <- &models.UMHMessage{
		InstanceUUID: p.instanceUUID,
		Content:      message.Content,
		Email:        message.Email,
	}
}

func (p *Pusher) push() {
	defer func() {
		p.stopMutex.Lock()
		defer p.stopMutex.Unlock()
		if p.doneChan != nil {
			close(p.doneChan)
			p.doneChan = nil
		}
	}()

	boPostRequest := p.backoff

	p.watcherMutex.Lock()

	if p.watcherUUID != uuid.Nil {
		p.dog.UnregisterHeartbeat(p.watcherUUID)
	}

	restartFunc := p.Restart
	if p.coordinatedRestartFunc != nil {
		restartFunc = p.coordinatedRestartFunc
	}

	p.watcherUUID = p.dog.RegisterHeartbeatWithRestart("Pusher", 12, 0, false, restartFunc)
	watcherUUID := p.watcherUUID
	p.watcherMutex.Unlock()

	var ticker = time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		p.stopMutex.Lock()
		stopChan := p.stopChan
		p.stopMutex.Unlock()

		select {
		case <-stopChan:
			// Clean shutdown - always unregister
			p.dog.UnregisterHeartbeat(watcherUUID)

			return
		case <-ticker.C:
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

			_, status, err := http.PostRequest[any, backend_api_structs.PushPayload](context.Background(), http.PushEndpoint, &payload, nil, &cookies, p.insecureTLS, p.apiURL, p.logger)
			if err != nil {
				error_handler.ReportHTTPErrors(err, status, string(http.PushEndpoint), "POST", &payload, nil)
				p.dog.ReportHeartbeatStatus(watcherUUID, watchdog.HEARTBEAT_STATUS_WARNING)

				if status == nethttp.StatusBadRequest {
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

			p.dog.ReportHeartbeatStatus(watcherUUID, watchdog.HEARTBEAT_STATUS_OK)
			error_handler.ResetErrorCounter()
			boPostRequest.Reset()

		case d, ok := <-p.deadletterCh:
			if !ok {
				continue
			}

			if len(d.messages) == 0 {
				continue
			}

			p.dog.ReportHeartbeatStatus(watcherUUID, watchdog.HEARTBEAT_STATUS_OK)
			// Retry the messages in deadletter channel only thrice. If it fails after 3 retryAttempts, log the message and drop.
			if d.retryAttempts > 2 {
				continue
			}

			d.retryAttempts++

			_, _, err := http.PostRequest[any, backend_api_structs.PushPayload](context.Background(), http.PushEndpoint, &backend_api_structs.PushPayload{UMHMessages: d.messages}, nil, &d.cookies, p.insecureTLS, p.apiURL, p.logger)
			if err != nil {
				p.dog.ReportHeartbeatStatus(watcherUUID, watchdog.HEARTBEAT_STATUS_WARNING)
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
	case deadLetterCh <- DeadLetter{
		messages:      messages,
		cookies:       cookies,
		retryAttempts: retryAttempt,
	}:
		// Message successfully enqueued to deadletter channel. Do nothing.
	default:
		sentry.ReportIssuef(sentry.IssueTypeError, logger, "[enqueueToDeadLetterChannel] Deadletter channel full or closed, cannot enqueue messages!")
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

// Restart performs graceful restart with HTTP client reset and DNS cache flush.
func (p *Pusher) Restart() error {
	logger := p.logger.With("component", "PUSH", "action", "restart")
	logger.Info("Starting PUSH restart sequence")

	p.isRestarting.Store(true)
	defer p.isRestarting.Store(false)

	logger.Debug("Step 1: Stopping PUSH goroutine")
	done := p.Stop()
	select {
	case <-done:
		logger.Debug("PUSH goroutine stopped successfully")
	case <-time.After(5 * time.Second):
		logger.Warn("Timeout waiting for PUSH goroutine to stop")
	}

	logger.Debug("Step 2: Resetting HTTP client connections")

	httpClient := http.GetClient(p.insecureTLS)
	if httpClient != nil {
		if closer, ok := httpClient.Transport.(connectionCloser); ok {
			closer.CloseIdleConnections()
			logger.Debug("HTTP connection pool flushed")
		} else {
			logger.Debug("Transport does not support CloseIdleConnections, skipping connection flush")
		}
	} else {
		logger.Warn("HTTP client not initialized, skipping connection flush")
	}

	logger.Debug("Step 3: Waiting 5s for DNS cache expiration")
	time.Sleep(5 * time.Second)

	logger.Debug("Step 4: Starting PUSH goroutine with fresh connections")
	p.Start()
	logger.Info("PUSH restart complete")

	return nil
}
