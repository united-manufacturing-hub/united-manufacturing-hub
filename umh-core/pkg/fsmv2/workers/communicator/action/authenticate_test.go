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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/action"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
)

var _ = Describe("AuthenticateAction", func() {
	var (
		act           *action.AuthenticateAction
		dependencies  *communicator.CommunicatorDependencies
		logger        *zap.SugaredLogger
		mockTransp    *mockTransport
	)

	BeforeEach(func() {
		logger = zap.NewNop().Sugar()
		mockTransp = &mockTransport{}
		identity := fsmv2.Identity{ID: "test-id", WorkerType: "communicator"}
		dependencies = communicator.NewCommunicatorDependencies(mockTransp, logger, nil, identity)
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
			// Create dependencies with nil transport (simulates factory creation)
			identity := fsmv2.Identity{ID: "test-nil-transport", WorkerType: "communicator"}
			depsWithNilTransport := communicator.NewCommunicatorDependencies(nil, logger, nil, identity)
			Expect(depsWithNilTransport.GetTransport()).To(BeNil(), "transport should be nil before first auth")

			// Create action with config (simulates TryingToAuthenticateState creating action)
			authAction := action.NewAuthenticateAction(
				"https://relay.example.com",
				"test-uuid",
				"test-token",
				10*time.Second,
			)

			ctx := context.Background()
			// Execute should create transport and store it in deps
			err := authAction.Execute(ctx, depsWithNilTransport)
			// Note: This will fail because we're hitting a real URL - that's expected
			// The important thing is that transport is created before the network call
			_ = err // Ignore network error

			// After Execute, transport should be non-nil
			Expect(depsWithNilTransport.GetTransport()).NotTo(BeNil(), "transport should be created after first auth execution")
		})

		It("should not replace existing transport on subsequent executions", func() {
			ctx := context.Background()

			// Get original transport
			originalTransport := dependencies.GetTransport()
			Expect(originalTransport).NotTo(BeNil())

			// Execute action
			err := act.Execute(ctx, dependencies)
			Expect(err).NotTo(HaveOccurred())

			// Transport should be the same (not replaced)
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

			// Verify JWT token is stored in dependencies
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

			// Verify JWT expiry is stored in dependencies (converted from Unix timestamp)
			storedExpiry := dependencies.GetJWTExpiry()
			Expect(storedExpiry.Unix()).To(Equal(expectedExpiry))
		})
	})
})

type mockTransport struct {
	authCallCount int
	authResponse  transport.AuthResponse
}

func (m *mockTransport) Authenticate(ctx context.Context, req transport.AuthRequest) (transport.AuthResponse, error) {
	m.authCallCount++

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
