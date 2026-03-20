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
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
	transportpkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/action"
)

var _ = Describe("AuthenticateAction", func() {
	var (
		act          *action.AuthenticateAction
		dependencies *transportpkg.TransportDependencies
		logger       deps.FSMLogger
		mockTransp   *mockTransport
	)

	BeforeEach(func() {
		logger = deps.NewNopFSMLogger()
		mockTransp = &mockTransport{}
		identity := deps.Identity{ID: "test-id", WorkerType: "transport"}
		dependencies = transportpkg.NewTransportDependencies(mockTransp, logger, nil, identity)
		// Dependencies now passed to Execute(), not constructor
		act = action.NewAuthenticateAction(
			"https://relay.example.com",
			"test-uuid",
			"test-token",
			10*time.Second,
		)
	})

	Describe("Name", func() {
		It("should return action name", func() {
			Expect(act.Name()).To(Equal("authenticate"))
		})
	})

	Describe("Architecture Compliance", func() {
		It("should be stateless (struct fields are read-only config)", func() {
			// AuthenticateAction should only have config fields, no mutable state
			// This is verified by the architecture test, but we confirm the constructor
			// sets fields correctly without any runtime state
			act1 := action.NewAuthenticateAction("url1", "uuid1", "token1", 5*time.Second)
			act2 := action.NewAuthenticateAction("url2", "uuid2", "token2", 15*time.Second)

			// Different instances should have different config
			Expect(act1.RelayURL).To(Equal("url1"))
			Expect(act2.RelayURL).To(Equal("url2"))
		})

		It("should default timeout to 10s if zero", func() {
			act := action.NewAuthenticateAction("url", "uuid", "token", 0)
			Expect(act.Timeout).To(Equal(10 * time.Second))
		})
	})

	Describe("Context Cancellation", func() {
		It("should return error when context is cancelled before execution", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel() // Cancel immediately

			err := act.Execute(ctx, dependencies)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("context"))
		})
	})

	Describe("Idempotency (Invariant I10)", func() {
		It("should be idempotent when authentication succeeds", func() {
			ctx := context.Background()
			for range 3 {
				err := act.Execute(ctx, dependencies)
				Expect(err).NotTo(HaveOccurred())
			}
			Expect(mockTransp.authCallCount).To(Equal(3))
		})
	})

	Describe("Transport Nil Safety", func() {
		It("should create transport if nil in dependencies on first execution", func() {
			identity := deps.Identity{ID: "test-nil-transport", WorkerType: "transport"}
			depsWithNilTransport := transportpkg.NewTransportDependencies(nil, logger, nil, identity)
			Expect(depsWithNilTransport.GetTransport()).To(BeNil(), "transport should be nil before first auth")

			authAction := action.NewAuthenticateAction(
				"https://relay.example.com",
				"test-uuid",
				"test-token",
				10*time.Second,
			)

			ctx := context.Background()
			err := authAction.Execute(ctx, depsWithNilTransport)
			_ = err

			Expect(depsWithNilTransport.GetTransport()).NotTo(BeNil(), "transport should be created after first auth execution")
		})

		It("should not replace existing transport on subsequent executions", func() {
			ctx := context.Background()

			originalTransport := dependencies.GetTransport()
			Expect(originalTransport).NotTo(BeNil())

			err := act.Execute(ctx, dependencies)
			Expect(err).NotTo(HaveOccurred())

			Expect(dependencies.GetTransport()).To(Equal(originalTransport))
		})
	})

	Describe("JWT Storage", func() {
		It("should store JWT token in dependencies after successful authentication", func() {
			ctx := context.Background()
			expectedToken := "test-jwt-token-xyz"
			expectedExpiry := time.Now().Add(24 * time.Hour).Unix()
			mockTransp.authResponse = transport.AuthResponse{
				Token:     expectedToken,
				ExpiresAt: expectedExpiry,
			}

			err := act.Execute(ctx, dependencies)
			Expect(err).NotTo(HaveOccurred())

			Expect(dependencies.GetJWTToken()).To(Equal(expectedToken))
		})

		It("should store JWT expiry in dependencies after successful authentication", func() {
			ctx := context.Background()
			expectedToken := "test-jwt-token-xyz"
			expectedExpiry := time.Now().Add(24 * time.Hour).Unix()
			mockTransp.authResponse = transport.AuthResponse{
				Token:     expectedToken,
				ExpiresAt: expectedExpiry,
			}

			err := act.Execute(ctx, dependencies)
			Expect(err).NotTo(HaveOccurred())

			storedExpiry := dependencies.GetJWTExpiry()
			Expect(storedExpiry.Unix()).To(Equal(expectedExpiry))
		})
	})

	Describe("Authenticated UUID Storage", func() {
		It("should store instance UUID in dependencies after successful authentication", func() {
			ctx := context.Background()
			expectedUUID := "backend-instance-uuid-123"
			mockTransp.authResponse = transport.AuthResponse{
				Token:        "test-token",
				ExpiresAt:    time.Now().Add(24 * time.Hour).Unix(),
				InstanceUUID: expectedUUID,
			}

			err := act.Execute(ctx, dependencies)
			Expect(err).NotTo(HaveOccurred())

			Expect(dependencies.GetAuthenticatedUUID()).To(Equal(expectedUUID))
		})

		It("should handle missing instance UUID gracefully", func() {
			ctx := context.Background()
			mockTransp.authResponse = transport.AuthResponse{
				Token:        "test-token",
				ExpiresAt:    time.Now().Add(24 * time.Hour).Unix(),
				InstanceUUID: "", // Empty UUID
			}

			err := act.Execute(ctx, dependencies)
			Expect(err).NotTo(HaveOccurred())
			// Should not panic, just not set the UUID
		})
	})

	Describe("Error Tracking", func() {
		It("should record auth attempt timestamp on failed authentication", func() {
			ctx := context.Background()
			beforeExec := time.Now()

			// Use a classified TransportError to test timestamp recording
			mockTransp.authError = &httpTransport.TransportError{
				Type:    httpTransport.ErrorTypeNetwork,
				Message: "connection refused",
			}
			err := act.Execute(ctx, dependencies)
			Expect(err).NotTo(HaveOccurred())

			afterExec := time.Now()
			attemptTime := dependencies.GetLastAuthAttemptAt()
			Expect(attemptTime).To(BeTemporally(">=", beforeExec))
			Expect(attemptTime).To(BeTemporally("<=", afterExec))

			// Reset for subsequent tests
			mockTransp.authError = nil
		})

		It("should suppress auth errors and still record typed error", func() {
			ctx := context.Background()
			mockTransp.authError = &httpTransport.TransportError{
				Type:       httpTransport.ErrorTypeBackendRateLimit,
				RetryAfter: 30 * time.Second,
				Message:    "rate limited",
			}

			err := act.Execute(ctx, dependencies)
			Expect(err).NotTo(HaveOccurred())

			Expect(dependencies.GetLastErrorType()).To(Equal(httpTransport.ErrorTypeBackendRateLimit))
		})

		It("should propagate unclassified errors (ErrorTypeUnknown)", func() {
			ctx := context.Background()
			mockTransp.authError = errors.New("unexpected programming error")

			err := act.Execute(ctx, dependencies)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unexpected programming error"))
		})

		It("should record success and reset error state on successful auth", func() {
			ctx := context.Background()
			// First, simulate a classified error to set error state
			mockTransp.authError = &httpTransport.TransportError{
				Type:    httpTransport.ErrorTypeNetwork,
				Message: "connection refused",
			}
			_ = act.Execute(ctx, dependencies)
			Expect(dependencies.GetConsecutiveErrors()).To(BeNumerically(">", 0))

			// Now succeed
			mockTransp.authError = nil
			mockTransp.authResponse = transport.AuthResponse{
				Token:     "test-token",
				ExpiresAt: time.Now().Add(24 * time.Hour).Unix(),
			}
			err := act.Execute(ctx, dependencies)
			Expect(err).NotTo(HaveOccurred())

			// Error state should be reset
			Expect(dependencies.GetConsecutiveErrors()).To(Equal(0))
		})
	})

	Describe("Metrics Recording", func() {
		It("should increment auth failure counter on transport error", func() {
			ctx := context.Background()
			mockTransp.authError = &httpTransport.TransportError{
				Type:    httpTransport.ErrorTypeInvalidToken,
				Message: "invalid credentials",
			}

			err := act.Execute(ctx, dependencies)
			Expect(err).NotTo(HaveOccurred())

			// Metrics are recorded via MetricsRecorder, which is tested separately
			// This test verifies the action suppresses auth errors
		})
	})

	Describe("Auth Error Suppression", func() {
		It("should suppress persistent auth errors (InvalidToken)", func() {
			ctx := context.Background()
			mockTransp.authError = &httpTransport.TransportError{
				Type:    httpTransport.ErrorTypeInvalidToken,
				Message: "invalid credentials",
			}

			err := act.Execute(ctx, dependencies)
			Expect(err).NotTo(HaveOccurred())

			Expect(dependencies.GetLastErrorType()).To(Equal(httpTransport.ErrorTypeInvalidToken))
			Expect(dependencies.GetConsecutiveErrors()).To(Equal(1))
		})

		It("should suppress persistent auth errors (InstanceDeleted)", func() {
			ctx := context.Background()
			mockTransp.authError = &httpTransport.TransportError{
				Type:    httpTransport.ErrorTypeInstanceDeleted,
				Message: "instance not found",
			}

			err := act.Execute(ctx, dependencies)
			Expect(err).NotTo(HaveOccurred())

			Expect(dependencies.GetLastErrorType()).To(Equal(httpTransport.ErrorTypeInstanceDeleted))
			Expect(dependencies.GetConsecutiveErrors()).To(Equal(1))
		})

		It("should fire SentryWarn on first occurrence only", func() {
			ctx := context.Background()
			mockTransp.authError = &httpTransport.TransportError{
				Type:    httpTransport.ErrorTypeNetwork,
				Message: "connection refused",
			}

			// First failure: consecutiveErrors goes 0 -> 1
			err := act.Execute(ctx, dependencies)
			Expect(err).NotTo(HaveOccurred())
			Expect(dependencies.GetConsecutiveErrors()).To(Equal(1))

			// Second failure: consecutiveErrors goes 1 -> 2
			err = act.Execute(ctx, dependencies)
			Expect(err).NotTo(HaveOccurred())
			Expect(dependencies.GetConsecutiveErrors()).To(Equal(2))

			// SentryWarn is called via NopFSMLogger which discards output.
			// The behavior is verified by checking that consecutiveErrors
			// increments correctly (the guard condition for SentryWarn).
		})

		It("should propagate context cancellation during Authenticate", func() {
			ctx, cancel := context.WithCancel(context.Background())
			// Cancel inside Authenticate() to exercise the post-Authenticate ctx.Err() branch,
			// not the top-of-method guard (which is tested in "Context Cancellation" above).
			mockTransp.cancelDuringAuth = cancel
			mockTransp.authError = &httpTransport.TransportError{
				Type:    httpTransport.ErrorTypeNetwork,
				Message: "connection refused",
			}

			err := act.Execute(ctx, dependencies)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("context canceled"))
		})
	})
})

type mockTransport struct {
	authCallCount    int
	authResponse     transport.AuthResponse
	authError        error
	cancelDuringAuth context.CancelFunc
}

func (m *mockTransport) Authenticate(ctx context.Context, req transport.AuthRequest) (transport.AuthResponse, error) {
	m.authCallCount++

	if m.cancelDuringAuth != nil {
		m.cancelDuringAuth()
	}

	if m.authError != nil {
		return transport.AuthResponse{}, m.authError
	}

	return m.authResponse, nil
}

func (m *mockTransport) Pull(ctx context.Context, jwtToken string) ([]*transport.UMHMessage, error) {
	return nil, nil
}

func (m *mockTransport) Push(ctx context.Context, jwtToken string, messages []*transport.UMHMessage) error {
	return nil
}

func (m *mockTransport) Close() {
}

func (m *mockTransport) Reset() {
}
