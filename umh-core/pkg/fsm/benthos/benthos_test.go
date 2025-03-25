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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/portmanager"
	benthossvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
	s6svc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

// TODO: needs to be refactored based on the test approach in umh-core/test/fsm/s6/manager_test.go
// and then also moved in to a shared package

// createMockBenthosInstance creates a BenthosInstance with a mock service for testing
func createMockBenthosInstance(name string, mockService benthossvc.IBenthosService) *BenthosInstance {
	cfg := config.BenthosConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name: name,
		},
		BenthosServiceConfig: config.BenthosServiceConfig{},
	}

	instance := NewBenthosInstance(constants.S6BaseDir, cfg)
	instance.service = mockService
	return instance
}

// Helper function to get an instance in Idle state
func getInstanceInIdleState(testID string, mockService *benthossvc.MockBenthosService, instance *BenthosInstance, ctx context.Context, tick uint64) (uint64, error) {
	// 1. Lifecycle creation phase
	// Initial state: to_be_created
	if err, _ := instance.Reconcile(ctx, tick); err != nil {
		return tick, err
	}
	tick++

	// Mock service creation success
	mockService.ServiceStates[testID] = &benthossvc.ServiceInfo{
		S6FSMState: s6fsm.OperationalStateStopped,
		S6ObservedState: s6fsm.S6ObservedState{
			ServiceInfo: s6svc.ServiceInfo{
				Status: s6svc.ServiceDown,
				Uptime: 5,
			},
		},
		BenthosStatus: benthossvc.BenthosStatus{
			HealthCheck: benthossvc.HealthCheck{
				IsLive:  false,
				IsReady: false,
			},
		},
	}

	// Setup port manager and allocate port
	mockPortManager := portmanager.NewMockPortManager()
	allocatedPort, err := mockPortManager.AllocatePort(testID)
	Expect(err).NotTo(HaveOccurred())
	instance.config.MetricsPort = allocatedPort

	// Make sure service is in existing services to avoid "service not found" error
	mockService.ExistingServices[testID] = true

	// Second reconcile: created -> stopped
	if err, _ := instance.Reconcile(ctx, tick); err != nil {
		return tick, err
	}
	tick++

	// 2. Activate the instance
	if err := instance.SetDesiredFSMState(OperationalStateActive); err != nil {
		return tick, err
	}

	// 3. Starting phase
	// First operational reconcile: starting
	if err, _ := instance.Reconcile(ctx, tick); err != nil {
		return tick, err
	}
	tick++

	// 4. ConfigLoading phase
	mockService.SetServiceState(testID, benthossvc.ServiceStateFlags{
		IsS6Running:          true,
		S6FSMState:           s6fsm.OperationalStateRunning,
		IsConfigLoaded:       true,
		IsHealthchecksPassed: true,
	})
	mockService.GetConfigResult = config.BenthosServiceConfig{}

	// Add this line to simulate config validation
	mockService.ServiceStates[testID].S6ObservedState.ServiceInfo.Uptime = 5

	// Update health check status
	mockService.ServiceStates[testID].BenthosStatus.HealthCheck = benthossvc.HealthCheck{
		IsLive:  true,
		IsReady: true,
	}

	if err, _ := instance.Reconcile(ctx, tick); err != nil {
		return tick, err
	}
	tick++

	// 5. WaitingForHealthchecks phase
	mockService.SetServiceState(testID, benthossvc.ServiceStateFlags{
		IsS6Running:          true,
		S6FSMState:           s6fsm.OperationalStateRunning,
		IsConfigLoaded:       true,
		IsHealthchecksPassed: true,
	})
	mockService.ServiceStates[testID].BenthosStatus.HealthCheck = benthossvc.HealthCheck{
		IsLive:  true,
		IsReady: true,
	}

	if err, _ := instance.Reconcile(ctx, tick); err != nil {
		return tick, err
	}
	tick++

	// 6. WaitingForServiceToRemainRunning phase
	mockService.SetServiceState(testID, benthossvc.ServiceStateFlags{
		IsS6Running:            true,
		S6FSMState:             s6fsm.OperationalStateRunning,
		IsConfigLoaded:         true,
		IsHealthchecksPassed:   true,
		IsRunningWithoutErrors: true,
	})
	mockService.ServiceStates[testID].BenthosStatus.HealthCheck = benthossvc.HealthCheck{
		IsLive:  true,
		IsReady: true,
	}

	// Add these lines to simulate stability checks
	mockService.ServiceStates[testID].S6ObservedState.ServiceInfo.Uptime = 15
	mockService.ServiceStates[testID].BenthosStatus.MetricsState = &benthossvc.BenthosMetricsState{
		IsActive: false,
	}

	if err, _ := instance.Reconcile(ctx, tick); err != nil {
		return tick, err
	}
	tick++

	// 7. Final transition to Idle
	// Let's simulate 3 successful reconciliations (to test the stability check)
	for i := 0; i < 3; i++ {
		if err, _ := instance.Reconcile(ctx, tick); err != nil {
			return tick, err
		}
		tick++
	}

	return tick, nil
}

