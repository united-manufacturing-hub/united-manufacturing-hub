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

package connection_test

import (
	"context"
	"errors"
	"reflect"
	"unsafe"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/connection"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/nmap"
)

var _ = Describe("Connection Service", func() {
	var (
		mockNmap    *nmap.MockNmapService
		connService *connection.ConnectionService
		ctx         context.Context
		tick        uint64
	)

	BeforeEach(func() {
		mockNmap = nmap.NewMockNmapService()
		connService = connection.NewConnectionServiceForTesting(
			connection.WithNmapService(mockNmap),
		)
		ctx = context.Background()
		tick = uint64(1)
	})

	Describe("Status", func() {
		var (
			connName  string
			info      connection.ServiceInfo
			statusErr error
		)

		BeforeEach(func() {
			connName = "test-connection"

			// Set up the mock nmap status info
			mockNmap.StatusResult = nmap.ServiceInfo{
				S6FSMState: "running",
				NmapStatus: nmap.NmapServiceInfo{
					IsRunning: true,
					LastScan: &nmap.NmapScanResult{
						PortResult: nmap.PortResult{
							State: "open",
						},
					},
				},
			}

			// Add the connection to the mock nmap service
			mockNmap.Configs[connName] = &nmapserviceconfig.NmapServiceConfig{
				Target: "localhost",
				Port:   8080,
			}
		})

		JustBeforeEach(func() {
			info, statusErr = connService.Status(ctx, nil, connName, tick)
		})

		Context("when the nmap service returns a valid status", func() {
			It("should return the correct service info", func() {
				Expect(statusErr).NotTo(HaveOccurred())
				Expect(info.IsRunning).To(BeTrue())
				Expect(info.PortStateOpen).To(BeTrue())
				Expect(info.IsReachable).To(BeTrue())
				Expect(info.LastChange).To(Equal(tick))
			})
		})

		Context("when the nmap service returns a closed port", func() {
			BeforeEach(func() {
				mockNmap.StatusResult.NmapStatus.LastScan.PortResult.State = "closed"
			})

			It("should indicate the port is not open", func() {
				Expect(statusErr).NotTo(HaveOccurred())
				Expect(info.IsRunning).To(BeTrue())
				Expect(info.PortStateOpen).To(BeFalse())
				Expect(info.IsReachable).To(BeFalse())
			})
		})

		Context("when the nmap service returns an error", func() {
			BeforeEach(func() {
				mockNmap.StatusError = errors.New("nmap error")
			})

			It("should return the error", func() {
				Expect(statusErr).To(HaveOccurred())
				Expect(statusErr.Error()).To(ContainSubstring("nmap error"))
			})
		})

		Context("when the nmap service indicates service does not exist", func() {
			BeforeEach(func() {
				delete(mockNmap.Configs, connName)
				mockNmap.StatusError = nmap.ErrServiceNotExist
			})

			It("should return the service not exist error", func() {
				Expect(statusErr).To(HaveOccurred())
				Expect(statusErr).To(MatchError(ContainSubstring("failed to get nmap status")))
			})
		})
	})

	Describe("AddConnection", func() {
		var (
			connName string
			cfg      *connectionserviceconfig.ConnectionServiceConfig
			addErr   error
		)

		BeforeEach(func() {
			connName = "test-connection"
			cfg = &connectionserviceconfig.ConnectionServiceConfig{
				NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
					Target: "localhost",
					Port:   8080,
				},
			}
		})

		JustBeforeEach(func() {
			addErr = connService.AddConnection(ctx, nil, cfg, connName)
		})

		It("should add the connection without error", func() {
			Expect(addErr).NotTo(HaveOccurred())
			Expect(mockNmap.Configs).To(HaveKey(connName))

			// Verify the config was converted correctly
			nmapConfig := mockNmap.Configs[connName]
			Expect(nmapConfig.Target).To(Equal("localhost"))
			Expect(nmapConfig.Port).To(Equal(8080))
		})

		Context("when the nmap service returns an error", func() {
			BeforeEach(func() {
				mockNmap.AddServiceError = errors.New("add error")
			})

			It("should return the error", func() {
				Expect(addErr).To(HaveOccurred())
				Expect(addErr.Error()).To(Equal("add error"))
			})
		})
	})

	Describe("UpdateConnection", func() {
		var (
			connName  string
			cfg       *connectionserviceconfig.ConnectionServiceConfig
			updateErr error
		)

		BeforeEach(func() {
			connName = "test-connection"

			// Add the connection to the mock nmap service first
			mockNmap.Configs[connName] = &nmapserviceconfig.NmapServiceConfig{
				Target: "localhost",
				Port:   8080,
			}

			// Updated config
			cfg = &connectionserviceconfig.ConnectionServiceConfig{
				NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
					Target: "example.com",
					Port:   9090,
				},
			}
		})

		JustBeforeEach(func() {
			updateErr = connService.UpdateConnection(ctx, nil, cfg, connName)
		})

		It("should update the connection without error", func() {
			Expect(updateErr).NotTo(HaveOccurred())

			// Verify the config was updated
			nmapConfig := mockNmap.Configs[connName]
			Expect(nmapConfig.Target).To(Equal("example.com"))
			Expect(nmapConfig.Port).To(Equal(9090))
		})

		Context("when the nmap service returns an error", func() {
			BeforeEach(func() {
				mockNmap.UpdateServiceError = errors.New("update error")
			})

			It("should return the error", func() {
				Expect(updateErr).To(HaveOccurred())
				Expect(updateErr.Error()).To(ContainSubstring("update error"))
			})
		})
	})

	Describe("RemoveConnection", func() {
		var (
			connName  string
			removeErr error
		)

		BeforeEach(func() {
			connName = "test-connection"

			// Add the connection to the mock nmap service first
			mockNmap.Configs[connName] = &nmapserviceconfig.NmapServiceConfig{
				Target: "localhost",
				Port:   8080,
			}
		})

		JustBeforeEach(func() {
			removeErr = connService.RemoveConnection(ctx, nil, connName)
		})

		It("should remove the connection without error", func() {
			Expect(removeErr).NotTo(HaveOccurred())
			Expect(mockNmap.Configs).NotTo(HaveKey(connName))
		})

		Context("when the nmap service returns an error", func() {
			BeforeEach(func() {
				mockNmap.RemoveServiceError = errors.New("remove error")
			})

			It("should return the error", func() {
				Expect(removeErr).To(HaveOccurred())
				Expect(removeErr.Error()).To(ContainSubstring("remove error"))
			})
		})
	})

	Describe("StartStopConnection", func() {
		var (
			connName string
			startErr error
			stopErr  error
		)

		BeforeEach(func() {
			connName = "test-connection"

			// Add the connection to the mock nmap service first
			mockNmap.Configs[connName] = &nmapserviceconfig.NmapServiceConfig{
				Target: "localhost",
				Port:   8080,
			}
		})

		Context("when starting a connection", func() {
			JustBeforeEach(func() {
				startErr = connService.StartConnection(ctx, nil, connName)
			})

			It("should start the connection without error", func() {
				Expect(startErr).NotTo(HaveOccurred())
				Expect(mockNmap.StateMap[connName]).To(Equal("running"))
			})

			Context("when the nmap service returns an error", func() {
				BeforeEach(func() {
					mockNmap.StartServiceError = errors.New("start error")
				})

				It("should return the error", func() {
					Expect(startErr).To(HaveOccurred())
					Expect(startErr.Error()).To(ContainSubstring("start error"))
				})
			})
		})

		Context("when stopping a connection", func() {
			BeforeEach(func() {
				mockNmap.StateMap[connName] = "running"
			})

			JustBeforeEach(func() {
				stopErr = connService.StopConnection(ctx, nil, connName)
			})

			It("should stop the connection without error", func() {
				Expect(stopErr).NotTo(HaveOccurred())
				Expect(mockNmap.StateMap[connName]).To(Equal("stopped"))
			})

			Context("when the nmap service returns an error", func() {
				BeforeEach(func() {
					mockNmap.StopServiceError = errors.New("stop error")
				})

				It("should return the error", func() {
					Expect(stopErr).To(HaveOccurred())
					Expect(stopErr.Error()).To(ContainSubstring("stop error"))
				})
			})
		})
	})

	Describe("ServiceExists", func() {
		var (
			connName string
			exists   bool
		)

		JustBeforeEach(func() {
			exists = connService.ServiceExists(ctx, nil, connName)
		})

		Context("when connection does not exist", func() {
			BeforeEach(func() {
				connName = "nonexistent-connection"
				mockNmap.ExistsError = true
			})

			It("should return false", func() {
				Expect(exists).To(BeFalse())
			})
		})

		Context("when connection exists", func() {
			BeforeEach(func() {
				connName = "test-connection"
				mockNmap.Configs[connName] = &nmapserviceconfig.NmapServiceConfig{}
				mockNmap.ExistsError = false
			})

			It("should return true", func() {
				Expect(exists).To(BeTrue())
			})
		})
	})

	Describe("ReconcileManager", func() {
		var (
			reconcileErr error
			reconciled   bool
		)

		BeforeEach(func() {
			mockNmap.ReconcileError = nil
		})

		JustBeforeEach(func() {
			reconcileErr, reconciled = connService.ReconcileManager(ctx, nil, tick)
		})

		It("should reconcile without error", func() {
			Expect(reconcileErr).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())
		})

		Context("when reconcile is configured to fail", func() {
			BeforeEach(func() {
				mockNmap.ReconcileError = errors.New("reconcile error")
			})

			It("should return the error", func() {
				Expect(reconcileErr).To(HaveOccurred())
				Expect(reconcileErr.Error()).To(Equal("reconcile error"))
			})
		})
	})

	Describe("Flaky detection", func() {
		var (
			connName  string
			info      connection.ServiceInfo
			statusErr error
		)

		BeforeEach(func() {
			connName = "flaky-connection"

			// Add the connection to the mock nmap service
			mockNmap.Configs[connName] = &nmapserviceconfig.NmapServiceConfig{
				Target: "localhost",
				Port:   8080,
			}
		})

		Context("when connection state changes frequently", func() {
			BeforeEach(func() {
				// First call to Status - to populate the recent scans cache
				mockNmap.SetServicePortState(connName, "open", 20.0)
				serviceInfo, err := connService.Status(ctx, nil, connName, tick)
				Expect(err).NotTo(HaveOccurred())
				Expect(serviceInfo.IsReachable).To(BeTrue())

				// Second call - different state
				mockNmap.SetServicePortState(connName, "closed", 100.0)
				serviceInfo, err = connService.Status(ctx, nil, connName, tick)
				Expect(err).NotTo(HaveOccurred())
				Expect(serviceInfo.IsReachable).To(BeFalse())

				// Third call - open again
				mockNmap.SetServicePortState(connName, "open", 25.0)
				serviceInfo, err = connService.Status(ctx, nil, connName, tick)
				Expect(err).NotTo(HaveOccurred())
				Expect(serviceInfo.IsReachable).To(BeTrue())
			})

			JustBeforeEach(func() {
				info, statusErr = connService.Status(ctx, nil, connName, tick)
			})

			It("should detect the connection as flaky", func() {
				Expect(statusErr).NotTo(HaveOccurred())
				Expect(info.IsFlaky).To(BeTrue())
				Expect(info.IsReachable).To(BeTrue()) // Currently open
			})
		})

		Context("when connection state is stable", func() {
			BeforeEach(func() {
				// Multiple calls with the same state
				mockNmap.SetServicePortState(connName, "open", 20.0)
				serviceInfo, err := connService.Status(ctx, nil, connName, tick)
				Expect(err).NotTo(HaveOccurred())
				Expect(serviceInfo.IsReachable).To(BeTrue())

				serviceInfo, err = connService.Status(ctx, nil, connName, tick)
				Expect(err).NotTo(HaveOccurred())
				Expect(serviceInfo.IsReachable).To(BeTrue())
			})

			JustBeforeEach(func() {
				info, statusErr = connService.Status(ctx, nil, connName, tick)
			})

			It("should not detect the connection as flaky", func() {
				Expect(statusErr).NotTo(HaveOccurred())
				Expect(info.IsFlaky).To(BeFalse())
				Expect(info.IsReachable).To(BeTrue())
			})
		})

		Context("when fewer than 3 samples are collected", func() {
			BeforeEach(func() {
				// First call - initial state "open"
				mockNmap.SetServicePortState(connName, "open", 20.0)
				serviceInfo, err := connService.Status(ctx, nil, connName, tick)
				Expect(err).NotTo(HaveOccurred())
				Expect(serviceInfo.IsReachable).To(BeTrue())

				// Second call - change to "closed"
				mockNmap.SetServicePortState(connName, "closed", 100.0)
				// do not call status here
			})

			JustBeforeEach(func() {
				// now call the status method, it will be called with the portstate closed
				info, statusErr = connService.Status(ctx, nil, connName, tick)
				// so we have now 2 samples in the cache
				// if we would have called the status method where I wrote "do not call status here", we would have 3 samples in the cache
			})

			It("should not detect the connection as flaky despite state changes", func() {
				Expect(statusErr).NotTo(HaveOccurred())
				Expect(info.IsFlaky).To(BeFalse())
				Expect(info.IsReachable).To(BeFalse()) // Currently closed
			})
		})

		Context("when connection alternates between open and filtered states", func() {
			BeforeEach(func() {
				// First call - initial state "open"
				mockNmap.SetServicePortState(connName, "open", 20.0)
				serviceInfo, err := connService.Status(ctx, nil, connName, tick)
				Expect(err).NotTo(HaveOccurred())
				Expect(serviceInfo.IsReachable).To(BeTrue())

				// Second call - change to "filtered"
				mockNmap.SetServicePortState(connName, "filtered", 150.0)
				serviceInfo, err = connService.Status(ctx, nil, connName, tick)
				Expect(err).NotTo(HaveOccurred())
				Expect(serviceInfo.IsReachable).To(BeFalse())

				// Third call - back to "open"
				mockNmap.SetServicePortState(connName, "open", 25.0)
				serviceInfo, err = connService.Status(ctx, nil, connName, tick)
				Expect(err).NotTo(HaveOccurred())
				Expect(serviceInfo.IsReachable).To(BeTrue())
			})

			JustBeforeEach(func() {
				info, statusErr = connService.Status(ctx, nil, connName, tick)
			})

			It("should detect the connection as flaky with open/filtered states", func() {
				Expect(statusErr).NotTo(HaveOccurred())
				Expect(info.IsFlaky).To(BeTrue())
				Expect(info.IsReachable).To(BeTrue()) // Currently open
			})
		})

		Context("when connection alternates between closed and filtered states", func() {
			BeforeEach(func() {
				// First call - initial state "closed"
				mockNmap.SetServicePortState(connName, "closed", 80.0)
				serviceInfo, err := connService.Status(ctx, nil, connName, tick)
				Expect(err).NotTo(HaveOccurred())
				Expect(serviceInfo.IsReachable).To(BeFalse())

				// Second call - change to "filtered"
				mockNmap.SetServicePortState(connName, "filtered", 150.0)
				serviceInfo, err = connService.Status(ctx, nil, connName, tick)
				Expect(err).NotTo(HaveOccurred())
				Expect(serviceInfo.IsReachable).To(BeFalse())

				// Third call - back to "closed"
				mockNmap.SetServicePortState(connName, "closed", 85.0)
				serviceInfo, err = connService.Status(ctx, nil, connName, tick)
				Expect(err).NotTo(HaveOccurred())
				Expect(serviceInfo.IsReachable).To(BeFalse())
			})

			JustBeforeEach(func() {
				info, statusErr = connService.Status(ctx, nil, connName, tick)
			})

			It("should detect the connection as flaky with closed/filtered states", func() {
				Expect(statusErr).NotTo(HaveOccurred())
				Expect(info.IsFlaky).To(BeTrue())
				Expect(info.IsReachable).To(BeFalse()) // Currently closed
			})
		})

		Context("when a flaky connection stabilizes", func() {
			BeforeEach(func() {
				// First create a flaky connection pattern
				mockNmap.SetServicePortState(connName, "open", 20.0)
				connService.Status(ctx, nil, connName, tick)

				mockNmap.SetServicePortState(connName, "closed", 100.0)
				connService.Status(ctx, nil, connName, tick)

				mockNmap.SetServicePortState(connName, "open", 25.0)
				info, err := connService.Status(ctx, nil, connName, tick)
				Expect(err).NotTo(HaveOccurred())
				Expect(info.IsFlaky).To(BeTrue()) // Confirm it's initially flaky

				// Then stabilize with several consistent "open" states
				for i := 0; i < constants.MaxRecentScans; i++ {
					mockNmap.SetServicePortState(connName, "open", 20.0+float64(i))
					connService.Status(ctx, nil, connName, tick)
				}
			})

			JustBeforeEach(func() {
				info, statusErr = connService.Status(ctx, nil, connName, tick)
			})

			It("should no longer detect the connection as flaky after stabilizing", func() {
				Expect(statusErr).NotTo(HaveOccurred())
				Expect(info.IsFlaky).To(BeFalse(), "Connection should no longer be flaky after stabilizing")
				Expect(info.IsReachable).To(BeTrue())
			})
		})

		Context("when LastScan is nil", func() {
			BeforeEach(func() {
				// Create custom ServiceInfo with nil LastScan
				nilScanInfo := nmap.ServiceInfo{
					S6FSMState: "running",
					NmapStatus: nmap.NmapServiceInfo{
						IsRunning: true,
						LastScan:  nil, // Explicitly nil
					},
				}

				// Set this as the status result
				mockNmap.StatusResult = nilScanInfo

				// Make a few calls to populate the history
				for i := 0; i < 3; i++ {
					serviceInfo, err := connService.Status(ctx, nil, connName, tick)
					Expect(err).NotTo(HaveOccurred())
					Expect(serviceInfo.PortStateOpen).To(BeFalse()) // Should be false when LastScan is nil
				}
			})

			JustBeforeEach(func() {
				info, statusErr = connService.Status(ctx, nil, connName, tick)
			})

			It("should gracefully handle nil LastScan without being flaky", func() {
				Expect(statusErr).NotTo(HaveOccurred())
				Expect(info.IsFlaky).To(BeFalse())
				Expect(info.PortStateOpen).To(BeFalse())
			})
		})

		Context("when connection experiences all three possible states", func() {
			BeforeEach(func() {
				// Create a pattern with open, closed, and filtered states
				mockNmap.SetServicePortState(connName, "open", 20.0)
				connService.Status(ctx, nil, connName, tick)

				mockNmap.SetServicePortState(connName, "closed", 100.0)
				connService.Status(ctx, nil, connName, tick)

				mockNmap.SetServicePortState(connName, "filtered", 80.0)
				connService.Status(ctx, nil, connName, tick)
			})

			JustBeforeEach(func() {
				info, statusErr = connService.Status(ctx, nil, connName, tick)
			})

			It("should detect the connection as flaky with mixed states", func() {
				Expect(statusErr).NotTo(HaveOccurred())
				Expect(info.IsFlaky).To(BeTrue())
				Expect(info.IsReachable).To(BeFalse()) // Currently filtered
			})
		})

		// Test case for verifying the scan history limit implementation
		Context("when maxRecentScans limit is reached", func() {
			const testMaxScans = 5 // Small value for testing

			var originalService *connection.ConnectionService
			var limitedService *connection.ConnectionService

			BeforeEach(func() {
				// Save reference to original service
				originalService = connService

				// Create a new service with a smaller maxRecentScans for testing
				limitedService = connection.NewConnectionServiceForTesting(
					connection.WithNmapService(mockNmap),
					connection.WithMaxRecentScans(testMaxScans),
				)

				// Use the limited service for this test
				connService = limitedService

				// Fill the history with more than testMaxScans entries
				states := []string{"open", "closed", "closed", "filtered", "closed", "open", "filtered"}
				for i, state := range states {
					mockNmap.SetServicePortState(connName, state, 20.0+float64(i*10))
					_, err := limitedService.Status(ctx, nil, connName, tick)
					Expect(err).NotTo(HaveOccurred())
				}
			})

			AfterEach(func() {
				// Restore the original service
				connService = originalService
			})

			JustBeforeEach(func() {
				info, statusErr = limitedService.Status(ctx, nil, connName, tick)
				// this will add 1 more item to the history of the latest service port state (which is filtered)
			})

			It("should limit the scan history to maxRecentScans", func() {
				Expect(statusErr).NotTo(HaveOccurred())

				// Get the internal state directly for testing
				var scans []nmap.ServiceInfo
				// Use unsafe to access unexported field
				rs := reflect.ValueOf(limitedService).Elem().FieldByName("recentScans")
				if rs.IsValid() {
					rsPtr := unsafe.Pointer(rs.UnsafeAddr())
					rsMap := *(*map[string][]nmap.ServiceInfo)(rsPtr)
					scans = rsMap[connName]
				}
				// The implementation may store maxRecentScans items before trimming
				Expect(len(scans)).To(BeNumerically("<=", testMaxScans))

				// Verify the oldest entries were removed (the first "open" and "closed" states and by calling again the status method also the second closed state)
				// and the newest entries are kept
				Expect(scans[0].NmapStatus.LastScan.PortResult.State).NotTo(Equal("open"))
				Expect(scans[0].NmapStatus.LastScan.PortResult.State).To(Equal("filtered"))
				Expect(scans[testMaxScans-1].NmapStatus.LastScan.PortResult.State).To(Equal("filtered"))
			})
		})
	})
})
