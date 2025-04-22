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

package dataflowcomponent_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsmtest"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/backoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	dataflowcomponentsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// Following the CursorRules, we never call manager.Reconcile(...) directly in loops.
// Instead, we use the fsmtest helpers.

var _ = Describe("DataflowComponentManager", func() {
	var (
		manager     *dataflowcomponent.DataflowComponentManager
		mockService *dataflowcomponentsvc.MockDataFlowComponentService
		ctx         context.Context
		tick        uint64
		cancel      context.CancelFunc
		mockFS      *filesystem.MockFileSystem
	)

	AfterEach(func() {
		cancel()
	})

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Minute) // we need to have a deadline as the reconcile logic in the base fsm manager requires it
		tick = 0
		mockFS = filesystem.NewMockFileSystem()
		// Create a new DataflowComponentManager with the mock service
		manager, mockService = fsmtest.CreateMockDataflowComponentManager("test-manager")

		// Initialize the mock service state to empty
		mockService.ExistingComponents = make(map[string]bool)
		mockService.ComponentStates = make(map[string]*dataflowcomponentsvc.ServiceInfo)
	})

	// -------------------------------------------------------------------------
	//  INITIALIZATION
	// -------------------------------------------------------------------------
	Context("Initialization", func() {
		It("should handle empty config without errors", func() {
			emptyConfig := config.FullConfig{DataFlow: []config.DataFlowComponentConfig{}}

			// Single call to a helper that wraps Reconcile
			newTick, err := fsmtest.WaitForDataflowComponentManagerStable(
				ctx, fsm.SystemSnapshot{CurrentConfig: emptyConfig, Tick: tick}, manager, mockFS,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())
			Expect(manager.GetInstances()).To(BeEmpty())
		})

		It("should create a service in stopped state and remain stable", func() {
			componentName := "test-stopped-component"
			cfg := config.FullConfig{
				DataFlow: []config.DataFlowComponentConfig{
					fsmtest.CreateDataflowComponentTestConfig(componentName, dataflowcomponent.OperationalStateStopped),
				},
			}

			// Configure the mock service to allow transition to Stopped
			fsmtest.ConfigureDataflowComponentManagerForState(mockService, componentName, dataflowcomponent.OperationalStateStopped)

			// Wait for instance creation and stable 'Stopped' state
			newTick, err := fsmtest.WaitForDataflowComponentManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick},
				manager,
				mockFS,
				componentName,
				dataflowcomponent.OperationalStateStopped,
				10,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Double-check the manager state
			inst, exists := manager.GetInstance(fmt.Sprintf("dataflow-%s", componentName))
			Expect(exists).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(dataflowcomponent.OperationalStateStopped))
		})

		It("should create a service in active state and reach idle or active", func() {
			componentName := "test-active-component"
			cfg := config.FullConfig{
				DataFlow: []config.DataFlowComponentConfig{
					fsmtest.CreateDataflowComponentTestConfig(componentName, dataflowcomponent.OperationalStateActive),
				},
			}

			// Configure the mock service for transition to Idle (or Active)
			fsmtest.ConfigureDataflowComponentManagerForState(mockService, componentName, dataflowcomponent.OperationalStateActive)

			newTick, err := fsmtest.WaitForDataflowComponentManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick},
				manager,
				mockFS,
				componentName,
				dataflowcomponent.OperationalStateIdle, // or OperationalStateActive, whichever is stable
				20,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Service Lifecycle", func() {
		It("should go from creation -> idle -> degrade -> removal", func() {

			serviceName := "test-lifecycle"

			fullCfg := config.FullConfig{
				DataFlow: []config.DataFlowComponentConfig{
					fsmtest.CreateDataflowComponentTestConfig(serviceName, dataflowcomponent.OperationalStateActive),
				},
			}

			fsmtest.ConfigureDataflowComponentManagerForState(mockService, serviceName, dataflowcomponent.OperationalStateIdle)

			// 1) Wait for idle
			newTick, err := fsmtest.WaitForDataflowComponentManagerInstanceState(ctx, fsm.SystemSnapshot{
				CurrentConfig: fullCfg,
				Tick:          tick,
			}, manager, mockFS, serviceName, dataflowcomponent.OperationalStateIdle, 20)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// 2) Now configure for degraded state
			fsmtest.ConfigureDataflowComponentManagerForState(mockService, serviceName, dataflowcomponent.OperationalStateDegraded)

			// Wait for state transition
			newTick, err = fsmtest.WaitForDataflowComponentManagerInstanceState(ctx, fsm.SystemSnapshot{
				CurrentConfig: fullCfg,
				Tick:          tick,
			}, manager, mockFS, serviceName, dataflowcomponent.OperationalStateDegraded, 20)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())
			Expect(manager.GetInstances()).To(HaveLen(1))

			// 3) Configure for stopped state before removal
			fsmtest.ConfigureDataflowComponentManagerForState(mockService, serviceName, dataflowcomponent.OperationalStateStopped)
			// Service is stopped at this point

			// Wait for removal
			emptyCfg := config.FullConfig{DataFlow: []config.DataFlowComponentConfig{}}
			newTick, err = fsmtest.WaitForDataflowComponentManagerInstanceState(ctx, fsm.SystemSnapshot{
				CurrentConfig: emptyCfg,
				Tick:          tick,
			}, manager, mockFS, serviceName, dataflowcomponent.OperationalStateStopped, 20)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())
			Expect(manager.GetInstances()).NotTo(HaveKey(serviceName))

		})

		It("should toggle from active/idle to stopped and back to active/idle with config changes", func() {
			componentName := "test-toggle-component"
			activeCfg := fsmtest.CreateDataflowComponentTestConfig(componentName, dataflowcomponent.OperationalStateActive)

			fullCfg := config.FullConfig{
				DataFlow: []config.DataFlowComponentConfig{activeCfg},
			}

			// Configure for idle state
			fsmtest.ConfigureDataflowComponentManagerForState(mockService, componentName, dataflowcomponent.OperationalStateIdle)

			// Wait for idle
			newTick, err := fsmtest.WaitForDataflowComponentManagerInstanceState(ctx, fsm.SystemSnapshot{
				CurrentConfig: fullCfg,
				Tick:          tick,
			}, manager, mockFS, componentName, dataflowcomponent.OperationalStateIdle, 20)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			fsmtest.ConfigureDataflowComponentManagerForState(mockService, componentName, dataflowcomponent.OperationalStateStopped)
			// Toggle to stopped
			stopCfg := fsmtest.CreateDataflowComponentTestConfig(componentName, dataflowcomponent.OperationalStateStopped)
			fullCfg.DataFlow = []config.DataFlowComponentConfig{stopCfg}

			// Wait for stopped
			newTick, err = fsmtest.WaitForDataflowComponentManagerInstanceState(ctx, fsm.SystemSnapshot{
				CurrentConfig: fullCfg,
				Tick:          tick,
			}, manager, mockFS, componentName, dataflowcomponent.OperationalStateStopped, 20)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			fullCfg.DataFlow = []config.DataFlowComponentConfig{activeCfg}
			// Toggle back to active
			fsmtest.ConfigureDataflowComponentManagerForState(mockService, componentName, dataflowcomponent.OperationalStateIdle)

			// Wait for active
			newTick, err = fsmtest.WaitForDataflowComponentManagerInstanceState(ctx, fsm.SystemSnapshot{
				CurrentConfig: fullCfg,
				Tick:          tick,
			}, manager, mockFS, componentName, dataflowcomponent.OperationalStateIdle, 20)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

		})
	})

	// -------------------------------------------------------------------------
	//  MULTIPLE SERVICES
	// -------------------------------------------------------------------------
	Context("Multiple Services", func() {
		It("should handle multiple components in parallel, each with its own state", func() {
			comp1Name := "component1"
			comp2Name := "component2"

			config1 := fsmtest.CreateDataflowComponentTestConfig(comp1Name, dataflowcomponent.OperationalStateActive)
			config2 := fsmtest.CreateDataflowComponentTestConfig(comp2Name, dataflowcomponent.OperationalStateStopped)
			fullCfg := config.FullConfig{
				DataFlow: []config.DataFlowComponentConfig{config1, config2},
			}

			// Configure component1 for Idle, component2 for Stopped
			fsmtest.ConfigureDataflowComponentManagerForState(mockService, comp1Name, dataflowcomponent.OperationalStateIdle)
			fsmtest.ConfigureDataflowComponentManagerForState(mockService, comp2Name, dataflowcomponent.OperationalStateStopped)

			// Wait for both components to reach their target states
			// Note: This requires a helper like WaitForDataflowComponentManagerMultiState in fsmtest
			// Assuming such a helper exists or can be created similar to the Benthos one.
			newTick, err := fsmtest.WaitForDataflowComponentManagerMultiState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick},
				manager,
				mockFS,
				map[string]string{
					comp1Name: dataflowcomponent.OperationalStateIdle,
					comp2Name: dataflowcomponent.OperationalStateStopped,
				},
				30, // Attempts
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Confirm manager sees both instances
			Expect(manager.GetInstances()).To(HaveLen(2))
			Expect(manager.GetInstances()).To(HaveKey(fmt.Sprintf("dataflow-%s", comp1Name)))
			Expect(manager.GetInstances()).To(HaveKey(fmt.Sprintf("dataflow-%s", comp2Name)))

			// Check the states
			inst1, exists := manager.GetInstance(fmt.Sprintf("dataflow-%s", comp1Name))
			Expect(exists).To(BeTrue())
			Expect(inst1.GetCurrentFSMState()).To(Equal(dataflowcomponent.OperationalStateIdle))

			inst2, exists := manager.GetInstance(fmt.Sprintf("dataflow-%s", comp2Name))
			Expect(exists).To(BeTrue())
			Expect(inst2.GetCurrentFSMState()).To(Equal(dataflowcomponent.OperationalStateStopped))
		})
	})

	Context("Error Handling", func() {
		It("should remove an instance if it hits a permanent error in stopped state", func() {
			serviceName := "perm-error-test"
			fullCfg := config.FullConfig{
				DataFlow: []config.DataFlowComponentConfig{
					fsmtest.CreateDataflowComponentTestConfig(serviceName, dataflowcomponent.OperationalStateActive),
				},
			}

			// Configure for transition to Idle
			fsmtest.ConfigureDataflowComponentManagerForState(mockService, serviceName, dataflowcomponent.OperationalStateIdle)

			// Wait for idle
			newTick, err := fsmtest.WaitForDataflowComponentManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick}, manager, mockFS,
				serviceName,
				dataflowcomponent.OperationalStateIdle,
				20,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Configure for transition to Stopped
			fsmtest.ConfigureDataflowComponentManagerForState(mockService, serviceName, dataflowcomponent.OperationalStateStopped)

			// Set desired state to stopped
			stoppedCfg := config.FullConfig{
				DataFlow: []config.DataFlowComponentConfig{
					fsmtest.CreateDataflowComponentTestConfig(serviceName, dataflowcomponent.OperationalStateStopped),
				},
			}

			// Wait for stopped
			newTick, err = fsmtest.WaitForDataflowComponentManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: stoppedCfg, Tick: tick}, manager, mockFS,
				serviceName,
				dataflowcomponent.OperationalStateStopped,
				20,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Simulate a permanent error by configuring the mock service
			mockService.AddDataFlowComponentToBenthosManagerError = fmt.Errorf("%s: forced error", backoff.PermanentFailureError)

			// Wait for manager to remove instance due to permanent error
			newTick, err = fsmtest.WaitForDataflowComponentManagerInstanceRemoval(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: config.FullConfig{DataFlow: []config.DataFlowComponentConfig{}}},
				manager,
				mockFS,
				serviceName,
				15,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			Expect(manager.GetInstances()).NotTo(HaveKey(serviceName))
		})

	})
})
