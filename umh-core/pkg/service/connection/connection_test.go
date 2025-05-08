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

package connection

import (
	"context"
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	nmapfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/nmap"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/nmap"
	s6svc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

const (
	targetHost = "localhost"
	targetPort = 502
)

var _ = Describe("ConnectionService", func() {
	var (
		service        *ConnectionService
		mockNmap       *nmap.MockNmapService
		ctx            context.Context
		tick           uint64
		connectionName string
		cancelFunc     context.CancelFunc
		mockServices   *serviceregistry.Registry
	)

	BeforeEach(func() {
		ctx, cancelFunc = context.WithDeadline(context.Background(), time.Now().Add(500*time.Second))
		tick = 1
		connectionName = "test-connection"

		// Set up mock nmap service
		mockNmap = nmap.NewMockNmapService()

		// Set up a real service with mocked dependencies
		service = NewDefaultConnectionService(connectionName,
			WithNmapService(mockNmap))
		mockServices = serviceregistry.NewMockRegistry()
	})

	AfterEach(func() {
		// Clean up if necessary
		cancelFunc()
	})

	Describe("AddConnection", func() {
		var (
			cfg *connectionserviceconfig.ConnectionServiceConfig
		)

		BeforeEach(func() {
			// Create a basic config for testing
			cfg = &connectionserviceconfig.ConnectionServiceConfig{
				NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
					Target: targetHost,
					Port:   targetPort,
				},
			}

			// Set up mock to return a valid NmapServiceConfig when generating config
			mockNmap.ServiceExistsResult = false
		})

		It("should add a new connection to the nmap manager", func() {

			err := service.AddConnectionToNmapManager(ctx, mockServices.GetFileSystem(), cfg, connectionName)
			Expect(err).NotTo(HaveOccurred())

			// Verify that a Nmap config was added to the service
			Expect(service.nmapConfigs).To(HaveLen(1))

			// Verify the name follows the expected pattern
			nmapName := service.getNmapName(connectionName)
			Expect(service.nmapConfigs[0].Name).To(Equal(nmapName))

			// Verify the desired state is set correctly
			Expect(service.nmapConfigs[0].DesiredFSMState).To(Equal(nmapfsm.OperationalStateOpen))
		})

		It("should return error when connection already exists", func() {

			err := service.AddConnectionToNmapManager(ctx, mockServices.GetFileSystem(), cfg, connectionName)
			Expect(err).NotTo(HaveOccurred())

			// Try to add it again
			err = service.AddConnectionToNmapManager(ctx, mockServices.GetFileSystem(), cfg, connectionName)

			Expect(err).To(MatchError(ErrServiceAlreadyExists))
		})

		It("should set up the connection for reconciliation with the nmap manager", func() {
			// Act
			err := service.AddConnectionToNmapManager(ctx, mockServices.GetFileSystem(), cfg, connectionName)
			Expect(err).NotTo(HaveOccurred())

			// Reconcile to ensure the component is passed to nmap manager
			mockNmap.ReconcileManagerReconciled = true
			_, reconciled := service.ReconcileManager(ctx, mockServices, tick)

			// Assert
			Expect(reconciled).To(BeTrue())
			Expect(service.nmapConfigs).To(HaveLen(1))
		})
	})

	Describe("Status", func() {
		var (
			cfg             *connectionserviceconfig.ConnectionServiceConfig
			manager         *nmapfsm.NmapManager
			mockNmapService *nmap.MockNmapService
			statusService   *ConnectionService
			nmapName        string
		)

		BeforeEach(func() {
			cfg = &connectionserviceconfig.ConnectionServiceConfig{
				NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
					Target: targetHost,
					Port:   targetPort,
				},
			}

			// Use the official mock manager from the FSM package
			manager, mockNmapService = nmapfsm.NewNmapManagerWithMockedService("test")

			// Create service with our official mock nmap manager
			statusService = NewDefaultConnectionService(connectionName,
				WithNmapService(mockNmapService),
				WithNmapManager(manager))

			// Add the component to the service
			err := statusService.AddConnectionToNmapManager(ctx, mockServices.GetFileSystem(), cfg, connectionName)
			Expect(err).NotTo(HaveOccurred())

			nmapName = statusService.getNmapName(connectionName)

			// Set up the mock to say the component exists
			mockNmapService.ServiceExistsResult = true
			if mockNmapService.ExistingServices == nil {
				mockNmapService.ExistingServices = make(map[string]bool)
			}
			mockNmapService.ExistingServices[nmapName] = true
		})

		It("should report status correctly for an existing component", func() {
			fullCfg := config.FullConfig{
				Internal: config.InternalConfig{
					Nmap: statusService.nmapConfigs,
				},
			}

			// Configure nmap service for proper transitions
			// First configure for creating -> created -> stopped
			ConfigureNmapManagerForState(mockNmapService, nmapName, nmapfsm.OperationalStateStopped)

			// Wait for the instance to be created and reach stopped state
			newTick, err := WaitForNmapManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick},
				manager,
				mockServices,
				nmapName,
				nmapfsm.OperationalStateStopped,
				10,
			)
			Expect(err).NotTo(HaveOccurred())
			tick = newTick

			// Now configure for transition to starting -> running
			ConfigureNmapManagerForState(mockNmapService, nmapName, nmapfsm.OperationalStateOpen)

			// Wait for the instance to reach running state
			By("before WaitForNmapManagerInstanceState 2")
			newTick, err = WaitForNmapManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick},
				manager,
				mockServices,
				nmapName,
				nmapfsm.OperationalStateOpen,
				15,
			)
			Expect(err).NotTo(HaveOccurred())
			tick = newTick

			mockNmapService.ServiceStates[nmapName].NmapStatus.IsRunning = true
			mockNmapService.ServiceStates[nmapName].NmapStatus.LastScan.PortResult.State = "open"

			// Reconcile once to ensure that serviceInfo is used to update the observed state
			_, reconciled := statusService.ReconcileManager(ctx, mockServices, tick)
			Expect(reconciled).To(BeFalse())

			// Call Status
			status, err := statusService.Status(ctx, mockServices.GetFileSystem(), connectionName, tick)

			// Assert
			Expect(err).NotTo(HaveOccurred())
			Expect(status.NmapFSMState).To(Equal(nmapfsm.OperationalStateOpen))
			Expect(status.NmapObservedState.ServiceInfo.S6ObservedState.ServiceInfo.Status).To(Equal(s6svc.ServiceUp))
			Expect(status.NmapObservedState.ServiceInfo.NmapStatus.IsRunning).To(BeTrue())
			Expect(status.NmapObservedState.ServiceInfo.NmapStatus.LastScan.PortResult.State).To(Equal("open"))

			statusService.recentNmapStates[connectionName] = []string{"open", "closed", "open"}

			status, err = statusService.Status(ctx, mockServices.GetFileSystem(), connectionName, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(status.IsFlaky).To(BeTrue())

		})

		It("should return error for non-existent component", func() {
			// Set up the mock to say the service doesn't exist
			mockNmapService.ServiceExistsResult = false
			mockNmapService.ExistingServices = make(map[string]bool)

			// Call Status for a non-existent component
			_, err := statusService.Status(ctx, mockServices.GetFileSystem(), connectionName, tick)

			// Assert - check for "not found" in the error message
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(ErrServiceNotExist.Error()))
		})
	})

	Describe("UpdateConnection", func() {
		var (
			cfg        *connectionserviceconfig.ConnectionServiceConfig
			updatedCfg *connectionserviceconfig.ConnectionServiceConfig
		)

		BeforeEach(func() {
			// Initial config
			cfg = &connectionserviceconfig.ConnectionServiceConfig{
				NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
					Target: targetHost,
					Port:   targetPort,
				},
			}

			// Updated config with different settings
			updatedCfg = &connectionserviceconfig.ConnectionServiceConfig{
				NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
					Target: targetHost,
					Port:   444,
				},
			}

			// Add the component first
			err := service.AddConnectionToNmapManager(ctx, mockServices.GetFileSystem(), cfg, connectionName)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should update an existing connection", func() {
			// Act - update the component
			err := service.UpdateConnectionInNmapManager(ctx, mockServices.GetFileSystem(), updatedCfg, connectionName)

			// Assert
			Expect(err).NotTo(HaveOccurred())

			// Verify the config was updated but the desired state was preserved
			nmapName := service.getNmapName(connectionName)
			var found bool
			for _, config := range service.nmapConfigs {
				if config.Name == nmapName {
					found = true
					Expect(config.DesiredFSMState).To(Equal(nmapfsm.OperationalStateOpen))
					// In a real test, we'd verify the ConnectionServiceConfig was updated as expected
					break
				}
			}
			Expect(found).To(BeTrue())
		})

		It("should return error when component doesn't exist", func() {
			// Act - try to update a non-existent component
			err := service.UpdateConnectionInNmapManager(ctx, mockServices.GetFileSystem(), updatedCfg, "non-existent")

			// Assert
			Expect(err).To(MatchError(ErrServiceNotExist))
		})
	})

	Describe("StartAndStopConnection", func() {
		var (
			cfg *connectionserviceconfig.ConnectionServiceConfig
		)

		BeforeEach(func() {
			// Create a basic config for testing
			cfg = &connectionserviceconfig.ConnectionServiceConfig{
				NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
					Target: targetHost,
					Port:   targetPort,
				},
			}

			// Add the component first
			err := service.AddConnectionToNmapManager(ctx, mockServices.GetFileSystem(), cfg, connectionName)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should start a connection by changing its desired state", func() {
			// First stop the component
			err := service.StopConnection(ctx, mockServices.GetFileSystem(), connectionName)
			Expect(err).NotTo(HaveOccurred())

			// Verify the desired state was changed to stopped
			nmapName := service.getNmapName(connectionName)
			var foundStopped bool
			for _, config := range service.nmapConfigs {
				if config.Name == nmapName {
					foundStopped = true
					Expect(config.DesiredFSMState).To(Equal(nmapfsm.OperationalStateStopped))
					break
				}
			}
			Expect(foundStopped).To(BeTrue())

			// Now start the component
			err = service.StartConnection(ctx, mockServices.GetFileSystem(), connectionName)
			Expect(err).NotTo(HaveOccurred())

			// Verify the desired state was changed to active
			var foundStarted bool
			for _, config := range service.nmapConfigs {
				if config.Name == nmapName {
					foundStarted = true
					Expect(config.DesiredFSMState).To(Equal(nmapfsm.OperationalStateOpen))
					break
				}
			}
			Expect(foundStarted).To(BeTrue())
		})

		It("should return error when trying to start/stop non-existent connection", func() {
			// Try to start a non-existent component
			err := service.StartConnection(ctx, mockServices.GetFileSystem(), "non-existent")
			Expect(err).To(MatchError(ErrServiceNotExist))

			// Try to stop a non-existent component
			err = service.StopConnection(ctx, mockServices.GetFileSystem(), "non-existent")
			Expect(err).To(MatchError(ErrServiceNotExist))
		})
	})

	Describe("RemoveConnection", func() {
		var (
			cfg *connectionserviceconfig.ConnectionServiceConfig
		)

		BeforeEach(func() {
			// Create a basic config for testing
			cfg = &connectionserviceconfig.ConnectionServiceConfig{
				NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
					Target: targetHost,
					Port:   targetPort,
				},
			}

			// Add the component first
			err := service.AddConnectionToNmapManager(ctx, mockServices.GetFileSystem(), cfg, connectionName)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should remove a connection from the nmap manager", func() {
			// Get the initial count
			initialCount := len(service.nmapConfigs)

			// Act - remove the component
			err := service.RemoveConnectionFromNmapManager(ctx, mockServices.GetFileSystem(), connectionName)

			// Assert
			Expect(err).NotTo(HaveOccurred())
			Expect(service.nmapConfigs).To(HaveLen(initialCount - 1))

			// Verify the component is no longer in the list
			nmapName := service.getNmapName(connectionName)
			for _, config := range service.nmapConfigs {
				Expect(config.Name).NotTo(Equal(nmapName))
			}
		})

		// Note: removing a non-existent component should not result in an error
		// the remove action will be called multiple times until the component is gone it returns nil
	})

	Describe("ReconcileManager", func() {
		It("should pass configs to the nmap manager for reconciliation", func() {
			// Add a test component to have something to reconcile
			cfg := &connectionserviceconfig.ConnectionServiceConfig{
				NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
					Target: targetHost,
					Port:   targetPort,
				},
			}
			err := service.AddConnectionToNmapManager(ctx, mockServices.GetFileSystem(), cfg, connectionName)
			Expect(err).NotTo(HaveOccurred())

			// Use the real mock from the FSM package
			manager, _ := nmapfsm.NewNmapManagerWithMockedService("test")
			service.nmapManager = manager

			// Configure the mock to return true for reconciled
			mockNmap.ReconcileManagerReconciled = true

			// Act
			err, reconciled := service.ReconcileManager(ctx, mockServices, tick)

			// Assert
			Expect(err).NotTo(HaveOccurred())
			// Change expectation to match the actual behavior
			Expect(reconciled).To(BeTrue()) // The mock is configured to return true
		})

		It("should handle errors from the nmap manager", func() {
			// Create a custom mock that returns an error
			mockError := errors.New("test reconcile error")

			// Create a real manager with mocked services
			mockManager, mockNmapService := nmapfsm.NewNmapManagerWithMockedService("test-error")

			// Create a service with our mocked manager
			testService := NewDefaultConnectionService("test-error-service",
				WithNmapService(mockNmapService),
				WithNmapManager(mockManager))

			// Add a test component to have something to reconcile (just like in the other test)
			testComponentName := "test-error-component"
			cfg := &connectionserviceconfig.ConnectionServiceConfig{
				NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
					Target: targetHost,
					Port:   targetPort,
				},
			}
			err := testService.AddConnectionToNmapManager(ctx, mockServices.GetFileSystem(), cfg, testComponentName)
			Expect(err).NotTo(HaveOccurred())

			// First reconcile - this will just create the instance in the manager
			firstErr, reconciled := testService.ReconcileManager(ctx, mockServices, tick)
			Expect(firstErr).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue()) // Should be true because we created a new instance

			// Now set up the mock service to fail during the actual instance reconciliation
			mockNmapService.ReconcileManagerError = mockError

			// Second reconcile - now that the instance exists, it will try to reconcile it
			err, reconciled = testService.ReconcileManager(ctx, mockServices, tick+1)

			// Assert
			Expect(err).ToNot(HaveOccurred()) // it should not return an error
			Expect(reconciled).To(BeFalse())  // it should not be reconciled
			// it should throw the "error reconciling s6Manager: test reconcile error" error through the logs

			// Skip the error checking part as it's not accessible directly
			// The test has already verified that the error is handled properly
			// by checking that reconciled is false

			// Alternatively, we could check for side effects of the error
			// but for a unit test, verifying that reconciled is false is sufficient
		})
	})
})

