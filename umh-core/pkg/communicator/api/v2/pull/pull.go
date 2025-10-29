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

package pull

import (
	"context"
	"errors"
	"fmt"
	nethttp "net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/error_handler"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/http"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/backend_api_structs"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

// connectionCloser is an interface for HTTP transports that support closing idle connections
type connectionCloser interface {
	CloseIdleConnections()
}

type Puller struct {
	coordinatedRestartFunc func() error
	jwt                    atomic.Value
	dog                    watchdog.Iface
	inboundMessageChannel  chan *models.UMHMessage
	logger                 *zap.SugaredLogger
	stopChan               chan struct{}
	doneChan               chan struct{}
	apiURL                 string
	watcherMutex           sync.RWMutex
	stopOnce               sync.Once
	stopMutex              sync.Mutex
	shallRun               atomic.Bool
	isRestarting           atomic.Bool
	watcherUUID            uuid.UUID
	insecureTLS            bool
}

func NewPuller(jwt string, dog watchdog.Iface, inboundChannel chan *models.UMHMessage, insecureTLS bool, apiURL string, logger *zap.SugaredLogger) *Puller {
	return NewPullerWithRestartFunc(jwt, dog, inboundChannel, insecureTLS, apiURL, logger, nil)
}

func NewPullerWithRestartFunc(jwt string, dog watchdog.Iface, inboundChannel chan *models.UMHMessage, insecureTLS bool, apiURL string, logger *zap.SugaredLogger, coordinatedRestartFunc func() error) *Puller {
	p := Puller{
		coordinatedRestartFunc: coordinatedRestartFunc,
		inboundMessageChannel:  inboundChannel,
		shallRun:               atomic.Bool{},
		jwt:                    atomic.Value{},
		dog:                    dog,
		insecureTLS:            insecureTLS,
		apiURL:                 apiURL,
		logger:                 logger,
		watcherUUID:            uuid.Nil,
	}
	p.jwt.Store(jwt)

	return &p
}

func (p *Puller) UpdateJWT(jwt string) {
	p.jwt.Store(jwt)
}

func (p *Puller) Start() {
	p.shallRun.Store(true)
	p.stopMutex.Lock()
	p.stopChan = make(chan struct{})
	p.doneChan = make(chan struct{})
	p.stopOnce = sync.Once{}
	p.stopMutex.Unlock()

	go p.pull()
}

// Stop stops the puller and returns a channel that will be closed when the goroutine finishes.
func (p *Puller) Stop() <-chan struct{} {
	p.stopMutex.Lock()
	stopChan := p.stopChan
	doneChan := p.doneChan
	p.stopMutex.Unlock()

	if stopChan != nil {
		p.stopOnce.Do(func() {
			p.logger.Info("[PULL] Stopping")
			close(stopChan)
			p.shallRun.Store(false)
		})
	}

	if doneChan == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}

	return doneChan
}

func (p *Puller) pull() {
	defer func() {
		p.stopMutex.Lock()
		defer p.stopMutex.Unlock()
		if p.doneChan != nil {
			close(p.doneChan)
			p.doneChan = nil
		}
	}()

	p.watcherMutex.Lock()

	if p.watcherUUID != uuid.Nil {
		p.dog.UnregisterHeartbeat(p.watcherUUID)
	}

	restartFunc := p.Restart
	if p.coordinatedRestartFunc != nil {
		restartFunc = p.coordinatedRestartFunc
	}

	p.watcherUUID = p.dog.RegisterHeartbeatWithRestart("Puller", 12, 0, false, restartFunc)
	watcherUUID := p.watcherUUID
	p.watcherMutex.Unlock()

	var ticker = time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for p.shallRun.Load() {
		p.stopMutex.Lock()
		stopChan := p.stopChan
		p.stopMutex.Unlock()

		select {
		case <-stopChan:
			// Clean shutdown - always unregister
			p.dog.UnregisterHeartbeat(watcherUUID)

			return
		case <-ticker.C:
		}

		var cookies = map[string]string{
			"token": p.jwt.Load().(string),
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		go func() {
			select {
			case <-stopChan:
				cancel()
			case <-ctx.Done():
			}
		}()

		incomingMessages, _, err := http.GetRequest[backend_api_structs.PullPayload](ctx, http.PullEndpoint, nil, &cookies, p.insecureTLS, p.apiURL, p.logger)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				time.Sleep(1 * time.Second)

				continue
			}

			p.logger.Errorf("Error pulling messages: %v", err)

			continue
		}

		p.dog.ReportHeartbeatStatus(watcherUUID, watchdog.HEARTBEAT_STATUS_OK)
		error_handler.ResetErrorCounter()

		if incomingMessages == nil || incomingMessages.UMHMessages == nil || len(incomingMessages.UMHMessages) == 0 {
			time.Sleep(1 * time.Second)

			continue
		}

		for _, message := range incomingMessages.UMHMessages {
			insertionTimeout := time.After(10 * time.Second)
			select {
			case p.inboundMessageChannel <- &models.UMHMessage{
				Email:        message.Email,
				Content:      message.Content,
				InstanceUUID: message.InstanceUUID,
				Metadata:     message.Metadata,
			}:
			case <-insertionTimeout:
				p.logger.Warnf("Inbound message channel is full !")
				p.dog.ReportHeartbeatStatus(watcherUUID, watchdog.HEARTBEAT_STATUS_WARNING)
			case <-stopChan:
				p.logger.Debug("Stop requested while sending to inbound channel")
				return
			}
		}
	}
}

// Restart performs graceful restart with HTTP client reset and DNS cache flush.
func (p *Puller) Restart() error {
	logger := p.logger.With("component", "PULL", "action", "restart")
	logger.Info("Starting PULL restart sequence")

	p.isRestarting.Store(true)
	defer p.isRestarting.Store(false)

	logger.Debug("Step 1: Stopping PULL goroutine")
	done := p.Stop()
	select {
	case <-done:
		logger.Debug("PULL goroutine stopped successfully")
	case <-time.After(5 * time.Second):
		logger.Warn("Timeout waiting for PULL goroutine to stop")
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

	logger.Debug("Step 4: Starting PULL goroutine with fresh connections")
	p.Start()
	logger.Info("PULL restart complete")

	return nil
}

// UserCertificateEndpoint is the endpoint for getting a user certificate.
var UserCertificateEndpoint http.Endpoint = "/v2/instance/user/certificate"

// UserCertificateResponse represents the response from the user certificate endpoint.
type UserCertificateResponse struct {
	UserEmail   string `json:"userEmail"`
	Certificate string `json:"certificate"`
}

// GetUserCertificate retrieves a user certificate from the backend
// This function is only for testing purposes.
func GetUserCertificate(ctx context.Context, userEmail string, cookies *map[string]string, insecureTLS bool, apiURL string, logger *zap.SugaredLogger) (*UserCertificateResponse, error) {
	// URL encode the email
	encodedEmail := url.QueryEscape(userEmail)

	// Create the endpoint with the query parameter
	endpoint := http.Endpoint(fmt.Sprintf("%s?email=%s", UserCertificateEndpoint, encodedEmail))

	// print endpoint
	logger.Debugf("Getting user certificate. Endpoint:  %s", endpoint)

	// Make the request
	response, statusCode, err := http.GetRequest[UserCertificateResponse](ctx, endpoint, nil, cookies, insecureTLS, apiURL, logger)
	if err != nil {
		if statusCode == nethttp.StatusNoContent {
			// User does not have a certificate
			return nil, nil
		}

		logger.Errorf("Failed to get user certificate: %v (status code: %d)", err, statusCode)

		return nil, err
	}

	return response, nil
}
