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
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second) // we need to have a deadline as the reconcile logic in the base fsm manager requires it
		tick = 0
		mockSvcRegistry = serviceregistry.NewMockRegistry()
		// Create a new TopicBrowserManager with the mock service
		manager, mockService = fsmtest.CreateMockTopicBrowserManager("test-manager")

		// Initialize the mock service state to empty
		mockService.Existing = make(map[string]bool)
		mockService.States = make(map[string]*topicbrowsersvc.ServiceInfo)
	})

	// -------------------------------------------------------------------------
	//  INITIALIZATION
	// -------------------------------------------------------------------------
	Context("Initialization", func() {
		It("should handle empty config without errors", func() {
			emptyConfig := config.FullConfig{Internal: config.InternalConfig{}}

			// Single call to a helper that wraps Reconcile
			newTick, err := fsmtest.WaitForTopicBrowserManagerStable(
				ctx, fsm.SystemSnapshot{CurrentConfig: emptyConfig, Tick: tick}, manager, mockSvcRegistry,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())
			Expect(manager.GetInstances()).To(BeEmpty())
		})

		It("should create a service in stopped state and remain stable", func() {
			tbName := "test-stopped-tb"
			cfg := config.FullConfig{
				Internal: config.InternalConfig{
					TopicBrowser: fsmtest.CreateTopicbrowserTestConfig(tbName, topicbrowser.OperationalStateStopped),
				},
			}

			// Configure the mock service to allow transition to Stopped
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, tbName, topicbrowser.OperationalStateStopped)

			// Wait for instance creation and stable 'Stopped' state
			newTick, err := fsmtest.WaitForTopicBrowserManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick},
				manager,
				mockSvcRegistry,
				tbName,
				topicbrowser.OperationalStateStopped,
				10,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Double-check the manager state
			inst, exists := manager.GetInstance(tbName)
			Expect(exists).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(topicbrowser.OperationalStateStopped))
		})

		It("should create a service in active state and reach active", func() {
			tbName := "test-active-tb"
			cfg := config.FullConfig{
				Internal: config.InternalConfig{
					TopicBrowser: fsmtest.CreateTopicbrowserTestConfig(tbName, topicbrowser.OperationalStateActive),
				},
			}

			// Configure the mock service for transition to Active
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, tbName, topicbrowser.OperationalStateActive)

			newTick, err := fsmtest.WaitForTopicBrowserManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick},
				manager,
				mockSvcRegistry,
				tbName,
				topicbrowser.OperationalStateActive,
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
		It("should go from creation → active → degrade → removal", func() {
			tbName := "test-lifecycle"
			// Start from active config
			fullCfg := config.FullConfig{
				Internal: config.InternalConfig{
					TopicBrowser: fsmtest.CreateTopicbrowserTestConfig(tbName, topicbrowser.OperationalStateActive),
				},
			}

			// Configure for transition to Active
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, tbName, topicbrowser.OperationalStateActive)

			// 1) Wait for active
			newTick, err := fsmtest.WaitForTopicBrowserManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick}, manager, mockSvcRegistry,
				tbName,
				topicbrowser.OperationalStateActive,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// 2) Now configure for degraded state
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, tbName, topicbrowser.OperationalStateDegraded)

			// Wait for state transition
			newTick, err = fsmtest.WaitForTopicBrowserManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick}, manager, mockSvcRegistry,
				tbName,
				topicbrowser.OperationalStateDegraded,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// 3) Configure for stopped state before removal
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, tbName, topicbrowser.OperationalStateStopped)

			// 4) Remove from config => instance eventually stops & is removed
			emptyConfig := config.FullConfig{Internal: config.InternalConfig{}}
			newTick, err = fsmtest.WaitForTopicBrowserManagerInstanceRemoval(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: emptyConfig, Tick: tick},
				manager,
				mockSvcRegistry,
				tbName,
				30, // More attempts for removal
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())
			Expect(manager.GetInstances()).NotTo(HaveKey(tbName))
		})

		It("should toggle from active to stopped and back to active with config changes", func() {
			tbName := "test-toggle-tb"
			activeCfg := fsmtest.CreateTopicbrowserTestConfig(tbName, topicbrowser.OperationalStateActive)

			fullCfg := config.FullConfig{
				Internal: config.InternalConfig{
					TopicBrowser: activeCfg,
				},
			}

			// Configure for active state
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, tbName, topicbrowser.OperationalStateActive)

			// 1) Wait for active
			newTick, err := fsmtest.WaitForTopicBrowserManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick}, manager, mockSvcRegistry,
				tbName,
				topicbrowser.OperationalStateActive,
				20,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// 2) Change config to stopped
			stoppedCfg := fsmtest.CreateTopicbrowserTestConfig(tbName, topicbrowser.OperationalStateStopped)
			stoppedFullCfg := config.FullConfig{
				Internal: config.InternalConfig{
					TopicBrowser: stoppedCfg,
				},
			}

			// Configure for stopped state
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, tbName, topicbrowser.OperationalStateStopped)

			// Wait for stopped
			newTick, err = fsmtest.WaitForTopicBrowserManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: stoppedFullCfg, Tick: tick}, manager, mockSvcRegistry,
				tbName,
				topicbrowser.OperationalStateStopped,
				20,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// 3) Change config back to active
			// Configure for active state again
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, tbName, topicbrowser.OperationalStateActive)

			// Wait for active again - increased attempts from 20 to 30 to handle slower CI environments
			newTick, err = fsmtest.WaitForTopicBrowserManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick}, manager, mockSvcRegistry,
				tbName,
				topicbrowser.OperationalStateActive,
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
		It("should handle degraded state transitions", func() {
			tbName := "test-degraded-tb"
			cfg := config.FullConfig{
				Internal: config.InternalConfig{
					TopicBrowser: fsmtest.CreateTopicbrowserTestConfig(tbName, topicbrowser.OperationalStateActive),
				},
			}

			// Start in active state
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, tbName, topicbrowser.OperationalStateActive)

			newTick, err := fsmtest.WaitForTopicBrowserManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick}, manager, mockSvcRegistry,
				tbName,
				topicbrowser.OperationalStateActive,
				20,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Test degraded state
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, tbName, topicbrowser.OperationalStateDegraded)

			newTick, err = fsmtest.WaitForTopicBrowserManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick}, manager, mockSvcRegistry,
				tbName,
				topicbrowser.OperationalStateDegraded,
				20,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Recovery back to active
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, tbName, topicbrowser.OperationalStateActive)

			newTick, err = fsmtest.WaitForTopicBrowserManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick}, manager, mockSvcRegistry,
				tbName,
				topicbrowser.OperationalStateActive,
				20,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// -------------------------------------------------------------------------
	//  ERROR HANDLING
	// -------------------------------------------------------------------------
	Context("Error Handling", func() {
		It("should handle service errors gracefully", func() {
			tbName := "test-error-tb"
			cfg := config.FullConfig{
				Internal: config.InternalConfig{
					TopicBrowser: fsmtest.CreateTopicbrowserTestConfig(tbName, topicbrowser.OperationalStateActive),
				},
			}

			// Configure the mock service for an error scenario
			mockService.AddToManagerError = fmt.Errorf("mock service error")

			// Try to reconcile - the manager should handle errors gracefully
			newTick, err := fsmtest.WaitForTopicBrowserManagerStable(
				ctx, fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick}, manager, mockSvcRegistry,
			)
			tick = newTick

			// The manager should handle the error gracefully and the operation should eventually succeed
			// Reset the error and try again
			mockService.AddToManagerError = nil
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, tbName, topicbrowser.OperationalStateActive)

			newTick, err = fsmtest.WaitForTopicBrowserManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick},
				manager,
				mockSvcRegistry,
				tbName,
				topicbrowser.OperationalStateActive,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// -------------------------------------------------------------------------
	//  CONFIGURATION VALIDATION
	// -------------------------------------------------------------------------
	Context("Configuration Validation", func() {
		It("should handle config changes properly", func() {
			tbName := "test-config-tb"

			// Create initial config
			cfg := config.FullConfig{
				Internal: config.InternalConfig{
					TopicBrowser: fsmtest.CreateTopicbrowserTestConfig(tbName, topicbrowser.OperationalStateActive),
				},
			}

			// Configure for active state
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, tbName, topicbrowser.OperationalStateActive)

			// Wait for the instance to be created and active
			newTick, err := fsmtest.WaitForTopicBrowserManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick},
				manager,
				mockSvcRegistry,
				tbName,
				topicbrowser.OperationalStateActive,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Verify the instance exists and is in the expected state
			inst, exists := manager.GetInstance(tbName)
			Expect(exists).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(topicbrowser.OperationalStateActive))
		})

		It("should handle empty name gracefully", func() {
			// Create config with empty name
			cfg := config.FullConfig{
				Internal: config.InternalConfig{
					TopicBrowser: fsmtest.CreateTopicbrowserTestConfig("", topicbrowser.OperationalStateActive),
				},
			}

			// Try to reconcile - should handle gracefully without errors
			newTick, err := fsmtest.WaitForTopicBrowserManagerStable(
				ctx, fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick}, manager, mockSvcRegistry,
			)
			tick = newTick

			// The manager should handle this gracefully
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
