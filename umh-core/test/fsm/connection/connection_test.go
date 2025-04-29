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

package connection_test

import (
	"context"
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/backoff"
	pkgfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/connection"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/nmap"
	connectionsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/connection"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsmtest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Connection FSM", func() {
	var (
		instance        *connection.ConnectionInstance
		mockService     *connectionsvc.MockConnectionService
		connectionName  string
		ctx             context.Context
		tick            uint64
		mockSvcRegistry *serviceregistry.Registry
	)

	BeforeEach(func() {
		connectionName = "test-connection-fsm"
		ctx = context.Background()
		tick = 0

		instance, mockService, _ = fsmtest.SetupConnectionInstance(connectionName, connection.OperationalStateUp)
		mockSvcRegistry = serviceregistry.NewMockRegistry()
	})

	Context("Basic State Transitions", func() {
		It("should transition from stopped to starting", func() {
			var err error

			// Setup to Stopped state
			// ToBeCreated -> Creating
			tick, err = fsmtest.TestConnectionStateTransition(
				ctx, instance, mockService, mockSvcRegistry, connectionName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Creating -> Stopped
			tick, err = fsmtest.TestConnectionStateTransition(
				ctx, instance, mockService, mockSvcRegistry, connectionName,
				internalfsm.LifecycleStateCreating,
				connection.OperationalStateStopped, 5, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddConnectionToNmapManagerCalled).To(BeTrue())

			// Set desired state to Up
			Expect(instance.SetDesiredFSMState(connection.OperationalStateUp)).To(Succeed())

			// Transition from Stopped to Starting
			fsmtest.TransitionToConnectionState(mockService, connectionName, connection.OperationalStateStarting)

			// Execute transition: Stopped -> Starting
			tick, err = fsmtest.TestConnectionStateTransition(
				ctx, instance, mockService, mockSvcRegistry, connectionName,
				connection.OperationalStateStopped,
				connection.OperationalStateStarting, 5, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.StartConnectionCalled).To(BeTrue())
		})
		It("should transition from starting to up", func() {
			var err error

			// Setup to Stopped state
			// ToBeCreated -> Creating
			tick, err = fsmtest.TestConnectionStateTransition(
				ctx, instance, mockService, mockSvcRegistry, connectionName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Creating -> Stopped
			tick, err = fsmtest.TestConnectionStateTransition(
				ctx, instance, mockService, mockSvcRegistry, connectionName,
				internalfsm.LifecycleStateCreating,
				connection.OperationalStateStopped, 5, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddConnectionToNmapManagerCalled).To(BeTrue())

			// Set desired state to Up
			Expect(instance.SetDesiredFSMState(connection.OperationalStateUp)).To(Succeed())

			// Transition from Stopped to Starting
			fsmtest.TransitionToConnectionState(mockService, connectionName, connection.OperationalStateStarting)

			// Execute transition: Stopped -> Starting
			tick, err = fsmtest.TestConnectionStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				connectionName,
				connection.OperationalStateStopped,
				connection.OperationalStateStarting, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Set the mock flags for an up state
			fsmtest.TransitionToConnectionState(mockService, connectionName, connection.OperationalStateUp)

			// Execute transition: Starting -> up
			tick, err = fsmtest.TestConnectionStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				connectionName,
				connection.OperationalStateStarting,
				connection.OperationalStateUp,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Stopping Flow", func() {
		It("should force-remove component on permanent error when already Stopped", func() {
			// Setup to Active state
			// 1. First get to Stopped state
			tick, err := fsmtest.TestConnectionStateTransition(
				ctx, instance, mockService, mockSvcRegistry, connectionName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Creating → Stopped
			tick, err = fsmtest.TestConnectionStateTransition(
				ctx, instance, mockService, mockSvcRegistry, connectionName,
				internalfsm.LifecycleStateCreating,
				connection.OperationalStateStopped, 5, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddConnectionToNmapManagerCalled).To(BeTrue())

			// inject permanent error
			mockService.StatusError = fmt.Errorf("%s: simulated", backoff.PermanentFailureError)

			_, recErr, reconciled := fsmtest.ReconcileConnectionUntilError(
				ctx,
				pkgfsm.SystemSnapshot{Tick: tick},
				instance,
				mockService,
				mockSvcRegistry,
				connectionName,
				5,
			)

			Expect(recErr).To(HaveOccurred())
			Expect(recErr.Error()).To(ContainSubstring(backoff.PermanentFailureError))
			Expect(reconciled).To(BeTrue())
			Expect(mockService.ForceRemoveConnectionCalled).To(BeTrue())

			mockService.StatusError = nil // cleanup for other tests
		})

	})

	Context("Error Handling", func() {
		It("should handle connection service not exist error correctly", func() {
			var err error
			// Setup to Creating state
			tick, err = fsmtest.TestConnectionStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				connectionName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Force the exact error message from the logs with proper error wrapping
			mockService.StatusError = fmt.Errorf("failed to get connection config: failed to get connection config file for service %s: %w", connectionName, connectionsvc.ErrServiceNotExist)

			// Attempt to reconcile - this should not set an FSM error if errors are correctly identified
			snapshot := pkgfsm.SystemSnapshot{Tick: tick}
			err, _ = instance.Reconcile(ctx, snapshot, mockSvcRegistry)
			Expect(err).To(BeNil()) // Should not propagate an error

			// The instance should not have an error set
			Expect(instance.GetLastError()).To(BeNil())

			// Clean up the mock for other tests
			mockService.StatusError = nil
		})
	})

})

var _ = Describe("ConnectionInstance state transitions", func() {
	var (
		ctx             context.Context
		instance        *connection.ConnectionInstance
		mockService     *connectionsvc.MockConnectionService
		mockSvcRegistry *serviceregistry.Registry
		serviceName     string
		tick            uint64
	)

	// Bring the instance to Degraded once for every test‑row so each scenario
	// starts at the same baseline.
	BeforeEach(func() {
		ctx = context.Background()
		tick = 0
		serviceName = "test-connection-fsm"

		instance, mockService, _ = fsmtest.SetupConnectionInstance(serviceName, connection.OperationalStateStopped)
		mockSvcRegistry = serviceregistry.NewMockRegistry()

		// to_be_created -> creating
		tick, _ = fsmtest.TestConnectionStateTransition(
			ctx, instance, mockService, mockSvcRegistry, serviceName,
			internalfsm.LifecycleStateToBeCreated,
			internalfsm.LifecycleStateCreating,
			5, tick,
		)
		// creating -> stopped
		mockService.ConnectionStates[serviceName] = &connectionsvc.ServiceInfo{NmapFSMState: nmap.OperationalStateStopped}
		mockService.ExistingConnections[serviceName] = true
		tick, _ = fsmtest.TestConnectionStateTransition(
			ctx, instance, mockService, mockSvcRegistry, serviceName,
			internalfsm.LifecycleStateCreating,
			connection.OperationalStateStopped,
			5, tick,
		)

		// let the instance start scanning
		Expect(instance.SetDesiredFSMState(connection.OperationalStateUp)).To(Succeed())
		// stopped -> starting
		tick, _ = fsmtest.TestConnectionStateTransition(
			ctx, instance, mockService, mockSvcRegistry, serviceName,
			connection.OperationalStateStopped,
			connection.OperationalStateStarting,
			5, tick,
		)
		// starting -> up (service up, port up)
		mockService.SetConnectionState(serviceName, connectionsvc.ConnectionStateFlags{
			IsNmapRunning: true,
			IsFlaky:       false,
			NmapFSMState:  nmap.OperationalStateOpen,
		})
		tick, _ = fsmtest.TestConnectionStateTransition(
			ctx, instance, mockService, mockSvcRegistry, serviceName,
			connection.OperationalStateStarting,
			connection.OperationalStateUp,
			5, tick,
		)
		Expect(instance.GetCurrentFSMState()).To(Equal(connection.OperationalStateUp))
	})
	AfterEach(func() {
		Expect(instance.SetDesiredFSMState(connection.OperationalStateStopped)).To(Succeed())

		mockService.SetConnectionState(serviceName, connectionsvc.ConnectionStateFlags{
			IsNmapRunning: false,
			NmapFSMState:  nmap.OperationalStateStopped,
		})

		current := instance.GetCurrentFSMState()
		if current != connection.OperationalStateStopping && current != connection.OperationalStateStopped {
			var err error
			tick, err = fsmtest.TestConnectionStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				serviceName,
				current,
				connection.OperationalStateStopping,
				5, tick,
			)
			Expect(err).NotTo(HaveOccurred())
		}

		if instance.GetCurrentFSMState() != connection.OperationalStateStopped {
			var err error
			tick, err = fsmtest.TestConnectionStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				serviceName,
				connection.OperationalStateStopping,
				connection.OperationalStateStopped,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(instance.GetCurrentFSMState()).To(Equal(connection.OperationalStateStopped))
	})

	DescribeTable("transitions: up -> from -> to",
		func(fromState, toState, nmapFromState, nmapToState string, fromIsFlaky, toIsFlaky bool) {
			// Up -> fromState
			mockService.SetConnectionState(serviceName, connectionsvc.ConnectionStateFlags{
				IsNmapRunning: true,
				IsFlaky:       fromIsFlaky,
				NmapFSMState:  nmapFromState,
			})
			var err error
			tick, err = fsmtest.TestConnectionStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				serviceName,
				connection.OperationalStateUp,
				fromState,
				10,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// fromState -> toState
			mockService.SetConnectionState(serviceName, connectionsvc.ConnectionStateFlags{
				IsNmapRunning: true,
				IsFlaky:       toIsFlaky,
				NmapFSMState:  nmapToState,
			})
			tick, err = fsmtest.TestConnectionStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				serviceName,
				fromState,
				toState,
				10,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetCurrentFSMState()).To(Equal(toState))
		},
		Entry(
			"up -> down (nmap closed)",
			connection.OperationalStateUp,
			connection.OperationalStateDown,
			nmap.OperationalStateOpen,
			nmap.OperationalStateClosed,
			false,
			false,
		),
		Entry(
			"up -> degraded (w flaky)",
			connection.OperationalStateUp,
			connection.OperationalStateDegraded,
			nmap.OperationalStateOpen,
			nmap.OperationalStateOpen,
			false,
			true,
		),
		Entry(
			"down -> up (from closed)",
			connection.OperationalStateDown,
			connection.OperationalStateUp,
			nmap.OperationalStateClosed,
			nmap.OperationalStateOpen,
			false,
			false,
		),
		Entry(
			"down -> degraded (w flaky)",
			connection.OperationalStateDown,
			connection.OperationalStateDegraded,
			nmap.OperationalStateClosed,
			nmap.OperationalStateOpen,
			false,
			true,
		),
		Entry(
			"degraded -> up",
			connection.OperationalStateDegraded,
			connection.OperationalStateUp,
			nmap.OperationalStateOpen,
			nmap.OperationalStateOpen,
			true,
			false,
		),
		Entry(
			"degraded -> down",
			connection.OperationalStateDegraded,
			connection.OperationalStateDown,
			nmap.OperationalStateOpen,
			nmap.OperationalStateClosed,
			true,
			false,
		),
	)
})
