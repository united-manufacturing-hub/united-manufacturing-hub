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
			portManager1, err := NewDefaultPortManager(nil)
			Expect(err).NotTo(HaveOccurred())

			portManager2, err := NewDefaultPortManager(nil)
			Expect(err).NotTo(HaveOccurred())

			Expect(portManager1).To(BeIdenticalTo(portManager2))
		})
	})

	Describe("AllocatePort", func() {
		Context("basic allocation", func() {
			It("allocates a port successfully", func() {
				portManager, err := NewDefaultPortManager(nil)
				Expect(err).NotTo(HaveOccurred())

				instance := "test-instance"
				port, err := portManager.AllocatePort(context.Background(), instance)
				Expect(err).NotTo(HaveOccurred())
				Expect(port).To(BeNumerically(">", 0))

				gotPort, exists := portManager.GetPort(instance)
				Expect(exists).To(BeTrue())
				Expect(gotPort).To(Equal(port))
			})

			It("returns the same port for an existing instance", func() {
				portManager, err := NewDefaultPortManager(nil)
				Expect(err).NotTo(HaveOccurred())

				instance := "test-instance"
				port1, err := portManager.AllocatePort(context.Background(), instance)
				Expect(err).NotTo(HaveOccurred())

				// Allocate again for the same instance
				port2, err := portManager.AllocatePort(context.Background(), instance)
				Expect(err).NotTo(HaveOccurred())
				Expect(port2).To(Equal(port1))
			})

			It("allocates multiple different ports", func() {
				portManager, err := NewDefaultPortManager(nil)
				Expect(err).NotTo(HaveOccurred())

				// Allocate ports for multiple instances
				instances := []string{"instance-1", "instance-2", "instance-3"}
				ports := make(map[uint16]bool)

				for _, instance := range instances {
					port, err := portManager.AllocatePort(context.Background(), instance)
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
				portManager, err := NewDefaultPortManager(nil)
				Expect(err).NotTo(HaveOccurred())

				// Allocate a port
				instance1 := "instance-1"
				_, err = portManager.AllocatePort(context.Background(), instance1)
				Expect(err).NotTo(HaveOccurred())

				// Release the port
				err = portManager.ReleasePort(instance1)
				Expect(err).NotTo(HaveOccurred())

				// Verify the port is released
				_, exists := portManager.GetPort(instance1)
				Expect(exists).To(BeFalse())

				// Allocate a new port for a different instance
				instance2 := "instance-2"
				port2, err := portManager.AllocatePort(context.Background(), instance2)
				Expect(err).NotTo(HaveOccurred())
				Expect(port2).To(BeNumerically(">", 0))

				// The new port might be the same as the old one (since it was released)
				// or different - both are valid with OS allocation
			})
		})
	})

	Describe("ReleasePort", func() {
		It("releases an allocated port", func() {
			portManager, err := NewDefaultPortManager(nil)
			Expect(err).NotTo(HaveOccurred())

			instance := "test-instance"
			_, err = portManager.AllocatePort(context.Background(), instance)
			Expect(err).NotTo(HaveOccurred())

			err = portManager.ReleasePort(instance)
			Expect(err).NotTo(HaveOccurred())

			// Verify the port is released
			_, exists := portManager.GetPort(instance)
			Expect(exists).To(BeFalse())

			// Verify we can allocate a port again
			newPort, err := portManager.AllocatePort(context.Background(), "another-instance")
			Expect(err).NotTo(HaveOccurred())
			Expect(newPort).To(BeNumerically(">", 0))
		})

		It("returns an error for non-existent instance", func() {
			portManager, err := NewDefaultPortManager(nil)
			Expect(err).NotTo(HaveOccurred())

			err = portManager.ReleasePort("non-existent")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("GetPort", func() {
		It("returns the correct port for an existing instance", func() {
			portManager, err := NewDefaultPortManager(nil)
			Expect(err).NotTo(HaveOccurred())

			instance := "test-instance"
			expectedPort, err := portManager.AllocatePort(context.Background(), instance)
			Expect(err).NotTo(HaveOccurred())

			gotPort, exists := portManager.GetPort(instance)
			Expect(exists).To(BeTrue())
			Expect(gotPort).To(Equal(expectedPort))
		})

		It("returns false for a non-existent instance", func() {
			portManager, err := NewDefaultPortManager(nil)
			Expect(err).NotTo(HaveOccurred())

			gotPort, exists := portManager.GetPort("non-existent")
			Expect(exists).To(BeFalse())
			Expect(gotPort).To(BeZero())
		})
	})

	Describe("ReservePort", func() {
		It("reserves an available port", func() {
			portManager, err := NewDefaultPortManager(nil)
			Expect(err).NotTo(HaveOccurred())

			instance := "test-instance"
			// Use a likely available port in a safe range
			portToReserve := uint16(55000)
			err = portManager.ReservePort(context.Background(), instance, portToReserve)

			// Note: This might fail if the port is already in use by another process
			// That's expected behavior with real OS allocation
			if err == nil {
				// Verify the port is reserved
				gotPort, exists := portManager.GetPort(instance)
				Expect(exists).To(BeTrue())
				Expect(gotPort).To(Equal(portToReserve))
			} else {
				// Port was not available, which is valid
				Expect(err.Error()).To(ContainSubstring("not available"))
			}
		})

		It("returns an error when port is already in use", func() {
			portManager, err := NewDefaultPortManager(nil)
			Expect(err).NotTo(HaveOccurred())

			portToReserve := uint16(55001)

			// Try to reserve the port for first instance
			err = portManager.ReservePort(context.Background(), "instance-1", portToReserve)
			if err != nil {
				// Port might already be in use by system, skip this test
				Skip("Port not available for testing")

				return
			}

			// Try to reserve the same port for another instance
			err = portManager.ReservePort(context.Background(), "instance-2", portToReserve)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already in use"))
		})

		It("returns an error when instance already has a different port", func() {
			portManager, err := NewDefaultPortManager(nil)
			Expect(err).NotTo(HaveOccurred())

			instance := "test-instance"
			port1 := uint16(55002)
			err = portManager.ReservePort(context.Background(), instance, port1)
			if err != nil {
				// Port might already be in use by system, skip this test
				Skip("Port not available for testing")

				return
			}

			// Try to reserve a different port for the same instance
			port2 := uint16(55003)
			err = portManager.ReservePort(context.Background(), instance, port2)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already has port"))
		})

		It("succeeds when reserving same port for same instance", func() {
			portManager, err := NewDefaultPortManager(nil)
			Expect(err).NotTo(HaveOccurred())

			instance := "test-instance"
			port := uint16(55004)
			err = portManager.ReservePort(context.Background(), instance, port)
			if err != nil {
				// Port might already be in use by system, skip this test
				Skip("Port not available for testing")

				return
			}

			// Reserve the same port for the same instance again
			err = portManager.ReservePort(context.Background(), instance, port)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns an error for invalid ports", func() {
			portManager, err := NewDefaultPortManager(nil)
			Expect(err).NotTo(HaveOccurred())

			err = portManager.ReservePort(context.Background(), "test-instance", 0)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid port"))
		})
	})

	Describe("Concurrent operations", func() {
		It("handles concurrent allocations safely", func() {
			portManager, err := NewDefaultPortManager(nil)
			Expect(err).NotTo(HaveOccurred())

			numGoroutines := 100
			var waitGroup sync.WaitGroup
			waitGroup.Add(numGoroutines)

			errors := make([]error, numGoroutines)
			ports := make([]uint16, numGoroutines)

			for goroutineIndex := range numGoroutines {
				go func(goroutineID int) {
					defer waitGroup.Done()
					instance := fmt.Sprintf("instance-%d", goroutineID)
					port, err := portManager.AllocatePort(context.Background(), instance)
					errors[goroutineID] = err
					if err == nil {
						ports[goroutineID] = port
					}
				}(goroutineIndex)
			}

			waitGroup.Wait()

			// Check results
			allocatedPorts := make(map[uint16]string)
			for errorIndex, err := range errors {
				if err == nil {
					port := ports[errorIndex]
					Expect(port).To(BeNumerically(">", 0))

					// Verify no port duplications
					instance, exists := allocatedPorts[port]
					if exists {
						Fail(fmt.Sprintf("Port %d allocated to both %s and instance-%d", port, instance, errorIndex))
					}
					allocatedPorts[port] = fmt.Sprintf("instance-%d", errorIndex)
				}
			}

			// Verify we have allocated correctly
			allocatedCount := len(portManager.instanceToPorts)
			Expect(allocatedCount).To(BeNumerically("<=", numGoroutines))
		})

		It("handles mixed concurrent operations safely", func() {
			portManager, err := NewDefaultPortManager(nil)
			Expect(err).NotTo(HaveOccurred())

			numGoroutines := 50
			var waitGroup sync.WaitGroup
			waitGroup.Add(numGoroutines * 3) // Allocate, Reserve, Release

			// Pre-allocate some instances for releasing
			instances := make([]string, numGoroutines)
			for i := range numGoroutines {
				instances[i] = fmt.Sprintf("instance-%d", i)
				_, err := portManager.AllocatePort(context.Background(), instances[i])
				Expect(err).NotTo(HaveOccurred())
			}

			// Concurrent allocations
			for goroutineIndex := range numGoroutines {
				go func(id int) {
					defer GinkgoRecover()
					defer waitGroup.Done()
					instance := fmt.Sprintf("new-instance-%d", id)
					_, err := portManager.AllocatePort(context.Background(), instance)
					Expect(err).NotTo(HaveOccurred())
				}(goroutineIndex)
			}

			// Concurrent reservations
			for goroutineIndex := range numGoroutines {
				go func(id int) {
					defer GinkgoRecover()
					defer waitGroup.Done()
					instance := fmt.Sprintf("reserve-instance-%d", id)
					port := uint16(55000 + id%1000)                                      // Use high port range to avoid conflicts
					err := portManager.ReservePort(context.Background(), instance, port) // Some might fail due to system usage
					if err != nil {
						// Error is expected - should be "port not available", "already in use" or "instance already has port"
						Expect(err.Error()).To(Or(
							ContainSubstring("not available"),
							ContainSubstring("already in use"),
							ContainSubstring("already has port"),
						))
					}
				}(goroutineIndex)
			}

			// Concurrent releases
			for i := range numGoroutines {
				go func(id int) {
					defer waitGroup.Done()
					err := portManager.ReleasePort(instances[id]) // Release pre-allocated instances
					Expect(err).NotTo(HaveOccurred())             // These should always succeed
				}(i)
			}

			waitGroup.Wait()
			// If we got here without deadlock or panic, the test passes
			Succeed()
		})
	})

	Describe("PreReconcile", func() {
		It("allocates ports for new instances", func() {
			portManager, err := NewDefaultPortManager(nil)
			Expect(err).NotTo(HaveOccurred())

			// Test with multiple instances
			instanceNames := []string{"instance-1", "instance-2", "instance-3"}
			err = portManager.PreReconcile(context.Background(), instanceNames)
			Expect(err).NotTo(HaveOccurred())

			// Verify ports were allocated
			for _, name := range instanceNames {
				port, exists := portManager.GetPort(name)
				Expect(exists).To(BeTrue())
				Expect(port).To(BeNumerically(">", 0))
			}
		})

		It("handles instances that already have ports", func() {
			portManager, err := NewDefaultPortManager(nil)
			Expect(err).NotTo(HaveOccurred())

			// Pre-allocate a port
			existingInstance := "existing-instance"
			existingPort, err := portManager.AllocatePort(context.Background(), existingInstance)
			Expect(err).NotTo(HaveOccurred())

			// Run PreReconcile with mix of existing and new instances
			instanceNames := []string{existingInstance, "new-instance"}
			err = portManager.PreReconcile(context.Background(), instanceNames)
			Expect(err).NotTo(HaveOccurred())

			// Verify existing instance kept its port
			port, exists := portManager.GetPort(existingInstance)
			Expect(exists).To(BeTrue())
			Expect(port).To(Equal(existingPort))

			// Verify new instance got a port
			port, exists = portManager.GetPort("new-instance")
			Expect(exists).To(BeTrue())
			Expect(port).To(BeNumerically(">", 0))
		})

		It("allocates successfully with OS port allocation", func() {
			portManager, err := NewDefaultPortManager(nil)
			Expect(err).NotTo(HaveOccurred())

			// With OS allocation, we should be able to allocate many ports
			instanceNames := []string{"instance-1", "instance-2", "instance-3", "instance-4", "instance-5"}
			err = portManager.PreReconcile(context.Background(), instanceNames)
			Expect(err).NotTo(HaveOccurred())

			// Verify all ports were allocated and are unique
			allocatedPorts := make(map[uint16]bool)
			for _, name := range instanceNames {
				port, exists := portManager.GetPort(name)
				Expect(exists).To(BeTrue())
				Expect(port).To(BeNumerically(">", 0))
				Expect(allocatedPorts[port]).To(BeFalse(), "Port %d was allocated twice", port)
				allocatedPorts[port] = true
			}
		})
	})

	Describe("PostReconcile", func() {
		It("completes without error", func() {
			portManager, err := NewDefaultPortManager(nil)
			Expect(err).NotTo(HaveOccurred())

			err = portManager.PostReconcile(context.Background())
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
