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

package root_supervisor_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2types "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	rootsupervisor "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/root_supervisor"
)

// Compile-time interface verification for workers.
// These ensure that our worker types satisfy the fsmv2.Worker interface.
var _ fsmv2.Worker = (*rootsupervisor.ChildWorker)(nil)

var _ = Describe("ChildWorker", func() {
	var (
		worker *rootsupervisor.ChildWorker
		logger *zap.SugaredLogger
		ctx    context.Context
	)

	BeforeEach(func() {
		zapLogger, _ := zap.NewDevelopment()
		logger = zapLogger.Sugar()
		worker = rootsupervisor.NewChildWorker("child-1", "test-child", logger)
		ctx = context.Background()
	})

	Describe("NewChildWorker", func() {
		It("should create a new ChildWorker with correct identity", func() {
			Expect(worker).NotTo(BeNil())
		})
	})

	Describe("CollectObservedState", func() {
		It("should return a ChildObservedState", func() {
			observed, err := worker.CollectObservedState(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(observed).NotTo(BeNil())

			// Type assert to ChildObservedState
			childObserved, ok := observed.(rootsupervisor.ChildObservedState)
			Expect(ok).To(BeTrue(), "Expected ChildObservedState type")
			Expect(childObserved.ID).To(Equal("child-1"))
			Expect(childObserved.CollectedAt).NotTo(BeZero())
		})
	})

	Describe("DeriveDesiredState", func() {
		It("should return a DesiredState with nil ChildrenSpecs (leaf node)", func() {
			desired, err := worker.DeriveDesiredState(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(desired.State).NotTo(BeEmpty())
			Expect(desired.ChildrenSpecs).To(BeNil(), "Child workers should return nil ChildrenSpecs (leaf node)")
		})

		It("should handle UserSpec input", func() {
			userSpec := fsmv2types.UserSpec{
				Config: "some_config: value",
			}
			desired, err := worker.DeriveDesiredState(userSpec)
			Expect(err).NotTo(HaveOccurred())
			Expect(desired.ChildrenSpecs).To(BeNil(), "Child workers should always return nil ChildrenSpecs")
		})

		It("should return error for invalid spec type", func() {
			_, err := worker.DeriveDesiredState("invalid")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("GetInitialState", func() {
		It("should return an initial state", func() {
			state := worker.GetInitialState()
			Expect(state).NotTo(BeNil())
			Expect(state.String()).To(Equal("ChildStopped"))
		})
	})
})
