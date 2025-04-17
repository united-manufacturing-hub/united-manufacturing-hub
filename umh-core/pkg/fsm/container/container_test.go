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

package container_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/backoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/container"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/container_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

var _ = Describe("Container FSM", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc

		mockSvc *container_monitor.MockService
		inst    *container.ContainerInstance

		mockFS *filesystem.MockFileSystem
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		mockSvc = container_monitor.NewMockService()
		// By default, let's set it up for healthy
		mockSvc.SetupMockForHealthyState()

		mockFS = filesystem.NewMockFileSystem()

		cfg := config.ContainerConfig{
			Name:            "test-container",
			DesiredFSMState: container.OperationalStateStopped,
		}
		inst = container.NewContainerInstanceWithService(cfg, mockSvc)
	})

	AfterEach(func() {
		cancel()
	})

	Context("When newly created", func() {
		It("Should initially be in lifecycle state `to_be_created` -> then `creating` -> `monitoring_stopped`", func() {
			// On first reconcile, it should handle creation
			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 1}, mockFS)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal("creating"))

			// next reconcile
			err, did = inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 2}, mockFS)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			// now we should be in operational state 'monitoring_stopped'
			Expect(inst.GetCurrentFSMState()).To(Equal(container.OperationalStateStopped))
		})
	})

	Context("Lifecycle transitions", func() {
		BeforeEach(func() {
			// Ensure we've walked from to_be_created -> creating -> monitoring_stopped
			inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 10}, mockFS)
			inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 11}, mockFS)
			Expect(inst.GetCurrentFSMState()).To(Equal(container.OperationalStateStopped))
		})

		It("Should go from `monitoring_stopped` to `degraded` on start", func() {
			// set desired state to active
			inst.SetDesiredFSMState(container.OperationalStateActive)

			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 12}, mockFS)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(container.OperationalStateStarting))

			// next reconcile
			err, did = inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 13}, mockFS)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(container.OperationalStateDegraded))
		})

		It("Should remain `monitoring_stopped` if desired is `stopped`", func() {
			// do one reconcile - no state change
			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 14}, mockFS)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeFalse())
			Expect(inst.GetCurrentFSMState()).To(Equal(container.OperationalStateStopped))
		})
	})

	Context("When monitoring is running", func() {
		BeforeEach(func() {
			// get to monitoring_stopped
			inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 20}, mockFS)
			inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 21}, mockFS)
			// set desired = active
			inst.SetDesiredFSMState(container.OperationalStateActive)
			// cause start
			inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 22}, mockFS)
			Expect(inst.GetCurrentFSMState()).To(Equal(container.OperationalStateStarting))

			// next reconcile
			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 23}, mockFS)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(container.OperationalStateDegraded))
		})

		It("Transitions from degraded -> active if metrics healthy", func() {
			// currently mockSvc returns healthy => we expect a transition
			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 24}, mockFS)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(container.OperationalStateActive))
		})

		It("Remains degraded if metrics are not healthy", func() {
			// Let's set the mock to return critical metrics
			mockSvc.SetupMockForDegradedState()

			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 25}, mockFS)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeFalse()) // no transition => still degraded
			Expect(inst.GetCurrentFSMState()).To(Equal(container.OperationalStateDegraded))
		})

		It("throw permanent Error when it fails to remove because not in transit state", func() {
			// currently mockSvc returns healthy => we expect a transition
			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 26}, mockFS)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(container.OperationalStateActive))

			mockSvc.SetupMockForError(fmt.Errorf("permanent failure: %s", backoff.PermanentFailureError))

			err, did = inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 27}, mockFS)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeFalse())
			Expect(inst.GetCurrentFSMState()).To(Equal(container.OperationalStateActive))

			err, did = inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 27}, mockFS)
			Expect(err).To(HaveOccurred())
			Expect(did).To(BeFalse())
			Expect(inst.GetCurrentFSMState()).To(Equal(container.OperationalStateActive))

		})
	})
})
