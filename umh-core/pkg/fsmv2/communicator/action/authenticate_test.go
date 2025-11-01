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

package action_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/communicator"
)

var _ = Describe("AuthenticateAction", func() {
	var (
		action        *communicator.AuthenticateAction
		observedState *communicator.CommunicatorObservedState
		server        *httptest.Server
		ctx           context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		observedState = &communicator.CommunicatorObservedState{}
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	Describe("Execute", func() {
		Context("when authentication succeeds", func() {
			BeforeEach(func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Expect(r.Method).To(Equal("POST"))
					Expect(r.URL.Path).To(Equal("/authenticate"))
					Expect(r.Header.Get("Content-Type")).To(Equal("application/json"))

					var authReq map[string]string
					err := json.NewDecoder(r.Body).Decode(&authReq)
					Expect(err).ToNot(HaveOccurred())
					Expect(authReq["instanceUUID"]).To(Equal("test-uuid"))
					Expect(authReq["authToken"]).To(Equal("test-token"))

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(map[string]interface{}{
						"token":     "jwt-token-123",
						"expiresAt": 1234567890,
					})
				}))

				action = communicator.NewAuthenticateAction(
					server.URL,
					"test-uuid",
					"test-token",
					observedState,
				)
			})

			It("should send authentication request with correct credentials", func() {
				err := action.Execute(ctx)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should store JWT token in observed state", func() {
				err := action.Execute(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(observedState.GetJWTToken()).To(Equal("jwt-token-123"))
			})

			It("should mark as authenticated", func() {
				err := action.Execute(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(observedState.IsAuthenticated()).To(BeTrue())
			})

			It("should store token expiration timestamp", func() {
				err := action.Execute(ctx)
				Expect(err).ToNot(HaveOccurred())

				expiresAt := observedState.GetTokenExpiresAt()
				Expect(expiresAt.IsZero()).To(BeFalse(), "Token expiration should be set")
				Expect(expiresAt.Unix()).To(Equal(int64(1234567890)), "Should store exact expiration from server")
			})

			It("should be idempotent (not re-auth if already authenticated with valid token)", func() {
				callCount := 0
				futureExpiry := time.Now().Add(1 * time.Hour).Unix()
				server.Close()
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					callCount++
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(map[string]interface{}{
						"token":     "jwt-token-123",
						"expiresAt": futureExpiry,
					})
				}))

				action = communicator.NewAuthenticateAction(
					server.URL,
					"test-uuid",
					"test-token",
					observedState,
				)

				// First call - should authenticate
				err := action.Execute(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(callCount).To(Equal(1))

				// Second call - should skip (already authenticated with valid token)
				err = action.Execute(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(callCount).To(Equal(1), "Should not call server again when already authenticated with valid token")
			})
		})

		Context("when server does not provide expiresAt", func() {
			BeforeEach(func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(map[string]interface{}{
						"token": "jwt-token-no-expiry",
					})
				}))

				action = communicator.NewAuthenticateAction(
					server.URL,
					"test-uuid",
					"test-token",
					observedState,
				)
			})

			It("should still authenticate successfully", func() {
				err := action.Execute(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(observedState.IsAuthenticated()).To(BeTrue())
				Expect(observedState.GetJWTToken()).To(Equal("jwt-token-no-expiry"))
			})

			It("should set default expiration (24 hours from now)", func() {
				beforeExec := time.Now()
				err := action.Execute(ctx)
				Expect(err).ToNot(HaveOccurred())

				expiresAt := observedState.GetTokenExpiresAt()
				Expect(expiresAt.IsZero()).To(BeFalse(), "Should set default expiration")

				expectedExpiry := beforeExec.Add(24 * time.Hour)
				Expect(expiresAt).To(BeTemporally("~", expectedExpiry, 5*time.Second), "Should default to ~24 hours")
			})
		})

		Context("when authentication fails", func() {
			BeforeEach(func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusUnauthorized)
					w.Write([]byte("Invalid credentials"))
				}))

				action = communicator.NewAuthenticateAction(
					server.URL,
					"test-uuid",
					"wrong-token",
					observedState,
				)
			})

			It("should return error", func() {
				err := action.Execute(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("authentication failed with status 401"))
			})

			It("should not mark as authenticated", func() {
				action.Execute(ctx)
				Expect(observedState.IsAuthenticated()).To(BeFalse())
			})

			It("should not store JWT token", func() {
				action.Execute(ctx)
				Expect(observedState.GetJWTToken()).To(BeEmpty())
			})
		})

		Context("when server is unreachable", func() {
			BeforeEach(func() {
				action = communicator.NewAuthenticateAction(
					"http://localhost:9999",
					"test-uuid",
					"test-token",
					observedState,
				)
			})

			It("should return connection error", func() {
				err := action.Execute(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("authentication request failed"))
			})
		})

		Context("when context is cancelled", func() {
			BeforeEach(func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Never respond
					<-r.Context().Done()
				}))

				action = communicator.NewAuthenticateAction(
					server.URL,
					"test-uuid",
					"test-token",
					observedState,
				)
			})

			It("should respect context cancellation", func() {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()

				err := action.Execute(ctx)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("Name", func() {
		It("should return action name", func() {
			action = communicator.NewAuthenticateAction(
				"http://localhost",
				"test-uuid",
				"test-token",
				observedState,
			)
			Expect(action.Name()).To(Equal("Authenticate"))
		})
	})
})