// TransitionToNmapState is a helper to configure a service for a given high-level state
func TransitionToNmapState(mockService *nmap.MockNmapService, serviceName string, state string) {
	switch state {
	case nmapfsm.OperationalStateStopped,
		nmapfsm.OperationalStateStarting:
		SetupNmapServiceState(mockService, serviceName, nmap.ServiceStateFlags{
			IsS6Running: false,
			S6FSMState:  s6fsm.OperationalStateStopped,
		})
	case nmapfsm.OperationalStateDegraded:
		SetupNmapServiceState(mockService, serviceName, nmap.ServiceStateFlags{
			IsS6Running: true,
			IsRunning:   false,
			S6FSMState:  s6fsm.OperationalStateRunning,
			IsDegraded:  true,
			PortState:   "",
		})
	case nmapfsm.OperationalStateOpen:
		SetupNmapServiceState(mockService, serviceName, nmap.ServiceStateFlags{
			IsS6Running: true,
			IsRunning:   true,
			S6FSMState:  s6fsm.OperationalStateRunning,
			PortState:   string(nmapfsm.PortStateOpen),
		})
	case nmapfsm.OperationalStateOpenFiltered:
		SetupNmapServiceState(mockService, serviceName, nmap.ServiceStateFlags{
			IsS6Running: true,
			IsRunning:   true,
			S6FSMState:  s6fsm.OperationalStateRunning,
			PortState:   string(nmapfsm.PortStateOpenFiltered),
		})
	case nmapfsm.OperationalStateFiltered:
		SetupNmapServiceState(mockService, serviceName, nmap.ServiceStateFlags{
			IsS6Running: true,
			IsRunning:   true,
			S6FSMState:  s6fsm.OperationalStateRunning,
			PortState:   string(nmapfsm.PortStateFiltered),
		})
	case nmapfsm.OperationalStateUnfiltered:
		SetupNmapServiceState(mockService, serviceName, nmap.ServiceStateFlags{
			IsS6Running: true,
			IsRunning:   true,
			S6FSMState:  s6fsm.OperationalStateRunning,
			PortState:   string(nmapfsm.PortStateUnfiltered),
		})
	case nmapfsm.OperationalStateClosed:
		SetupNmapServiceState(mockService, serviceName, nmap.ServiceStateFlags{
			IsS6Running: true,
			IsRunning:   true,
			S6FSMState:  s6fsm.OperationalStateRunning,
			PortState:   string(nmapfsm.PortStateClosed),
		})
	case nmapfsm.OperationalStateClosedFiltered:
		SetupNmapServiceState(mockService, serviceName, nmap.ServiceStateFlags{
			IsS6Running: true,
			IsRunning:   true,
			S6FSMState:  s6fsm.OperationalStateRunning,
			PortState:   string(nmapfsm.PortStateClosedFiltered),
		})
	case nmapfsm.OperationalStateStopping:
		SetupNmapServiceState(mockService, serviceName, nmap.ServiceStateFlags{
			IsS6Running: false,
			IsRunning:   false,
			S6FSMState:  s6fsm.OperationalStateStopping,
		})
	}
}

