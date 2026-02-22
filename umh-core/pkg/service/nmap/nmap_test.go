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
	"net"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Nmap Service", func() {
	var (
		service  *NmapService
		nmapName string
	)

	BeforeEach(func() {
		// Clear the shared global registry before each test
		globalInstancesMu.Lock()
		for name, inst := range globalInstances {
			if inst.cancel != nil {
				inst.cancel()
			}
			delete(globalInstances, name)
		}
		globalInstancesMu.Unlock()

		nmapName = "test-scan"
		service = NewDefaultNmapService(nmapName)
	})

	Describe("TCP Check", func() {
		Context("with an open port", func() {
			It("should return state open", func() {
				// Start a local TCP listener
				ln, err := net.Listen("tcp", "127.0.0.1:0")
				Expect(err).NotTo(HaveOccurred())
				defer ln.Close()

				addr := ln.Addr().(*net.TCPAddr)
				result := tcpCheck("127.0.0.1", uint16(addr.Port), 2*time.Second)
				Expect(result).NotTo(BeNil())
				Expect(result.PortResult.State).To(Equal("open"))
				Expect(result.PortResult.Port).To(Equal(uint16(addr.Port)))
				Expect(result.Timestamp).NotTo(BeZero())
				Expect(result.PortResult.LatencyMs).To(BeNumerically(">", 0))
			})
		})

		Context("with a closed port", func() {
			It("should return state closed", func() {
				// Use a port that is not listening (bind+close to get a free port)
				ln, err := net.Listen("tcp", "127.0.0.1:0")
				Expect(err).NotTo(HaveOccurred())
				addr := ln.Addr().(*net.TCPAddr)
				ln.Close() // Close so port is closed

				result := tcpCheck("127.0.0.1", uint16(addr.Port), 2*time.Second)
				Expect(result).NotTo(BeNil())
				Expect(result.PortResult.State).To(Equal("closed"))
			})
		})

		Context("with a filtered port (timeout)", func() {
			It("should return state filtered on timeout", func() {
				// Use a non-routable address to trigger timeout
				result := tcpCheck("198.51.100.1", 12345, 100*time.Millisecond)
				Expect(result).NotTo(BeNil())
				Expect(result.PortResult.State).To(Equal("filtered"))
			})
		})
	})

	Describe("Service Lifecycle", func() {
		Context("adding a service", func() {
			It("should add and start the TCP check goroutine", func() {
				ctx := context.Background()
				cfg := &nmapserviceconfig.NmapServiceConfig{
					Target: "127.0.0.1",
					Port:   12345,
				}

				err := service.AddNmapToS6Manager(ctx, cfg, nmapName)
				Expect(err).NotTo(HaveOccurred())

				// Verify service exists
				Expect(service.ServiceExists(ctx, nil, nmapName)).To(BeTrue())

				// Clean up
				err = service.RemoveNmapFromS6Manager(ctx, nmapName)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return error when service already exists", func() {
				ctx := context.Background()
				cfg := &nmapserviceconfig.NmapServiceConfig{
					Target: "127.0.0.1",
					Port:   12345,
				}

				err := service.AddNmapToS6Manager(ctx, cfg, nmapName)
				Expect(err).NotTo(HaveOccurred())

				err = service.AddNmapToS6Manager(ctx, cfg, nmapName)
				Expect(err).To(Equal(ErrServiceAlreadyExists))

				// Clean up
				_ = service.RemoveNmapFromS6Manager(ctx, nmapName)
			})
		})

		Context("removing a service", func() {
			It("should remove the service and stop the goroutine", func() {
				ctx := context.Background()
				cfg := &nmapserviceconfig.NmapServiceConfig{
					Target: "127.0.0.1",
					Port:   12345,
				}

				err := service.AddNmapToS6Manager(ctx, cfg, nmapName)
				Expect(err).NotTo(HaveOccurred())

				err = service.RemoveNmapFromS6Manager(ctx, nmapName)
				Expect(err).NotTo(HaveOccurred())

				Expect(service.ServiceExists(ctx, nil, nmapName)).To(BeFalse())
			})

			It("should not error when removing non-existent service", func() {
				ctx := context.Background()
				err := service.RemoveNmapFromS6Manager(ctx, "nonexistent")
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("updating a service", func() {
			It("should update the config and restart the goroutine", func() {
				ctx := context.Background()
				cfg := &nmapserviceconfig.NmapServiceConfig{
					Target: "127.0.0.1",
					Port:   12345,
				}

				err := service.AddNmapToS6Manager(ctx, cfg, nmapName)
				Expect(err).NotTo(HaveOccurred())

				updatedCfg := &nmapserviceconfig.NmapServiceConfig{
					Target: "127.0.0.1",
					Port:   8080,
				}

				err = service.UpdateNmapInS6Manager(ctx, updatedCfg, nmapName)
				Expect(err).NotTo(HaveOccurred())

				// Verify config was updated
				gotCfg, err := service.GetConfig(ctx, nil, nmapName)
				Expect(err).NotTo(HaveOccurred())
				Expect(gotCfg.Port).To(Equal(uint16(8080)))

				// Clean up
				_ = service.RemoveNmapFromS6Manager(ctx, nmapName)
			})

			It("should return error when service doesn't exist", func() {
				ctx := context.Background()
				cfg := &nmapserviceconfig.NmapServiceConfig{
					Target: "127.0.0.1",
					Port:   8080,
				}

				err := service.UpdateNmapInS6Manager(ctx, cfg, "nonexistent")
				Expect(err).To(Equal(ErrServiceNotExist))
			})
		})

		Context("starting and stopping", func() {
			BeforeEach(func() {
				ctx := context.Background()
				cfg := &nmapserviceconfig.NmapServiceConfig{
					Target: "127.0.0.1",
					Port:   12345,
				}
				err := service.AddNmapToS6Manager(ctx, cfg, nmapName)
				Expect(err).NotTo(HaveOccurred())
			})

			AfterEach(func() {
				_ = service.RemoveNmapFromS6Manager(context.Background(), nmapName)
			})

			It("should stop and start the goroutine", func() {
				ctx := context.Background()

				// Stop the service
				err := service.StopNmap(ctx, nmapName)
				Expect(err).NotTo(HaveOccurred())

				// Verify status shows stopped
				status, err := service.Status(ctx, nil, nmapName, 0)
				Expect(err).NotTo(HaveOccurred())
				Expect(status.S6FSMState).To(Equal(s6fsm.OperationalStateStopped))
				Expect(status.NmapStatus.IsRunning).To(BeFalse())

				// Start the service
				err = service.StartNmap(ctx, nmapName)
				Expect(err).NotTo(HaveOccurred())

				// Verify status shows running
				status, err = service.Status(ctx, nil, nmapName, 0)
				Expect(err).NotTo(HaveOccurred())
				Expect(status.S6FSMState).To(Equal(s6fsm.OperationalStateRunning))
				Expect(status.NmapStatus.IsRunning).To(BeTrue())
			})

			It("should return error for non-existent service", func() {
				ctx := context.Background()
				err := service.StartNmap(ctx, "nonexistent")
				Expect(err).To(Equal(ErrServiceNotExist))

				err = service.StopNmap(ctx, "nonexistent")
				Expect(err).To(Equal(ErrServiceNotExist))
			})
		})
	})

	Describe("Status", func() {
		It("should return error for non-existent service", func() {
			ctx := context.Background()
			_, err := service.Status(ctx, nil, "nonexistent", 0)
			Expect(err).To(Equal(ErrServiceNotExist))
		})

		It("should return scan results after goroutine runs", func() {
			// Start a local TCP listener
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			Expect(err).NotTo(HaveOccurred())
			defer ln.Close()

			addr := ln.Addr().(*net.TCPAddr)
			ctx := context.Background()
			cfg := &nmapserviceconfig.NmapServiceConfig{
				Target: "127.0.0.1",
				Port:   uint16(addr.Port),
			}

			err = service.AddNmapToS6Manager(ctx, cfg, nmapName)
			Expect(err).NotTo(HaveOccurred())
			defer service.RemoveNmapFromS6Manager(ctx, nmapName)

			// Wait briefly for the initial scan to complete
			Eventually(func() string {
				status, err := service.Status(ctx, nil, nmapName, 0)
				if err != nil {
					return ""
				}
				if status.NmapStatus.LastScan == nil {
					return ""
				}
				return status.NmapStatus.LastScan.PortResult.State
			}, 3*time.Second, 100*time.Millisecond).Should(Equal("open"))
		})
	})

	Describe("GetConfig", func() {
		It("should return the stored config", func() {
			ctx := context.Background()
			cfg := &nmapserviceconfig.NmapServiceConfig{
				Target: "example.com",
				Port:   443,
			}

			err := service.AddNmapToS6Manager(ctx, cfg, nmapName)
			Expect(err).NotTo(HaveOccurred())
			defer service.RemoveNmapFromS6Manager(ctx, nmapName)

			gotCfg, err := service.GetConfig(ctx, nil, nmapName)
			Expect(err).NotTo(HaveOccurred())
			Expect(gotCfg.Target).To(Equal("example.com"))
			Expect(gotCfg.Port).To(Equal(uint16(443)))
		})

		It("should return error for non-existent service", func() {
			ctx := context.Background()
			_, err := service.GetConfig(ctx, nil, "nonexistent")
			Expect(err).To(Equal(ErrServiceNotExist))
		})
	})

	Describe("ForceRemoveNmap", func() {
		It("should remove the service", func() {
			ctx := context.Background()
			cfg := &nmapserviceconfig.NmapServiceConfig{
				Target: "127.0.0.1",
				Port:   12345,
			}

			err := service.AddNmapToS6Manager(ctx, cfg, nmapName)
			Expect(err).NotTo(HaveOccurred())

			err = service.ForceRemoveNmap(ctx, nil, nmapName)
			Expect(err).NotTo(HaveOccurred())

			Expect(service.ServiceExists(ctx, nil, nmapName)).To(BeFalse())
		})
	})

	Describe("ReconcileManager", func() {
		It("should be a no-op and return nil", func() {
			ctx := context.Background()
			err, reconciled := service.ReconcileManager(ctx, nil, fsm.SystemSnapshot{})
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeFalse())
		})
	})
})
