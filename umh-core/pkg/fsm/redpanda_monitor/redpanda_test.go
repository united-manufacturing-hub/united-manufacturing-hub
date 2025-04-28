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

package redpanda_monitor_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/redpanda_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	redpanda_monitor_svc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda_monitor"
)

var _ = Describe("Redpanda Monitor FSM", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc

		mockSvc *redpanda_monitor_svc.MockRedpandaMonitorService
		inst    *redpanda_monitor.RedpandaMonitorInstance

		mockFS *filesystem.MockFileSystem
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		mockFS = filesystem.NewMockFileSystem()

		mockSvc = redpanda_monitor_svc.NewMockRedpandaMonitorService()
		// By default, let's set it up for healthy
		mockSvc.SetServiceState(redpanda_monitor_svc.ServiceStateFlags{
			IsRunning:       true,
			IsMetricsActive: true,
		})

		// Set a recent last scan time to pass the staleness check
		mockSvc.SetGoodLastScan(time.Now())

		// RedpandaMonitorConfig embeds FSMInstanceConfig which has name and desiredState fields
		cfg := config.RedpandaMonitorConfig{
			FSMInstanceConfig: config.FSMInstanceConfig{
				Name:            "test-redpanda",
				DesiredFSMState: redpanda_monitor.OperationalStateStopped,
			},
		}
		inst = redpanda_monitor.NewRedpandaMonitorInstanceWithService(cfg, mockSvc)
	})

	AfterEach(func() {
		cancel()
	})

	Context("When newly created", func() {
		It("Should initially be in lifecycle state `to_be_created` -> then `creating` -> `redpanda_monitoring_stopped`", func() {
			// On first reconcile, it should handle creation
			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 1}, mockFS)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal("creating"))

			// next reconcile
			err, did = inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 2}, mockFS)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			// now we should be in operational state 'redpanda_monitoring_stopped'
			Expect(inst.GetCurrentFSMState()).To(Equal(redpanda_monitor.OperationalStateStopped))
		})
	})

	Context("Lifecycle transitions", func() {
		BeforeEach(func() {
			// Ensure we've walked from to_be_created -> creating -> redpanda_monitoring_stopped
			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 10}, mockFS)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			err, did = inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 11}, mockFS)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(redpanda_monitor.OperationalStateStopped))
		})

		It("Should go from `redpanda_monitoring_stopped` to `degraded` on start", func() {
			// set desired state to active
			err := inst.SetDesiredFSMState(redpanda_monitor.OperationalStateActive)
			Expect(err).ToNot(HaveOccurred())

			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 12}, mockFS)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(redpanda_monitor.OperationalStateStarting))

			// next reconcile
			err, did = inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 13}, mockFS)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(redpanda_monitor.OperationalStateDegraded))
		})

		It("Should remain `redpanda_monitoring_stopped` if desired is `stopped`", func() {
			// do one reconcile - no state change
			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 14}, mockFS)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeFalse())
			Expect(inst.GetCurrentFSMState()).To(Equal(redpanda_monitor.OperationalStateStopped))
		})
	})

	Context("When monitoring is running", func() {
		BeforeEach(func() {
			// get to redpanda_monitoring_stopped
			err, did := inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 20}, mockFS)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			err, did = inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 21}, mockFS)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			// set desired = active
			err = inst.SetDesiredFSMState(redpanda_monitor.OperationalStateActive)
			Expect(err).ToNot(HaveOccurred())
			// cause start
			err, did = inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 22}, mockFS)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(redpanda_monitor.OperationalStateStarting))

			// next reconcile
			err, did = inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 23}, mockFS)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(redpanda_monitor.OperationalStateDegraded))
		})

		It("Transitions from degraded -> active if metrics healthy", func() {
			// Make sure the mock service has valid, recent last scan time and metrics
			mockSvc.SetGoodLastScan(time.Now())

			// Setup mock with ready/live status to pass health checks
			mockSvc.SetReadyStatus(true, true, "")
			mockSvc.SetLiveStatus(true)

			// Add some example metrics
			mockSvc.SetMetricsResponse(redpanda_monitor_svc.Metrics{
				Infrastructure: redpanda_monitor_svc.InfrastructureMetrics{
					Storage: redpanda_monitor_svc.StorageMetrics{
						FreeBytes:      1000,
						TotalBytes:     10000,
						FreeSpaceAlert: false,
					},
				},
				Cluster: redpanda_monitor_svc.ClusterMetrics{
					Topics:            10,
					UnavailableTopics: 0,
				},
				Throughput: redpanda_monitor_svc.ThroughputMetrics{
					BytesIn:  1000,
					BytesOut: 1000,
				},
				Topic: redpanda_monitor_svc.TopicMetrics{
					TopicPartitionMap: map[string]int64{
						"test-topic": 6,
					},
				},
			})

			// The SystemSnapshot contains the time in SnapshotTime field
			snapshot := fsm.SystemSnapshot{
				Tick:         24,
				SnapshotTime: time.Now(),
			}

			// Currently mockSvc returns healthy metrics => we expect a transition
			err, did := inst.Reconcile(ctx, snapshot, mockFS)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(redpanda_monitor.OperationalStateActive))
		})

		It("Remains degraded if metrics are not healthy", func() {
			// IMPORTANT: Looking at the isMonitorHealthy method in actions.go,
			// it returns false if RedpandaMetrics is nil, so we need to create a service
			// state where ServiceInfo is present, LastScan is present, but RedpandaMetrics is nil

			// Create a fresh mock with basic setup - we don't want any previous metrics settings
			freshMock := redpanda_monitor_svc.NewMockRedpandaMonitorService()

			// Replace the existing mock in our instance
			cfg := config.RedpandaMonitorConfig{
				FSMInstanceConfig: config.FSMInstanceConfig{
					Name:            "test-redpanda",
					DesiredFSMState: redpanda_monitor.OperationalStateActive,
				},
			}
			// Create a new instance with our fresh mock
			freshInst := redpanda_monitor.NewRedpandaMonitorInstanceWithService(cfg, freshMock)

			// Get it to the degraded state
			err, did := freshInst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 1}, mockFS)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue()) // to_be_created -> creating

			err, did = freshInst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 2}, mockFS)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue()) // creating -> stopped

			err, did = freshInst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 3}, mockFS)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue()) // stopped -> starting

			err, did = freshInst.Reconcile(ctx, fsm.SystemSnapshot{Tick: 4}, mockFS)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue()) // starting -> degraded

			Expect(freshInst.GetCurrentFSMState()).To(Equal(redpanda_monitor.OperationalStateDegraded))

			// Setup necessary state but WITHOUT RedpandaMetrics
			mockSvc.SetServiceState(redpanda_monitor_svc.ServiceStateFlags{
				IsRunning:       true,
				IsMetricsActive: false, // set this to false
			})

			// Set a recent scan time to pass the staleness check
			freshMock.SetGoodLastScan(time.Now())

			// Set some valid service states, but make sure to not add metrics
			freshMock.SetReadyStatus(false, false, "Some error")
			freshMock.SetLiveStatus(false)

			// But make sure RedpandaMetrics is nil by creating a ServiceInfo directly
			// Since we know from the isMonitorHealthy method it checks if b.ObservedState.ServiceInfo.RedpandaStatus.LastScan.RedpandaMetrics is nil
			freshMock.ServiceState = &redpanda_monitor_svc.ServiceInfo{
				RedpandaStatus: redpanda_monitor_svc.RedpandaMonitorStatus{
					LastScan: &redpanda_monitor_svc.RedpandaMetricsScan{
						LastUpdatedAt: time.Now(),
						// Deliberately not setting RedpandaMetrics, so it will be nil
					},
				},
			}

			snapshot := fsm.SystemSnapshot{
				Tick:         25,
				SnapshotTime: time.Now(),
			}

			err, did = freshInst.Reconcile(ctx, snapshot, mockFS)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeFalse()) // no transition => still degraded
			Expect(freshInst.GetCurrentFSMState()).To(Equal(redpanda_monitor.OperationalStateDegraded))
		})

		It("Should transition to stopping when desired state is stopped", func() {
			// First get to active state
			// Setup for healthy metrics
			mockSvc.SetGoodLastScan(time.Now())
			mockSvc.SetReadyStatus(true, true, "")
			mockSvc.SetLiveStatus(true)
			mockSvc.SetMetricsResponse(redpanda_monitor_svc.Metrics{
				Infrastructure: redpanda_monitor_svc.InfrastructureMetrics{
					Storage: redpanda_monitor_svc.StorageMetrics{
						FreeBytes:      1000,
						TotalBytes:     10000,
						FreeSpaceAlert: false,
					},
				},
				Cluster: redpanda_monitor_svc.ClusterMetrics{
					Topics:            1,
					UnavailableTopics: 0,
				},
				Throughput: redpanda_monitor_svc.ThroughputMetrics{
					BytesIn:  1000,
					BytesOut: 1000,
				},
				Topic: redpanda_monitor_svc.TopicMetrics{
					TopicPartitionMap: map[string]int64{
						"test-topic": 6,
					},
				},
			})

			snapshot1 := fsm.SystemSnapshot{
				Tick:         26,
				SnapshotTime: time.Now(),
			}

			err, did := inst.Reconcile(ctx, snapshot1, mockFS)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(redpanda_monitor.OperationalStateActive))

			// Set desired state to stopped
			err = inst.SetDesiredFSMState(redpanda_monitor.OperationalStateStopped)
			Expect(err).ToNot(HaveOccurred())

			// Should transition to stopping
			snapshot2 := fsm.SystemSnapshot{
				Tick:         27,
				SnapshotTime: time.Now(),
			}

			err, did = inst.Reconcile(ctx, snapshot2, mockFS)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(redpanda_monitor.OperationalStateStopping))

			// Next reconcile should go to stopped
			snapshot3 := fsm.SystemSnapshot{
				Tick:         28,
				SnapshotTime: time.Now(),
			}

			err, did = inst.Reconcile(ctx, snapshot3, mockFS)
			Expect(err).ToNot(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(redpanda_monitor.OperationalStateStopped))
		})
	})
})
