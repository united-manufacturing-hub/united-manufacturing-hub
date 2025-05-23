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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsmtest"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/backoff"
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
		It("should stop gracefully from Active state", func() {
			var err error

			// Phase 1: Establish Active state
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

			// Progress through startup phases to Active
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

			Expect(instance.GetCurrentFSMState()).To(Equal(protocolconverterfsm.OperationalStateActive))

			// Phase 2: Request graceful stop and verify shutdown sequence
			Expect(instance.SetDesiredFSMState(protocolconverterfsm.OperationalStateStopped)).To(Succeed())

			// Active -> Stopping
			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateActive,
				protocolconverterfsm.OperationalStateStopping, 10, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.StopCalled).To(BeTrue()) // stop should have been invoked

			// Simulate sub-components stopped, complete shutdown
			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateStopped)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStopping,
				protocolconverterfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// =========================================================================
	//  ERROR HANDLING AND REMOVAL
	// =========================================================================
	Context("Error Handling and Removal", func() {
		It("should perform graceful self-removal when encountering a permanent error in Active state", func() {
			var err error

			// Phase 1: Establish Active state
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

			// Reach Active state through full startup sequence
			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateStopped,
				protocolconverterfsm.OperationalStateIdle, 15, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToProtocolConverterState(mockService, componentName, protocolconverterfsm.OperationalStateActive)

			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				protocolconverterfsm.OperationalStateIdle,
				protocolconverterfsm.OperationalStateActive, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.GetCurrentFSMState()).To(Equal(protocolconverterfsm.OperationalStateActive))

			// Phase 2: Inject permanent error and verify self-removal behavior
			mockService.StatusError = fmt.Errorf("%s: simulated permanent error", backoff.PermanentFailureError)

			// Wait for FSM to react by changing its desired state to "stopped" (self-removal trigger)
			tick, err = fsmtest.WaitForProtocolConverterDesiredState(
				ctx, fsm.SystemSnapshot{Tick: tick}, instance, mockRegistry,
				protocolconverterfsm.OperationalStateStopped, 10)
			Expect(err).NotTo(HaveOccurred(), "ProtocolConverter should change desired state to stopped on permanent error")

			// Cleanup for subsequent tests
			mockService.StatusError = nil
		})

		It("should force-remove the ProtocolConverter when a permanent error occurs in a stopped state", func() {
			var err error

			// Phase 1: Establish stable stopped state
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

			Expect(instance.GetCurrentFSMState()).To(Equal(protocolconverterfsm.OperationalStateStopped))

			// Phase 2: Inject permanent error while stopped and verify force removal
			mockService.StatusError = fmt.Errorf("%s: simulated", backoff.PermanentFailureError)

			_, recErr, reconciled := fsmtest.ReconcileProtocolConverterUntilError(
				ctx, fsm.SystemSnapshot{Tick: tick}, instance, mockService, mockRegistry, componentName, 5)
			Expect(recErr).To(HaveOccurred())
			Expect(recErr.Error()).To(ContainSubstring(backoff.PermanentFailureError))
			Expect(reconciled).To(BeTrue())
			Expect(mockService.ForceRemoveCalled).To(BeTrue())

			// Cleanup for subsequent tests
			mockService.StatusError = nil
		})
	})

	// =========================================================================
	//  STATE STABILITY
	// =========================================================================
	Context("State Stability", func() {
		It("should remain stable in Idle state when no activity occurs", func() {
			var err error

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
	//  BACKOFF AND RETRY LOGIC
	// =========================================================================
	Context("Backoff and Retry Logic", func() {
		It("should respect backoff when transitioning fails", func() {
			var err error

			// Phase 1: Establish stopped state
			tick, err = fsmtest.TestProtocolConverterStateTransition(
				ctx, instance, mockService, mockRegistry, componentName,
				internalfsm.LifecycleStateToBeCreated,
				protocolconverterfsm.OperationalStateStopped, 10, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Test backoff behavior with transient error
			Expect(instance.SetDesiredFSMState(protocolconverterfsm.OperationalStateActive)).To(Succeed())
			mockService.StartError = fmt.Errorf("transient start error")

			// First reconcile should fail and set backoff
			snapshot := fsm.SystemSnapshot{Tick: tick}
			_, reconciled := instance.Reconcile(ctx, snapshot, mockRegistry)
			Expect(reconciled).To(BeTrue())                                                               // reconcile was attempted
			Expect(instance.GetCurrentFSMState()).To(Equal(protocolconverterfsm.OperationalStateStopped)) // should remain stopped

			// Clear the error but immediate retry should be blocked by backoff
			mockService.StartError = nil
			snapshot.Tick++
			_, reconciled = instance.Reconcile(ctx, snapshot, mockRegistry)
			Expect(reconciled).To(BeFalse()) // should be blocked by backoff
			Expect(instance.GetCurrentFSMState()).To(Equal(protocolconverterfsm.OperationalStateStopped))
		})
	})
})
