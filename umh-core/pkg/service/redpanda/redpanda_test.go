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
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/redpandaserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

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

		// Add the service to the S6 manager
		err := service.AddRedpandaToS6Manager(context.Background(), &redpandaserviceconfig.RedpandaServiceConfig{
			RetentionMs:    1000000,
			RetentionBytes: 1000000000,
		})
		Expect(err).NotTo(HaveOccurred())

		// Reconcile the S6 manager
		err, _ = service.ReconcileManager(context.Background(), tick)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("GetHealthCheckAndMetrics", func() {
		Context("with valid metrics endpoint", func() {
			BeforeEach(func() {
				// Configure mock client with healthy metrics
				client.SetMetricsResponse(MetricsConfig{
					Infrastructure: InfrastructureMetricsConfig{
						Storage: StorageMetricsConfig{
							FreeBytes:      5000000000,
							TotalBytes:     10000000000,
							FreeSpaceAlert: false,
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
			})

			It("should return health check and metrics", func() {
				ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
				defer cancel()
				status, err := service.GetHealthCheckAndMetrics(ctx, tick)
				tick++
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
			})

			It("should return error", func() {
				ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
				defer cancel()
				_, err := service.GetHealthCheckAndMetrics(ctx, tick)
				tick++
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("connection refused"))
			})
		})

		Context("with context cancellation", func() {
			BeforeEach(func() {
				client.SetResponse("/public_metrics", MockResponse{
					StatusCode: 200,
					Delay:      100 * time.Millisecond,
				})
			})

			It("should return error when context is cancelled", func() {
				ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
				defer cancel()

				_, err := service.GetHealthCheckAndMetrics(ctx, tick)
				tick++
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("context deadline exceeded"))
			})
		})
	})

	Describe("GenerateS6ConfigForRedpanda", func() {
		Context("with valid configuration", func() {
			It("should generate valid YAML", func() {
				cfg := &redpandaserviceconfig.RedpandaServiceConfig{
					RetentionMs:    1000000,
					RetentionBytes: 1000000000,
				}

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
				RetentionMs:    1000000,
				RetentionBytes: 1000000000,
			}

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
				RetentionMs:    1000000,
				RetentionBytes: 1000000000,
			}

			// Add the service with initial config
			By("Adding the Redpanda service with initial config")
			err := mockRedpandaService.AddRedpandaToS6Manager(ctx, initialConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockRedpandaService.AddRedpandaToS6ManagerCalled).To(BeTrue())

			// Updated config with different retention
			updatedConfig := &redpandaserviceconfig.RedpandaServiceConfig{
				RetentionMs:    2000000,    // Doubled
				RetentionBytes: 2000000000, // Doubled
			}

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
			err := service.UpdateRedpandaInS6Manager(ctx, &redpandaserviceconfig.RedpandaServiceConfig{})
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
			err := service.AddRedpandaToS6Manager(ctx, &redpandaserviceconfig.RedpandaServiceConfig{})
			Expect(err).NotTo(HaveOccurred())

			// Try to add the same service again
			By("Trying to add the same service again")
			err = service.AddRedpandaToS6Manager(ctx, &redpandaserviceconfig.RedpandaServiceConfig{})
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
						UnavailableTopics: 0,
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
				RetentionMs:    1000000,
				RetentionBytes: 1000000000,
			})
			Expect(err).NotTo(HaveOccurred())

			// Reconcile the S6 manager
			err, _ = service.ReconcileManager(context.Background(), tick)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should calculate throughput based on ticks", func() {
			// Set up metrics for first tick with some throughput
			client.SetMetricsResponse(MetricsConfig{
				Throughput: ThroughputMetricsConfig{
					BytesIn:  1000,
					BytesOut: 900,
				},
			})

			// First status check
			status1, err := service.Status(context.Background(), tick)
			tick++
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
			tick++
			Expect(err).NotTo(HaveOccurred())

			// Verify new throughput values
			Expect(status2.RedpandaStatus.Metrics.Throughput.BytesIn).To(Equal(int64(2000)))
			Expect(status2.RedpandaStatus.Metrics.Throughput.BytesOut).To(Equal(int64(1800)))

			// Verify that the MetricsState is tracking activity
			Expect(status2.RedpandaStatus.MetricsState.IsActive).To(BeTrue())
		})

		It("should detect inactivity", func() {
			// First tick with some activity
			client.SetMetricsResponse(MetricsConfig{
				Throughput: ThroughputMetricsConfig{
					BytesIn:  1000,
					BytesOut: 900,
				},
			})

			// First status check
			status1, err := service.Status(context.Background(), tick)
			tick++
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
			tick++
			Expect(err).NotTo(HaveOccurred())

			// Should detect inactivity
			Expect(status2.RedpandaStatus.MetricsState.IsActive).To(BeFalse())
		})
	})
})
