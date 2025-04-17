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

package redpanda_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	s6 "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/redpandaserviceconfig"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda_monitor"
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/storage"
)

var _ = Describe("RedpandaService State Transitions", func() {
	var (
		mockS6Service      *s6service.MockService
		mockFileSystem     *filesystem.MockFileSystem
		mockMonitorService *redpanda_monitor.MockRedpandaMonitorService
		redpandaService    *redpanda.RedpandaService
		ctx                context.Context
		cancel             context.CancelFunc
		redpandaConfig     *redpandaserviceconfig.RedpandaServiceConfig
		archiveStorage     storage.ArchiveStorer
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		mockS6Service = s6service.NewMockService()
		mockFileSystem = filesystem.NewMockFileSystem()
		mockMonitorService = redpanda_monitor.NewMockRedpandaMonitorService()

		// Set up mock logs for S6 service
		mockS6Service.GetLogsResult = createRedpandaMockLogs()

		// Set default state to stopped
		mockS6Service.StatusResult = s6service.ServiceInfo{
			Status: s6service.ServiceDown,
		}

		// Create a mocked S6 manager to prevent using real S6 functionality
		mockedS6Manager := s6fsm.NewS6ManagerWithMockedServices("redpanda")

		// Set up the mock monitor service with valid data structures
		mockMonitorService.ServiceExistsResult = true

		// Initialize metrics and metrics state for the monitor
		mockMonitorService.SetMetricsState(false, 10000000000, 20000000000, false)

		// Setup mock monitor's Status response with valid LastScan
		monitorMetrics := &redpanda_monitor.RedpandaMetrics{
			Metrics: redpanda_monitor.Metrics{
				Infrastructure: redpanda_monitor.InfrastructureMetrics{
					Storage: redpanda_monitor.StorageMetrics{
						FreeBytes:      10000000000,
						TotalBytes:     20000000000,
						FreeSpaceAlert: false,
					},
				},
				Cluster: redpanda_monitor.ClusterMetrics{
					Topics:            5,
					UnavailableTopics: 0,
				},
				Throughput: redpanda_monitor.ThroughputMetrics{
					BytesIn:  1000,
					BytesOut: 2000,
				},
				Topic: redpanda_monitor.TopicMetrics{
					TopicPartitionMap: map[string]int64{"test-topic": 3},
				},
			},
			MetricsState: redpanda_monitor.NewRedpandaMetricsState(),
		}
		mockMonitorService.StatusResult = redpanda_monitor.ServiceInfo{
			S6FSMState: s6fsm.OperationalStateStopped,
			RedpandaStatus: redpanda_monitor.RedpandaMonitorStatus{
				IsRunning: false,
				LastScan: &redpanda_monitor.RedpandaMetricsAndClusterConfig{
					Metrics: monitorMetrics,
					ClusterConfig: &redpanda_monitor.ClusterConfig{
						Topic: redpanda_monitor.TopicConfig{
							DefaultTopicRetentionMs:    1000000,
							DefaultTopicRetentionBytes: 1000000000,
						},
					},
					LastUpdatedAt: time.Now(),
				},
			},
		}

		// Create basic redpanda config
		redpandaConfig = &redpandaserviceconfig.RedpandaServiceConfig{
			Topic: redpandaserviceconfig.TopicConfig{
				DefaultTopicRetentionMs:    1000000,
				DefaultTopicRetentionBytes: 1000000000,
			},
			Resources: redpandaserviceconfig.ResourcesConfig{
				MaxCores:             1,
				MemoryPerCoreInBytes: 2048 * 1024 * 1024,
			},
		}

		// Create the service with mocked dependencies
		redpandaService = redpanda.NewDefaultRedpandaService(
			"test-redpanda",
			archiveStorage,
			redpanda.WithS6Service(mockS6Service),
			redpanda.WithS6Manager(mockedS6Manager),
			redpanda.WithMonitorService(mockMonitorService),
		)
	})

	AfterEach(func() {
		cancel()
	})

	Context("Service lifecycle with state transitions", func() {
		It("should transition from stopped to running and remain stable", func() {
			var serviceInfo redpanda.ServiceInfo
			var err error
			tick := uint64(0)

			By("Adding Redpanda to S6 manager")
			err = redpandaService.AddRedpandaToS6Manager(ctx, redpandaConfig, mockFileSystem)
			Expect(err).NotTo(HaveOccurred())

			By("Reconciling the service to create it")
			tick = reconcileRedpandaUntilState(ctx, redpandaService, mockFileSystem, tick, s6.LifecycleStateCreating)

			By("Reconciling until the service is stopped")
			tick = reconcileRedpandaUntilState(ctx, redpandaService, mockFileSystem, tick, s6fsm.OperationalStateStopped)

			// Verify state
			serviceInfo, err = redpandaService.Status(ctx, mockFileSystem, tick, time.Now())
			Expect(err).NotTo(HaveOccurred())
			Expect(serviceInfo.S6FSMState).To(Equal(s6fsm.OperationalStateStopped))

			By("Starting the redpanda service")
			err = redpandaService.StartRedpanda(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Update mock services to simulate running state
			mockS6Service.StatusResult = s6service.ServiceInfo{
				Status: s6service.ServiceUp,
			}

			// Update monitor service to indicate running state
			metricsState := redpanda_monitor.NewRedpandaMetricsState()
			metricsState.IsActive = true

			monitorMetrics := &redpanda_monitor.RedpandaMetrics{
				Metrics: redpanda_monitor.Metrics{
					Infrastructure: redpanda_monitor.InfrastructureMetrics{
						Storage: redpanda_monitor.StorageMetrics{
							FreeBytes:      10000000000,
							TotalBytes:     20000000000,
							FreeSpaceAlert: false,
						},
					},
					Cluster: redpanda_monitor.ClusterMetrics{
						Topics:            5,
						UnavailableTopics: 0,
					},
					Throughput: redpanda_monitor.ThroughputMetrics{
						BytesIn:  1000,
						BytesOut: 2000,
					},
				},
				MetricsState: metricsState,
			}

			mockMonitorService.StatusResult = redpanda_monitor.ServiceInfo{
				S6FSMState: s6fsm.OperationalStateRunning,
				RedpandaStatus: redpanda_monitor.RedpandaMonitorStatus{
					IsRunning: true,
					LastScan: &redpanda_monitor.RedpandaMetricsAndClusterConfig{
						Metrics: monitorMetrics,
						ClusterConfig: &redpanda_monitor.ClusterConfig{
							Topic: redpanda_monitor.TopicConfig{
								DefaultTopicRetentionMs:    1000000,
								DefaultTopicRetentionBytes: 1000000000,
							},
						},
						LastUpdatedAt: time.Now(),
					},
				},
			}

			By("Reconciling until the service is running")
			tick = reconcileRedpandaUntilState(ctx, redpandaService, mockFileSystem, tick, s6fsm.OperationalStateRunning)

			// Verify the service stays running
			ensureRedpandaState(ctx, redpandaService, mockFileSystem, tick, s6fsm.OperationalStateRunning, 10)
		})

		It("should transition from running to stopped when requested", func() {
			var serviceInfo redpanda.ServiceInfo
			var err error
			tick := uint64(0)

			By("Adding and starting Redpanda")
			err = redpandaService.AddRedpandaToS6Manager(ctx, redpandaConfig, mockFileSystem)
			Expect(err).NotTo(HaveOccurred())

			tick = reconcileRedpandaUntilState(ctx, redpandaService, mockFileSystem, tick, s6.LifecycleStateCreating)
			tick = reconcileRedpandaUntilState(ctx, redpandaService, mockFileSystem, tick, s6fsm.OperationalStateStopped)

			err = redpandaService.StartRedpanda(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Update mock services to simulate running state
			mockS6Service.StatusResult = s6service.ServiceInfo{
				Status: s6service.ServiceUp,
			}

			// Update monitor service to indicate running state
			metricsState := redpanda_monitor.NewRedpandaMetricsState()
			metricsState.IsActive = true

			monitorMetrics := &redpanda_monitor.RedpandaMetrics{
				Metrics: redpanda_monitor.Metrics{
					Infrastructure: redpanda_monitor.InfrastructureMetrics{
						Storage: redpanda_monitor.StorageMetrics{
							FreeBytes:      10000000000,
							TotalBytes:     20000000000,
							FreeSpaceAlert: false,
						},
					},
					Cluster: redpanda_monitor.ClusterMetrics{
						Topics:            5,
						UnavailableTopics: 0,
					},
					Throughput: redpanda_monitor.ThroughputMetrics{
						BytesIn:  1000,
						BytesOut: 2000,
					},
				},
				MetricsState: metricsState,
			}

			mockMonitorService.StatusResult = redpanda_monitor.ServiceInfo{
				S6FSMState: s6fsm.OperationalStateRunning,
				RedpandaStatus: redpanda_monitor.RedpandaMonitorStatus{
					IsRunning: true,
					LastScan: &redpanda_monitor.RedpandaMetricsAndClusterConfig{
						Metrics: monitorMetrics,
						ClusterConfig: &redpanda_monitor.ClusterConfig{
							Topic: redpanda_monitor.TopicConfig{
								DefaultTopicRetentionMs:    1000000,
								DefaultTopicRetentionBytes: 1000000000,
							},
						},
						LastUpdatedAt: time.Now(),
					},
				},
			}

			tick = reconcileRedpandaUntilState(ctx, redpandaService, mockFileSystem, tick, s6fsm.OperationalStateRunning)

			By("Stopping the redpanda service")
			err = redpandaService.StopRedpanda(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Update mock services to simulate stopped state
			mockS6Service.StatusResult = s6service.ServiceInfo{
				Status: s6service.ServiceDown,
			}

			// Update monitor service to indicate stopped state
			metricsState = redpanda_monitor.NewRedpandaMetricsState()
			metricsState.IsActive = false

			mockMonitorService.StatusResult = redpanda_monitor.ServiceInfo{
				S6FSMState: s6fsm.OperationalStateStopped,
				RedpandaStatus: redpanda_monitor.RedpandaMonitorStatus{
					IsRunning: false,
					LastScan: &redpanda_monitor.RedpandaMetricsAndClusterConfig{
						Metrics: &redpanda_monitor.RedpandaMetrics{
							Metrics: redpanda_monitor.Metrics{
								Infrastructure: redpanda_monitor.InfrastructureMetrics{
									Storage: redpanda_monitor.StorageMetrics{
										FreeBytes:      10000000000,
										TotalBytes:     20000000000,
										FreeSpaceAlert: false,
									},
								},
							},
							MetricsState: metricsState,
						},
						ClusterConfig: &redpanda_monitor.ClusterConfig{
							Topic: redpanda_monitor.TopicConfig{
								DefaultTopicRetentionMs:    1000000,
								DefaultTopicRetentionBytes: 1000000000,
							},
						},
						LastUpdatedAt: time.Now(),
					},
				},
			}

			By("Reconciling until the service is stopped")
			tick = reconcileRedpandaUntilState(ctx, redpandaService, mockFileSystem, tick, s6fsm.OperationalStateStopped)

			// Verify service is stopped
			serviceInfo, err = redpandaService.Status(ctx, mockFileSystem, tick, time.Now())
			Expect(err).NotTo(HaveOccurred())
			Expect(serviceInfo.S6FSMState).To(Equal(s6fsm.OperationalStateStopped))
		})

		It("should handle configuration updates", func() {
			var err error
			tick := uint64(0)

			By("Adding Redpanda to S6 manager with initial config")
			err = redpandaService.AddRedpandaToS6Manager(ctx, redpandaConfig, mockFileSystem)
			Expect(err).NotTo(HaveOccurred())

			tick = reconcileRedpandaUntilState(ctx, redpandaService, mockFileSystem, tick, s6fsm.OperationalStateStopped)

			By("Updating the Redpanda configuration")
			updatedConfig := &redpandaserviceconfig.RedpandaServiceConfig{
				Topic: redpandaserviceconfig.TopicConfig{
					DefaultTopicRetentionMs:    2000000,    // Changed value
					DefaultTopicRetentionBytes: 2000000000, // Changed value
				},
				Resources: redpandaserviceconfig.ResourcesConfig{
					MaxCores:             2,                  // Changed value
					MemoryPerCoreInBytes: 4096 * 1024 * 1024, // Changed value
				},
			}

			err = redpandaService.UpdateRedpandaInS6Manager(ctx, updatedConfig)
			Expect(err).NotTo(HaveOccurred())

			By("Reconciling with updated configuration")
			err, reconciled := redpandaService.ReconcileManager(ctx, mockFileSystem, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())
			tick++

			// Set up mock so GetConfig returns the updated config
			mockMonitorService.StatusResult = redpanda_monitor.ServiceInfo{
				S6FSMState: s6fsm.OperationalStateStopped,
				RedpandaStatus: redpanda_monitor.RedpandaMonitorStatus{
					IsRunning: false,
					LastScan: &redpanda_monitor.RedpandaMetricsAndClusterConfig{
						Metrics: &redpanda_monitor.RedpandaMetrics{
							Metrics: redpanda_monitor.Metrics{
								Infrastructure: redpanda_monitor.InfrastructureMetrics{
									Storage: redpanda_monitor.StorageMetrics{
										FreeBytes:      10000000000,
										TotalBytes:     20000000000,
										FreeSpaceAlert: false,
									},
								},
							},
							MetricsState: redpanda_monitor.NewRedpandaMetricsState(),
						},
						ClusterConfig: &redpanda_monitor.ClusterConfig{
							Topic: redpanda_monitor.TopicConfig{
								DefaultTopicRetentionMs:    2000000,
								DefaultTopicRetentionBytes: 2000000000,
							},
						},
						LastUpdatedAt: time.Now(),
					},
				},
			}

			// Check updated config was applied
			config, err := redpandaService.GetConfig(ctx, mockFileSystem, tick, time.Now())
			Expect(err).NotTo(HaveOccurred())
			Expect(config.Topic.DefaultTopicRetentionMs).To(Equal(int64(2000000)))
		})

		It("should handle service removal gracefully", func() {
			var err error
			tick := uint64(0)

			By("Setting up a running Redpanda service")
			err = redpandaService.AddRedpandaToS6Manager(ctx, redpandaConfig, mockFileSystem)
			Expect(err).NotTo(HaveOccurred())

			tick = reconcileRedpandaUntilState(ctx, redpandaService, mockFileSystem, tick, s6fsm.OperationalStateStopped)

			err = redpandaService.StartRedpanda(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Update mock services to simulate running state
			mockS6Service.StatusResult = s6service.ServiceInfo{
				Status: s6service.ServiceUp,
			}

			// Update monitor service to indicate running state
			metricsState := redpanda_monitor.NewRedpandaMetricsState()
			metricsState.IsActive = true

			mockMonitorService.StatusResult = redpanda_monitor.ServiceInfo{
				S6FSMState: s6fsm.OperationalStateRunning,
				RedpandaStatus: redpanda_monitor.RedpandaMonitorStatus{
					IsRunning: true,
					LastScan: &redpanda_monitor.RedpandaMetricsAndClusterConfig{
						Metrics: &redpanda_monitor.RedpandaMetrics{
							Metrics: redpanda_monitor.Metrics{
								Infrastructure: redpanda_monitor.InfrastructureMetrics{
									Storage: redpanda_monitor.StorageMetrics{
										FreeBytes:      10000000000,
										TotalBytes:     20000000000,
										FreeSpaceAlert: false,
									},
								},
							},
							MetricsState: metricsState,
						},
						ClusterConfig: &redpanda_monitor.ClusterConfig{
							Topic: redpanda_monitor.TopicConfig{
								DefaultTopicRetentionMs:    1000000,
								DefaultTopicRetentionBytes: 1000000000,
							},
						},
						LastUpdatedAt: time.Now(),
					},
				},
			}

			tick = reconcileRedpandaUntilState(ctx, redpandaService, mockFileSystem, tick, s6fsm.OperationalStateRunning)

			By("Removing the Redpanda service")
			err = redpandaService.RemoveRedpandaFromS6Manager(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Update mocks for removal
			mockS6Service.ServiceExistsResult = false
			mockMonitorService.ServiceExistsResult = false
			mockMonitorService.StatusError = redpanda_monitor.ErrServiceNotExist

			// Disable error checks for the reconcile call after removal
			// since the service is supposed to be gone
			By("Verifying the service no longer exists")
			exists := redpandaService.ServiceExists(ctx, mockFileSystem)
			Expect(exists).To(BeFalse())

			// Let's just verify with direct checks rather than using reconcile
			// since reconcile is meant for services that still exist
			By("Ensuring removal is clean")
			err = redpandaService.StartRedpanda(ctx)
			Expect(err).To(Equal(redpanda.ErrServiceNotExist))

			err = redpandaService.StopRedpanda(ctx)
			Expect(err).To(Equal(redpanda.ErrServiceNotExist))
		})
	})
})

// Helper function to create mock logs for redpanda
func createRedpandaMockLogs() []s6service.LogEntry {
	// Create simple log entries for the redpanda service
	timestamp := time.Now()
	return []s6service.LogEntry{
		{Content: "Starting redpanda...", Timestamp: timestamp.Add(-5 * time.Second)},
		{Content: "Loading configuration...", Timestamp: timestamp.Add(-4 * time.Second)},
		{Content: "Started redpanda successfully", Timestamp: timestamp.Add(-3 * time.Second)},
	}
}

// Reconcile until a specific state is reached
func reconcileRedpandaUntilState(ctx context.Context, redpandaService *redpanda.RedpandaService, mockFileSystem *filesystem.MockFileSystem, tick uint64, expectedState string) uint64 {
	for i := 0; i < 10; i++ {
		err, _ := redpandaService.ReconcileManager(ctx, mockFileSystem, tick)
		Expect(err).NotTo(HaveOccurred())
		tick++

		// Check state
		serviceInfo, err := redpandaService.Status(ctx, mockFileSystem, tick, time.Now())
		if err == nil && serviceInfo.S6FSMState == expectedState {
			return tick
		}
	}

	Fail(fmt.Sprintf("Expected state %s not reached after 10 reconciliations", expectedState))
	return 0
}

// Ensure the service remains in a specific state for multiple reconciliations
func ensureRedpandaState(ctx context.Context, redpandaService *redpanda.RedpandaService, mockFileSystem *filesystem.MockFileSystem, tick uint64, expectedState string, iterations int) {
	for i := 0; i < iterations; i++ {
		err, _ := redpandaService.ReconcileManager(ctx, mockFileSystem, tick)
		Expect(err).NotTo(HaveOccurred())
		tick++

		// Check state
		serviceInfo, err := redpandaService.Status(ctx, mockFileSystem, tick, time.Now())
		Expect(err).NotTo(HaveOccurred())
		Expect(serviceInfo.S6FSMState).To(Equal(expectedState))
	}
}
