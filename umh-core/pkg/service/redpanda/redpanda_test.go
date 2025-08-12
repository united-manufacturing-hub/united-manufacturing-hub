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
	"path/filepath"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/redpandaserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda_monitor"
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6_orig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6_shared"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"

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

// newTimeoutContext creates a context with a 30-second timeout
func newTimeoutContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

// createTestSnapshot creates a test SystemSnapshot with the given tick value
func createTestSnapshot(tick uint64) fsm.SystemSnapshot {
	return fsm.SystemSnapshot{
		Tick:         tick,
		SnapshotTime: time.Now(),
		Managers:     make(map[string]fsm.ManagerSnapshot),
	}
}

var _ = Describe("Redpanda Service", func() {
	var (
		service         *RedpandaService
		tick            uint64
		mockSvcRegistry *serviceregistry.Registry
		redpandaName    string
	)

	BeforeEach(func() {
		// Set up mock schema registry server for real network calls
		mockSchemaRegistry := NewMockSchemaRegistry()
		mockSchemaRegistry.SetupTestSchemas()

		schemaRegistry := NewSchemaRegistry(WithSchemaRegistryAddress(mockSchemaRegistry.URL()))

		service = NewDefaultRedpandaService("redpanda",
			WithSchemaRegistryManager(schemaRegistry),
		)
		tick = 0

		// Clean up mock server after each test
		DeferCleanup(func() {
			mockSchemaRegistry.Close()
		})
		mockSvcRegistry = serviceregistry.NewMockRegistry()
		redpandaName = "redpanda"
		// Cleanup the data directory
		ctx, cancel := newTimeoutContext()
		defer cancel()
		err := mockSvcRegistry.GetFileSystem().RemoveAll(ctx, getTmpDir())
		Expect(err).NotTo(HaveOccurred())

		// Add the service to the S6 manager
		config := &redpandaserviceconfig.RedpandaServiceConfig{
			BaseDir: getTmpDir(),
		}
		config.Topic.DefaultTopicRetentionMs = 1000000
		config.Topic.DefaultTopicRetentionBytes = 1000000000
		config.Topic.DefaultTopicCompressionAlgorithm = "snappy"
		config.Topic.DefaultTopicCleanupPolicy = "compact"
		config.Topic.DefaultTopicSegmentMs = 3600000
		ctx, cancel = newTimeoutContext()
		defer cancel()
		err = service.AddRedpandaToS6Manager(ctx, config, mockSvcRegistry.GetFileSystem(), redpandaName)
		Expect(err).NotTo(HaveOccurred())

		// Reconcile the S6 manager
		ctx, cancel = newTimeoutContext()
		defer cancel()
		snapshot := createTestSnapshot(tick)
		err, _ = service.ReconcileManager(ctx, mockSvcRegistry, snapshot)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("GenerateS6ConfigForRedpanda", func() {
		Context("with valid configuration", func() {
			It("should generate valid YAML", func() {
				cfg := &redpandaserviceconfig.RedpandaServiceConfig{
					BaseDir: getTmpDir(),
				}
				cfg.Topic.DefaultTopicRetentionMs = 1000000
				cfg.Topic.DefaultTopicRetentionBytes = 1000000000
				cfg.Topic.DefaultTopicCompressionAlgorithm = "snappy"
				cfg.Topic.DefaultTopicCleanupPolicy = "compact"
				cfg.Topic.DefaultTopicSegmentMs = 3600000

				s6Config, err := service.GenerateS6ConfigForRedpanda(cfg, service.GetS6ServiceName(redpandaName))
				Expect(err).NotTo(HaveOccurred())
				Expect(s6Config.ConfigFiles).To(HaveKey("redpanda.yaml"))
				yaml := s6Config.ConfigFiles["redpanda.yaml"]

				Expect(yaml).To(ContainSubstring("log_retention_ms: 1000000"))
				Expect(yaml).To(ContainSubstring("retention_bytes: 1000000000"))
				Expect(yaml).To(ContainSubstring("log_compression_type: \"snappy\""))
				Expect(yaml).To(ContainSubstring("log_cleanup_policy: \"compact\""))
				Expect(yaml).To(ContainSubstring("log_segment_ms: 3600000"))
			})
		})

		Context("with nil configuration", func() {
			It("should return error", func() {
				s6Config, err := service.GenerateS6ConfigForRedpanda(nil, service.GetS6ServiceName(redpandaName))
				Expect(err).To(HaveOccurred())
				Expect(s6Config).To(Equal(s6serviceconfig.S6ServiceConfig{}))
			})
		})
	})

	Describe("Lifecycle Management", func() {
		var (
			service       *RedpandaService
			mockS6Service *s6service.MockService
		)

		BeforeEach(func() {
			mockS6Service = s6service.NewMockService()

			// Set up mock schema registry server for real network calls
			mockSchemaRegistry := NewMockSchemaRegistry()
			mockSchemaRegistry.SetupTestSchemas()

			schemaRegistry := NewSchemaRegistry(WithSchemaRegistryAddress(mockSchemaRegistry.URL()))

			service = NewDefaultRedpandaService("redpanda",
				WithS6Service(mockS6Service),
				WithSchemaRegistryManager(schemaRegistry),
			)

			// Clean up mock server after each test
			DeferCleanup(func() {
				mockSchemaRegistry.Close()
			})
		})

		It("should handle configuration updates", func() {
			ctx, cancel := newTimeoutContext()
			defer cancel()

			// Use MockRedpandaService for this test to verify method calls
			mockRedpandaService := NewMockRedpandaService()

			// Initial config
			initialConfig := &redpandaserviceconfig.RedpandaServiceConfig{
				BaseDir: getTmpDir(),
			}
			initialConfig.Topic.DefaultTopicRetentionMs = 1000000
			initialConfig.Topic.DefaultTopicRetentionBytes = 1000000000
			initialConfig.Topic.DefaultTopicCompressionAlgorithm = "snappy"
			initialConfig.Topic.DefaultTopicCleanupPolicy = "compact"
			initialConfig.Topic.DefaultTopicSegmentMs = 3600000

			// Add the service with initial config
			By("Adding the Redpanda service with initial config")
			err := mockRedpandaService.AddRedpandaToS6Manager(ctx, initialConfig, mockSvcRegistry.GetFileSystem(), redpandaName)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockRedpandaService.AddRedpandaToS6ManagerCalled).To(BeTrue())

			// Updated config with different retention
			updatedConfig := &redpandaserviceconfig.RedpandaServiceConfig{
				BaseDir: getTmpDir(),
			}
			updatedConfig.Topic.DefaultTopicRetentionMs = 2000000
			updatedConfig.Topic.DefaultTopicRetentionBytes = 2000000000
			updatedConfig.Topic.DefaultTopicCompressionAlgorithm = "lz4"
			updatedConfig.Topic.DefaultTopicCleanupPolicy = "delete"
			updatedConfig.Topic.DefaultTopicSegmentMs = 604800000

			// Update the service configuration
			By("Updating the Redpanda service configuration")
			err = mockRedpandaService.UpdateRedpandaInS6Manager(ctx, updatedConfig, redpandaName)
			Expect(err).NotTo(HaveOccurred())

			// Reconcile to apply changes
			By("Reconciling the manager to apply configuration changes")
			snapshot := createTestSnapshot(0)
			err, _ = mockRedpandaService.ReconcileManager(ctx, mockSvcRegistry, snapshot)
			Expect(err).NotTo(HaveOccurred())

			// Verify the configuration was updated in the S6 manager
			By("Verifying the UpdateRedpandaInS6Manager was called")
			Expect(mockRedpandaService.UpdateRedpandaInS6ManagerCalled).To(BeTrue())
		})

		It("should handle non-existent service errors", func() {
			ctx, cancel := newTimeoutContext()
			defer cancel()

			// Try to update a non-existent service
			By("Trying to update a non-existent service")
			err := service.UpdateRedpandaInS6Manager(ctx, &redpandaserviceconfig.RedpandaServiceConfig{
				BaseDir: getTmpDir(),
			}, redpandaName)
			Expect(err).To(Equal(ErrServiceNotExist))

			// Try to stop a non-existent service
			By("Trying to stop a non-existent service")
			err = service.StopRedpanda(ctx, redpandaName)
			Expect(err).To(Equal(ErrServiceNotExist))

			// Try to remove a non-existent service
			By("Trying to remove a non-existent service")
			err = service.RemoveRedpandaFromS6Manager(ctx, redpandaName)
			Expect(err).To(Equal(ErrServiceNotExist))
		})

		It("should handle already existing service errors", func() {
			ctx, cancel := newTimeoutContext()
			defer cancel()

			// Add a service
			By("Adding a service")
			err := service.AddRedpandaToS6Manager(ctx, &redpandaserviceconfig.RedpandaServiceConfig{
				BaseDir: getTmpDir(),
			}, mockSvcRegistry.GetFileSystem(), redpandaName)
			Expect(err).NotTo(HaveOccurred())

			// Try to add the same service again
			By("Trying to add the same service again")
			err = service.AddRedpandaToS6Manager(ctx, &redpandaserviceconfig.RedpandaServiceConfig{
				BaseDir: getTmpDir(),
			}, mockSvcRegistry.GetFileSystem(), redpandaName)
			Expect(err).To(Equal(ErrServiceAlreadyExists))
		})
	})

	Context("Metrics Analysis", func() {
		var service *RedpandaService

		BeforeEach(func() {
			// Set up mock schema registry server for real network calls
			mockSchemaRegistry := NewMockSchemaRegistry()
			mockSchemaRegistry.SetupTestSchemas()

			schemaRegistry := NewSchemaRegistry(WithSchemaRegistryAddress(mockSchemaRegistry.URL()))

			service = NewDefaultRedpandaService("redpanda",
				WithSchemaRegistryManager(schemaRegistry),
			)

			// Clean up mock server after each test
			DeferCleanup(func() {
				mockSchemaRegistry.Close()
			})
		})

		Context("IsMetricsErrorFree", func() {
			It("should return true when there are no errors", func() {
				metrics := redpanda_monitor.Metrics{
					Infrastructure: redpanda_monitor.InfrastructureMetrics{
						Storage: redpanda_monitor.StorageMetrics{
							FreeBytes:      5000000000,
							TotalBytes:     10000000000,
							FreeSpaceAlert: false,
						},
					},
					Cluster: redpanda_monitor.ClusterMetrics{
						Topics:            5,
						UnavailableTopics: 0, // No unavailable topics
					},
				}
				result, _ := service.IsMetricsErrorFree(metrics)
				Expect(result).To(BeTrue())
			})

			It("should detect storage free space alerts", func() {
				metrics := redpanda_monitor.Metrics{
					Infrastructure: redpanda_monitor.InfrastructureMetrics{
						Storage: redpanda_monitor.StorageMetrics{
							FreeBytes:      100000,
							TotalBytes:     10000000000,
							FreeSpaceAlert: true, // Alert triggered
						},
					},
				}
				result, _ := service.IsMetricsErrorFree(metrics)
				Expect(result).To(BeFalse())
			})

			It("should detect unavailable topics", func() {
				metrics := redpanda_monitor.Metrics{
					Infrastructure: redpanda_monitor.InfrastructureMetrics{
						Storage: redpanda_monitor.StorageMetrics{
							FreeBytes:      5000000000,
							TotalBytes:     10000000000,
							FreeSpaceAlert: false, // No storage alert
						},
					},
					Cluster: redpanda_monitor.ClusterMetrics{
						Topics:            10,
						UnavailableTopics: 2, // Some topics are unavailable
					},
				}
				result, _ := service.IsMetricsErrorFree(metrics)
				Expect(result).To(BeFalse())
			})

			It("should pass when no storage alerts and all topics available", func() {
				metrics := redpanda_monitor.Metrics{
					Infrastructure: redpanda_monitor.InfrastructureMetrics{
						Storage: redpanda_monitor.StorageMetrics{
							FreeBytes:      5000000000,
							TotalBytes:     10000000000,
							FreeSpaceAlert: false, // No storage alert
						},
					},
					Cluster: redpanda_monitor.ClusterMetrics{
						Topics:            10,
						UnavailableTopics: 0, // All topics available
					},
				}
				result, _ := service.IsMetricsErrorFree(metrics)
				Expect(result).To(BeTrue())
			})
		})

		Context("HasProcessingActivity", func() {
			It("should detect processing activity", func() {
				metricsState := redpanda_monitor.NewRedpandaMetricsState()
				metricsState.IsActive = true

				status := RedpandaStatus{
					RedpandaMetrics: redpanda_monitor.RedpandaMetrics{
						MetricsState: metricsState,
					},
				}

				result, _ := service.HasProcessingActivity(status)
				Expect(result).To(BeTrue())
			})

			It("should detect lack of processing activity", func() {
				metricsState := redpanda_monitor.NewRedpandaMetricsState()
				metricsState.IsActive = false

				status := RedpandaStatus{
					RedpandaMetrics: redpanda_monitor.RedpandaMetrics{
						MetricsState: metricsState,
					},
				}

				result, _ := service.HasProcessingActivity(status)
				Expect(result).To(BeFalse())
			})

			It("should handle nil metrics state", func() {
				status := RedpandaStatus{
					RedpandaMetrics: redpanda_monitor.RedpandaMetrics{
						MetricsState: nil,
					},
				}

				result, _ := service.HasProcessingActivity(status)
				Expect(result).To(BeFalse())
			})
		})
	})

	Context("Log Analysis", func() {
		var (
			service     *RedpandaService
			currentTime time.Time
			logWindow   time.Duration
		)

		BeforeEach(func() {
			service = NewDefaultRedpandaService("redpanda",
				WithSchemaRegistryManager(NewNoOpSchemaRegistry()),
			)
			currentTime = time.Now()
			logWindow = 5 * time.Minute
		})

		Context("IsLogsFine", func() {
			It("should return true when there are no logs", func() {
				result, _ := service.IsLogsFine([]s6_shared.LogEntry{}, currentTime, logWindow, time.Time{})
				Expect(result).To(BeTrue())
			})

			It("should detect 'Address already in use' errors", func() {
				logs := []s6_shared.LogEntry{
					{
						Timestamp: currentTime.Add(-1 * time.Minute),
						Content:   "INFO Starting Redpanda",
					},
					{
						Timestamp: currentTime.Add(-30 * time.Second),
						Content:   "ERROR Address already in use (port 9092)",
					},
				}
				result, _ := service.IsLogsFine(logs, currentTime, logWindow, time.Time{})
				Expect(result).To(BeFalse())
			})

			It("should detect 'Reactor stalled for' errors", func() {
				logs := []s6_shared.LogEntry{
					{
						Timestamp: currentTime.Add(-1 * time.Minute),
						Content:   "INFO Starting Redpanda",
					},
					{
						Timestamp: currentTime.Add(-30 * time.Second),
						Content:   "WARN Reactor stalled for 2000 ms",
					},
				}
				result, _ := service.IsLogsFine(logs, currentTime, logWindow, time.Time{})
				Expect(result).To(BeFalse())
			})

			It("should pass with normal logs", func() {
				logs := []s6_shared.LogEntry{
					{
						Timestamp: currentTime.Add(-1 * time.Minute),
						Content:   "INFO Starting Redpanda",
					},
					{
						Timestamp: currentTime.Add(-30 * time.Second),
						Content:   "INFO Created topic 'test-topic' with 3 partitions",
					},
				}
				result, _ := service.IsLogsFine(logs, currentTime, logWindow, time.Time{})
				Expect(result).To(BeTrue())
			})

			It("should ignore logs outside the time window", func() {
				logs := []s6_shared.LogEntry{
					{
						Timestamp: currentTime.Add(-10 * time.Minute),
						Content:   "ERROR Address already in use (port 9092)",
					},
					{
						Timestamp: currentTime.Add(-30 * time.Second),
						Content:   "INFO Created topic 'test-topic' with 3 partitions",
					},
				}
				result, _ := service.IsLogsFine(logs, currentTime, logWindow, time.Time{})
				Expect(result).To(BeTrue())
			})

			It("should handle logs at the edge of the time window", func() {
				errorLog := s6_shared.LogEntry{
					Timestamp: currentTime.Add(-logWindow),
					Content:   "ERROR Address already in use (port 9092)",
				}

				windowStart := currentTime.Add(-logWindow)
				fmt.Printf("Debug - Window start: %v\n", windowStart)
				fmt.Printf("Debug - Error log time: %v\n", errorLog.Timestamp)
				fmt.Printf("Debug - Is error log before window start? %v\n", errorLog.Timestamp.Before(windowStart))
				fmt.Printf("Debug - Is error log equal to window start? %v\n", errorLog.Timestamp.Equal(windowStart))

				logs := []s6_shared.LogEntry{errorLog}

				result, _ := service.IsLogsFine(logs, currentTime, logWindow, time.Time{})
				fmt.Printf("Debug - IsLogsFine result: %v\n", result)
				Expect(result).To(BeFalse(), "Error log exactly at window boundary should be excluded")

				normalLog := s6_shared.LogEntry{
					Timestamp: currentTime.Add(-logWindow).Add(1 * time.Millisecond),
					Content:   "INFO Normal log inside window",
				}
				logs = append(logs, normalLog)
				fmt.Printf("Debug - Normal log time: %v\n", normalLog.Timestamp)
				fmt.Printf("Debug - Is normal log before window start? %v\n", normalLog.Timestamp.Before(windowStart))

				result, _ = service.IsLogsFine(logs, currentTime, logWindow, time.Time{})
				fmt.Printf("Debug - IsLogsFine result with normal log: %v\n", result)
				Expect(result).To(BeFalse(), "Adding a normal log inside the window should not affect the result")

				errorLogInside := s6_shared.LogEntry{
					Timestamp: currentTime.Add(-logWindow).Add(2 * time.Millisecond),
					Content:   "ERROR Reactor stalled for 5s",
				}
				logs = append(logs, errorLogInside)
				fmt.Printf("Debug - Error log inside time: %v\n", errorLogInside.Timestamp)
				fmt.Printf("Debug - Is error log inside before window start? %v\n", errorLogInside.Timestamp.Before(windowStart))

				result, _ = service.IsLogsFine(logs, currentTime, logWindow, time.Time{})
				fmt.Printf("Debug - IsLogsFine result with error log inside: %v\n", result)
				Expect(result).To(BeFalse(), "Error log inside window should cause the check to fail")
			})

			It("should evaluate logs with mixed timestamps correctly", func() {
				logs := []s6_shared.LogEntry{
					{
						Timestamp: currentTime.Add(-10 * time.Minute),
						Content:   "ERROR Address already in use (port 9092)",
					},
					{
						Timestamp: currentTime.Add(-7 * time.Minute),
						Content:   "INFO Starting Redpanda",
					},
					{
						Timestamp: currentTime.Add(-4 * time.Minute),
						Content:   "INFO Created topic 'test-topic'",
					},
					{
						Timestamp: currentTime.Add(-3 * time.Minute),
						Content:   "ERROR Reactor stalled for 10000 ms",
					},
					{
						Timestamp: currentTime.Add(-1 * time.Minute),
						Content:   "INFO Successfully processed batch",
					},
				}

				result, _ := service.IsLogsFine(logs, currentTime, logWindow, time.Time{})
				Expect(result).To(BeFalse())

				adjustedTime := currentTime.Add(3 * time.Minute)
				result, _ = service.IsLogsFine(logs, adjustedTime, logWindow, time.Time{})
				Expect(result).To(BeTrue())
			})

			It("should respect different window sizes", func() {
				logs := []s6_shared.LogEntry{
					{
						Timestamp: currentTime.Add(-3 * time.Minute),
						Content:   "ERROR Address already in use (port 9092)",
					},
				}

				result, _ := service.IsLogsFine(logs, currentTime, 5*time.Minute, time.Time{})
				Expect(result).To(BeFalse())

				result, _ = service.IsLogsFine(logs, currentTime, 2*time.Minute, time.Time{})
				Expect(result).To(BeTrue())

				result, _ = service.IsLogsFine(logs, currentTime, 10*time.Minute, time.Time{})
				Expect(result).To(BeFalse())
			})

			It("should ignore errors before transition time", func() {
				transitionTime := currentTime.Add(-2 * time.Minute)
				logs := []s6_shared.LogEntry{
					{
						Timestamp: currentTime.Add(-3 * time.Minute), // Before transition
						Content:   "ERROR Reactor stalled for 1000 ms",
					},
					{
						Timestamp: currentTime.Add(-1 * time.Minute), // After transition
						Content:   "INFO Normal operation",
					},
				}
				result, _ := service.IsLogsFine(logs, currentTime, logWindow, transitionTime)
				Expect(result).To(BeTrue())
			})

			It("should detect errors after transition time", func() {
				transitionTime := currentTime.Add(-2 * time.Minute)
				logs := []s6_shared.LogEntry{
					{
						Timestamp: currentTime.Add(-1 * time.Minute), // After transition
						Content:   "ERROR Reactor stalled for 1000 ms",
					},
				}
				result, _ := service.IsLogsFine(logs, currentTime, logWindow, transitionTime)
				Expect(result).To(BeFalse())
			})
		})
	})

	Describe("ForceRemoveRedpanda", func() {
		var (
			service       *RedpandaService
			mockS6Service *s6service.MockService
			mockFS        *filesystem.MockFileSystem
		)

		BeforeEach(func() {
			mockS6Service = s6service.NewMockService()
			mockFS = filesystem.NewMockFileSystem()

			// Set up mock schema registry server for real network calls
			mockSchemaRegistry := NewMockSchemaRegistry()
			mockSchemaRegistry.SetupTestSchemas()

			schemaRegistry := NewSchemaRegistry(WithSchemaRegistryAddress(mockSchemaRegistry.URL()))

			service = NewDefaultRedpandaService("redpanda",
				WithS6Service(mockS6Service),
				WithSchemaRegistryManager(schemaRegistry),
			)

			// Clean up mock server after each test
			DeferCleanup(func() {
				mockSchemaRegistry.Close()
			})
		})

		It("should call S6 ForceRemove with the correct service path", func() {
			ctx, cancel := newTimeoutContext()
			defer cancel()

			// Call ForceRemoveRedpanda
			err := service.ForceRemoveRedpanda(ctx, mockFS, redpandaName)

			// Verify no error
			Expect(err).NotTo(HaveOccurred())

			// Verify S6Service ForceRemove was called
			Expect(mockS6Service.ForceRemoveCalled).To(BeTrue())

			// Verify the path is correct
			expectedS6ServiceName := service.GetS6ServiceName(redpandaName)
			expectedS6ServicePath := filepath.Join(constants.S6BaseDir, expectedS6ServiceName)
			Expect(mockS6Service.ForceRemovePath).To(Equal(expectedS6ServicePath))
		})

		It("should propagate errors from S6 service", func() {
			ctx, cancel := newTimeoutContext()
			defer cancel()

			// Set up mock to return an error
			mockError := fmt.Errorf("mock force remove error")
			mockS6Service.ForceRemoveError = mockError

			// Call ForceRemoveRedpanda
			err := service.ForceRemoveRedpanda(ctx, mockFS, redpandaName)

			// Verify error is propagated
			Expect(err).To(MatchError(mockError))
		})
	})
})
