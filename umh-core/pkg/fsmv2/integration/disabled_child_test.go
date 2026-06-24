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

package integration_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmconfig "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
)

// residentDisabledParentState is a parent state that unconditionally emits one
// disabled examplechild spec every tick. It has no actions and never transitions.
type residentDisabledParentState struct{}

func (s *residentDisabledParentState) String() string {
	return "ResidentDisabledParent"
}

func (s *residentDisabledParentState) LifecyclePhase() fsmconfig.LifecyclePhase {
	return fsmconfig.PhaseRunningHealthy
}

func (s *residentDisabledParentState) Next(_ any) fsmv2.NextResult[any, any] {
	return fsmv2.Transition(
		s,
		fsmv2.SignalNone,
		nil,
		"emitting one disabled child",
		[]fsmconfig.ChildSpec{
			{
				Name:       "child-0",
				WorkerType: "examplechild",
				UserSpec:   fsmconfig.UserSpec{},
				Enabled:    false,
			},
		},
	)
}

// residentDisabledParentWorker is a minimal parent worker used exclusively by
// the resident-disabled-child test. It always returns fresh observations and a
// default desired state so the supervisor's freshness check passes.
type residentDisabledParentWorker struct{}

func (w *residentDisabledParentWorker) CollectObservedState(_ context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	return &supervisor.TestObservedState{
		CollectedAt: time.Now(),
		ID:          "resident-parent-001",
	}, nil
}

func (w *residentDisabledParentWorker) DeriveDesiredState(_ interface{}) (fsmv2.DesiredState, error) {
	return &supervisor.TestDesiredState{}, nil
}

func (w *residentDisabledParentWorker) GetInitialState() fsmv2.State[any, any] {
	return &residentDisabledParentState{}
}

var _ = Describe("Resident-disabled child stays in Stopped, not removed", func() {
	It("child-0 (Enabled=false) remains resident in s.children after multiple ticks", func() {
		ctx := context.Background()

		store := storage.NewTriangularStore(
			memory.NewInMemoryStore(),
			deps.NewNopFSMLogger(),
		)

		parentSup := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
			WorkerType:              "resident-disabled-parent",
			Store:                   store,
			Logger:                  deps.NewNopFSMLogger(),
			GracefulShutdownTimeout: 10 * time.Second,
		})

		identity := deps.Identity{
			ID:         "resident-parent-001",
			Name:       "Resident Disabled Parent",
			WorkerType: "resident-disabled-parent",
		}

		err := parentSup.AddWorker(identity, &residentDisabledParentWorker{})
		Expect(err).ToNot(HaveOccurred())

		for range 3 {
			Expect(parentSup.TestTick(ctx)).To(Succeed())
		}

		children := parentSup.GetChildren()
		Expect(children).To(HaveKey("child-0"),
			"child-0 must remain resident (not removed) when Enabled=false — "+
				"the disable-mapping pass sets IsDisabled, not IsShutdownRequested, so "+
				"SignalNeedsRemoval is never emitted")

		childStateName := children["child-0"].GetCurrentStateName()
		Expect(childStateName).To(ContainSubstring("Stopped"),
			"child-0 must stay in Stopped state, not transition elsewhere")
	})
})
