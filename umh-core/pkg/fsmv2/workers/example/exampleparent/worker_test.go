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

package exampleparent_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2types "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent/state"
)

func TestExampleParent(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Example Parent Suite")
}

var _ = Describe("ParentWorker", func() {
	var (
		worker *exampleparent.ParentWorker
		logger deps.FSMLogger
	)

	BeforeEach(func() {
		logger = deps.NewNopFSMLogger()
		identity := deps.Identity{ID: "test-parent", Name: "Test Parent"}
		var err error
		worker, err = exampleparent.NewParentWorker(identity, logger, nil)
		Expect(err).ToNot(HaveOccurred())
	})

	Describe("NewParentWorker", func() {
		It("should create a worker", func() {
			Expect(worker).NotTo(BeNil())
		})
	})

	Describe("CollectObservedState", func() {
		It("should return observed state", func() {
			observed, err := worker.CollectObservedState(context.Background(), nil)

			Expect(err).ToNot(HaveOccurred())
			Expect(observed).NotTo(BeNil())
			// CollectedAt is set by the collector post-COS (zero on direct return).
			Expect(observed.GetTimestamp()).To(BeZero())
		})
	})

	Describe("DeriveDesiredState", func() {
		It("should return running state with empty config", func() {
			spec := fsmv2types.UserSpec{
				Config:    "children_count: 0",
				Variables: fsmv2types.VariableBundle{},
			}

			desiredIface, err := worker.DeriveDesiredState(spec)

			Expect(err).ToNot(HaveOccurred())
			desired := desiredIface.(*fsmv2.WrappedDesiredState[exampleparent.ExampleparentConfig])
			Expect(desired.IsShutdownRequested()).To(BeFalse())
			// ChildrenCount == 0 -> non-nil empty slice (worker.go: spec parse with ChildrenCount=0).
			Expect(desired.ChildrenSpecs).To(BeEmpty())
			Expect(desired.ChildrenSpecs).NotTo(BeNil())

			// Canonical RenderChildren is called with *ExampleparentConfig and
			// also returns the same shape (preserved unchanged in PR4-A.5;
			// PR4-C will extract this to a children/ package).
			children := exampleparent.RenderChildren(&exampleparent.ExampleparentConfig{ChildrenCount: 0})
			Expect(children).To(BeEmpty())
			Expect(children).NotTo(BeNil())
		})

		It("should create child specs when children_count is specified", func() {
			spec := fsmv2types.UserSpec{
				Config:    "children_count: 3",
				Variables: fsmv2types.VariableBundle{},
			}

			desiredIface, err := worker.DeriveDesiredState(spec)

			Expect(err).ToNot(HaveOccurred())
			desired := desiredIface.(*fsmv2.WrappedDesiredState[exampleparent.ExampleparentConfig])
			Expect(desired.IsShutdownRequested()).To(BeFalse())
			Expect(desired.ChildrenSpecs).To(HaveLen(3))
			Expect(desired.ChildrenSpecs[0].Name).To(Equal("child-0"))
			Expect(desired.ChildrenSpecs[0].Enabled).To(BeTrue(), "ChildSpec.Enabled must be true (§4-C/F4⊕G1 invariant)")

			children := exampleparent.RenderChildren(&exampleparent.ExampleparentConfig{ChildrenCount: 3})
			Expect(children).To(HaveLen(3))
			Expect(children[0].Name).To(Equal("child-0"))
			Expect(children[1].Name).To(Equal("child-1"))
			Expect(children[2].Name).To(Equal("child-2"))
		})
	})

	Describe("GetInitialState", func() {
		It("should return StoppedState", func() {
			initialState := worker.GetInitialState()

			Expect(initialState).To(BeAssignableToTypeOf(&state.StoppedState{}))
		})
	})

	Describe("GetDependenciesAny", func() {
		It("returns *ParentDependencies", func() {
			var w fsmv2.Worker = worker
			dp, ok := w.(fsmv2.DependencyProvider)
			Expect(ok).To(BeTrue(), "worker must implement DependencyProvider")
			got := dp.GetDependenciesAny()
			_, ok = got.(*exampleparent.ParentDependencies)
			Expect(ok).To(BeTrue(), "expected *ParentDependencies, got %T", got)
		})
	})
})
