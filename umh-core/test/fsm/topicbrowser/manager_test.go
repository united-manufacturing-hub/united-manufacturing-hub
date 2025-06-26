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

package topicbrowser_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsmtest"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/topicbrowser"
	topicbrowsersvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// Following the CursorRules, we never call manager.Reconcile(...) directly in loops.
// Instead, we use the fsmtest helpers.

var _ = Describe("TopicBrowserManager", func() {
	var (
		manager         *topicbrowser.Manager
		mockService     *topicbrowsersvc.MockService
		ctx             context.Context
		tick            uint64
		cancel          context.CancelFunc
		mockSvcRegistry *serviceregistry.Registry
	)

	AfterEach(func() {
		cancel()
	})

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Minute) // we need to have a deadline as the reconcile logic in the base fsm manager requires it
		tick = 0
		mockSvcRegistry = serviceregistry.NewMockRegistry()
		// Create a new TopicBrowserManager with the mock service
		manager, mockService = fsmtest.CreateMockTopicBrowserManager("test-manager")

		// Initialize the mock service state to empty - using correct field names
		mockService.Existing = make(map[string]bool)
		mockService.States = make(map[string]*topicbrowsersvc.ServiceInfo)
	})

	// -------------------------------------------------------------------------
	//  INITIALIZATION
	// -------------------------------------------------------------------------
	Context("Initialization", func() {
		It("should handle empty config without errors", func() {
			emptyConfig := config.FullConfig{Internal: config.InternalConfig{TopicBrowser: config.TopicBrowserConfig{}}}

			// Single call to a helper that wraps Reconcile
			newTick, err := fsmtest.WaitForTopicBrowserManagerStable(
				ctx, fsm.SystemSnapshot{CurrentConfig: emptyConfig, Tick: tick}, manager, mockSvcRegistry,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())
			Expect(manager.GetInstances()).To(BeEmpty())
		})

		It("should create a service in stopped state and remain stable", func() {
			converterName := "topic-browser"
			cfg := config.FullConfig{
				Internal: config.InternalConfig{
					TopicBrowser: fsmtest.CreateTopicBrowserTestConfig(converterName, topicbrowser.OperationalStateStopped),
				},
			}

			// Configure the mock service to allow transition to Stopped
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, converterName, topicbrowser.OperationalStateStopped)

			// Wait for instance creation and stable 'Stopped' state
			newTick, err := fsmtest.WaitForTopicBrowserManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick},
				manager,
				mockSvcRegistry,
				converterName,
				topicbrowser.OperationalStateStopped,
				10,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Double-check the manager state
			inst, exists := manager.GetInstance(fmt.Sprintf("topicbrowser-%s", converterName))
			Expect(exists).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(topicbrowser.OperationalStateStopped))
		})

		FIt("should create a service in active state and reach idle or active", func() {
			converterName := "topic-browser"
			cfg := config.FullConfig{
				Internal: config.InternalConfig{
					TopicBrowser: fsmtest.CreateTopicBrowserTestConfig(converterName, topicbrowser.OperationalStateActive),
				},
			}

			// Configure the mock service for transition to Idle (or Active)
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, converterName, topicbrowser.OperationalStateIdle)

			// Some TopicBrowser FSMs pass through 'Idle' before 'Active', so we might
			// check for either. In your code, you might unify them in a single final "active-like" state.

			newTick, err := fsmtest.WaitForTopicBrowserManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick},
				manager,
				mockSvcRegistry,
				converterName,
				topicbrowser.OperationalStateIdle, // or OperationalStateActive, whichever is stable
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
			converterName := "topic-browser"
			// Start from active config
			fullCfg := config.FullConfig{
				Internal: config.InternalConfig{
					TopicBrowser: fsmtest.CreateTopicBrowserTestConfig(converterName, topicbrowser.OperationalStateActive),
				},
			}

			// Configure for transition to Idle
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, converterName, topicbrowser.OperationalStateIdle)

			// 1) Wait for idle
			newTick, err := fsmtest.WaitForTopicBrowserManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick}, manager, mockSvcRegistry,
				converterName,
				topicbrowser.OperationalStateIdle,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// 2) Now configure for degraded benthos state
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, converterName, topicbrowser.OperationalStateDegradedBenthos)

			// Wait for state transition
			newTick, err = fsmtest.WaitForTopicBrowserManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick}, manager, mockSvcRegistry,
				converterName,
				topicbrowser.OperationalStateDegradedBenthos,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// 3) Configure for stopped state before removal
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, converterName, topicbrowser.OperationalStateStopped)

			// 4) Remove from config => instance eventually stops & is removed
			emptyConfig := config.FullConfig{Internal: config.InternalConfig{}}
			newTick, err = fsmtest.WaitForTopicBrowserManagerInstanceRemoval(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: emptyConfig, Tick: tick},
				manager,
				mockSvcRegistry,
				fmt.Sprintf("topicbrowser-%s", converterName),
				30, // More attempts for removal
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())
			Expect(manager.GetInstances()).NotTo(HaveKey(fmt.Sprintf("topicbrowser-%s", converterName)))
		})

		It("should toggle from active/idle to stopped and back to active/idle with config changes", func() {
			converterName := "topic-browser"
			activeCfg := fsmtest.CreateTopicBrowserTestConfig(converterName, topicbrowser.OperationalStateActive)

			fullCfg := config.FullConfig{
				Internal: config.InternalConfig{
					TopicBrowser: activeCfg,
				},
			}

			// Configure for idle state
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, converterName, topicbrowser.OperationalStateIdle)

			// 1) Wait for idle
			newTick, err := fsmtest.WaitForTopicBrowserManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick}, manager, mockSvcRegistry,
				converterName,
				topicbrowser.OperationalStateIdle,
				20,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// 2) Change config to stopped
			stoppedCfg := fsmtest.CreateTopicBrowserTestConfig(converterName, topicbrowser.OperationalStateStopped)
			stoppedFullCfg := config.FullConfig{
				Internal: config.InternalConfig{
					TopicBrowser: stoppedCfg,
				},
			}

			// Configure for stopped state
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, converterName, topicbrowser.OperationalStateStopped)

			// Wait for stopped
			newTick, err = fsmtest.WaitForTopicBrowserManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: stoppedFullCfg, Tick: tick}, manager, mockSvcRegistry,
				converterName,
				topicbrowser.OperationalStateStopped,
				20,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// 3) Change config back to active
			// Configure for idle state again
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, converterName, topicbrowser.OperationalStateIdle)

			// Wait for idle again - increased attempts from 20 to 30 to handle slower CI environments
			newTick, err = fsmtest.WaitForTopicBrowserManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick}, manager, mockSvcRegistry,
				converterName,
				topicbrowser.OperationalStateIdle,
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
			converterName := "topic-browser"
			cfg := config.FullConfig{
				Internal: config.InternalConfig{
					TopicBrowser: fsmtest.CreateTopicBrowserTestConfig(converterName, topicbrowser.OperationalStateActive),
				},
			}

			// Start in idle state
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, converterName, topicbrowser.OperationalStateIdle)

			newTick, err := fsmtest.WaitForTopicBrowserManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick}, manager, mockSvcRegistry,
				converterName,
				topicbrowser.OperationalStateIdle,
				20,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Test degraded benthos state
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, converterName, topicbrowser.OperationalStateDegradedBenthos)

			newTick, err = fsmtest.WaitForTopicBrowserManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick}, manager, mockSvcRegistry,
				converterName,
				topicbrowser.OperationalStateDegradedBenthos,
				20,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Test degraded Redpanda state
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, converterName, topicbrowser.OperationalStateDegradedRedpanda)

			newTick, err = fsmtest.WaitForTopicBrowserManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick}, manager, mockSvcRegistry,
				converterName,
				topicbrowser.OperationalStateDegradedRedpanda,
				20,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Recovery back to idle
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, converterName, topicbrowser.OperationalStateIdle)

			newTick, err = fsmtest.WaitForTopicBrowserManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick}, manager, mockSvcRegistry,
				converterName,
				topicbrowser.OperationalStateIdle,
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
		It("should manage multiple protocol converter instances simultaneously", func() {
			converter1Name := "test-multi-converter-1"
			converter2Name := "test-multi-converter-2"

			cfg := config.FullConfig{
				Internal: config.InternalConfig{
					TopicBrowser: fsmtest.CreateTopicBrowserTestConfig(converter1Name, topicbrowser.OperationalStateActive),
				},
			}

			// Configure both services
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, converter1Name, topicbrowser.OperationalStateIdle)
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, converter2Name, topicbrowser.OperationalStateStopped)

			// Wait for both to reach their target states
			desiredStates := map[string]string{
				converter1Name: topicbrowser.OperationalStateIdle,
				converter2Name: topicbrowser.OperationalStateStopped,
			}

			newTick, err := fsmtest.WaitForTopicBrowserManagerMultiState(
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
			inst1, exists1 := manager.GetInstance(fmt.Sprintf("topicbrowser-%s", converter1Name))
			Expect(exists1).To(BeTrue())
			Expect(inst1.GetCurrentFSMState()).To(Equal(topicbrowser.OperationalStateIdle))

			inst2, exists2 := manager.GetInstance(fmt.Sprintf("topicbrowser-%s", converter2Name))
			Expect(exists2).To(BeTrue())
			Expect(inst2.GetCurrentFSMState()).To(Equal(topicbrowser.OperationalStateStopped))
		})

		It("should handle partial removal of instances", func() {
			converter1Name := "test-partial-removal-1"
			converter2Name := "test-partial-removal-2"

			// Start with both instances
			fullCfg := config.FullConfig{
				Internal: config.InternalConfig{
					TopicBrowser: fsmtest.CreateTopicBrowserTestConfig(converter1Name, topicbrowser.OperationalStateActive),
				},
			}

			// Configure both for idle state
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, converter1Name, topicbrowser.OperationalStateIdle)
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, converter2Name, topicbrowser.OperationalStateIdle)

			// Wait for both to be idle
			desiredStates := map[string]string{
				converter1Name: topicbrowser.OperationalStateIdle,
				converter2Name: topicbrowser.OperationalStateIdle,
			}

			newTick, err := fsmtest.WaitForTopicBrowserManagerMultiState(
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
				Internal: config.InternalConfig{
					TopicBrowser: fsmtest.CreateTopicBrowserTestConfig(converter2Name, topicbrowser.OperationalStateActive),
				},
			}

			// Configure the remaining instance for stopped state before removal
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, converter1Name, topicbrowser.OperationalStateStopped)

			// Wait for first instance to be removed
			newTick, err = fsmtest.WaitForTopicBrowserManagerInstanceRemoval(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: partialCfg, Tick: tick},
				manager,
				mockSvcRegistry,
				fmt.Sprintf("topicbrowser-%s", converter1Name),
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Verify first instance is gone and second still exists
			_, exists1 := manager.GetInstance(fmt.Sprintf("topicbrowser-%s", converter1Name))
			Expect(exists1).To(BeFalse())

			inst2, exists2 := manager.GetInstance(fmt.Sprintf("topicbrowser-%s", converter2Name))
			Expect(exists2).To(BeTrue())
			Expect(inst2.GetCurrentFSMState()).To(Equal(topicbrowser.OperationalStateIdle))
		})
	})

	// -------------------------------------------------------------------------
	//  ERROR HANDLING
	// -------------------------------------------------------------------------
	Context("Error Handling", func() {
		It("should handle basic error scenarios gracefully", func() {
			converterName := "topic-browser"
			cfg := config.FullConfig{
				Internal: config.InternalConfig{
					TopicBrowser: fsmtest.CreateTopicBrowserTestConfig(converterName, topicbrowser.OperationalStateActive),
				},
			}

			// Configure the mock service for a starting state
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, converterName, topicbrowser.OperationalStateStarting)

			// The instance should reach the starting state
			newTick, err := fsmtest.WaitForTopicBrowserManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick},
				manager,
				mockSvcRegistry,
				converterName,
				topicbrowser.OperationalStateStarting,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Verify the instance is in the expected state
			inst, exists := manager.GetInstance(fmt.Sprintf("topicbrowser-%s", converterName))
			Expect(exists).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(topicbrowser.OperationalStateStarting))
		})
	})

	// -------------------------------------------------------------------------
	//  CONFIGURATION VALIDATION
	// -------------------------------------------------------------------------
	Context("Configuration Validation", func() {
		It("should handle basic configuration changes", func() {
			converterName := "topic-browser"

			// Create a basic config
			cfg := config.FullConfig{
				Internal: config.InternalConfig{
					TopicBrowser: fsmtest.CreateTopicBrowserTestConfig(converterName, topicbrowser.OperationalStateActive),
				},
			}

			// Try to reconcile
			newTick, err := fsmtest.WaitForTopicBrowserManagerStable(
				ctx, fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick}, manager, mockSvcRegistry,
			)
			tick = newTick

			// The manager should handle this gracefully
			Expect(err).NotTo(HaveOccurred())

			// The instance should be created
			instances := manager.GetInstances()
			Expect(instances).To(HaveLen(1))
			Expect(instances).To(HaveKey(fmt.Sprintf("topicbrowser-%s", converterName)))
		})
	})
})
