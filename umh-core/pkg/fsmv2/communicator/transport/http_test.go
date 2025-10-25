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

package transport_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/protocol"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/communicator/transport"
)

func TestHTTPTransport(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HTTPTransport Suite")
}

var _ = Describe("HTTPTransport", func() {
	var (
		ctx          context.Context
		cancel       context.CancelFunc
		server       *httptest.Server
		httpTransport *transport.HTTPTransport
		jwtToken     string
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		jwtToken = "test-jwt-token"
	})

	AfterEach(func() {
		cancel()
		if httpTransport != nil {
			_ = httpTransport.Close()
		}
		if server != nil {
			server.Close()
		}
	})

	Describe("Send", func() {
		Context("with successful HTTP POST", func() {
			It("should send payload to relay server via POST", func() {
				receivedPayload := make(chan []byte, 1)
				receivedRecipient := make(chan string, 1)

				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Expect(r.Method).To(Equal("POST"))
					Expect(r.Header.Get("Authorization")).To(Equal("Bearer " + jwtToken))
					Expect(r.Header.Get("Content-Type")).To(Equal("application/json"))

					body, err := io.ReadAll(r.Body)
					Expect(err).ToNot(HaveOccurred())

					var msg map[string]interface{}
					err = json.Unmarshal(body, &msg)
					Expect(err).ToNot(HaveOccurred())

					receivedRecipient <- msg["to"].(string)
					receivedPayload <- []byte(msg["payload"].(string))

					w.WriteHeader(http.StatusOK)
				}))

				httpTransport = transport.NewHTTPTransport(server.URL, jwtToken)

				testPayload := []byte("test-message-data")
				testRecipient := "recipient-uuid-123"

				err := httpTransport.Send(ctx, testRecipient, testPayload)
				Expect(err).ToNot(HaveOccurred())

				Eventually(receivedRecipient).Should(Receive(Equal(testRecipient)))
				Eventually(receivedPayload).Should(Receive(Equal(testPayload)))
			})
		})

		Context("with JWT authentication", func() {
			It("should include JWT token in Authorization header", func() {
				headerReceived := make(chan string, 1)

				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					headerReceived <- r.Header.Get("Authorization")
					w.WriteHeader(http.StatusOK)
				}))

				httpTransport = transport.NewHTTPTransport(server.URL, jwtToken)

				err := httpTransport.Send(ctx, "recipient", []byte("test"))
				Expect(err).ToNot(HaveOccurred())

				Eventually(headerReceived).Should(Receive(Equal("Bearer " + jwtToken)))
			})
		})

		Context("with network errors", func() {
			It("should return TransportNetworkError on connection refused", func() {
				httpTransport = transport.NewHTTPTransport("http://localhost:1", jwtToken)

				err := httpTransport.Send(ctx, "recipient", []byte("test"))
				Expect(err).To(HaveOccurred())

				var networkErr protocol.TransportNetworkError
				Expect(errors.As(err, &networkErr)).To(BeTrue())
			})
		})

		Context("with authentication errors", func() {
			It("should return TransportAuthError on 401 Unauthorized", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusUnauthorized)
				}))

				httpTransport = transport.NewHTTPTransport(server.URL, "invalid-token")

				err := httpTransport.Send(ctx, "recipient", []byte("test"))
				Expect(err).To(HaveOccurred())

				var authErr protocol.TransportAuthError
				Expect(errors.As(err, &authErr)).To(BeTrue())
			})

			It("should return TransportAuthError on 403 Forbidden", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusForbidden)
				}))

				httpTransport = transport.NewHTTPTransport(server.URL, "invalid-token")

				err := httpTransport.Send(ctx, "recipient", []byte("test"))
				Expect(err).To(HaveOccurred())

				var authErr protocol.TransportAuthError
				Expect(errors.As(err, &authErr)).To(BeTrue())
			})
		})

		Context("with context cancellation", func() {
			It("should respect context cancellation", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					time.Sleep(100 * time.Millisecond)
					w.WriteHeader(http.StatusOK)
				}))

				httpTransport = transport.NewHTTPTransport(server.URL, jwtToken)

				cancelCtx, cancelFn := context.WithCancel(ctx)
				cancelFn()

				err := httpTransport.Send(cancelCtx, "recipient", []byte("test"))
				Expect(err).To(HaveOccurred())
			})
		})

		Context("with timeout", func() {
			It("should return TransportTimeoutError on request timeout", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					time.Sleep(5 * time.Second)
				}))

				httpTransport = transport.NewHTTPTransport(server.URL, jwtToken)

				timeoutCtx, timeoutCancel := context.WithTimeout(ctx, 100*time.Millisecond)
				defer timeoutCancel()

				err := httpTransport.Send(timeoutCtx, "recipient", []byte("test"))
				Expect(err).To(HaveOccurred())

				var timeoutErr protocol.TransportTimeoutError
				Expect(errors.As(err, &timeoutErr)).To(BeTrue())
			})
		})

		Context("when transport is closed", func() {
			It("should return error when sending after close", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}))

				httpTransport = transport.NewHTTPTransport(server.URL, jwtToken)
				err := httpTransport.Close()
				Expect(err).ToNot(HaveOccurred())

				err = httpTransport.Send(ctx, "recipient", []byte("test"))
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("closed"))
			})
		})
	})

	Describe("Receive", func() {
		Context("with long-polling", func() {
			It("should receive messages from relay server via long-polling", func() {
				messagesSent := make(chan bool, 1)

				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Expect(r.Method).To(Equal("GET"))
					Expect(r.Header.Get("Authorization")).To(Equal("Bearer " + jwtToken))

					msg := protocol.RawMessage{
						From:      "sender-uuid",
						Payload:   []byte("test-payload"),
						Timestamp: time.Now(),
					}

					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(msg)
					messagesSent <- true
				}))

				httpTransport = transport.NewHTTPTransport(server.URL, jwtToken)

				msgChan, err := httpTransport.Receive(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(msgChan).ToNot(BeNil())

				Eventually(messagesSent).Should(Receive(BeTrue()))

				var received protocol.RawMessage
				Eventually(msgChan).Should(Receive(&received))
				Expect(received.From).To(Equal("sender-uuid"))
				Expect(received.Payload).To(Equal([]byte("test-payload")))
			})
		})

		Context("with authentication errors", func() {
			It("should handle 401 errors in polling loop", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusUnauthorized)
				}))

				httpTransport = transport.NewHTTPTransport(server.URL, "invalid-token")

				msgChan, err := httpTransport.Receive(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(msgChan).ToNot(BeNil())
			})
		})

		Context("when transport is closed", func() {
			It("should return error when receiving after close", func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}))

				httpTransport = transport.NewHTTPTransport(server.URL, jwtToken)
				err := httpTransport.Close()
				Expect(err).ToNot(HaveOccurred())

				_, err = httpTransport.Receive(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("closed"))
			})
		})

		Context("with context cancellation", func() {
			It("should stop polling when context is cancelled", func() {
				pollCount := 0
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					pollCount++
					time.Sleep(50 * time.Millisecond)
					w.WriteHeader(http.StatusNoContent)
				}))

				httpTransport = transport.NewHTTPTransport(server.URL, jwtToken)

				cancelCtx, cancelFn := context.WithCancel(ctx)
				msgChan, err := httpTransport.Receive(cancelCtx)
				Expect(err).ToNot(HaveOccurred())

				time.Sleep(100 * time.Millisecond)
				cancelFn()

				Eventually(msgChan).Should(BeClosed())
			})
		})
	})

	Describe("Close", func() {
		It("should close without error", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			httpTransport = transport.NewHTTPTransport(server.URL, jwtToken)

			err := httpTransport.Close()
			Expect(err).ToNot(HaveOccurred())
		})

		It("should be idempotent", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			httpTransport = transport.NewHTTPTransport(server.URL, jwtToken)

			err := httpTransport.Close()
			Expect(err).ToNot(HaveOccurred())

			err = httpTransport.Close()
			Expect(err).ToNot(HaveOccurred())
		})

		It("should stop ongoing Receive polling", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(50 * time.Millisecond)
				w.WriteHeader(http.StatusNoContent)
			}))

			httpTransport = transport.NewHTTPTransport(server.URL, jwtToken)

			msgChan, err := httpTransport.Receive(ctx)
			Expect(err).ToNot(HaveOccurred())

			time.Sleep(100 * time.Millisecond)

			err = httpTransport.Close()
			Expect(err).ToNot(HaveOccurred())

			Eventually(msgChan).Should(BeClosed())
		})
	})
})
