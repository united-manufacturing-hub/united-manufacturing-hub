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

package bridge_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsmtest"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	bridgefsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/bridge"
	connectionfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/connection"
	dataflowcomponentfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	nmapfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/nmap"
	redpandafsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/redpanda"
	bridgesvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/bridge"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

var _ = Describe("Bridge FSM", func() {
	var (
		instance      *bridgefsm.Instance
		mockService   *bridgesvc.MockService
		componentName string
		ctx           context.Context
		tick          uint64
		mockRegistry  *serviceregistry.Registry
		startTime     time.Time
	)

	BeforeEach(func() {
		componentName = "test-bridge"
		ctx = context.Background()
		tick = 0
		startTime = time.Now()

		// Initialize a Bridge instance with default desired state (Stopped to begin with)
		instance, mockService, _ = fsmtest.SetupBridgeInstance(componentName, bridgefsm.OperationalStateStopped)
		mockRegistry = serviceregistry.NewMockRegistry()
	})

	// =========================================================================
	//  BASIC STATE TRANSITIONS
	// =========================================================================
	Context("Basic State Transitions", func() {
		It("should transition from Stopped to StartingConnection when activated", func() {
			var err error

			// Phase 1: Initial lifecycle setup (to_be_created -> creating -> stopped)
			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Simulate successful creation: service now exists in stopped state
			mockService.ExistingComponents[componentName] = true
			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStopped)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateCreating,
				bridgefsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddToManagerCalled).To(BeTrue()) // added to manager on creation

			// Phase 2: Activation sequence (set desired state to Active)
			Expect(instance.SetDesiredFSMState(bridgefsm.OperationalStateActive)).To(Succeed())
			Expect(instance.GetDesiredFSMState()).To(Equal(bridgefsm.OperationalStateActive))

			// Execute transition: Stopped -> StartingConnection
			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStopped,
				bridgefsm.OperationalStateStartingConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.StartCalled).To(BeTrue()) // ensure start initiated
		})

		It("should transition from StartingConnection to StartingRedpanda when connection becomes active", func() {
			var err error

			// Phase 1: Setup to stopped state
			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[componentName] = true
			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStopped)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateCreating,
				bridgefsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Activate and reach StartingConnection
			Expect(instance.SetDesiredFSMState(bridgefsm.OperationalStateActive)).To(Succeed())

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStopped,
				bridgefsm.OperationalStateStartingConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 3: Simulate connection established, transition to StartingRedpanda
			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStartingRedpanda)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingConnection,
				bridgefsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should transition from StartingRedpanda to StartingDFC when Redpanda is ready", func() {
			var err error

			// Phase 1: Setup to stopped state
			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[componentName] = true
			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStopped)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateCreating,
				bridgefsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Progress through connection startup to StartingRedpanda
			Expect(instance.SetDesiredFSMState(bridgefsm.OperationalStateActive)).To(Succeed())

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStopped,
				bridgefsm.OperationalStateStartingConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStartingRedpanda)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingConnection,
				bridgefsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 3: Simulate Redpanda ready, transition to StartingDFC
			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStartingDFC)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingRedpanda,
				bridgefsm.OperationalStateStartingDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should transition from StartingDFC to Idle when DFC is up and running", func() {
			var err error

			// Phase 1: Setup through StartingDFC state
			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[componentName] = true
			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStopped)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateCreating,
				bridgefsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(bridgefsm.OperationalStateActive)).To(Succeed())

			// Progress through startup sequence
			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStopped,
				bridgefsm.OperationalStateStartingConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStartingRedpanda)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingConnection,
				bridgefsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStartingDFC)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingRedpanda,
				bridgefsm.OperationalStateStartingDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Simulate DFC successfully started, transition to Idle
			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateIdle)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingDFC,
				bridgefsm.OperationalStateIdle, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should transition from Idle to Active when processing data", func() {
			var err error

			// Phase 1: Reach Idle state through full startup sequence
			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[componentName] = true
			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStopped)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateCreating,
				bridgefsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(bridgefsm.OperationalStateActive)).To(Succeed())

			// Progress through starting states to Idle
			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStopped,
				bridgefsm.OperationalStateStartingConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStartingRedpanda)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingConnection,
				bridgefsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStartingDFC)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingRedpanda,
				bridgefsm.OperationalStateStartingDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateIdle)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingDFC,
				bridgefsm.OperationalStateIdle, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Simulate data flow starting (Idle -> Active)
			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateActive)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateIdle,
				bridgefsm.OperationalStateActive, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should transition from Active back to Idle when processing stops", func() {
			var err error

			// Phase 1: Reach Active state through full sequence
			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[componentName] = true
			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStopped)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateCreating,
				bridgefsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(bridgefsm.OperationalStateActive)).To(Succeed())

			// Progress through startup sequence to Active
			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStopped,
				bridgefsm.OperationalStateStartingConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStartingRedpanda)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingConnection,
				bridgefsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStartingDFC)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingRedpanda,
				bridgefsm.OperationalStateStartingDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateIdle)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingDFC,
				bridgefsm.OperationalStateIdle, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateActive)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateIdle,
				bridgefsm.OperationalStateActive, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Simulate stopping of data flow (Active -> Idle)
			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateIdle)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateActive,
				bridgefsm.OperationalStateIdle, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// =========================================================================
	//  DEGRADED AND RECOVERY SCENARIOS
	// =========================================================================
	Context("Degraded and Recovery Scenarios", func() {
		It("should transition to DegradedConnection when connection is lost and recover to Idle when restored", func() {
			var err error

			// Phase 1: Establish Idle state
			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[componentName] = true
			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStopped)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateCreating,
				bridgefsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(bridgefsm.OperationalStateActive)).To(Succeed())

			// Progress through startup to Idle
			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStopped,
				bridgefsm.OperationalStateStartingConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStartingRedpanda)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingConnection,
				bridgefsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStartingDFC)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingRedpanda,
				bridgefsm.OperationalStateStartingDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateIdle)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingDFC,
				bridgefsm.OperationalStateIdle, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Simulate connection failure and recovery
			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateDegradedConnection)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateIdle,
				bridgefsm.OperationalStateDegradedConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Restore connection and verify recovery
			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateIdle)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateDegradedConnection,
				bridgefsm.OperationalStateIdle, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should transition to DegradedRedpanda when Redpanda becomes unavailable and recover when restored", func() {
			var err error

			// Phase 1: Establish Idle state
			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[componentName] = true
			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStopped)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateCreating,
				bridgefsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(bridgefsm.OperationalStateActive)).To(Succeed())

			// Progress through startup to Idle
			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStopped,
				bridgefsm.OperationalStateStartingConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStartingRedpanda)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingConnection,
				bridgefsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStartingDFC)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingRedpanda,
				bridgefsm.OperationalStateStartingDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateIdle)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingDFC,
				bridgefsm.OperationalStateIdle, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Simulate Redpanda failure and recovery
			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateDegradedRedpanda)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateIdle,
				bridgefsm.OperationalStateDegradedRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Restore Redpanda and verify recovery
			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateIdle)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateDegradedRedpanda,
				bridgefsm.OperationalStateIdle, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should transition to DegradedDFC when the DFC goes down and recover when it comes back", func() {
			var err error

			// Phase 1: Establish Idle state
			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[componentName] = true
			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStopped)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateCreating,
				bridgefsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(bridgefsm.OperationalStateActive)).To(Succeed())

			// Progress through startup to Idle
			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStopped,
				bridgefsm.OperationalStateStartingConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStartingRedpanda)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingConnection,
				bridgefsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStartingDFC)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingRedpanda,
				bridgefsm.OperationalStateStartingDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateIdle)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingDFC,
				bridgefsm.OperationalStateIdle, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Simulate DFC failure and recovery
			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateDegradedDFC)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateIdle,
				bridgefsm.OperationalStateDegradedDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Restore DFC and verify recovery
			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateIdle)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateDegradedDFC,
				bridgefsm.OperationalStateIdle, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should transition to DegradedOther for inconsistent state and recover when resolved", func() {
			var err error

			// Phase 1: Establish Idle state
			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[componentName] = true
			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStopped)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateCreating,
				bridgefsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(bridgefsm.OperationalStateActive)).To(Succeed())

			// Progress through startup to Idle
			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStopped,
				bridgefsm.OperationalStateStartingConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStartingRedpanda)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingConnection,
				bridgefsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStartingDFC)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingRedpanda,
				bridgefsm.OperationalStateStartingDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateIdle)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingDFC,
				bridgefsm.OperationalStateIdle, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Force inconsistent state scenario and recovery
			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateDegradedOther)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateIdle,
				bridgefsm.OperationalStateDegradedOther, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Resolve inconsistency and verify recovery
			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateIdle)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateDegradedOther,
				bridgefsm.OperationalStateIdle, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// =========================================================================
	//  STARTUP FAILURE SCENARIOS
	// =========================================================================
	Context("Startup Failure Scenarios", func() {
		It("should transition to StartingFailedDFC when the DFC fails to start", func() {
			Skip("needs to be implemented first in the DFC")
			var err error

			// Phase 1: Progress to StartingDFC state
			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[componentName] = true
			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStopped)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateCreating,
				bridgefsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(bridgefsm.OperationalStateActive)).To(Succeed())

			// Progress through startup sequence to StartingDFC
			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStopped,
				bridgefsm.OperationalStateStartingConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStartingRedpanda)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingConnection,
				bridgefsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStartingDFC)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingRedpanda,
				bridgefsm.OperationalStateStartingDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Simulate DFC startup failure
			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStartingFailedDFC)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingDFC,
				bridgefsm.OperationalStateStartingFailedDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should transition to StartingFailedDFCMissing when the DFC is not present", func() {
			// Setup instance with missing DFC
			instance, mockService, _ = fsmtest.SetupBridgeInstanceWithMissingDFC(componentName, bridgefsm.OperationalStateStopped)

			var err error

			// Phase 1: Progress to StartingDFC state
			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[componentName] = true
			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStopped)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateCreating,
				bridgefsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(bridgefsm.OperationalStateActive)).To(Succeed())

			// Progress through startup sequence to StartingDFC
			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStopped,
				bridgefsm.OperationalStateStartingConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStartingRedpanda)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingConnection,
				bridgefsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStartingDFC)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingRedpanda,
				bridgefsm.OperationalStateStartingDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Simulate DFC component missing entirely
			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStartingFailedDFCMissing)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingDFC,
				bridgefsm.OperationalStateStartingFailedDFCMissing, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// =========================================================================
	//  STOPPING FLOW
	// =========================================================================
	Context("Stopping Flow", func() {
		// Helper functions to setup different states
		setupActive := func() {
			var err error
			// Reset for clean state
			instance, mockService, _ = fsmtest.SetupBridgeInstance(componentName, bridgefsm.OperationalStateStopped)
			tick = 0

			// Progress to Active state
			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[componentName] = true
			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStopped)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateCreating,
				bridgefsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(bridgefsm.OperationalStateActive)).To(Succeed())

			// Progress through startup to Active
			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStopped,
				bridgefsm.OperationalStateStartingConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStartingRedpanda)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingConnection,
				bridgefsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStartingDFC)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingRedpanda,
				bridgefsm.OperationalStateStartingDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateIdle)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingDFC,
				bridgefsm.OperationalStateIdle, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateActive)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateIdle,
				bridgefsm.OperationalStateActive, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		}

		setupIdle := func() {
			var err error
			// Reset for clean state
			instance, mockService, _ = fsmtest.SetupBridgeInstance(componentName, bridgefsm.OperationalStateStopped)
			tick = 0

			// Progress to Idle state (similar to setupActive but stop at Idle)
			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[componentName] = true
			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStopped)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateCreating,
				bridgefsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(bridgefsm.OperationalStateActive)).To(Succeed())

			// Progress through startup to Idle
			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStopped,
				bridgefsm.OperationalStateStartingConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStartingRedpanda)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingConnection,
				bridgefsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStartingDFC)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingRedpanda,
				bridgefsm.OperationalStateStartingDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateIdle)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingDFC,
				bridgefsm.OperationalStateIdle, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		}

		setupStartingConnection := func() {
			var err error
			// Reset for clean state
			instance, mockService, _ = fsmtest.SetupBridgeInstance(componentName, bridgefsm.OperationalStateStopped)
			tick = 0

			// Progress to StartingConnection
			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[componentName] = true
			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStopped)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateCreating,
				bridgefsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(bridgefsm.OperationalStateActive)).To(Succeed())

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStopped,
				bridgefsm.OperationalStateStartingConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		}

		setupStartingRedpanda := func() {
			setupStartingConnection()
			var err error

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStartingRedpanda)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingConnection,
				bridgefsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		}

		setupStartingDFC := func() {
			setupStartingRedpanda()
			var err error

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStartingDFC)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingRedpanda,
				bridgefsm.OperationalStateStartingDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		}

		setupStartingFailedDFCMissing := func() {
			var err error
			// Setup instance with missing DFC
			instance, mockService, _ = fsmtest.SetupBridgeInstanceWithMissingDFC(componentName, bridgefsm.OperationalStateStopped)
			tick = 0

			// Progress to StartingFailedDFCMissing
			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[componentName] = true
			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStopped)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateCreating,
				bridgefsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(bridgefsm.OperationalStateActive)).To(Succeed())

			// Progress through startup sequence to StartingFailedDFCMissing
			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStopped,
				bridgefsm.OperationalStateStartingConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStartingRedpanda)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingConnection,
				bridgefsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStartingDFC)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingRedpanda,
				bridgefsm.OperationalStateStartingDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStartingFailedDFCMissing)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStartingDFC,
				bridgefsm.OperationalStateStartingFailedDFCMissing, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		}

		setupDegradedConnection := func() {
			setupIdle()
			var err error

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateDegradedConnection)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateIdle,
				bridgefsm.OperationalStateDegradedConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		}

		setupDegradedRedpanda := func() {
			setupIdle()
			var err error

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateDegradedRedpanda)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateIdle,
				bridgefsm.OperationalStateDegradedRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		}

		setupDegradedDFC := func() {
			setupIdle()
			var err error

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateDegradedDFC)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateIdle,
				bridgefsm.OperationalStateDegradedDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		}

		setupDegradedOther := func() {
			setupIdle()
			var err error

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateDegradedOther)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateIdle,
				bridgefsm.OperationalStateDegradedOther, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		}

		// Table-driven test covering all possible stopping transitions
		DescribeTable("should stop gracefully from any state",
			func(fromState string, setupState func()) {
				var err error

				// Phase 1: Setup the instance to the starting state
				setupState()

				// Verify we're in the expected starting state
				Expect(instance.GetCurrentFSMState()).To(Equal(fromState))

				// Phase 2: Request graceful stop
				Expect(instance.SetDesiredFSMState(bridgefsm.OperationalStateStopped)).To(Succeed())

				// Phase 3: Verify transition to stopping
				tick, err = fsmtest.TestBridgeStateTransition(
					ctx, instance, mockService, mockRegistry, componentName,
					fromState,
					bridgefsm.OperationalStateStopping, 10, tick, startTime)
				Expect(err).NotTo(HaveOccurred())
				Expect(mockService.StopCalled).To(BeTrue())

				// Phase 4: Complete shutdown
				fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateStopped)

				tick, err = fsmtest.TestBridgeStateTransition(
					ctx, instance, mockService, mockRegistry, componentName,
					bridgefsm.OperationalStateStopping,
					bridgefsm.OperationalStateStopped, 5, tick, startTime)
				Expect(err).NotTo(HaveOccurred())
			},

			// Starting states
			Entry("from starting_connection", bridgefsm.OperationalStateStartingConnection, setupStartingConnection),
			Entry("from starting_redpanda", bridgefsm.OperationalStateStartingRedpanda, setupStartingRedpanda),
			Entry("from starting_dfc", bridgefsm.OperationalStateStartingDFC, setupStartingDFC),
			// Note: starting_failed_dfc is excluded because DFCs cannot reach this state in practice yet.
			// The business logic supports stopping from this state, but test setup is complex and
			// the state is not reachable through normal DFC lifecycle. Will add test when DFC
			// failure scenarios are properly implemented.
			// Entry("from starting_failed_dfc", bridgefsm.OperationalStateStartingFailedDFC, setupStartingFailedDFC),
			Entry("from starting_failed_dfc_missing", bridgefsm.OperationalStateStartingFailedDFCMissing, setupStartingFailedDFCMissing),

			// Running states
			Entry("from idle", bridgefsm.OperationalStateIdle, setupIdle),
			Entry("from active", bridgefsm.OperationalStateActive, setupActive),

			// Degraded states
			Entry("from degraded_connection", bridgefsm.OperationalStateDegradedConnection, setupDegradedConnection),
			Entry("from degraded_redpanda", bridgefsm.OperationalStateDegradedRedpanda, setupDegradedRedpanda),
			Entry("from degraded_dfc", bridgefsm.OperationalStateDegradedDFC, setupDegradedDFC),
			Entry("from degraded_other", bridgefsm.OperationalStateDegradedOther, setupDegradedOther),
		)
	})

	// =========================================================================
	//  STATE STABILITY
	// =========================================================================
	Context("State Stability", func() {
		It("should remain stable in Idle state when no activity occurs", func() {
			var err error

			// Set to active from stopped state
			Expect(instance.SetDesiredFSMState(bridgefsm.OperationalStateActive)).To(Succeed())

			// Phase 1: Establish Idle state
			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				bridgefsm.OperationalStateIdle, 15, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Verify stability over multiple reconcile cycles
			tick, err = fsmtest.VerifyBridgeStableState(
				ctx, fsm.SystemSnapshot{Tick: tick}, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateIdle, 5)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should remain stable in Active state when processing continues", func() {
			var err error

			// Set to active from stopped state
			Expect(instance.SetDesiredFSMState(bridgefsm.OperationalStateActive)).To(Succeed())

			// Phase 1: Establish Active state
			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				bridgefsm.OperationalStateIdle, 15, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToBridgeState(mockService, componentName, bridgefsm.OperationalStateActive)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateIdle,
				bridgefsm.OperationalStateActive, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Verify stability over multiple reconcile cycles
			tick, err = fsmtest.VerifyBridgeStableState(
				ctx, fsm.SystemSnapshot{Tick: tick}, instance, mockService, mockRegistry, componentName,
				bridgefsm.OperationalStateActive, 5)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// =========================================================================
	//  CONFIGURATION VALIDATION
	// =========================================================================
	Context("Configuration Validation", func() {
		It("should get stuck in creating state with error message when config is invalid", func() {
			var err error

			// Create an instance with invalid port configuration
			invalidInstance, invalidMockService, _ := fsmtest.SetupBridgeInstanceWithInvalidPort(componentName, bridgefsm.OperationalStateStopped, "invalid-port")

			// Attempt reconciliation several times
			finalTick, err, _ := fsmtest.ReconcileBridgeUntilError(
				ctx, fsm.SystemSnapshot{Tick: tick}, invalidInstance, invalidMockService, mockRegistry, componentName, 5)

			// Should NOT get an error propagated up (FSM handles it gracefully)
			Expect(err).NotTo(HaveOccurred())
			Expect(finalTick).To(BeNumerically(">", tick))

			// The instance should get stuck in creating state
			currentState := invalidInstance.GetCurrentFSMState()
			Expect(currentState).To(Equal(internalfsm.LifecycleStateCreating))

			// The observed state should contain the configuration error in StatusReason
			observedState := invalidInstance.GetLastObservedState()
			Expect(observedState).NotTo(BeNil())
			if pcObservedState, ok := observedState.(bridgefsm.ObservedState); ok {
				Expect(pcObservedState.ServiceInfo.StatusReason).To(ContainSubstring("config error"))
				Expect(pcObservedState.ServiceInfo.StatusReason).To(ContainSubstring("invalid syntax"))
			} else {
				Fail("Could not cast observed state to BridgeObservedState")
			}
		})

		It("should fail during startup when connection target is unreachable", func() {
			var err error

			// Create a standard instance (we'll configure the mock to simulate unreachable port)
			// This reproduces the exact scenario from the bug report: localhost:8082 should be unreachable
			unreachableInstance, unreachableMockService, _ := fsmtest.SetupBridgeInstance(
				componentName, bridgefsm.OperationalStateStopped)

			// Set desired state to active to trigger startup sequence
			Expect(unreachableInstance.SetDesiredFSMState(bridgefsm.OperationalStateActive)).To(Succeed())

			// Progress through initial lifecycle creation
			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, unreachableInstance, unreachableMockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Simulate service created but with stopped state (normal startup)
			unreachableMockService.ExistingComponents[componentName] = true
			fsmtest.TransitionToBridgeState(unreachableMockService, componentName, bridgefsm.OperationalStateStopped)

			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, unreachableInstance, unreachableMockService, mockRegistry, componentName,
				internalfsm.LifecycleStateCreating,
				bridgefsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Now try to start - this should get to starting_connection
			tick, err = fsmtest.TestBridgeStateTransition(
				ctx, unreachableInstance, unreachableMockService, mockRegistry, componentName,
				bridgefsm.OperationalStateStopped,
				bridgefsm.OperationalStateStartingConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// HERE IS THE KEY: Configure the connection as DOWN/CLOSED to simulate unreachable port
			// This simulates what would happen if nmap found port 8082 to be closed
			fsmtest.SetupBridgeServiceState(unreachableMockService, componentName,
				bridgesvc.StateFlags{
					IsDFCRunning:       false,
					IsConnectionUp:     false, // Connection is DOWN because port is unreachable
					IsRedpandaRunning:  false,
					DfcFSMReadState:    dataflowcomponentfsm.OperationalStateStopped,
					ConnectionFSMState: connectionfsm.OperationalStateDown, // Connection is DOWN!
					RedpandaFSMState:   redpandafsm.OperationalStateStopped,
					PortState:          nmapfsm.PortStateClosed, // Port is CLOSED (unreachable)
				})

			// The instance should stay stuck in starting_connection because connection is down
			// Let's try multiple reconcile cycles to confirm it doesn't progress past starting states
			for i := 0; i < 10; i++ {
				tick++
				currentSnapshot := fsm.SystemSnapshot{Tick: tick}
				_, reconciled := unreachableInstance.Reconcile(ctx, currentSnapshot, mockRegistry)
				_ = reconciled

				currentState := unreachableInstance.GetCurrentFSMState()

				// The bug is that it somehow progresses past starting states to degraded_other
				// It should stay in starting_connection or starting_* states when connection is down
				if currentState == bridgefsm.OperationalStateDegradedOther {
					Fail(fmt.Sprintf("BUG REPRODUCED: Instance progressed to degraded_other (state=%s) instead of staying in starting states. This means the connection test didn't catch the unreachable port during startup. Connection state was: %s", currentState, unreachableInstance.ObservedState.ServiceInfo.ConnectionFSMState))
				}

				// Also check if it wrongly progresses to other running states
				if currentState == bridgefsm.OperationalStateIdle || currentState == bridgefsm.OperationalStateActive {
					Fail(fmt.Sprintf("BUG REPRODUCED: Instance progressed to running state %s despite connection being down. This means the connection check during startup is not working properly.", currentState))
				}

				// Expected behavior: should stay in starting_connection since connection is down
				Expect(currentState).To(Equal(bridgefsm.OperationalStateStartingConnection),
					fmt.Sprintf("Instance should stay in starting_connection when connection is down, but got: %s. Connection state: %s",
						currentState, unreachableInstance.ObservedState.ServiceInfo.ConnectionFSMState))
			}

			// Verify the connection state shows it's not up
			connectionUp, reason := unreachableInstance.IsConnectionUp()
			Expect(connectionUp).To(BeFalse(), fmt.Sprintf("Connection should not be up for unreachable port, reason: %s", reason))

			// Verify the observed connection state is indeed DOWN
			Expect(unreachableInstance.ObservedState.ServiceInfo.ConnectionFSMState).To(Equal(connectionfsm.OperationalStateDown))
		})
	})
})
