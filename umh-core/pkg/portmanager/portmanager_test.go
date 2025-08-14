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

package portmanager

import (
	"context"
	"fmt"
	"sync"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPortManager(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "PortManager Suite")
}

var _ = Describe("PortManager", func() {
	BeforeEach(func() {
		// Reset the singleton before each test
		ResetDefaultPortManager()
	})

	Describe("NewDefaultPortManager", func() {
		It("creates a port manager successfully", func() {
			portManager, err := NewDefaultPortManager(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(portManager).NotTo(BeNil())
			Expect(portManager.instanceToPorts).NotTo(BeNil())
			Expect(portManager.portToInstances).NotTo(BeNil())
		})

		It("returns the same instance when called multiple times", func() {
			pm1, err := NewDefaultPortManager(nil)
			Expect(err).NotTo(HaveOccurred())

			pm2, err := NewDefaultPortManager(nil)
			Expect(err).NotTo(HaveOccurred())

			Expect(pm1).To(BeIdenticalTo(pm2))
		})
	})

	Describe("AllocatePort", func() {
		Context("basic allocation", func() {
			It("allocates a port successfully", func() {
				pm, err := NewDefaultPortManager(nil)
				Expect(err).NotTo(HaveOccurred())

				instance := "test-instance"
				port, err := pm.AllocatePort(context.Background(), instance)
				Expect(err).NotTo(HaveOccurred())
				Expect(port).To(BeNumerically(">", 0))

				gotPort, exists := pm.GetPort(instance)
				Expect(exists).To(BeTrue())
				Expect(gotPort).To(Equal(port))
			})

			It("returns the same port for an existing instance", func() {
				pm, err := NewDefaultPortManager(nil)
				Expect(err).NotTo(HaveOccurred())

				instance := "test-instance"
				port1, err := pm.AllocatePort(context.Background(), instance)
				Expect(err).NotTo(HaveOccurred())

				// Allocate again for the same instance
				port2, err := pm.AllocatePort(context.Background(), instance)
				Expect(err).NotTo(HaveOccurred())
				Expect(port2).To(Equal(port1))
			})

			It("allocates multiple different ports", func() {
				pm, err := NewDefaultPortManager(nil)
				Expect(err).NotTo(HaveOccurred())

				// Allocate ports for multiple instances
				instances := []string{"instance-1", "instance-2", "instance-3"}
				ports := make(map[uint16]bool)

				for _, instance := range instances {
					port, err := pm.AllocatePort(context.Background(), instance)
					Expect(err).NotTo(HaveOccurred())
					Expect(port).To(BeNumerically(">", 0))

					// Ensure we don't get duplicate ports
					Expect(ports[port]).To(BeFalse(), "Port %d was allocated twice", port)
					ports[port] = true
				}

				// Verify we got 3 different ports
				Expect(ports).To(HaveLen(3))
			})

			It("handles port release and reallocation", func() {
				pm, err := NewDefaultPortManager(nil)
				Expect(err).NotTo(HaveOccurred())

				// Allocate a port
				instance1 := "instance-1"
				_, err = pm.AllocatePort(context.Background(), instance1)
				Expect(err).NotTo(HaveOccurred())

				// Release the port
				err = pm.ReleasePort(instance1)
				Expect(err).NotTo(HaveOccurred())

				// Verify the port is released
				_, exists := pm.GetPort(instance1)
				Expect(exists).To(BeFalse())

				// Allocate a new port for a different instance
				instance2 := "instance-2"
				port2, err := pm.AllocatePort(context.Background(), instance2)
				Expect(err).NotTo(HaveOccurred())
				Expect(port2).To(BeNumerically(">", 0))

				// The new port might be the same as the old one (since it was released)
				// or different - both are valid with OS allocation
			})
		})
	})

	Describe("ReleasePort", func() {
		It("releases an allocated port", func() {
			pm, err := NewDefaultPortManager(nil)
			Expect(err).NotTo(HaveOccurred())

			instance := "test-instance"
			_, err = pm.AllocatePort(context.Background(), instance)
			Expect(err).NotTo(HaveOccurred())

			err = pm.ReleasePort(instance)
			Expect(err).NotTo(HaveOccurred())

			// Verify the port is released
			_, exists := pm.GetPort(instance)
			Expect(exists).To(BeFalse())

			// Verify we can allocate a port again
			newPort, err := pm.AllocatePort(context.Background(), "another-instance")
			Expect(err).NotTo(HaveOccurred())
			Expect(newPort).To(BeNumerically(">", 0))
		})

		It("returns an error for non-existent instance", func() {
			pm, err := NewDefaultPortManager(nil)
			Expect(err).NotTo(HaveOccurred())

			err = pm.ReleasePort("non-existent")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("GetPort", func() {
		It("returns the correct port for an existing instance", func() {
			pm, err := NewDefaultPortManager(nil)
			Expect(err).NotTo(HaveOccurred())

			instance := "test-instance"
			expectedPort, err := pm.AllocatePort(context.Background(), instance)
			Expect(err).NotTo(HaveOccurred())

			gotPort, exists := pm.GetPort(instance)
			Expect(exists).To(BeTrue())
			Expect(gotPort).To(Equal(expectedPort))
		})

		It("returns false for a non-existent instance", func() {
			pm, err := NewDefaultPortManager(nil)
			Expect(err).NotTo(HaveOccurred())

			gotPort, exists := pm.GetPort("non-existent")
			Expect(exists).To(BeFalse())
			Expect(gotPort).To(BeZero())
		})
	})

	Describe("ReservePort", func() {
		It("reserves an available port", func() {
			pm, err := NewDefaultPortManager(nil)
			Expect(err).NotTo(HaveOccurred())

			instance := "test-instance"
			// Use a likely available port in a safe range
			portToReserve := uint16(55000)
			err = pm.ReservePort(context.Background(), instance, portToReserve)

			// Note: This might fail if the port is already in use by another process
			// That's expected behavior with real OS allocation
			if err == nil {
				// Verify the port is reserved
				gotPort, exists := pm.GetPort(instance)
				Expect(exists).To(BeTrue())
				Expect(gotPort).To(Equal(portToReserve))
			} else {
				// Port was not available, which is valid
				Expect(err.Error()).To(ContainSubstring("not available"))
			}
		})

		It("returns an error when port is already in use", func() {
			pm, err := NewDefaultPortManager(nil)
			Expect(err).NotTo(HaveOccurred())

			portToReserve := uint16(55001)

			// Try to reserve the port for first instance
			err = pm.ReservePort(context.Background(), "instance-1", portToReserve)
			if err != nil {
				// Port might already be in use by system, skip this test
				Skip("Port not available for testing")

				return
			}

			// Try to reserve the same port for another instance
			err = pm.ReservePort(context.Background(), "instance-2", portToReserve)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already in use"))
		})

		It("returns an error when instance already has a different port", func() {
			pm, err := NewDefaultPortManager(nil)
			Expect(err).NotTo(HaveOccurred())

			instance := "test-instance"
			port1 := uint16(55002)
			err = pm.ReservePort(context.Background(), instance, port1)
			if err != nil {
				// Port might already be in use by system, skip this test
				Skip("Port not available for testing")

				return
			}

			// Try to reserve a different port for the same instance
			port2 := uint16(55003)
			err = pm.ReservePort(context.Background(), instance, port2)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already has port"))
		})

		It("succeeds when reserving same port for same instance", func() {
			pm, err := NewDefaultPortManager(nil)
			Expect(err).NotTo(HaveOccurred())

			instance := "test-instance"
			port := uint16(55004)
			err = pm.ReservePort(context.Background(), instance, port)
			if err != nil {
				// Port might already be in use by system, skip this test
				Skip("Port not available for testing")

				return
			}

			// Reserve the same port for the same instance again
			err = pm.ReservePort(context.Background(), instance, port)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns an error for invalid ports", func() {
			pm, err := NewDefaultPortManager(nil)
			Expect(err).NotTo(HaveOccurred())

			err = pm.ReservePort(context.Background(), "test-instance", 0)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid port"))
		})
	})

	Describe("Concurrent operations", func() {
		It("handles concurrent allocations safely", func() {
			pm, err := NewDefaultPortManager(nil)
			Expect(err).NotTo(HaveOccurred())

			numGoroutines := 100
			var wg sync.WaitGroup
			wg.Add(numGoroutines)

			errors := make([]error, numGoroutines)
			ports := make([]uint16, numGoroutines)

			for i := range numGoroutines {
				go func(id int) {
					defer wg.Done()
					instance := fmt.Sprintf("instance-%d", id)
					port, err := pm.AllocatePort(context.Background(), instance)
					errors[id] = err
					if err == nil {
						ports[id] = port
					}
				}(i)
			}

			wg.Wait()

			// Check results
			allocatedPorts := make(map[uint16]string)
			for i, err := range errors {
				if err == nil {
					port := ports[i]
					Expect(port).To(BeNumerically(">", 0))

					// Verify no port duplications
					instance, exists := allocatedPorts[port]
					if exists {
						Fail(fmt.Sprintf("Port %d allocated to both %s and instance-%d", port, instance, i))
					}
					allocatedPorts[port] = fmt.Sprintf("instance-%d", i)
				}
			}

			// Verify we have allocated correctly
			allocatedCount := len(pm.instanceToPorts)
			Expect(allocatedCount).To(BeNumerically("<=", numGoroutines))
		})

		It("handles mixed concurrent operations safely", func() {
			pm, err := NewDefaultPortManager(nil)
			Expect(err).NotTo(HaveOccurred())

			numGoroutines := 50
			var wg sync.WaitGroup
			wg.Add(numGoroutines * 3) // Allocate, Reserve, Release

			// Pre-allocate some instances for releasing
			instances := make([]string, numGoroutines)
			for i := range numGoroutines {
				instances[i] = fmt.Sprintf("instance-%d", i)
				_, err := pm.AllocatePort(context.Background(), instances[i])
				Expect(err).NotTo(HaveOccurred())
			}

			// Concurrent allocations
			for i := range numGoroutines {
				go func(id int) {
					defer GinkgoRecover()
					defer wg.Done()
					instance := fmt.Sprintf("new-instance-%d", id)
					_, err := pm.AllocatePort(context.Background(), instance)
					Expect(err).NotTo(HaveOccurred())
				}(i)
			}

			// Concurrent reservations
			for i := range numGoroutines {
				go func(id int) {
					defer GinkgoRecover()
					defer wg.Done()
					instance := fmt.Sprintf("reserve-instance-%d", id)
					port := uint16(55000 + id%1000)                             // Use high port range to avoid conflicts
					err := pm.ReservePort(context.Background(), instance, port) // Some might fail due to system usage
					if err != nil {
						// Error is expected - should be "port not available", "already in use" or "instance already has port"
						Expect(err.Error()).To(Or(
							ContainSubstring("not available"),
							ContainSubstring("already in use"),
							ContainSubstring("already has port"),
						))
					}
				}(i)
			}

			// Concurrent releases
			for i := range numGoroutines {
				go func(id int) {
					defer wg.Done()
					err := pm.ReleasePort(instances[id]) // Release pre-allocated instances
					Expect(err).NotTo(HaveOccurred())    // These should always succeed
				}(i)
			}

			wg.Wait()
			// If we got here without deadlock or panic, the test passes
			Succeed()
		})
	})

	Describe("PreReconcile", func() {
		It("allocates ports for new instances", func() {
			pm, err := NewDefaultPortManager(nil)
			Expect(err).NotTo(HaveOccurred())

			// Test with multiple instances
			instanceNames := []string{"instance-1", "instance-2", "instance-3"}
			err = pm.PreReconcile(context.Background(), instanceNames)
			Expect(err).NotTo(HaveOccurred())

			// Verify ports were allocated
			for _, name := range instanceNames {
				port, exists := pm.GetPort(name)
				Expect(exists).To(BeTrue())
				Expect(port).To(BeNumerically(">", 0))
			}
		})

		It("handles instances that already have ports", func() {
			pm, err := NewDefaultPortManager(nil)
			Expect(err).NotTo(HaveOccurred())

			// Pre-allocate a port
			existingInstance := "existing-instance"
			existingPort, err := pm.AllocatePort(context.Background(), existingInstance)
			Expect(err).NotTo(HaveOccurred())

			// Run PreReconcile with mix of existing and new instances
			instanceNames := []string{existingInstance, "new-instance"}
			err = pm.PreReconcile(context.Background(), instanceNames)
			Expect(err).NotTo(HaveOccurred())

			// Verify existing instance kept its port
			port, exists := pm.GetPort(existingInstance)
			Expect(exists).To(BeTrue())
			Expect(port).To(Equal(existingPort))

			// Verify new instance got a port
			port, exists = pm.GetPort("new-instance")
			Expect(exists).To(BeTrue())
			Expect(port).To(BeNumerically(">", 0))
		})

		It("allocates successfully with OS port allocation", func() {
			pm, err := NewDefaultPortManager(nil)
			Expect(err).NotTo(HaveOccurred())

			// With OS allocation, we should be able to allocate many ports
			instanceNames := []string{"instance-1", "instance-2", "instance-3", "instance-4", "instance-5"}
			err = pm.PreReconcile(context.Background(), instanceNames)
			Expect(err).NotTo(HaveOccurred())

			// Verify all ports were allocated and are unique
			allocatedPorts := make(map[uint16]bool)
			for _, name := range instanceNames {
				port, exists := pm.GetPort(name)
				Expect(exists).To(BeTrue())
				Expect(port).To(BeNumerically(">", 0))
				Expect(allocatedPorts[port]).To(BeFalse(), "Port %d was allocated twice", port)
				allocatedPorts[port] = true
			}
		})
	})

	Describe("PostReconcile", func() {
		It("completes without error", func() {
			pm, err := NewDefaultPortManager(nil)
			Expect(err).NotTo(HaveOccurred())

			err = pm.PostReconcile(context.Background())
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
