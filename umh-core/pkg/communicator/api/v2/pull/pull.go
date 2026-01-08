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
	jwt                   atomic.Value
	dog                   watchdog.Iface
	inboundMessageChannel chan *models.UMHMessageWithAdditionalInfo
	logger                *zap.SugaredLogger
	apiURL                string
	shallRun              atomic.Bool
	insecureTLS           bool
	userCertCache         *expiremap.ExpireMap[string, *x509.Certificate]
	// rootCA is the root certificate authority for certificate chain validation
	rootCA *x509.Certificate
	// intermediateCerts are the intermediate certificates in the chain
	intermediateCerts []*x509.Certificate
	// channel to ping subscriber handler for changed certs
	certSubChan chan struct {
		Email string
		Cert  *x509.Certificate
	}
}

func NewPuller(jwt string, dog watchdog.Iface, inboundChannel chan *models.UMHMessageWithAdditionalInfo, insecureTLS bool, apiURL string, logger *zap.SugaredLogger, certSubChan chan struct {
	Email string
	Cert  *x509.Certificate
},
) *Puller {
	p := Puller{
		inboundMessageChannel: inboundChannel,
		shallRun:              atomic.Bool{},
		jwt:                   atomic.Value{},
		dog:                   dog,
		insecureTLS:           insecureTLS,
		apiURL:                apiURL,
		logger:                logger,
		userCertCache:         expiremap.NewEx[string, *x509.Certificate](10*time.Second, 10*time.Second),
		certSubChan:           certSubChan,
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
			// Circuit breaker: prevent log spam when backend unreachable.
			// 1 second provides breathing room without impacting recovery time,
			// since this runs async and won't block data infrastructure.
			time.Sleep(1 * time.Second)

			continue
		}

		error_handler.ResetErrorCounter()

		if incomingMessages == nil || incomingMessages.UMHMessages == nil || len(incomingMessages.UMHMessages) == 0 {
			time.Sleep(1 * time.Second)

			continue
		}

		for _, message := range incomingMessages.UMHMessages {

			userCertificate, ok := p.userCertCache.Load(message.Email)
			if !ok {
				zap.S().Infof("Getting user certificate for %s", message.Email)
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
					p.userCertCache.Set(message.Email, x509Cert)
					userCertificate = &x509Cert

					// Parse and cache RootCA if provided (only need to do this once)
					if p.rootCA == nil && cert.RootCA != "" {
						rootCADecoded, err := base64.StdEncoding.DecodeString(cert.RootCA)
						if err != nil {
							p.logger.Errorf("Failed to decode root CA: %v", err)
						} else {
							p.rootCA, err = x509.ParseCertificate(rootCADecoded)
							if err != nil {
								p.logger.Errorf("Failed to parse root CA: %v", err)
								p.rootCA = nil
							}
						}
					}

					// Parse and cache intermediate certs if provided (only need to do this once)
					if p.intermediateCerts == nil && len(cert.IntermediateCerts) > 0 {
						p.intermediateCerts = make([]*x509.Certificate, 0, len(cert.IntermediateCerts))
						for _, intermediateCert := range cert.IntermediateCerts {
							intermediateDecoded, err := base64.StdEncoding.DecodeString(intermediateCert)
							if err != nil {
								p.logger.Errorf("Failed to decode intermediate cert: %v", err)
								continue
							}
							parsedCert, err := x509.ParseCertificate(intermediateDecoded)
							if err != nil {
								p.logger.Errorf("Failed to parse intermediate cert: %v", err)
								continue
							}
							p.intermediateCerts = append(p.intermediateCerts, parsedCert)
						}
					}

					// Send the new certificate to the channel
					if p.certSubChan != nil {
						p.certSubChan <- struct {
							Email string
							Cert  *x509.Certificate
						}{
							Email: message.Email,
							Cert:  x509Cert,
						}
					}
				} else {
					zap.S().Errorf("Failed to get user certificate: %v", err)
				}
			}

			insertionTimeout := time.After(10 * time.Second)

			msgWithInfo := &models.UMHMessageWithAdditionalInfo{
				UMHMessage:        message,
				RootCA:            p.rootCA,
				IntermediateCerts: p.intermediateCerts,
			}
			if userCertificate != nil {
				msgWithInfo.Certificate = *userCertificate
			}

			select {
			case p.inboundMessageChannel <- msgWithInfo:
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
	UserEmail          string   `json:"userEmail"`
	Certificate        string   `json:"certificate"`
	RootCA             string   `json:"rootCA,omitempty"`
	IntermediateCerts  []string `json:"intermediateCerts,omitempty"`
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
