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
		It("creates a new port manager successfully", func() {
			pm := NewDefaultPortManager()
			Expect(pm).NotTo(BeNil())
			Expect(pm.instanceToPorts).NotTo(BeNil())
		})

		It("returns the same singleton instance on subsequent calls", func() {
			pm1 := NewDefaultPortManager()
			pm2 := NewDefaultPortManager()
			Expect(pm1).To(BeIdenticalTo(pm2))
		})
	})

	Describe("AllocatePort", func() {
		Context("basic allocation", func() {
			It("allocates a port successfully", func() {
				pm := NewDefaultPortManager()

				instance := "test-instance"
				port, err := pm.AllocatePort(instance)
				Expect(err).NotTo(HaveOccurred())
				Expect(port).To(BeNumerically(">", 0))

				gotPort, exists := pm.GetPort(instance)
				Expect(exists).To(BeTrue())
				Expect(gotPort).To(Equal(port))
			})

			It("returns the same port for an existing instance", func() {
				pm := NewDefaultPortManager()

				instance := "test-instance"
				port1, err := pm.AllocatePort(instance)
				Expect(err).NotTo(HaveOccurred())

				// Allocate again for the same instance
				port2, err := pm.AllocatePort(instance)
				Expect(err).NotTo(HaveOccurred())
				Expect(port2).To(Equal(port1))
			})

			It("allocates different ports for different instances", func() {
				pm := NewDefaultPortManager()

				port1, err := pm.AllocatePort("instance-1")
				Expect(err).NotTo(HaveOccurred())

				port2, err := pm.AllocatePort("instance-2")
				Expect(err).NotTo(HaveOccurred())

				Expect(port1).NotTo(Equal(port2))
			})
		})
	})

	Describe("ReleasePort", func() {
		It("releases an allocated port", func() {
			pm := NewDefaultPortManager()

			instance := "test-instance"
			_, err := pm.AllocatePort(instance)
			Expect(err).NotTo(HaveOccurred())

			err = pm.ReleasePort(instance)
			Expect(err).NotTo(HaveOccurred())

			// Verify the port is released
			_, exists := pm.GetPort(instance)
			Expect(exists).To(BeFalse())
		})

		It("returns an error for non-existent instance", func() {
			pm := NewDefaultPortManager()

			err := pm.ReleasePort("non-existent")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("GetPort", func() {
		It("returns the correct port for an existing instance", func() {
			pm := NewDefaultPortManager()

			instance := "test-instance"
			expectedPort, err := pm.AllocatePort(instance)
			Expect(err).NotTo(HaveOccurred())

			gotPort, exists := pm.GetPort(instance)
			Expect(exists).To(BeTrue())
			Expect(gotPort).To(Equal(expectedPort))
		})

		It("returns false for a non-existent instance", func() {
			pm := NewDefaultPortManager()

			gotPort, exists := pm.GetPort("non-existent")
			Expect(exists).To(BeFalse())
			Expect(gotPort).To(BeZero())
		})
	})

	Describe("ReservePort", func() {
		It("reserves an available port", func() {
			pm := NewDefaultPortManager()

			instance := "test-instance"
			portToReserve := uint16(8500)
			err := pm.ReservePort(instance, portToReserve)
			Expect(err).NotTo(HaveOccurred())

			// Verify the port is reserved
			gotPort, exists := pm.GetPort(instance)
			Expect(exists).To(BeTrue())
			Expect(gotPort).To(Equal(portToReserve))
		})

		It("returns an error when port is already in use", func() {
			pm := NewDefaultPortManager()

			portToReserve := uint16(8500)
			err := pm.ReservePort("instance-1", portToReserve)
			Expect(err).NotTo(HaveOccurred())

			// Try to reserve the same port for another instance
			err = pm.ReservePort("instance-2", portToReserve)
			Expect(err).To(HaveOccurred())
		})

		It("returns an error when instance already has a different port", func() {
			pm := NewDefaultPortManager()

			instance := "test-instance"
			port1 := uint16(8500)
			err := pm.ReservePort(instance, port1)
			Expect(err).NotTo(HaveOccurred())

			// Try to reserve a different port for the same instance
			port2 := uint16(8600)
			err = pm.ReservePort(instance, port2)
			Expect(err).To(HaveOccurred())
		})

		It("succeeds when reserving same port for same instance", func() {
			pm := NewDefaultPortManager()

			instance := "test-instance"
			port := uint16(8500)
			err := pm.ReservePort(instance, port)
			Expect(err).NotTo(HaveOccurred())

			// Reserve the same port for the same instance again
			err = pm.ReservePort(instance, port)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns an error for invalid ports", func() {
			pm := NewDefaultPortManager()

			err := pm.ReservePort("test-instance", 0)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Concurrent operations", func() {
		It("handles concurrent allocations safely", func() {
			pm := NewDefaultPortManager()

			numGoroutines := 100
			var wg sync.WaitGroup
			wg.Add(numGoroutines)

			errors := make([]error, numGoroutines)
			ports := make([]uint16, numGoroutines)

			for i := 0; i < numGoroutines; i++ {
				go func(id int) {
					defer wg.Done()
					instance := fmt.Sprintf("instance-%d", id)
					port, err := pm.AllocatePort(instance)
					errors[id] = err
					if err == nil {
						ports[id] = port
					}
				}(i)
			}

			wg.Wait()

			// Check results
			allocatedPorts := make(map[uint16]string)
			successCount := 0
			for i, err := range errors {
				if err == nil {
					successCount++
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
			Expect(successCount).To(Equal(numGoroutines))
			allocatedCount := len(pm.instanceToPorts)
			Expect(allocatedCount).To(Equal(numGoroutines))
		})

		It("handles mixed concurrent operations safely", func() {
			pm := NewDefaultPortManager()

			numGoroutines := 50
			var wg sync.WaitGroup
			wg.Add(numGoroutines * 3) // Allocate, Reserve, Release

			// Pre-allocate some instances for releasing
			instances := make([]string, numGoroutines)
			for i := 0; i < numGoroutines; i++ {
				instances[i] = fmt.Sprintf("instance-%d", i)
				_, err := pm.AllocatePort(instances[i])
				Expect(err).NotTo(HaveOccurred())
			}

			// Concurrent allocations
			for i := 0; i < numGoroutines; i++ {
				go func(id int) {
					defer GinkgoRecover()
					defer wg.Done()
					instance := fmt.Sprintf("new-instance-%d", id)
					_, err := pm.AllocatePort(instance)
					Expect(err).NotTo(HaveOccurred())
				}(i)
			}

			// Concurrent reservations
			for i := 0; i < numGoroutines; i++ {
				go func(id int) {
					defer GinkgoRecover()
					defer wg.Done()
					instance := fmt.Sprintf("reserve-instance-%d", id)
					port := uint16(8000 + id)             // Use different ports to avoid conflicts
					err := pm.ReservePort(instance, port) // Some might fail due to port conflicts
					if err != nil {
						// Error is expected - should be "port is not available" or "instance already has port"
						Expect(err.Error()).To(Or(
							ContainSubstring("not available"),
							ContainSubstring("already has port"),
						))
					}
				}(i)
			}

			// Concurrent releases
			for i := 0; i < numGoroutines; i++ {
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
			pm := NewDefaultPortManager()

			// Test with multiple instances
			instanceNames := []string{"instance-1", "instance-2", "instance-3"}
			err := pm.PreReconcile(context.Background(), instanceNames)
			Expect(err).NotTo(HaveOccurred())

			// Verify ports were allocated
			for _, name := range instanceNames {
				port, exists := pm.GetPort(name)
				Expect(exists).To(BeTrue())
				Expect(port).To(BeNumerically(">", 0))
			}
		})

		It("handles instances that already have ports", func() {
			pm := NewDefaultPortManager()

			// Pre-allocate a port
			existingInstance := "existing-instance"
			existingPort, err := pm.AllocatePort(existingInstance)
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
	})

	Describe("PostReconcile", func() {
		It("completes without error", func() {
			pm := NewDefaultPortManager()

			err := pm.PostReconcile(context.Background())
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