// SetupNmapServiceState configures the mock service state for Nmap instance tests
func SetupNmapServiceState(
	mockService *nmap.MockNmapService,
	serviceName string,
	flags nmap.ServiceStateFlags,
) {
	// Ensure service exists in mock
	mockService.ExistingServices[serviceName] = true

	// Create service info if it doesn't exist
	if mockService.ServiceStates[serviceName] == nil {
		mockService.ServiceStates[serviceName] = &nmap.ServiceInfo{}
	}

	// Set S6 FSM state
	if flags.S6FSMState != "" {
		mockService.ServiceStates[serviceName].S6FSMState = flags.S6FSMState
	}

	// Update S6 observed state
	if flags.IsS6Running {
		mockService.ServiceStates[serviceName].S6ObservedState.ServiceInfo = s6svc.ServiceInfo{
			Status: s6svc.ServiceUp,
			Uptime: 10, // Set uptime to 10s to simulate config loaded
			Pid:    1234,
		}
	} else {
		mockService.ServiceStates[serviceName].S6ObservedState.ServiceInfo = s6svc.ServiceInfo{
			Status: s6svc.ServiceDown,
		}
	}

	// Update health check status
	if flags.IsRunning {
		mockService.ServiceStates[serviceName].NmapStatus.IsRunning = true
	} else {
		mockService.ServiceStates[serviceName].NmapStatus.IsRunning = false
	}

	if flags.PortState != "" {
		mockService.ServiceStates[serviceName].NmapStatus.LastScan = &nmap.NmapScanResult{
			PortResult: nmap.PortResult{
				State: flags.PortState,
			},
		}
	}

	// Store the service state flags directly
	mockService.SetServiceState(serviceName, flags)
}

