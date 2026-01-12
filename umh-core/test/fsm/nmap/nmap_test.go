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

package nmap_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	// Adjust these imports to your actual module paths
	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm" // for LifecycleStateToBeCreated, etc.
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsmtest"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/backoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/nmap"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	nmapsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/nmap"
	s6svc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

var _ = Describe("NmapInstance FSM", func() {
	var (
		instance    *nmap.NmapInstance
		mockService *nmapsvc.MockNmapService
		serviceName string
		ctx         context.Context
		tick        uint64

		mockServices *serviceregistry.Registry
	)

	BeforeEach(func() {
		ctx = context.Background()
		tick = 0

		// We create a default instance with a desired state of "stopped" initially
		// You can adapt as needed. Or you can do it inside each test scenario if you prefer.
		serviceName = "test-nmap"
		inst, ms, _ := fsmtest.SetupNmapInstance(serviceName, nmap.OperationalStateStopped)
		instance = inst
		mockService = ms
		instance.SetService(mockService)
		mockServices = serviceregistry.NewMockRegistry()
	})

	// -------------------------------------------------------------------------
	//  BASIC STATE TRANSITIONS
	// -------------------------------------------------------------------------
	Context("Basic State Transitions", func() {
		It("should transition from Stopped to Starting when activated", func() {
			// 1. Initially, the instance is "to_be_created"
			//    Let's do a short path: to_be_created => creating => stopped
			var err error

			// from "to_be_created" => "creating"
			tick, err = fsmtest.TestNmapStateTransition(
				ctx, instance, mockService, mockServices, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating,
				5, // attempts
				tick,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddNmapToS6ManagerCalled).To(BeTrue())

			// Next, mock the service creation success
			mockService.ServiceStates[serviceName] = &nmapsvc.ServiceInfo{
				S6FSMState: s6fsm.OperationalStateStopped,
				S6ObservedState: s6fsm.S6ObservedState{
					ServiceInfo: s6svc.ServiceInfo{Status: s6svc.ServiceDown, Uptime: 5},
				},
				NmapStatus: nmapsvc.NmapServiceInfo{
					IsRunning: true,
				},
			}
			mockService.ExistingServices[serviceName] = true

			// from "creating" => "stopped"
			tick, err = fsmtest.TestNmapStateTransition(
				ctx, instance, mockService, mockServices, serviceName,
				internalfsm.LifecycleStateCreating,
				nmap.OperationalStateStopped,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// 2. Now set desired state = Active => from "stopped" => "starting"
			Expect(instance.SetDesiredFSMState(nmap.OperationalStateOpen)).To(Succeed())
			Expect(instance.GetDesiredFSMState()).To(Equal(nmap.OperationalStateOpen))

			// Also set the mock flags for an initial start attempt
			mockService.SetServiceState(serviceName, nmapsvc.ServiceStateFlags{
				IsS6Running: false,
			})

			tick, err = fsmtest.TestNmapStateTransition(
				ctx, instance, mockService, mockServices, serviceName,
				nmap.OperationalStateStopped,
				nmap.OperationalStateStarting,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// check that StartNmap was called
			Expect(mockService.StartNmapCalled).To(BeTrue())
		})

		It("should transition to Idle when healthchecks pass", func() {
			// We'll do a multi-step approach:
			//  (1) to_be_created => creating => stopped
			//  (2) desired=active => starting => degraded

			var err error

			// Step 1: to_be_created => creating => stopped
			tick, err = fsmtest.TestNmapStateTransition(
				ctx, instance, mockService, mockServices, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			mockService.ServiceStates[serviceName] = &nmapsvc.ServiceInfo{
				S6FSMState: s6fsm.OperationalStateStopped}
			mockService.ExistingServices[serviceName] = true

			tick, err = fsmtest.TestNmapStateTransition(
				ctx, instance, mockService, mockServices, serviceName,
				internalfsm.LifecycleStateCreating,
				nmap.OperationalStateStopped,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Step 2: from stopped => starting => degraded
			Expect(instance.SetDesiredFSMState(nmap.OperationalStateOpen)).To(Succeed())

			// from stopped => starting
			tick, err = fsmtest.TestNmapStateTransition(
				ctx, instance, mockService, mockServices, serviceName,
				nmap.OperationalStateStopped,
				nmap.OperationalStateStarting,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// from starting => degraded
			mockService.SetServiceState(serviceName, nmapsvc.ServiceStateFlags{
				IsS6Running: true,
				S6FSMState:  s6fsm.OperationalStateRunning,
			})
			tick, err = fsmtest.TestNmapStateTransition(
				ctx, instance, mockService, mockServices, serviceName,
				nmap.OperationalStateStarting,
				nmap.OperationalStateDegraded,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetCurrentFSMState()).To(Equal(nmap.OperationalStateDegraded))
		})
	})

	// -------------------------------------------------------------------------
	//  RUNNING STATE TRANSITIONS
	// -------------------------------------------------------------------------
	Context("Running State Transitions", func() {
		It("should transition from Idle to Active when processing data", func() {
			// Let's get from to_be_created => idle using step-by-step approach
			var err error

			// Step 1: to_be_created => creating => stopped
			tick, err = fsmtest.TestNmapStateTransition(
				ctx, instance, mockService, mockServices, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			mockService.ServiceStates[serviceName] = &nmapsvc.ServiceInfo{
				S6FSMState: s6fsm.OperationalStateStopped}
			mockService.ExistingServices[serviceName] = true

			tick, err = fsmtest.TestNmapStateTransition(
				ctx, instance, mockService, mockServices, serviceName,
				internalfsm.LifecycleStateCreating,
				nmap.OperationalStateStopped,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Step 2: from stopped => starting => degraded
			Expect(instance.SetDesiredFSMState(nmap.OperationalStateOpen)).To(Succeed())

			// from stopped => starting
			tick, err = fsmtest.TestNmapStateTransition(
				ctx, instance, mockService, mockServices, serviceName,
				nmap.OperationalStateStopped,
				nmap.OperationalStateStarting,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// from starting => degraded
			mockService.SetServiceState(serviceName, nmapsvc.ServiceStateFlags{
				IsRunning:   true,
				IsS6Running: true,
				S6FSMState:  s6fsm.OperationalStateRunning,
			})
			tick, err = fsmtest.TestNmapStateTransition(
				ctx, instance, mockService, mockServices, serviceName,
				nmap.OperationalStateStarting,
				nmap.OperationalStateDegraded,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetCurrentFSMState()).To(Equal(nmap.OperationalStateDegraded))

			// Step 3: from Idle => Active when processing data
			mockService.SetServiceState(serviceName, nmapsvc.ServiceStateFlags{
				IsS6Running: true,
				IsRunning:   true,
				S6FSMState:  s6fsm.OperationalStateRunning,
				PortState:   string(nmap.PortStateOpen),
			})

			tick, err = fsmtest.TestNmapStateTransition(
				ctx, instance, mockService, mockServices, serviceName,
				nmap.OperationalStateDegraded,
				nmap.OperationalStateOpen,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should transition to Degraded when issues occur", func() {
			// Step 1: to_be_created => creating => stopped
			var err error
			tick, err = fsmtest.TestNmapStateTransition(
				ctx, instance, mockService, mockServices, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			mockService.ServiceStates[serviceName] = &nmapsvc.ServiceInfo{
				S6FSMState: s6fsm.OperationalStateStopped}
			mockService.ExistingServices[serviceName] = true

			tick, err = fsmtest.TestNmapStateTransition(
				ctx, instance, mockService, mockServices, serviceName,
				internalfsm.LifecycleStateCreating,
				nmap.OperationalStateStopped,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Step 2: from stopped => starting => degraded => open
			Expect(instance.SetDesiredFSMState(nmap.OperationalStateOpen)).To(Succeed())

			// from stopped => starting
			tick, err = fsmtest.TestNmapStateTransition(
				ctx, instance, mockService, mockServices, serviceName,
				nmap.OperationalStateStopped,
				nmap.OperationalStateStarting,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// from starting => degraded
			mockService.SetServiceState(serviceName, nmapsvc.ServiceStateFlags{
				IsS6Running: true,
				S6FSMState:  s6fsm.OperationalStateRunning,
				IsRunning:   true,
			})
			tick, err = fsmtest.TestNmapStateTransition(
				ctx, instance, mockService, mockServices, serviceName,
				nmap.OperationalStateStarting,
				nmap.OperationalStateDegraded,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// from idle => active
			mockService.SetServiceState(serviceName, nmapsvc.ServiceStateFlags{
				IsS6Running: true,
				IsRunning:   true,
				S6FSMState:  s6fsm.OperationalStateRunning,
				PortState:   string(nmap.PortStateOpen),
			})
			tick, err = fsmtest.TestNmapStateTransition(
				ctx, instance, mockService, mockServices, serviceName,
				nmap.OperationalStateDegraded,
				nmap.OperationalStateOpen,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Step 3: Then degrade => set flags => "degraded"
			mockService.SetServiceState(serviceName, nmapsvc.ServiceStateFlags{
				IsS6Running: true,
				IsRunning:   true,
				S6FSMState:  s6fsm.OperationalStateRunning,
				IsDegraded:  true,
				PortState:   "",
			})
			tick, err = fsmtest.TestNmapStateTransition(
				ctx, instance, mockService, mockServices, serviceName,
				nmap.OperationalStateOpen,
				nmap.OperationalStateDegraded,
				10,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should transition to Degraded when last scan is too old", func() {
			// Step 1: to_be_created => creating => stopped
			var err error
			tick, err = fsmtest.TestNmapStateTransition(
				ctx, instance, mockService, mockServices, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			mockService.ServiceStates[serviceName] = &nmapsvc.ServiceInfo{
				S6FSMState: s6fsm.OperationalStateStopped}
			mockService.ExistingServices[serviceName] = true

			tick, err = fsmtest.TestNmapStateTransition(
				ctx, instance, mockService, mockServices, serviceName,
				internalfsm.LifecycleStateCreating,
				nmap.OperationalStateStopped,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Step 2: from stopped => starting => degraded => open
			Expect(instance.SetDesiredFSMState(nmap.OperationalStateOpen)).To(Succeed())

			// from stopped => starting
			tick, err = fsmtest.TestNmapStateTransition(
				ctx, instance, mockService, mockServices, serviceName,
				nmap.OperationalStateStopped,
				nmap.OperationalStateStarting,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// from starting => degraded
			mockService.SetServiceState(serviceName, nmapsvc.ServiceStateFlags{
				IsS6Running: true,
				S6FSMState:  s6fsm.OperationalStateRunning,
				IsRunning:   true,
			})
			tick, err = fsmtest.TestNmapStateTransition(
				ctx, instance, mockService, mockServices, serviceName,
				nmap.OperationalStateStarting,
				nmap.OperationalStateDegraded,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// from degraded => open
			mockService.SetServiceState(serviceName, nmapsvc.ServiceStateFlags{
				IsS6Running: true,
				IsRunning:   true,
				S6FSMState:  s6fsm.OperationalStateRunning,
				PortState:   string(nmap.PortStateOpen),
			})
			tick, err = fsmtest.TestNmapStateTransition(
				ctx, instance, mockService, mockServices, serviceName,
				nmap.OperationalStateDegraded,
				nmap.OperationalStateOpen,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Step 3: Set an old timestamp that exceeds NmapScanTimeout
			// First, ensure we have a LastScan with an old timestamp
			if mockService.ServiceStates[serviceName].NmapStatus.LastScan == nil {
				mockService.ServiceStates[serviceName].NmapStatus.LastScan = &nmapsvc.NmapScanResult{}
			}
			// Set timestamp to 15 seconds ago (NmapScanTimeout is 10 seconds)
			oldTimestamp := time.Now().Add(-15 * time.Second)
			mockService.ServiceStates[serviceName].NmapStatus.LastScan.Timestamp = oldTimestamp
			mockService.ServiceStates[serviceName].NmapStatus.LastScan.PortResult.State = string(nmap.PortStateOpen)

			// The instance should transition from open => degraded due to timeout
			tick, err = fsmtest.TestNmapStateTransition(
				ctx, instance, mockService, mockServices, serviceName,
				nmap.OperationalStateOpen,
				nmap.OperationalStateDegraded,
				10,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// -------------------------------------------------------------------------
	//  STOPPING FLOW
	// -------------------------------------------------------------------------
	Context("Stopping Flow", func() {
		It("should stop gracefully from Active state", func() {
			// Step 1: to_be_created => creating => stopped
			var err error
			tick, err = fsmtest.TestNmapStateTransition(
				ctx, instance, mockService, mockServices, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			mockService.ServiceStates[serviceName] = &nmapsvc.ServiceInfo{S6FSMState: s6fsm.OperationalStateStopped}
			mockService.ExistingServices[serviceName] = true

			tick, err = fsmtest.TestNmapStateTransition(
				ctx, instance, mockService, mockServices, serviceName,
				internalfsm.LifecycleStateCreating,
				nmap.OperationalStateStopped,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Step 2: from stopped => starting => degraded => active
			Expect(instance.SetDesiredFSMState(nmap.OperationalStateOpen)).To(Succeed())

			// from stopped => starting
			tick, err = fsmtest.TestNmapStateTransition(
				ctx, instance, mockService, mockServices, serviceName,
				nmap.OperationalStateStopped,
				nmap.OperationalStateStarting,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// from starting => degraded
			mockService.SetServiceState(serviceName, nmapsvc.ServiceStateFlags{
				IsS6Running: true,
				S6FSMState:  s6fsm.OperationalStateRunning,
				IsRunning:   true,
			})
			tick, err = fsmtest.TestNmapStateTransition(
				ctx, instance, mockService, mockServices, serviceName,
				nmap.OperationalStateStarting,
				nmap.OperationalStateDegraded,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// from startingConfigLoading => open
			mockService.SetServiceState(serviceName, nmapsvc.ServiceStateFlags{
				IsS6Running: true,
				S6FSMState:  s6fsm.OperationalStateRunning,
				IsRunning:   true,
				PortState:   string(nmap.PortStateOpen),
			})
			tick, err = fsmtest.TestNmapStateTransition(
				ctx, instance, mockService, mockServices, serviceName,
				nmap.OperationalStateDegraded,
				nmap.OperationalStateOpen,
				10,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Step 3: from active => stopping => stopped
			Expect(instance.SetDesiredFSMState(nmap.OperationalStateStopped)).To(Succeed())

			// from active => stopping
			tick, err = fsmtest.TestNmapStateTransition(
				ctx, instance, mockService, mockServices, serviceName,
				nmap.OperationalStateOpen,
				nmap.OperationalStateStopping,
				10,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// simulate S6 stopping
			mockService.SetServiceState(serviceName, nmapsvc.ServiceStateFlags{
				IsS6Running: false,
				S6FSMState:  s6fsm.OperationalStateStopped,
			})

			tick, err = fsmtest.TestNmapStateTransition(
				ctx, instance, mockService, mockServices, serviceName,
				nmap.OperationalStateStopping,
				nmap.OperationalStateStopped,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should continue reconciling despite permanent errors in UpdateObservedState when in starting state", func() {
			// ARCHITECTURAL DECISION: We now continue reconciling even when UpdateObservedState
			// encounters permanent errors, regardless of whether we're in a terminal or non-terminal state.
			// This enables force-kill recovery scenarios where S6 services exist on filesystem
			// but FSM managers lose their in-memory mappings.
			//
			// The trade-off: We prioritize system recovery over immediate error handling.
			// Permanent errors in UpdateObservedState no longer trigger automatic FSM removal,
			// allowing the system to restore services after unexpected shutdowns/restarts.

			// 1) Get to stopped state using proper transitions
			var err error

			// First progress to creating state
			tick, err = fsmtest.TestNmapStateTransition(
				ctx, instance, mockService, mockServices, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Setup service in stopped state
			mockService.ServiceStates[serviceName] = &nmapsvc.ServiceInfo{S6FSMState: s6fsm.OperationalStateStopped}
			mockService.ExistingServices[serviceName] = true

			// Progress to stopped state
			tick, err = fsmtest.TestNmapStateTransition(
				ctx, instance, mockService, mockServices, serviceName,
				internalfsm.LifecycleStateCreating,
				nmap.OperationalStateStopped,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			err = instance.SetDesiredFSMState(nmap.OperationalStateOpen)
			Expect(err).NotTo(HaveOccurred())

			// Progress to starting state
			tick, err = fsmtest.TestNmapStateTransition(
				ctx, instance, mockService, mockServices, serviceName,
				nmap.OperationalStateStopped,
				nmap.OperationalStateStarting,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Create a permanent error that will be encountered during UpdateObservedState
			mockService.StatusError = fmt.Errorf("%s: test permanent error", backoff.PermanentFailureError)

			// Attempt single reconciliation - should continue despite the error
			snapshot := fsm.SystemSnapshot{Tick: tick}
			recErr, _ := instance.Reconcile(ctx, snapshot, mockServices)

			// With the new architecture, reconcile should NOT return an error for UpdateObservedState failures
			Expect(recErr).NotTo(HaveOccurred(), "Reconcile should continue despite UpdateObservedState errors")

			// FSM should maintain its desired state and continue operating
			Expect(instance.GetDesiredFSMState()).To(Equal(nmap.OperationalStateOpen))
			// The current state should remain in starting or may progress
			currentState := instance.GetCurrentFSMState()
			Expect(nmap.IsStartingState(currentState) || nmap.IsRunningState(currentState)).To(BeTrue(),
				"FSM should maintain starting state or progress despite UpdateObservedState errors")

			// Force removal should NOT be attempted since we continue reconciling
			Expect(mockService.ForceRemoveNmapCalled).To(BeFalse(), "Force removal should not be triggered for UpdateObservedState errors")

			// Clear error for other tests
			mockService.StatusError = nil
		})
	})

})

var _ = Describe("NmapInstance port‑state transitions", func() {
	var (
		ctx          context.Context
		instance     *nmap.NmapInstance
		mockService  *nmapsvc.MockNmapService
		mockServices *serviceregistry.Registry
		serviceName  string
		tick         uint64
	)

	// Bring the instance to Degraded once for every test‑row so each scenario
	// starts at the same baseline.
	BeforeEach(func() {
		ctx = context.Background()
		tick = 0
		serviceName = "test-nmap"

		instance, mockService, _ = fsmtest.SetupNmapInstance(serviceName, nmap.OperationalStateStopped)
		mockServices = serviceregistry.NewMockRegistry()
		// to_be_created → creating
		tick, _ = fsmtest.TestNmapStateTransition(
			ctx, instance, mockService, mockServices, serviceName,
			internalfsm.LifecycleStateToBeCreated,
			internalfsm.LifecycleStateCreating,
			5, tick,
		)
		// creating → stopped
		mockService.ServiceStates[serviceName] = &nmapsvc.ServiceInfo{S6FSMState: s6fsm.OperationalStateStopped}
		mockService.ExistingServices[serviceName] = true
		tick, _ = fsmtest.TestNmapStateTransition(
			ctx, instance, mockService, mockServices, serviceName,
			internalfsm.LifecycleStateCreating,
			nmap.OperationalStateStopped,
			5, tick,
		)

		// let the instance start scanning
		Expect(instance.SetDesiredFSMState(nmap.OperationalStateOpen)).To(Succeed())
		// stopped → starting
		tick, _ = fsmtest.TestNmapStateTransition(
			ctx, instance, mockService, mockServices, serviceName,
			nmap.OperationalStateStopped,
			nmap.OperationalStateStarting,
			5, tick,
		)
		// starting → degraded (service up, no port result yet)
		mockService.SetServiceState(serviceName, nmapsvc.ServiceStateFlags{
			IsS6Running: true,
			IsRunning:   true,
			S6FSMState:  s6fsm.OperationalStateRunning,
		})
		tick, _ = fsmtest.TestNmapStateTransition(
			ctx, instance, mockService, mockServices, serviceName,
			nmap.OperationalStateStarting,
			nmap.OperationalStateDegraded,
			5, tick,
		)
		Expect(instance.GetCurrentFSMState()).To(Equal(nmap.OperationalStateDegraded))
	})
	AfterEach(func() {
		Expect(instance.SetDesiredFSMState(nmap.OperationalStateStopped)).To(Succeed())

		mockService.SetServiceState(serviceName, nmapsvc.ServiceStateFlags{
			IsS6Running: false,
			S6FSMState:  s6fsm.OperationalStateStopped,
		})

		cur := instance.GetCurrentFSMState()
		if cur != nmap.OperationalStateStopping && cur != nmap.OperationalStateStopped {
			var err error
			tick, err = fsmtest.TestNmapStateTransition(
				ctx, instance, mockService, mockServices, serviceName,
				cur, nmap.OperationalStateStopping,
				5, tick,
			)
			Expect(err).NotTo(HaveOccurred())
		}

		if instance.GetCurrentFSMState() != nmap.OperationalStateStopped {
			var err error
			tick, err = fsmtest.TestNmapStateTransition(
				ctx, instance, mockService, mockServices, serviceName,
				nmap.OperationalStateStopping, nmap.OperationalStateStopped,
				5, tick,
			)
			Expect(err).NotTo(HaveOccurred())
		}

		// sanity check
		Expect(instance.GetCurrentFSMState()).To(Equal(nmap.OperationalStateStopped))
	})

	DescribeTable("follows the degraded → from → to matrix", func(fromState, toState, fromPort, toPort string, _ uint64) {
		// Degraded → fromState
		mockService.SetServiceState(serviceName, nmapsvc.ServiceStateFlags{
			IsS6Running: true,
			IsRunning:   true,
			S6FSMState:  s6fsm.OperationalStateRunning,
			PortState:   fromPort,
		})
		var err error
		tick, err = fsmtest.TestNmapStateTransition(ctx, instance, mockService, mockServices, serviceName,
			nmap.OperationalStateDegraded, fromState, 10, tick)
		Expect(err).NotTo(HaveOccurred())

		// fromState → toState
		mockService.SetServiceState(serviceName, nmapsvc.ServiceStateFlags{
			IsS6Running: true,
			IsRunning:   true,
			S6FSMState:  s6fsm.OperationalStateRunning,
			PortState:   toPort,
		})
		tick, err = fsmtest.TestNmapStateTransition(ctx, instance, mockService, mockServices, serviceName,
			fromState, toState, 10, tick)
		Expect(err).NotTo(HaveOccurred())
		Expect(instance.GetCurrentFSMState()).To(Equal(toState))
	},
		// -------------
		//  open → …
		Entry("open → closed", nmap.OperationalStateOpen, nmap.OperationalStateClosed, "open", "closed", uint64(24)),
		Entry("open → filtered", nmap.OperationalStateOpen, nmap.OperationalStateFiltered, "open", "filtered", uint64(29)),
		Entry("open → unfiltered", nmap.OperationalStateOpen, nmap.OperationalStateUnfiltered, "open", "unfiltered", uint64(31)),
		Entry("open → open_filtered", nmap.OperationalStateOpen, nmap.OperationalStateOpenFiltered, "open", "open|filtered", uint64(36)),
		Entry("open → closed_filtered", nmap.OperationalStateOpen, nmap.OperationalStateClosedFiltered, "open", "closed|filtered", uint64(41)),

		//  filtered → …
		Entry("filtered → open", nmap.OperationalStateFiltered, nmap.OperationalStateOpen, "filtered", "open", uint64(46)),
		Entry("filtered → closed", nmap.OperationalStateFiltered, nmap.OperationalStateClosed, "filtered", "closed", uint64(51)),
		Entry("filtered → unfiltered", nmap.OperationalStateFiltered, nmap.OperationalStateUnfiltered, "filtered", "unfiltered", uint64(56)),
		Entry("filtered → open_filtered", nmap.OperationalStateFiltered, nmap.OperationalStateOpenFiltered, "filtered", "open|filtered", uint64(61)),
		Entry("filtered → closed_filtered", nmap.OperationalStateFiltered, nmap.OperationalStateClosedFiltered, "filtered", "closed|filtered", uint64(66)),

		//  closed → …
		Entry("closed → open", nmap.OperationalStateClosed, nmap.OperationalStateOpen, "closed", "open", uint64(71)),
		Entry("closed → filtered", nmap.OperationalStateClosed, nmap.OperationalStateFiltered, "closed", "filtered", uint64(76)),
		Entry("closed → unfiltered", nmap.OperationalStateClosed, nmap.OperationalStateUnfiltered, "closed", "unfiltered", uint64(81)),
		Entry("closed → open_filtered", nmap.OperationalStateClosed, nmap.OperationalStateOpenFiltered, "closed", "open|filtered", uint64(86)),
		Entry("closed → closed_filtered", nmap.OperationalStateClosed, nmap.OperationalStateClosedFiltered, "closed", "closed|filtered", uint64(91)),

		//  unfiltered → …
		Entry("unfiltered → open", nmap.OperationalStateUnfiltered, nmap.OperationalStateOpen, "unfiltered", "open", uint64(96)),
		Entry("unfiltered → filtered", nmap.OperationalStateUnfiltered, nmap.OperationalStateFiltered, "unfiltered", "filtered", uint64(101)),
		Entry("unfiltered → closed", nmap.OperationalStateUnfiltered, nmap.OperationalStateClosed, "unfiltered", "closed", uint64(106)),
		Entry("unfiltered → open_filtered", nmap.OperationalStateUnfiltered, nmap.OperationalStateOpenFiltered, "unfiltered", "open|filtered", uint64(111)),
		Entry("unfiltered → closed_filtered", nmap.OperationalStateUnfiltered, nmap.OperationalStateClosedFiltered, "unfiltered", "closed|filtered", uint64(116)),

		//  open_filtered → …
		Entry("open_filtered → open", nmap.OperationalStateOpenFiltered, nmap.OperationalStateOpen, "open|filtered", "open", uint64(121)),
		Entry("open_filtered → filtered", nmap.OperationalStateOpenFiltered, nmap.OperationalStateFiltered, "open|filtered", "filtered", uint64(126)),
		Entry("open_filtered → closed", nmap.OperationalStateOpenFiltered, nmap.OperationalStateClosed, "open|filtered", "closed", uint64(131)),
		Entry("open_filtered → unfiltered", nmap.OperationalStateOpenFiltered, nmap.OperationalStateUnfiltered, "open|filtered", "unfiltered", uint64(136)),
		Entry("open_filtered → closed_filtered", nmap.OperationalStateOpenFiltered, nmap.OperationalStateClosedFiltered, "open|filtered", "closed|filtered", uint64(141)),

		//  closed_filtered → …
		Entry("closed_filtered → open", nmap.OperationalStateClosedFiltered, nmap.OperationalStateOpen, "closed|filtered", "open", uint64(146)),
		Entry("closed_filtered → filtered", nmap.OperationalStateClosedFiltered, nmap.OperationalStateFiltered, "closed|filtered", "filtered", uint64(151)),
		Entry("closed_filtered → closed", nmap.OperationalStateClosedFiltered, nmap.OperationalStateClosed, "closed|filtered", "closed", uint64(156)),
		Entry("closed_filtered → unfiltered", nmap.OperationalStateClosedFiltered, nmap.OperationalStateUnfiltered, "closed|filtered", "unfiltered", uint64(161)),
		Entry("closed_filtered → open_filtered", nmap.OperationalStateClosedFiltered, nmap.OperationalStateOpenFiltered, "closed|filtered", "open|filtered", uint64(166)),
	)
})
