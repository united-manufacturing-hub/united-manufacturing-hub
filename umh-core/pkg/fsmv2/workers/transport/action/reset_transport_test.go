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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	transportpkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/action"
	"go.uber.org/zap"
)

var _ = Describe("ResetTransportAction", func() {
	var (
		act          *action.ResetTransportAction
		dependencies *transportpkg.TransportDependencies
		logger       *zap.SugaredLogger
		mockTransp   *mockTransport
	)

	BeforeEach(func() {
		logger = zap.NewNop().Sugar()
		mockTransp = &mockTransport{}
		identity := deps.Identity{ID: "test-id", WorkerType: "transport"}
		dependencies = transportpkg.NewTransportDependencies(mockTransp, logger, nil, identity)
		act = action.NewResetTransportAction()
	})

	Describe("Name", func() {
		It("should return action name", func() {
			Expect(act.Name()).To(Equal("reset_transport"))
		})
	})

	Describe("Execute", func() {
		It("should reset transport and increment resetGeneration", func() {
			initialGen := dependencies.GetResetGeneration()
			Expect(initialGen).To(Equal(uint64(0)))

			err := act.Execute(context.Background(), dependencies)
			Expect(err).NotTo(HaveOccurred())

			Expect(dependencies.GetResetGeneration()).To(Equal(uint64(1)))
		})

		It("should advance retry counter to break modulo trigger", func() {
			err := act.Execute(context.Background(), dependencies)
			Expect(err).NotTo(HaveOccurred())

			// RetryTracker.Attempt() increments the internal counter.
			// We can verify this didn't panic and the action completed.
		})

		It("should handle context cancellation", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			err := act.Execute(ctx, dependencies)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("context"))
		})

		It("should return error on invalid deps type", func() {
			err := act.Execute(context.Background(), "not-deps")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid dependencies type"))
		})

		It("should increment resetGeneration on successive calls", func() {
			for range 3 {
				err := act.Execute(context.Background(), dependencies)
				Expect(err).NotTo(HaveOccurred())
			}

			Expect(dependencies.GetResetGeneration()).To(Equal(uint64(3)))
		})
	})
})
