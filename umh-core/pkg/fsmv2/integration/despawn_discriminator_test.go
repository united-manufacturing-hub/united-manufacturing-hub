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
	exampleparent "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
)

// despawnPhaseState emits [child-0] on tick 1, then non-nil empty []ChildSpec{} forever.
// Pins the rendered != nil discriminator: non-nil empty must despawn child-0,
// not fall back to GetChildrenSpecs() (which would re-create it from DeriveDesiredState).
type despawnPhaseState struct {
	spawned bool
}

func (s *despawnPhaseState) String() string { return "DespawnPhaseState" }

func (s *despawnPhaseState) LifecyclePhase() fsmconfig.LifecyclePhase {
	return fsmconfig.PhaseRunningHealthy
}

func (s *despawnPhaseState) Next(_ any) fsmv2.NextResult[any, any] {
	if !s.spawned {
		s.spawned = true
		return fsmv2.Transition(s, fsmv2.SignalNone, nil, "spawning child-0",
			[]fsmconfig.ChildSpec{
				{
					Name:             "child-0",
					WorkerType:       "examplechild",
					UserSpec:         fsmconfig.UserSpec{},
					Enabled:          true,
					ChildStartStates: []string{"TryingToStart", "Running"},
				},
			})
	}
	// Non-nil empty = despawn signal. The supervisor must use this set directly,
	// not fall back to the legacy GetChildrenSpecs() path.
	return fsmv2.Transition(s, fsmv2.SignalNone, nil, "despawning all children",
		[]fsmconfig.ChildSpec{})
}

// DeriveDesiredState returns [child-0] unconditionally. If the supervisor
// falls back to GetChildrenSpecs() it will keep re-creating child-0 forever,
// even when the state machine emits non-nil empty.
type despawnParentWorker struct{}

func (w *despawnParentWorker) CollectObservedState(_ context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	return &supervisor.TestObservedState{
		CollectedAt: time.Now(),
		ID:          "despawn-parent-001",
	}, nil
}

func (w *despawnParentWorker) DeriveDesiredState(_ interface{}) (fsmv2.DesiredState, error) {
	return &fsmv2.WrappedDesiredState[exampleparent.ExampleparentConfig]{
		State: fsmconfig.DesiredStateRunning,
		ChildrenSpecs: []fsmconfig.ChildSpec{
			{
				Name:             "child-0",
				WorkerType:       "examplechild",
				UserSpec:         fsmconfig.UserSpec{},
				Enabled:          true,
				ChildStartStates: []string{"TryingToStart", "Running"},
			},
		},
	}, nil
}

func (w *despawnParentWorker) GetInitialState() fsmv2.State[any, any] {
	return &despawnPhaseState{}
}

var _ = Describe("Despawn discriminator: non-nil empty ChildSpec slice despawns children", func() {
	It("child-0 is removed after state machine emits []ChildSpec{} (non-nil empty)", func() {
		ctx := context.Background()

		store := storage.NewTriangularStore(
			memory.NewInMemoryStore(),
			deps.NewNopFSMLogger(),
		)

		parentSup := supervisor.NewSupervisor[*supervisor.TestObservedState, *fsmv2.WrappedDesiredState[exampleparent.ExampleparentConfig]](supervisor.Config{
			WorkerType:              "despawn-phase-parent",
			Store:                   store,
			Logger:                  deps.NewNopFSMLogger(),
			GracefulShutdownTimeout: 10 * time.Second,
		})

		identity := deps.Identity{
			ID:         "despawn-parent-001",
			Name:       "Despawn Phase Parent",
			WorkerType: "despawn-phase-parent",
		}

		err := parentSup.AddWorker(identity, &despawnParentWorker{})
		Expect(err).ToNot(HaveOccurred())

		// Tick 1: state machine emits [child-0] → child-0 is created in s.children.
		Expect(parentSup.TestTick(ctx)).To(Succeed())
		Expect(parentSup.GetChildren()).To(HaveKey("child-0"),
			"child-0 must be created on the first tick")

		// Subsequent ticks: state machine emits []ChildSpec{} (non-nil empty → despawn path).
		// DeriveDesiredState still returns [child-0] unconditionally, so a broken len()>0
		// discriminator falls through to legacy and child-0 is never removed.
		Eventually(func() bool {
			_ = parentSup.TestTick(ctx)
			return len(parentSup.GetChildren()) == 0
		}, "5s", "100ms").Should(BeTrue(),
			"child-0 must be removed once state machine emits non-nil empty ChildSpec slice — "+
				"guards the !=nil discriminator: if changed to len()>0, the empty slice "+
				"falls through to legacy GetChildrenSpecs() which always returns [child-0]")
	})
})
