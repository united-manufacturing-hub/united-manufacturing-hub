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

package benthos

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/backoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/portmanager"
	benthossvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
)

// TODO: needs to be refactored based on the test approach in umh-core/test/fsm/s6/manager_test.go
// and then also moved in to a shared package

// createMockBenthosManagerInstance creates a BenthosInstance specifically for the manager tests
func createMockBenthosManagerInstance(name string, mockService benthossvc.IBenthosService, desiredState string) *BenthosInstance {
	cfg := config.BenthosConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            name,
			DesiredFSMState: desiredState,
		},
		BenthosServiceConfig: config.BenthosServiceConfig{},
	}

	instance := NewBenthosInstance(baseBenthosDir, cfg)
	// Replace the service with our mock
	instance.service = mockService
	return instance
}

// createMockBenthosManager creates a BenthosManager with a mock service for testing
func createMockBenthosManager(name string) (*BenthosManager, *benthossvc.MockBenthosService) {
	mockService := benthossvc.NewMockBenthosService()
	manager := NewBenthosManager(name)
	return manager, mockService
}

// createBenthosConfig creates a test BenthosConfig with the given name and desired state
func createBenthosConfig(name, desiredState string) config.BenthosConfig {
	return config.BenthosConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            name,
			DesiredFSMState: desiredState,
		},
		BenthosServiceConfig: config.BenthosServiceConfig{},
	}
}

// setupServiceState configures the mock service state for a given service
func setupServiceState(mockService *benthossvc.MockBenthosService, serviceName string, flags benthossvc.ServiceStateFlags) {
	mockService.SetServiceState(serviceName, flags)

	if mockService.ServiceStates == nil {
		mockService.ServiceStates = make(map[string]*benthossvc.ServiceInfo)
	}

	if mockService.ServiceStates[serviceName] == nil {
		mockService.ServiceStates[serviceName] = &benthossvc.ServiceInfo{}
	}

	mockService.ServiceStates[serviceName].BenthosStatus.HealthCheck = benthossvc.HealthCheck{
		IsLive:  flags.IsHealthchecksPassed,
		IsReady: flags.IsHealthchecksPassed,
	}

	// Set up metrics state if service is processing
	if flags.HasProcessingActivity {
		mockService.ServiceStates[serviceName].BenthosStatus.MetricsState = &benthossvc.BenthosMetricsState{
			IsActive: true,
		}
	}

	// Set up S6 observed state
	if flags.IsS6Running {
		mockService.ServiceStates[serviceName].S6ObservedState.ServiceInfo.Uptime = 15
	}

	// Mark the service as existing in the mock service
	mockService.ExistingServices[serviceName] = true

	// Setup default config results
	mockService.GetConfigResult = config.BenthosServiceConfig{}
}

// setupServiceInManager adds a service to the manager with the mock instance
func setupServiceInManager(manager *BenthosManager, mockService benthossvc.IBenthosService, serviceName string, desiredState string) {
	instance := createMockBenthosManagerInstance(serviceName, mockService, desiredState)
	manager.BaseFSMManager.AddInstanceForTest(serviceName, instance)
}

// waitForStateBenthosManager repeatedly reconciles until the desired state is reached or maxAttempts is exceeded
func waitForStateBenthosManager(ctx context.Context, manager *BenthosManager, config config.FullConfig, startTick uint64, desiredState string, maxAttempts int) (uint64, error) {
	tick := startTick
	var lastErr error

	for i := 0; i < maxAttempts; i++ {
		lastErr, _ = manager.Reconcile(ctx, config, tick)
		if lastErr != nil {
			return tick, lastErr
		}

		// Check if all instances have reached the desired state
		allInDesiredState := true
		GinkgoWriter.Printf("\n=== Attempt %d/%d waiting for state %s ===\n", i+1, maxAttempts, desiredState)
		for name, instance := range manager.GetInstances() {
			currentState := instance.GetCurrentFSMState()
			if currentState != desiredState {
				allInDesiredState = false
				GinkgoWriter.Printf("Instance %s: current=%s (waiting for %s)\n",
					name, currentState, desiredState)
			} else {
				GinkgoWriter.Printf("Instance %s: reached target state %s\n",
					name, desiredState)
			}

			// If it's a BenthosInstance, print more details
			if benthosInstance, ok := instance.(*BenthosInstance); ok {
				if benthosInstance.baseFSMInstance.GetError() != nil {
					GinkgoWriter.Printf("  Error: %v\n", benthosInstance.baseFSMInstance.GetError())
				}
				GinkgoWriter.Printf("  Desired state: %s\n", benthosInstance.GetDesiredFSMState())
			}
		}

		if allInDesiredState {
			GinkgoWriter.Printf("All instances reached target state %s\n", desiredState)
			return tick, nil
		}

		tick++
	}

	return tick, fmt.Errorf("failed to reach state %s after %d attempts", desiredState, maxAttempts)
}

