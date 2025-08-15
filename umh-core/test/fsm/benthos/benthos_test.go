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

package benthos_test

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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthosfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/portmanager"
	benthossvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
	benthos_monitor "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
	s6svc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

var _ = Describe("BenthosInstance FSM", func() {
	var (
		instance    *benthosfsm.BenthosInstance
		mockService *benthossvc.MockBenthosService
		serviceName string
		ctx         context.Context
		tick        uint64

		mockSvcRegistry *serviceregistry.Registry
	)

	BeforeEach(func() {
		ctx = context.Background()
		tick = 0

		// We create a default instance with a desired state of "stopped" initially
		// You can adapt as needed. Or you can do it inside each test scenario if you prefer.
		serviceName = "test-benthos"
		inst, ms, _ := fsmtest.SetupBenthosInstance(serviceName, benthosfsm.OperationalStateStopped)
		instance = inst
		mockService = ms
		mockSvcRegistry = serviceregistry.NewMockRegistry()
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
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating,
				5, // attempts
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddBenthosToS6ManagerCalled).To(BeTrue())

			// Next, mock the service creation success
			mockService.ServiceStates[serviceName] = &benthossvc.ServiceInfo{
				S6FSMState: s6fsm.OperationalStateStopped,
				S6ObservedState: s6fsm.S6ObservedState{
					ServiceInfo: s6svc.ServiceInfo{Status: s6svc.ServiceDown, Uptime: 5},
				},
				BenthosStatus: benthossvc.BenthosStatus{
					BenthosMetrics: benthos_monitor.BenthosMetrics{
						MetricsState: &benthos_monitor.BenthosMetricsState{
							IsActive: false,
						},
					},
				},
			}
			mockService.ExistingServices[serviceName] = true

			// from "creating" => "stopped"
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				internalfsm.LifecycleStateCreating,
				benthosfsm.OperationalStateStopped,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// 2. Now set desired state = Active => from "stopped" => "starting"
			Expect(instance.SetDesiredFSMState(benthosfsm.OperationalStateActive)).To(Succeed())
			Expect(instance.GetDesiredFSMState()).To(Equal(benthosfsm.OperationalStateActive))

			// Also set the mock flags for an initial start attempt
			mockService.SetServiceState(serviceName, benthossvc.ServiceStateFlags{
				IsS6Running: false,
			})

			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateStopped,
				benthosfsm.OperationalStateStarting,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// check that StartBenthos was called
			Expect(mockService.StartBenthosCalled).To(BeTrue())
		})

		It("should transition from Starting to ConfigLoading when S6 is running", func() {
			// Suppose we do a short path again:
			// (1) to_be_created => creating => stopped
			// (2) desired=active => starting => configLoading
			var err error

			// Step 1: from to_be_created => creating => stopped
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating,
				3,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			mockService.ServiceStates[serviceName] = &benthossvc.ServiceInfo{
				S6FSMState: s6fsm.OperationalStateStopped,
			}
			mockService.ExistingServices[serviceName] = true

			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				internalfsm.LifecycleStateCreating,
				benthosfsm.OperationalStateStopped,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// Step 2: from stopped => starting => configLoading
			Expect(instance.SetDesiredFSMState(benthosfsm.OperationalStateActive)).To(Succeed())
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateStopped,
				benthosfsm.OperationalStateStarting,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// Now set the S6 running so we go to config loading
			mockService.SetServiceState(serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:          true,
				S6FSMState:           s6fsm.OperationalStateRunning,
				IsConfigLoaded:       true,
				IsHealthchecksPassed: true,
			})

			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateStarting,
				benthosfsm.OperationalStateStartingConfigLoading,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should transition to Idle when healthchecks pass", func() {
			// We'll do a multi-step approach:
			//  (1) to_be_created => creating => stopped
			//  (2) desired=active => starting => configLoading => waiting => idle
			// We'll just skip directly to configLoading by setting mock flags.

			var err error

			// Step 1: to_be_created => creating => stopped
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			mockService.ServiceStates[serviceName] = &benthossvc.ServiceInfo{S6FSMState: s6fsm.OperationalStateStopped}
			mockService.ExistingServices[serviceName] = true

			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				internalfsm.LifecycleStateCreating,
				benthosfsm.OperationalStateStopped,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// Step 2: from stopped => starting => configLoading => idle
			Expect(instance.SetDesiredFSMState(benthosfsm.OperationalStateActive)).To(Succeed())

			// from stopped => starting
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateStopped,
				benthosfsm.OperationalStateStarting,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// from starting => startingConfigLoading
			mockService.SetServiceState(serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:          true,
				S6FSMState:           s6fsm.OperationalStateRunning,
				IsConfigLoaded:       true,
				IsHealthchecksPassed: true,
			})
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateStarting,
				benthosfsm.OperationalStateStartingConfigLoading,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// Finally from startingConfigLoading => idle
			// set flags for success
			mockService.SetServiceState(serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				S6FSMState:             s6fsm.OperationalStateRunning,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   true,
				IsRunningWithoutErrors: true,
			})
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateStartingConfigLoading,
				benthosfsm.OperationalStateIdle,
				60, // need to wait for at least 60 ticks (constants.benthosStartingConfigLoadingDuration, which is currently 5 seconds, and assuming a DefaultTickDuration of 100ms + 10 ticks for the other state transitions)
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetCurrentFSMState()).To(Equal(benthosfsm.OperationalStateIdle))
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
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			mockService.ServiceStates[serviceName] = &benthossvc.ServiceInfo{S6FSMState: s6fsm.OperationalStateStopped}
			mockService.ExistingServices[serviceName] = true

			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				internalfsm.LifecycleStateCreating,
				benthosfsm.OperationalStateStopped,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// Step 2: from stopped => starting => configLoading => waiting => idle
			Expect(instance.SetDesiredFSMState(benthosfsm.OperationalStateActive)).To(Succeed())

			// from stopped => starting
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateStopped,
				benthosfsm.OperationalStateStarting,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// from starting => startingConfigLoading
			mockService.SetServiceState(serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:          true,
				S6FSMState:           s6fsm.OperationalStateRunning,
				IsConfigLoaded:       true,
				IsHealthchecksPassed: true,
			})
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateStarting,
				benthosfsm.OperationalStateStartingConfigLoading,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// from startingConfigLoading => idle
			mockService.SetServiceState(serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				S6FSMState:             s6fsm.OperationalStateRunning,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   true,
				IsRunningWithoutErrors: true,
			})
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateStartingConfigLoading,
				benthosfsm.OperationalStateIdle,
				60,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetCurrentFSMState()).To(Equal(benthosfsm.OperationalStateIdle))

			// Step 3: from Idle => Active when processing data
			mockService.SetServiceState(serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   true,
				IsRunningWithoutErrors: true,
				HasProcessingActivity:  true,
			})

			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateIdle,
				benthosfsm.OperationalStateActive,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should transition to Degraded when issues occur", func() {
			// Step 1: to_be_created => creating => stopped
			var err error
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			mockService.ServiceStates[serviceName] = &benthossvc.ServiceInfo{S6FSMState: s6fsm.OperationalStateStopped}
			mockService.ExistingServices[serviceName] = true

			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				internalfsm.LifecycleStateCreating,
				benthosfsm.OperationalStateStopped,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// Step 2: from stopped => starting => configLoading => waiting => idle => active
			Expect(instance.SetDesiredFSMState(benthosfsm.OperationalStateActive)).To(Succeed())

			// from stopped => starting
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateStopped,
				benthosfsm.OperationalStateStarting,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// from starting => startingConfigLoading
			mockService.SetServiceState(serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:          true,
				S6FSMState:           s6fsm.OperationalStateRunning,
				IsConfigLoaded:       true,
				IsHealthchecksPassed: true,
			})
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateStarting,
				benthosfsm.OperationalStateStartingConfigLoading,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// from startingConfigLoading => idle
			mockService.SetServiceState(serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				S6FSMState:             s6fsm.OperationalStateRunning,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   true,
				IsRunningWithoutErrors: true,
			})
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateStartingConfigLoading,
				benthosfsm.OperationalStateIdle,
				60,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// from idle => active
			mockService.SetServiceState(serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   true,
				IsRunningWithoutErrors: true,
				HasProcessingActivity:  true,
			})
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateIdle,
				benthosfsm.OperationalStateActive,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// Step 3: Then degrade => set flags => "degraded"
			mockService.SetServiceState(serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   false,
				IsRunningWithoutErrors: false,
				HasProcessingActivity:  true,
			})
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateActive,
				benthosfsm.OperationalStateDegraded,
				10,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should recover from Degraded state when issues resolve", func() {
			// Step 1: to_be_created => creating => stopped
			var err error
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			mockService.ServiceStates[serviceName] = &benthossvc.ServiceInfo{S6FSMState: s6fsm.OperationalStateStopped}
			mockService.ExistingServices[serviceName] = true

			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				internalfsm.LifecycleStateCreating,
				benthosfsm.OperationalStateStopped,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// Step 2: from stopped => starting => configLoading => waiting => idle => active => degraded
			Expect(instance.SetDesiredFSMState(benthosfsm.OperationalStateActive)).To(Succeed())

			// from stopped => starting
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateStopped,
				benthosfsm.OperationalStateStarting,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// from starting => startingConfigLoading
			mockService.SetServiceState(serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:          true,
				S6FSMState:           s6fsm.OperationalStateRunning,
				IsConfigLoaded:       true,
				IsHealthchecksPassed: true,
			})
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateStarting,
				benthosfsm.OperationalStateStartingConfigLoading,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// from startingConfigLoading => idle
			mockService.SetServiceState(serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				S6FSMState:             s6fsm.OperationalStateRunning,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   true,
				IsRunningWithoutErrors: true,
			})
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateStartingConfigLoading,
				benthosfsm.OperationalStateIdle,
				60,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// from idle => active
			mockService.SetServiceState(serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   true,
				IsRunningWithoutErrors: true,
				HasProcessingActivity:  true,
			})
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateIdle,
				benthosfsm.OperationalStateActive,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// from active => degraded
			mockService.SetServiceState(serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   false,
				IsRunningWithoutErrors: false,
				HasProcessingActivity:  true,
			})
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateActive,
				benthosfsm.OperationalStateDegraded,
				10,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// Step 3: From degraded => idle when fixed
			mockService.SetServiceState(serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   true,
				IsRunningWithoutErrors: true,
				HasProcessingActivity:  false, // no data => idle
			})

			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateDegraded,
				benthosfsm.OperationalStateIdle,
				60,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// Step 4: from idle => active if HasProcessingActivity again
			mockService.SetServiceState(serviceName, benthossvc.ServiceStateFlags{
				HasProcessingActivity: true,
				IsS6Running:           true,
				IsConfigLoaded:        true,
				IsHealthchecksPassed:  true,
			})
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateIdle,
				benthosfsm.OperationalStateActive,
				5,
				tick,
				time.Now(),
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
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			mockService.ServiceStates[serviceName] = &benthossvc.ServiceInfo{S6FSMState: s6fsm.OperationalStateStopped}
			mockService.ExistingServices[serviceName] = true

			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				internalfsm.LifecycleStateCreating,
				benthosfsm.OperationalStateStopped,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// Step 2: from stopped => starting => configLoading => waiting => idle => active
			Expect(instance.SetDesiredFSMState(benthosfsm.OperationalStateActive)).To(Succeed())

			// from stopped => starting
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateStopped,
				benthosfsm.OperationalStateStarting,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// from starting => startingConfigLoading
			mockService.SetServiceState(serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:          true,
				S6FSMState:           s6fsm.OperationalStateRunning,
				IsConfigLoaded:       true,
				IsHealthchecksPassed: true,
			})
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateStarting,
				benthosfsm.OperationalStateStartingConfigLoading,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// from startingConfigLoading => idle
			mockService.SetServiceState(serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				S6FSMState:             s6fsm.OperationalStateRunning,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   true,
				IsRunningWithoutErrors: true,
			})
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateStartingConfigLoading,
				benthosfsm.OperationalStateIdle,
				60,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// from idle => active
			mockService.SetServiceState(serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				S6FSMState:             s6fsm.OperationalStateRunning,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   true,
				IsRunningWithoutErrors: true,
				HasProcessingActivity:  true,
			})
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateIdle,
				benthosfsm.OperationalStateActive,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// Step 3: from active => stopping => stopped
			Expect(instance.SetDesiredFSMState(benthosfsm.OperationalStateStopped)).To(Succeed())

			// from active => stopping
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateActive,
				benthosfsm.OperationalStateStopping,
				10,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// simulate S6 stopping
			mockService.SetServiceState(serviceName, benthossvc.ServiceStateFlags{
				IsS6Running: false,
				S6FSMState:  s6fsm.OperationalStateStopped,
			})

			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateStopping,
				benthosfsm.OperationalStateStopped,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should stop gracefully from Degraded state", func() {
			// Step 1: to_be_created => creating => stopped
			var err error
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			mockService.ServiceStates[serviceName] = &benthossvc.ServiceInfo{S6FSMState: s6fsm.OperationalStateStopped}
			mockService.ExistingServices[serviceName] = true

			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				internalfsm.LifecycleStateCreating,
				benthosfsm.OperationalStateStopped,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// Step 2: from stopped => starting => configLoading => waiting => idle => active => degraded
			Expect(instance.SetDesiredFSMState(benthosfsm.OperationalStateActive)).To(Succeed())

			// from stopped => starting
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateStopped,
				benthosfsm.OperationalStateStarting,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// from starting => startingConfigLoading
			mockService.SetServiceState(serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:          true,
				S6FSMState:           s6fsm.OperationalStateRunning,
				IsConfigLoaded:       true,
				IsHealthchecksPassed: true,
			})
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateStarting,
				benthosfsm.OperationalStateStartingConfigLoading,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// from startingConfigLoading => idle
			mockService.SetServiceState(serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				S6FSMState:             s6fsm.OperationalStateRunning,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   true,
				IsRunningWithoutErrors: true,
			})
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateStartingConfigLoading,
				benthosfsm.OperationalStateIdle,
				60,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// from idle => active
			mockService.SetServiceState(serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   true,
				IsRunningWithoutErrors: true,
				HasProcessingActivity:  true,
			})
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateIdle,
				benthosfsm.OperationalStateActive,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// from active => degraded
			mockService.SetServiceState(serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   false,
				IsRunningWithoutErrors: false,
				HasProcessingActivity:  true,
			})
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateActive,
				benthosfsm.OperationalStateDegraded,
				10,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// Step 3: set desired=stopped => degraded => stopping => stopped
			Expect(instance.SetDesiredFSMState(benthosfsm.OperationalStateStopped)).To(Succeed())

			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateDegraded,
				benthosfsm.OperationalStateStopping,
				10,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// simulate S6 stopping
			mockService.SetServiceState(serviceName, benthossvc.ServiceStateFlags{
				IsS6Running: false,
				S6FSMState:  s6fsm.OperationalStateStopped,
			})
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateStopping,
				benthosfsm.OperationalStateStopped,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// -------------------------------------------------------------------------
	//  COMPLEX STATE TRANSITIONS
	// -------------------------------------------------------------------------
	Context("complex state transitions", func() {
		It("should restart from Starting when config loading fails due to S6 instability", func() {
			var err error

			// from to_be_created => creating
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// mock creation success
			mockService.ServiceStates[serviceName] = &benthossvc.ServiceInfo{
				S6FSMState: s6fsm.OperationalStateStopped,
			}
			mockService.ExistingServices[serviceName] = true

			// from creating => stopped
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				internalfsm.LifecycleStateCreating,
				benthosfsm.OperationalStateStopped,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// set desired => active => from stopped => starting
			Expect(instance.SetDesiredFSMState(benthosfsm.OperationalStateActive)).To(Succeed())
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateStopped,
				benthosfsm.OperationalStateStarting,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// from starting => startingConfigLoading
			mockService.SetServiceState(serviceName, benthossvc.ServiceStateFlags{
				IsS6Running: true,
				S6FSMState:  s6fsm.OperationalStateRunning,
			})
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateStarting,
				benthosfsm.OperationalStateStartingConfigLoading,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// S6 crash => go back to "starting"
			mockService.SetServiceState(serviceName, benthossvc.ServiceStateFlags{
				IsS6Running: false,
				S6FSMState:  s6fsm.OperationalStateStopped,
			})
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateStartingConfigLoading,
				benthosfsm.OperationalStateStarting,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

		})
	})

	// -------------------------------------------------------------------------
	//  ERROR HANDLING
	// -------------------------------------------------------------------------
	Context("Error Handling", func() {
		It("should handle errors during startup", func() {
			// 1) We simulate port allocation, but don't need to use the result
			mockPortManager := portmanager.NewMockPortManager()
			_, portErr := mockPortManager.AllocatePort(context.Background(), serviceName)
			Expect(portErr).NotTo(HaveOccurred())

			// 2) Simulate service creation failure
			mockService.AddBenthosToS6ManagerError = fmt.Errorf("simulated creation error")

			// 3) Initial state: to_be_created
			Expect(instance.GetCurrentFSMState()).To(Equal(internalfsm.LifecycleStateToBeCreated))

			// 4) First reconcile attempt => triggers error internally
			//    According to the old test, we expect:
			//      - no external error returned (err == nil)
			//      - reconciled == false
			//      - instance remains in to_be_created
			err, reconciled := instance.Reconcile(ctx, fsm.SystemSnapshot{Tick: tick}, mockSvcRegistry)
			tick++
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeFalse())
			Expect(instance.GetCurrentFSMState()).To(Equal(internalfsm.LifecycleStateToBeCreated))

			// 5) Verify service creation was attempted
			Expect(mockService.AddBenthosToS6ManagerCalled).To(BeTrue())

			// 6) Clear the error & let the instance proceed
			mockService.AddBenthosToS6ManagerError = nil
			mockService.ServiceStates[serviceName] = &benthossvc.ServiceInfo{
				S6FSMState: s6fsm.OperationalStateStopped,
			}
			mockService.ExistingServices[serviceName] = true

			// 7) Next reconcile => now we succeed => instance transitions to "creating"
			err, reconciled = instance.Reconcile(ctx, fsm.SystemSnapshot{Tick: tick}, mockSvcRegistry)
			tick++
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())
			Expect(instance.GetCurrentFSMState()).To(Equal(internalfsm.LifecycleStateCreating))

			// 8) Another reconcile => we complete creation => "stopped"
			err, reconciled = instance.Reconcile(ctx, fsm.SystemSnapshot{Tick: tick}, mockSvcRegistry)
			tick++
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())
			Expect(instance.GetCurrentFSMState()).To(Equal(benthosfsm.OperationalStateStopped))
		})

		It("should handle errors during runtime", func() {

			// Step A: to_be_created => creating => stopped
			var err error
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			mockService.ServiceStates[serviceName] = &benthossvc.ServiceInfo{
				S6FSMState: s6fsm.OperationalStateStopped,
			}
			mockService.ExistingServices[serviceName] = true

			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				internalfsm.LifecycleStateCreating,
				benthosfsm.OperationalStateStopped,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// Step B: from stopped => idle => active
			// We can go "stopped => starting => configLoading => waiting => idle => active"
			// For brevity, we'll do short transitions:

			//  B1) desired=active
			Expect(instance.SetDesiredFSMState(benthosfsm.OperationalStateActive)).To(Succeed())

			//  B2) from "stopped => starting"
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateStopped,
				benthosfsm.OperationalStateStarting,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			//  B3) from "starting => startingConfigLoading"
			mockService.SetServiceState(serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:          true,
				S6FSMState:           s6fsm.OperationalStateRunning,
				IsConfigLoaded:       true,
				IsHealthchecksPassed: true,
			})
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateStarting,
				benthosfsm.OperationalStateStartingConfigLoading,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			//  B4) from "startingConfigLoading => idle"
			mockService.SetServiceState(serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				S6FSMState:             s6fsm.OperationalStateRunning,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   true,
				IsRunningWithoutErrors: true,
			})
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateStartingConfigLoading,
				benthosfsm.OperationalStateIdle,
				60,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			//  B5) from "idle => active"
			mockService.SetServiceState(serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   true,
				IsRunningWithoutErrors: true,
				HasProcessingActivity:  true,
			})
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateIdle,
				benthosfsm.OperationalStateActive,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// Step C: simulate runtime crash => degrade
			mockService.SetServiceState(serviceName, benthossvc.ServiceStateFlags{
				IsS6Running: false,
				S6FSMState:  s6fsm.OperationalStateStopped,
			})
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateActive,
				benthosfsm.OperationalStateDegraded,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// Step D: fix => degrade => idle
			mockService.SetServiceState(serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:          true,
				S6FSMState:           s6fsm.OperationalStateRunning,
				IsConfigLoaded:       true,
				IsHealthchecksPassed: true,
			})
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateDegraded,
				benthosfsm.OperationalStateIdle,
				60,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should continue reconciling despite permanent errors in UpdateObservedState", func() {
			// ARCHITECTURAL DECISION: We now continue reconciling even when UpdateObservedState
			// encounters permanent errors. This enables force-kill recovery scenarios where
			// S6 services exist on filesystem but FSM managers lose their in-memory mappings.
			//
			// The trade-off: We prioritize system recovery over immediate error handling.
			// Permanent errors in UpdateObservedState no longer trigger automatic FSM removal,
			// allowing the system to restore services after unexpected shutdowns/restarts.
			//
			// This test verifies that FSMs remain operational and continue state transitions
			// even when encountering persistent errors during observed state updates.

			var err error

			// First get to stopped state
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			mockService.ServiceStates[serviceName] = &benthossvc.ServiceInfo{S6FSMState: s6fsm.OperationalStateStopped}
			mockService.ExistingServices[serviceName] = true

			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				internalfsm.LifecycleStateCreating,
				benthosfsm.OperationalStateStopped,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// Now set desired state to active and transition to it
			Expect(instance.SetDesiredFSMState(benthosfsm.OperationalStateActive)).To(Succeed())

			// Set up service state for active transition
			mockService.SetServiceState(serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:            true,
				S6FSMState:             s6fsm.OperationalStateRunning,
				IsConfigLoaded:         true,
				IsHealthchecksPassed:   true,
				IsRunningWithoutErrors: true,
				HasProcessingActivity:  true,
			})

			// Transition through states to active
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateStopped,
				benthosfsm.OperationalStateActive,
				65, // Allow more attempts for multiple transitions
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// Verify we're in active state
			Expect(instance.GetCurrentFSMState()).To(Equal(benthosfsm.OperationalStateActive))
			Expect(instance.GetDesiredFSMState()).To(Equal(benthosfsm.OperationalStateActive))

			// Create a permanent error in StatusError
			mockService.StatusError = fmt.Errorf("%s: test permanent error", backoff.PermanentFailureError)

			// With the new architecture, the FSM should continue reconciling despite the permanent error
			// Run several reconciliation cycles to verify it doesn't get stuck or crash
			snapshot := fsm.SystemSnapshot{Tick: tick}
			for i := 0; i < 5; i++ {
				err, _ := instance.Reconcile(ctx, snapshot, mockSvcRegistry)
				Expect(err).NotTo(HaveOccurred(), "Reconciliation should continue despite permanent UpdateObservedState errors")
				tick++
				snapshot.Tick = tick
			}

			// FSM should maintain its desired state and continue operating
			Expect(instance.GetCurrentFSMState()).To(Equal(benthosfsm.OperationalStateActive))
			Expect(instance.GetDesiredFSMState()).To(Equal(benthosfsm.OperationalStateActive))

			// Clear error for other tests
			mockService.StatusError = nil
		})

		It("should continue reconciling despite permanent errors in UpdateObservedState", func() {
			// ARCHITECTURAL DECISION: We now continue reconciling even when UpdateObservedState
			// encounters permanent errors. This enables force-kill recovery scenarios where
			// S6 services exist on filesystem but FSM managers lose their in-memory mappings.
			//
			// The trade-off: We prioritize system recovery over immediate error handling.
			// Permanent errors in UpdateObservedState no longer trigger automatic FSM removal,
			// allowing the system to restore services after unexpected shutdowns/restarts.

			// 1) Get to stopped state using proper transitions
			var err error

			// First progress to creating state
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// Setup service in stopped state
			mockService.ServiceStates[serviceName] = &benthossvc.ServiceInfo{S6FSMState: s6fsm.OperationalStateStopped}
			mockService.ExistingServices[serviceName] = true

			// Progress to stopped state
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				internalfsm.LifecycleStateCreating,
				benthosfsm.OperationalStateStopped,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// Set desired state to active to trigger reconciliation
			Expect(instance.SetDesiredFSMState(benthosfsm.OperationalStateActive)).To(Succeed())

			// Create a permanent error that will be encountered during UpdateObservedState
			mockService.StatusError = fmt.Errorf("%s: test permanent error", backoff.PermanentFailureError)

			// Attempt reconciliation - should continue despite the error
			var recErr error
			snapshot := fsm.SystemSnapshot{Tick: tick}

			// Single reconcile attempt should succeed despite the UpdateObservedState error
			recErr, _ = instance.Reconcile(ctx, snapshot, mockSvcRegistry)

			// With the new architecture, reconcile should NOT return an error for UpdateObservedState failures
			Expect(recErr).NotTo(HaveOccurred(), "Reconcile should continue despite UpdateObservedState errors")

			// FSM should continue reconciling toward the desired state despite UpdateObservedState errors
			// Since we set desired state to active and continue reconciling, the FSM should progress toward starting
			Expect(instance.GetDesiredFSMState()).To(Equal(benthosfsm.OperationalStateActive))
			// The current state may have progressed from stopped to starting since reconciliation continues
			currentState := instance.GetCurrentFSMState()
			Expect(currentState).To(Or(Equal(benthosfsm.OperationalStateStopped), Equal(benthosfsm.OperationalStateStarting)),
				"FSM should either remain stopped or progress to starting despite UpdateObservedState errors")

			// Force removal should NOT be attempted since we continue reconciling
			Expect(mockService.ForceRemoveBenthosCalled).To(BeFalse(), "Force removal should not be triggered for UpdateObservedState errors")

			// Clear error for other tests
			mockService.StatusError = nil
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
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// Setup service in stopped state
			mockService.ServiceStates[serviceName] = &benthossvc.ServiceInfo{S6FSMState: s6fsm.OperationalStateStopped}
			mockService.ExistingServices[serviceName] = true

			// Progress to stopped state
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				internalfsm.LifecycleStateCreating,
				benthosfsm.OperationalStateStopped,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			instance.SetDesiredFSMState(benthosfsm.OperationalStateActive)

			// Progress to starting state
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateStopped,
				benthosfsm.OperationalStateStarting,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// Progress to config loading state
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				benthosfsm.OperationalStateStarting,
				benthosfsm.OperationalStateStartingConfigLoading,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// Create a permanent error that will be encountered during UpdateObservedState
			mockService.StatusError = fmt.Errorf("%s: test permanent error", backoff.PermanentFailureError)

			// Attempt single reconciliation - should continue despite the error
			snapshot := fsm.SystemSnapshot{Tick: tick}
			recErr, _ := instance.Reconcile(ctx, snapshot, mockSvcRegistry)

			// With the new architecture, reconcile should NOT return an error for UpdateObservedState failures
			Expect(recErr).NotTo(HaveOccurred(), "Reconcile should continue despite UpdateObservedState errors")

			// FSM should maintain its desired state and continue operating
			Expect(instance.GetDesiredFSMState()).To(Equal(benthosfsm.OperationalStateActive))
			// The current state should remain in starting config loading or may progress
			currentState := instance.GetCurrentFSMState()
			Expect(benthosfsm.IsStartingState(currentState) || benthosfsm.IsRunningState(currentState)).To(BeTrue(),
				"FSM should maintain starting state or progress despite UpdateObservedState errors")

			// Force removal should NOT be attempted since we continue reconciling
			Expect(mockService.ForceRemoveBenthosCalled).To(BeFalse(), "Force removal should not be triggered for UpdateObservedState errors")

			// Clear error for other tests
			mockService.StatusError = nil
		})
	})

	// -------------------------------------------------------------------------
	//  STATE TRANSITION ERROR HANDLING TESTS
	// -------------------------------------------------------------------------
	Context("State Transition Error Handling", func() {
		It("should attempt forced removal when encountering permanent errors in state transitions", func() {
			// This test verifies that permanent errors in state transition actions (not UpdateObservedState)
			// still trigger the traditional forced removal behavior. This ensures the distinction between:
			// - UpdateObservedState errors (ignored for recovery)
			// - State transition errors (still trigger removal)

			// 1) Get to stopped state using proper transitions
			var err error

			// First progress to creating state
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// Setup service in stopped state
			mockService.ServiceStates[serviceName] = &benthossvc.ServiceInfo{S6FSMState: s6fsm.OperationalStateStopped}
			mockService.ExistingServices[serviceName] = true

			// Progress to stopped state
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				internalfsm.LifecycleStateCreating,
				benthosfsm.OperationalStateStopped,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// Set desired state to active to trigger start transition
			Expect(instance.SetDesiredFSMState(benthosfsm.OperationalStateActive)).To(Succeed())

			// Create a permanent error in StartBenthos action (NOT StatusError)
			mockService.StartBenthosError = fmt.Errorf("%s: test permanent error", backoff.PermanentFailureError)

			// Use the helper function to reconcile until error
			var recErr error
			var reconciled bool
			tick, recErr, reconciled = fsmtest.ReconcileBenthosUntilError(
				ctx, fsm.SystemSnapshot{Tick: tick}, instance, mockService, mockSvcRegistry, serviceName, 10,
			)

			// Should get the error since it's in state transition, not UpdateObservedState
			Expect(recErr).To(HaveOccurred())
			Expect(recErr.Error()).To(ContainSubstring(backoff.PermanentFailureError))
			Expect(reconciled).To(BeTrue(), "Should have reconciled during error handling")

			// Verify force removal was attempted for state transition errors
			Expect(mockService.ForceRemoveBenthosCalled).To(BeTrue())

			// Clear error for other tests
			mockService.StartBenthosError = nil
		})

		It("should validate architectural principle that UpdateObservedState errors don't block reconciliation", func() {
			// This test validates the key architectural principle:
			// UpdateObservedState errors (like StatusError) should continue reconciling (not block FSM progression)
			// This enables force-kill recovery scenarios where S6 services exist on filesystem
			// but FSM managers lose their in-memory mappings.
			//
			// Note: The first test in this context already proved that state transition errors
			// (like StartError) still trigger forced removal as expected.

			var err error

			// Setup FSM in stopped state
			tick, err = fsmtest.TestBenthosStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				benthosfsm.OperationalStateStopped,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// Inject permanent error in UpdateObservedState (StatusError)
			mockService.StatusError = fmt.Errorf("%s: test permanent error", backoff.PermanentFailureError)

			// Set desired state to trigger reconciliation
			Expect(instance.SetDesiredFSMState(benthosfsm.OperationalStateActive)).To(Succeed())

			// Multiple reconciliation attempts should continue despite StatusError
			for i := 0; i < 5; i++ {
				recErr, _ := instance.Reconcile(ctx, fsm.SystemSnapshot{Tick: tick}, mockSvcRegistry)
				Expect(recErr).NotTo(HaveOccurred(), "StatusError should not block reconciliation")
				tick++
			}

			// FSM should progress toward desired state despite UpdateObservedState errors
			currentState := instance.GetCurrentFSMState()
			Expect(benthosfsm.IsStartingState(currentState) || benthosfsm.IsRunningState(currentState)).To(BeTrue(),
				"FSM should progress despite UpdateObservedState errors")

			// Force removal should NOT have been called for UpdateObservedState errors
			Expect(mockService.ForceRemoveBenthosCalled).To(BeFalse(), "UpdateObservedState errors should not trigger forced removal")

			// Clear error for other tests
			mockService.StatusError = nil
		})
	})

	// -------------------------------------------------------------------------
	//  HEALTH CHECK TESTS
	// -------------------------------------------------------------------------
	Context("Health Check Tests", func() {
		It("should require stable duration before reporting success", func() {
			// Set up service with passing health checks
			fsmtest.SetupBenthosServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:          true,
				S6FSMState:           s6fsm.OperationalStateRunning,
				IsConfigLoaded:       true,
				IsHealthchecksPassed: true,
			})

			// First check - should not pass due to debounce
			tick := uint64(1)
			passed, reason := instance.IsBenthosHealthchecksPassed(tick, mockSvcRegistry.GetLoopManager().GetTickerTime())
			Expect(passed).To(BeFalse())
			Expect(reason).To(ContainSubstring("healthchecks did not pass: live=false, ready=false")) // first execution of the healthcheck will always fail
			tick++

			// now lets reconcile once to set the healthcheck to passing
			snapshot := fsm.SystemSnapshot{Tick: tick}
			err, reconciled := instance.Reconcile(ctx, snapshot, mockSvcRegistry)
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())
			tick = tick + 1

			// now lets check the healthcheck again
			passed, reason = instance.IsBenthosHealthchecksPassed(tick, mockSvcRegistry.GetLoopManager().GetTickerTime())
			Expect(passed).To(BeFalse())
			Expect(reason).To(ContainSubstring("healthchecks passing but not stable yet"))

			// Advance time past debounce duration
			tx := constants.BenthosHealthCheckStableDuration / mockSvcRegistry.GetLoopManager().GetTickerTime()
			tick = tick + uint64(tx.Seconds())
			passed, reason = instance.IsBenthosHealthchecksPassed(tick, mockSvcRegistry.GetLoopManager().GetTickerTime())
			Expect(passed).To(BeTrue())
			Expect(reason).To(BeEmpty())
		})

		It("should report failure with detailed reason", func() {
			// Set up service with failing health checks
			fsmtest.SetupBenthosServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:          true,
				S6FSMState:           s6fsm.OperationalStateRunning,
				IsConfigLoaded:       true,
				IsHealthchecksPassed: false,
			})

			tick := uint64(0)
			passed, reason := instance.IsBenthosHealthchecksPassed(tick, mockSvcRegistry.GetLoopManager().GetTickerTime())
			Expect(passed).To(BeFalse())
			Expect(reason).To(ContainSubstring("healthchecks did not pass: live=false, ready=false"))
		})

		It("should reset debounce timer when health checks fail", func() {
			// First set up passing health checks
			fsmtest.SetupBenthosServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:          true,
				S6FSMState:           s6fsm.OperationalStateRunning,
				IsConfigLoaded:       true,
				IsHealthchecksPassed: true,
			})

			// Initial check - should fail
			tick := uint64(1)
			passed, _ := instance.IsBenthosHealthchecksPassed(tick, mockSvcRegistry.GetLoopManager().GetTickerTime())
			Expect(passed).To(BeFalse())
			tick++

			// reconcile once to set the healthcheck to passing
			snapshot := fsm.SystemSnapshot{Tick: tick}
			err, reconciled := instance.Reconcile(ctx, snapshot, mockSvcRegistry)
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())
			tick++

			// now b.healthChecksPassingSinceTick is set
			tick = tick + 1
			passed, _ = instance.IsBenthosHealthchecksPassed(tick, mockSvcRegistry.GetLoopManager().GetTickerTime())
			Expect(passed).To(BeFalse()) // cannot pass as it just detected that the healthchecks are passing now

			// now set to half of the debounce duration
			tx := constants.BenthosHealthCheckStableDuration / mockSvcRegistry.GetLoopManager().GetTickerTime()
			tick = tick + uint64(tx.Seconds()) + 1
			passed, _ = instance.IsBenthosHealthchecksPassed(tick, mockSvcRegistry.GetLoopManager().GetTickerTime())
			Expect(passed).To(BeTrue()) // is now passing

			// Now make health checks fail
			fsmtest.SetupBenthosServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
				IsS6Running:          true,
				S6FSMState:           s6fsm.OperationalStateRunning,
				IsConfigLoaded:       true,
				IsHealthchecksPassed: false,
			})

			// reconcile once to set the healthcheck to failing
			snapshot = fsm.SystemSnapshot{Tick: tick}
			err, reconciled = instance.Reconcile(ctx, snapshot, mockSvcRegistry)
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())
			tick++

			// Check again - should fail and reset timer
			passed, reason := instance.IsBenthosHealthchecksPassed(tick, mockSvcRegistry.GetLoopManager().GetTickerTime())
			Expect(passed).To(BeFalse())
			Expect(reason).To(ContainSubstring("healthchecks did not pass"))

			// Advance time past original debounce duration
			tx = constants.BenthosHealthCheckStableDuration / mockSvcRegistry.GetLoopManager().GetTickerTime()
			tick = tick + uint64(tx.Seconds())
			passed, _ = instance.IsBenthosHealthchecksPassed(tick, mockSvcRegistry.GetLoopManager().GetTickerTime())
			Expect(passed).To(BeFalse()) // Should still fail because timer was reset
		})
	})

})
