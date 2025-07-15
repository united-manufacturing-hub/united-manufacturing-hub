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
	"fmt"
	"path/filepath"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/process_shared"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// parseLogOutput converts raw log output (like from the current log file) into []s6service.LogEntry
// Input format: "2025-05-27 09:29:59.576902049  NMAP_SCAN_START"
// Just copy-paste the actual log lines and this will convert them to the test format
func parseLogOutput(rawLogs string) []process_shared.LogEntry {
	// Use the existing optimized s6 parsing function
	entries, err := process_shared.ParseLogsFromBytes([]byte(rawLogs))
	if err != nil {
		// Fallback to empty slice if parsing fails
		return []process_shared.LogEntry{}
	}
	return entries
}

var _ = Describe("Nmap Service", func() {
	var (
		service         *NmapService
		mockS6          *process_shared.MockService
		tick            uint64
		nmapName        string
		s6Service       string
		mockSvcRegistry *serviceregistry.Registry
	)

	BeforeEach(func() {
		mockS6 = process_shared.NewMockService()
		nmapName = "test-scan"
		service = NewDefaultNmapService(nmapName, WithS6Service(mockS6))
		tick = 0
		s6Service = service.getS6ServiceName(nmapName)
		mockSvcRegistry = serviceregistry.NewMockRegistry()
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

			// Note: removing a non-existent component should not result in an error
			// the remove action will be called multiple times until the component is gone it returns nil
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

				config, err := service.GetConfig(ctx, mockSvcRegistry.GetFileSystem(), nmapName)
				Expect(err).NotTo(HaveOccurred())
				Expect(config.Target).To(Equal("test-host.local"))
				Expect(config.Port).To(Equal(uint16(8080)))
			})

			It("should return error on context cancellation", func() {
				ctx, cancel := context.WithCancel(context.Background())
				cancel() // Cancel immediately

				_, err := service.GetConfig(ctx, mockSvcRegistry.GetFileSystem(), nmapName)
				Expect(err).To(Equal(context.Canceled))
			})
		})
	})

	Describe("Log Parsing", func() {
		Context("with valid nmap scan logs", func() {
			It("should parse scan results correctly", func() {
				logs := []process_shared.LogEntry{
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
				Expect(result.PortResult.Port).To(Equal(uint16(80)))
				Expect(result.PortResult.State).To(Equal("open"))
				Expect(result.Metrics.ScanDuration).To(Equal(0.102345))
				Expect(result.Timestamp.Format(time.RFC3339)).To(Equal("2023-04-01T12:34:56Z"))
			})

			It("should parse scan results correctly using helper", func() {
				rawLogs := `2025-05-27 09:29:59.576902049  NMAP_SCAN_START
2025-05-27 09:29:59.577869966  NMAP_TIMESTAMP: 2025-05-27T09:29:59+00:00
2025-05-27 09:29:59.578415299  NMAP_COMMAND: nmap -n -Pn -p 8080 localhost -v
2025-05-27 09:29:59.582778091  Starting Nmap 7.95 ( https://nmap.org ) at 2025-05-27 09:29 UTC
2025-05-27 09:29:59.619361132  Initiating Connect Scan at 09:29
2025-05-27 09:29:59.619495299  Scanning localhost (127.0.0.1) [1 port]
2025-05-27 09:29:59.619496716  Discovered open port 8080/tcp on 127.0.0.1
2025-05-27 09:29:59.619503841  Completed Connect Scan at 09:29, 0.00s elapsed (1 total ports)
2025-05-27 09:29:59.619507799  Nmap scan report for localhost (127.0.0.1)
2025-05-27 09:29:59.619549341  Host is up (0.00011s latency).
2025-05-27 09:29:59.619550132  Other addresses for localhost (not scanned): ::1
2025-05-27 09:29:59.619550382  
2025-05-27 09:29:59.619550799  PORT     STATE SERVICE
2025-05-27 09:29:59.619551216  8080/tcp open  http-proxy
2025-05-27 09:29:59.619551466  
2025-05-27 09:29:59.619653591  Read data files from: /usr/bin/../share/nmap
2025-05-27 09:29:59.619654632  Nmap done: 1 IP address (1 host up) scanned in 0.04 seconds
2025-05-27 09:29:59.623002007  NMAP_EXIT_CODE: 0
2025-05-27 09:29:59.623003591  NMAP_DURATION: 0
2025-05-27 09:29:59.623004091  NMAP_SCAN_END`

				logs := parseLogOutput(rawLogs)

				result := service.parseScanLogs(logs, 8080)
				Expect(result).NotTo(BeNil())
				Expect(result.PortResult.Port).To(Equal(uint16(8080)))
				Expect(result.PortResult.State).To(Equal("open"))
				Expect(result.Metrics.ScanDuration).To(Equal(0.0))
				Expect(result.Timestamp.Format(time.RFC3339)).To(Equal("2025-05-27T09:29:59Z"))
			})

			It("should parse scan results correctly with multiple scans", func() {
				rawLogs := `2025-05-27 11:34:24.541823586  Read data files from: /usr/bin/../share/nmap
2025-05-27 11:34:24.541824086  Nmap done: 1 IP address (1 host up) scanned in 0.03 seconds
2025-05-27 11:34:24.545440419  NMAP_EXIT_CODE: 0
2025-05-27 11:34:24.545443252  NMAP_DURATION: 0
2025-05-27 11:34:24.545443669  NMAP_SCAN_END
2025-05-27 11:34:25.548278878  NMAP_SCAN_START
2025-05-27 11:34:25.549245169  NMAP_TIMESTAMP: 2025-05-27T11:34:25+00:00
2025-05-27 11:34:25.549834961  NMAP_COMMAND: nmap -n -Pn -p 8080 localhost -v
2025-05-27 11:34:25.556032003  Starting Nmap 7.95 ( https://nmap.org ) at 2025-05-27 11:34 UTC
2025-05-27 11:34:25.593831253  Initiating Connect Scan at 11:34
2025-05-27 11:34:25.594110253  Scanning localhost (127.0.0.1) [1 port]
2025-05-27 11:34:25.594113086  Discovered open port 8080/tcp on 127.0.0.1
2025-05-27 11:34:25.594114044  Completed Connect Scan at 11:34, 0.00s elapsed (1 total ports)
2025-05-27 11:34:25.594114794  Nmap scan report for localhost (127.0.0.1)
2025-05-27 11:34:25.594115419  Host is up (0.00011s latency).
2025-05-27 11:34:25.594116086  Other addresses for localhost (not scanned): ::1
2025-05-27 11:34:25.594116669  
2025-05-27 11:34:25.594117544  PORT     STATE SERVICE
2025-05-27 11:34:25.594118086  8080/tcp open  http-proxy
2025-05-27 11:34:25.594118503  
2025-05-27 11:34:25.594119169  Read data files from: /usr/bin/../share/nmap
2025-05-27 11:34:25.594119878  Nmap done: 1 IP address (1 host up) scanned in 0.04 seconds
2025-05-27 11:34:25.598106336  NMAP_EXIT_CODE: 0
2025-05-27 11:34:25.598109169  NMAP_DURATION: 0
2025-05-27 11:34:25.598109794  NMAP_SCAN_END
2025-05-27 11:34:26.600001795  NMAP_SCAN_START
2025-05-27 11:34:26.601069045  NMAP_TIMESTAMP: 2025-05-27T11:34:26+00:00
2025-05-27 11:34:26.601508920  NMAP_COMMAND: nmap -n -Pn -p 8080 localhost -v
2025-05-27 11:34:26.605778670  Starting Nmap 7.95 ( https://nmap.org ) at 2025-05-27 11:34 UTC
2025-05-27 11:34:26.656555253  Initiating Connect Scan at 11:34
2025-05-27 11:34:26.657079128  Scanning localhost (127.0.0.1) [1 port]
2025-05-27 11:34:26.657080420  Discovered open port 8080/tcp on 127.0.0.1
2025-05-27 11:34:26.657081420  Completed Connect Scan at 11:34, 0.00s elapsed (1 total ports)
2025-05-27 11:34:26.657082087  Nmap scan report for localhost (127.0.0.1)
2025-05-27 11:34:26.657082670  Host is up (0.00030s latency).
2025-05-27 11:34:26.657083378  Other addresses for localhost (not scanned): ::1
2025-05-27 11:34:26.657083795  
2025-05-27 11:34:26.657084378  PORT     STATE SERVICE
2025-05-27 11:34:26.657084920  8080/tcp open  http-proxy
2025-05-27 11:34:26.657085337  
2025-05-27 11:34:26.657085962  Read data files from: /usr/bin/../share/nmap
2025-05-27 11:34:26.657086712  Nmap done: 1 IP address (1 host up) scanned in 0.05 seconds
2025-05-27 11:34:26.676868378  NMAP_EXIT_CODE: 0
2025-05-27 11:34:26.676876170  NMAP_DURATION: 0
2025-05-27 11:34:26.676876878  NMAP_SCAN_END
2025-05-27 11:34:27.680137920  NMAP_SCAN_START
2025-05-27 11:34:27.681254962  NMAP_TIMESTAMP: 2025-05-27T11:34:27+00:00
2025-05-27 11:34:27.682408004  NMAP_COMMAND: nmap -n -Pn -p 8080 localhost -v
2025-05-27 11:34:27.689312920  Starting Nmap 7.95 ( https://nmap.org ) at 2025-05-27 11:34 UTC
2025-05-27 11:34:27.746081045  Initiating Connect Scan at 11:34
2025-05-27 11:34:27.746506129  Scanning localhost (127.0.0.1) [1 port]
2025-05-27 11:34:27.746507629  Discovered open port 8080/tcp on 127.0.0.1
2025-05-27 11:34:27.746529337  Completed Connect Scan at 11:34, 0.00s elapsed (1 total ports)
2025-05-27 11:34:27.746563129  Nmap scan report for localhost (127.0.0.1)
2025-05-27 11:34:27.746878129  Host is up (0.00036s latency).
2025-05-27 11:34:27.746878795  Other addresses for localhost (not scanned): ::1
2025-05-27 11:34:27.746879045  
2025-05-27 11:34:27.746879337  PORT     STATE SERVICE
2025-05-27 11:34:27.746879670  8080/tcp open  http-proxy
2025-05-27 11:34:27.746879837  
2025-05-27 11:34:27.746880170  Read data files from: /usr/bin/../share/nmap
2025-05-27 11:34:27.746880629  Nmap done: 1 IP address (1 host up) scanned in 0.06 seconds
2025-05-27 11:34:27.752365504  NMAP_EXIT_CODE: 0
2025-05-27 11:34:27.752371879  NMAP_DURATION: 0
2025-05-27 11:34:27.752372337  NMAP_SCAN_END
2025-05-27 11:34:28.754622213  NMAP_SCAN_START
2025-05-27 11:34:28.756162879  NMAP_TIMESTAMP: 2025-05-27T11:34:28+00:00
2025-05-27 11:34:28.758028879  NMAP_COMMAND: nmap -n -Pn -p 8080 localhost -v
2025-05-27 11:34:28.764159338  Starting Nmap 7.95 ( https://nmap.org ) at 2025-05-27 11:34 UTC
2025-05-27 11:34:28.806071004  Initiating Connect Scan at 11:34
2025-05-27 11:34:28.806162754  Scanning localhost (127.0.0.1) [1 port]
2025-05-27 11:34:28.806164754  Discovered open port 8080/tcp on 127.0.0.1
2025-05-27 11:34:28.806165671  Completed Connect Scan at 11:34, 0.00s elapsed (1 total ports)
2025-05-27 11:34:28.806196004  Nmap scan report for localhost (127.0.0.1)
2025-05-27 11:34:28.806270754  Host is up (0.000052s latency).
2025-05-27 11:34:28.806271879  Other addresses for localhost (not scanned): ::1
2025-05-27 11:34:28.806272296  
2025-05-27 11:34:28.806272713  PORT     STATE SERVICE
2025-05-27 11:34:28.806273046  8080/tcp open  http-proxy
2025-05-27 11:34:28.806273254  
2025-05-27 11:34:28.806348796  Read data files from: /usr/bin/../share/nmap
2025-05-27 11:34:28.806350171  Nmap done: 1 IP address (1 host up) scanned in 0.04 seconds
2025-05-27 11:34:28.811113129  NMAP_EXIT_CODE: 0
2025-05-27 11:34:28.811116296  NMAP_DURATION: 0
2025-05-27 11:34:28.811116921  NMAP_SCAN_END
2025-05-27 11:34:29.816124546  NMAP_SCAN_START
2025-05-27 11:34:29.817692796  NMAP_TIMESTAMP: 2025-05-27T11:34:29+00:00
2025-05-27 11:34:29.818496213  NMAP_COMMAND: nmap -n -Pn -p 8080 localhost -v
2025-05-27 11:34:29.825612296  Starting Nmap 7.95 ( https://nmap.org ) at 2025-05-27 11:34 UTC
2025-05-27 11:34:29.860800088  Initiating Connect Scan at 11:34
2025-05-27 11:34:29.860994338  Scanning localhost (127.0.0.1) [1 port]
2025-05-27 11:34:29.860995422  Discovered open port 8080/tcp on 127.0.0.1
2025-05-27 11:34:29.860996088  Completed Connect Scan at 11:34, 0.00s elapsed (1 total ports)
2025-05-27 11:34:29.860996588  Nmap scan report for localhost (127.0.0.1)
2025-05-27 11:34:29.860997005  Host is up (0.000084s latency).
2025-05-27 11:34:29.860997505  Other addresses for localhost (not scanned): ::1
2025-05-27 11:34:29.860997755  
2025-05-27 11:34:29.860998172  PORT     STATE SERVICE
2025-05-27 11:34:29.860998547  8080/tcp open  http-proxy
2025-05-27 11:34:29.860998797  
2025-05-27 11:34:29.860999255  Read data files from: /usr/bin/../share/nmap
2025-05-27 11:34:29.860999797  Nmap done: 1 IP address (1 host up) scanned in 0.04 seconds
2025-05-27 11:34:29.864417047  NMAP_EXIT_CODE: 0
2025-05-27 11:34:29.864420005  NMAP_DURATION: 0
2025-05-27 11:34:29.864420713  NMAP_SCAN_END
2025-05-27 11:34:30.867514505  NMAP_SCAN_START
2025-05-27 11:34:30.868255047  NMAP_TIMESTAMP: 2025-05-27T11:34:30+00:00
2025-05-27 11:34:30.868953089  NMAP_COMMAND: nmap -n -Pn -p 8080 localhost -v
2025-05-27 11:34:30.872808297  Starting Nmap 7.95 ( https://nmap.org ) at 2025-05-27 11:34 UTC
2025-05-27 11:34:30.900431505  Initiating Connect Scan at 11:34
2025-05-27 11:34:30.900505089  Scanning localhost (127.0.0.1) [1 port]
2025-05-27 11:34:30.900505880  Discovered open port 8080/tcp on 127.0.0.1
2025-05-27 11:34:30.900506339  Completed Connect Scan at 11:34, 0.00s elapsed (1 total ports)
2025-05-27 11:34:30.900510130  Nmap scan report for localhost (127.0.0.1)
2025-05-27 11:34:30.900542297  Host is up (0.000058s latency).
2025-05-27 11:34:30.900542964  Other addresses for localhost (not scanned): ::1
2025-05-27 11:34:30.900543214  
2025-05-27 11:34:30.900543630  PORT     STATE SERVICE
2025-05-27 11:34:30.900543964  8080/tcp open  http-proxy
2025-05-27 11:34:30.900544172  
2025-05-27 11:34:30.900556005  Read data files from: /usr/bin/../share/nmap
2025-05-27 11:34:30.900556589  Nmap done: 1 IP address (1 host up) scanned in 0.03 seconds
2025-05-27 11:34:30.903293839  NMAP_EXIT_CODE: 0
2025-05-27 11:34:30.903294880  NMAP_DURATION: 0
2025-05-27 11:34:30.903295422  NMAP_SCAN_END
2025-05-27 11:34:31.904449381  NMAP_SCAN_START
2025-05-27 11:34:31.905455839  NMAP_TIMESTAMP: 2025-05-27T11:34:31+00:00
2025-05-27 11:34:31.905992589  NMAP_COMMAND: nmap -n -Pn -p 8080 localhost -v
2025-05-27 11:34:31.910073589  Starting Nmap 7.95 ( https://nmap.org ) at 2025-05-27 11:34 UTC
2025-05-27 11:34:31.949354506  Initiating Connect Scan at 11:34
2025-05-27 11:34:31.949459756  Scanning localhost (127.0.0.1) [1 port]
2025-05-27 11:34:31.949461214  Discovered open port 8080/tcp on 127.0.0.1
2025-05-27 11:34:31.949476381  Completed Connect Scan at 11:34, 0.00s elapsed (1 total ports)
2025-05-27 11:34:31.949477172  Nmap scan report for localhost (127.0.0.1)
2025-05-27 11:34:31.949516297  Host is up (0.000081s latency).
2025-05-27 11:34:31.949518339  Other addresses for localhost (not scanned): ::1
2025-05-27 11:34:31.949519214  
2025-05-27 11:34:31.949519839  PORT     STATE SERVICE
2025-05-27 11:34:31.949520422  8080/tcp open  http-proxy
2025-05-27 11:34:31.949520797  
2025-05-27 11:34:31.949528381  Read data files from: /usr/bin/../share/nmap
2025-05-27 11:34:31.949529089  Nmap done: 1 IP address (1 host up) scanned in 0.04 seconds
2025-05-27 11:34:31.953113131  NMAP_EXIT_CODE: 0
2025-05-27 11:34:31.953114131  NMAP_DURATION: 0
2025-05-27 11:34:31.953114548  NMAP_SCAN_END
2025-05-27 11:34:32.954848298  NMAP_SCAN_START
2025-05-27 11:34:32.956201131  NMAP_TIMESTAMP: 2025-05-27T11:34:32+00:00
2025-05-27 11:34:32.956995298  NMAP_COMMAND: nmap -n -Pn -p 8080 localhost -v
2025-05-27 11:34:32.961301923  Starting Nmap 7.95 ( https://nmap.org ) at 2025-05-27 11:34 UTC
2025-05-27 11:34:32.993875548  Initiating Connect Scan at 11:34
2025-05-27 11:34:32.993918215  Scanning localhost (127.0.0.1) [1 port]
2025-05-27 11:34:32.993918923  Discovered open port 8080/tcp on 127.0.0.1
2025-05-27 11:34:32.993946423  Completed Connect Scan at 11:34, 0.00s elapsed (1 total ports)
2025-05-27 11:34:32.993947173  Nmap scan report for localhost (127.0.0.1)
2025-05-27 11:34:32.993970798  Host is up (0.000055s latency).
2025-05-27 11:34:32.993971256  Other addresses for localhost (not scanned): ::1
2025-05-27 11:34:32.993971506  
2025-05-27 11:34:32.993971798  PORT     STATE SERVICE
2025-05-27 11:34:32.993972090  8080/tcp open  http-proxy
2025-05-27 11:34:32.993972298  
2025-05-27 11:34:32.994054881  Read data files from: /usr/bin/../share/nmap
2025-05-27 11:34:32.994055423  Nmap done: 1 IP address (1 host up) scanned in 0.03 seconds
2025-05-27 11:34:32.996537756  NMAP_EXIT_CODE: 0
2025-05-27 11:34:32.996539006  NMAP_DURATION: 0
2025-05-27 11:34:32.996539256  NMAP_SCAN_END
2025-05-27 11:34:34.000670298  NMAP_SCAN_START
2025-05-27 11:34:34.001160507  NMAP_TIMESTAMP: 2025-05-27T11:34:34+00:00
2025-05-27 11:34:34.002239798  NMAP_COMMAND: nmap -n -Pn -p 8080 localhost -v
2025-05-27 11:34:34.006276090  Starting Nmap 7.95 ( https://nmap.org ) at 2025-05-27 11:34 UTC
2025-05-27 11:34:34.036303007  Initiating Connect Scan at 11:34
2025-05-27 11:34:34.036353048  Scanning localhost (127.0.0.1) [1 port]
2025-05-27 11:34:34.036354215  Discovered open port 8080/tcp on 127.0.0.1
2025-05-27 11:34:34.036354923  Completed Connect Scan at 11:34, 0.00s elapsed (1 total ports)
2025-05-27 11:34:34.036359590  Nmap scan report for localhost (127.0.0.1)
2025-05-27 11:34:34.036389632  Host is up (0.000047s latency).
2025-05-27 11:34:34.036390382  Other addresses for localhost (not scanned): ::1
2025-05-27 11:34:34.036390673  
2025-05-27 11:34:34.036391090  PORT     STATE SERVICE
2025-05-27 11:34:34.036391465  8080/tcp open  http-proxy
2025-05-27 11:34:34.036391715  
2025-05-27 11:34:34.036395465  Read data files from: /usr/bin/../share/nmap
2025-05-27 11:34:34.036396007  Nmap done: 1 IP address (1 host up) scanned in 0.03 seconds
2025-05-27 11:34:34.038972632  NMAP_EXIT_CODE: 0
2025-05-27 11:34:34.038973423  NMAP_DURATION: 0
2025-05-27 11:34:34.038973840  NMAP_SCAN_END
2025-05-27 11:34:35.041277216  NMAP_SCAN_START
2025-05-27 11:34:35.042263174  NMAP_TIMESTAMP: 2025-05-27T11:34:35+00:00
2025-05-27 11:34:35.042762632  NMAP_COMMAND: nmap -n -Pn -p 8080 localhost -v
2025-05-27 11:34:35.046832424  Starting Nmap 7.95 ( https://nmap.org ) at 2025-05-27 11:34 UTC
2025-05-27 11:34:35.080530216  Initiating Connect Scan at 11:34
2025-05-27 11:34:35.080688466  Scanning localhost (127.0.0.1) [1 port]
2025-05-27 11:34:35.080691424  Discovered open port 8080/tcp on 127.0.0.1
2025-05-27 11:34:35.080692341  Completed Connect Scan at 11:34, 0.00s elapsed (1 total ports)
2025-05-27 11:34:35.080693007  Nmap scan report for localhost (127.0.0.1)
2025-05-27 11:34:35.080717507  Host is up (0.00011s latency).
2025-05-27 11:34:35.080718382  Other addresses for localhost (not scanned): ::1
2025-05-27 11:34:35.080718799  
2025-05-27 11:34:35.080719382  PORT     STATE SERVICE
2025-05-27 11:34:35.080719924  8080/tcp open  http-proxy
2025-05-27 11:34:35.080720299  
2025-05-27 11:34:35.080720966  Read data files from: /usr/bin/../share/nmap
2025-05-27 11:34:35.080721674  Nmap done: 1 IP address (1 host up) scanned in 0.03 seconds
2025-05-27 11:34:35.084767507  NMAP_EXIT_CODE: 0
2025-05-27 11:34:35.084770799  NMAP_DURATION: 0
2025-05-27 11:34:35.084771257  NMAP_SCAN_END
2025-05-27 11:34:36.086178716  NMAP_SCAN_START
2025-05-27 11:34:36.086969174  NMAP_TIMESTAMP: 2025-05-27T11:34:36+00:00
2025-05-27 11:34:36.087380591  NMAP_COMMAND: nmap -n -Pn -p 8080 localhost -v
2025-05-27 11:34:36.092114341  Starting Nmap 7.95 ( https://nmap.org ) at 2025-05-27 11:34 UTC
2025-05-27 11:34:36.132607549  Initiating Connect Scan at 11:34
2025-05-27 11:34:36.132764133  Scanning localhost (127.0.0.1) [1 port]
2025-05-27 11:34:36.132765508  Discovered open port 8080/tcp on 127.0.0.1
2025-05-27 11:34:36.132766424  Completed Connect Scan at 11:34, 0.00s elapsed (1 total ports)
2025-05-27 11:34:36.132777591  Nmap scan report for localhost (127.0.0.1)
2025-05-27 11:34:36.132813591  Host is up (0.000067s latency).
2025-05-27 11:34:36.132814424  Other addresses for localhost (not scanned): ::1
2025-05-27 11:34:36.132814716  
2025-05-27 11:34:36.132815091  PORT     STATE SERVICE
2025-05-27 11:34:36.132815466  8080/tcp open  http-proxy
2025-05-27 11:34:36.132815716  
2025-05-27 11:34:36.132832716  Read data files from: /usr/bin/../share/nmap
2025-05-27 11:34:36.132833383  Nmap done: 1 IP address (1 host up) scanned in 0.04 seconds
2025-05-27 11:34:36.135745799  NMAP_EXIT_CODE: 0
2025-05-27 11:34:36.135746549  NMAP_DURATION: 0
2025-05-27 11:34:36.135746966  NMAP_SCAN_END
2025-05-27 11:34:37.137163258  NMAP_SCAN_START
2025-05-27 11:34:37.138189967  NMAP_TIMESTAMP: 2025-05-27T11:34:37+00:00
2025-05-27 11:34:37.139050008  NMAP_COMMAND: nmap -n -Pn -p 8080 localhost -v
2025-05-27 11:34:37.143792633  Starting Nmap 7.95 ( https://nmap.org ) at 2025-05-27 11:34 UTC
2025-05-27 11:34:37.182596550  Initiating Connect Scan at 11:34
2025-05-27 11:34:37.182680550  Scanning localhost (127.0.0.1) [1 port]
2025-05-27 11:34:37.182681925  Discovered open port 8080/tcp on 127.0.0.1
2025-05-27 11:34:37.182690467  Completed Connect Scan at 11:34, 0.00s elapsed (1 total ports)
2025-05-27 11:34:37.182695342  Nmap scan report for localhost (127.0.0.1)
2025-05-27 11:34:37.182729800  Host is up (0.000061s latency).
2025-05-27 11:34:37.182730717  Other addresses for localhost (not scanned): ::1
2025-05-27 11:34:37.182731092  
2025-05-27 11:34:37.182731633  PORT     STATE SERVICE
2025-05-27 11:34:37.182732133  8080/tcp open  http-proxy
2025-05-27 11:34:37.182732467  
2025-05-27 11:34:37.182769633  Read data files from: /usr/bin/../share/nmap
2025-05-27 11:34:37.182770467  Nmap done: 1 IP address (1 host up) scanned in 0.04 seconds
2025-05-27 11:34:37.186297633  NMAP_EXIT_CODE: 0
2025-05-27 11:34:37.186298175  NMAP_DURATION: 0
2025-05-27 11:34:37.186298508  NMAP_SCAN_END
2025-05-27 11:34:38.187599300  NMAP_SCAN_START
2025-05-27 11:34:38.189017759  NMAP_TIMESTAMP: 2025-05-27T11:34:38+00:00
2025-05-27 11:34:38.189682259  NMAP_COMMAND: nmap -n -Pn -p 8080 localhost -v
2025-05-27 11:34:38.195296717  Starting Nmap 7.95 ( https://nmap.org ) at 2025-05-27 11:34 UTC
2025-05-27 11:34:38.235209800  Initiating Connect Scan at 11:34
2025-05-27 11:34:38.235300884  Scanning localhost (127.0.0.1) [1 port]
2025-05-27 11:34:38.235301717  Discovered open port 8080/tcp on 127.0.0.1
2025-05-27 11:34:38.235314884  Completed Connect Scan at 11:34, 0.00s elapsed (1 total ports)
2025-05-27 11:34:38.235315717  Nmap scan report for localhost (127.0.0.1)
2025-05-27 11:34:38.235352634  Host is up (0.000071s latency).
2025-05-27 11:34:38.235353217  Other addresses for localhost (not scanned): ::1
2025-05-27 11:34:38.235353509  
2025-05-27 11:34:38.235353925  PORT     STATE SERVICE
2025-05-27 11:34:38.235354300  8080/tcp open  http-proxy
2025-05-27 11:34:38.235354550  
2025-05-27 11:34:38.235387800  Read data files from: /usr/bin/../share/nmap
2025-05-27 11:34:38.235389300  Nmap done: 1 IP address (1 host up) scanned in 0.04 seconds
2025-05-27 11:34:38.238810134  NMAP_EXIT_CODE: 0
2025-05-27 11:34:38.238811509  NMAP_DURATION: 0
2025-05-27 11:34:38.238811884  NMAP_SCAN_END
2025-05-27 11:34:39.242201343  NMAP_SCAN_START
2025-05-27 11:34:39.243654259  NMAP_TIMESTAMP: 2025-05-27T11:34:39+00:00
2025-05-27 11:34:39.244388634  NMAP_COMMAND: nmap -n -Pn -p 8080 localhost -v
2025-05-27 11:34:39.250140509  Starting Nmap 7.95 ( https://nmap.org ) at 2025-05-27 11:34 UTC
2025-05-27 11:34:39.283769801  Initiating Connect Scan at 11:34
2025-05-27 11:34:39.283852551  Scanning localhost (127.0.0.1) [1 port]
2025-05-27 11:34:39.283854468  Discovered closed port 8080/tcp on 127.0.0.1
2025-05-27 11:34:39.283855759  Completed Connect Scan at 11:34, 0.00s elapsed (1 total ports)
2025-05-27 11:34:39.283866968  Nmap scan report for localhost (127.0.0.1)
2025-05-27 11:34:39.283905426  Host is up (0.000071s latency).
2025-05-27 11:34:39.283906343  Other addresses for localhost (not scanned): ::1
2025-05-27 11:34:39.283906634  
2025-05-27 11:34:39.283907093  PORT     STATE SERVICE
2025-05-27 11:34:39.283907551  8080/tcp closed  http-proxy
2025-05-27 11:34:39.283907884  
2025-05-27 11:34:39.283912968  Read data files from: /usr/bin/../share/nmap
2025-05-27 11:34:39.283913759  Nmap done: 1 IP address (1 host up) scanned in 0.03 seconds
2025-05-27 11:34:39.287081926  NMAP_EXIT_CODE: 0
2025-05-27 11:34:39.287083134  NMAP_DURATION: 0
2025-05-27 11:34:39.287083509  NMAP_SCAN_END
`

				logs := parseLogOutput(rawLogs)

				result := service.parseScanLogs(logs, 8080)
				Expect(result).NotTo(BeNil())
				Expect(result.PortResult.Port).To(Equal(uint16(8080)))
				Expect(result.PortResult.State).To(Equal("closed"))
				Expect(result.Metrics.ScanDuration).To(Equal(0.0))
				Expect(result.Timestamp.Format(time.RFC3339)).To(Equal("2025-05-27T11:34:39Z"))
			})

			It("should handle closed ports correctly", func() {
				logs := []process_shared.LogEntry{
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
				Expect(result.PortResult.Port).To(Equal(uint16(81)))
				Expect(result.PortResult.State).To(Equal("closed"))
			})

			It("should return nil for empty logs", func() {
				result := service.parseScanLogs([]process_shared.LogEntry{}, 80)
				Expect(result).To(BeNil())
			})

			It("should return nil for incomplete scan logs", func() {
				logs := []process_shared.LogEntry{
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
			err, _ = service.ReconcileManager(ctx, mockSvcRegistry, tick)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when service doesn't exist", func() {
			It("should return the appropriate error", func() {
				ctx := context.Background()
				mockS6.ServiceExistsResult = false

				_, err := service.Status(ctx, mockSvcRegistry.GetFileSystem(), "nonexistent", tick)
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
			err, _ = service.ReconcileManager(ctx, mockSvcRegistry, tick)
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

	Describe("ForceRemoveNmap", func() {
		var (
			service       *NmapService
			mockS6Service *process_shared.MockService
			mockServices  *serviceregistry.Registry
			nmapName      string
		)

		BeforeEach(func() {
			mockS6Service = process_shared.NewMockService()
			mockServices = serviceregistry.NewMockRegistry()
			nmapName = "test-nmap"
			service = NewDefaultNmapService(nmapName, WithS6Service(mockS6Service))
		})

		It("should call S6 ForceRemove with the correct service path", func() {
			ctx := context.Background()

			// Call ForceRemoveNmap
			err := service.ForceRemoveNmap(ctx, mockServices.GetFileSystem(), nmapName)

			// Verify no error
			Expect(err).NotTo(HaveOccurred())

			// Verify S6Service ForceRemove was called
			Expect(mockS6Service.ForceRemoveCalled).To(BeTrue())

			// Verify the path is correct
			expectedS6ServiceName := service.getS6ServiceName(nmapName)
			expectedS6ServicePath := filepath.Join(constants.S6BaseDir, expectedS6ServiceName)
			Expect(mockS6Service.ForceRemovePath).To(Equal(expectedS6ServicePath))
		})

		It("should propagate errors from S6 service", func() {
			ctx := context.Background()

			// Set up mock to return an error
			mockError := fmt.Errorf("mock force remove error")
			mockS6Service.ForceRemoveError = mockError

			// Call ForceRemoveNmap
			err := service.ForceRemoveNmap(ctx, mockServices.GetFileSystem(), nmapName)

			// Verify error is propagated
			Expect(err).To(MatchError(mockError))
		})
	})
})
