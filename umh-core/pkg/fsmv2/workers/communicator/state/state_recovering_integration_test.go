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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/state"
)

// This test verifies RecoveringState correctly transitions through the
// child-health recovery cycle: unhealthy children → stay recovering → healthy children → syncing.
var _ = Describe("RecoveringState Integration - Recovery Cycle", func() {
	var (
		stateObj *state.RecoveringState
	)

	BeforeEach(func() {
		stateObj = &state.RecoveringState{}
	})

	Describe("Recovery Cycle", func() {
		It("should complete full cycle: healthy -> degraded -> recovery", func() {
			// Phase 1: Children go unhealthy — stay in Recovering
			snap1 := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					ChildrenHealthy:   0,
					ChildrenUnhealthy: 1,
				},
				Desired: &snapshot.CommunicatorDesiredState{},
			}

			result1 := stateObj.Next(snap1)
			Expect(result1.State).To(BeAssignableToTypeOf(&state.RecoveringState{}))
			Expect(result1.Action).To(BeNil())

			// Phase 2: Children still unhealthy — stay in Recovering
			snap2 := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					ChildrenHealthy:   0,
					ChildrenUnhealthy: 1,
				},
				Desired: &snapshot.CommunicatorDesiredState{},
			}

			result2 := stateObj.Next(snap2)
			Expect(result2.State).To(BeAssignableToTypeOf(&state.RecoveringState{}))
			Expect(result2.Action).To(BeNil())

			// Phase 3: Children recover — transition to Syncing
			snap3 := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					ChildrenHealthy:   1,
					ChildrenUnhealthy: 0,
				},
				Desired: &snapshot.CommunicatorDesiredState{},
			}

			result3 := stateObj.Next(snap3)
			Expect(result3.State).To(BeAssignableToTypeOf(&state.SyncingState{}))
			Expect(result3.Action).To(BeNil())
		})
	})
})