// transitionToIdle helps transition a service through all startup states to reach idle
func transitionToIdle(ctx context.Context, manager *BenthosManager, mockService *benthossvc.MockBenthosService, serviceName string, config config.FullConfig, startTick uint64) (uint64, error) {
	tick := startTick

	// Add the instance to the manager
	setupServiceInManager(manager, mockService, serviceName, OperationalStateActive)

	// Setup initial mock service response for creation
	mockService.ExistingServices[serviceName] = true

	// Setup service as created but stopped
	setupServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
		IsS6Running: false,
		S6FSMState:  s6fsm.OperationalStateStopped,
	})

	// First reconcile to update instance state
	err, _ := manager.Reconcile(ctx, config, tick)
	if err != nil {
		return tick, fmt.Errorf("failed initial reconcile: %w", err)
	}
	tick++

	// Wait for instance to reach stopped state
	tick, err = waitForStateBenthosManager(ctx, manager, config, tick, OperationalStateStopped, 10)
	if err != nil {
		return tick, fmt.Errorf("failed to reach stopped state: %w", err)
	}

	// Configure service for starting phase
	setupServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
		IsS6Running:            true,
		S6FSMState:             s6fsm.OperationalStateRunning,
		IsConfigLoaded:         true,
		IsHealthchecksPassed:   true,
		IsRunningWithoutErrors: true,
	})

	// Wait for instance to reach idle state
	tick, err = waitForStateBenthosManager(ctx, manager, config, tick, OperationalStateIdle, 20)
	if err != nil {
		return tick, fmt.Errorf("failed to reach idle state: %w", err)
	}

	return tick, nil
}

