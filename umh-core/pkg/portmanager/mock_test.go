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
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("MockPortManager", func() {
	It("implements basic functionality correctly", func() {
		portManager := NewMockPortManager()

		// Allocate a port
		instanceName := "test-instance"
		port, err := portManager.AllocatePort(context.Background(), instanceName)
		Expect(err).NotTo(HaveOccurred())
		Expect(port).To(Equal(uint16(9000)))
		Expect(portManager.AllocatePortCalled).To(BeTrue())

		// Get the port
		gotPort, exists := portManager.GetPort(instanceName)
		Expect(exists).To(BeTrue())
		Expect(gotPort).To(Equal(port))
		Expect(portManager.GetPortCalled).To(BeTrue())

		// Release the port
		err = portManager.ReleasePort(instanceName)
		Expect(err).NotTo(HaveOccurred())
		Expect(portManager.ReleasePortCalled).To(BeTrue())

		// Verify port is released
		_, exists = portManager.GetPort(instanceName)
		Expect(exists).To(BeFalse())
	})

	It("handles predefined results correctly", func() {
		portManager := NewMockPortManager()

		// Set predefined return values
		expectedPort := uint16(8888)
		portManager.AllocatePortResult = expectedPort
		expectedErr := errors.New("test error")
		portManager.ReleasePortError = expectedErr

		// Allocate a port
		port, err := portManager.AllocatePort(context.Background(), "test-instance")
		Expect(err).NotTo(HaveOccurred())
		Expect(port).To(Equal(expectedPort))

		// Try to release with error
		err = portManager.ReleasePort("test-instance")
		Expect(err).To(Equal(expectedErr))
	})

	It("handles port reservation correctly", func() {
		portManager := NewMockPortManager()

		// Reserve a port
		instanceName := "test-instance"
		portToReserve := uint16(8500)
		err := portManager.ReservePort(context.Background(), instanceName, portToReserve)
		Expect(err).NotTo(HaveOccurred())
		Expect(portManager.ReservePortCalled).To(BeTrue())

		// Verify the port is reserved
		gotPort, exists := portManager.GetPort(instanceName)
		Expect(exists).To(BeTrue())
		Expect(gotPort).To(Equal(portToReserve))

		// Try to reserve the same port for another instance
		err = portManager.ReservePort(context.Background(), "another-instance", portToReserve)
		Expect(err).To(HaveOccurred())
	})

	It("handles pre-reconciliation correctly", func() {
		portManager := NewMockPortManager()

		// Test with multiple instances
		instanceNames := []string{"instance-1", "instance-2", "instance-3"}
		err := portManager.PreReconcile(context.Background(), instanceNames)
		Expect(err).NotTo(HaveOccurred())
		Expect(portManager.PreReconcileCalled).To(BeTrue())

		// Verify ports were allocated
		for _, name := range instanceNames {
			port, exists := portManager.GetPort(name)
			Expect(exists).To(BeTrue())
			Expect(port).To(BeNumerically(">=", 9000))
		}

		// Test error handling
		portManager.PreReconcileError = errors.New("test error")
		err = portManager.PreReconcile(context.Background(), []string{"new-instance"})
		Expect(err).To(Equal(portManager.PreReconcileError))
	})

	It("handles post-reconciliation correctly", func() {
		portManager := NewMockPortManager()

		// Test normal operation
		err := portManager.PostReconcile(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(portManager.PostReconcileCalled).To(BeTrue())

		// Test error handling
		portManager.PostReconcileError = errors.New("test error")
		err = portManager.PostReconcile(context.Background())
		Expect(err).To(Equal(portManager.PostReconcileError))
	})
})
