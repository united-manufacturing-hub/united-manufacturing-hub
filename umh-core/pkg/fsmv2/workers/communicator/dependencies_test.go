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

package communicator_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
)

// Test suite is registered in worker_test.go to avoid duplicate RunSpecs

type mockTransport struct{}

func (m *mockTransport) Authenticate(_ context.Context, _ transport.AuthRequest) (transport.AuthResponse, error) {
	return transport.AuthResponse{}, nil
}
func (m *mockTransport) Pull(_ context.Context, _ string) ([]*transport.UMHMessage, error) {
	return nil, nil
}
func (m *mockTransport) Push(_ context.Context, _ string, _ []*transport.UMHMessage) error {
	return nil
}
func (m *mockTransport) Close() {}

var _ = Describe("CommunicatorDependencies", func() {
	var (
		mt     transport.Transport
		logger *zap.SugaredLogger
	)

	BeforeEach(func() {
		mt = &mockTransport{}
		logger = zap.NewNop().Sugar()
	})

	Describe("NewCommunicatorDependencies", func() {
		Context("when creating a new dependencies", func() {
			It("should return a non-nil dependencies", func() {
				deps := communicator.NewCommunicatorDependencies(mt, logger, "communicator", "test-id")
				Expect(deps).NotTo(BeNil())
			})

			It("should store the transport", func() {
				deps := communicator.NewCommunicatorDependencies(mt, logger, "communicator", "test-id")
				Expect(deps.GetTransport()).To(Equal(mt))
			})

			It("should store the logger", func() {
				deps := communicator.NewCommunicatorDependencies(mt, logger, "communicator", "test-id")
				// Logger is enriched with worker context, so it won't equal original
				Expect(deps.GetLogger()).NotTo(BeNil())
			})
		})
	})

	Describe("GetTransport", func() {
		It("should return the transport passed to the constructor", func() {
			deps := communicator.NewCommunicatorDependencies(mt, logger, "communicator", "test-id")
			Expect(deps.GetTransport()).To(Equal(mt))
		})
	})

	Describe("GetLogger", func() {
		It("should return the logger inherited from BaseDependencies", func() {
			deps := communicator.NewCommunicatorDependencies(mt, logger, "communicator", "test-id")
			// Logger is enriched with worker context
			Expect(deps.GetLogger()).NotTo(BeNil())
		})
	})

	Describe("Dependencies interface implementation", func() {
		It("should implement fsmv2.Dependencies interface", func() {
			deps := communicator.NewCommunicatorDependencies(mt, logger, "communicator", "test-id")
			var _ fsmv2.Dependencies = deps
			Expect(deps).To(Satisfy(func(d interface{}) bool {
				_, ok := d.(fsmv2.Dependencies)

				return ok
			}))
		})
	})
})
