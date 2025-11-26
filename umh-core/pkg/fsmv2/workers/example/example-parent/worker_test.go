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

package example_parent_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	fsmv2types "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	example_parent "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-parent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-parent/state"
)

func TestExampleParent(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Example Parent Suite")
}

var _ = Describe("ParentWorker", func() {
	var (
		worker *example_parent.ParentWorker
		logger *zap.SugaredLogger
	)

	BeforeEach(func() {
		logger = zap.NewNop().Sugar()
		var err error
		worker, err = example_parent.NewParentWorker("test-parent", "Test Parent", logger)
		Expect(err).ToNot(HaveOccurred())
	})

	Describe("NewParentWorker", func() {
		It("should create a worker", func() {
			Expect(worker).NotTo(BeNil())
		})
	})

	Describe("CollectObservedState", func() {
		It("should return observed state with timestamp", func() {
			observed, err := worker.CollectObservedState(context.Background())

			Expect(err).ToNot(HaveOccurred())
			Expect(observed).NotTo(BeNil())
			Expect(observed.GetTimestamp()).NotTo(BeZero())
		})
	})

	Describe("DeriveDesiredState", func() {
		It("should return running state with empty config", func() {
			spec := fsmv2types.UserSpec{
				Config:    "children_count: 0",
				Variables: fsmv2types.VariableBundle{},
			}

			desired, err := worker.DeriveDesiredState(spec)

			Expect(err).ToNot(HaveOccurred())
			Expect(desired.State).To(Equal("running"))
			Expect(desired.ChildrenSpecs).To(BeNil())
		})

		It("should create child specs when children_count is specified", func() {
			spec := fsmv2types.UserSpec{
				Config:    "children_count: 3",
				Variables: fsmv2types.VariableBundle{},
			}

			desired, err := worker.DeriveDesiredState(spec)

			Expect(err).ToNot(HaveOccurred())
			Expect(desired.State).To(Equal("running"))
			Expect(desired.ChildrenSpecs).To(HaveLen(3))
			Expect(desired.ChildrenSpecs[0].Name).To(Equal("child-0"))
			Expect(desired.ChildrenSpecs[1].Name).To(Equal("child-1"))
			Expect(desired.ChildrenSpecs[2].Name).To(Equal("child-2"))
		})
	})

	Describe("GetInitialState", func() {
		It("should return StoppedState", func() {
			initialState := worker.GetInitialState()

			Expect(initialState).To(BeAssignableToTypeOf(&state.StoppedState{}))
		})
	})
})
