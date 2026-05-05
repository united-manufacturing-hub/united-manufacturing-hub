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

package supervisor_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// CHANGE-19 reducer behaviour for the cold-boot path.
//
// PR3-I introduced a skip-on-create guard in reconcileChildren: when a
// ChildSpec arrives Enabled=false and the child is not yet resident in
// s.children, the supervisor must not allocate dependencies / construct
// the worker. The full pause/resume + reducer-on-resident-child semantics
// are covered in reconciliation_pause_resume_test.go; the tests here
// pin the cold-boot guard specifically.
var _ = Describe("CHANGE-19 Reducer (skip-on-create)", func() {
	var (
		ctx       context.Context
		mockStore *mockTriangularStore
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockStore = newMockTriangularStore()
	})

	It("TestReconcileChildren_EnabledFalse_ColdBoot_SkipsCreation", func() {
		// Spec arrives Enabled=false on the very first reconcile tick.
		// The supervisor must NOT create the child supervisor — that would
		// allocate dependencies and construct the worker only for the
		// reducer + child's StoppedState to immediately tear it down.
		initialSpecs := []config.ChildSpec{
			{
				Name:       "child-a",
				WorkerType: "child",
				UserSpec:   config.UserSpec{Config: "child-config"},
				Enabled:    false,
			},
		}
		parentSuper, _ := newPauseResumeFixture(ctx, mockStore, initialSpecs)

		Expect(parentSuper.TestTick(ctx)).To(Succeed())

		// Assert 1: child was NOT created.
		Expect(parentSuper.GetChildren()).NotTo(HaveKey("child-a"),
			"PR3-I skip-on-create: Enabled=false on cold boot must not create the child supervisor")

		// Assert 2: child is NOT in pendingRemoval — Enabled=false ≠ omission;
		// only absence from the spec list drives Phase-1 teardown.
		Expect(parentSuper.TestIsPendingRemoval("child-a")).To(BeFalse(),
			"skip-on-create must not write pendingRemoval; only omission from the spec list does")
	})

	It("TestReconcileChildren_EnabledFalseTrue_FlipFromColdBoot_CreatesChild", func() {
		// Tick 1: spec arrives Enabled=false → no child created (PR3-I skip).
		// Tick 2: parent flips Enabled=true on the still-non-resident spec.
		//         The next reconcileChildren must create the child normally.
		initialSpecs := []config.ChildSpec{
			{
				Name:       "child-a",
				WorkerType: "child",
				UserSpec:   config.UserSpec{Config: "child-config"},
				Enabled:    false,
			},
		}
		parentSuper, worker := newPauseResumeFixture(ctx, mockStore, initialSpecs)

		// Tick 1: skip-on-create.
		Expect(parentSuper.TestTick(ctx)).To(Succeed())
		Expect(parentSuper.GetChildren()).NotTo(HaveKey("child-a"),
			"precondition: tick 1 must not create the disabled child")

		// Flip Enabled=true and bust the DDS cache so reconcileChildren
		// picks up the new spec value (see hierarchicalWorker doc comment).
		worker.childrenSpecs = []config.ChildSpec{
			{
				Name:       "child-a",
				WorkerType: "child",
				UserSpec:   config.UserSpec{Config: "child-config"},
				Enabled:    true,
			},
		}
		parentSuper.TestUpdateUserSpec(config.UserSpec{Config: "parent-config-enable"})

		// Tick 2: child must now be created.
		Expect(parentSuper.TestTick(ctx)).To(Succeed())

		Expect(parentSuper.GetChildren()).To(HaveKey("child-a"),
			"flipping Enabled false→true on a non-resident spec must trigger normal creation")
		Expect(parentSuper.TestIsPendingRemoval("child-a")).To(BeFalse(),
			"newly-created child must not be in pendingRemoval")
	})
})