var _ = Describe("BenthosManager", func() {
	var (
		manager     *BenthosManager
		mockService *benthossvc.MockBenthosService
		ctx         context.Context
		tick        uint64
	)

	BeforeEach(func() {
		manager, mockService = createMockBenthosManager("test-manager")
		ctx = context.Background()
		tick = uint64(0)

		// Reset the mock service state
		mockService.ExistingServices = make(map[string]bool)
		mockService.ServiceStates = make(map[string]*benthossvc.ServiceInfo)
	})

	Context("Basic Operations", func() {
		It("should handle empty config without errors", func() {
			emptyConfig := config.FullConfig{
				Benthos: []config.BenthosConfig{},
			}

			err, _ := manager.Reconcile(ctx, emptyConfig, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(manager.GetInstances()).To(BeEmpty())
		})

		It("should manage a service that's manually added", func() {
			serviceName := "test-service"
			var err error

			// Setup the instance
			setupServiceInManager(manager, mockService, serviceName, OperationalStateActive)
			setupServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				S6FSMState:             s6fsm.OperationalStateRunning,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   true,
				IsRunningWithoutErrors: true,
			})

			// Create config that references this service
			benthosConfig := createBenthosConfig(serviceName, OperationalStateActive)
			fullConfig := config.FullConfig{
				Benthos: []config.BenthosConfig{benthosConfig},
			}

			// Wait for service to reach idle state
			tick, err = waitForStateBenthosManager(ctx, manager, fullConfig, tick, OperationalStateIdle, 20)
			Expect(err).NotTo(HaveOccurred())

			// Verify instance state
			instance, exists := manager.GetInstance(serviceName)
			Expect(exists).To(BeTrue())
			Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateIdle))
			Expect(instance.GetDesiredFSMState()).To(Equal(OperationalStateActive))
		})
	})

	Context("Service Lifecycle", func() {
		It("should transition through all states and handle removal", func() {
			serviceName := "test-lifecycle"
			benthosConfig := createBenthosConfig(serviceName, OperationalStateActive)
			fullConfig := config.FullConfig{
				Benthos: []config.BenthosConfig{benthosConfig},
			}

			// Get to idle state first
			tick, err := transitionToIdle(ctx, manager, mockService, serviceName, fullConfig, tick)
			Expect(err).NotTo(HaveOccurred())

			// Setup for transition to active
			setupServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				S6FSMState:             s6fsm.OperationalStateRunning,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   true,
				IsRunningWithoutErrors: true,
				HasProcessingActivity:  true,
			})

			// Wait for active state
			tick, err = waitForStateBenthosManager(ctx, manager, fullConfig, tick, OperationalStateActive, 10)
			Expect(err).NotTo(HaveOccurred())

			// Simulate degradation
			setupServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				S6FSMState:             s6fsm.OperationalStateRunning,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   false,
				IsRunningWithoutErrors: false,
				HasProcessingActivity:  true,
			})

			// Wait for degraded state
			tick, err = waitForStateBenthosManager(ctx, manager, fullConfig, tick, OperationalStateDegraded, 10)
			Expect(err).NotTo(HaveOccurred())

			// Now remove the service from config
			emptyConfig := config.FullConfig{
				Benthos: []config.BenthosConfig{},
			}

			// We need to transition from degraded to stopped before we can remove
			// First, get the instance
			instance, found := manager.GetInstance(serviceName)
			Expect(found).To(BeTrue(), "Service should exist before removal")

			// Update mock service state to allow transitioning to stopped
			setupServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:          false,
				S6FSMState:           s6fsm.OperationalStateStopped,
				IsConfigLoaded:       false,
				IsHealthchecksPassed: false,
			})

			// Reconcile to move to stopped state
			err, _ = manager.Reconcile(ctx, emptyConfig, tick)
			Expect(err).NotTo(HaveOccurred())
			tick += 5

			// Wait for service to reach stopped state by reconciling multiple times
			maxStopAttempts := 20
			for attempt := 0; attempt < maxStopAttempts; attempt++ {
				err, _ = manager.Reconcile(ctx, emptyConfig, tick)
				Expect(err).NotTo(HaveOccurred())
				tick += 5

				// Check if service has reached stopped state
				if instance, found := manager.GetInstance(serviceName); found {
					if instance.GetCurrentFSMState() == OperationalStateStopped {
						break
					}
					GinkgoWriter.Printf("Service %s currently in state %s (attempt %d/%d)\n",
						serviceName, instance.GetCurrentFSMState(), attempt+1, maxStopAttempts)
				} else {
					GinkgoWriter.Printf("Service %s not found (attempt %d/%d)\n", serviceName, attempt+1, maxStopAttempts)
					break
				}

				// If we reach max attempts, fail the test
				if attempt == maxStopAttempts-1 {
					Fail(fmt.Sprintf("Service did not reach stopped state after %d attempts", maxStopAttempts))
				}
			}

			// Verify we're in stopped state before proceeding
			instance, found = manager.GetInstance(serviceName)
			Expect(found).To(BeTrue(), "Service should exist after transitioning to stopped")
			Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateStopped),
				"Service should be in stopped state before removal")

			// Now that it's stopped, we can remove it
			err = instance.Remove(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Check removal with reconciliation multiple times
			maxRemoveAttempts := 20
			for attempt := 0; attempt < maxRemoveAttempts; attempt++ {
				// Reconcile and advance tick
				err, _ = manager.Reconcile(ctx, emptyConfig, tick)
				Expect(err).NotTo(HaveOccurred())
				tick += 20 // Use larger increments to bypass rate limiting

				// Check if service has been removed
				if len(manager.GetInstances()) == 0 {
					GinkgoWriter.Printf("Service %s successfully removed after %d attempts\n",
						serviceName, attempt+1)
					break
				}

				// Print status for debugging
				if instance, found := manager.GetInstance(serviceName); found {
					GinkgoWriter.Printf("Service %s still exists in state %s (attempt %d/%d)\n",
						serviceName, instance.GetCurrentFSMState(), attempt+1, maxRemoveAttempts)
				} else {
					GinkgoWriter.Printf("Service no longer in manager but instances not cleared (attempt %d/%d)\n",
						attempt+1, maxRemoveAttempts)
				}

				// If we reach max attempts, fail the test
				if attempt == maxRemoveAttempts-1 {
					Fail(fmt.Sprintf("Service was not removed after %d attempts", maxRemoveAttempts))
				}
			}

			// Final verification
			Expect(manager.GetInstances()).To(BeEmpty(), "All services should be removed")
		})

		It("should transition from active to stopped and back to active", func() {
			serviceName := "test-active-to-stopped-to-active"
			benthosConfig := createBenthosConfig(serviceName, OperationalStateActive)
			fullConfig := config.FullConfig{
				Benthos: []config.BenthosConfig{benthosConfig},
			}

			// First, get to idle state
			tick, err := transitionToIdle(ctx, manager, mockService, serviceName, fullConfig, tick)
			Expect(err).NotTo(HaveOccurred())

			// Setup for transition to active
			setupServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				S6FSMState:             s6fsm.OperationalStateRunning,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   true,
				IsRunningWithoutErrors: true,
				HasProcessingActivity:  true,
			})

			// Wait for active state
			tick, err = waitForStateBenthosManager(ctx, manager, fullConfig, tick, OperationalStateActive, 10)
			Expect(err).NotTo(HaveOccurred())

			// Get the instance reference for later verification
			instance, found := manager.GetInstance(serviceName)
			Expect(found).To(BeTrue(), "Service should exist before changing state")
			benthosInstance, ok := instance.(*BenthosInstance)
			Expect(ok).To(BeTrue(), "Instance should be a BenthosInstance")

			// Now change the desired state to stopped
			GinkgoWriter.Printf("Changing %s desired state from %s to %s\n",
				serviceName, benthosInstance.GetDesiredFSMState(), OperationalStateStopped)

			fullConfig.Benthos[0].FSMInstanceConfig.DesiredFSMState = OperationalStateStopped

			// Update mock service to allow transitioning to stopped
			setupServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:          false,
				S6FSMState:           s6fsm.OperationalStateStopped,
				IsConfigLoaded:       false,
				IsHealthchecksPassed: false,
			})

			// Wait for stopped state
			tick, err = waitForStateBenthosManager(ctx, manager, fullConfig, tick, OperationalStateStopped, 15)
			Expect(err).NotTo(HaveOccurred())

			// Verify we're in stopped state
			Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateStopped),
				"Service should be in stopped state before reactivating")

			// Now change desired state back to active
			GinkgoWriter.Printf("Changing %s desired state from %s to %s\n",
				serviceName, benthosInstance.GetDesiredFSMState(), OperationalStateActive)

			err = benthosInstance.SetDesiredFSMState(OperationalStateActive)
			Expect(err).NotTo(HaveOccurred())

			// Setup service for starting phase
			setupServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				S6FSMState:             s6fsm.OperationalStateRunning,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   true,
				IsRunningWithoutErrors: true,
			})

			// Wait for instance to reach idle state first
			tick, err = waitForStateBenthosManager(ctx, manager, fullConfig, tick, OperationalStateIdle, 20)
			Expect(err).NotTo(HaveOccurred(), "Service should transition to idle state after reactivation")

			// Setup for transition to active again
			setupServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				S6FSMState:             s6fsm.OperationalStateRunning,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   true,
				IsRunningWithoutErrors: true,
				HasProcessingActivity:  true,
			})

			// Wait for active state
			tick, err = waitForStateBenthosManager(ctx, manager, fullConfig, tick, OperationalStateActive, 10)
			Expect(err).NotTo(HaveOccurred(), "Service should transition back to active state")

			// Verify instance state
			Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateActive),
				"Service should be in active state after reactivation")
			Expect(instance.GetDesiredFSMState()).To(Equal(OperationalStateActive),
				"Service should retain active as desired state")
		})
	})

	Context("Error Handling", func() {
		It("should handle startup failures and retry appropriately", func() {
			serviceName := "test-error"
			benthosConfig := createBenthosConfig(serviceName, OperationalStateActive)
			fullConfig := config.FullConfig{
				Benthos: []config.BenthosConfig{benthosConfig},
			}

			// Add the instance directly
			setupServiceInManager(manager, mockService, serviceName, OperationalStateActive)

			// Setup initial failed state
			setupServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:          false,
				S6FSMState:           s6fsm.OperationalStateStopped,
				IsConfigLoaded:       false,
				IsHealthchecksPassed: false,
			})

			// Should stay in starting state while S6 is not running
			tick, err := waitForStateBenthosManager(ctx, manager, fullConfig, tick, OperationalStateStarting, 5)
			Expect(err).NotTo(HaveOccurred())

			// Simulate S6 starting but config failing to load
			setupServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:          true,
				S6FSMState:           s6fsm.OperationalStateRunning,
				IsConfigLoaded:       false,
				IsHealthchecksPassed: false,
			})

			// Should move to config loading state
			instance, exists := manager.GetInstance(serviceName)
			Expect(exists).To(BeTrue())

			err, _ = manager.Reconcile(ctx, fullConfig, tick)
			Expect(err).NotTo(HaveOccurred())
			tick++

			// Should move to config loading state after S6 is running
			Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateStartingConfigLoading))

			// Now fail the service by stopping S6
			setupServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:          false, // This is what triggers EventStartFailed
				S6FSMState:           s6fsm.OperationalStateStopped,
				IsConfigLoaded:       false,
				IsHealthchecksPassed: false,
			})

			// This should trigger the transition back to starting
			err, _ = manager.Reconcile(ctx, fullConfig, tick)
			Expect(err).NotTo(HaveOccurred())
			tick++

			// Should have gone back to starting state immediately
			Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateStarting),
				"Service should go back to starting state when S6 fails")

			// Finally let it succeed
			setupServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				S6FSMState:             s6fsm.OperationalStateRunning,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   true,
				IsRunningWithoutErrors: true,
			})

			// Should reach idle state
			tick, err = waitForStateBenthosManager(ctx, manager, fullConfig, tick, OperationalStateIdle, 20)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle 'service not found' errors gracefully", func() {
			serviceName := "nonexistent-service"

			// Create config for the non-existent service
			benthosConfig := createBenthosConfig(serviceName, OperationalStateActive)
			fullConfig := config.FullConfig{
				Benthos: []config.BenthosConfig{benthosConfig},
			}

			// Create and configure mock HTTP client
			mockHTTPClient := benthossvc.NewMockHTTPClient()
			mockHTTPClient.SetServiceNotFound(serviceName)

			// Configure the manager's mock service (not a new one)
			mockService.HTTPClient = mockHTTPClient
			mockService.AddBenthosToS6ManagerError = nil

			// Add the instance to the manager with the mock service
			setupServiceInManager(manager, mockService, serviceName, OperationalStateActive)

			// Mark service as existing in mock service
			mockService.ExistingServices[serviceName] = true

			// Set up a mock port manager
			mockPortManager := portmanager.NewMockPortManager()
			manager.WithPortManager(mockPortManager)

			// Allocate a port for the service
			allocatedPort, err := mockPortManager.AllocatePort(serviceName)
			Expect(err).NotTo(HaveOccurred())

			// Set the port in the instance config
			instance, exists := manager.GetInstance(serviceName)
			Expect(exists).To(BeTrue())
			benthosInstance, ok := instance.(*BenthosInstance)
			Expect(ok).To(BeTrue())
			benthosInstance.config.MetricsPort = allocatedPort

			// First reconcile attempt
			err, reconciled := manager.Reconcile(ctx, fullConfig, tick)
			tick++

			// After first reconcile, service should be created
			mockService.ExistingServices[serviceName] = true

			// Verify error handling
			Expect(err).NotTo(HaveOccurred(), "Reconcile should handle service not found gracefully")
			Expect(reconciled).To(BeTrue(), "Reconcile should indicate changes were made")

			// Wait for instance to go through lifecycle states
			tick, err = waitForStateBenthosInstance(ctx, benthosInstance, tick, internalfsm.LifecycleStateCreating, 5)
			Expect(err).NotTo(HaveOccurred(), "Should transition to creating state")

			tick, err = waitForStateBenthosInstance(ctx, benthosInstance, tick, OperationalStateStopped, 5)
			Expect(err).NotTo(HaveOccurred(), "Should transition to stopped state")

			// Set desired state to active to trigger transition to starting
			err = benthosInstance.SetDesiredFSMState(OperationalStateActive)
			Expect(err).NotTo(HaveOccurred())

			// Wait for transition to starting state
			tick, err = waitForStateBenthosInstance(ctx, benthosInstance, tick, OperationalStateStarting, 5)
			Expect(err).NotTo(HaveOccurred(), "Should transition to starting state")

			// Verify port was allocated
			Expect(benthosInstance.config.MetricsPort).To(Equal(allocatedPort),
				"Metrics port should be allocated")
		})

		It("should remove an instance from the manager when it encounters a permanent error in a terminal state", func() {
			// Create a service with a name
			serviceName := "test-permanent-error"

			// Setup the manager and service
			manager, mockService := createMockBenthosManager("test-manager")
			ctx := context.Background()
			tick := uint64(0)

			// Create config
			benthosConfig := createBenthosConfig(serviceName, OperationalStateActive)
			fullConfig := config.FullConfig{
				Benthos: []config.BenthosConfig{benthosConfig},
			}

			// First, transition to a stable state (Idle) using the helper function
			tick, err := transitionToIdle(ctx, manager, mockService, serviceName, fullConfig, tick)
			Expect(err).NotTo(HaveOccurred(), "Failed to transition to idle state")

			// Get the instance after it's in a stable state
			instance, exists := manager.GetInstance(serviceName)
			Expect(exists).To(BeTrue())

			// Put the instance in a stopped state
			benthosInstance, ok := instance.(*BenthosInstance)
			Expect(ok).To(BeTrue())

			// Update desired state to stopped FIRST
			benthosInstance.baseFSMInstance.SetDesiredFSMState(OperationalStateStopped)

			// Also update the config to match the stopped state
			fullConfig.Benthos[0].FSMInstanceConfig.DesiredFSMState = OperationalStateStopped

			// Update the instance state to stopped
			benthosInstance.baseFSMInstance.SetCurrentFSMState(OperationalStateStopped)

			// Update mock service to indicate the service is stopped
			setupServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
				IsS6Running: false,
				S6FSMState:  s6fsm.OperationalStateStopped,
				IsS6Stopped: true,
			})

			// Run multiple reconciliations with larger tick increments to stabilize the state
			// and bypass rate limiting in BaseFSMManager
			for i := 0; i < 3; i++ {
				err, _ = manager.Reconcile(ctx, fullConfig, tick)
				Expect(err).NotTo(HaveOccurred())
				tick += 20 // Large increment to bypass rate limiting
			}

			// Ensure the state is still stopped
			Expect(benthosInstance.GetCurrentFSMState()).To(Equal(OperationalStateStopped))
			Expect(benthosInstance.GetDesiredFSMState()).To(Equal(OperationalStateStopped))

			// NOW create a permanent error and set it on the instance
			permanentError := fmt.Errorf("%s: test permanent error", backoff.PermanentFailureError)
			benthosInstance.baseFSMInstance.SetError(permanentError, tick)

			// Run the manager reconciliation - it should detect and remove the instance
			// Use an even larger tick increment to ensure no rate limiting interferes
			err, reconciled := manager.Reconcile(ctx, fullConfig, tick+50)
			Expect(err).To(BeNil())
			Expect(reconciled).To(BeTrue())

			// Verify the instance was removed from the manager
			_, exists = manager.GetInstance(serviceName)
			Expect(exists).To(BeFalse())
		})
	})

	Context("Multiple Services", func() {
		It("should manage multiple services independently", func() {
			service1Name := "test-service-1"
			service2Name := "test-service-2"

			// Create configs for both services
			config1 := createBenthosConfig(service1Name, OperationalStateActive)
			config2 := createBenthosConfig(service2Name, OperationalStateActive)
			fullConfig := config.FullConfig{
				Benthos: []config.BenthosConfig{config1, config2},
			}

			// Set up a mock port manager
			mockPortManager := portmanager.NewMockPortManager()
			manager.WithPortManager(mockPortManager)

			// Add both services to the manager with ACTIVE state
			setupServiceInManager(manager, mockService, service1Name, OperationalStateActive)
			setupServiceInManager(manager, mockService, service2Name, OperationalStateActive)

			// Allocate ports for both services
			port1, err := mockPortManager.AllocatePort(service1Name)
			Expect(err).NotTo(HaveOccurred())
			port2, err := mockPortManager.AllocatePort(service2Name)
			Expect(err).NotTo(HaveOccurred())

			// Set the ports in the instance configs
			instance1, exists := manager.GetInstance(service1Name)
			Expect(exists).To(BeTrue())
			benthosInstance1, ok := instance1.(*BenthosInstance)
			Expect(ok).To(BeTrue())
			benthosInstance1.config.MetricsPort = port1

			instance2, exists := manager.GetInstance(service2Name)
			Expect(exists).To(BeTrue())
			benthosInstance2, ok := instance2.(*BenthosInstance)
			Expect(ok).To(BeTrue())
			benthosInstance2.config.MetricsPort = port2

			// Mark services as existing in mock service
			mockService.ExistingServices[service1Name] = true
			mockService.ExistingServices[service2Name] = true

			// Phase 2: Initial State Progression
			// Set service1 for fast success
			setupServiceState(mockService, service1Name, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				S6FSMState:             s6fsm.OperationalStateRunning,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   true,
				IsRunningWithoutErrors: true,
			})

			// Set service2 for slow start
			setupServiceState(mockService, service2Name, benthossvc.ServiceStateFlags{
				IsS6Running:            false,
				S6FSMState:             s6fsm.OperationalStateStopped,
				IsConfigLoaded:         false,
				IsHealthchecksPassed:   false,
				IsRunningWithoutErrors: false,
				HasProcessingActivity:  false,
				IsDegraded:             false,
				IsS6Stopped:            true,
			})

			// Initial reconcile to start state progression
			err, _ = manager.Reconcile(ctx, fullConfig, tick)
			Expect(err).NotTo(HaveOccurred())
			tick++

			// Wait for service1 to reach idle state (it's already set up for success)
			maxAttempts := 20
			var service1State, service2State string
			for i := 0; i < maxAttempts; i++ {
				err, _ = manager.Reconcile(ctx, fullConfig, tick)
				Expect(err).NotTo(HaveOccurred())
				tick++

				service1State = benthosInstance1.GetCurrentFSMState()
				service2State = benthosInstance2.GetCurrentFSMState()

				GinkgoWriter.Printf("\n=== Service1 Progress Attempt %d/%d ===\n", i+1, maxAttempts)
				GinkgoWriter.Printf("Service1 state: %s\n", service1State)
				GinkgoWriter.Printf("Service2 state: %s\n", service2State)

				// Continue reconciling until service1 reaches idle
				if service1State == OperationalStateIdle {
					break
				}
			}

			// Verify service1 reached idle state
			Expect(service1State).To(Equal(OperationalStateIdle),
				"Service1 should reach idle state")

			// Now wait for service2 to reach starting state
			for i := 0; i < maxAttempts; i++ {
				err, _ = manager.Reconcile(ctx, fullConfig, tick)
				Expect(err).NotTo(HaveOccurred())
				tick++

				service1State = benthosInstance1.GetCurrentFSMState()
				service2State = benthosInstance2.GetCurrentFSMState()

				GinkgoWriter.Printf("\n=== Service2 Initial Progress Attempt %d/%d ===\n", i+1, maxAttempts)
				GinkgoWriter.Printf("Service1 state: %s\n", service1State)
				GinkgoWriter.Printf("Service2 state: %s\n", service2State)

				// Continue reconciling until service2 reaches at least starting
				if service2State == OperationalStateStarting {
					break
				}
			}

			// Verify service2 is in starting state
			Expect(service2State).To(Equal(OperationalStateStarting),
				"Service2 should be in starting state")

			// Phase 4: Allow service2 to progress
			setupServiceState(mockService, service2Name, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				S6FSMState:             s6fsm.OperationalStateRunning,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   true,
				IsRunningWithoutErrors: true,
			})

			// Wait for service2 to reach idle state
			for i := 0; i < maxAttempts; i++ {
				err, _ = manager.Reconcile(ctx, fullConfig, tick)
				Expect(err).NotTo(HaveOccurred())
				tick++

				service1State = benthosInstance1.GetCurrentFSMState()
				service2State = benthosInstance2.GetCurrentFSMState()

				GinkgoWriter.Printf("\n=== Service2 Progress Attempt %d/%d ===\n", i+1, maxAttempts)
				GinkgoWriter.Printf("Service1 state: %s\n", service1State)
				GinkgoWriter.Printf("Service2 state: %s\n", service2State)

				if service2State == OperationalStateIdle {
					break
				}
			}

			// Verify both services are in idle state
			Expect(service1State).To(Equal(OperationalStateIdle),
				"Service1 should maintain idle state")
			Expect(service2State).To(Equal(OperationalStateIdle),
				"Service2 should reach idle state")

			// Phase 5: Verify independent state changes
			// Degrade service1 while keeping service2 healthy
			setupServiceState(mockService, service1Name, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				S6FSMState:             s6fsm.OperationalStateRunning,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   false, // This will cause degradation
				IsRunningWithoutErrors: false,
				HasProcessingActivity:  false,
			})

			// Keep service2 healthy
			setupServiceState(mockService, service2Name, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				S6FSMState:             s6fsm.OperationalStateRunning,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   true,
				IsRunningWithoutErrors: true,
				HasProcessingActivity:  false,
			})

			// Wait for service1 to degrade while service2 remains healthy
			for i := 0; i < maxAttempts; i++ {
				err, _ = manager.Reconcile(ctx, fullConfig, tick)
				Expect(err).NotTo(HaveOccurred())
				tick++

				service1State = benthosInstance1.GetCurrentFSMState()
				service2State = benthosInstance2.GetCurrentFSMState()

				GinkgoWriter.Printf("\n=== Final Verification Attempt %d/%d ===\n", i+1, maxAttempts)
				GinkgoWriter.Printf("Service1 state: %s\n", service1State)
				GinkgoWriter.Printf("Service2 state: %s\n", service2State)

				if service1State == OperationalStateDegraded {
					break
				}
			}

			// Final state verification
			Expect(service1State).To(Equal(OperationalStateDegraded),
				"Service1 should be degraded")
			Expect(service2State).To(Equal(OperationalStateIdle),
				"Service2 should remain healthy")
		})
	})

	Context("Port Management", func() {
		It("should allocate ports before base reconciliation", func() {
			serviceName := "test-service"

			// Create config for the service
			benthosConfig := createBenthosConfig(serviceName, OperationalStateActive)
			fullConfig := config.FullConfig{
				Benthos: []config.BenthosConfig{benthosConfig},
			}

			// Set up a mock port manager
			mockPortManager := portmanager.NewMockPortManager()
			manager.WithPortManager(mockPortManager)

			// Verify that PreReconcile is called before base reconciliation
			err, reconciled := manager.Reconcile(ctx, fullConfig, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())

			// Verify that PreReconcile was called
			Expect(mockPortManager.PreReconcileCalled).To(BeTrue())

			// Verify that the service has a port allocated
			instance, exists := manager.GetInstance(serviceName)
			Expect(exists).To(BeTrue())
			benthosInstance, ok := instance.(*BenthosInstance)
			Expect(ok).To(BeTrue())
			Expect(benthosInstance.config.MetricsPort).To(BeNumerically(">", 0))
		})

		It("should handle port allocation failures gracefully", func() {
			serviceName := "test-service"

			// Create config for the service
			benthosConfig := createBenthosConfig(serviceName, OperationalStateActive)
			fullConfig := config.FullConfig{
				Benthos: []config.BenthosConfig{benthosConfig},
			}

			// Set up a mock port manager with error
			mockPortManager := portmanager.NewMockPortManager()
			mockPortManager.PreReconcileError = fmt.Errorf("test error")
			manager.WithPortManager(mockPortManager)

			// Verify that reconciliation fails when port allocation fails
			err, reconciled := manager.Reconcile(ctx, fullConfig, tick)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("port pre-allocation failed"))
			Expect(reconciled).To(BeFalse())
		})

		It("should call post-reconciliation after base reconciliation", func() {
			serviceName := "test-service"

			// Create config for the service
			benthosConfig := createBenthosConfig(serviceName, OperationalStateActive)
			fullConfig := config.FullConfig{
				Benthos: []config.BenthosConfig{benthosConfig},
			}

			// Set up a mock port manager
			mockPortManager := portmanager.NewMockPortManager()
			manager.WithPortManager(mockPortManager)

			// Verify that PostReconcile is called after base reconciliation
			err, reconciled := manager.Reconcile(ctx, fullConfig, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())

			// Verify that PostReconcile was called
			Expect(mockPortManager.PostReconcileCalled).To(BeTrue())
		})
	})
})
