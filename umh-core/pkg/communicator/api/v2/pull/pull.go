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
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	http2 "net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/united-manufacturing-hub/expiremap/v2/pkg/expiremap"
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
	jwt                                   atomic.Value
	dog                                   watchdog.Iface
	inboundMessageChannel                 chan *models.UMHMessageWithAdditionalInfo
	logger                                *zap.SugaredLogger
	userCertificateCache                  *expiremap.ExpireMap[string, *x509.Certificate]
	certChangeChannelForSubscriberHandler chan struct {
		Cert  *x509.Certificate
		Email string
	}
	apiURL      string
	shallRun    atomic.Bool
	insecureTLS bool
}

func NewPuller(jwt string, dog watchdog.Iface, inboundChannel chan *models.UMHMessageWithAdditionalInfo, insecureTLS bool, apiURL string, logger *zap.SugaredLogger, certChan chan struct {
	Cert  *x509.Certificate
	Email string
},
) *Puller {
	p := Puller{
		inboundMessageChannel:                 inboundChannel,
		shallRun:                              atomic.Bool{},
		jwt:                                   atomic.Value{},
		dog:                                   dog,
		insecureTLS:                           insecureTLS,
		apiURL:                                apiURL,
		logger:                                logger,
		userCertificateCache:                  expiremap.NewEx[string, *x509.Certificate](10*time.Second, 10*time.Second), // Refresh every 10 seconds
		certChangeChannelForSubscriberHandler: certChan,
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

	ticker := time.NewTicker(10 * time.Millisecond)
	for p.shallRun.Load() {
		<-ticker.C

		p.dog.ReportHeartbeatStatus(watcherUUID, watchdog.HEARTBEAT_STATUS_OK)

		cookies := map[string]string{
			"token": p.jwt.Load().(string),
		}

		incomingMessages, _, err := http.GetRequest[backend_api_structs.PullPayload](context.Background(), http.PullEndpoint, nil, &cookies, p.insecureTLS, p.apiURL, p.logger)
		if err != nil {
			// Ignore context canceled errors
			if errors.Is(err, context.Canceled) {
				time.Sleep(1 * time.Second)

				continue
			}

			p.logger.Errorf("Error pulling messages: %v", err)

			continue
		}

		error_handler.ResetErrorCounter()

		if incomingMessages == nil || incomingMessages.UMHMessages == nil || len(incomingMessages.UMHMessages) == 0 {
			time.Sleep(1 * time.Second)

			continue
		}

		for _, message := range incomingMessages.UMHMessages {
			userCertificate, ok := p.userCertificateCache.Load(message.Email)
			if !ok {
				p.logger.Infof("Getting user certificate for %s", message.Email)

				cert, err := GetUserCertificate(context.Background(), message.Email, &cookies, p.insecureTLS, p.apiURL, p.logger)
				if err == nil && cert != nil && cert.Certificate != "" {
					p.logger.Infof("User certificate for %s found", message.Email)

					base64Decoded, err := base64.StdEncoding.DecodeString(cert.Certificate)
					if err != nil {
						p.logger.Errorf("Failed to decode user certificate: %v", err)
					}

					x509Cert, err := x509.ParseCertificate(base64Decoded)
					if err != nil {
						p.logger.Errorf("Failed to parse user certificate: %v", err)

						x509Cert = nil
					}

					p.userCertificateCache.Set(message.Email, x509Cert)
					userCertificate = &x509Cert

					// Send the new certificate to the channel
					if p.certChangeChannelForSubscriberHandler != nil {
						p.certChangeChannelForSubscriberHandler <- struct {
							Cert  *x509.Certificate
							Email string
						}{
							Cert:  x509Cert,
							Email: message.Email,
						}
					}
				} else {
					p.logger.Errorf("Failed to get user certificate: %v", err)
				}
			}

			insertionTimeout := time.After(10 * time.Second)

			if userCertificate == nil {
				select {
				case p.inboundMessageChannel <- &models.UMHMessageWithAdditionalInfo{
					UMHMessage:  message,
					Certificate: nil,
				}:
				case <-insertionTimeout:
					p.logger.Warnf("Inbound message channel is full !")
					p.dog.ReportHeartbeatStatus(watcherUUID, watchdog.HEARTBEAT_STATUS_WARNING)
				}
			} else {
				select {
				case p.inboundMessageChannel <- &models.UMHMessageWithAdditionalInfo{
					Certificate: *userCertificate,
					UMHMessage:  message,
				}:
				case <-insertionTimeout:
					p.logger.Warnf("Inbound message channel is full !")
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
