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

package dfc_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	benthosserviceconfig "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dfc"
)

// MockBenthosConfigManager is a mock implementation of the BenthosConfigManager interface for testing
type MockBenthosConfigManager struct {
	components           map[string]dfc.DataFlowComponentConfig
	addCalled            bool
	removeCalled         bool
	updateCalled         bool
	checkExistenceCalled bool
	shouldFailAdd        bool
	shouldFailRemove     bool
	shouldFailUpdate     bool
	shouldFailExistence  bool
}

// NewMockBenthosConfigManager creates a new MockBenthosConfigManager
func NewMockBenthosConfigManager() *MockBenthosConfigManager {
	return &MockBenthosConfigManager{
		components: make(map[string]dfc.DataFlowComponentConfig),
	}
}

// AddComponentToBenthosConfig adds a component to the benthos config
func (m *MockBenthosConfigManager) AddComponentToBenthosConfig(component dfc.DataFlowComponentConfig) error {
	m.addCalled = true
	if m.shouldFailAdd {
		return fmt.Errorf("mock error adding component")
	}
	m.components[component.Name] = component
	return nil
}

// RemoveComponentFromBenthosConfig removes a component from the benthos config
func (m *MockBenthosConfigManager) RemoveComponentFromBenthosConfig(componentName string) error {
	m.removeCalled = true
	if m.shouldFailRemove {
		return fmt.Errorf("mock error removing component")
	}
	delete(m.components, componentName)
	return nil
}

// UpdateComponentInBenthosConfig updates a component in the benthos config
func (m *MockBenthosConfigManager) UpdateComponentInBenthosConfig(component dfc.DataFlowComponentConfig) error {
	m.updateCalled = true
	if m.shouldFailUpdate {
		return fmt.Errorf("mock error updating component")
	}
	m.components[component.Name] = component
	return nil
}

// ComponentExistsInBenthosConfig checks if a component exists in the benthos config
func (m *MockBenthosConfigManager) ComponentExistsInBenthosConfig(componentName string) (bool, error) {
	m.checkExistenceCalled = true
	if m.shouldFailExistence {
		return false, fmt.Errorf("mock error checking component existence")
	}
	_, exists := m.components[componentName]
	return exists, nil
}

