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

package streamprocessor_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsmtest"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	spfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/streamprocessor"
	spsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/streamprocessor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// Following the CursorRules, we never call manager.Reconcile(...) directly in loops.
// Instead, we use the fsmtest helpers.

var _ = Describe("StreamProcessorManager", func() {
	var (
		manager         *spfsm.Manager
		mockService     *spsvc.MockService
		ctx             context.Context
		tick            uint64
		cancel          context.CancelFunc
		mockSvcRegistry *serviceregistry.Registry
	)

	AfterEach(func() {
		cancel()
	})

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second) // we need to have a deadline as the reconcile logic in the base fsm manager requires it
		tick = 0
		mockSvcRegistry = serviceregistry.NewMockRegistry()
		// Create a new StreamProcessorManager with the mock service
		manager, mockService = fsmtest.CreateMockStreamProcessorManager("test-manager")

		// Initialize the mock service state to empty
		mockService.ExistingComponents = make(map[string]bool)
		mockService.States = make(map[string]*spsvc.ServiceInfo)
	})

	// -------------------------------------------------------------------------
	//  INITIALIZATION
	// -------------------------------------------------------------------------
	Context("Initialization", func() {
		It("should handle empty config without errors", func() {
			emptyConfig := config.FullConfig{StreamProcessor: []config.StreamProcessorConfig{}}

			// Single call to a helper that wraps Reconcile
			newTick, err := fsmtest.WaitForStreamProcessorManagerStable(
				ctx, fsm.SystemSnapshot{CurrentConfig: emptyConfig, Tick: tick}, manager, mockSvcRegistry,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())
			Expect(manager.GetInstances()).To(BeEmpty())
		})

		It("should create a service in stopped state and remain stable", func() {
			processorName := "test-stopped-processor"
			cfg := config.FullConfig{
				StreamProcessor: []config.StreamProcessorConfig{
					fsmtest.CreateStreamProcessorTestConfig(processorName, spfsm.OperationalStateStopped),
				},
			}

			// Configure the mock service to allow transition to Stopped
			fsmtest.ConfigureStreamProcessorManagerForState(mockService, processorName, spfsm.OperationalStateStopped)

			// Wait for instance creation and stable 'Stopped' state
			newTick, err := fsmtest.WaitForStreamProcessorManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick},
				manager,
				mockSvcRegistry,
				processorName,
				spfsm.OperationalStateStopped,
				10,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Double-check the manager state
			inst, exists := manager.GetInstance(fmt.Sprintf("streamprocessor-%s", processorName))
			Expect(exists).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(spfsm.OperationalStateStopped))
		})

		It("should create a service in active state and reach idle or active", func() {
			processorName := "test-active-processor"
			cfg := config.FullConfig{
				StreamProcessor: []config.StreamProcessorConfig{
					fsmtest.CreateStreamProcessorTestConfig(processorName, spfsm.OperationalStateActive),
				},
			}

			// Configure the mock service for transition to Idle (or Active)
			fsmtest.ConfigureStreamProcessorManagerForState(mockService, processorName, spfsm.OperationalStateIdle)

			// StreamProcessor FSMs typically pass through 'Idle' before 'Active'
			newTick, err := fsmtest.WaitForStreamProcessorManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick},
				manager,
				mockSvcRegistry,
				processorName,
				spfsm.OperationalStateIdle, // or OperationalStateActive, whichever is stable
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// -------------------------------------------------------------------------
	//  SERVICE LIFECYCLE
	// -------------------------------------------------------------------------
	Context("Service Lifecycle", func() {
		It("should go from creation → idle → degrade → removal", func() {
			processorName := "test-lifecycle"
			// Start from active config
			fullCfg := config.FullConfig{
				StreamProcessor: []config.StreamProcessorConfig{
					fsmtest.CreateStreamProcessorTestConfig(processorName, spfsm.OperationalStateActive),
				},
			}

			// Configure for transition to Idle
			fsmtest.ConfigureStreamProcessorManagerForState(mockService, processorName, spfsm.OperationalStateIdle)

			// 1) Wait for idle
			newTick, err := fsmtest.WaitForStreamProcessorManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick}, manager, mockSvcRegistry,
				processorName,
				spfsm.OperationalStateIdle,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// 2) Now configure for degraded state
			fsmtest.ConfigureStreamProcessorManagerForState(mockService, processorName, spfsm.OperationalStateDegradedDFC)

			// Wait for state transition
			newTick, err = fsmtest.WaitForStreamProcessorManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick}, manager, mockSvcRegistry,
				processorName,
				spfsm.OperationalStateDegradedDFC,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// 3) Configure for stopped state before removal
			fsmtest.ConfigureStreamProcessorManagerForState(mockService, processorName, spfsm.OperationalStateStopped)

			// 4) Remove from config => instance eventually stops & is removed
			emptyConfig := config.FullConfig{StreamProcessor: []config.StreamProcessorConfig{}}
			newTick, err = fsmtest.WaitForStreamProcessorManagerInstanceRemoval(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: emptyConfig, Tick: tick},
				manager,
				mockSvcRegistry,
				fmt.Sprintf("streamprocessor-%s", processorName),
				30, // More attempts for removal
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())
			Expect(manager.GetInstances()).NotTo(HaveKey(fmt.Sprintf("streamprocessor-%s", processorName)))
		})

		It("should toggle from active/idle to stopped and back to active/idle with config changes", func() {
			processorName := "test-toggle-processor"
			activeCfg := fsmtest.CreateStreamProcessorTestConfig(processorName, spfsm.OperationalStateActive)

			fullCfg := config.FullConfig{
				StreamProcessor: []config.StreamProcessorConfig{activeCfg},
			}

			// Configure for transition to Idle
			fsmtest.ConfigureStreamProcessorManagerForState(mockService, processorName, spfsm.OperationalStateIdle)

			// 1) Wait for idle
			newTick, err := fsmtest.WaitForStreamProcessorManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick}, manager, mockSvcRegistry,
				processorName,
				spfsm.OperationalStateIdle,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// 2) Change to stopped config
			stoppedCfg := fsmtest.CreateStreamProcessorTestConfig(processorName, spfsm.OperationalStateStopped)
			fullCfg.StreamProcessor = []config.StreamProcessorConfig{stoppedCfg}

			// Configure for transition to Stopped
			fsmtest.ConfigureStreamProcessorManagerForState(mockService, processorName, spfsm.OperationalStateStopped)

			// Wait for stopped state
			newTick, err = fsmtest.WaitForStreamProcessorManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick}, manager, mockSvcRegistry,
				processorName,
				spfsm.OperationalStateStopped,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// 3) Change back to active config
			activeCfg = fsmtest.CreateStreamProcessorTestConfig(processorName, spfsm.OperationalStateActive)
			fullCfg.StreamProcessor = []config.StreamProcessorConfig{activeCfg}

			// Configure for transition back to Idle
			fsmtest.ConfigureStreamProcessorManagerForState(mockService, processorName, spfsm.OperationalStateIdle)

			// Wait for idle state again
			newTick, err = fsmtest.WaitForStreamProcessorManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick}, manager, mockSvcRegistry,
				processorName,
				spfsm.OperationalStateIdle,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// -------------------------------------------------------------------------
	//  MULTI-INSTANCE MANAGEMENT
	// -------------------------------------------------------------------------
	Context("Multi-Instance Management", func() {
		It("should manage multiple processors independently", func() {
			processor1Name := "test-processor-1"
			processor2Name := "test-processor-2"
			processor3Name := "test-processor-3"

			// Create three processors with different desired states
			cfg1 := fsmtest.CreateStreamProcessorTestConfig(processor1Name, spfsm.OperationalStateActive)
			cfg2 := fsmtest.CreateStreamProcessorTestConfig(processor2Name, spfsm.OperationalStateStopped)
			cfg3 := fsmtest.CreateStreamProcessorTestConfig(processor3Name, spfsm.OperationalStateActive)

			fullCfg := config.FullConfig{
				StreamProcessor: []config.StreamProcessorConfig{cfg1, cfg2, cfg3},
			}

			// Configure all processors
			fsmtest.ConfigureStreamProcessorManagerForState(mockService, processor1Name, spfsm.OperationalStateIdle)
			fsmtest.ConfigureStreamProcessorManagerForState(mockService, processor2Name, spfsm.OperationalStateStopped)
			fsmtest.ConfigureStreamProcessorManagerForState(mockService, processor3Name, spfsm.OperationalStateIdle)

			// Wait for all instances to reach their target states
			newTick, err := fsmtest.WaitForStreamProcessorManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick}, manager, mockSvcRegistry,
				processor1Name,
				spfsm.OperationalStateIdle,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			newTick, err = fsmtest.WaitForStreamProcessorManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick}, manager, mockSvcRegistry,
				processor2Name,
				spfsm.OperationalStateStopped,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			newTick, err = fsmtest.WaitForStreamProcessorManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick}, manager, mockSvcRegistry,
				processor3Name,
				spfsm.OperationalStateIdle,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Verify all instances exist and are in correct states
			instances := manager.GetInstances()
			Expect(instances).To(HaveLen(3))
			Expect(instances).To(HaveKey(fmt.Sprintf("streamprocessor-%s", processor1Name)))
			Expect(instances).To(HaveKey(fmt.Sprintf("streamprocessor-%s", processor2Name)))
			Expect(instances).To(HaveKey(fmt.Sprintf("streamprocessor-%s", processor3Name)))
		})

		It("should handle partial processor removal", func() {
			processor1Name := "test-processor-remove-1"
			processor2Name := "test-processor-remove-2"

			// Create two processors
			cfg1 := fsmtest.CreateStreamProcessorTestConfig(processor1Name, spfsm.OperationalStateActive)
			cfg2 := fsmtest.CreateStreamProcessorTestConfig(processor2Name, spfsm.OperationalStateActive)

			fullCfg := config.FullConfig{
				StreamProcessor: []config.StreamProcessorConfig{cfg1, cfg2},
			}

			// Configure both processors
			fsmtest.ConfigureStreamProcessorManagerForState(mockService, processor1Name, spfsm.OperationalStateIdle)
			fsmtest.ConfigureStreamProcessorManagerForState(mockService, processor2Name, spfsm.OperationalStateIdle)

			// Wait for both to reach idle state
			newTick, err := fsmtest.WaitForStreamProcessorManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick}, manager, mockSvcRegistry,
				processor1Name,
				spfsm.OperationalStateIdle,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			newTick, err = fsmtest.WaitForStreamProcessorManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick}, manager, mockSvcRegistry,
				processor2Name,
				spfsm.OperationalStateIdle,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Remove processor1 from config
			fullCfg.StreamProcessor = []config.StreamProcessorConfig{cfg2}

			// Configure processor1 for stopped state before removal
			fsmtest.ConfigureStreamProcessorManagerForState(mockService, processor1Name, spfsm.OperationalStateStopped)

			// Wait for processor1 to be removed
			newTick, err = fsmtest.WaitForStreamProcessorManagerInstanceRemoval(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick},
				manager,
				mockSvcRegistry,
				fmt.Sprintf("streamprocessor-%s", processor1Name),
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Verify processor1 is gone but processor2 remains
			instances := manager.GetInstances()
			Expect(instances).To(HaveLen(1))
			Expect(instances).NotTo(HaveKey(fmt.Sprintf("streamprocessor-%s", processor1Name)))
			Expect(instances).To(HaveKey(fmt.Sprintf("streamprocessor-%s", processor2Name)))

			// Verify processor2 is still in idle state
			inst, exists := manager.GetInstance(fmt.Sprintf("streamprocessor-%s", processor2Name))
			Expect(exists).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(spfsm.OperationalStateIdle))
		})
	})

	// -------------------------------------------------------------------------
	//  CONFIGURATION UPDATES
	// -------------------------------------------------------------------------
	Context("Configuration Updates", func() {
		It("should handle configuration updates without recreation", func() {
			processorName := "test-config-update"
			initialCfg := fsmtest.CreateStreamProcessorTestConfig(processorName, spfsm.OperationalStateActive)

			fullCfg := config.FullConfig{
				StreamProcessor: []config.StreamProcessorConfig{initialCfg},
			}

			// Configure for transition to Idle
			fsmtest.ConfigureStreamProcessorManagerForState(mockService, processorName, spfsm.OperationalStateIdle)

			// Wait for idle state
			newTick, err := fsmtest.WaitForStreamProcessorManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick}, manager, mockSvcRegistry,
				processorName,
				spfsm.OperationalStateIdle,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Get the instance ID before update
			instancesBefore := manager.GetInstances()
			instanceKeyBefore := fmt.Sprintf("streamprocessor-%s", processorName)
			instanceBefore, exists := instancesBefore[instanceKeyBefore]
			Expect(exists).To(BeTrue())

			// Update configuration (modify some parameters)
			updatedCfg := initialCfg
			updatedCfg.StreamProcessorServiceConfig.Config.Sources = map[string]string{"updated-source": "updated-topic"}
			fullCfg.StreamProcessor = []config.StreamProcessorConfig{updatedCfg}

			// Apply the updated configuration
			newTick, err = fsmtest.WaitForStreamProcessorManagerStable(
				ctx, fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick}, manager, mockSvcRegistry,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Verify the instance still exists and is the same object (not recreated)
			instancesAfter := manager.GetInstances()
			instanceAfter, exists := instancesAfter[instanceKeyBefore]
			Expect(exists).To(BeTrue())
			Expect(instanceAfter).To(BeIdenticalTo(instanceBefore)) // Same object in memory
		})

		It("should handle processor name changes as removal + creation", func() {
			originalName := "test-processor-original"
			newName := "test-processor-renamed"

			// Create processor with original name
			originalCfg := fsmtest.CreateStreamProcessorTestConfig(originalName, spfsm.OperationalStateActive)
			fullCfg := config.FullConfig{
				StreamProcessor: []config.StreamProcessorConfig{originalCfg},
			}

			// Configure for transition to Idle
			fsmtest.ConfigureStreamProcessorManagerForState(mockService, originalName, spfsm.OperationalStateIdle)

			// Wait for idle state
			newTick, err := fsmtest.WaitForStreamProcessorManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick}, manager, mockSvcRegistry,
				originalName,
				spfsm.OperationalStateIdle,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Change processor name (simulates removal of old + creation of new)
			newCfg := fsmtest.CreateStreamProcessorTestConfig(newName, spfsm.OperationalStateActive)
			fullCfg.StreamProcessor = []config.StreamProcessorConfig{newCfg}

			// Configure for original processor to be stopped before removal
			fsmtest.ConfigureStreamProcessorManagerForState(mockService, originalName, spfsm.OperationalStateStopped)
			// Configure for new processor to reach idle
			fsmtest.ConfigureStreamProcessorManagerForState(mockService, newName, spfsm.OperationalStateIdle)

			// Wait for new processor to be created and reach idle state
			newTick, err = fsmtest.WaitForStreamProcessorManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick}, manager, mockSvcRegistry,
				newName,
				spfsm.OperationalStateIdle,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Verify old processor is gone and new processor exists
			instances := manager.GetInstances()
			Expect(instances).To(HaveLen(1))
			Expect(instances).NotTo(HaveKey(fmt.Sprintf("streamprocessor-%s", originalName)))
			Expect(instances).To(HaveKey(fmt.Sprintf("streamprocessor-%s", newName)))
		})
	})

	// -------------------------------------------------------------------------
	//  ERROR HANDLING
	// -------------------------------------------------------------------------
	Context("Error Handling", func() {
		It("should handle service creation failures gracefully", func() {
			processorName := "test-creation-failure"
			cfg := config.FullConfig{
				StreamProcessor: []config.StreamProcessorConfig{
					fsmtest.CreateStreamProcessorTestConfig(processorName, spfsm.OperationalStateActive),
				},
			}

			// Configure mock service to NOT exist and fail during creation
			mockService.ExistingComponents[processorName] = false
			mockService.AddToManagerError = fmt.Errorf("simulated creation failure")

			// Attempt to create the processor
			newTick, err := fsmtest.WaitForStreamProcessorManagerStable(
				ctx, fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick}, manager, mockSvcRegistry,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Verify instance was created and progressed to starting_redpanda state
			// Even though AddToManager failed, the mock service still sets up state, so FSM progresses
			instances := manager.GetInstances()
			Expect(instances).To(HaveLen(1))
			inst, exists := instances[fmt.Sprintf("streamprocessor-%s", processorName)]
			Expect(exists).To(BeTrue())
			// Should be in starting_redpanda state as FSM progresses despite creation error
			Expect(inst.GetCurrentFSMState()).To(Equal("starting_redpanda"))
		})

		It("should handle missing DFC dependencies gracefully", func() {
			processorName := "test-missing-dfc"
			cfg := config.FullConfig{
				StreamProcessor: []config.StreamProcessorConfig{
					fsmtest.CreateStreamProcessorTestConfigWithMissingDfc(processorName, spfsm.OperationalStateActive),
				},
			}

			// Attempt to create the processor with missing DFC
			newTick, err := fsmtest.WaitForStreamProcessorManagerStable(
				ctx, fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick}, manager, mockSvcRegistry,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Verify instance exists and continues reconciling despite config errors
			// With the new behavior, it transitions to operational states rather than
			// getting stuck in creating state
			instances := manager.GetInstances()
			Expect(instances).To(HaveLen(1))
			inst, exists := instances[fmt.Sprintf("streamprocessor-%s", processorName)]
			Expect(exists).To(BeTrue())
			// Should progress past creating state with the new reconciliation behavior
			currentState := inst.GetCurrentFSMState()
			Expect(currentState).ToNot(Equal("creating"))
			// It might be in stopped or starting_redpanda state
			Expect(currentState).To(Or(Equal("stopped"), Equal("starting_redpanda")))
		})

		It("should recover from temporary service failures", func() {
			processorName := "test-recovery"
			cfg := config.FullConfig{
				StreamProcessor: []config.StreamProcessorConfig{
					fsmtest.CreateStreamProcessorTestConfig(processorName, spfsm.OperationalStateActive),
				},
			}

			// Configure for transition to Idle initially
			fsmtest.ConfigureStreamProcessorManagerForState(mockService, processorName, spfsm.OperationalStateIdle)

			// Wait for idle state
			newTick, err := fsmtest.WaitForStreamProcessorManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick}, manager, mockSvcRegistry,
				processorName,
				spfsm.OperationalStateIdle,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Simulate service failure (transition to degraded)
			fsmtest.ConfigureStreamProcessorManagerForState(mockService, processorName, spfsm.OperationalStateDegradedDFC)

			// Wait for degraded state
			newTick, err = fsmtest.WaitForStreamProcessorManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick}, manager, mockSvcRegistry,
				processorName,
				spfsm.OperationalStateDegradedDFC,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Simulate recovery (transition back to idle)
			fsmtest.ConfigureStreamProcessorManagerForState(mockService, processorName, spfsm.OperationalStateIdle)

			// Wait for recovery to idle state
			newTick, err = fsmtest.WaitForStreamProcessorManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick}, manager, mockSvcRegistry,
				processorName,
				spfsm.OperationalStateIdle,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Verify instance is back to idle state
			instances := manager.GetInstances()
			inst, exists := instances[fmt.Sprintf("streamprocessor-%s", processorName)]
			Expect(exists).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(spfsm.OperationalStateIdle))
		})
	})
})
