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
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/protocol"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/communicator/transport"
)

var _ = Describe("HTTPTransport Contract", func() {
	var (
		ctx        context.Context
		cancel     context.CancelFunc
		server     *httptest.Server
		httpTrans  protocol.Transport
		messages   chan protocol.RawMessage
		mu         sync.Mutex
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		messages = make(chan protocol.RawMessage, 100)

		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/send" {
				w.WriteHeader(http.StatusOK)

				var msg map[string]interface{}
				json.NewDecoder(r.Body).Decode(&msg)

				mu.Lock()
				defer mu.Unlock()

				select {
				case messages <- protocol.RawMessage{
					From:      "test-sender",
					Payload:   []byte(msg["payload"].(string)),
					Timestamp: time.Now(),
				}:
				default:
				}
			} else if r.URL.Path == "/receive" {
				mu.Lock()
				defer mu.Unlock()

				select {
				case msg := <-messages:
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(msg)
				default:
					w.WriteHeader(http.StatusNoContent)
				}
			}
		}))

		httpTrans = transport.NewHTTPTransport(server.URL, "test-token")
	})

	AfterEach(func() {
		cancel()
		if httpTrans != nil {
			_ = httpTrans.Close()
		}
		if server != nil {
			server.Close()
		}
		close(messages)
	})

	Describe("Send/Receive roundtrip", func() {
		It("should receive messages that were sent", func() {
			msgChan, err := httpTrans.Receive(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(msgChan).ToNot(BeNil())

			testPayload := []byte("test-payload-data")
			err = httpTrans.Send(ctx, "recipient-uuid", testPayload)
			Expect(err).ToNot(HaveOccurred())

			var received protocol.RawMessage
			Eventually(msgChan).Should(Receive(&received))
			Expect(received.Payload).To(Equal(testPayload))
			Expect(received.Timestamp).To(BeTemporally("~", time.Now(), time.Second))
		})

		It("should receive multiple messages in order", func() {
			msgChan, err := httpTrans.Receive(ctx)
			Expect(err).ToNot(HaveOccurred())

			payloads := [][]byte{
				[]byte("message-1"),
				[]byte("message-2"),
				[]byte("message-3"),
			}

			for _, payload := range payloads {
				err = httpTrans.Send(ctx, "recipient-uuid", payload)
				Expect(err).ToNot(HaveOccurred())
			}

			for _, expectedPayload := range payloads {
				var received protocol.RawMessage
				Eventually(msgChan, 2*time.Second).Should(Receive(&received))
				Expect(received.Payload).To(Equal(expectedPayload))
			}
		})
	})

	Describe("Close behavior", func() {
		It("should close without error", func() {
			err := httpTrans.Close()
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return error when sending after close", func() {
			err := httpTrans.Close()
			Expect(err).ToNot(HaveOccurred())

			err = httpTrans.Send(ctx, "recipient-uuid", []byte("test"))
			Expect(err).To(HaveOccurred())
		})

		It("should return error when receiving after close", func() {
			err := httpTrans.Close()
			Expect(err).ToNot(HaveOccurred())

			_, err = httpTrans.Receive(ctx)
			Expect(err).To(HaveOccurred())
		})

		It("should be idempotent", func() {
			err := httpTrans.Close()
			Expect(err).ToNot(HaveOccurred())

			err = httpTrans.Close()
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("Error propagation", func() {
		It("should propagate network errors as TransportNetworkError", func() {
			badTransport := transport.NewHTTPTransport("http://localhost:1", "test-token")
			defer badTransport.Close()

			err := badTransport.Send(ctx, "recipient-uuid", []byte("test"))
			Expect(err).To(HaveOccurred())

			var netErr protocol.TransportNetworkError
			Expect(errors.As(err, &netErr)).To(BeTrue())
		})
	})
})
