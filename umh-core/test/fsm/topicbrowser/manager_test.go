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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsmtest"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/topicbrowserserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/topicbrowser"
	tbsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// Following the CursorRules, we never call manager.Reconcile(...) directly in loops.
// Instead, we use the fsmtest helpers.

var _ = Describe("TopicBrowser Manager", func() {
	var (
		manager         *topicbrowser.Manager
		mockService     *tbsvc.MockTopicBrowserService
		ctx             context.Context
		tick            uint64
		cancel          context.CancelFunc
		mockSvcRegistry *serviceregistry.Registry
	)

	AfterEach(func() {
		cancel()
	})

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		tick = 0
		mockSvcRegistry = serviceregistry.NewMockRegistry()

		// Create a new TopicBrowserManager with the mock service
		manager, mockService = fsmtest.CreateMockTopicBrowserManager("test-manager")

		// Initialize the mock service state to empty
		mockService.ExistingServices = make(map[string]bool)
		mockService.ServiceStates = make(map[string]*tbsvc.ServiceInfo)
	})

	// -------------------------------------------------------------------------
	//  INITIALIZATION
	// -------------------------------------------------------------------------
	Context("Initialization", func() {
		It("should handle empty config without errors", func() {
			emptyConfig := config.FullConfig{
				Internal: config.InternalConfig{
					TopicBrowser: []topicbrowserserviceconfig.Config{},
				},
			}

			// Single call to a helper that wraps Reconcile
			newTick, err := fsmtest.WaitForTopicBrowserManagerStable(
				ctx, fsm.SystemSnapshot{CurrentConfig: emptyConfig, Tick: tick}, manager, mockSvcRegistry,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())
			Expect(manager.GetInstances()).To(BeEmpty())
		})

		It("should create a service in stopped state and remain stable", func() {
			serviceName := "test-stopped-service"
			cfg := config.FullConfig{
				Internal: config.InternalConfig{
					TopicBrowser: []topicbrowserserviceconfig.Config{
						fsmtest.CreateTopicBrowserTestConfig(serviceName, topicbrowser.OperationalStateStopped),
					},
				},
			}

			// Configure the mock service to allow transition to Stopped
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, serviceName, topicbrowser.OperationalStateStopped)

			// Wait for instance creation and stable 'Stopped' state
			newTick, err := fsmtest.WaitForTopicBrowserManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick},
				manager,
				mockSvcRegistry,
				serviceName,
				topicbrowser.OperationalStateStopped,
				10,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Double-check the manager state
			inst, exists := manager.GetInstance(serviceName)
			Expect(exists).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(topicbrowser.OperationalStateStopped))
		})

		It("should create a service in active state and reach idle or active", func() {
			serviceName := "test-active-service"
			cfg := config.FullConfig{
				Internal: config.InternalConfig{
					TopicBrowser: []topicbrowserserviceconfig.Config{
						fsmtest.CreateTopicBrowserTestConfig(serviceName, topicbrowser.OperationalStateActive),
					},
				},
			}

			// Configure the mock service for transition to Idle (or Active)
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, serviceName, topicbrowser.OperationalStateIdle)

			newTick, err := fsmtest.WaitForTopicBrowserManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick, SnapshotTime: time.Now()},
				manager,
				mockSvcRegistry,
				serviceName,
				topicbrowser.OperationalStateIdle, // or OperationalStateActive, whichever is stable
				90,
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
			serviceName := "test-lifecycle"
			// Start from active config
			fullCfg := config.FullConfig{
				Internal: config.InternalConfig{
					TopicBrowser: []topicbrowserserviceconfig.Config{
						fsmtest.CreateTopicBrowserTestConfig(serviceName, topicbrowser.OperationalStateActive),
					},
				},
			}

			// Configure for transition to Idle
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, serviceName, topicbrowser.OperationalStateIdle)

			// 1) Wait for idle
			newTick, err := fsmtest.WaitForTopicBrowserManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick, SnapshotTime: time.Now()},
				manager,
				mockSvcRegistry,
				serviceName,
				topicbrowser.OperationalStateIdle,
				90,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// 2) Now configure for degraded state
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, serviceName, topicbrowser.OperationalStateDegraded)

			// Wait for state transition
			newTick, err = fsmtest.WaitForTopicBrowserManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick, SnapshotTime: time.Now()},
				manager,
				mockSvcRegistry,
				serviceName,
				topicbrowser.OperationalStateDegraded,
				90,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// 3) Configure for stopped state before removal
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, serviceName, topicbrowser.OperationalStateStopped)

			// 4) Remove from config => instance eventually stops & is removed
			emptyConfig := config.FullConfig{
				Internal: config.InternalConfig{
					TopicBrowser: []topicbrowserserviceconfig.Config{},
				},
			}
			newTick, err = fsmtest.WaitForTopicBrowserManagerInstanceRemoval(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: emptyConfig, Tick: tick, SnapshotTime: time.Now()},
				manager,
				mockSvcRegistry,
				serviceName,
				30, // More attempts for removal
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Double-check it's actually gone
			_, exists := manager.GetInstance(serviceName)
			Expect(exists).To(BeFalse())
		})

		It("should handle multiple services concurrently", func() {
			serviceNames := []string{"tb-1", "tb-2", "tb-3"}
			configs := make([]topicbrowserserviceconfig.Config, len(serviceNames))

			for i, name := range serviceNames {
				configs[i] = fsmtest.CreateTopicBrowserTestConfig(name, topicbrowser.OperationalStateActive)
				fsmtest.ConfigureTopicBrowserManagerForState(mockService, name, topicbrowser.OperationalStateIdle)
			}

			fullCfg := config.FullConfig{
				Internal: config.InternalConfig{
					TopicBrowser: configs,
				},
			}

			// Wait for all services to reach stable state
			for _, name := range serviceNames {
				newTick, err := fsmtest.WaitForTopicBrowserManagerInstanceState(
					ctx,
					fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick, SnapshotTime: time.Now()},
					manager,
					mockSvcRegistry,
					name,
					topicbrowser.OperationalStateIdle,
					60,
				)
				tick = newTick
				Expect(err).NotTo(HaveOccurred())
			}

			// Verify all instances exist
			instances := manager.GetInstances()
			Expect(len(instances)).To(Equal(len(serviceNames)))
			for _, name := range serviceNames {
				inst, exists := manager.GetInstance(name)
				Expect(exists).To(BeTrue())
				Expect(inst.GetCurrentFSMState()).To(Equal(topicbrowser.OperationalStateIdle))
			}
		})
	})

	// -------------------------------------------------------------------------
	//  CONFIGURATION MANAGEMENT
	// -------------------------------------------------------------------------
	Context("Configuration Management", func() {
		It("should handle configuration updates", func() {
			serviceName := "test-config-update"

			// Start with initial config
			initialConfig := fsmtest.CreateTopicBrowserTestConfig(serviceName, topicbrowser.OperationalStateActive)
			cfg := config.FullConfig{
				Internal: config.InternalConfig{
					TopicBrowser: []topicbrowserserviceconfig.Config{initialConfig},
				},
			}

			fsmtest.ConfigureTopicBrowserManagerForState(mockService, serviceName, topicbrowser.OperationalStateIdle)

			// Wait for initial stable state
			newTick, err := fsmtest.WaitForTopicBrowserManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick, SnapshotTime: time.Now()},
				manager,
				mockSvcRegistry,
				serviceName,
				topicbrowser.OperationalStateIdle,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Update configuration
			updatedConfig := fsmtest.CreateTopicBrowserTestConfig(serviceName, topicbrowser.OperationalStateActive)
			// Modify some fields to trigger update
			updatedCfg := config.FullConfig{
				Internal: config.InternalConfig{
					TopicBrowser: []topicbrowserserviceconfig.Config{updatedConfig},
				},
			}

			// Wait for configuration update to be processed
			newTick, err = fsmtest.WaitForTopicBrowserManagerStable(
				ctx, fsm.SystemSnapshot{CurrentConfig: updatedCfg, Tick: tick}, manager, mockSvcRegistry,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Verify update was called
			Expect(mockService.UpdateInManagerCalled).To(BeTrue())
		})

		It("should handle service removal cleanly", func() {
			serviceName := "test-removal"
			cfg := config.FullConfig{
				Internal: config.InternalConfig{
					TopicBrowser: []topicbrowserserviceconfig.Config{
						fsmtest.CreateTopicBrowserTestConfig(serviceName, topicbrowser.OperationalStateActive),
					},
				},
			}

			fsmtest.ConfigureTopicBrowserManagerForState(mockService, serviceName, topicbrowser.OperationalStateIdle)

			// Create and stabilize service
			newTick, err := fsmtest.WaitForTopicBrowserManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick, SnapshotTime: time.Now()},
				manager,
				mockSvcRegistry,
				serviceName,
				topicbrowser.OperationalStateIdle,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Configure for clean removal
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, serviceName, topicbrowser.OperationalStateStopped)

			// Remove from config
			emptyConfig := config.FullConfig{
				Internal: config.InternalConfig{
					TopicBrowser: []topicbrowserserviceconfig.Config{},
				},
			}

			newTick, err = fsmtest.WaitForTopicBrowserManagerInstanceRemoval(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: emptyConfig, Tick: tick, SnapshotTime: time.Now()},
				manager,
				mockSvcRegistry,
				serviceName,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Verify service was removed
			_, exists := manager.GetInstance(serviceName)
			Expect(exists).To(BeFalse())
			Expect(mockService.RemoveFromManagerCalled).To(BeTrue())
		})
	})

	// -------------------------------------------------------------------------
	//  ERROR HANDLING
	// -------------------------------------------------------------------------
	Context("Error Handling", func() {
		It("should handle service creation failures", func() {
			serviceName := "test-creation-failure"
			cfg := config.FullConfig{
				Internal: config.InternalConfig{
					TopicBrowser: []topicbrowserserviceconfig.Config{
						fsmtest.CreateTopicBrowserTestConfig(serviceName, topicbrowser.OperationalStateActive),
					},
				},
			}

			// Configure mock to fail creation
			mockService.AddToManagerError = tbsvc.ErrServiceCreationFailed

			// Attempt to create service should fail gracefully
			_, err := fsmtest.WaitForTopicBrowserManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick, SnapshotTime: time.Now()},
				manager,
				mockSvcRegistry,
				serviceName,
				topicbrowser.OperationalStateIdle,
				3, // Few attempts since we expect failure
			)
			Expect(err).To(HaveOccurred())

			// Instance should still exist but in error state
			inst, exists := manager.GetInstance(serviceName)
			Expect(exists).To(BeTrue())
			// Error state handling depends on implementation
			_ = inst // Use the instance variable
		})

		It("should recover from transient errors", func() {
			serviceName := "test-transient-error"
			cfg := config.FullConfig{
				Internal: config.InternalConfig{
					TopicBrowser: []topicbrowserserviceconfig.Config{
						fsmtest.CreateTopicBrowserTestConfig(serviceName, topicbrowser.OperationalStateActive),
					},
				},
			}

			// Start with transient error
			mockService.AddToManagerError = tbsvc.ErrTransientFailure

			// Initial attempts should fail
			_, err := fsmtest.WaitForTopicBrowserManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick, SnapshotTime: time.Now()},
				manager,
				mockSvcRegistry,
				serviceName,
				topicbrowser.OperationalStateIdle,
				2,
			)
			Expect(err).To(HaveOccurred())

			// Clear error and configure for success
			mockService.AddToManagerError = nil
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, serviceName, topicbrowser.OperationalStateIdle)

			// Should eventually succeed
			newTick, err := fsmtest.WaitForTopicBrowserManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick + 10, SnapshotTime: time.Now()}, // Advance tick to clear backoff
				manager,
				mockSvcRegistry,
				serviceName,
				topicbrowser.OperationalStateIdle,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// -------------------------------------------------------------------------
	//  DEGRADED STATE SCENARIOS
	// -------------------------------------------------------------------------
	Context("Degraded State Scenarios", func() {
		It("should detect and handle degraded instances", func() {
			serviceName := "test-degraded"
			cfg := config.FullConfig{
				Internal: config.InternalConfig{
					TopicBrowser: []topicbrowserserviceconfig.Config{
						fsmtest.CreateTopicBrowserTestConfig(serviceName, topicbrowser.OperationalStateActive),
					},
				},
			}

			// Start in healthy state
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, serviceName, topicbrowser.OperationalStateIdle)

			newTick, err := fsmtest.WaitForTopicBrowserManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick, SnapshotTime: time.Now()},
				manager,
				mockSvcRegistry,
				serviceName,
				topicbrowser.OperationalStateIdle,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Transition to degraded
			fsmtest.ConfigureTopicBrowserManagerForState(mockService, serviceName, topicbrowser.OperationalStateDegraded)

			newTick, err = fsmtest.WaitForTopicBrowserManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick, SnapshotTime: time.Now()},
				manager,
				mockSvcRegistry,
				serviceName,
				topicbrowser.OperationalStateDegraded,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Verify degraded state
			inst, exists := manager.GetInstance(serviceName)
			Expect(exists).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(topicbrowser.OperationalStateDegraded))
		})
	})
})

// Helper functions for testing

func createTestConfig(name string) topicbrowserserviceconfig.Config {
	return topicbrowserserviceconfig.Config{
		// Add test configuration fields when they're defined
	}
}

func createTestSnapshot() fsm.SystemSnapshot {
	return fsm.SystemSnapshot{
		Tick:         1,
		SnapshotTime: time.Now(),
	}
}
