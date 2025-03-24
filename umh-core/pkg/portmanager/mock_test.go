package portmanager

import (
	"context"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("MockPortManager", func() {
	It("implements basic functionality correctly", func() {
		pm := NewMockPortManager()

		// Allocate a port
		instanceName := "test-instance"
		port, err := pm.AllocatePort(instanceName)
		Expect(err).NotTo(HaveOccurred())
		Expect(port).To(Equal(9000))
		Expect(pm.AllocatePortCalled).To(BeTrue())

		// Get the port
		gotPort, exists := pm.GetPort(instanceName)
		Expect(exists).To(BeTrue())
		Expect(gotPort).To(Equal(port))
		Expect(pm.GetPortCalled).To(BeTrue())

		// Release the port
		err = pm.ReleasePort(instanceName)
		Expect(err).NotTo(HaveOccurred())
		Expect(pm.ReleasePortCalled).To(BeTrue())

		// Verify port is released
		_, exists = pm.GetPort(instanceName)
		Expect(exists).To(BeFalse())
	})

	It("handles predefined results correctly", func() {
		pm := NewMockPortManager()

		// Set predefined return values
		expectedPort := 8888
		pm.AllocatePortResult = expectedPort
		expectedErr := errors.New("test error")
		pm.ReleasePortError = expectedErr

		// Allocate a port
		port, err := pm.AllocatePort("test-instance")
		Expect(err).NotTo(HaveOccurred())
		Expect(port).To(Equal(expectedPort))

		// Try to release with error
		err = pm.ReleasePort("test-instance")
		Expect(err).To(Equal(expectedErr))
	})

	It("handles port reservation correctly", func() {
		pm := NewMockPortManager()

		// Reserve a port
		instanceName := "test-instance"
		portToReserve := 8500
		err := pm.ReservePort(instanceName, portToReserve)
		Expect(err).NotTo(HaveOccurred())
		Expect(pm.ReservePortCalled).To(BeTrue())

		// Verify the port is reserved
		gotPort, exists := pm.GetPort(instanceName)
		Expect(exists).To(BeTrue())
		Expect(gotPort).To(Equal(portToReserve))

		// Try to reserve the same port for another instance
		err = pm.ReservePort("another-instance", portToReserve)
		Expect(err).To(HaveOccurred())
	})

	It("handles pre-reconciliation correctly", func() {
		pm := NewMockPortManager()

		// Test with multiple instances
		instanceNames := []string{"instance-1", "instance-2", "instance-3"}
		err := pm.PreReconcile(context.Background(), instanceNames)
		Expect(err).NotTo(HaveOccurred())
		Expect(pm.PreReconcileCalled).To(BeTrue())

		// Verify ports were allocated
		for _, name := range instanceNames {
			port, exists := pm.GetPort(name)
			Expect(exists).To(BeTrue())
			Expect(port).To(BeNumerically(">=", 9000))
		}

		// Test error handling
		pm.PreReconcileError = fmt.Errorf("test error")
		err = pm.PreReconcile(context.Background(), []string{"new-instance"})
		Expect(err).To(Equal(pm.PreReconcileError))
	})

	It("handles post-reconciliation correctly", func() {
		pm := NewMockPortManager()

		// Test normal operation
		err := pm.PostReconcile(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(pm.PostReconcileCalled).To(BeTrue())

		// Test error handling
		pm.PostReconcileError = fmt.Errorf("test error")
		err = pm.PostReconcile(context.Background())
		Expect(err).To(Equal(pm.PostReconcileError))
	})
})
