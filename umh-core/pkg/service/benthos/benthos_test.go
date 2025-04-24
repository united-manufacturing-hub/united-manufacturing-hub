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

package benthos

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Benthos Service", func() {
	var (
		service                   *BenthosService
		tick                      uint64
		benthosName               string
		mockFS                    *filesystem.MockFileSystem
		benthosMonitorMockService *benthos_monitor.MockBenthosMonitorService
	)

	BeforeEach(func() {
		benthosMonitorMockService = benthos_monitor.NewMockBenthosMonitorService()
		service = NewDefaultBenthosService(benthosName, WithMonitorService(benthosMonitorMockService))
		tick = 0
		mockFS = filesystem.NewMockFileSystem()
		// Add the service to the S6 manager
		err := service.AddBenthosToS6Manager(context.Background(), mockFS, &benthosserviceconfig.BenthosServiceConfig{
			MetricsPort: 4195,
			LogLevel:    "info",
		}, benthosName)
		Expect(err).NotTo(HaveOccurred())

		// Reconcile the S6 and benthos monitor manager
		err, _ = service.ReconcileManager(context.Background(), mockFS, tick)
		Expect(err).NotTo(HaveOccurred())

		benthosMonitorMockService.SetReadyStatus(true, true, "")

		benthosMonitorMockService.SetMetricsResponse(benthos_monitor.Metrics{
			Input: benthos_monitor.InputMetrics{
				Received:     10,
				ConnectionUp: 1,
				LatencyNS: benthos_monitor.Latency{
					P50:   1000000,
					P90:   2000000,
					P99:   3000000,
					Sum:   1500000,
					Count: 5,
				},
			},
			Output: benthos_monitor.OutputMetrics{
				Sent:         8,
				BatchSent:    2,
				ConnectionUp: 1,
				LatencyNS: benthos_monitor.Latency{
					P50:   1000000,
					P90:   2000000,
					P99:   3000000,
					Sum:   1500000,
					Count: 5,
				},
			},
			Process: benthos_monitor.ProcessMetrics{
				Processors: map[string]benthos_monitor.ProcessorMetrics{
					"/pipeline/processors/0": {
						Label:         "0",
						Received:      5,
						BatchReceived: 1,
						Sent:          5,
						BatchSent:     1,
						Error:         0,
						LatencyNS: benthos_monitor.Latency{
							P50:   1000000,
							P90:   2000000,
							P99:   3000000,
							Sum:   1500000,
							Count: 5,
						},
					},
				},
			},
		})
	})

	Describe("GetHealthCheckAndMetrics", func() {
		Context("with valid metrics port", func() {
			BeforeEach(func() {
				benthosMonitorMockService.SetReadyStatus(true, true, "")
				benthosMonitorMockService.SetMetricsResponse(benthos_monitor.Metrics{
					Input: benthos_monitor.InputMetrics{
						Received:     10,
						ConnectionUp: 1,
						LatencyNS: benthos_monitor.Latency{
							P50:   1000000,
							P90:   2000000,
							P99:   3000000,
							Sum:   1500000,
							Count: 5,
						},
					},
					Output: benthos_monitor.OutputMetrics{
						Sent:         8,
						BatchSent:    2,
						ConnectionUp: 1,
						LatencyNS: benthos_monitor.Latency{
							P50:   1000000,
							P90:   2000000,
							P99:   3000000,
							Sum:   1500000,
							Count: 5,
						},
					},
					Process: benthos_monitor.ProcessMetrics{
						Processors: map[string]benthos_monitor.ProcessorMetrics{
							"/pipeline/processors/0": {
								Label:         "0",
								Received:      5,
								BatchReceived: 1,
								Sent:          5,
								BatchSent:     1,
								Error:         0,
								LatencyNS: benthos_monitor.Latency{
									P50:   1000000,
									P90:   2000000,
									P99:   3000000,
									Sum:   1500000,
									Count: 5,
								},
							},
						},
					},
				})
			})

			It("should return health check and metrics", func() {

				ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
				defer cancel()
				status, err := service.metricsService.Status(ctx, mockFS, tick)
				tick++
				Expect(err).NotTo(HaveOccurred())
				Expect(status.BenthosStatus.LastScan.HealthCheck.IsReady).To(BeTrue())
				Expect(status.BenthosStatus.LastScan.BenthosMetrics.Metrics.Input.Received).To(Equal(int64(10)))
				Expect(status.BenthosStatus.LastScan.BenthosMetrics.Metrics.Input.ConnectionUp).To(Equal(int64(1)))
				Expect(status.BenthosStatus.LastScan.BenthosMetrics.Metrics.Output.Sent).To(Equal(int64(8)))
				Expect(status.BenthosStatus.LastScan.BenthosMetrics.Metrics.Output.BatchSent).To(Equal(int64(2)))
				Expect(status.BenthosStatus.LastScan.BenthosMetrics.Metrics.Process.Processors).To(HaveLen(1))
				proc := status.BenthosStatus.LastScan.BenthosMetrics.Metrics.Process.Processors["/pipeline/processors/0"]
				Expect(proc.Label).To(Equal("0"))
				Expect(proc.Received).To(Equal(int64(5)))
				Expect(proc.BatchReceived).To(Equal(int64(1)))
				Expect(proc.Sent).To(Equal(int64(5)))
				Expect(proc.BatchSent).To(Equal(int64(1)))
				Expect(proc.Error).To(Equal(int64(0)))
				Expect(proc.LatencyNS.P50).To(Equal(float64(1000000)))
				Expect(proc.LatencyNS.P90).To(Equal(float64(2000000)))
				Expect(proc.LatencyNS.P99).To(Equal(float64(3000000)))
				Expect(proc.LatencyNS.Sum).To(Equal(float64(1500000)))
				Expect(proc.LatencyNS.Count).To(Equal(int64(5)))
			})
		})
	})

	Describe("GenerateS6ConfigForBenthos", func() {
		Context("with minimal configuration", func() {
			It("should generate valid YAML", func() {
				cfg := &benthosserviceconfig.BenthosServiceConfig{
					MetricsPort: 4195,
					LogLevel:    "INFO",
				}

				s6Config, err := service.GenerateS6ConfigForBenthos(cfg, "test")
				Expect(err).NotTo(HaveOccurred())
				Expect(s6Config.ConfigFiles).To(HaveKey("benthos.yaml"))
				yaml := s6Config.ConfigFiles["benthos.yaml"]

				Expect(yaml).To(ContainSubstring("input: []"))
				Expect(yaml).To(ContainSubstring("output: []"))
				Expect(yaml).To(ContainSubstring("pipeline:"))
				Expect(yaml).To(ContainSubstring("processors: []"))
				Expect(yaml).To(ContainSubstring("http:\n  address: 0.0.0.0:4195"))
				Expect(yaml).To(ContainSubstring("logger:\n  level: INFO"))
			})
		})

		Context("with complete configuration", func() {
			It("should generate valid YAML with all sections", func() {
				cfg := &benthosserviceconfig.BenthosServiceConfig{
					Input: map[string]interface{}{
						"mqtt": map[string]interface{}{"topic": "test/topic"},
					},
					Output: map[string]interface{}{
						"kafka": map[string]interface{}{"topic": "test-output"},
					},
					Pipeline: map[string]interface{}{
						"processors": []map[string]interface{}{
							{"text": map[string]interface{}{"operator": "to_upper"}},
						},
					},
					CacheResources: []map[string]interface{}{
						{"memory": map[string]interface{}{"ttl": "60s"}},
					},
					RateLimitResources: []map[string]interface{}{
						{"local": map[string]interface{}{"count": "100"}},
					},
					Buffer: map[string]interface{}{
						"memory": map[string]interface{}{"limit": "10MB"},
					},
					MetricsPort: 4195,
					LogLevel:    "INFO",
				}

				s6Config, err := service.GenerateS6ConfigForBenthos(cfg, "test")
				Expect(err).NotTo(HaveOccurred())
				Expect(s6Config.ConfigFiles).To(HaveKey("benthos.yaml"))
				yaml := s6Config.ConfigFiles["benthos.yaml"]

				Expect(yaml).To(ContainSubstring("input:\n    mqtt:\n        topic: test/topic"))
				Expect(yaml).To(ContainSubstring("output:\n    kafka:\n        topic: test-output"))
				Expect(yaml).To(ContainSubstring("pipeline:\n    processors:\n        - text:\n            operator: to_upper"))
				Expect(yaml).To(ContainSubstring("cache_resources:\n    - memory:\n        ttl: 60s"))
				Expect(yaml).To(ContainSubstring("rate_limit_resources:\n    - local:\n        count: \"100\""))
				Expect(yaml).To(ContainSubstring("buffer:\n    memory:\n        limit: 10MB"))
				Expect(yaml).To(ContainSubstring("http:\n  address: 0.0.0.0:4195"))
				Expect(yaml).To(ContainSubstring("logger:\n  level: INFO"))
			})
		})

		Context("with nil configuration", func() {
			It("should return error", func() {
				s6Config, err := service.GenerateS6ConfigForBenthos(nil, "test")
				Expect(err).To(HaveOccurred())
				Expect(s6Config).To(Equal(s6serviceconfig.S6ServiceConfig{}))
			})
		})
	})

	Context("Log Analysis", func() {
		var (
			service     *BenthosService
			currentTime time.Time
			logWindow   time.Duration
		)

		BeforeEach(func() {
			service = NewDefaultBenthosService("test")
			currentTime = time.Now()
			logWindow = 5 * time.Minute
		})

		Context("IsLogsFine", func() {
			It("should return true when there are no logs", func() {
				Expect(service.IsLogsFine([]s6service.LogEntry{}, currentTime, logWindow)).To(BeTrue())
			})

			It("should detect official Benthos error logs", func() {
				logs := []s6service.LogEntry{
					{
						Timestamp: currentTime.Add(-1 * time.Minute),
						Content:   `level=error msg="failed to connect to broker"`,
					},
				}
				Expect(service.IsLogsFine(logs, currentTime, logWindow)).To(BeFalse())
			})

			It("should detect critical warnings in official Benthos logs", func() {
				logs := []s6service.LogEntry{
					{
						Timestamp: currentTime.Add(-1 * time.Minute),
						Content:   `level=warn msg="failed to process message"`,
					},
					{
						Timestamp: currentTime.Add(-2 * time.Minute),
						Content:   `level=warn msg="connection lost to server"`,
					},
					{
						Timestamp: currentTime.Add(-3 * time.Minute),
						Content:   `level=warn msg="unable to reach endpoint"`,
					},
				}
				Expect(service.IsLogsFine(logs, currentTime, logWindow)).To(BeFalse())
			})

			It("should ignore non-critical warnings in official Benthos logs", func() {
				logs := []s6service.LogEntry{
					{
						Timestamp: currentTime.Add(-1 * time.Minute),
						Content:   `level=warn msg="rate limit applied"`,
					},
					{
						Timestamp: currentTime.Add(-2 * time.Minute),
						Content:   `level=warn msg="message batch partially processed"`,
					},
				}
				Expect(service.IsLogsFine(logs, currentTime, logWindow)).To(BeTrue())
			})

			It("should detect configuration file read errors", func() {
				logs := []s6service.LogEntry{
					{
						Timestamp: currentTime.Add(-1 * time.Minute),
						Content:   `configuration file read error: file not found`,
					},
				}
				Expect(service.IsLogsFine(logs, currentTime, logWindow)).To(BeFalse())
			})

			It("should detect logger creation errors", func() {
				logs := []s6service.LogEntry{
					{
						Timestamp: currentTime.Add(-1 * time.Minute),
						Content:   `failed to create logger: invalid log level`,
					},
				}
				Expect(service.IsLogsFine(logs, currentTime, logWindow)).To(BeFalse())
			})

			It("should detect linter errors", func() {
				logs := []s6service.LogEntry{
					{
						Timestamp: currentTime.Add(-1 * time.Minute),
						Content:   `Config lint error: invalid input type`,
					},
					{
						Timestamp: currentTime.Add(-2 * time.Minute),
						Content:   `shutting down due to linter errors`,
					},
				}
				Expect(service.IsLogsFine(logs, currentTime, logWindow)).To(BeFalse())
			})

			It("should ignore error-like strings in message content", func() {
				logs := []s6service.LogEntry{
					{
						Timestamp: currentTime.Add(-1 * time.Minute),
						Content:   `level=info msg="Processing message: configuration file read error in payload"`,
					},
					{
						Timestamp: currentTime.Add(-2 * time.Minute),
						Content:   `level=info msg="Log contains warning: user notification"`,
					},
					{
						Timestamp: currentTime.Add(-3 * time.Minute),
						Content:   `level=info msg="Error rate metrics collected"`,
					},
				}
				Expect(service.IsLogsFine(logs, currentTime, logWindow)).To(BeTrue())
			})

			It("should ignore error patterns not at start of line", func() {
				logs := []s6service.LogEntry{
					{
						Timestamp: currentTime.Add(-1 * time.Minute),
						Content:   `level=info msg="User reported: configuration file read error"`,
					},
					{
						Timestamp: currentTime.Add(-2 * time.Minute),
						Content:   `level=info msg="System status: failed to create logger mentioned in docs"`,
					},
					{
						Timestamp: currentTime.Add(-3 * time.Minute),
						Content:   `level=info msg="Documentation: Config lint error examples"`,
					},
				}
				Expect(service.IsLogsFine(logs, currentTime, logWindow)).To(BeTrue())
			})

			It("should handle mixed log types correctly", func() {
				logs := []s6service.LogEntry{
					{
						Timestamp: currentTime.Add(-1 * time.Minute),
						Content:   `level=info msg="Starting up Benthos service"`,
					},
					{
						Timestamp: currentTime.Add(-2 * time.Minute),
						Content:   `level=warn msg="rate limit applied"`,
					},
					{
						Timestamp: currentTime.Add(-3 * time.Minute),
						Content:   `level=info msg="Processing message: warning in content"`,
					},
					{
						Timestamp: currentTime.Add(-4 * time.Minute),
						Content:   `level=error msg="failed to connect"`,
					},
				}
				Expect(service.IsLogsFine(logs, currentTime, logWindow)).To(BeFalse())
			})

			It("should handle malformed Benthos logs gracefully", func() {
				logs := []s6service.LogEntry{
					{
						Timestamp: currentTime.Add(-1 * time.Minute),
						Content:   `levelerror msg="broken log format"`,
					},
					{
						Timestamp: currentTime.Add(-2 * time.Minute),
						Content:   `level=info missing quote`,
					},
					{
						Timestamp: currentTime.Add(-3 * time.Minute),
						Content:   `random text with warning and error keywords`,
					},
				}
				Expect(service.IsLogsFine(logs, currentTime, logWindow)).To(BeTrue())
			})

			It("should ignore logs outside the time window", func() {
				logs := []s6service.LogEntry{
					{
						Timestamp: currentTime.Add(-1 * time.Minute),
						Content:   `level=info msg="Recent normal log"`,
					},
					{
						Timestamp: currentTime.Add(-10 * time.Minute),
						Content:   `level=error msg="Old error that should be ignored"`,
					},
				}
				Expect(service.IsLogsFine(logs, currentTime, logWindow)).To(BeTrue())
			})
		})
	})

	Context("Metrics Analysis", func() {
		var service *BenthosService

		BeforeEach(func() {
			service = NewDefaultBenthosService("test")
		})

		Context("IsMetricsErrorFree", func() {
			It("should return true when there are no errors", func() {
				metrics := benthos_monitor.BenthosMetrics{
					Metrics: benthos_monitor.Metrics{
						Input: benthos_monitor.InputMetrics{
							ConnectionFailed: 5,
							ConnectionLost:   1,
							ConnectionUp:     1,
							Received:         100,
						},
						Output: benthos_monitor.OutputMetrics{
							Error:            0,
							ConnectionFailed: 3,
							ConnectionLost:   0,
							ConnectionUp:     1,
							Sent:             90,
							BatchSent:        10,
						},
						Process: benthos_monitor.ProcessMetrics{
							Processors: map[string]benthos_monitor.ProcessorMetrics{
								"proc1": {
									Error:         0,
									Received:      100,
									Sent:          100,
									BatchReceived: 10,
									BatchSent:     10,
								},
							},
						},
					},
				}
				Expect(service.IsMetricsErrorFree(metrics)).To(BeTrue())
			})

			It("should detect output errors", func() {
				metrics := benthos_monitor.BenthosMetrics{
					Metrics: benthos_monitor.Metrics{
						Output: benthos_monitor.OutputMetrics{
							Error: 1,
						},
					},
				}
				Expect(service.IsMetricsErrorFree(metrics)).To(BeFalse())
			})

			It("should ignore connection failures", func() {
				metrics := benthos_monitor.BenthosMetrics{
					Metrics: benthos_monitor.Metrics{
						Input: benthos_monitor.InputMetrics{
							ConnectionFailed: 1,
						},
						Output: benthos_monitor.OutputMetrics{
							ConnectionFailed: 1,
						},
					},
				}
				Expect(service.IsMetricsErrorFree(metrics)).To(BeTrue())
			})

			It("should detect processor errors", func() {
				metrics := benthos_monitor.BenthosMetrics{
					Metrics: benthos_monitor.Metrics{
						Process: benthos_monitor.ProcessMetrics{
							Processors: map[string]benthos_monitor.ProcessorMetrics{
								"proc1": {
									Error: 1,
								},
							},
						},
					},
				}
				Expect(service.IsMetricsErrorFree(metrics)).To(BeFalse())
			})

			It("should detect errors in any processor", func() {
				metrics := benthos_monitor.BenthosMetrics{
					Metrics: benthos_monitor.Metrics{
						Process: benthos_monitor.ProcessMetrics{
							Processors: map[string]benthos_monitor.ProcessorMetrics{
								"proc1": {Error: 0},
								"proc2": {Error: 1},
								"proc3": {Error: 0},
							},
						},
					},
				}
				Expect(service.IsMetricsErrorFree(metrics)).To(BeFalse())
			})
		})
	})

	// TestGetConfig tests the GetConfig method with S6 service integration
	Describe("GetConfig", func() {
		var (
			service     *BenthosService
			benthosName string
			mockFS      *filesystem.MockFileSystem
		)

		BeforeEach(func() {
			benthosName = "test-benthos"
			service = NewDefaultBenthosService(benthosName)
			mockFS = filesystem.NewMockFileSystem()
		})

		Context("when context is cancelled", func() {
			It("should return context error", func() {
				cancelledCtx, cancel := context.WithCancel(context.Background())
				cancel() // Cancel immediately

				_, err := service.GetConfig(cancelledCtx, mockFS, benthosName)
				Expect(err).To(Equal(context.Canceled))
			})
		})
	})

	Describe("Configuration update and service restart", func() {
		var (
			service                   *BenthosService
			benthosMonitorMockService *benthos_monitor.MockBenthosMonitorService
			tick                      uint64
			benthosName               string
			s6ServiceMock             *s6service.MockService
			initialConfig             *benthosserviceconfig.BenthosServiceConfig
			updatedConfig             *benthosserviceconfig.BenthosServiceConfig
			mockFS                    *filesystem.MockFileSystem
		)

		BeforeEach(func() {
			// Setup mocks
			benthosMonitorMockService = benthos_monitor.NewMockBenthosMonitorService()
			service = NewDefaultBenthosService(benthosName, WithMonitorService(benthosMonitorMockService))
			s6ServiceMock = s6service.NewMockService()
			tick = 0
			mockFS = filesystem.NewMockFileSystem()
			// Set up a mock HTTP client for Benthos health and metrics endpoints
			benthosMonitorMockService.SetReadyStatus(true, true, "")
			benthosMonitorMockService.SetLiveStatus(true)
			benthosMonitorMockService.SetMetricsResponse(benthos_monitor.Metrics{
				Input: benthos_monitor.InputMetrics{
					Received:     10,
					ConnectionUp: 1,
				},
				Output: benthos_monitor.OutputMetrics{
					Sent:         10,
					ConnectionUp: 1,
				},
			})
			benthosMonitorMockService.SetBenthosMonitorRunning()

			// Create service with mocks
			service = NewDefaultBenthosService(benthosName,
				WithMonitorService(benthosMonitorMockService),
				WithS6Service(s6ServiceMock))

			// Setup initial configuration
			initialConfig = &benthosserviceconfig.BenthosServiceConfig{
				MetricsPort: 4195,
				LogLevel:    "info",
				Input: map[string]interface{}{
					"mqtt": map[string]interface{}{
						"urls":   []string{"tcp://localhost:1883"},
						"topics": []string{"test-topic"},
					},
				},
				Output: map[string]interface{}{
					"stdout": map[string]interface{}{},
				},
			}

			// Setup updated configuration with different input
			updatedConfig = &benthosserviceconfig.BenthosServiceConfig{
				MetricsPort: 4195,
				LogLevel:    "info",
				Input: map[string]interface{}{
					"mqtt": map[string]interface{}{
						"urls":   []string{"tcp://localhost:1883"},
						"topics": []string{"updated-topic"}, // Changed topic
					},
				},
				Output: map[string]interface{}{
					"stdout": map[string]interface{}{},
				},
			}
		})

		It("should restart the service when configuration changes", func() {
			ctx := context.Background()

			// Initial service creation
			By("Adding the Benthos service to S6 manager with initial config")
			err := service.AddBenthosToS6Manager(ctx, mockFS, initialConfig, benthosName)
			Expect(err).NotTo(HaveOccurred())
			Expect(s6ServiceMock.CreateCalled).To(BeFalse(), "S6 service shouldn't be created directly")

			// First reconciliation - creates the service
			By("Reconciling the manager to create the service")
			err, _ = service.ReconcileManager(ctx, mockFS, tick)
			Expect(err).NotTo(HaveOccurred())

			// Set up the mock S6 service status to simulate running service
			s6ServiceMock.StatusResult = s6service.ServiceInfo{
				Status:   s6service.ServiceUp,
				WantUp:   true,
				Pid:      12345,
				Uptime:   10,
				ExitCode: 0,
			}

			// Start the service
			By("Starting the Benthos service")
			err = service.StartBenthos(ctx, mockFS, benthosName)
			Expect(err).NotTo(HaveOccurred())

			// Mock the GetConfig return values - first return initialConfig
			s6ServiceMock.GetS6ConfigFileResult = []byte("initial-config-content")

			// Create a mock config for the MockService.GetConfig method
			serviceConfigYaml, err := service.generateBenthosYaml(initialConfig)
			Expect(err).NotTo(HaveOccurred())

			// Create expected S6 config with initial Benthos config
			s6ServiceMock.GetConfigResult = s6serviceconfig.S6ServiceConfig{
				ConfigFiles: map[string]string{
					"config.yaml": serviceConfigYaml,
				},
				Command: []string{"benthos", "-c", "config.yaml"},
				Env:     map[string]string{"BENTHOS_LOG_LEVEL": "info"},
			}

			s6ServiceMock.GetLogsResult = []s6service.LogEntry{
				{
					Timestamp: time.Now(),
					Content:   "test log",
				},
			}

			// Verify service is running with initial configuration
			By("Verifying service is running with initial configuration")
			info, err := service.Status(ctx, mockFS, benthosName, initialConfig.MetricsPort, tick, time.Now())
			Expect(err).NotTo(HaveOccurred())
			Expect(info.BenthosStatus.HealthCheck.IsLive).To(BeTrue())
			Expect(info.BenthosStatus.HealthCheck.IsReady).To(BeTrue())

			// Update the service configuration
			By("Updating the Benthos service configuration")
			err = service.UpdateBenthosInS6Manager(ctx, mockFS, updatedConfig, benthosName)
			Expect(err).NotTo(HaveOccurred())

			// Second reconciliation - detects the config change and triggers restart
			By("Reconciling the manager to apply the configuration change")
			tick++
			err, _ = service.ReconcileManager(ctx, mockFS, tick)
			Expect(err).NotTo(HaveOccurred())

			// Change the mock to return the updated config
			updatedServiceConfigYaml, err := service.generateBenthosYaml(updatedConfig)
			Expect(err).NotTo(HaveOccurred())

			// Create a valid YAML string that can be parsed by yaml.Unmarshal
			validYaml := `
input:
  mqtt:
    urls:
      - tcp://localhost:1883
    topics:
      - updated-topic
output:
  stdout: {}
metrics:
  http:
    address: ":4195"
logger:
  level: info
`

			// Update mock service to return the new config
			s6ServiceMock.GetConfigResult = s6serviceconfig.S6ServiceConfig{
				ConfigFiles: map[string]string{
					"config.yaml": updatedServiceConfigYaml,
				},
				Command: []string{"benthos", "-c", "config.yaml"},
				Env:     map[string]string{"BENTHOS_LOG_LEVEL": "info"},
			}
			s6ServiceMock.GetS6ConfigFileResult = []byte(validYaml)

			// Verify the service is running with new configuration
			By("Verifying service is running with updated configuration")
			info, err = service.Status(ctx, mockFS, benthosName, updatedConfig.MetricsPort, tick, time.Now())
			Expect(err).NotTo(HaveOccurred())
			Expect(info.BenthosStatus.HealthCheck.IsLive).To(BeTrue())
			Expect(info.BenthosStatus.HealthCheck.IsReady).To(BeTrue())

			// Get the config via the service and validate it reflects the updated configuration
			cfg, err := service.GetConfig(ctx, mockFS, benthosName)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Input).NotTo(BeNil(), "Config Input should not be nil")
			Expect(cfg.Input["mqtt"]).NotTo(BeNil(), "MQTT config should not be nil")

			// Ensure we're accessing the MQTT config safely, with nil checks
			if mqttConfig, ok := cfg.Input["mqtt"].(map[string]interface{}); ok {
				Expect(mqttConfig).NotTo(BeNil(), "MQTT config should be a valid map")

				if topicsInterface, ok := mqttConfig["topics"]; ok {
					if topics, ok := topicsInterface.([]interface{}); ok {
						Expect(len(topics)).To(Equal(1), "Should have one topic")
						if topicStr, ok := topics[0].(string); ok {
							Expect(topicStr).To(Equal("updated-topic"), "The configuration should be updated to the new topic")
						} else {
							Fail("Topic should be a string")
						}
					} else {
						Fail("Topics should be an array")
					}
				} else {
					Fail("Topics key should exist in MQTT config")
				}
			} else {
				Fail("MQTT config should be a map")
			}
		})
	})

	Context("Instance status assessment", func() {
		var (
			service                   *BenthosService
			mockS6Service             *s6service.MockService
			benthosMonitorMockService *benthos_monitor.MockBenthosMonitorService
			ctx                       context.Context
			benthosName               string
			s6ServiceName             string
			currentTime               time.Time
			logWindow                 time.Duration
			mockFS                    *filesystem.MockFileSystem
		)

		BeforeEach(func() {
			ctx = context.Background()
			mockS6Service = s6service.NewMockService()
			benthosMonitorMockService = benthos_monitor.NewMockBenthosMonitorService()
			benthosMonitorMockService.SetLiveStatus(true)
			benthosMonitorMockService.SetReadyStatus(true, true, "")
			benthosMonitorMockService.SetMetricsResponse(benthos_monitor.Metrics{
				Input: benthos_monitor.InputMetrics{
					Received: 100,
				},
				Output: benthos_monitor.OutputMetrics{
					Sent: 100,
				},
			})
			benthosMonitorMockService.SetBenthosMonitorRunning()
			benthosName = "test-instance"
			currentTime = time.Now()
			logWindow = 5 * time.Minute
			mockFS = filesystem.NewMockFileSystem()
			service = NewDefaultBenthosService(benthosName,
				WithS6Service(mockS6Service),
				WithMonitorService(benthosMonitorMockService),
			)

			s6ServiceName = service.getS6ServiceName(benthosName)

			mockS6Service.ExistingServices[s6ServiceName] = true
			mockS6Service.ServiceExistsResult = true

			// Add the service to the S6 manager
			err := service.AddBenthosToS6Manager(ctx, mockFS, &benthosserviceconfig.BenthosServiceConfig{
				MetricsPort: 4195,
				LogLevel:    "info",
			}, benthosName)
			Expect(err).NotTo(HaveOccurred())

			// Reconcile the S6 manager
			err, _ = service.ReconcileManager(ctx, mockFS, 0)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should identify an instance as bad when logs contain errors", func() {
			// Prepare error logs that would make an instance bad
			errorLogs := []s6service.LogEntry{
				{
					Timestamp: currentTime.Add(-1 * time.Minute),
					Content:   `level=error msg="failed to connect to broker"`,
				},
			}

			// Set up the mock to return these logs
			mockS6Service.GetLogsResult = errorLogs

			// Setup S6 service status to reflect a running service
			mockS6Service.StatusResult = s6service.ServiceInfo{
				Status:   s6service.ServiceUp,
				WantUp:   true,
				Pid:      12345,
				Uptime:   60, // Running for a minute
				ExitCode: 0,
			}

			// Get the service status (which includes the logs)
			info, err := service.Status(ctx, mockFS, benthosName, 4195, 1, time.Now())
			Expect(err).NotTo(HaveOccurred())

			// Check if logs are fine - this should return false with our error logs
			isLogsFine := service.IsLogsFine(info.BenthosStatus.BenthosLogs, currentTime, logWindow)
			Expect(isLogsFine).To(BeFalse(), "Service with error logs should be identified as having issues")
		})

		It("should identify different types of errors in the logs", func() {
			// Test various error types
			errorCases := []struct {
				description string
				logs        []s6service.LogEntry
				expectBad   bool
			}{
				{
					description: "Official Benthos error logs",
					logs: []s6service.LogEntry{
						{
							Timestamp: currentTime.Add(-1 * time.Minute),
							Content:   `level=error msg="failed to connect to broker"`,
						},
					},
					expectBad: true,
				},
				{
					description: "Configuration file errors",
					logs: []s6service.LogEntry{
						{
							Timestamp: currentTime.Add(-1 * time.Minute),
							Content:   `configuration file read error: invalid syntax in line 10`,
						},
					},
					expectBad: true,
				},
				{
					description: "Logger creation errors",
					logs: []s6service.LogEntry{
						{
							Timestamp: currentTime.Add(-1 * time.Minute),
							Content:   `failed to create logger: invalid log level "unknown"`,
						},
					},
					expectBad: true,
				},
				{
					description: "Linter errors",
					logs: []s6service.LogEntry{
						{
							Timestamp: currentTime.Add(-1 * time.Minute),
							Content:   `Config lint error: unknown input type "not_real_input"`,
						},
					},
					expectBad: true,
				},
				{
					description: "Critical warnings",
					logs: []s6service.LogEntry{
						{
							Timestamp: currentTime.Add(-1 * time.Minute),
							Content:   `level=warn msg="failed to process message batch"`,
						},
					},
					expectBad: true,
				},
				{
					description: "Non-critical warnings",
					logs: []s6service.LogEntry{
						{
							Timestamp: currentTime.Add(-1 * time.Minute),
							Content:   `level=warn msg="rate limit applied"`,
						},
					},
					expectBad: false,
				},
				{
					description: "Mixed logs with one error",
					logs: []s6service.LogEntry{
						{
							Timestamp: currentTime.Add(-1 * time.Minute),
							Content:   `level=info msg="Service started"`,
						},
						{
							Timestamp: currentTime.Add(-2 * time.Minute),
							Content:   `level=warn msg="rate limit applied"`,
						},
						{
							Timestamp: currentTime.Add(-3 * time.Minute),
							Content:   `level=error msg="connection failed"`,
						},
					},
					expectBad: true,
				},
				{
					description: "Error outside time window",
					logs: []s6service.LogEntry{
						{
							Timestamp: currentTime.Add(-10 * time.Minute), // Outside our 5-minute window
							Content:   `level=error msg="connection failed"`,
						},
					},
					expectBad: false,
				},
			}

			for _, testCase := range errorCases {
				By(testCase.description)
				// Configure the mock with the test logs
				mockS6Service.GetLogsResult = testCase.logs

				// Get status with the test logs
				info, err := service.Status(ctx, mockFS, benthosName, 4195, 1, time.Now())
				Expect(err).NotTo(HaveOccurred())

				// Check logs status
				isLogsFine := service.IsLogsFine(info.BenthosStatus.BenthosLogs, currentTime, logWindow)
				if testCase.expectBad {
					Expect(isLogsFine).To(BeFalse(), "Should detect problems in logs")
				} else {
					Expect(isLogsFine).To(BeTrue(), "Should not detect problems in logs")
				}
			}
		})

		It("should check logs in ServiceInfo when determining instance health", func() {
			// Set up a ServiceInfo with error logs
			errorLogs := []s6service.LogEntry{
				{
					Timestamp: currentTime.Add(-1 * time.Minute),
					Content:   `level=error msg="failed to process message"`,
				},
			}

			// Mock the S6 service to return these logs
			mockS6Service.GetLogsResult = errorLogs

			// Configure the client to return healthy metrics/status
			benthosMonitorMockService.SetReadyStatus(true, true, "")

			// Set up metrics with no errors
			benthosMonitorMockService.SetMetricsResponse(benthos_monitor.Metrics{
				Input: benthos_monitor.InputMetrics{
					Received: 100,
				},
				Output: benthos_monitor.OutputMetrics{
					Sent:  100,
					Error: 0,
				},
			})

			// Get the service status
			info, err := service.Status(ctx, mockFS, benthosName, 4195, 1, time.Now())
			Expect(err).NotTo(HaveOccurred())

			// Check if logs are fine
			isLogsFine := service.IsLogsFine(info.BenthosStatus.BenthosLogs, currentTime, logWindow)
			Expect(isLogsFine).To(BeFalse(), "Service with error logs should be identified as having issues")

			// Verify that the instance health is properly reflected
			// A real implementation might use the FSM's `IsBenthosDegraded` method, but for our test
			// we'll directly check `IsLogsFine` since it's part of the "bad" assessment
			isBad := !isLogsFine
			Expect(isBad).To(BeTrue(), "Instance with error logs should be considered bad")
		})
	})

	Describe("ForceRemoveBenthos", func() {
		var (
			service       *BenthosService
			mockS6Service *s6service.MockService
			mockFS        *filesystem.MockFileSystem
			benthosName   string
		)

		BeforeEach(func() {
			mockS6Service = s6service.NewMockService()
			mockFS = filesystem.NewMockFileSystem()
			benthosName = "test-benthos"
			service = NewDefaultBenthosService(benthosName, WithS6Service(mockS6Service))
		})

		It("should call S6 ForceRemove with the correct service path", func() {
			// Call ForceRemoveBenthos
			err := service.ForceRemoveBenthos(context.Background(), mockFS, benthosName)

			// Verify no error
			Expect(err).NotTo(HaveOccurred())

			// Verify S6Service ForceRemove was called
			Expect(mockS6Service.ForceRemoveCalled).To(BeTrue())

			// Verify the path is correct
			expectedS6ServiceName := service.getS6ServiceName(benthosName)
			expectedS6ServicePath := filepath.Join(constants.S6BaseDir, expectedS6ServiceName)
			Expect(mockS6Service.ForceRemovePath).To(Equal(expectedS6ServicePath))
		})

		It("should propagate errors from S6 service", func() {
			// Set up mock to return an error
			mockError := fmt.Errorf("mock force remove error")
			mockS6Service.ForceRemoveError = mockError

			// Call ForceRemoveBenthos
			err := service.ForceRemoveBenthos(context.Background(), mockFS, benthosName)

			// Verify error is propagated
			Expect(err).To(MatchError(mockError))
		})
	})

	Describe("Benthos ⇄ Benthos-monitor coordination", func() {
		var (
			service                   *BenthosService
			mockS6Manager             *s6fsm.S6Manager
			tick                      uint64
			benthosName               string
			mockFS                    *filesystem.MockFileSystem
			benthosMonitorMockService *benthos_monitor.MockBenthosMonitorService
			ctx                       context.Context
			cancel                    context.CancelFunc
		)

		BeforeEach(func() {
			benthosName = "test-benthos"
			ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(100*time.Minute))
			benthosMonitorMockService = benthos_monitor.NewMockBenthosMonitorService()
			mockS6Manager = s6fsm.NewS6ManagerWithMockedServices(benthosName)
			service = NewDefaultBenthosService(benthosName, WithMonitorService(benthosMonitorMockService), WithS6Manager(mockS6Manager))
			tick = 0

			mockFS = filesystem.NewMockFileSystem()
			// Add the service to the S6 manager
			err := service.AddBenthosToS6Manager(ctx, mockFS, &benthosserviceconfig.BenthosServiceConfig{
				MetricsPort: 4195,
				LogLevel:    "info",
			}, benthosName)
			Expect(err).NotTo(HaveOccurred())

			// Reconcile the S6 and benthos monitor manager
			err, _ = service.ReconcileManager(ctx, mockFS, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(benthosMonitorMockService.ReconcileManagerCalled).To(BeTrue())
			Expect(benthosMonitorMockService.S6ServiceConfig.DesiredFSMState).To(Equal(s6fsm.OperationalStateRunning))
			Expect(benthosMonitorMockService.AddBenthosToS6ManagerCalled).To(BeTrue())

			for i := 0; i < 10; i++ {
				err, _ = service.ReconcileManager(ctx, mockFS, tick)
				Expect(err).NotTo(HaveOccurred())
				tick++
			}
		})

		AfterEach(func() {
			cancel()
		})

		When("the instance is stopped and started again", func() {
			It("stops and restarts the monitor too", func() {
				err := service.StopBenthos(ctx, mockFS, benthosName)
				Expect(err).NotTo(HaveOccurred())
				Expect(benthosMonitorMockService.StopBenthosCalled).To(BeTrue())

				err = service.StartBenthos(ctx, mockFS, benthosName)
				Expect(err).NotTo(HaveOccurred())
				Expect(benthosMonitorMockService.StartBenthosCalled).To(BeTrue())
			})
		})

		When("the Benthos config changes", func() {
			It("regenerates the monitor’s S6 script", func() {
				err := service.UpdateBenthosInS6Manager(ctx, mockFS, &benthosserviceconfig.BenthosServiceConfig{
					MetricsPort: 9123,
					LogLevel:    "info",
				}, benthosName)
				Expect(err).NotTo(HaveOccurred())
				Expect(benthosMonitorMockService.UpdateBenthosMonitorInS6ManagerCalled).To(BeTrue())

				// Expect that the metrics port was updated
				Expect(benthosMonitorMockService.UpdateLastPort).To(Equal(uint16(9123)))
			})
		})
	})

})
