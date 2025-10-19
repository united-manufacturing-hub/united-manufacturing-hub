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

package agent_monitor_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/agent_monitor"
)

var _ = Describe("State Timestamp Independence", func() {
	Describe("ActiveState", func() {
		Context("when observation data is very stale", func() {
			It("should not check timestamp - focus only on health metrics", func() {
				state := &agent_monitor.ActiveState{}

				observed := &agent_monitor.AgentMonitorObservedState{
					ServiceInfo: healthyServiceInfo(),
					CollectedAt: time.Now().Add(-60 * time.Second),
				}

				snapshot := fsmv2.Snapshot{
					Desired:  &agent_monitor.AgentMonitorDesiredState{},
					Observed: observed,
				}

				nextState, _, _ := state.Next(snapshot)

				_, ok := nextState.(*agent_monitor.ActiveState)
				Expect(ok).To(BeTrue(), "should remain in ActiveState with stale timestamp")
			})
		})
	})

	Describe("DegradedState", func() {
		Context("when observation data is very stale but metrics are healthy", func() {
			It("should not check timestamp - focus only on health metrics", func() {
				state := &agent_monitor.DegradedState{}

				observed := &agent_monitor.AgentMonitorObservedState{
					ServiceInfo: healthyServiceInfo(),
					CollectedAt: time.Now().Add(-60 * time.Second),
				}

				snapshot := fsmv2.Snapshot{
					Desired:  &agent_monitor.AgentMonitorDesiredState{},
					Observed: observed,
				}

				nextState, _, _ := state.Next(snapshot)

				_, ok := nextState.(*agent_monitor.ActiveState)
				Expect(ok).To(BeTrue(), "should transition to Active on healthy metrics (ignoring timestamp)")
			})
		})
	})
})
