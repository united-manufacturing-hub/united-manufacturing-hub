// benthos_monitor_fsm_test.go
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

//go:build test
// +build test

package benthos_monitor_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos_monitor"
	benthos_monitor_service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// This test suite validates the Benthos monitoring FSM, which was implemented as
// an alternative to traditional health checks. The monitor approach was chosen because
// health checks frequently time out when Benthos to long to respond.
// The monitor tracks Benthos's status through log parsing and metrics collection
// rather than direct health check calls.
var _ = Describe("BenthosMonitor FSM", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc

		mockSvc *benthos_monitor_service.MockBenthosMonitorService
		inst    *benthos_monitor.BenthosMonitorInstance

		mockServices *serviceregistry.Registry
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		// Create a mock benthos monitor service.
		mockSvc = benthos_monitor_service.NewMockBenthosMonitorService()

		// Create a mock filesystem service.
		mockServices = serviceregistry.NewMockRegistry()

		// Create a BenthosMonitorConfig.
		cfg := config.BenthosMonitorConfig{
			FSMInstanceConfig: config.FSMInstanceConfig{
				Name:            "monitor-benthos-testing",
				DesiredFSMState: benthos_monitor.OperationalStateStopped,
			},
			MetricsPort: 8080,
		}

		// Create an instance using NewBenthosMonitorInstanceWithService.
		inst = benthos_monitor.NewBenthosMonitorInstanceWithService(cfg, mockSvc)
	})

	AfterEach(func() {
		cancel()
	})

	Context("When newly created", func() {
		It("should initially transition from creation to operational stopped", func() {
			// On the first reconcile, the instance should process creation steps.
			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 1, SnapshotTime: time.Now()}, mockServices)
			Expect(err).NotTo(HaveOccurred())
			Expect(did).To(BeTrue())
			// Assuming the FSM goes to a "LifecycleStateCreating" state during the creation phase.
			Expect(inst.GetCurrentFSMState()).To(Equal(internalfsm.LifecycleStateCreating))

			// On the next reconcile, the instance should complete creation and be operational in the stopped state.
			err, did = inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 2, SnapshotTime: time.Now()}, mockServices)
			Expect(err).NotTo(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(benthos_monitor.OperationalStateStopped))

		})
	})

	Context("Lifecycle transitions", func() {
		BeforeEach(func() {
			// Advance the instance to an operational state.
			inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 10, SnapshotTime: time.Now()}, mockServices)
			inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 11, SnapshotTime: time.Now()}, mockServices)
			Expect(inst.GetCurrentFSMState()).To(Equal(benthos_monitor.OperationalStateStopped))
		})

		It("stopped -> starting -> stopped", func() {
			// Set desired state to active (e.g. "monitoring_open")
			err := inst.SetDesiredFSMState(benthos_monitor.OperationalStateActive)
			Expect(err).NotTo(HaveOccurred())

			// Reconcile to trigger the start sequence.
			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 12, SnapshotTime: time.Now()}, mockServices)
			Expect(err).NotTo(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(benthos_monitor.OperationalStateStarting))

			err = inst.SetDesiredFSMState(benthos_monitor.OperationalStateStopped)
			Expect(err).NotTo(HaveOccurred())

			err, did = inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 13, SnapshotTime: time.Now()}, mockServices)
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(benthos_monitor.OperationalStateStopping))

			err, did = inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 14, SnapshotTime: time.Now()}, mockServices)
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(benthos_monitor.OperationalStateStopped))
		})

		It("should remain stopped when desired state is stopped", func() {
			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 15, SnapshotTime: time.Now()}, mockServices)
			Expect(err).NotTo(HaveOccurred())
			Expect(did).To(BeFalse())
			Expect(inst.GetCurrentFSMState()).To(Equal(benthos_monitor.OperationalStateStopped))
		})
	})

	Context("When monitoring is running", func() {
		BeforeEach(func() {
			// Advance the instance to an operational state.
			inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 20, SnapshotTime: time.Now()}, mockServices)
			inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 21, SnapshotTime: time.Now()}, mockServices)
			err := inst.SetDesiredFSMState(benthos_monitor.OperationalStateActive)
			Expect(err).NotTo(HaveOccurred())

			// Reconcile to trigger the start sequence: from stopped -> starting -> degraded.
			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 22, SnapshotTime: time.Now()}, mockServices)
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(benthos_monitor.OperationalStateStarting))

			err, did = inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 23, SnapshotTime: time.Now()}, mockServices)
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(benthos_monitor.OperationalStateDegraded))
		})

		It("should remain degraded when nothing is happening", func() {
			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 24, SnapshotTime: time.Now()}, mockServices)
			Expect(err).NotTo(HaveOccurred())
			Expect(did).To(BeFalse())
			Expect(inst.GetCurrentFSMState()).To(Equal(benthos_monitor.OperationalStateDegraded))
		})

		It("should remain degraded when the S6 service is running, but there was no last scan yet", func() {
			mockSvc.SetBenthosMonitorRunning()

			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 25, SnapshotTime: time.Now()}, mockServices)
			Expect(err).NotTo(HaveOccurred())
			Expect(did).To(BeFalse())
			Expect(inst.GetCurrentFSMState()).To(Equal(benthos_monitor.OperationalStateDegraded))
		})

		It("should remain degraded when it is only ready", func() {
			mockSvc.SetBenthosMonitorRunning()
			mockSvc.SetReadyStatus(true, true, "")
			mockSvc.SetGoodLastScan(time.Now())

			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 26, SnapshotTime: time.Now()}, mockServices)
			Expect(err).NotTo(HaveOccurred())
			Expect(did).To(BeFalse())
			Expect(inst.GetCurrentFSMState()).To(Equal(benthos_monitor.OperationalStateDegraded))
		})

		It("should remain degraded when it is live and ready (without metrics)", func() {
			mockSvc.SetBenthosMonitorRunning()
			mockSvc.SetReadyStatus(true, true, "")
			mockSvc.SetLiveStatus(true)
			mockSvc.SetGoodLastScan(time.Now())

			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 26, SnapshotTime: time.Now()}, mockServices)
			Expect(err).NotTo(HaveOccurred())
			Expect(did).To(BeFalse())
			Expect(inst.GetCurrentFSMState()).To(Equal(benthos_monitor.OperationalStateDegraded))
		})

		It("should transition to active when it is live and ready (with metrics)", func() {
			mockSvc.SetBenthosMonitorRunning()
			mockSvc.SetReadyStatus(true, true, "")
			mockSvc.SetLiveStatus(true)
			mockSvc.SetMetricsState(true)
			mockSvc.SetGoodLastScan(time.Now())

			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 26, SnapshotTime: time.Now()}, mockServices)
			Expect(err).NotTo(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(benthos_monitor.OperationalStateActive))
		})

		It("should transition to active and then back to degraded when the metrics are not available", func() {
			mockSvc.SetBenthosMonitorRunning()
			mockSvc.SetReadyStatus(true, true, "")
			mockSvc.SetLiveStatus(true)
			mockSvc.SetMetricsState(true)
			currentTime := time.Now()
			mockSvc.SetGoodLastScan(currentTime)

			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 26, SnapshotTime: currentTime}, mockServices)
			Expect(err).NotTo(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(benthos_monitor.OperationalStateActive))

			mockSvc.SetMetricsState(false) // this means benthos is not active, but the monitor still is to it remains active
			currentTime = currentTime.Add(1 * time.Second)

			err, did = inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 27, SnapshotTime: currentTime}, mockServices)
			Expect(err).NotTo(HaveOccurred())
			Expect(did).To(BeFalse())
			Expect(inst.GetCurrentFSMState()).To(Equal(benthos_monitor.OperationalStateActive))

			mockSvc.SetOutdatedLastScan(currentTime)
			currentTime = currentTime.Add(1 * time.Second)

			err, did = inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 28, SnapshotTime: currentTime}, mockServices)
			Expect(err).NotTo(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(benthos_monitor.OperationalStateDegraded))
		})
	})
})
