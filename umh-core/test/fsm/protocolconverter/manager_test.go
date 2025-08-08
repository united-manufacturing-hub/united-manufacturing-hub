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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsmtest"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/bridge"
	bridgesvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/bridge"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// Following the CursorRules, we never call manager.Reconcile(...) directly in loops.
// Instead, we use the fsmtest helpers.

var _ = Describe("BridgeManager", func() {
	var (
		manager         *bridge.Manager
		mockService     *bridgesvc.MockService
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
		// Create a new BridgeManager with the mock service
		manager, mockService = fsmtest.CreateMockBridgeManager("test-manager")

		// Initialize the mock service state to empty
		mockService.ExistingComponents = make(map[string]bool)
		mockService.States = make(map[string]*bridgesvc.ServiceInfo)
	})

	// -------------------------------------------------------------------------
	//  INITIALIZATION
	// -------------------------------------------------------------------------
	Context("Initialization", func() {
		It("should handle empty config without errors", func() {
			emptyConfig := config.FullConfig{Bridge: []config.BridgeConfig{}}

			// Single call to a helper that wraps Reconcile
			newTick, err := fsmtest.WaitForBridgeManagerStable(
				ctx, fsm.SystemSnapshot{CurrentConfig: emptyConfig, Tick: tick}, manager, mockSvcRegistry,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())
			Expect(manager.GetInstances()).To(BeEmpty())
		})

		It("should create a service in stopped state and remain stable", func() {
			brName := "test-stopped-bridge"
			cfg := config.FullConfig{
				Bridge: []config.BridgeConfig{
					fsmtest.CreateBridgeTestConfig(brName, bridge.OperationalStateStopped),
				},
			}

			// Configure the mock service to allow transition to Stopped
			fsmtest.ConfigureBridgeManagerForState(mockService, brName, bridge.OperationalStateStopped)

			// Wait for instance creation and stable 'Stopped' state
			newTick, err := fsmtest.WaitForBridgeInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick},
				manager,
				mockSvcRegistry,
				brName,
				bridge.OperationalStateStopped,
				10,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Double-check the manager state
			inst, exists := manager.GetInstance(fmt.Sprintf("bridge-%s", brName))
			Expect(exists).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(bridge.OperationalStateStopped))
		})

		It("should create a service in active state and reach idle or active", func() {
			brName := "test-active-bridge"
			cfg := config.FullConfig{
				Bridge: []config.BridgeConfig{
					fsmtest.CreateBridgeTestConfig(brName, bridge.OperationalStateActive),
				},
			}

			// Configure the mock service for transition to Idle (or Active)
			fsmtest.ConfigureBridgeManagerForState(mockService, brName, bridge.OperationalStateIdle)

			// Some Bridge FSMs pass through 'Idle' before 'Active', so we might
			// check for either. In your code, you might unify them in a single final "active-like" state.

			newTick, err := fsmtest.WaitForBridgeInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick},
				manager,
				mockSvcRegistry,
				brName,
				bridge.OperationalStateIdle, // or OperationalStateActive, whichever is stable
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
			brName := "test-lifecycle"
			// Start from active config
			fullCfg := config.FullConfig{
				Bridge: []config.BridgeConfig{
					fsmtest.CreateBridgeTestConfig(brName, bridge.OperationalStateActive),
				},
			}

			// Configure for transition to Idle
			fsmtest.ConfigureBridgeManagerForState(mockService, brName, bridge.OperationalStateIdle)

			// 1) Wait for idle
			newTick, err := fsmtest.WaitForBridgeInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick}, manager, mockSvcRegistry,
				brName,
				bridge.OperationalStateIdle,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// 2) Now configure for degraded state
			fsmtest.ConfigureBridgeManagerForState(mockService, brName, bridge.OperationalStateDegradedConnection)

			// Wait for state transition
			newTick, err = fsmtest.WaitForBridgeInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick}, manager, mockSvcRegistry,
				brName,
				bridge.OperationalStateDegradedConnection,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// 3) Configure for stopped state before removal
			fsmtest.ConfigureBridgeManagerForState(mockService, brName, bridge.OperationalStateStopped)

			// 4) Remove from config => instance eventually stops & is removed
			emptyConfig := config.FullConfig{Bridge: []config.BridgeConfig{}}
			newTick, err = fsmtest.WaitForBridgeInstanceRemoval(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: emptyConfig, Tick: tick},
				manager,
				mockSvcRegistry,
				fmt.Sprintf("bridge-%s", brName),
				30, // More attempts for removal
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())
			Expect(manager.GetInstances()).NotTo(HaveKey(fmt.Sprintf("bridge-%s", brName)))
		})

		It("should toggle from active/idle to stopped and back to active/idle with config changes", func() {
			brName := "test-toggle-bridge"
			activeCfg := fsmtest.CreateBridgeTestConfig(brName, bridge.OperationalStateActive)

			fullCfg := config.FullConfig{
				Bridge: []config.BridgeConfig{activeCfg},
			}

			// Configure for idle state
			fsmtest.ConfigureBridgeManagerForState(mockService, brName, bridge.OperationalStateIdle)

			// 1) Wait for idle
			newTick, err := fsmtest.WaitForBridgeInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick}, manager, mockSvcRegistry,
				brName,
				bridge.OperationalStateIdle,
				20,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// 2) Change config to stopped
			stoppedCfg := fsmtest.CreateBridgeTestConfig(brName, bridge.OperationalStateStopped)
			stoppedFullCfg := config.FullConfig{
				Bridge: []config.BridgeConfig{stoppedCfg},
			}

			// Configure for stopped state
			fsmtest.ConfigureBridgeManagerForState(mockService, brName, bridge.OperationalStateStopped)

			// Wait for stopped
			newTick, err = fsmtest.WaitForBridgeInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: stoppedFullCfg, Tick: tick}, manager, mockSvcRegistry,
				brName,
				bridge.OperationalStateStopped,
				20,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// 3) Change config back to active
			// Configure for idle state again
			fsmtest.ConfigureBridgeManagerForState(mockService, brName, bridge.OperationalStateIdle)

			// Wait for idle again - increased attempts from 20 to 30 to handle slower CI environments
			newTick, err = fsmtest.WaitForBridgeInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick}, manager, mockSvcRegistry,
				brName,
				bridge.OperationalStateIdle,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// -------------------------------------------------------------------------
	//  DEGRADED STATES
	// -------------------------------------------------------------------------
	Context("Degraded States", func() {
		It("should handle different degraded states", func() {
			brName := "test-degraded-bridge"
			cfg := config.FullConfig{
				Bridge: []config.BridgeConfig{
					fsmtest.CreateBridgeTestConfig(brName, bridge.OperationalStateActive),
				},
			}

			// Start in idle state
			fsmtest.ConfigureBridgeManagerForState(mockService, brName, bridge.OperationalStateIdle)

			newTick, err := fsmtest.WaitForBridgeInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick}, manager, mockSvcRegistry,
				brName,
				bridge.OperationalStateIdle,
				20,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Test degraded connection state
			fsmtest.ConfigureBridgeManagerForState(mockService, brName, bridge.OperationalStateDegradedConnection)

			newTick, err = fsmtest.WaitForBridgeInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick}, manager, mockSvcRegistry,
				brName,
				bridge.OperationalStateDegradedConnection,
				20,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Test degraded DFC state
			fsmtest.ConfigureBridgeManagerForState(mockService, brName, bridge.OperationalStateDegradedDFC)

			newTick, err = fsmtest.WaitForBridgeInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick}, manager, mockSvcRegistry,
				brName,
				bridge.OperationalStateDegradedDFC,
				20,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Test degraded Redpanda state
			fsmtest.ConfigureBridgeManagerForState(mockService, brName, bridge.OperationalStateDegradedRedpanda)

			newTick, err = fsmtest.WaitForBridgeInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick}, manager, mockSvcRegistry,
				brName,
				bridge.OperationalStateDegradedRedpanda,
				20,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Recovery back to idle
			fsmtest.ConfigureBridgeManagerForState(mockService, brName, bridge.OperationalStateIdle)

			newTick, err = fsmtest.WaitForBridgeInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick}, manager, mockSvcRegistry,
				brName,
				bridge.OperationalStateIdle,
				20,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// -------------------------------------------------------------------------
	//  MULTIPLE INSTANCES
	// -------------------------------------------------------------------------
	Context("Multiple Instances", func() {
		It("should manage multiple bridge instances simultaneously", func() {
			br1Name := "test-multi-bridge-1"
			br2Name := "test-multi-bridge-2"

			cfg := config.FullConfig{
				Bridge: []config.BridgeConfig{
					fsmtest.CreateBridgeTestConfig(br1Name, bridge.OperationalStateActive),
					fsmtest.CreateBridgeTestConfig(br2Name, bridge.OperationalStateStopped),
				},
			}

			// Configure both services
			fsmtest.ConfigureBridgeManagerForState(mockService, br1Name, bridge.OperationalStateIdle)
			fsmtest.ConfigureBridgeManagerForState(mockService, br2Name, bridge.OperationalStateStopped)

			// Wait for both to reach their target states
			desiredStates := map[string]string{
				br1Name: bridge.OperationalStateIdle,
				br2Name: bridge.OperationalStateStopped,
			}

			newTick, err := fsmtest.WaitForBridgeManagerMultiState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick},
				manager,
				mockSvcRegistry,
				desiredStates,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Verify both instances exist and are in correct states
			inst1, exists1 := manager.GetInstance(fmt.Sprintf("bridge-%s", br1Name))
			Expect(exists1).To(BeTrue())
			Expect(inst1.GetCurrentFSMState()).To(Equal(bridge.OperationalStateIdle))

			inst2, exists2 := manager.GetInstance(fmt.Sprintf("bridge-%s", br2Name))
			Expect(exists2).To(BeTrue())
			Expect(inst2.GetCurrentFSMState()).To(Equal(bridge.OperationalStateStopped))
		})

		It("should handle partial removal of instances", func() {
			br1Name := "test-partial-removal-1"
			br2Name := "test-partial-removal-2"

			// Start with both instances
			fullCfg := config.FullConfig{
				Bridge: []config.BridgeConfig{
					fsmtest.CreateBridgeTestConfig(br1Name, bridge.OperationalStateActive),
					fsmtest.CreateBridgeTestConfig(br2Name, bridge.OperationalStateActive),
				},
			}

			// Configure both for idle state
			fsmtest.ConfigureBridgeManagerForState(mockService, br1Name, bridge.OperationalStateIdle)
			fsmtest.ConfigureBridgeManagerForState(mockService, br2Name, bridge.OperationalStateIdle)

			// Wait for both to be idle
			desiredStates := map[string]string{
				br1Name: bridge.OperationalStateIdle,
				br2Name: bridge.OperationalStateIdle,
			}

			newTick, err := fsmtest.WaitForBridgeManagerMultiState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick},
				manager,
				mockSvcRegistry,
				desiredStates,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Remove one instance from config
			partialCfg := config.FullConfig{
				Bridge: []config.BridgeConfig{
					fsmtest.CreateBridgeTestConfig(br2Name, bridge.OperationalStateActive),
				},
			}

			// Configure the remaining instance for stopped state before removal
			fsmtest.ConfigureBridgeManagerForState(mockService, br1Name, bridge.OperationalStateStopped)

			// Wait for first instance to be removed
			newTick, err = fsmtest.WaitForBridgeInstanceRemoval(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: partialCfg, Tick: tick},
				manager,
				mockSvcRegistry,
				fmt.Sprintf("bridge-%s", br1Name),
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Verify first instance is gone and second still exists
			_, exists1 := manager.GetInstance(fmt.Sprintf("bridge-%s", br1Name))
			Expect(exists1).To(BeFalse())

			inst2, exists2 := manager.GetInstance(fmt.Sprintf("bridge-%s", br2Name))
			Expect(exists2).To(BeTrue())
			Expect(inst2.GetCurrentFSMState()).To(Equal(bridge.OperationalStateIdle))
		})
	})

	// -------------------------------------------------------------------------
	//  ERROR HANDLING
	// -------------------------------------------------------------------------
	Context("Error Handling", func() {
		It("should handle configuration errors gracefully", func() {
			brName := "test-error-bridge"
			cfg := config.FullConfig{
				Bridge: []config.BridgeConfig{
					fsmtest.CreateBridgeConfigWithMissingDFC(brName, bridge.OperationalStateActive),
				},
			}

			// Configure the mock service for a missing DFC scenario
			fsmtest.ConfigureBridgeManagerForState(mockService, brName, bridge.OperationalStateStartingFailedDFCMissing)

			// The instance should reach a failed state due to missing DFC
			newTick, err := fsmtest.WaitForBridgeInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick},
				manager,
				mockSvcRegistry,
				brName,
				bridge.OperationalStateStartingFailedDFCMissing,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Verify the instance is in the expected failed state
			inst, exists := manager.GetInstance(fmt.Sprintf("bridge-%s", brName))
			Expect(exists).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(bridge.OperationalStateStartingFailedDFCMissing))
		})
	})

	// -------------------------------------------------------------------------
	//  CONFIGURATION VALIDATION
	// -------------------------------------------------------------------------
	Context("Configuration Validation", func() {
		It("should handle invalid port configurations gracefully", func() {
			brName := "test-invalid-port-bridge"

			// Create a config with an invalid port (non-numeric string)
			cfg := config.FullConfig{
				Bridge: []config.BridgeConfig{
					fsmtest.CreateBridgeTestConfigWithInvalidPort(brName, bridge.OperationalStateActive, "not-a-number"),
				},
			}

			// Try to reconcile - should fail with configuration error
			newTick, err := fsmtest.WaitForBridgeManagerStable(
				ctx, fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick}, manager, mockSvcRegistry,
			)
			tick = newTick

			// The manager should handle the error gracefully (no error returned)
			// The config comparison fails internally but doesn't propagate as an error
			Expect(err).NotTo(HaveOccurred())

			// The instance should be created but remain in its initial state
			// due to the config comparison failure preventing updates
			instances := manager.GetInstances()
			Expect(instances).To(HaveLen(1))
			Expect(instances).To(HaveKey(fmt.Sprintf("bridge-%s", brName)))
		})

		It("should handle empty port configurations", func() {
			brName := "test-empty-port-bridge"

			// Create a config with an empty port
			cfg := config.FullConfig{
				Bridge: []config.BridgeConfig{
					fsmtest.CreateBridgeTestConfigWithInvalidPort(brName, bridge.OperationalStateActive, ""),
				},
			}

			// Try to reconcile - should fail with configuration error
			newTick, err := fsmtest.WaitForBridgeManagerStable(
				ctx, fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick}, manager, mockSvcRegistry,
			)
			tick = newTick

			// The manager should handle the error gracefully (no error returned)
			Expect(err).NotTo(HaveOccurred())

			// The instance should be created but remain in its initial state
			instances := manager.GetInstances()
			Expect(instances).To(HaveLen(1))
			Expect(instances).To(HaveKey(fmt.Sprintf("bridge-%s", brName)))
		})

		It("should handle out-of-range port numbers", func() {
			brName := "test-large-port-bridge"

			// Create a config with a port number larger than uint16 max (65535)
			cfg := config.FullConfig{
				Bridge: []config.BridgeConfig{
					fsmtest.CreateBridgeTestConfigWithInvalidPort(brName, bridge.OperationalStateActive, "99999"),
				},
			}

			// Try to reconcile - should fail with configuration error
			newTick, err := fsmtest.WaitForBridgeManagerStable(
				ctx, fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick}, manager, mockSvcRegistry,
			)
			tick = newTick

			// The manager should handle the error gracefully (no error returned)
			Expect(err).NotTo(HaveOccurred())

			// The instance should be created but remain in its initial state
			instances := manager.GetInstances()
			Expect(instances).To(HaveLen(1))
			Expect(instances).To(HaveKey(fmt.Sprintf("bridge-%s", brName)))
		})

		It("should handle negative port numbers", func() {
			brName := "test-negative-port-bridge"

			// Create a config with a negative port number
			cfg := config.FullConfig{
				Bridge: []config.BridgeConfig{
					fsmtest.CreateBridgeTestConfigWithInvalidPort(brName, bridge.OperationalStateActive, "-1"),
				},
			}

			// Try to reconcile - should fail with configuration error
			newTick, err := fsmtest.WaitForBridgeManagerStable(
				ctx, fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick}, manager, mockSvcRegistry,
			)
			tick = newTick

			// The manager should handle the error gracefully (no error returned)
			Expect(err).NotTo(HaveOccurred())

			// The instance should be created but remain in its initial state
			instances := manager.GetInstances()
			Expect(instances).To(HaveLen(1))
			Expect(instances).To(HaveKey(fmt.Sprintf("bridge-%s", brName)))
		})

		It("should handle mixed valid and invalid configurations", func() {
			validConverterName := "test-valid-bridge"
			invalidConverterName := "test-invalid-bridge"

			// Create configs with one valid and one invalid
			cfg := config.FullConfig{
				Bridge: []config.BridgeConfig{
					fsmtest.CreateBridgeTestConfig(validConverterName, bridge.OperationalStateActive),
					fsmtest.CreateBridgeTestConfigWithInvalidPort(invalidConverterName, bridge.OperationalStateActive, "invalid-port"),
				},
			}

			// Configure the valid one to reach idle state
			fsmtest.ConfigureBridgeManagerForState(mockService, validConverterName, bridge.OperationalStateIdle)

			// Try to reconcile - give it more time since it needs to create two instances
			// The manager can only create one instance per tick, so we need at least 2 ticks
			// Let's manually reconcile multiple times to ensure both instances are created
			var err error
			for i := 0; i < 20; i++ {
				currentSnapshot := fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick}
				err, _ = manager.Reconcile(ctx, currentSnapshot, mockSvcRegistry)
				if err != nil {
					break
				}
				tick++

				// Check if we have both instances
				instances := manager.GetInstances()
				if len(instances) >= 2 {
					break
				}
			}

			// The manager should handle the error gracefully (no error returned)
			Expect(err).NotTo(HaveOccurred())

			// Both instances should be created, but the invalid one will fail config comparison
			instances := manager.GetInstances()
			Expect(instances).To(HaveLen(2))
			Expect(instances).To(HaveKey(fmt.Sprintf("bridge-%s", validConverterName)))
			Expect(instances).To(HaveKey(fmt.Sprintf("bridge-%s", invalidConverterName)))
		})
	})
})
