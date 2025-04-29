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

package benthos_monitor_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos_monitor"
	benthos_monitor_svc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

var _ = Describe("Benthos Monitor FSM", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc

		mockSvc *benthos_monitor_svc.MockBenthosMonitorService
		inst    *benthos_monitor.BenthosMonitorInstance

		mockServices *serviceregistry.Registry
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		mockServices = serviceregistry.NewMockRegistry()
		mockSvc = benthos_monitor_svc.NewMockBenthosMonitorService()
		// By default, let's set it up for healthy
		mockSvc.SetServiceState(benthos_monitor_svc.ServiceStateFlags{
			IsRunning:       true,
			IsMetricsActive: true,
		})

		// Set a recent last scan time to pass the staleness check
		mockSvc.SetGoodLastScan(time.Now())

		// BenthosMonitorConfig embeds FSMInstanceConfig which has name and desiredState fields
		cfg := config.BenthosMonitorConfig{
			FSMInstanceConfig: config.FSMInstanceConfig{
				Name:            "test-benthos",
				DesiredFSMState: benthos_monitor.OperationalStateStopped,
			},
			MetricsPort: 9091,
		}
		inst = benthos_monitor.NewBenthosMonitorInstanceWithService(cfg, mockSvc)
	})

	AfterEach(func() {
		cancel()
	})

	Context("When newly created", func() {
		It("Should initially be in lifecycle state `to_be_created` -> then `creating` -> `benthos_monitoring_stopped`", func() {
			// On first reconcile, it should handle creation
			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 1}, mockServices)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal("creating"))

			// next reconcile
			err, did = inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 2}, mockServices)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			// now we should be in operational state 'benthos_monitoring_stopped'
			Expect(inst.GetCurrentFSMState()).To(Equal(benthos_monitor.OperationalStateStopped))
		})
	})

	Context("Lifecycle transitions", func() {
		BeforeEach(func() {
			// Ensure we've walked from to_be_created -> creating -> benthos_monitoring_stopped
			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 10}, mockServices)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			err, did = inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 11}, mockServices)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(benthos_monitor.OperationalStateStopped))
		})

		It("Should go from `benthos_monitoring_stopped` to `degraded` on start", func() {
			// set desired state to active
			err := inst.SetDesiredFSMState(benthos_monitor.OperationalStateActive)
			Expect(err).ToNot(HaveOccurred())

			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 12}, mockServices)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(benthos_monitor.OperationalStateStarting))

			// next reconcile
			err, did = inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 13}, mockServices)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(benthos_monitor.OperationalStateDegraded))
		})

		It("Should remain `benthos_monitoring_stopped` if desired is `stopped`", func() {
			// do one reconcile - no state change
			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 14}, mockServices)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeFalse())
			Expect(inst.GetCurrentFSMState()).To(Equal(benthos_monitor.OperationalStateStopped))
		})
	})

	Context("When monitoring is running", func() {
		BeforeEach(func() {
			// get to benthos_monitoring_stopped
			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 20}, mockServices)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			err, did = inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 21}, mockServices)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			// set desired = active
			err = inst.SetDesiredFSMState(benthos_monitor.OperationalStateActive)
			Expect(err).ToNot(HaveOccurred())
			// cause start
			err, did = inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 22}, mockServices)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(benthos_monitor.OperationalStateStarting))

			// next reconcile
			err, did = inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 23}, mockServices)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(benthos_monitor.OperationalStateDegraded))
		})

		It("Transitions from degraded -> active if metrics healthy", func() {
			// Make sure the mock service has valid, recent last scan time and metrics
			mockSvc.SetGoodLastScan(time.Now())

			// Setup mock with ready/live status to pass health checks
			mockSvc.SetReadyStatus(true, true, "")
			mockSvc.SetLiveStatus(true)

			// Add some example metrics
			mockSvc.SetMetricsResponse(benthos_monitor_svc.Metrics{
				Input: benthos_monitor_svc.InputMetrics{
					ConnectionUp: 1,
				},
				Output: benthos_monitor_svc.OutputMetrics{
					ConnectionUp: 1,
				},
			})

			// The SystemSnapshot contains the time in SnapshotTime field
			snapshot := fsm.SystemSnapshot{
				Tick:         24,
				SnapshotTime: time.Now(),
			}

			// Currently mockSvc returns healthy metrics => we expect a transition
			err, did := inst.Reconcile(ctx, snapshot, mockServices)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(benthos_monitor.OperationalStateActive))
		})

		It("Remains degraded if metrics are not healthy", func() {
			// IMPORTANT: Looking at the isMonitorHealthy method in actions.go,
			// it returns false if BenthosMetrics is nil, so we need to create a service
			// state where ServiceInfo is present, LastScan is present, but BenthosMetrics is nil

			// Create a fresh mock with basic setup - we don't want any previous metrics settings
			freshMock := benthos_monitor_svc.NewMockBenthosMonitorService()

			// Replace the existing mock in our instance
			cfg := config.BenthosMonitorConfig{
				FSMInstanceConfig: config.FSMInstanceConfig{
					Name:            "test-benthos",
					DesiredFSMState: benthos_monitor.OperationalStateActive,
				},
				MetricsPort: 9091,
			}
			// Create a new instance with our fresh mock
			freshInst := benthos_monitor.NewBenthosMonitorInstanceWithService(cfg, freshMock)

			// Get it to the degraded state
			err, did := freshInst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 1}, mockServices)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue()) // to_be_created -> creating

			err, did = freshInst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 2}, mockServices)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue()) // creating -> stopped

			err, did = freshInst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 3}, mockServices)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue()) // stopped -> starting

			err, did = freshInst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 4}, mockServices)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue()) // starting -> degraded

			Expect(freshInst.GetCurrentFSMState()).To(Equal(benthos_monitor.OperationalStateDegraded))

			// Setup necessary state but WITHOUT BenthosMetrics
			mockSvc.SetServiceState(benthos_monitor_svc.ServiceStateFlags{
				IsRunning:       true,
				IsMetricsActive: false, // set this to false
			})

			// Set a recent scan time to pass the staleness check
			freshMock.SetGoodLastScan(time.Now())

			// Set some valid service states, but make sure to not add metrics
			freshMock.SetReadyStatus(false, false, "Some error")
			freshMock.SetLiveStatus(false)

			// But make sure BenthosMetrics is nil by creating a ServiceInfo directly
			// Since we know from the isMonitorHealthy method it checks if b.ObservedState.ServiceInfo.BenthosStatus.LastScan.BenthosMetrics is nil
			freshMock.ServiceState = &benthos_monitor_svc.ServiceInfo{
				BenthosStatus: benthos_monitor_svc.BenthosMonitorStatus{
					LastScan: &benthos_monitor_svc.BenthosMetricsScan{
						LastUpdatedAt: time.Now(),
						// Deliberately not setting BenthosMetrics, so it will be nil
					},
				},
			}

			snapshot := fsm.SystemSnapshot{
				Tick:         25,
				SnapshotTime: time.Now(),
			}

			err, did = freshInst.Reconcile(ctx, snapshot, mockServices)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeFalse()) // no transition => still degraded
			Expect(freshInst.GetCurrentFSMState()).To(Equal(benthos_monitor.OperationalStateDegraded))
		})

		It("Should transition to stopping when desired state is stopped", func() {
			// First get to active state
			// Setup for healthy metrics
			mockSvc.SetGoodLastScan(time.Now())
			mockSvc.SetReadyStatus(true, true, "")
			mockSvc.SetLiveStatus(true)
			mockSvc.SetMetricsResponse(benthos_monitor_svc.Metrics{
				Input: benthos_monitor_svc.InputMetrics{
					ConnectionUp: 1,
				},
				Output: benthos_monitor_svc.OutputMetrics{
					ConnectionUp: 1,
				},
			})

			snapshot1 := fsm.SystemSnapshot{
				Tick:         26,
				SnapshotTime: time.Now(),
			}

			err, did := inst.Reconcile(ctx, snapshot1, mockServices)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(benthos_monitor.OperationalStateActive))

			// Set desired state to stopped
			err = inst.SetDesiredFSMState(benthos_monitor.OperationalStateStopped)
			Expect(err).ToNot(HaveOccurred())

			// Should transition to stopping
			snapshot2 := fsm.SystemSnapshot{
				Tick:         27,
				SnapshotTime: time.Now(),
			}

			err, did = inst.Reconcile(ctx, snapshot2, mockServices)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(benthos_monitor.OperationalStateStopping))

			// Next reconcile should go to stopped
			snapshot3 := fsm.SystemSnapshot{
				Tick:         28,
				SnapshotTime: time.Now(),
			}

			err, did = inst.Reconcile(ctx, snapshot3, mockServices)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(benthos_monitor.OperationalStateStopped))
		})
	})
})