var _ = Describe("DataFlowComponent FSM", func() {
	var (
		ctx                context.Context
		mockConfigManager  *MockBenthosConfigManager
		testComponent      *dfc.DataFlowComponent
		componentConfig    dfc.DataFlowComponentConfig
		tempConfigFilePath string
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockConfigManager = NewMockBenthosConfigManager()

		// Create a temporary directory for config files
		tempDir, err := os.MkdirTemp("", "dfc-test")
		Expect(err).NotTo(HaveOccurred())
		tempConfigFilePath = filepath.Join(tempDir, "test-config.yaml")

		// Basic component config
		componentConfig = dfc.DataFlowComponentConfig{
			Name:         "test-component",
			DesiredState: "stopped",
			VersionUUID:  "test-uuid-123",
			ServiceConfig: benthosserviceconfig.BenthosServiceConfig{
				Input: map[string]interface{}{
					"generate": map[string]interface{}{
						"mapping":  "root = \"hello world from test!\"",
						"interval": "1s",
						"count":    0,
					},
				},
				Output: map[string]interface{}{
					"stdout": map[string]interface{}{},
				},
			},
		}

		// Create a new DataFlowComponent
		testComponent = dfc.NewDataFlowComponent(componentConfig, mockConfigManager)
		Expect(testComponent).NotTo(BeNil())
	})

	AfterEach(func() {
		// Clean up the temporary directory
		if tempConfigFilePath != "" {
			os.RemoveAll(filepath.Dir(tempConfigFilePath))
		}
	})

	Describe("State Transitions", func() {
		It("should start in the Stopped state", func() {
			Expect(testComponent.GetCurrentFSMState()).To(Equal(dfc.OperationalStateStopped))
		})

		It("should transition to Active when the desired state is set to Active", func() {
			// Set the desired state to Active
			err := testComponent.SetDesiredFSMState(dfc.OperationalStateActive)
			Expect(err).NotTo(HaveOccurred())

			// First reconcile call should transition from Stopped to Starting
			err, reconciled := testComponent.Reconcile(ctx, 1)
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())
			Expect(testComponent.GetCurrentFSMState()).To(Equal(dfc.OperationalStateStarting))

			// Second reconcile call should transition from Starting to Active
			err, reconciled = testComponent.Reconcile(ctx, 2)
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())
			Expect(testComponent.GetCurrentFSMState()).To(Equal(dfc.OperationalStateActive))

			// Verify that the component was added to the benthos config
			Expect(mockConfigManager.addCalled).To(BeTrue())
		})

		It("should transition to Stopped when the desired state is set to Stopped", func() {
			// Set the desired state to Active and reconcile to get to Active state
			err := testComponent.SetDesiredFSMState(dfc.OperationalStateActive)
			Expect(err).NotTo(HaveOccurred())
			err, _ = testComponent.Reconcile(ctx, 1) // -> Starting
			Expect(err).NotTo(HaveOccurred())
			err, _ = testComponent.Reconcile(ctx, 2) // -> Active
			Expect(err).NotTo(HaveOccurred())
			Expect(testComponent.GetCurrentFSMState()).To(Equal(dfc.OperationalStateActive))

			// Now set the desired state to Stopped
			err = testComponent.SetDesiredFSMState(dfc.OperationalStateStopped)
			Expect(err).NotTo(HaveOccurred())

			// First reconcile call should transition from Active to Stopping
			err, reconciled := testComponent.Reconcile(ctx, 3)
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())
			Expect(testComponent.GetCurrentFSMState()).To(Equal(dfc.OperationalStateStopping))

			// Second reconcile call should transition from Stopping to Stopped
			err, reconciled = testComponent.Reconcile(ctx, 4)
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())
			Expect(testComponent.GetCurrentFSMState()).To(Equal(dfc.OperationalStateStopped))

			// Verify that the component was removed from the benthos config
			Expect(mockConfigManager.removeCalled).To(BeTrue())
		})
	})

	Describe("Benthos Config Management", func() {
		It("should add the component to the benthos config when starting", func() {
			// Set the desired state to Active
			err := testComponent.SetDesiredFSMState(dfc.OperationalStateActive)
			Expect(err).NotTo(HaveOccurred())

			// Reconcile to trigger config modification
			err, _ = testComponent.Reconcile(ctx, 1)
			Expect(err).NotTo(HaveOccurred())

			// Verify that the component was added to the benthos config
			Expect(mockConfigManager.addCalled).To(BeTrue())
		})

		It("should remove the component from the benthos config when stopping", func() {
			// Set the desired state to Active and reconcile to get to Active state
			err := testComponent.SetDesiredFSMState(dfc.OperationalStateActive)
			Expect(err).NotTo(HaveOccurred())
			err, _ = testComponent.Reconcile(ctx, 1) // -> Starting
			Expect(err).NotTo(HaveOccurred())
			err, _ = testComponent.Reconcile(ctx, 2) // -> Active
			Expect(err).NotTo(HaveOccurred())

			// Now set the desired state to Stopped
			err = testComponent.SetDesiredFSMState(dfc.OperationalStateStopped)
			Expect(err).NotTo(HaveOccurred())

			// Reconcile to trigger config modification
			err, _ = testComponent.Reconcile(ctx, 3)
			Expect(err).NotTo(HaveOccurred())

			// Verify that the component was removed from the benthos config
			Expect(mockConfigManager.removeCalled).To(BeTrue())
		})

		It("should update the component in the benthos config when already active", func() {
			// Set the desired state to Active and reconcile to get to Active state
			err := testComponent.SetDesiredFSMState(dfc.OperationalStateActive)
			Expect(err).NotTo(HaveOccurred())
			err, _ = testComponent.Reconcile(ctx, 1) // -> Starting
			Expect(err).NotTo(HaveOccurred())
			err, _ = testComponent.Reconcile(ctx, 2) // -> Active
			Expect(err).NotTo(HaveOccurred())

			// Make a change to the component config
			testComponent.Config.ServiceConfig.Input = map[string]interface{}{
				"generate": map[string]interface{}{
					"mapping":  "root = \"updated hello world!\"",
					"interval": "2s",
					"count":    0,
				},
			}

			// Reset mock flags
			mockConfigManager.updateCalled = false

			// Reconcile again to trigger config update
			err, _ = testComponent.Reconcile(ctx, 3)
			Expect(err).NotTo(HaveOccurred())

			// Verify that the component was updated in the benthos config
			Expect(mockConfigManager.updateCalled).To(BeTrue())
		})
	})
})
