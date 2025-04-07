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

type Puller struct {
	inboundMessageChannel chan *models.UMHMessage
	shallRun              atomic.Bool
	jwt                   atomic.Value
	dog                   watchdog.Iface
	insecureTLS           bool
}

func NewPuller(jwt string, dog watchdog.Iface, inboundChannel chan *models.UMHMessage, insecureTLS bool) *Puller {
	p := Puller{
		inboundMessageChannel: inboundChannel,
		shallRun:              atomic.Bool{},
		jwt:                   atomic.Value{},
		dog:                   dog,
		insecureTLS:           insecureTLS,
	}
	p.jwt.Store(jwt)
	return &p
}

func (p *Puller) UpdateJWT(jwt string) {
	p.jwt.Store(jwt)
}

func (p *Puller) Start() {
	p.shallRun.Store(true)
	go p.pull()
}

// Stop stops the puller
// This function is only for testing purposes
func (p *Puller) Stop() {
	if helper.IsTest() {
		zap.S().Warnf("WARNING: Stopping puller !")
		p.shallRun.Store(false)
	} else {
		sentry.ReportIssuef(sentry.IssueTypeError, zap.S(), "[Puller.Stop()] Stop MUST NOT be used outside tests")
	}
}

func (p *Puller) pull() {
	watcherUUID := p.dog.RegisterHeartbeat("pull", 10, 600, false)
	var ticker = time.NewTicker(10 * time.Millisecond)
	for p.shallRun.Load() {
		select {
		case <-ticker.C:

			p.dog.ReportHeartbeatStatus(watcherUUID, watchdog.HEARTBEAT_STATUS_OK)
			var cookies = map[string]string{
				"token": p.jwt.Load().(string),
			}
			incomingMessages, err, _ := http.GetRequest[backend_api_structs.PullPayload](context.Background(), http.PullEndpoint, nil, &cookies, p.insecureTLS)
			if err != nil {
				// Ignore context canceled errors
				if errors.Is(err, context.Canceled) {
					time.Sleep(1 * time.Second)
					continue
				}
				continue
			}
			error_handler.ResetErrorCounter()
			if incomingMessages == nil || incomingMessages.UMHMessages == nil || len((*incomingMessages).UMHMessages) == 0 {
				time.Sleep(1 * time.Second)
				continue
			}

			for _, message := range (*incomingMessages).UMHMessages {

				zap.S().Infof("Received message: %v", message)

				insertionTimeout := time.After(10 * time.Second)
				select {
				case p.inboundMessageChannel <- &models.UMHMessage{
					Email:        message.Email,
					Content:      message.Content,
					InstanceUUID: message.InstanceUUID,
					Metadata:     message.Metadata,
				}:
				case <-insertionTimeout:
					zap.S().Warnf("Inbound message channel is full !")
					p.dog.ReportHeartbeatStatus(watcherUUID, watchdog.HEARTBEAT_STATUS_WARNING)
				}
			}
		}
	}
	if helper.IsTest() {
		p.dog.UnregisterHeartbeat(watcherUUID)
	} else {
		p.dog.ReportHeartbeatStatus(watcherUUID, watchdog.HEARTBEAT_STATUS_ERROR)
	}
}

// UserCertificateEndpoint is the endpoint for getting a user certificate
var UserCertificateEndpoint http.Endpoint = "/v2/instance/user/certificate"

// UserCertificateResponse represents the response from the user certificate endpoint
type UserCertificateResponse struct {
	UserEmail   string `json:"userEmail"`
	Certificate string `json:"certificate"`
}

// GetUserCertificate retrieves a user certificate from the backend
func GetUserCertificate(ctx context.Context, userEmail string, cookies *map[string]string, insecureTLS bool) (*UserCertificateResponse, error) {
	// URL encode the email
	encodedEmail := url.QueryEscape(userEmail)

	// Create the endpoint with the query parameter
	endpoint := http.Endpoint(fmt.Sprintf("%s?email=%s", UserCertificateEndpoint, encodedEmail))

	// print endpoint
	zap.S().Infof("Getting user certificate. Endpoint:  %s", endpoint)

	// Make the request
	response, err, statusCode := http.GetRequest[UserCertificateResponse](ctx, endpoint, nil, cookies, insecureTLS)
	if err != nil {
		if statusCode == http2.StatusNoContent {
			// User does not have a certificate
			return nil, nil
		}
		zap.S().Errorf("Failed to get user certificate: %v (status code: %d)", err, statusCode)
		return nil, err
	}

	return response, nil
}
