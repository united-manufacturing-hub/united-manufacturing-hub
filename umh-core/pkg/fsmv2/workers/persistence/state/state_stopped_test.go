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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2"
	fsmv2config "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence/state"
)

// stoppedSnapshot builds a snapshot that differs between the test cases only
// in whether shutdown is requested and whether the parent disabled the worker.
func stoppedSnapshot(shutdownRequested, disabled bool) fsmv2.Snapshot {
	return fsmv2.Snapshot{
		Identity: deps.Identity{ID: "test", WorkerType: "persistence"},
		Observed: fsmv2.Observation[snapshot.PersistenceStatus]{
			CollectedAt: time.Now(),
		},
		Desired: &fsmv2.WrappedDesiredState[snapshot.PersistenceConfig]{
			BaseDesiredState: fsmv2config.BaseDesiredState{
				ShutdownRequested: shutdownRequested,
				Disabled:          disabled,
			},
			Config: snapshot.PersistenceConfig{
				CompactionInterval:  5 * time.Minute,
				RetentionWindow:     24 * time.Hour,
				MaintenanceInterval: 7 * 24 * time.Hour,
			},
		},
	}
}

var _ = Describe("StoppedState", func() {
	var stateObj *state.StoppedState

	BeforeEach(func() {
		stateObj = &state.StoppedState{}
	})

	Describe("Next", func() {
		Context("when shutdown is requested", func() {
			It("should stay in stopped state with SignalNeedsRemoval", func() {
				snap := stoppedSnapshot(true, false)

				result := stateObj.Next(snap)
				Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
				Expect(result.Signal).To(Equal(fsmv2.SignalNeedsRemoval))
				Expect(result.Action).To(BeNil())
			})
		})

		Context("when the parent disabled the worker", func() {
			It("should stay in stopped state without signalling removal", func() {
				snap := stoppedSnapshot(false, true)

				result := stateObj.Next(snap)
				Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
				Expect(result.Signal).To(Equal(fsmv2.SignalNone))
				Expect(result.Action).To(BeNil())
			})
		})

		Context("when shutdown is requested for a disabled worker", func() {
			It("should signal needs removal", func() {
				snap := stoppedSnapshot(true, true)

				result := stateObj.Next(snap)
				Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
				Expect(result.Signal).To(Equal(fsmv2.SignalNeedsRemoval))
			})
		})

		Context("when shutdown is not requested", func() {
			It("should transition to TryingToStartState", func() {
				snap := stoppedSnapshot(false, false)

				result := stateObj.Next(snap)
				Expect(result.State).To(BeAssignableToTypeOf(&state.TryingToStartState{}))
				Expect(result.Signal).To(Equal(fsmv2.SignalNone))
				Expect(result.Action).To(BeNil())
			})
		})
	})

	Describe("String", func() {
		It("should return Stopped", func() {
			Expect(stateObj.String()).To(Equal("Stopped"))
		})
	})
})
