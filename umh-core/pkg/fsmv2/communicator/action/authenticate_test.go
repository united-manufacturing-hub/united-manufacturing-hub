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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/communicator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/communicator/action"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/communicator/transport"
)

var _ = Describe("AuthenticateAction", func() {
	var (
		act      *action.AuthenticateAction
		registry *communicator.CommunicatorRegistry
		logger   *zap.SugaredLogger
	)

	BeforeEach(func() {
		logger = zap.NewNop().Sugar()
		mockTransport := &mockTransport{}
		registry = communicator.NewCommunicatorRegistry(mockTransport, logger)
		act = action.NewAuthenticateAction(
			registry,
			"https://relay.example.com",
			"test-uuid",
			"test-token",
		)
	})

	Describe("Name", func() {
		It("should return action name", func() {
			Expect(act.Name()).To(Equal("authenticate"))
		})
	})
})

type mockTransport struct{}

func (m *mockTransport) Authenticate(ctx context.Context, req transport.AuthRequest) (transport.AuthResponse, error) {
	return transport.AuthResponse{}, nil
}

func (m *mockTransport) Pull(ctx context.Context, jwtToken string) ([]*transport.UMHMessage, error) {
	return nil, nil
}

func (m *mockTransport) Push(ctx context.Context, jwtToken string, messages []*transport.UMHMessage) error {
	return nil
}

func (m *mockTransport) Close() {
}
