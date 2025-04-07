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

package redpanda

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/redpandaserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// getTmpDir returns the temporary directory for a container
func getTmpDir() string {
	tmpDir := "/tmp"
	// If we are in a devcontainer, use the workspace as tmp dir
	if os.Getenv("REMOTE_CONTAINERS") != "" || os.Getenv("CODESPACE_NAME") != "" || os.Getenv("USER") == "vscode" {
		tmpDir = "/workspaces/united-manufacturing-hub/umh-core/tmp"
	}
	return tmpDir
}

var _ = Describe("Redpanda Service", func() {
	var (
		service *RedpandaService
		client  *MockHTTPClient
		tick    uint64
	)

	BeforeEach(func() {
		client = NewMockHTTPClient()
		service = NewDefaultRedpandaService("redpanda", WithHTTPClient(client))
		tick = 0

		// Cleanup the data directory
		service.filesystem.RemoveAll(context.Background(), getTmpDir())

		// Add the service to the S6 manager
		config := &redpandaserviceconfig.RedpandaServiceConfig{
			BaseDir: getTmpDir(),
		}
		config.Topic.DefaultTopicRetentionMs = 1000000
		config.Topic.DefaultTopicRetentionBytes = 1000000000
		err := service.AddRedpandaToS6Manager(context.Background(), config)
		Expect(err).NotTo(HaveOccurred())

		// Reconcile the S6 manager
		err, _ = service.ReconcileManager(context.Background(), tick)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("GetHealthCheckAndMetrics", func() {
		var mockS6Service *s6service.MockService

		Context("with valid metrics endpoint", func() {
			BeforeEach(func() {
				// Create a fresh client to avoid interference from defaults
				client = NewMockHTTPClient()
				service.httpClient = client

				// Configure mock client with healthy metrics
				client.SetMetricsResponse(MetricsConfig{
					Infrastructure: InfrastructureMetricsConfig{
						Storage: StorageMetricsConfig{
							FreeBytes:      5000000000,
							TotalBytes:     10000000000,
							FreeSpaceAlert: false, // Explicitly set to false
						},
						Uptime: UptimeMetricsConfig{
							Uptime: 3600, // 1 hour in seconds
						},
					},
					Cluster: ClusterMetricsConfig{
						Topics:            5,
						UnavailableTopics: 0,
					},
					Throughput: ThroughputMetricsConfig{
						BytesIn:  1024,
						BytesOut: 2048,
					},
					Topic: TopicMetricsConfig{
						TopicPartitionMap: map[string]int64{
							"test-topic": 3,
						},
					},
				})

				// Setup mock S6 service to return logs with successful startup message
				mockS6Service = s6service.NewMockService()
				mockS6Service.GetLogsResult = []s6service.LogEntry{
					{
						Timestamp: time.Now().Add(-1 * time.Minute),
						Content:   "INFO Starting Redpanda",
					},
					{
						Timestamp: time.Now().Add(-30 * time.Second),
						Content:   "Successfully started Redpanda!",
					},
				}
				service.s6Service = mockS6Service
			})

			It("should return health check and metrics", func() {
				ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
				defer cancel()
				status, err := service.GetHealthCheckAndMetrics(ctx, tick, mockS6Service.GetLogsResult)
				tick += 10
				Expect(err).NotTo(HaveOccurred())
				Expect(status.HealthCheck.IsLive).To(BeTrue())
				Expect(status.HealthCheck.IsReady).To(BeTrue())
				Expect(status.Metrics.Infrastructure.Storage.FreeBytes).To(Equal(int64(5000000000)))
				Expect(status.Metrics.Infrastructure.Storage.TotalBytes).To(Equal(int64(10000000000)))
				Expect(status.Metrics.Infrastructure.Storage.FreeSpaceAlert).To(BeFalse())
				Expect(status.Metrics.Infrastructure.Uptime.Uptime).To(Equal(int64(3600)))
				Expect(status.Metrics.Cluster.Topics).To(Equal(int64(5)))
				Expect(status.Metrics.Cluster.UnavailableTopics).To(Equal(int64(0)))
				Expect(status.Metrics.Throughput.BytesIn).To(Equal(int64(1024)))
				Expect(status.Metrics.Throughput.BytesOut).To(Equal(int64(2048)))
				Expect(status.Metrics.Topic.TopicPartitionMap).To(HaveLen(1))
				Expect(status.Metrics.Topic.TopicPartitionMap["test-topic"]).To(Equal(int64(3)))
			})
		})

		Context("with connection issues", func() {
			BeforeEach(func() {
				client.SetResponse("/public_metrics", MockResponse{
					StatusCode: 500,
					Body:       []byte("connection refused"),
				})

				// Setup mock S6 service to return logs with successful startup message
				mockS6Service = s6service.NewMockService()
				mockS6Service.GetLogsResult = []s6service.LogEntry{
					{
						Timestamp: time.Now().Add(-1 * time.Minute),
						Content:   "INFO Starting Redpanda",
					},
					{
						Timestamp: time.Now().Add(-30 * time.Second),
						Content:   "Successfully started Redpanda!",
					},
				}
				service.s6Service = mockS6Service
			})

			It("should return error", func() {
				ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
				defer cancel()
				status, err := service.GetHealthCheckAndMetrics(ctx, tick, mockS6Service.GetLogsResult)
				tick += 10
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("connection refused"))
				Expect(status.HealthCheck.IsReady).To(BeFalse())
			})
		})

		Context("with context cancellation", func() {
			BeforeEach(func() {
				client.SetResponse("/public_metrics", MockResponse{
					StatusCode: 200,
					Delay:      100 * time.Millisecond,
				})

				// Setup mock S6 service to return logs with successful startup message
				mockS6Service = s6service.NewMockService()
				mockS6Service.GetLogsResult = []s6service.LogEntry{
					{
						Timestamp: time.Now().Add(-1 * time.Minute),
						Content:   "INFO Starting Redpanda",
					},
					{
						Timestamp: time.Now().Add(-30 * time.Second),
						Content:   "Successfully started Redpanda!",
					},
				}
				service.s6Service = mockS6Service
			})

			It("should return error when context is cancelled", func() {
				ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
				defer cancel()

				status, err := service.GetHealthCheckAndMetrics(ctx, tick, mockS6Service.GetLogsResult)
				tick += 10
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("context deadline exceeded"))
				Expect(status.HealthCheck.IsReady).To(BeFalse())
			})
		})

		Context("without successful startup message in logs", func() {
			BeforeEach(func() {
				// Create a fresh client to avoid interference from defaults
				client = NewMockHTTPClient()
				service.httpClient = client

				// Configure mock client with healthy metrics
				client.SetMetricsResponse(MetricsConfig{
					Infrastructure: InfrastructureMetricsConfig{
						Storage: StorageMetricsConfig{
							FreeBytes:      5000000000,
							TotalBytes:     10000000000,
							FreeSpaceAlert: false,
						},
						Uptime: UptimeMetricsConfig{
							Uptime: 3600,
						},
					},
					Cluster: ClusterMetricsConfig{
						Topics:            5,
						UnavailableTopics: 0,
					},
					Throughput: ThroughputMetricsConfig{
						BytesIn:  1024,
						BytesOut: 2048,
					},
					Topic: TopicMetricsConfig{
						TopicPartitionMap: map[string]int64{
							"test-topic": 3,
						},
					},
				})

				// Setup mock S6 service to return logs WITHOUT successful startup message
				mockS6Service = s6service.NewMockService()
				mockS6Service.GetLogsResult = []s6service.LogEntry{
					{
						Timestamp: time.Now().Add(-1 * time.Minute),
						Content:   "INFO Starting Redpanda",
					},
					{
						Timestamp: time.Now().Add(-30 * time.Second),
						Content:   "INFO Initializing resources",
					},
				}
				service.s6Service = mockS6Service
			})

			It("should report ready with healthy metrics", func() {
				ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
				defer cancel()
				status, err := service.GetHealthCheckAndMetrics(ctx, tick, mockS6Service.GetLogsResult)
				tick += 10
				Expect(err).NotTo(HaveOccurred())
				Expect(status.HealthCheck.IsLive).To(BeTrue())
				Expect(status.HealthCheck.IsReady).To(BeTrue())
			})
		})
	})

	Describe("GenerateS6ConfigForRedpanda", func() {
		Context("with valid configuration", func() {
			It("should generate valid YAML", func() {
				cfg := &redpandaserviceconfig.RedpandaServiceConfig{
					BaseDir: getTmpDir(),
				}
				cfg.Topic.DefaultTopicRetentionMs = 1000000
				cfg.Topic.DefaultTopicRetentionBytes = 1000000000

				s6Config, err := service.GenerateS6ConfigForRedpanda(cfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(s6Config.ConfigFiles).To(HaveKey("redpanda.yaml"))
				yaml := s6Config.ConfigFiles["redpanda.yaml"]

				Expect(yaml).To(ContainSubstring("log_retention_ms: 1000000"))
				Expect(yaml).To(ContainSubstring("retention_bytes: 1000000000"))
			})
		})

		Context("with nil configuration", func() {
			It("should return error", func() {
				s6Config, err := service.GenerateS6ConfigForRedpanda(nil)
				Expect(err).To(HaveOccurred())
				Expect(s6Config).To(Equal(s6serviceconfig.S6ServiceConfig{}))
			})
		})
	})

	Describe("Lifecycle Management", func() {
		var (
			service       *RedpandaService
			mockClient    *MockHTTPClient
			mockS6Service *s6service.MockService
		)

		BeforeEach(func() {
			mockClient = NewMockHTTPClient()
			mockS6Service = s6service.NewMockService()
			service = NewDefaultRedpandaService("redpanda",
				WithHTTPClient(mockClient),
				WithS6Service(mockS6Service),
			)
		})

		It("should add, start, stop and remove a Redpanda service", func() {
			ctx := context.Background()

			// Initial config
			config := &redpandaserviceconfig.RedpandaServiceConfig{
				BaseDir: getTmpDir(),
			}
			config.Topic.DefaultTopicRetentionMs = 1000000
			config.Topic.DefaultTopicRetentionBytes = 1000000000

			// Add the service
			By("Adding the Redpanda service")
			err := service.AddRedpandaToS6Manager(ctx, config)
			Expect(err).NotTo(HaveOccurred())

			// Service should exist in the S6 manager
			Expect(len(service.s6ServiceConfigs)).To(Equal(1))

			// Start the service
			By("Starting the Redpanda service")
			err = service.StartRedpanda(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Reconcile to apply changes
			By("Reconciling the manager")
			err, _ = service.ReconcileManager(ctx, 0)
			Expect(err).NotTo(HaveOccurred())

			// Stop the service
			By("Stopping the Redpanda service")
			err = service.StopRedpanda(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Reconcile to apply changes
			By("Reconciling the manager again")
			err, _ = service.ReconcileManager(ctx, 1)
			Expect(err).NotTo(HaveOccurred())

			// Remove the service
			By("Removing the Redpanda service")
			err = service.RemoveRedpandaFromS6Manager(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Service should no longer exist in the S6 manager
			Expect(len(service.s6ServiceConfigs)).To(Equal(0))
		})

		It("should handle configuration updates", func() {
			ctx := context.Background()

			// Use MockRedpandaService for this test to verify method calls
			mockRedpandaService := NewMockRedpandaService()

			// Initial config
			initialConfig := &redpandaserviceconfig.RedpandaServiceConfig{
				BaseDir: getTmpDir(),
			}
			initialConfig.Topic.DefaultTopicRetentionMs = 1000000
			initialConfig.Topic.DefaultTopicRetentionBytes = 1000000000

			// Add the service with initial config
			By("Adding the Redpanda service with initial config")
			err := mockRedpandaService.AddRedpandaToS6Manager(ctx, initialConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockRedpandaService.AddRedpandaToS6ManagerCalled).To(BeTrue())

			// Updated config with different retention
			updatedConfig := &redpandaserviceconfig.RedpandaServiceConfig{
				BaseDir: getTmpDir(),
			}
			updatedConfig.Topic.DefaultTopicRetentionMs = 2000000
			updatedConfig.Topic.DefaultTopicRetentionBytes = 2000000000

			// Update the service configuration
			By("Updating the Redpanda service configuration")
			err = mockRedpandaService.UpdateRedpandaInS6Manager(ctx, updatedConfig)
			Expect(err).NotTo(HaveOccurred())

			// Reconcile to apply changes
			By("Reconciling the manager to apply configuration changes")
			err, _ = mockRedpandaService.ReconcileManager(ctx, 0)
			Expect(err).NotTo(HaveOccurred())

			// Verify the configuration was updated in the S6 manager
			By("Verifying the UpdateRedpandaInS6Manager was called")
			Expect(mockRedpandaService.UpdateRedpandaInS6ManagerCalled).To(BeTrue())
		})

		It("should handle non-existent service errors", func() {
			ctx := context.Background()

			// Try to update a non-existent service
			By("Trying to update a non-existent service")
			err := service.UpdateRedpandaInS6Manager(ctx, &redpandaserviceconfig.RedpandaServiceConfig{
				BaseDir: getTmpDir(),
			})
			Expect(err).To(Equal(ErrServiceNotExist))

			// Try to stop a non-existent service
			By("Trying to stop a non-existent service")
			err = service.StopRedpanda(ctx)
			Expect(err).To(Equal(ErrServiceNotExist))

			// Try to remove a non-existent service
			By("Trying to remove a non-existent service")
			err = service.RemoveRedpandaFromS6Manager(ctx)
			Expect(err).To(Equal(ErrServiceNotExist))
		})

		It("should handle already existing service errors", func() {
			ctx := context.Background()

			// Add a service
			By("Adding a service")
			err := service.AddRedpandaToS6Manager(ctx, &redpandaserviceconfig.RedpandaServiceConfig{
				BaseDir: getTmpDir(),
			})
			Expect(err).NotTo(HaveOccurred())

			// Try to add the same service again
			By("Trying to add the same service again")
			err = service.AddRedpandaToS6Manager(ctx, &redpandaserviceconfig.RedpandaServiceConfig{
				BaseDir: getTmpDir(),
			})
			Expect(err).To(Equal(ErrServiceAlreadyExists))
		})
	})

	Context("Metrics Analysis", func() {
		var service *RedpandaService

		BeforeEach(func() {
			service = NewDefaultRedpandaService("redpanda")
		})

		Context("IsMetricsErrorFree", func() {
			It("should return true when there are no errors", func() {
				metrics := Metrics{
					Infrastructure: InfrastructureMetrics{
						Storage: StorageMetrics{
							FreeBytes:      5000000000,
							TotalBytes:     10000000000,
							FreeSpaceAlert: false,
						},
					},
					Cluster: ClusterMetrics{
						Topics:            5,
						UnavailableTopics: 0, // No unavailable topics
					},
				}
				Expect(service.IsMetricsErrorFree(metrics)).To(BeTrue())
			})

			It("should detect storage free space alerts", func() {
				metrics := Metrics{
					Infrastructure: InfrastructureMetrics{
						Storage: StorageMetrics{
							FreeBytes:      100000,
							TotalBytes:     10000000000,
							FreeSpaceAlert: true, // Alert triggered
						},
					},
				}
				Expect(service.IsMetricsErrorFree(metrics)).To(BeFalse())
			})

			It("should detect unavailable topics", func() {
				metrics := Metrics{
					Infrastructure: InfrastructureMetrics{
						Storage: StorageMetrics{
							FreeBytes:      5000000000,
							TotalBytes:     10000000000,
							FreeSpaceAlert: false, // No storage alert
						},
					},
					Cluster: ClusterMetrics{
						Topics:            10,
						UnavailableTopics: 2, // Some topics are unavailable
					},
				}
				Expect(service.IsMetricsErrorFree(metrics)).To(BeFalse())
			})

			It("should pass when no storage alerts and all topics available", func() {
				metrics := Metrics{
					Infrastructure: InfrastructureMetrics{
						Storage: StorageMetrics{
							FreeBytes:      5000000000,
							TotalBytes:     10000000000,
							FreeSpaceAlert: false, // No storage alert
						},
					},
					Cluster: ClusterMetrics{
						Topics:            10,
						UnavailableTopics: 0, // All topics available
					},
				}
				Expect(service.IsMetricsErrorFree(metrics)).To(BeTrue())
			})
		})

		Context("HasProcessingActivity", func() {
			It("should detect processing activity", func() {
				metricsState := NewRedpandaMetricsState()
				metricsState.IsActive = true

				status := RedpandaStatus{
					MetricsState: metricsState,
				}

				Expect(service.HasProcessingActivity(status)).To(BeTrue())
			})

			It("should detect lack of processing activity", func() {
				metricsState := NewRedpandaMetricsState()
				metricsState.IsActive = false

				status := RedpandaStatus{
					MetricsState: metricsState,
				}

				Expect(service.HasProcessingActivity(status)).To(BeFalse())
			})

			It("should handle nil metrics state", func() {
				status := RedpandaStatus{
					MetricsState: nil,
				}

				Expect(service.HasProcessingActivity(status)).To(BeFalse())
			})
		})
	})

	Context("Tick-based throughput tracking", Label("tick_based_throughput_tracking"), func() {
		var (
			service       *RedpandaService
			mockClient    *MockHTTPClient
			mockS6Service *s6service.MockService
			tick          uint64
		)

		BeforeEach(func() {
			mockClient = NewMockHTTPClient()
			mockS6Service = s6service.NewMockService()
			service = NewDefaultRedpandaService("redpanda",
				WithHTTPClient(mockClient),
				WithS6Service(mockS6Service),
			)
			tick = 0

			mockS6Service.ServiceExistsResult = true

			// Add the service to the S6 manager
			err := service.AddRedpandaToS6Manager(context.Background(), &redpandaserviceconfig.RedpandaServiceConfig{
				BaseDir: getTmpDir(),
			})
			Expect(err).NotTo(HaveOccurred())

			// Reconcile the S6 manager
			err, _ = service.ReconcileManager(context.Background(), tick)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should calculate throughput based on ticks", func() {
			// Create a fresh client to avoid default value conflicts
			client := NewMockHTTPClient()
			service.httpClient = client

			// Set up metrics for first tick with some throughput
			client.SetMetricsResponse(MetricsConfig{
				Throughput: ThroughputMetricsConfig{
					BytesIn:  1000,
					BytesOut: 900,
				},
				// Add required metrics to prevent nil errors
				Infrastructure: InfrastructureMetricsConfig{
					Storage: StorageMetricsConfig{
						FreeBytes:      5000000000,
						TotalBytes:     10000000000,
						FreeSpaceAlert: false,
					},
					Uptime: UptimeMetricsConfig{
						Uptime: 3600,
					},
				},
				Cluster: ClusterMetricsConfig{
					Topics:            5,
					UnavailableTopics: 0,
				},
				Topic: TopicMetricsConfig{
					TopicPartitionMap: map[string]int64{
						"test-topic": 3,
					},
				},
			})

			// First status check
			status1, err := service.Status(context.Background(), tick)
			tick += 10
			Expect(err).NotTo(HaveOccurred())

			// Verify initial state
			Expect(status1.RedpandaStatus.Metrics.Throughput.BytesIn).To(Equal(int64(1000)))
			Expect(status1.RedpandaStatus.Metrics.Throughput.BytesOut).To(Equal(int64(900)))

			// Set up metrics for second tick with increased throughput
			client.SetMetricsResponse(MetricsConfig{
				Throughput: ThroughputMetricsConfig{
					BytesIn:  2000, // +1000
					BytesOut: 1800, // +900
				},
			})

			// Second status check
			status2, err := service.Status(context.Background(), tick)
			tick += 10
			Expect(err).NotTo(HaveOccurred())

			// Verify new throughput values
			Expect(status2.RedpandaStatus.Metrics.Throughput.BytesIn).To(Equal(int64(2000)))
			Expect(status2.RedpandaStatus.Metrics.Throughput.BytesOut).To(Equal(int64(1800)))

			// Verify that the MetricsState is tracking activity
			Expect(status2.RedpandaStatus.MetricsState.IsActive).To(BeTrue())
		})

		It("should detect inactivity", func() {
			// Create a fresh client to avoid default value conflicts
			client := NewMockHTTPClient()
			service.httpClient = client

			// First tick with some activity
			client.SetMetricsResponse(MetricsConfig{
				Throughput: ThroughputMetricsConfig{
					BytesIn:  1000,
					BytesOut: 900,
				},
				// Add required metrics to prevent nil errors
				Infrastructure: InfrastructureMetricsConfig{
					Storage: StorageMetricsConfig{
						FreeBytes:      5000000000,
						TotalBytes:     10000000000,
						FreeSpaceAlert: false,
					},
					Uptime: UptimeMetricsConfig{
						Uptime: 3600,
					},
				},
				Cluster: ClusterMetricsConfig{
					Topics:            5,
					UnavailableTopics: 0,
				},
				Topic: TopicMetricsConfig{
					TopicPartitionMap: map[string]int64{
						"test-topic": 3,
					},
				},
			})

			// First status check
			status1, err := service.Status(context.Background(), tick)
			tick += 10
			Expect(err).NotTo(HaveOccurred())
			Expect(status1.RedpandaStatus.MetricsState.IsActive).To(BeTrue())

			// Second tick with no change in throughput
			client.SetMetricsResponse(MetricsConfig{
				Throughput: ThroughputMetricsConfig{
					BytesIn:  1000, // No change
					BytesOut: 900,  // No change
				},
			})

			// Second status check
			status2, err := service.Status(context.Background(), tick)
			tick += 10
			Expect(err).NotTo(HaveOccurred())

			// Should detect inactivity
			Expect(status2.RedpandaStatus.MetricsState.IsActive).To(BeFalse())
		})
	})

	Context("Log Analysis", func() {
		var (
			service     *RedpandaService
			currentTime time.Time
			logWindow   time.Duration
		)

		BeforeEach(func() {
			service = NewDefaultRedpandaService("redpanda")
			currentTime = time.Now()
			logWindow = 5 * time.Minute
		})

		Context("IsLogsFine", func() {
			It("should return true when there are no logs", func() {
				Expect(service.IsLogsFine([]s6service.LogEntry{}, currentTime, logWindow)).To(BeTrue())
			})

			It("should detect 'Address already in use' errors", func() {
				logs := []s6service.LogEntry{
					{
						Timestamp: currentTime.Add(-1 * time.Minute),
						Content:   "INFO Starting Redpanda",
					},
					{
						Timestamp: currentTime.Add(-30 * time.Second),
						Content:   "ERROR Address already in use (port 9092)",
					},
				}
				Expect(service.IsLogsFine(logs, currentTime, logWindow)).To(BeFalse())
			})

			It("should detect 'Reactor stalled for' errors", func() {
				logs := []s6service.LogEntry{
					{
						Timestamp: currentTime.Add(-1 * time.Minute),
						Content:   "INFO Starting Redpanda",
					},
					{
						Timestamp: currentTime.Add(-30 * time.Second),
						Content:   "WARN Reactor stalled for 2s",
					},
				}
				Expect(service.IsLogsFine(logs, currentTime, logWindow)).To(BeFalse())
			})

			It("should pass with normal logs", func() {
				logs := []s6service.LogEntry{
					{
						Timestamp: currentTime.Add(-1 * time.Minute),
						Content:   "INFO Starting Redpanda",
					},
					{
						Timestamp: currentTime.Add(-30 * time.Second),
						Content:   "INFO Created topic 'test-topic' with 3 partitions",
					},
				}
				Expect(service.IsLogsFine(logs, currentTime, logWindow)).To(BeTrue())
			})

			It("should ignore logs outside the time window", func() {
				logs := []s6service.LogEntry{
					{
						Timestamp: currentTime.Add(-10 * time.Minute), // Outside our 5-minute window
						Content:   "ERROR Address already in use (port 9092)",
					},
					{
						Timestamp: currentTime.Add(-30 * time.Second), // Inside window
						Content:   "INFO Created topic 'test-topic' with 3 partitions",
					},
				}
				Expect(service.IsLogsFine(logs, currentTime, logWindow)).To(BeTrue())
			})

			It("should handle logs at the edge of the time window", func() {
				// Create test logs with error exactly at the window boundary
				errorLog := s6service.LogEntry{
					Timestamp: currentTime.Add(-logWindow), // Exactly at boundary (should be excluded)
					Content:   "ERROR Address already in use (port 9092)",
				}

				// Debug info
				windowStart := currentTime.Add(-logWindow)
				fmt.Printf("Debug - Window start: %v\n", windowStart)
				fmt.Printf("Debug - Error log time: %v\n", errorLog.Timestamp)
				fmt.Printf("Debug - Is error log before window start? %v\n", errorLog.Timestamp.Before(windowStart))
				fmt.Printf("Debug - Is error log equal to window start? %v\n", errorLog.Timestamp.Equal(windowStart))

				logs := []s6service.LogEntry{errorLog}

				// Should pass because error log is exactly at boundary and will be excluded
				result := service.IsLogsFine(logs, currentTime, logWindow)
				fmt.Printf("Debug - IsLogsFine result: %v\n", result)
				Expect(result).To(BeFalse(), "Error log exactly at window boundary should be excluded")

				// Add a normal log just inside the window
				normalLog := s6service.LogEntry{
					Timestamp: currentTime.Add(-logWindow).Add(1 * time.Millisecond), // Just inside window
					Content:   "INFO Normal log inside window",
				}
				logs = append(logs, normalLog)
				fmt.Printf("Debug - Normal log time: %v\n", normalLog.Timestamp)
				fmt.Printf("Debug - Is normal log before window start? %v\n", normalLog.Timestamp.Before(windowStart))

				// Should still pass
				result = service.IsLogsFine(logs, currentTime, logWindow)
				fmt.Printf("Debug - IsLogsFine result with normal log: %v\n", result)
				Expect(result).To(BeFalse(), "Adding a normal log inside the window should not affect the result")

				// Now add an error log just inside the window
				errorLogInside := s6service.LogEntry{
					Timestamp: currentTime.Add(-logWindow).Add(2 * time.Millisecond), // Just inside window
					Content:   "ERROR Reactor stalled for 5s",
				}
				logs = append(logs, errorLogInside)
				fmt.Printf("Debug - Error log inside time: %v\n", errorLogInside.Timestamp)
				fmt.Printf("Debug - Is error log inside before window start? %v\n", errorLogInside.Timestamp.Before(windowStart))

				// Should fail because there's now an error log inside the window
				result = service.IsLogsFine(logs, currentTime, logWindow)
				fmt.Printf("Debug - IsLogsFine result with error log inside: %v\n", result)
				Expect(result).To(BeFalse(), "Error log inside window should cause the check to fail")
			})

			It("should evaluate logs with mixed timestamps correctly", func() {
				// Mix of:
				// - Old errors (outside window)
				// - Recent errors (inside window)
				// - Old normal logs
				// - Recent normal logs
				logs := []s6service.LogEntry{
					// Outside window, error - should be ignored
					{
						Timestamp: currentTime.Add(-10 * time.Minute),
						Content:   "ERROR Address already in use (port 9092)",
					},
					// Outside window, normal - should be ignored
					{
						Timestamp: currentTime.Add(-7 * time.Minute),
						Content:   "INFO Starting Redpanda",
					},
					// Inside window, normal - should not trigger failure
					{
						Timestamp: currentTime.Add(-4 * time.Minute),
						Content:   "INFO Created topic 'test-topic'",
					},
					// Inside window with error - should trigger failure
					{
						Timestamp: currentTime.Add(-3 * time.Minute),
						Content:   "ERROR Reactor stalled for 10s",
					},
					// Latest log, normal - should not affect result
					{
						Timestamp: currentTime.Add(-1 * time.Minute),
						Content:   "INFO Successfully processed batch",
					},
				}

				// Should fail because there's an error within the time window
				Expect(service.IsLogsFine(logs, currentTime, logWindow)).To(BeFalse())

				// Now move the error outside the window by adjusting the current time
				adjustedTime := currentTime.Add(3 * time.Minute)
				Expect(service.IsLogsFine(logs, adjustedTime, logWindow)).To(BeTrue())
			})

			It("should respect different window sizes", func() {
				logs := []s6service.LogEntry{
					{
						Timestamp: currentTime.Add(-3 * time.Minute),
						Content:   "ERROR Address already in use (port 9092)",
					},
				}

				// With 5 minute window (default), error is detected
				Expect(service.IsLogsFine(logs, currentTime, 5*time.Minute)).To(BeFalse())

				// With 2 minute window, error is outside window and ignored
				Expect(service.IsLogsFine(logs, currentTime, 2*time.Minute)).To(BeTrue())

				// With 10 minute window, error is detected
				Expect(service.IsLogsFine(logs, currentTime, 10*time.Minute)).To(BeFalse())
			})
		})
	})
})
