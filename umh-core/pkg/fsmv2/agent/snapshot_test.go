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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/agent_monitor"
)

var _ = Describe("AgentMonitorObservedState", func() {
	Describe("GetTimestamp", func() {
		It("should return the CollectedAt timestamp", func() {
			expectedTime := time.Date(2025, 10, 18, 12, 0, 0, 0, time.UTC)
			observed := &agent_monitor.AgentMonitorObservedState{
				CollectedAt: expectedTime,
			}
			Expect(observed.GetTimestamp()).To(Equal(expectedTime))
		})

		It("should return zero time when not set", func() {
			observed := &agent_monitor.AgentMonitorObservedState{}
			Expect(observed.GetTimestamp()).To(Equal(time.Time{}))
		})
	})

	Describe("GetObservedDesiredState", func() {
		It("should return an empty AgentMonitorDesiredState", func() {
			observed := &agent_monitor.AgentMonitorObservedState{}
			desiredState := observed.GetObservedDesiredState()
			Expect(desiredState).NotTo(BeNil())

			agentDesiredState, ok := desiredState.(*agent_monitor.AgentMonitorDesiredState)
			Expect(ok).To(BeTrue())
			Expect(agentDesiredState.ShutdownRequested()).To(BeFalse())
		})
	})
})

var _ = Describe("AgentMonitorDesiredState", func() {
	Describe("ShutdownRequested", func() {
		It("should return false by default", func() {
			desired := &agent_monitor.AgentMonitorDesiredState{}
			Expect(desired.ShutdownRequested()).To(BeFalse())
		})

		It("should return true when set", func() {
			desired := &agent_monitor.AgentMonitorDesiredState{}
			desired.SetShutdownRequested(true)
			Expect(desired.ShutdownRequested()).To(BeTrue())
		})

		It("should return false when set back", func() {
			desired := &agent_monitor.AgentMonitorDesiredState{}
			desired.SetShutdownRequested(true)
			desired.SetShutdownRequested(false)
			Expect(desired.ShutdownRequested()).To(BeFalse())
		})
	})
})
