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
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/agent_monitor"
	agentmonitorsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/agent_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

var _ = Describe("Agent FSM", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc

		mockSvc *agentmonitorsvc.MockService
		inst    *agent_monitor.AgentInstance

		mockFS *filesystem.MockFileSystem
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		mockSvc = agentmonitorsvc.NewMockService()
		// By default, let's set it up for healthy
		mockSvc.SetupMockForHealthyState()

		mockFS = filesystem.NewMockFileSystem()

		cfg := config.AgentMonitorConfig{
			Name:            "test-agent",
			DesiredFSMState: agent_monitor.OperationalStateStopped,
		}
		inst = agent_monitor.NewAgentInstanceWithService(cfg, mockSvc)
	})

	AfterEach(func() {
		cancel()
	})

	Context("When newly created", func() {
		It("Should initially be in lifecycle state `to_be_created` -> then `creating` -> `agent_monitoring_stopped`", func() {
			// On first reconcile, it should handle creation
			err, did := inst.Reconcile(ctx, mockFS, 1)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal("creating"))

			// next reconcile
			err, did = inst.Reconcile(ctx, mockFS, 2)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			// now we should be in operational state 'agent_monitoring_stopped'
			Expect(inst.GetCurrentFSMState()).To(Equal(agent_monitor.OperationalStateStopped))
		})
	})

	Context("Lifecycle transitions", func() {
		BeforeEach(func() {
			// Ensure we've walked from to_be_created -> creating -> agent_monitoring_stopped
			inst.Reconcile(ctx, mockFS, 10)
			inst.Reconcile(ctx, mockFS, 11)
			Expect(inst.GetCurrentFSMState()).To(Equal(agent_monitor.OperationalStateStopped))
		})

		It("Should go from `agent_monitoring_stopped` to `degraded` on start", func() {
			// set desired state to active
			inst.SetDesiredFSMState(agent_monitor.OperationalStateActive)

			err, did := inst.Reconcile(ctx, mockFS, 12)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(agent_monitor.OperationalStateStarting))

			// next reconcile
			err, did = inst.Reconcile(ctx, mockFS, 13)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(agent_monitor.OperationalStateDegraded))
		})

		It("Should remain `agent_monitoring_stopped` if desired is `stopped`", func() {
			// do one reconcile - no state change
			err, did := inst.Reconcile(ctx, mockFS, 14)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeFalse())
			Expect(inst.GetCurrentFSMState()).To(Equal(agent_monitor.OperationalStateStopped))
		})
	})

	Context("When monitoring is running", func() {
		BeforeEach(func() {
			// get to agent_monitoring_stopped
			inst.Reconcile(ctx, mockFS, 20)
			inst.Reconcile(ctx, mockFS, 21)
			// set desired = active
			inst.SetDesiredFSMState(agent_monitor.OperationalStateActive)
			// cause start
			inst.Reconcile(ctx, mockFS, 22)
			Expect(inst.GetCurrentFSMState()).To(Equal(agent_monitor.OperationalStateStarting))

			// next reconcile
			err, did := inst.Reconcile(ctx, mockFS, 23)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(agent_monitor.OperationalStateDegraded))
		})

		It("Transitions from degraded -> active if metrics healthy", func() {
			// currently mockSvc returns healthy => we expect a transition
			err, did := inst.Reconcile(ctx, mockFS, 24)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(agent_monitor.OperationalStateActive))
		})

		It("Remains degraded if metrics are not healthy", func() {
			// Let's set the mock to return critical metrics
			mockSvc.SetupMockForDegradedState()

			err, did := inst.Reconcile(ctx, mockFS, 25)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeFalse()) // no transition => still degraded
			Expect(inst.GetCurrentFSMState()).To(Equal(agent_monitor.OperationalStateDegraded))
		})
	})
})
