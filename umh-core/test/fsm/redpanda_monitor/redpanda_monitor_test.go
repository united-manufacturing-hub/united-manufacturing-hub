// redpanda_monitor_fsm_test.go
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

package redpanda_monitor_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/redpanda_monitor"
	redpanda_monitor_service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda_monitor"
	serviceregistry "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// This test suite validates the Redpanda monitoring FSM, which was implemented as
// an alternative to traditional health checks. The monitor approach was chosen because
// health checks frequently time out when Redpanda to long to respond.
// The monitor tracks Redpanda's status through log parsing and metrics collection
// rather than direct health check calls.
var _ = Describe("RedpandaMonitor FSM", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc

		mockSvc *redpanda_monitor_service.MockRedpandaMonitorService
		inst    *redpanda_monitor.RedpandaMonitorInstance

		mockSvcRegistry *serviceregistry.Registry
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		// Create a mock redpanda monitor service.
		mockSvc = redpanda_monitor_service.NewMockRedpandaMonitorService()

		// Create a mock filesystem service.
		mockSvcRegistry = serviceregistry.NewMockRegistry()

		// Create a RedpandaMonitorConfig.
		cfg := config.RedpandaMonitorConfig{
			FSMInstanceConfig: config.FSMInstanceConfig{
				Name:            "monitor-redpanda-testing",
				DesiredFSMState: redpanda_monitor.OperationalStateStopped,
			},
		}

		// Create an instance using NewRedpandaMonitorInstanceWithService.
		inst = redpanda_monitor.NewRedpandaMonitorInstanceWithService(cfg, mockSvc)
	})

	AfterEach(func() {
		cancel()
	})

	Context("When newly created", func() {
		It("should initially transition from creation to operational stopped", func() {
			// On the first reconcile, the instance should process creation steps.
			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 1, SnapshotTime: time.Now()}, mockSvcRegistry)
			Expect(err).NotTo(HaveOccurred())
			Expect(did).To(BeTrue())
			// Assuming the FSM goes to a "LifecycleStateCreating" state during the creation phase.
			Expect(inst.GetCurrentFSMState()).To(Equal(internalfsm.LifecycleStateCreating))

			// On the next reconcile, the instance should complete creation and be operational in the stopped state.
			err, did = inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 2, SnapshotTime: time.Now()}, mockSvcRegistry)
			Expect(err).NotTo(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(redpanda_monitor.OperationalStateStopped))

		})
	})

	Context("Lifecycle transitions", func() {
		BeforeEach(func() {
			// Advance the instance to an operational state.
			inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 10, SnapshotTime: time.Now()}, mockSvcRegistry)
			inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 11, SnapshotTime: time.Now()}, mockSvcRegistry)
			Expect(inst.GetCurrentFSMState()).To(Equal(redpanda_monitor.OperationalStateStopped))
		})

		It("stopped -> starting -> stopped", func() {
			// Set desired state to active (e.g. "monitoring_open")
			err := inst.SetDesiredFSMState(redpanda_monitor.OperationalStateActive)
			Expect(err).NotTo(HaveOccurred())

			// Reconcile to trigger the start sequence.
			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 12, SnapshotTime: time.Now()}, mockSvcRegistry)
			Expect(err).NotTo(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(redpanda_monitor.OperationalStateStarting))

			err = inst.SetDesiredFSMState(redpanda_monitor.OperationalStateStopped)
			Expect(err).NotTo(HaveOccurred())

			err, did = inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 13, SnapshotTime: time.Now()}, mockSvcRegistry)
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(redpanda_monitor.OperationalStateStopping))

			err, did = inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 14, SnapshotTime: time.Now()}, mockSvcRegistry)
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(redpanda_monitor.OperationalStateStopped))
		})

		It("should remain stopped when desired state is stopped", func() {
			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 15, SnapshotTime: time.Now()}, mockSvcRegistry)
			Expect(err).NotTo(HaveOccurred())
			Expect(did).To(BeFalse())
			Expect(inst.GetCurrentFSMState()).To(Equal(redpanda_monitor.OperationalStateStopped))
		})
	})

	Context("When monitoring is running", func() {
		BeforeEach(func() {
			// Advance the instance to an operational state.
			inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 20, SnapshotTime: time.Now()}, mockSvcRegistry)
			inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 21, SnapshotTime: time.Now()}, mockSvcRegistry)
			err := inst.SetDesiredFSMState(redpanda_monitor.OperationalStateActive)
			Expect(err).NotTo(HaveOccurred())

			// Reconcile to trigger the start sequence: from stopped -> starting -> degraded.
			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 22, SnapshotTime: time.Now()}, mockSvcRegistry)
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(redpanda_monitor.OperationalStateStarting))

			err, did = inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 23, SnapshotTime: time.Now()}, mockSvcRegistry)
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(redpanda_monitor.OperationalStateDegraded))
		})

		It("should remain degraded when nothing is happening", func() {
			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 24, SnapshotTime: time.Now()}, mockSvcRegistry)
			Expect(err).NotTo(HaveOccurred())
			Expect(did).To(BeFalse())
			Expect(inst.GetCurrentFSMState()).To(Equal(redpanda_monitor.OperationalStateDegraded))
		})

		It("should remain degraded when the S6 service is running, but there was no last scan yet", func() {
			mockSvc.SetRedpandaMonitorRunning()

			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 25, SnapshotTime: time.Now()}, mockSvcRegistry)
			Expect(err).NotTo(HaveOccurred())
			Expect(did).To(BeFalse())
			Expect(inst.GetCurrentFSMState()).To(Equal(redpanda_monitor.OperationalStateDegraded))
		})

		It("should remain degraded when it is only ready", func() {
			mockSvc.SetRedpandaMonitorRunning()
			mockSvc.SetReadyStatus(true, true, "")
			mockSvc.SetGoodLastScan(time.Now())

			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 26, SnapshotTime: time.Now()}, mockSvcRegistry)
			Expect(err).NotTo(HaveOccurred())
			Expect(did).To(BeFalse())
			Expect(inst.GetCurrentFSMState()).To(Equal(redpanda_monitor.OperationalStateDegraded))
		})

		It("should remain degraded when it is live and ready (without metrics)", func() {
			mockSvc.SetRedpandaMonitorRunning()
			mockSvc.SetReadyStatus(true, true, "")
			mockSvc.SetLiveStatus(true)
			mockSvc.SetGoodLastScan(time.Now())

			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 26, SnapshotTime: time.Now()}, mockSvcRegistry)
			Expect(err).NotTo(HaveOccurred())
			Expect(did).To(BeFalse())
			Expect(inst.GetCurrentFSMState()).To(Equal(redpanda_monitor.OperationalStateDegraded))
		})

		It("should transition to active when it is live and ready (with metrics)", func() {
			mockSvc.SetRedpandaMonitorRunning()
			mockSvc.SetReadyStatus(true, true, "")
			mockSvc.SetLiveStatus(true)
			mockSvc.SetMetricsState(true)
			mockSvc.SetGoodLastScan(time.Now())

			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 26, SnapshotTime: time.Now()}, mockSvcRegistry)
			Expect(err).NotTo(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(redpanda_monitor.OperationalStateActive))
		})

		It("should transition to active and then back to degraded when the metrics are not available", func() {
			mockSvc.SetRedpandaMonitorRunning()
			mockSvc.SetReadyStatus(true, true, "")
			mockSvc.SetLiveStatus(true)
			mockSvc.SetMetricsState(true)
			currentTime := time.Now()
			mockSvc.SetGoodLastScan(currentTime)

			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 26, SnapshotTime: currentTime}, mockSvcRegistry)
			Expect(err).NotTo(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(redpanda_monitor.OperationalStateActive))

			mockSvc.SetMetricsState(false) // this means redpanda is not active, but the monitor still is to it remains active
			currentTime = currentTime.Add(1 * time.Second)

			err, did = inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 27, SnapshotTime: currentTime}, mockSvcRegistry)
			Expect(err).NotTo(HaveOccurred())
			Expect(did).To(BeFalse())
			Expect(inst.GetCurrentFSMState()).To(Equal(redpanda_monitor.OperationalStateActive))

			mockSvc.SetOutdatedLastScan(currentTime)
			currentTime = currentTime.Add(1 * time.Second)

			err, did = inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 28, SnapshotTime: currentTime}, mockSvcRegistry)
			Expect(err).NotTo(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(redpanda_monitor.OperationalStateDegraded))
		})
	})
})
