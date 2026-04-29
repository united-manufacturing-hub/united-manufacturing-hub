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
)

func TestExampleParent(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Example Parent Suite")
}

var _ = Describe("ParentWorker", func() {
	var (
		logger       deps.FSMLogger
		identity     deps.Identity
		dependencies *exampleparent.ParentDependencies
	)

	BeforeEach(func() {
		logger = deps.NewNopFSMLogger()
		identity = deps.Identity{ID: "test-parent", Name: "Test Parent", WorkerType: exampleparent.WorkerTypeName}
		dependencies = exampleparent.NewParentDependencies(logger, nil, identity)
	})

	Describe("NewParentWorker", func() {
		It("should create a worker successfully", func() {
			worker, err := exampleparent.NewParentWorker(identity, logger, nil, dependencies)

			Expect(err).NotTo(HaveOccurred())
			Expect(worker).NotTo(BeNil())
		})

		It("should have a non-nil initial state", func() {
			worker, err := exampleparent.NewParentWorker(identity, logger, nil, dependencies)

			Expect(err).NotTo(HaveOccurred())
			Expect(worker.GetInitialState()).NotTo(BeNil())
		})

		It("should provision default dependencies when nil is passed", func() {
			worker, err := exampleparent.NewParentWorker(identity, logger, nil, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(worker).NotTo(BeNil())
		})

		It("should reject nil logger", func() {
			_, err := exampleparent.NewParentWorker(identity, nil, nil, dependencies)

			Expect(err).To(HaveOccurred())
		})
	})

	Describe("CollectObservedState", func() {
		It("should return a valid observed state", func() {
			worker, err := exampleparent.NewParentWorker(identity, logger, nil, dependencies)
			Expect(err).NotTo(HaveOccurred())

			observed, err := worker.CollectObservedState(context.Background(), nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(observed).NotTo(BeNil())
		})
	})

	Describe("DeriveDesiredState", func() {
		It("should produce an empty ChildrenSpecs slice when children_count is 0", func() {
			worker, err := exampleparent.NewParentWorker(identity, logger, nil, dependencies)
			Expect(err).NotTo(HaveOccurred())

			spec := fsmv2types.UserSpec{
				Config:    "children_count: 0",
				Variables: fsmv2types.VariableBundle{},
			}

			desiredIface, err := worker.DeriveDesiredState(spec)
			Expect(err).NotTo(HaveOccurred())

			// WorkerBase.DeriveDesiredState returns the typed wrapper;
			// children are emitted via RenderChildren (state.Next path),
			// not from DDS, so wrapped.ChildrenSpecs is nil.
			wrapped, ok := desiredIface.(*fsmv2.WrappedDesiredState[exampleparent.ExampleparentConfig])
			Expect(ok).To(BeTrue())
			Expect(wrapped.ChildrenSpecs).To(BeNil())

			// RenderChildren with ChildrenCount == 0 returns non-nil empty
			// (the authoritative "zero children" sentinel per P2.4
			// discriminator).
			snap := fsmv2.WorkerSnapshot[exampleparent.ExampleparentConfig, exampleparent.ExampleparentStatus]{
				Desired: fsmv2.WrappedDesiredState[exampleparent.ExampleparentConfig]{
					Config: exampleparent.ExampleparentConfig{ChildrenCount: 0},
				},
			}
			children := exampleparent.RenderChildren(snap)
			Expect(children).To(BeEmpty())
			Expect(children).NotTo(BeNil())
		})

		It("should produce ChildrenSpecs when children_count is specified", func() {
			worker, err := exampleparent.NewParentWorker(identity, logger, nil, dependencies)
			Expect(err).NotTo(HaveOccurred())

			spec := fsmv2types.UserSpec{
				Config:    "children_count: 3",
				Variables: fsmv2types.VariableBundle{},
			}

			desiredIface, err := worker.DeriveDesiredState(spec)
			Expect(err).NotTo(HaveOccurred())

			// WorkerBase produces the typed wrapper; the children-set is
			// produced by RenderChildren (state.Next path), so verify
			// independently against the canonical emitter.
			_, ok := desiredIface.(*fsmv2.WrappedDesiredState[exampleparent.ExampleparentConfig])
			Expect(ok).To(BeTrue())

			snap := fsmv2.WorkerSnapshot[exampleparent.ExampleparentConfig, exampleparent.ExampleparentStatus]{
				Desired: fsmv2.WrappedDesiredState[exampleparent.ExampleparentConfig]{
					Config: exampleparent.ExampleparentConfig{ChildrenCount: 3},
				},
			}
			children := exampleparent.RenderChildren(snap)
			Expect(children).To(HaveLen(3))
			Expect(children[0].Name).To(Equal("child-0"))
			Expect(children[1].Name).To(Equal("child-1"))
			Expect(children[2].Name).To(Equal("child-2"))
		})
	})
})
