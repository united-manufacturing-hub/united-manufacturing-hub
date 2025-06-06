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

//go:build test
// +build test

package protocolconverter_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsmtest"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	protocolconverterfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/protocolconverter"
	protocolconvertersvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/protocolconverter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

var _ = Describe("ProtocolConverter FSM", func() {
	var (
		instance      *protocolconverterfsm.ProtocolConverterInstance
		mockService   *protocolconvertersvc.MockProtocolConverterService
		componentName string
		ctx           context.Context
		tick          uint64
		mockRegistry  *serviceregistry.Registry
		startTime     time.Time
	)

	BeforeEach(func() {
		componentName = "test-protocol-converter"
		ctx = context.Background()
		tick = 0
		startTime = time.Now()

		// Initialize a ProtocolConverter instance with default desired state (Stopped to begin with)
		instance, mockService, _ = fsmtest.SetupProtocolConverterInstance(componentName, protocolconverterfsm.OperationalStateStopped)
		mockRegistry = serviceregistry.NewMockRegistry()
	})

	// =========================================================================
	//  BASIC STATE TRANSITIONS
	// =========================================================================
	Context("Basic State Transitions", func() {
		It("should transition from Stopped to StartingConnection when activated", func() {
			var err error

			// Phase 1: Initial lifecycle setup (to_be_created -> creating -> stopped)
			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Simulate successful creation: service now exists in stopped state
			mockService.ExistingComponents[componentName] = true
			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStopped)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateCreating,
				protocolconverterfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddToManagerCalled).To(BeTrue()) // added to manager on creation

			// Phase 2: Activation sequence (set desired state to Active)
			Expect(instance.SetDesiredFSMState(protocolconverterfsm.OperationalStateActive)).To(Succeed())
			Expect(instance.GetDesiredFSMState()).To(Equal(protocolconverterfsm.OperationalStateActive))

			// Execute transition: Stopped -> StartingConnection
			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStopped,
				protocolconverterfsm.OperationalStateStartingConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.StartCalled).To(BeTrue()) // ensure start initiated
		})

		It("should transition from StartingConnection to StartingRedpanda when connection becomes active", func() {
			var err error

			// Phase 1: Setup to stopped state
			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[componentName] = true
			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStopped)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateCreating,
				protocolconverterfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Activate and reach StartingConnection
			Expect(instance.SetDesiredFSMState(protocolconverterfsm.OperationalStateActive)).To(Succeed())

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStopped,
				protocolconverterfsm.OperationalStateStartingConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 3: Simulate connection established, transition to StartingRedpanda
			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStartingRedpanda)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingConnection,
				protocolconverterfsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should transition from StartingRedpanda to StartingDFC when Redpanda is ready", func() {
			var err error

			// Phase 1: Setup to stopped state
			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[componentName] = true
			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStopped)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateCreating,
				protocolconverterfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Progress through connection startup to StartingRedpanda
			Expect(instance.SetDesiredFSMState(protocolconverterfsm.OperationalStateActive)).To(Succeed())

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStopped,
				protocolconverterfsm.OperationalStateStartingConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStartingRedpanda)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingConnection,
				protocolconverterfsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 3: Simulate Redpanda ready, transition to StartingDFC
			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStartingDFC)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingRedpanda,
				protocolconverterfsm.OperationalStateStartingDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should transition from StartingDFC to Idle when DFC is up and running", func() {
			var err error

			// Phase 1: Setup through StartingDFC state
			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[componentName] = true
			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStopped)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateCreating,
				protocolconverterfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(protocolconverterfsm.OperationalStateActive)).To(Succeed())

			// Progress through startup sequence
			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStopped,
				protocolconverterfsm.OperationalStateStartingConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStartingRedpanda)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingConnection,
				protocolconverterfsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStartingDFC)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingRedpanda,
				protocolconverterfsm.OperationalStateStartingDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Simulate DFC successfully started, transition to Idle
			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateIdle)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingDFC,
				protocolconverterfsm.OperationalStateIdle, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should transition from Idle to Active when processing data", func() {
			var err error

			// Phase 1: Reach Idle state through full startup sequence
			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[componentName] = true
			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStopped)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateCreating,
				protocolconverterfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(protocolconverterfsm.OperationalStateActive)).To(Succeed())

			// Progress through starting states to Idle
			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStopped,
				protocolconverterfsm.OperationalStateStartingConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStartingRedpanda)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingConnection,
				protocolconverterfsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStartingDFC)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingRedpanda,
				protocolconverterfsm.OperationalStateStartingDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateIdle)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingDFC,
				protocolconverterfsm.OperationalStateIdle, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Simulate data flow starting (Idle -> Active)
			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateActive)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateIdle,
				protocolconverterfsm.OperationalStateActive, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should transition from Active back to Idle when processing stops", func() {
			var err error

			// Phase 1: Reach Active state through full sequence
			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[componentName] = true
			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStopped)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateCreating,
				protocolconverterfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(protocolconverterfsm.OperationalStateActive)).To(Succeed())

			// Progress through startup sequence to Active
			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStopped,
				protocolconverterfsm.OperationalStateStartingConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStartingRedpanda)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingConnection,
				protocolconverterfsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStartingDFC)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingRedpanda,
				protocolconverterfsm.OperationalStateStartingDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateIdle)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingDFC,
				protocolconverterfsm.OperationalStateIdle, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateActive)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateIdle,
				protocolconverterfsm.OperationalStateActive, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Simulate stopping of data flow (Active -> Idle)
			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateIdle)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateActive,
				protocolconverterfsm.OperationalStateIdle, 5, tick, startTime)
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
			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[componentName] = true
			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStopped)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateCreating,
				protocolconverterfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(protocolconverterfsm.OperationalStateActive)).To(Succeed())

			// Progress through startup to Idle
			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStopped,
				protocolconverterfsm.OperationalStateStartingConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStartingRedpanda)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingConnection,
				protocolconverterfsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStartingDFC)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingRedpanda,
				protocolconverterfsm.OperationalStateStartingDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateIdle)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingDFC,
				protocolconverterfsm.OperationalStateIdle, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Simulate connection failure and recovery
			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateDegradedConnection)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateIdle,
				protocolconverterfsm.OperationalStateDegradedConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Restore connection and verify recovery
			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateIdle)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateDegradedConnection,
				protocolconverterfsm.OperationalStateIdle, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should transition to DegradedRedpanda when Redpanda becomes unavailable and recover when restored", func() {
			var err error

			// Phase 1: Establish Idle state
			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[componentName] = true
			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStopped)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateCreating,
				protocolconverterfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(protocolconverterfsm.OperationalStateActive)).To(Succeed())

			// Progress through startup to Idle
			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStopped,
				protocolconverterfsm.OperationalStateStartingConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStartingRedpanda)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingConnection,
				protocolconverterfsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStartingDFC)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingRedpanda,
				protocolconverterfsm.OperationalStateStartingDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateIdle)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingDFC,
				protocolconverterfsm.OperationalStateIdle, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Simulate Redpanda failure and recovery
			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateDegradedRedpanda)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateIdle,
				protocolconverterfsm.OperationalStateDegradedRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Restore Redpanda and verify recovery
			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateIdle)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateDegradedRedpanda,
				protocolconverterfsm.OperationalStateIdle, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should transition to DegradedDFC when the DFC goes down and recover when it comes back", func() {
			var err error

			// Phase 1: Establish Idle state
			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[componentName] = true
			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStopped)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateCreating,
				protocolconverterfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(protocolconverterfsm.OperationalStateActive)).To(Succeed())

			// Progress through startup to Idle
			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStopped,
				protocolconverterfsm.OperationalStateStartingConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStartingRedpanda)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingConnection,
				protocolconverterfsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStartingDFC)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingRedpanda,
				protocolconverterfsm.OperationalStateStartingDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateIdle)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingDFC,
				protocolconverterfsm.OperationalStateIdle, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Simulate DFC failure and recovery
			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateDegradedDFC)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateIdle,
				protocolconverterfsm.OperationalStateDegradedDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Restore DFC and verify recovery
			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateIdle)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateDegradedDFC,
				protocolconverterfsm.OperationalStateIdle, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should transition to DegradedOther for inconsistent state and recover when resolved", func() {
			var err error

			// Phase 1: Establish Idle state
			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[componentName] = true
			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStopped)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateCreating,
				protocolconverterfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(protocolconverterfsm.OperationalStateActive)).To(Succeed())

			// Progress through startup to Idle
			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStopped,
				protocolconverterfsm.OperationalStateStartingConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStartingRedpanda)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingConnection,
				protocolconverterfsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStartingDFC)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingRedpanda,
				protocolconverterfsm.OperationalStateStartingDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateIdle)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingDFC,
				protocolconverterfsm.OperationalStateIdle, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Force inconsistent state scenario and recovery
			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateDegradedOther)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateIdle,
				protocolconverterfsm.OperationalStateDegradedOther, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Resolve inconsistency and verify recovery
			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateIdle)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateDegradedOther,
				protocolconverterfsm.OperationalStateIdle, 5, tick, startTime)
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
			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[componentName] = true
			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStopped)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateCreating,
				protocolconverterfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(protocolconverterfsm.OperationalStateActive)).To(Succeed())

			// Progress through startup sequence to StartingDFC
			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStopped,
				protocolconverterfsm.OperationalStateStartingConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStartingRedpanda)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingConnection,
				protocolconverterfsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStartingDFC)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingRedpanda,
				protocolconverterfsm.OperationalStateStartingDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Simulate DFC startup failure
			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStartingFailedDFC)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingDFC,
				protocolconverterfsm.OperationalStateStartingFailedDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should transition to StartingFailedDFCMissing when the DFC is not present", func() {
			// Setup instance with missing DFC
			instance, mockService, _ = fsmtest.SetupProtocolConverterInstanceWithMissingDfc(componentName, protocolconverterfsm.OperationalStateStopped)

			var err error

			// Phase 1: Progress to StartingDFC state
			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[componentName] = true
			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStopped)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateCreating,
				protocolconverterfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(protocolconverterfsm.OperationalStateActive)).To(Succeed())

			// Progress through startup sequence to StartingDFC
			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStopped,
				protocolconverterfsm.OperationalStateStartingConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStartingRedpanda)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingConnection,
				protocolconverterfsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStartingDFC)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingRedpanda,
				protocolconverterfsm.OperationalStateStartingDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Simulate DFC component missing entirely
			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStartingFailedDFCMissing)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingDFC,
				protocolconverterfsm.OperationalStateStartingFailedDFCMissing, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// =========================================================================
	//  STOPPING FLOW
	// =========================================================================
	Context("Stopping Flow", func() {
		// Helper functions to setup different states
		var setupActive = func() {
			var err error
			// Reset for clean state
			instance, mockService, _ = fsmtest.SetupProtocolConverterInstance(componentName, protocolconverterfsm.OperationalStateStopped)
			tick = 0

			// Progress to Active state
			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[componentName] = true
			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStopped)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateCreating,
				protocolconverterfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(protocolconverterfsm.OperationalStateActive)).To(Succeed())

			// Progress through startup to Active
			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStopped,
				protocolconverterfsm.OperationalStateStartingConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStartingRedpanda)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingConnection,
				protocolconverterfsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStartingDFC)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingRedpanda,
				protocolconverterfsm.OperationalStateStartingDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateIdle)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingDFC,
				protocolconverterfsm.OperationalStateIdle, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateActive)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateIdle,
				protocolconverterfsm.OperationalStateActive, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		}

		var setupIdle = func() {
			var err error
			// Reset for clean state
			instance, mockService, _ = fsmtest.SetupProtocolConverterInstance(componentName, protocolconverterfsm.OperationalStateStopped)
			tick = 0

			// Progress to Idle state (similar to setupActive but stop at Idle)
			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[componentName] = true
			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStopped)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateCreating,
				protocolconverterfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(protocolconverterfsm.OperationalStateActive)).To(Succeed())

			// Progress through startup to Idle
			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStopped,
				protocolconverterfsm.OperationalStateStartingConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStartingRedpanda)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingConnection,
				protocolconverterfsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStartingDFC)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingRedpanda,
				protocolconverterfsm.OperationalStateStartingDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateIdle)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingDFC,
				protocolconverterfsm.OperationalStateIdle, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		}

		var setupStartingConnection = func() {
			var err error
			// Reset for clean state
			instance, mockService, _ = fsmtest.SetupProtocolConverterInstance(componentName, protocolconverterfsm.OperationalStateStopped)
			tick = 0

			// Progress to StartingConnection
			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[componentName] = true
			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStopped)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateCreating,
				protocolconverterfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(protocolconverterfsm.OperationalStateActive)).To(Succeed())

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStopped,
				protocolconverterfsm.OperationalStateStartingConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		}

		var setupStartingRedpanda = func() {
			setupStartingConnection()
			var err error

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStartingRedpanda)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingConnection,
				protocolconverterfsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		}

		var setupStartingDFC = func() {
			setupStartingRedpanda()
			var err error

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStartingDFC)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingRedpanda,
				protocolconverterfsm.OperationalStateStartingDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		}

		var setupStartingFailedDFCMissing = func() {
			var err error
			// Setup instance with missing DFC
			instance, mockService, _ = fsmtest.SetupProtocolConverterInstanceWithMissingDfc(componentName, protocolconverterfsm.OperationalStateStopped)
			tick = 0

			// Progress to StartingFailedDFCMissing
			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[componentName] = true
			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStopped)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateCreating,
				protocolconverterfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(protocolconverterfsm.OperationalStateActive)).To(Succeed())

			// Progress through startup sequence to StartingFailedDFCMissing
			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStopped,
				protocolconverterfsm.OperationalStateStartingConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStartingRedpanda)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingConnection,
				protocolconverterfsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStartingDFC)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingRedpanda,
				protocolconverterfsm.OperationalStateStartingDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStartingFailedDFCMissing)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStartingDFC,
				protocolconverterfsm.OperationalStateStartingFailedDFCMissing, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		}

		var setupDegradedConnection = func() {
			setupIdle()
			var err error

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateDegradedConnection)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateIdle,
				protocolconverterfsm.OperationalStateDegradedConnection, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		}

		var setupDegradedRedpanda = func() {
			setupIdle()
			var err error

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateDegradedRedpanda)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateIdle,
				protocolconverterfsm.OperationalStateDegradedRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		}

		var setupDegradedDFC = func() {
			setupIdle()
			var err error

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateDegradedDFC)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateIdle,
				protocolconverterfsm.OperationalStateDegradedDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		}

		var setupDegradedOther = func() {
			setupIdle()
			var err error

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateDegradedOther)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateIdle,
				protocolconverterfsm.OperationalStateDegradedOther, 5, tick, startTime)
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
				Expect(instance.SetDesiredFSMState(protocolconverterfsm.OperationalStateStopped)).To(Succeed())

				// Phase 3: Verify transition to stopping
				tick, err = fsmtest.TestProtocolConverterStateTransition(
					ctx, instance, mockService, mockRegistry, componentName,
					fromState,
					protocolconverterfsm.OperationalStateStopping, 10, tick, startTime)
				Expect(err).NotTo(HaveOccurred())
				Expect(mockService.StopCalled).To(BeTrue())

				// Phase 4: Complete shutdown
				fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStopped)

				tick, err = fsmtest.TestProtocolConverterStateTransition(
					ctx, instance, mockService, mockRegistry, componentName,
					protocolconverterfsm.OperationalStateStopping,
					protocolconverterfsm.OperationalStateStopped, 5, tick, startTime)
				Expect(err).NotTo(HaveOccurred())
			},

			// Starting states
			Entry("from starting_connection", protocolconverterfsm.OperationalStateStartingConnection, setupStartingConnection),
			Entry("from starting_redpanda", protocolconverterfsm.OperationalStateStartingRedpanda, setupStartingRedpanda),
			Entry("from starting_dfc", protocolconverterfsm.OperationalStateStartingDFC, setupStartingDFC),
			// Note: starting_failed_dfc is excluded because DFCs cannot reach this state in practice yet.
			// The business logic supports stopping from this state, but test setup is complex and
			// the state is not reachable through normal DFC lifecycle. Will add test when DFC
			// failure scenarios are properly implemented.
			// Entry("from starting_failed_dfc", protocolconverterfsm.OperationalStateStartingFailedDFC, setupStartingFailedDFC),
			Entry("from starting_failed_dfc_missing", protocolconverterfsm.OperationalStateStartingFailedDFCMissing, setupStartingFailedDFCMissing),

			// Running states
			Entry("from idle", protocolconverterfsm.OperationalStateIdle, setupIdle),
			Entry("from active", protocolconverterfsm.OperationalStateActive, setupActive),

			// Degraded states
			Entry("from degraded_connection", protocolconverterfsm.OperationalStateDegradedConnection, setupDegradedConnection),
			Entry("from degraded_redpanda", protocolconverterfsm.OperationalStateDegradedRedpanda, setupDegradedRedpanda),
			Entry("from degraded_dfc", protocolconverterfsm.OperationalStateDegradedDFC, setupDegradedDFC),
			Entry("from degraded_other", protocolconverterfsm.OperationalStateDegradedOther, setupDegradedOther),
		)
	})

	// =========================================================================
	//  STATE STABILITY
	// =========================================================================
	Context("State Stability", func() {
		It("should remain stable in Idle state when no activity occurs", func() {
			var err error

			// Set to active from stopped state
			Expect(instance.SetDesiredFSMState(protocolconverterfsm.OperationalStateActive)).To(Succeed())

			// Phase 1: Establish Idle state
			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				protocolconverterfsm.OperationalStateIdle, 15, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Verify stability over multiple reconcile cycles
			tick, err = fsmtest.VerifyProtocolConverterStableState(
				ctx, fsm.SystemSnapshot{Tick: tick}, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateIdle, 5)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should remain stable in Active state when processing continues", func() {
			var err error

			// Set to active from stopped state
			Expect(instance.SetDesiredFSMState(protocolconverterfsm.OperationalStateActive)).To(Succeed())

			// Phase 1: Establish Active state
			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				protocolconverterfsm.OperationalStateIdle, 15, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateActive)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateIdle,
				protocolconverterfsm.OperationalStateActive, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Verify stability over multiple reconcile cycles
			tick, err = fsmtest.VerifyProtocolConverterStableState(
				ctx, fsm.SystemSnapshot{Tick: tick}, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateActive, 5)
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
			invalidInstance, invalidMockService, _ := fsmtest.SetupProtocolConverterInstanceWithInvalidPort(componentName, protocolconverterfsm.OperationalStateStopped, "invalid-port")

			// Attempt reconciliation several times
			finalTick, err, _ := fsmtest.ReconcileProtocolConverterUntilError(
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
			if pcObservedState, ok := observedState.(protocolconverterfsm.ProtocolConverterObservedState); ok {
				Expect(pcObservedState.ServiceInfo.StatusReason).To(ContainSubstring("config error"))
				Expect(pcObservedState.ServiceInfo.StatusReason).To(ContainSubstring("invalid syntax"))
			} else {
				Fail("Could not cast observed state to ProtocolConverterObservedState")
			}
		})
	})
})