// Helper function to wait for a specific state
func waitForStateBenthosInstance(ctx context.Context, instance *BenthosInstance, tick uint64, targetState string, maxAttempts int) (uint64, error) {
	for i := 0; i < maxAttempts; i++ {
		err, _ := instance.Reconcile(ctx, tick)
		if err != nil {
			return tick, err
		}
		tick++
		if instance.GetCurrentFSMState() == targetState {
			return tick, nil
		}
	}
	return tick, fmt.Errorf("failed to reach state %s after %d attempts", targetState, maxAttempts)
}

var _ = Describe("Benthos FSM", func() {
	var (
		mockService *benthossvc.MockBenthosService
		instance    *BenthosInstance
		testID      string
		ctx         context.Context
		tick        uint64
	)

	BeforeEach(func() {
		testID = "test-benthos"
		mockService = benthossvc.NewMockBenthosService()
		instance = createMockBenthosInstance(testID, mockService)
		ctx = context.Background()
		tick = 0
	})

	Context("Basic State Transitions", func() {
		It("should transition from Stopped to Starting when activated", func() {
			// First go through the lifecycle states
			// Initial state should be to_be_created
			Expect(instance.GetCurrentFSMState()).To(Equal(internalfsm.LifecycleStateToBeCreated))

			// Reconcile should trigger create event
			err, _ := instance.Reconcile(ctx, tick)
			tick++
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetCurrentFSMState()).To(Equal(internalfsm.LifecycleStateCreating))
			Expect(mockService.AddBenthosToS6ManagerCalled).To(BeTrue())

			// Mock service creation success
			mockService.ServiceStates[testID] = &benthossvc.ServiceInfo{
				S6FSMState: s6fsm.OperationalStateStopped,
				S6ObservedState: s6fsm.S6ObservedState{
					ServiceInfo: s6svc.ServiceInfo{
						Status: s6svc.ServiceDown,
						Uptime: 5,
					},
				},
				BenthosStatus: benthossvc.BenthosStatus{
					HealthCheck: benthossvc.HealthCheck{
						IsLive:  false,
						IsReady: false,
					},
				},
			}

			// Setup port manager and allocate port
			mockPortManager := portmanager.NewMockPortManager()
			allocatedPort, err := mockPortManager.AllocatePort(testID)
			Expect(err).NotTo(HaveOccurred())
			instance.config.MetricsPort = allocatedPort

			// Make sure service is in existing services to avoid "service not found" error
			mockService.ExistingServices[testID] = true

			// Another reconcile should complete the creation
			err, _ = instance.Reconcile(ctx, tick)
			tick++
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateStopped))

			// Now test the operational state transition
			// Set desired state to active
			Expect(instance.SetDesiredFSMState(OperationalStateActive)).To(Succeed())
			Expect(instance.GetDesiredFSMState()).To(Equal(OperationalStateActive))

			// Setup mock service state for initial reconciliation
			mockService.SetServiceState(testID, benthossvc.ServiceStateFlags{
				IsS6Running: false,
			})

			// First reconciliation should trigger transition to Starting
			err, reconciled := instance.Reconcile(ctx, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())
			Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateStarting))

			// Verify service interactions
			Expect(mockService.StartBenthosCalled).To(BeTrue())
		})

		It("should transition from Starting to ConfigLoading when S6 is running", func() {
			// First complete the creation process
			// Initial state should be to_be_created
			Expect(instance.GetCurrentFSMState()).To(Equal(internalfsm.LifecycleStateToBeCreated))

			// Setup port manager and allocate port
			mockPortManager := portmanager.NewMockPortManager()
			allocatedPort, err := mockPortManager.AllocatePort(testID)
			Expect(err).NotTo(HaveOccurred())
			instance.config.MetricsPort = allocatedPort

			// First reconcile moves to creating
			err, _ = instance.Reconcile(ctx, tick)
			tick++
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetCurrentFSMState()).To(Equal(internalfsm.LifecycleStateCreating))
			Expect(mockService.AddBenthosToS6ManagerCalled).To(BeTrue())

			// Mock service creation success
			mockService.ServiceStates[testID] = &benthossvc.ServiceInfo{
				S6FSMState: s6fsm.OperationalStateStopped,
				S6ObservedState: s6fsm.S6ObservedState{
					ServiceInfo: s6svc.ServiceInfo{
						Status: s6svc.ServiceDown,
						Uptime: 5,
					},
				},
				BenthosStatus: benthossvc.BenthosStatus{
					HealthCheck: benthossvc.HealthCheck{
						IsLive:  false,
						IsReady: false,
					},
				},
			}

			// Make sure service is in existing services to avoid "service not found" error
			mockService.ExistingServices[testID] = true

			// Another reconcile should complete the creation
			err, _ = instance.Reconcile(ctx, tick)
			tick++
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateStopped))

			// Activate the instance
			Expect(instance.SetDesiredFSMState(OperationalStateActive)).To(Succeed())

			// First operational reconcile moves to starting
			err, _ = instance.Reconcile(ctx, tick)
			tick++
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateStarting))

			// Now setup S6 running state and mock config response
			mockService.SetServiceState(testID, benthossvc.ServiceStateFlags{
				IsS6Running:          true,
				S6FSMState:           s6fsm.OperationalStateRunning,
				IsConfigLoaded:       true,
				IsHealthchecksPassed: true,
			})

			// Update health check status
			mockService.ServiceStates[testID].BenthosStatus.HealthCheck = benthossvc.HealthCheck{
				IsLive:  true,
				IsReady: true,
			}

			// Reconcile again - should transition to ConfigLoading
			err, reconciled := instance.Reconcile(ctx, tick)
			tick++
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())
			Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateStartingConfigLoading))
		})

		It("should transition to Idle when healthchecks pass", func() {
			// Set up mock port manager and allocate port
			mockPortManager := portmanager.NewMockPortManager()
			allocatedPort, err := mockPortManager.AllocatePort(testID)
			Expect(err).NotTo(HaveOccurred())

			// Set up instance with allocated port
			instance.config.MetricsPort = allocatedPort

			// First get to ConfigLoading state
			// Initial creation flow
			Expect(instance.GetCurrentFSMState()).To(Equal(internalfsm.LifecycleStateToBeCreated))

			// 1. Create the service
			err, _ = instance.Reconcile(ctx, tick)
			tick++
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetCurrentFSMState()).To(Equal(internalfsm.LifecycleStateCreating))

			// Mock service creation success
			mockService.ServiceStates[testID] = &benthossvc.ServiceInfo{
				S6FSMState: s6fsm.OperationalStateStopped,
				S6ObservedState: s6fsm.S6ObservedState{
					ServiceInfo: s6svc.ServiceInfo{
						Status: s6svc.ServiceDown,
						Uptime: 5,
					},
				},
				BenthosStatus: benthossvc.BenthosStatus{
					HealthCheck: benthossvc.HealthCheck{
						IsLive:  false,
						IsReady: false,
					},
				},
			}
			mockService.ExistingServices[testID] = true

			// 2. Complete creation
			err, _ = instance.Reconcile(ctx, tick)
			tick++
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateStopped))

			// 3. Activate the instance
			Expect(instance.SetDesiredFSMState(OperationalStateActive)).To(Succeed())

			// Set up service state for transition to idle
			mockService.SetServiceState(testID, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				S6FSMState:             s6fsm.OperationalStateRunning,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   true,
				IsRunningWithoutErrors: true,
			})

			// Add required S6 observed state for config loaded check
			mockService.ServiceStates[testID].S6ObservedState.ServiceInfo.Uptime = 15

			// Update health check status to ready
			mockService.ServiceStates[testID].BenthosStatus.HealthCheck = benthossvc.HealthCheck{
				IsLive:  true,
				IsReady: true,
			}

			// Add metrics state to indicate activity
			mockService.ServiceStates[testID].BenthosStatus.MetricsState = &benthossvc.BenthosMetricsState{
				IsActive: true,
			}

			// First reconcile attempt
			err, reconciled := instance.Reconcile(ctx, tick)
			tick++

			// Verify error handling
			Expect(err).NotTo(HaveOccurred(), "Reconcile should handle service not found gracefully")
			Expect(reconciled).To(BeTrue(), "Reconcile should indicate changes were made")

			// Wait for transition to Idle state
			tick, err = waitForStateBenthosInstance(ctx, instance, tick, OperationalStateIdle, 20)
			Expect(err).NotTo(HaveOccurred(), "Should transition to Idle state")
			Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateIdle))
		})
	})

	Context("Running State Transitions", func() {
		It("should transition from Idle to Active when processing data", func() {
			// First reach Idle state through normal startup process
			tick, err := getInstanceInIdleState(testID, mockService, instance, ctx, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateIdle))

			// Maintain healthy state flags
			mockService.SetServiceState(testID, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   true,
				IsRunningWithoutErrors: true,
				HasProcessingActivity:  true,
			})

			// Add this to initialize the MetricsState
			mockService.ServiceStates[testID].BenthosStatus.MetricsState = &benthossvc.BenthosMetricsState{
				IsActive: true, // This is what HasProcessingActivity checks
			}

			err, reconciled := instance.Reconcile(ctx, tick)
			tick++
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())
			Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateActive))
		})

		It("should transition to Degraded when issues occur", func() {
			// First reach Active state through normal startup and processing activity
			tick, err := getInstanceInIdleState(testID, mockService, instance, ctx, tick)
			Expect(err).NotTo(HaveOccurred())

			// Transition to Active state first
			mockService.SetServiceState(testID, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   true,
				IsRunningWithoutErrors: true,
				HasProcessingActivity:  true,
			})
			mockService.ServiceStates[testID].BenthosStatus.MetricsState = &benthossvc.BenthosMetricsState{
				IsActive: true,
			}
			err, _ = instance.Reconcile(ctx, tick)
			tick++
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateActive))

			// Simulate healthcheck failure
			mockService.SetServiceState(testID, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   false, // Healthcheck failure
				IsRunningWithoutErrors: true,
				HasProcessingActivity:  true,
			})
			mockService.ServiceStates[testID].BenthosStatus.HealthCheck = benthossvc.HealthCheck{
				IsLive:  false,
				IsReady: false,
			}

			// Trigger reconcile to detect degradation
			err, reconciled := instance.Reconcile(ctx, tick)
			tick++
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())
			Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateDegraded))

			// Verify metrics state is still present
			Expect(mockService.ServiceStates[testID].BenthosStatus.MetricsState).ToNot(BeNil())
			Expect(mockService.ServiceStates[testID].BenthosStatus.MetricsState.IsActive).To(BeTrue())
		})

		It("should recover from Degraded state when issues resolve", func() {
			// First reach Degraded state
			tick, err := getInstanceInIdleState(testID, mockService, instance, ctx, tick)
			Expect(err).NotTo(HaveOccurred())

			// Transition to Active then Degraded
			mockService.SetServiceState(testID, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   false,
				IsRunningWithoutErrors: true,
				HasProcessingActivity:  true,
			})
			mockService.ServiceStates[testID].BenthosStatus.HealthCheck = benthossvc.HealthCheck{
				IsLive:  false,
				IsReady: false,
			}
			err, _ = instance.Reconcile(ctx, tick)
			tick++
			Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateDegraded))

			// Fix the healthcheck issues
			mockService.SetServiceState(testID, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   true,
				IsRunningWithoutErrors: true,
				HasProcessingActivity:  true,
			})
			mockService.ServiceStates[testID].BenthosStatus.HealthCheck = benthossvc.HealthCheck{
				IsLive:  true,
				IsReady: true,
			}

			// Add stability checks (uptime > 10s)
			mockService.ServiceStates[testID].S6ObservedState.ServiceInfo.Uptime = 15

			// Trigger recovery
			err, reconciled := instance.Reconcile(ctx, tick)
			tick++
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())
			Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateIdle))

			// Verify stable state after recovery
			// To bring this to active, we would need to also have active metrics
			for i := 0; i < 3; i++ {
				err, reconciled = instance.Reconcile(ctx, tick)
				tick++
				Expect(err).NotTo(HaveOccurred())
				Expect(reconciled).To(BeFalse())
				Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateIdle))
			}

			// Now set the metrics to active
			mockService.SetServiceState(testID, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   true,
				IsRunningWithoutErrors: true,
				HasProcessingActivity:  true,
			})
			mockService.ServiceStates[testID].BenthosStatus.MetricsState = &benthossvc.BenthosMetricsState{
				IsActive: true,
			}

			err, reconciled = instance.Reconcile(ctx, tick)
			tick++
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())
			Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateActive))
		})
	})

	Context("Stopping Flow", func() {
		It("should stop gracefully from Active state", func() {
			// First reach Active state
			tick, err := getInstanceInIdleState(testID, mockService, instance, ctx, tick)
			Expect(err).NotTo(HaveOccurred())

			// Transition to Active
			mockService.SetServiceState(testID, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   true,
				IsRunningWithoutErrors: true,
				HasProcessingActivity:  true,
			})
			mockService.ServiceStates[testID].BenthosStatus.MetricsState = &benthossvc.BenthosMetricsState{
				IsActive: true,
			}
			err, _ = instance.Reconcile(ctx, tick)
			tick++
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateActive))

			// Trigger stop
			Expect(instance.SetDesiredFSMState(OperationalStateStopped)).To(Succeed())

			// First reconcile should transition to Stopping
			err, reconciled := instance.Reconcile(ctx, tick)
			tick++
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())
			Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateStopping))
			Expect(mockService.StopBenthosCalled).To(BeTrue())

			// Simulate S6 stopping
			mockService.SetServiceState(testID, benthossvc.ServiceStateFlags{
				IsS6Running:            false,
				S6FSMState:             s6fsm.OperationalStateStopped,
				IsConfigLoaded:         false,
				IsHealthchecksPassed:   false,
				IsRunningWithoutErrors: false,
			})

			// Final reconcile should transition to Stopped
			err, reconciled = instance.Reconcile(ctx, tick)
			tick++
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())
			Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateStopped))
		})

		It("should stop gracefully from Degraded state", func() {
			// First reach Degraded state
			tick, err := getInstanceInIdleState(testID, mockService, instance, ctx, tick)
			Expect(err).NotTo(HaveOccurred())

			// Transition to Degraded
			mockService.SetServiceState(testID, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   false,
				IsRunningWithoutErrors: false,
				HasProcessingActivity:  true,
			})
			mockService.ServiceStates[testID].BenthosStatus.HealthCheck = benthossvc.HealthCheck{
				IsLive:  false,
				IsReady: false,
			}
			err, _ = instance.Reconcile(ctx, tick)
			tick++
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateDegraded))

			// Trigger stop
			Expect(instance.SetDesiredFSMState(OperationalStateStopped)).To(Succeed())

			// First reconcile should transition to Stopping
			err, reconciled := instance.Reconcile(ctx, tick)
			tick++
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())
			Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateStopping))
			Expect(mockService.StopBenthosCalled).To(BeTrue())

			// Simulate S6 stopping
			mockService.SetServiceState(testID, benthossvc.ServiceStateFlags{
				IsS6Running:            false,
				S6FSMState:             s6fsm.OperationalStateStopped,
				IsConfigLoaded:         false,
				IsHealthchecksPassed:   false,
				IsRunningWithoutErrors: false,
			})

			// Final reconcile should transition to Stopped
			err, reconciled = instance.Reconcile(ctx, tick)
			tick++
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())
			Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateStopped))
		})
	})

	Context("complex state transitions", func() {

		It("should restart from Starting when config loading fails due to S6 instability", func() {
			instance.config.MetricsPort = 100

			// 1. Create the service
			err, _ := instance.Reconcile(ctx, tick)
			tick++
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetCurrentFSMState()).To(Equal(internalfsm.LifecycleStateCreating))

			// Mock service creation success
			mockService.ServiceStates[testID] = &benthossvc.ServiceInfo{
				S6FSMState: s6fsm.OperationalStateStopped,
				S6ObservedState: s6fsm.S6ObservedState{
					ServiceInfo: s6svc.ServiceInfo{
						Status: s6svc.ServiceDown,
						Uptime: 5,
					},
				},
				BenthosStatus: benthossvc.BenthosStatus{
					HealthCheck: benthossvc.HealthCheck{
						IsLive:  false,
						IsReady: false,
					},
				},
			}
			mockService.ExistingServices[testID] = true

			// 2. Wait for transition to Stopped
			tick, err = waitForStateBenthosInstance(ctx, instance, tick, OperationalStateStopped, 20)
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateStopped))

			// 3. Activate the instance
			Expect(instance.SetDesiredFSMState(OperationalStateActive)).To(Succeed())

			// 4. Wait for transition to Starting
			tick, err = waitForStateBenthosInstance(ctx, instance, tick, OperationalStateStarting, 20)
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateStarting))

			// 5. Set up for transition to ConfigLoading
			mockService.SetServiceState(testID, benthossvc.ServiceStateFlags{
				IsS6Running: true,
				S6FSMState:  s6fsm.OperationalStateRunning,
			})

			// Wait for transition to ConfigLoading
			tick, err = waitForStateBenthosInstance(ctx, instance, tick, OperationalStateStartingConfigLoading, 20)
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateStartingConfigLoading))

			// Simulate S6 crash during config loading
			mockService.SetServiceState(testID, benthossvc.ServiceStateFlags{
				IsS6Running:          false,
				S6FSMState:           s6fsm.OperationalStateStopped,
				IsConfigLoaded:       false,
				IsHealthchecksPassed: false,
			})

			// Wait for transition back to Starting
			tick, err = waitForStateBenthosInstance(ctx, instance, tick, OperationalStateStarting, 20)
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateStarting))

			// Simulate S6 comes back up but fails again before config loads
			mockService.SetServiceState(testID, benthossvc.ServiceStateFlags{
				IsS6Running:          true,
				S6FSMState:           s6fsm.OperationalStateRunning,
				IsConfigLoaded:       false,
				IsHealthchecksPassed: false,
			})

			// Wait for transition to ConfigLoading
			tick, err = waitForStateBenthosInstance(ctx, instance, tick, OperationalStateStartingConfigLoading, 20)
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateStartingConfigLoading))

			// Simulate another S6 crash
			mockService.SetServiceState(testID, benthossvc.ServiceStateFlags{
				IsS6Running:          false,
				S6FSMState:           s6fsm.OperationalStateStopped,
				IsConfigLoaded:       false,
				IsHealthchecksPassed: false,
			})

			// Wait for final transition back to Starting
			tick, err = waitForStateBenthosInstance(ctx, instance, tick, OperationalStateStarting, 20)
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateStarting))
		})
	})

	Context("Error Handling", func() {
		It("should handle errors during startup", func() {
			// Set up a mock port manager and allocate port
			mockPortManager := portmanager.NewMockPortManager()
			allocatedPort, err := mockPortManager.AllocatePort(testID)
			Expect(err).NotTo(HaveOccurred())
			instance.config.MetricsPort = allocatedPort

			// Simulate service creation failure
			mockService.AddBenthosToS6ManagerError = fmt.Errorf("simulated creation error")

			// Initial state: to_be_created
			Expect(instance.GetCurrentFSMState()).To(Equal(internalfsm.LifecycleStateToBeCreated))

			// First reconcile attempt - should trigger error and start backoff
			// Backoff will be 1 tick, so it triggers immediately
			err, reconciled := instance.Reconcile(ctx, tick)
			tick++
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeFalse())
			Expect(instance.GetCurrentFSMState()).To(Equal(internalfsm.LifecycleStateToBeCreated))

			// Verify service creation was attempted
			Expect(mockService.AddBenthosToS6ManagerCalled).To(BeTrue())

			// Clear the error immediately and verify recovery
			mockService.AddBenthosToS6ManagerError = nil
			mockService.ServiceStates[testID] = &benthossvc.ServiceInfo{
				S6FSMState: s6fsm.OperationalStateStopped,
			}

			// Next reconcile should succeed despite previous error
			err, reconciled = instance.Reconcile(ctx, tick)
			tick++
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())
			Expect(instance.GetCurrentFSMState()).To(Equal(internalfsm.LifecycleStateCreating))

			// Complete the creation process
			err, reconciled = instance.Reconcile(ctx, tick)
			tick++
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())
			Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateStopped))
		})

		It("should handle errors during runtime", func() {
			// Reach Active state first
			tick, err := getInstanceInIdleState(testID, mockService, instance, ctx, tick)
			Expect(err).NotTo(HaveOccurred())

			// Transition to Active
			mockService.SetServiceState(testID, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   true,
				IsRunningWithoutErrors: true,
				HasProcessingActivity:  true,
			})
			mockService.ServiceStates[testID].BenthosStatus.MetricsState = &benthossvc.BenthosMetricsState{
				IsActive: true,
			}
			err, _ = instance.Reconcile(ctx, tick)
			tick++
			Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateActive))

			// Simulate runtime failure (service crash)
			mockService.SetServiceState(testID, benthossvc.ServiceStateFlags{
				IsS6Running:            false,
				S6FSMState:             s6fsm.OperationalStateStopped,
				IsConfigLoaded:         false,
				IsHealthchecksPassed:   false,
				IsRunningWithoutErrors: false,
			})

			// Detect degradation
			err, reconciled := instance.Reconcile(ctx, tick)
			tick++
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())
			Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateDegraded))

			// Verify recovery attempt
			mockService.SetServiceState(testID, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				S6FSMState:             s6fsm.OperationalStateRunning,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   true,
				IsRunningWithoutErrors: true,
			})
			mockService.ServiceStates[testID].BenthosStatus.HealthCheck = benthossvc.HealthCheck{
				IsLive:  true,
				IsReady: true,
			}

			// Add stability checks
			mockService.ServiceStates[testID].S6ObservedState.ServiceInfo.Uptime = 15

			// Reconcile to recover
			err, reconciled = instance.Reconcile(ctx, tick)
			tick++
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())
			Expect(instance.GetCurrentFSMState()).To(Equal(OperationalStateIdle))
		})

		It("should attempt self-removal when encountering a permanent error", func() {
			// Setup instance
			mockService := benthossvc.NewMockBenthosService()
			instance := createMockBenthosInstance("test-permanent-error", mockService)
			ctx := context.Background()
			tick := uint64(0)

			// Setup the service to exist
			mockService.ExistingServices[instance.baseFSMInstance.GetID()] = true

			// Setup metrics port - necessary to avoid "could not find metrics port" error
			mockPortManager := portmanager.NewMockPortManager()
			allocatedPort, err := mockPortManager.AllocatePort(instance.baseFSMInstance.GetID())
			Expect(err).NotTo(HaveOccurred())
			instance.config.MetricsPort = allocatedPort

			// Set up mock service state
			mockService.ServiceStates[instance.baseFSMInstance.GetID()] = &benthossvc.ServiceInfo{
				S6FSMState: s6fsm.OperationalStateStopped,
			}

			// Now transition to active state to test error handling in a non-terminal state
			instance.baseFSMInstance.SetCurrentFSMState(OperationalStateActive)

			// Set up a permanent error in the instance
			permanentError := fmt.Errorf("%s: test permanent error", backoff.PermanentFailureError)
			instance.baseFSMInstance.SetError(permanentError, tick)

			// Reconcile with the permanent error - use the same tick to ensure ShouldSkipReconcileBecauseOfError triggers
			err, _ = instance.Reconcile(ctx, tick)

			// Verify no error is returned to allow removal process
			Expect(err).To(BeNil())

			// Verify the instance initiated the removal process
			Expect(instance.baseFSMInstance.GetDesiredFSMState()).To(Equal(OperationalStateStopped))
			Expect(instance.baseFSMInstance.GetError()).To(BeNil())
		})

		It("should attempt forced removal when in a terminal state with a permanent error", func() {
			// Setup instance
			mockService := benthossvc.NewMockBenthosService()
			instance := createMockBenthosInstance("test-permanent-error-terminal", mockService)
			ctx := context.Background()
			tick := uint64(0)

			// Setup the service to exist
			mockService.ExistingServices[instance.baseFSMInstance.GetID()] = true

			// Put instance in a terminal state (stopped)
			instance.baseFSMInstance.SetCurrentFSMState(OperationalStateStopped)

			// Set up a permanent error in the instance
			permanentError := fmt.Errorf("%s: test permanent error", backoff.PermanentFailureError)
			instance.baseFSMInstance.SetError(permanentError, tick)

			// Reconcile with the permanent error
			err, _ := instance.Reconcile(ctx, tick)

			// Verify error is returned to be handled by manager
			Expect(err).NotTo(BeNil())
			Expect(err.Error()).To(ContainSubstring(backoff.PermanentFailureError))

			// Verify forced removal was attempted
			Expect(mockService.ForceRemoveBenthosCalled).To(BeTrue())
		})
	})
})
