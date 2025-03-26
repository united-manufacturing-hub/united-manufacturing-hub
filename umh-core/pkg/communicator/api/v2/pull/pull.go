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

	"github.com/getsentry/sentry-go"
	"github.com/united-manufacturing-hub/expiremap/v2/pkg/expiremap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/error_handler"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/http"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/backend_api_structs"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/helper"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/fail"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/tracing"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/shared/models"
	"go.uber.org/zap"
)

type Puller struct {
	inboundMessageChannel                 chan *models.UMHMessageWithAdditionalInfo
	shallRun                              atomic.Bool
	jwt                                   atomic.Value
	dog                                   watchdog.Iface
	insecureTLS                           bool
	userCertificateCache                  *expiremap.ExpireMap[string, *x509.Certificate]
	certChangeChannelForSubscriberHandler chan struct {
		Email string
		Cert  *x509.Certificate
	}
}

func NewPuller(jwt string, dog watchdog.Iface, inboundChannel chan *models.UMHMessageWithAdditionalInfo, insecureTLS bool, certChangeChannelForSubscriberHandler chan struct {
	Email string
	Cert  *x509.Certificate
}) *Puller {
	p := Puller{
		inboundMessageChannel:                 inboundChannel,
		shallRun:                              atomic.Bool{},
		jwt:                                   atomic.Value{},
		dog:                                   dog,
		insecureTLS:                           insecureTLS,
		userCertificateCache:                  expiremap.NewEx[string, *x509.Certificate](10*time.Second, 10*time.Second), // Refresh every 10 seconds
		certChangeChannelForSubscriberHandler: certChangeChannelForSubscriberHandler,
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
		fail.Fatal("Stop MUST NOT be used outside tests")
	}
}

func (p *Puller) pull() {
	watcherUUID := p.dog.RegisterHeartbeat("pull", 10, 600, false)
	var ticker = time.NewTicker(10 * time.Millisecond)
	for p.shallRun.Load() {
		select {
		case <-ticker.C:
			// Start a new transaction for this pull cycle
			transaction := sentry.StartTransaction(context.Background(), "pull.cycle")

			p.dog.ReportHeartbeatStatus(watcherUUID, watchdog.HEARTBEAT_STATUS_OK)
			var cookies = map[string]string{
				"token": p.jwt.Load().(string),
			}
			incomingMessages, err, status := http.GetRequest[backend_api_structs.PullPayload](transaction.Context(), http.PullEndpoint, nil, &cookies, p.insecureTLS)
			if err != nil {
				// Ignore context canceled errors
				if errors.Is(err, context.Canceled) {
					time.Sleep(1 * time.Second)
					continue
				}
				// Start a new transaction for this error
				errorTransaction := sentry.StartTransaction(context.Background(), "error.pull")
				transaction.Status = sentry.SpanStatusInternalError
				error_handler.ReportHTTPErrors(err, status, string(http.PullEndpoint), "GET", nil, nil)
				time.Sleep(1 * time.Second)
				errorTransaction.Finish()
				transaction.Finish()
				continue
			}
			error_handler.ResetErrorCounter()
			if incomingMessages == nil || incomingMessages.UMHMessages == nil || len((*incomingMessages).UMHMessages) == 0 {
				time.Sleep(1 * time.Second)
				continue
			}

			for _, message := range (*incomingMessages).UMHMessages {
				var span *sentry.Span
				if transaction != nil {
					// Create a span for each message processing
					span = transaction.StartChild("process.message")
					if message.Metadata != nil && span != nil {
						tracing.AddTraceUUIDToSpan(span, message.Metadata.TraceID)
					}
				}

				userCertificate, ok := p.userCertificateCache.Load(message.Email)
				if !ok {
					zap.S().Infof("Getting user certificate for %s", message.Email)
					cert, err := GetUserCertificate(transaction.Context(), message.Email, &cookies, p.insecureTLS)
					if err == nil && cert != nil && cert.Certificate != "" {
						zap.S().Infof("User certificate for %s found", message.Email)

						base64Decoded, err := base64.StdEncoding.DecodeString(cert.Certificate)
						if err != nil {
							zap.S().Errorf("Failed to decode user certificate: %v", err)
						}
						x509Cert, err := x509.ParseCertificate(base64Decoded)
						if err != nil {
							zap.S().Errorf("Failed to parse user certificate: %v", err)
							x509Cert = nil
						}
						p.userCertificateCache.Set(message.Email, x509Cert)
						userCertificate = &x509Cert

						// Send the new certificate to the channel
						if p.certChangeChannelForSubscriberHandler != nil {
							p.certChangeChannelForSubscriberHandler <- struct {
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
				if userCertificate == nil {
					select {
					case p.inboundMessageChannel <- &models.UMHMessageWithAdditionalInfo{
						UMHMessage:  message,
						Certificate: nil,
					}:
					case <-insertionTimeout:
						zap.S().Warnf("Inbound message channel is full !")
						p.dog.ReportHeartbeatStatus(watcherUUID, watchdog.HEARTBEAT_STATUS_WARNING)
					}
				} else {
					select {
					case p.inboundMessageChannel <- &models.UMHMessageWithAdditionalInfo{
						UMHMessage:  message,
						Certificate: *userCertificate,
					}:
					case <-insertionTimeout:
						zap.S().Warnf("Inbound message channel is full !")
						p.dog.ReportHeartbeatStatus(watcherUUID, watchdog.HEARTBEAT_STATUS_WARNING)
					}
				}
				if span != nil {
					span.Finish()
				}
			}
			if transaction != nil {
				transaction.Finish()
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
