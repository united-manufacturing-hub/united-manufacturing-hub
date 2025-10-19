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

package agent_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/agent"
)

// These tests enforce the architectural boundary between Supervisor and States.
//
// From README.md (lines 122-141): "Collector health is a supervisor concern, not
// a state concern. States never see stale data - they assume observations are
// always fresh and focus purely on business logic."
//
// The supervisor checks data freshness BEFORE calling state.Next(). If data is
// stale, the supervisor pauses the FSM and handles collector recovery.
//
// These tests verify states make decisions based ONLY on health metrics, never
// timestamps. If a state starts checking CollectedAt, these tests will fail,
// preventing architectural erosion.
var _ = Describe("State Timestamp Independence", func() {
	Describe("ActiveState", func() {
		Context("when observation data is very stale", func() {
			It("should not check timestamp - focus only on health metrics", func() {
				state := &agent.ActiveState{}

				observed := &agent.AgentMonitorObservedState{
					ServiceInfo: healthyServiceInfo(),
					CollectedAt: time.Now().Add(-60 * time.Second),
				}

				snapshot := fsmv2.Snapshot{
					Desired:  &agent.AgentMonitorDesiredState{},
					Observed: observed,
				}

				nextState, _, _ := state.Next(snapshot)

				_, ok := nextState.(*agent.ActiveState)
				Expect(ok).To(BeTrue(), "should remain in ActiveState with stale timestamp")
			})
		})
	})

	Describe("DegradedState", func() {
		Context("when observation data is very stale but metrics are healthy", func() {
			It("should not check timestamp - focus only on health metrics", func() {
				state := &agent.DegradedState{}

				observed := &agent.AgentMonitorObservedState{
					ServiceInfo: healthyServiceInfo(),
					CollectedAt: time.Now().Add(-60 * time.Second),
				}

				snapshot := fsmv2.Snapshot{
					Desired:  &agent.AgentMonitorDesiredState{},
					Observed: observed,
				}

				nextState, _, _ := state.Next(snapshot)

				_, ok := nextState.(*agent.ActiveState)
				Expect(ok).To(BeTrue(), "should transition to Active on healthy metrics (ignoring timestamp)")
			})
		})
	})
})
