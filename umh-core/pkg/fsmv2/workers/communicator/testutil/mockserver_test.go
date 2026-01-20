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

package testutil_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/testutil"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
)

func TestMockServer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "MockServer Suite")
}

var _ = Describe("MockRelayServer", func() {
	var server *testutil.MockRelayServer

	BeforeEach(func() {
		server = testutil.NewMockRelayServer()
	})

	AfterEach(func() {
		server.Close()
	})

	Describe("Authentication", func() {
		It("accepts login and returns JWT token in cookie and uuid/name in body", func() {
			// Prepare login request
			loginReq := map[string]string{
				"instanceUUID": "test-instance-123",
				"email":        "test@example.com",
			}
			body, err := json.Marshal(loginReq)
			Expect(err).NotTo(HaveOccurred())

			// Send login request
			resp, err := http.Post(server.URL()+"/v2/instance/login", "application/json", bytes.NewBuffer(body))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Verify response
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			// Bug #6 fix: real backend returns uuid and name in response body
			var authResp map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&authResp)
			Expect(err).NotTo(HaveOccurred())
			Expect(authResp["uuid"]).NotTo(BeEmpty())
			Expect(authResp["name"]).NotTo(BeEmpty())

			// Verify JWT cookie is set (real backend behavior)
			cookies := resp.Cookies()
			var tokenCookie *http.Cookie
			for _, c := range cookies {
				if c.Name == "token" {
					tokenCookie = c

					break
				}
			}
			Expect(tokenCookie).NotTo(BeNil())
			Expect(tokenCookie.Value).NotTo(BeEmpty())
		})

		It("tracks authentication calls", func() {
			Expect(server.AuthCallCount()).To(Equal(0))

			// First login
			loginReq := map[string]string{
				"instanceUUID": "test-instance-123",
				"email":        "test@example.com",
			}
			body, _ := json.Marshal(loginReq)
			resp, err := http.Post(server.URL()+"/v2/instance/login", "application/json", bytes.NewBuffer(body))
			Expect(err).NotTo(HaveOccurred())
			_ = resp.Body.Close()
			Expect(server.AuthCallCount()).To(Equal(1))

			// Second login
			resp, err = http.Post(server.URL()+"/v2/instance/login", "application/json", bytes.NewBuffer(body))
			Expect(err).NotTo(HaveOccurred())
			_ = resp.Body.Close()
			Expect(server.AuthCallCount()).To(Equal(2))
		})
	})

	Describe("Pull", func() {
		It("returns queued messages", func() {
			// Queue some messages
			msg1 := &transport.UMHMessage{
				InstanceUUID: "test-instance-123",
				Content:      "message-1",
				Email:        "test@example.com",
			}
			msg2 := &transport.UMHMessage{
				InstanceUUID: "test-instance-123",
				Content:      "message-2",
				Email:        "test@example.com",
			}
			server.QueuePullMessage(msg1)
			server.QueuePullMessage(msg2)

			// Create request with JWT cookie
			req, err := http.NewRequest(http.MethodGet, server.URL()+"/v2/instance/pull", nil)
			Expect(err).NotTo(HaveOccurred())
			req.AddCookie(&http.Cookie{Name: "token", Value: "test-jwt-token"})

			// Send request
			client := &http.Client{}
			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Verify response
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var payload struct {
				UMHMessages []*transport.UMHMessage `json:"UMHMessages"`
			}
			err = json.NewDecoder(resp.Body).Decode(&payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(payload.UMHMessages).To(HaveLen(2))
			Expect(payload.UMHMessages[0].Content).To(Equal("message-1"))
			Expect(payload.UMHMessages[1].Content).To(Equal("message-2"))
		})

		It("returns empty when no messages queued", func() {
			// Create request with JWT cookie
			req, err := http.NewRequest(http.MethodGet, server.URL()+"/v2/instance/pull", nil)
			Expect(err).NotTo(HaveOccurred())
			req.AddCookie(&http.Cookie{Name: "token", Value: "test-jwt-token"})

			// Send request
			client := &http.Client{}
			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Verify response
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var payload struct {
				UMHMessages []*transport.UMHMessage `json:"UMHMessages"`
			}
			err = json.NewDecoder(resp.Body).Decode(&payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(payload.UMHMessages).To(BeEmpty())
		})

		It("clears queue after pull", func() {
			// Queue a message
			msg := &transport.UMHMessage{
				InstanceUUID: "test-instance-123",
				Content:      "message-1",
				Email:        "test@example.com",
			}
			server.QueuePullMessage(msg)

			// First pull
			req, _ := http.NewRequest(http.MethodGet, server.URL()+"/v2/instance/pull", nil)
			req.AddCookie(&http.Cookie{Name: "token", Value: "test-jwt-token"})
			client := &http.Client{}
			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			_ = resp.Body.Close()

			// Second pull should return empty
			req, _ = http.NewRequest(http.MethodGet, server.URL()+"/v2/instance/pull", nil)
			req.AddCookie(&http.Cookie{Name: "token", Value: "test-jwt-token"})
			resp, err = client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			var payload struct {
				UMHMessages []*transport.UMHMessage `json:"UMHMessages"`
			}
			err = json.NewDecoder(resp.Body).Decode(&payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(payload.UMHMessages).To(BeEmpty())
		})
	})

	Describe("Push", func() {
		It("records pushed messages", func() {
			// Prepare push request
			msg1 := &transport.UMHMessage{
				InstanceUUID: "test-instance-123",
				Content:      "pushed-message-1",
				Email:        "test@example.com",
			}
			msg2 := &transport.UMHMessage{
				InstanceUUID: "test-instance-123",
				Content:      "pushed-message-2",
				Email:        "test@example.com",
			}
			payload := struct {
				UMHMessages []*transport.UMHMessage `json:"UMHMessages"`
			}{
				UMHMessages: []*transport.UMHMessage{msg1, msg2},
			}
			body, err := json.Marshal(payload)
			Expect(err).NotTo(HaveOccurred())

			// Create request with JWT cookie
			req, err := http.NewRequest(http.MethodPost, server.URL()+"/v2/instance/push", bytes.NewBuffer(body))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(&http.Cookie{Name: "token", Value: "test-jwt-token"})

			// Send request
			client := &http.Client{}
			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Verify response
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			// Verify messages were recorded
			pushed := server.GetPushedMessages()
			Expect(pushed).To(HaveLen(2))
			Expect(pushed[0].Content).To(Equal("pushed-message-1"))
			Expect(pushed[1].Content).To(Equal("pushed-message-2"))
		})

		It("allows clearing pushed messages", func() {
			// Push a message
			msg := &transport.UMHMessage{
				InstanceUUID: "test-instance-123",
				Content:      "pushed-message",
				Email:        "test@example.com",
			}
			payload := struct {
				UMHMessages []*transport.UMHMessage `json:"UMHMessages"`
			}{
				UMHMessages: []*transport.UMHMessage{msg},
			}
			body, _ := json.Marshal(payload)

			req, _ := http.NewRequest(http.MethodPost, server.URL()+"/v2/instance/push", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(&http.Cookie{Name: "token", Value: "test-jwt-token"})
			client := &http.Client{}
			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			_ = resp.Body.Close()

			// Verify message was recorded
			Expect(server.GetPushedMessages()).To(HaveLen(1))

			// Clear and verify
			server.ClearPushedMessages()
			Expect(server.GetPushedMessages()).To(BeEmpty())
		})
	})

	Describe("Error Injection", func() {
		It("simulates 401 for re-auth testing", func() {
			server.SimulateAuthExpiry()

			// Try to pull - should get 401
			req, err := http.NewRequest(http.MethodGet, server.URL()+"/v2/instance/pull", nil)
			Expect(err).NotTo(HaveOccurred())
			req.AddCookie(&http.Cookie{Name: "token", Value: "test-jwt-token"})

			client := &http.Client{}
			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		})

		It("simulates server errors (500)", func() {
			server.SimulateServerError(http.StatusInternalServerError)

			// Try to pull - should get 500
			req, err := http.NewRequest(http.MethodGet, server.URL()+"/v2/instance/pull", nil)
			Expect(err).NotTo(HaveOccurred())
			req.AddCookie(&http.Cookie{Name: "token", Value: "test-jwt-token"})

			client := &http.Client{}
			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusInternalServerError))
		})

		It("simulates slow response for timeout testing", func() {
			server.SimulateSlowResponse(200 * time.Millisecond)

			// Create client with short timeout
			client := &http.Client{
				Timeout: 50 * time.Millisecond,
			}

			req, err := http.NewRequest(http.MethodGet, server.URL()+"/v2/instance/pull", nil)
			Expect(err).NotTo(HaveOccurred())
			req.AddCookie(&http.Cookie{Name: "token", Value: "test-jwt-token"})

			// Should timeout
			_, err = client.Do(req)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("deadline exceeded"))
		})

		It("error injection is one-time only", func() {
			server.SimulateServerError(http.StatusInternalServerError)

			// First request gets error
			req, _ := http.NewRequest(http.MethodGet, server.URL()+"/v2/instance/pull", nil)
			req.AddCookie(&http.Cookie{Name: "token", Value: "test-jwt-token"})
			client := &http.Client{}
			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusInternalServerError))
			_ = resp.Body.Close()

			// Second request succeeds
			req, _ = http.NewRequest(http.MethodGet, server.URL()+"/v2/instance/pull", nil)
			req.AddCookie(&http.Cookie{Name: "token", Value: "test-jwt-token"})
			resp, err = client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			_ = resp.Body.Close()
		})
	})

	Describe("Connection Header Tracking for Bug #3 Validation", func() {
		It("tracks Connection headers from requests", func() {
			// Create transport with DisableKeepAlives (like the real HTTPTransport)
			client := &http.Client{
				Transport: &http.Transport{
					DisableKeepAlives: true,
				},
			}

			// Make two requests
			req1, _ := http.NewRequest(http.MethodGet, server.URL()+"/v2/instance/pull", nil)
			req1.AddCookie(&http.Cookie{Name: "token", Value: "test-jwt-token"})
			resp, err := client.Do(req1)
			Expect(err).NotTo(HaveOccurred())
			_ = resp.Body.Close()

			req2, _ := http.NewRequest(http.MethodGet, server.URL()+"/v2/instance/pull", nil)
			req2.AddCookie(&http.Cookie{Name: "token", Value: "test-jwt-token"})
			resp, err = client.Do(req2)
			Expect(err).NotTo(HaveOccurred())
			_ = resp.Body.Close()

			// Verify Connection headers were tracked
			headers := server.GetReceivedConnectionHeaders()
			Expect(headers).To(HaveLen(2))
			// When DisableKeepAlives=true, Go HTTP client sends "Connection: close"
			for _, header := range headers {
				Expect(header).To(Equal("close"))
			}
		})
	})
})
