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
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/communicator"
)

func TestCommunicatorRegistry(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CommunicatorRegistry Suite")
}

var _ = Describe("CommunicatorRegistry", func() {
	var (
		mockTransport *MockTransport
		logger        *zap.SugaredLogger
	)

	BeforeEach(func() {
		mockTransport = NewMockTransport()
		logger = zap.NewNop().Sugar()
	})

	Describe("NewCommunicatorRegistry", func() {
		Context("when creating a new registry", func() {
			It("should return a non-nil registry", func() {
				registry := communicator.NewCommunicatorRegistry(mockTransport, logger)
				Expect(registry).NotTo(BeNil())
			})

			It("should store the transport", func() {
				registry := communicator.NewCommunicatorRegistry(mockTransport, logger)
				Expect(registry.GetTransport()).To(Equal(mockTransport))
			})

			It("should store the logger", func() {
				registry := communicator.NewCommunicatorRegistry(mockTransport, logger)
				Expect(registry.GetLogger()).To(Equal(logger))
			})
		})
	})

	Describe("GetTransport", func() {
		It("should return the transport passed to the constructor", func() {
			registry := communicator.NewCommunicatorRegistry(mockTransport, logger)
			Expect(registry.GetTransport()).To(Equal(mockTransport))
		})
	})

	Describe("GetLogger", func() {
		It("should return the logger inherited from BaseRegistry", func() {
			registry := communicator.NewCommunicatorRegistry(mockTransport, logger)
			Expect(registry.GetLogger()).To(Equal(logger))
		})
	})

	Describe("Registry interface implementation", func() {
		It("should implement fsmv2.Registry interface", func() {
			registry := communicator.NewCommunicatorRegistry(mockTransport, logger)
			var _ fsmv2.Registry = registry
			Expect(registry).To(Satisfy(func(r interface{}) bool {
				_, ok := r.(fsmv2.Registry)
				return ok
			}))
		})
	})
})
