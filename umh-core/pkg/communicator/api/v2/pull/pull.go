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

// TODO: check why this was modified here in this branch, if not necessarily required, leave old communicator the same
// only acceptable changes are for the bridge/adapter to allow the new fsmv2 transport

package pull

import (
	"context"
	"errors"
	"fmt"
	http2 "net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/error_handler"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/http"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/backend_api_structs"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/helper"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"go.uber.org/zap"
)

// RequestInFlightThreshold is the duration after which a request is considered
// stuck and the health check should report WARNING status.
const RequestInFlightThreshold = 10 * time.Second

type Puller struct {
	jwt                   atomic.Value
	dog                   watchdog.Iface
	inboundMessageChannel chan *models.UMHMessage
	logger                *zap.SugaredLogger
	apiURL                string
	shallRun              atomic.Bool
	insecureTLS           bool

	// Request state tracking for health checks
	inFlightRequest   atomic.Bool
	requestStartTime  atomic.Value // stores time.Time
	lastRequestFailed atomic.Bool
}

func NewPuller(jwt string, dog watchdog.Iface, inboundChannel chan *models.UMHMessage, insecureTLS bool, apiURL string, logger *zap.SugaredLogger) *Puller {
	p := Puller{
		inboundMessageChannel: inboundChannel,
		shallRun:              atomic.Bool{},
		jwt:                   atomic.Value{},
		dog:                   dog,
		insecureTLS:           insecureTLS,
		apiURL:                apiURL,
		logger:                logger,
	}
	p.jwt.Store(jwt)

	return &p
}

func (p *Puller) UpdateJWT(jwt string) {
	p.jwt.Store(jwt)
}

// SetRequestInFlight sets whether an HTTP request is currently in flight.
// This is used for health check status reporting.
func (p *Puller) SetRequestInFlight(inFlight bool) {
	p.inFlightRequest.Store(inFlight)
}

// IsRequestInFlight returns whether an HTTP request is currently in flight.
func (p *Puller) IsRequestInFlight() bool {
	return p.inFlightRequest.Load()
}

// SetRequestStartTime sets the time when the current request started.
func (p *Puller) SetRequestStartTime(t time.Time) {
	p.requestStartTime.Store(t)
}

// GetRequestStartTime returns the time when the current request started.
// Returns zero time if no request has been started.
func (p *Puller) GetRequestStartTime() time.Time {
	if t, ok := p.requestStartTime.Load().(time.Time); ok {
		return t
	}

	return time.Time{}
}

// SetLastRequestFailed sets whether the last request failed.
func (p *Puller) SetLastRequestFailed(failed bool) {
	p.lastRequestFailed.Store(failed)
}

// GetCurrentStatus returns the current health status of the puller.
// This can be used by external health checks to determine if the puller
// is operating normally or experiencing issues.
//
// Returns:
//   - HEARTBEAT_STATUS_WARNING if a request has been in flight for > 10s
//   - HEARTBEAT_STATUS_WARNING if the last request failed
//   - HEARTBEAT_STATUS_OK otherwise
func (p *Puller) GetCurrentStatus() watchdog.HeartbeatStatus {
	// Check if request has been in flight too long
	if p.inFlightRequest.Load() {
		if startTime := p.GetRequestStartTime(); !startTime.IsZero() {
			if time.Since(startTime) > RequestInFlightThreshold {
				return watchdog.HEARTBEAT_STATUS_WARNING
			}
		}
	}

	// Check if last request failed
	if p.lastRequestFailed.Load() {
		return watchdog.HEARTBEAT_STATUS_WARNING
	}

	return watchdog.HEARTBEAT_STATUS_OK
}

func (p *Puller) Start() {
	p.shallRun.Store(true)

	go p.pull()
}

// Stop stops the puller
// This function is only for testing purposes.
func (p *Puller) Stop() {
	if helper.IsTest() {
		p.logger.Warnf("WARNING: Stopping puller !")
		p.shallRun.Store(false)
	} else {
		sentry.ReportIssuef(sentry.IssueTypeError, p.logger, "[Puller.Stop()] Stop MUST NOT be used outside tests")
	}
}

func (p *Puller) pull() {
	watcherUUID := p.dog.RegisterHeartbeat("pull", 10, 600, false)

	var ticker = time.NewTicker(10 * time.Millisecond)
	for p.shallRun.Load() {
		<-ticker.C

		var cookies = map[string]string{
			"token": p.jwt.Load().(string),
		}

		// Track request state for health monitoring
		p.SetRequestInFlight(true)
		p.SetRequestStartTime(time.Now())

		incomingMessages, _, err := http.GetRequest[backend_api_structs.PullPayload](context.Background(), http.PullEndpoint, nil, &cookies, p.insecureTLS, p.apiURL, p.logger)

		// Request completed - update state
		p.SetRequestInFlight(false)

		if err != nil {
			p.SetLastRequestFailed(true)

			// Report current status to watchdog (will be WARNING due to error)
			p.dog.ReportHeartbeatStatus(watcherUUID, p.GetCurrentStatus())

			// Ignore context canceled errors
			if errors.Is(err, context.Canceled) {
				time.Sleep(1 * time.Second)

				continue
			}

			p.logger.Errorf("Error pulling messages: %v", err)
			// Circuit breaker: prevent log spam when backend unreachable.
			// 1 second provides breathing room without impacting recovery time,
			// since this runs async and won't block data infrastructure.
			time.Sleep(1 * time.Second)

			continue
		}

		// Request succeeded - clear error state
		p.SetLastRequestFailed(false)
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
			}
		}
	}

	if helper.IsTest() {
		p.dog.UnregisterHeartbeat(watcherUUID)
	} else {
		p.dog.ReportHeartbeatStatus(watcherUUID, watchdog.HEARTBEAT_STATUS_ERROR)
	}
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
		if statusCode == http2.StatusNoContent {
			// User does not have a certificate
			return nil, nil
		}

		logger.Errorf("Failed to get user certificate: %v (status code: %d)", err, statusCode)

		return nil, err
	}

	return response, nil
}