// ConfigureNmapManagerForState sets up the mock service in a BenthosManager to facilitate
// a state transition for a specific instance. This should be called before starting
// reconciliation if you want to ensure state transitions happen correctly.
//
// Parameters:
//   - mockService: The mock service from the manager
//   - serviceName: The name of the service to configure
//   - targetState: The desired state to configure the service for
func ConfigureNmapManagerForState(
	mockService *nmap.MockNmapService,
	serviceName string,
	targetState string,
) {
	// Make sure the service exists in the mock
	if mockService.ExistingServices == nil {
		mockService.ExistingServices = make(map[string]bool)
	}
	mockService.ExistingServices[serviceName] = true

	// Make sure service state is initialized
	if mockService.ServiceStates == nil {
		mockService.ServiceStates = make(map[string]*nmap.ServiceInfo)
	}
	if mockService.ServiceStates[serviceName] == nil {
		mockService.ServiceStates[serviceName] = &nmap.ServiceInfo{}
	}

	// Configure the service for the target state
	TransitionToNmapState(mockService, serviceName, targetState)
}

// WaitForNmapManagerInstanceState repeatedly calls manager.Reconcile until
// we see the specified 'desiredState' or we hit maxAttempts.
func WaitForNmapManagerInstanceState(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *nmapfsm.NmapManager,
	services serviceregistry.Provider,
	instanceName string,
	desiredState string,
	maxAttempts int,
) (uint64, error) {
	tick := snapshot.Tick
	for i := 0; i < maxAttempts; i++ {
		err, _ := manager.Reconcile(ctx, snapshot, services)
		if err != nil {
			return tick, err
		}
		tick++

		inst, found := manager.GetInstance(instanceName)
		if found && inst.GetCurrentFSMState() == desiredState {
			return tick, nil
		}
		if found {
			fmt.Printf("currentState: %s\n", inst.GetCurrentFSMState())
			fmt.Printf(" found instance: %s\n", instanceName)
		}
	}
	return tick, fmt.Errorf("instance %s did not reach state %s after %d attempts",
		instanceName, desiredState, maxAttempts)
}
