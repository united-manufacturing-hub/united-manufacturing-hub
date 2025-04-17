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

package nmap

import (
	"context"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Nmap Service", func() {
	var (
		service   *NmapService
		mockS6    *s6service.MockService
		tick      uint64
		nmapName  string
		s6Service string
		mockFS    *filesystem.MockFileSystem
	)

	BeforeEach(func() {
		mockS6 = s6service.NewMockService()
		nmapName = "test-scan"
		service = NewDefaultNmapService(nmapName, WithS6Service(mockS6))
		tick = 0
		s6Service = service.getS6ServiceName(nmapName)
		mockFS = filesystem.NewMockFileSystem()
	})

	Describe("Script Generation", func() {
		Context("with valid config", func() {
			It("should generate a valid shell script", func() {
				config := &nmapserviceconfig.NmapServiceConfig{
					Target: "localhost",
					Port:   80,
				}

				script, err := service.generateNmapScript(config)
				Expect(err).NotTo(HaveOccurred())
				Expect(script).To(ContainSubstring("#!/bin/sh"))
				Expect(script).To(ContainSubstring("nmap -n -Pn -p 80 localhost -v"))
				Expect(script).To(ContainSubstring("NMAP_SCAN_START"))
				Expect(script).To(ContainSubstring("NMAP_TIMESTAMP"))
				Expect(script).To(ContainSubstring("NMAP_EXIT_CODE"))
				Expect(script).To(ContainSubstring("NMAP_DURATION"))
				Expect(script).To(ContainSubstring("sleep 1"))
			})
		})

		Context("with nil config", func() {
			It("should return an error", func() {
				script, err := service.generateNmapScript(nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("config is nil"))
				Expect(script).To(Equal(""))
			})
		})
	})

	Describe("Config Management", func() {
		Context("when adding a new service", func() {
			It("should add the service to S6 manager", func() {
				ctx := context.Background()
				cfg := &nmapserviceconfig.NmapServiceConfig{
					Target: "example.com",
					Port:   443,
				}

				err := service.AddNmapToS6Manager(ctx, cfg, nmapName)
				Expect(err).NotTo(HaveOccurred())

				// Check if config is added to s6ServiceConfigs
				Expect(service.s6ServiceConfigs).To(HaveLen(1))
				Expect(service.s6ServiceConfigs[0].Name).To(Equal(s6Service))
			})

			It("should return error when service already exists", func() {
				ctx := context.Background()
				cfg := &nmapserviceconfig.NmapServiceConfig{
					Target: "example.com",
					Port:   443,
				}

				// Add service first time
				err := service.AddNmapToS6Manager(ctx, cfg, nmapName)
				Expect(err).NotTo(HaveOccurred())

				// Try to add again with same name
				err = service.AddNmapToS6Manager(ctx, cfg, nmapName)
				Expect(err).To(Equal(ErrServiceAlreadyExists))
			})
		})

		Context("when updating a service", func() {
			BeforeEach(func() {
				ctx := context.Background()
				cfg := &nmapserviceconfig.NmapServiceConfig{
					Target: "example.com",
					Port:   443,
				}

				err := service.AddNmapToS6Manager(ctx, cfg, nmapName)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should update the service config", func() {
				ctx := context.Background()
				updatedCfg := &nmapserviceconfig.NmapServiceConfig{
					Target: "example.com",
					Port:   8080, // Changed port
				}

				err := service.UpdateNmapInS6Manager(ctx, updatedCfg, nmapName)
				Expect(err).NotTo(HaveOccurred())

				// Verify the config was updated
				s6cfg, err := service.GenerateS6ConfigForNmap(updatedCfg, s6Service)
				Expect(err).NotTo(HaveOccurred())
				Expect(s6cfg.ConfigFiles["run_nmap.sh"]).To(ContainSubstring("nmap -n -Pn -p 8080 example.com -v"))
			})

			It("should return error when service doesn't exist", func() {
				ctx := context.Background()
				updatedCfg := &nmapserviceconfig.NmapServiceConfig{
					Target: "example.com",
					Port:   8080,
				}

				err := service.UpdateNmapInS6Manager(ctx, updatedCfg, "nonexistent")
				Expect(err).To(Equal(ErrServiceNotExist))
			})
		})

		Context("when removing a service", func() {
			BeforeEach(func() {
				ctx := context.Background()
				cfg := &nmapserviceconfig.NmapServiceConfig{
					Target: "example.com",
					Port:   443,
				}

				err := service.AddNmapToS6Manager(ctx, cfg, nmapName)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should remove the service from S6 manager", func() {
				ctx := context.Background()
				err := service.RemoveNmapFromS6Manager(ctx, nmapName)
				Expect(err).NotTo(HaveOccurred())
				Expect(service.s6ServiceConfigs).To(BeEmpty())
			})

			It("should return error when service doesn't exist", func() {
				ctx := context.Background()
				err := service.RemoveNmapFromS6Manager(ctx, "nonexistent")
				Expect(err).To(Equal(ErrServiceNotExist))
			})
		})
	})

	Describe("Config Extraction", func() {
		Context("when getting config from S6 service", func() {
			It("should extract config from script content", func() {
				ctx := context.Background()
				scriptContent := `#!/bin/sh
while true; do
  echo "NMAP_SCAN_START"
  echo "NMAP_TIMESTAMP: $(date -Iseconds)"
  SCAN_START=$(date +%s.%N)
  echo "NMAP_COMMAND: nmap -n -Pn -p 8080 test-host.local -v"
  nmap -n -Pn -p 8080 test-host.local -v
  EXIT_CODE=$?
  SCAN_END=$(date +%s.%N)
  SCAN_DURATION=$(echo "$SCAN_END - $SCAN_START" | bc)
  echo "NMAP_EXIT_CODE: $EXIT_CODE"
  echo "NMAP_DURATION: $SCAN_DURATION"
  echo "NMAP_SCAN_END"
  sleep 1
done`

				// Setup mock response
				mockS6.GetS6ConfigFileResult = []byte(scriptContent)
				mockS6.ServiceExistsResult = true

				config, err := service.GetConfig(ctx, mockFS, nmapName)
				Expect(err).NotTo(HaveOccurred())
				Expect(config.Target).To(Equal("test-host.local"))
				Expect(config.Port).To(Equal(8080))
			})

			It("should return error on context cancellation", func() {
				ctx, cancel := context.WithCancel(context.Background())
				cancel() // Cancel immediately

				_, err := service.GetConfig(ctx, mockFS, nmapName)
				Expect(err).To(Equal(context.Canceled))
			})
		})
	})

	Describe("Log Parsing", func() {
		Context("with valid nmap scan logs", func() {
			It("should parse scan results correctly", func() {
				logs := []s6service.LogEntry{
					{
						Timestamp: time.Now(),
						Content:   "NMAP_SCAN_START",
					},
					{
						Timestamp: time.Now(),
						Content:   "NMAP_TIMESTAMP: 2023-04-01T12:34:56+00:00",
					},
					{
						Timestamp: time.Now(),
						Content:   "NMAP_COMMAND: nmap -n -Pn -p 80 example.com -v",
					},
					{
						Timestamp: time.Now(),
						Content:   "Starting Nmap 7.92 ( https://nmap.org ) at 2023-04-01 12:34 UTC",
					},
					{
						Timestamp: time.Now(),
						Content:   "Scanning example.com (93.184.216.34) [1 port]",
					},
					{
						Timestamp: time.Now(),
						Content:   "Completed SYN Stealth Scan at 12:34, 0.05s elapsed (1 total ports)",
					},
					{
						Timestamp: time.Now(),
						Content:   "Nmap scan report for example.com (93.184.216.34)",
					},
					{
						Timestamp: time.Now(),
						Content:   "Host is up (0.045s latency).",
					},
					{
						Timestamp: time.Now(),
						Content:   "PORT   STATE SERVICE",
					},
					{
						Timestamp: time.Now(),
						Content:   "80/tcp open  http",
					},
					{
						Timestamp: time.Now(),
						Content:   "Read data files from: /usr/bin/../share/nmap",
					},
					{
						Timestamp: time.Now(),
						Content:   "Nmap done: 1 IP address (1 host up) scanned in 0.10 seconds",
					},
					{
						Timestamp: time.Now(),
						Content:   "           Raw packets sent: 1 (44B) | Rcvd: 1 (44B)",
					},
					{
						Timestamp: time.Now(),
						Content:   "NMAP_EXIT_CODE: 0",
					},
					{
						Timestamp: time.Now(),
						Content:   "NMAP_DURATION: .102345",
					},
					{
						Timestamp: time.Now(),
						Content:   "NMAP_SCAN_END",
					},
				}

				result := service.parseScanLogs(logs, 80)
				Expect(result).NotTo(BeNil())
				Expect(result.PortResult.Port).To(Equal(80))
				Expect(result.PortResult.State).To(Equal("open"))
				Expect(result.Metrics.ScanDuration).To(Equal(0.102345))
				Expect(result.Timestamp.Format(time.RFC3339)).To(Equal("2023-04-01T12:34:56Z"))
			})

			It("should handle closed ports correctly", func() {
				logs := []s6service.LogEntry{
					{
						Timestamp: time.Now(),
						Content:   "NMAP_SCAN_START",
					},
					{
						Timestamp: time.Now(),
						Content:   "NMAP_TIMESTAMP: 2023-04-01T12:34:56+00:00",
					},
					{
						Timestamp: time.Now(),
						Content:   "NMAP_COMMAND: nmap -n -Pn -p 81 example.com -v",
					},
					{
						Timestamp: time.Now(),
						Content:   "Nmap scan report for example.com (93.184.216.34)",
					},
					{
						Timestamp: time.Now(),
						Content:   "Host is up (0.045s latency).",
					},
					{
						Timestamp: time.Now(),
						Content:   "PORT   STATE  SERVICE",
					},
					{
						Timestamp: time.Now(),
						Content:   "81/tcp closed http-alt",
					},
					{
						Timestamp: time.Now(),
						Content:   "NMAP_EXIT_CODE: 0",
					},
					{
						Timestamp: time.Now(),
						Content:   "NMAP_DURATION: .102345",
					},
					{
						Timestamp: time.Now(),
						Content:   "NMAP_SCAN_END",
					},
				}

				result := service.parseScanLogs(logs, 81)
				Expect(result).NotTo(BeNil())
				Expect(result.PortResult.Port).To(Equal(81))
				Expect(result.PortResult.State).To(Equal("closed"))
			})

			It("should return nil for empty logs", func() {
				result := service.parseScanLogs([]s6service.LogEntry{}, 80)
				Expect(result).To(BeNil())
			})

			It("should return nil for incomplete scan logs", func() {
				logs := []s6service.LogEntry{
					{
						Timestamp: time.Now(),
						Content:   "NMAP_SCAN_START",
					},
					{
						Timestamp: time.Now(),
						Content:   "NMAP_TIMESTAMP: 2023-04-01T12:34:56+00:00",
					},
					// Missing NMAP_SCAN_END
				}

				result := service.parseScanLogs(logs, 80)
				Expect(result).To(BeNil())
			})
		})
	})

	Describe("Service Status", func() {
		BeforeEach(func() {
			ctx := context.Background()
			cfg := &nmapserviceconfig.NmapServiceConfig{
				Target: "example.com",
				Port:   443,
			}

			// Add and reconcile service
			err := service.AddNmapToS6Manager(ctx, cfg, nmapName)
			Expect(err).NotTo(HaveOccurred())
			err, _ = service.ReconcileManager(ctx, mockFS, tick)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when service doesn't exist", func() {
			It("should return the appropriate error", func() {
				ctx := context.Background()
				mockS6.ServiceExistsResult = false

				_, err := service.Status(ctx, mockFS, "nonexistent", tick)
				Expect(err).To(Equal(ErrServiceNotExist))
			})
		})
	})

	Describe("Service Control", func() {
		BeforeEach(func() {
			ctx := context.Background()
			cfg := &nmapserviceconfig.NmapServiceConfig{
				Target: "example.com",
				Port:   443,
			}

			// Add and reconcile service
			err := service.AddNmapToS6Manager(ctx, cfg, nmapName)
			Expect(err).NotTo(HaveOccurred())
			err, _ = service.ReconcileManager(ctx, mockFS, tick)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when starting a service", func() {
			It("should set the desired state to running", func() {
				ctx := context.Background()

				// First, stop the service to ensure we're testing the change
				err := service.StopNmap(ctx, nmapName)
				Expect(err).NotTo(HaveOccurred())
				Expect(service.s6ServiceConfigs[0].DesiredFSMState).To(Equal("stopped"))

				// Now start it
				err = service.StartNmap(ctx, nmapName)
				Expect(err).NotTo(HaveOccurred())
				Expect(service.s6ServiceConfigs[0].DesiredFSMState).To(Equal("running"))
			})
		})

		Context("when stopping a service", func() {
			It("should set the desired state to stopped", func() {
				ctx := context.Background()

				// The service starts with running as default
				Expect(service.s6ServiceConfigs[0].DesiredFSMState).To(Equal("running"))

				// Stop it
				err := service.StopNmap(ctx, nmapName)
				Expect(err).NotTo(HaveOccurred())
				Expect(service.s6ServiceConfigs[0].DesiredFSMState).To(Equal("stopped"))
			})
		})
	})
})
