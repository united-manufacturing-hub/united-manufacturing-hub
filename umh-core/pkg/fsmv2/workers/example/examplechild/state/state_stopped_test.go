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

package state_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild/state"
)

// makeChildSnapshot constructs an examplechild snapshot for state-machine
// behavioral tests. parentMappedState mirrors what the supervisor's
// MappedParentStateProvider injects each tick. Post-PR3-C2 the worker uses
// fsmv2.Observation[ExamplechildStatus] + *fsmv2.WrappedDesiredState[ExamplechildConfig];
// the snapshot envelope carries those typed values so ConvertWorkerSnapshot
// produces a usable WorkerSnapshot for state.Next.
//
// Population invariant: ConvertWorkerSnapshot reads IsShutdownRequested from
// the Desired side (BaseDesiredState.ShutdownRequested via
// WrappedDesiredState.IsShutdownRequested()), and ParentMappedState from the
// Observed side. So shutdown tests must set ShutdownRequested on Desired only;
// setting it on Observation would be dead data the conversion ignores.
func makeChildSnapshot(parentMappedState string, shutdownRequested bool) fsmv2.Snapshot {
	desired := &fsmv2.WrappedDesiredState[snapshot.ExamplechildConfig]{
		BaseDesiredState: config.BaseDesiredState{
			ShutdownRequested: shutdownRequested,
		},
	}
	observed := fsmv2.Observation[snapshot.ExamplechildStatus]{
		ParentMappedState: parentMappedState,
	}

	return fsmv2.Snapshot{
		Observed: observed,
		Desired:  desired,
		Identity: deps.Identity{ID: "test-child", Name: "child", WorkerType: "examplechild"},
	}
}

var _ = Describe("StoppedState (examplechild)", func() {
	var s *state.StoppedState

	BeforeEach(func() {
		s = &state.StoppedState{}
	})

	It("should return PhaseStopped LifecyclePhase", func() {
		Expect(s.LifecyclePhase()).To(Equal(config.PhaseStopped))
	})

	It("signals NeedsRemoval when shutdown is requested", func() {
		snap := makeChildSnapshot(config.DesiredStateRunning, true)
		result := s.Next(snap)
		Expect(result.Signal).To(Equal(fsmv2.SignalNeedsRemoval))
		Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
	})

	It("transitions to TryingToConnect when parent wants children running", func() {
		snap := makeChildSnapshot(config.DesiredStateRunning, false)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.TryingToConnectState{}))
		Expect(result.Signal).To(Equal(fsmv2.SignalNone))
	})

	It("stays Stopped when parent is not running", func() {
		snap := makeChildSnapshot(config.DesiredStateStopped, false)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
		Expect(result.Signal).To(Equal(fsmv2.SignalNone))
	})

	It("returns a stable String() name", func() {
		Expect(s.String()).To(Equal("Stopped"))
	})
})
