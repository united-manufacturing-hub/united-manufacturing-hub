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

package benthos_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsmtest"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/backoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	benthosfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	benthossvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
)

// Following the CursorRules, we never call manager.Reconcile(...) directly in loops.
// Instead, we use the fsmtest helpers.

var _ = Describe("BenthosManager", func() {
	var (
		manager     *benthosfsm.BenthosManager
		mockService *benthossvc.MockBenthosService
		ctx         context.Context
		tick        uint64
	)

	BeforeEach(func() {
		ctx = context.Background()
		tick = 0

		// Create a new BenthosManager with the mock service
		manager, mockService = fsmtest.CreateMockBenthosManager("test-manager")

		// Initialize the mock service state to empty
		mockService.ExistingServices = make(map[string]bool)
		mockService.ServiceStates = make(map[string]*benthossvc.ServiceInfo)
	})

	// -------------------------------------------------------------------------
	//  INITIALIZATION
	// -------------------------------------------------------------------------
	Context("Initialization", func() {
		It("should handle empty config without errors", func() {
			emptyConfig := config.FullConfig{Benthos: []config.BenthosConfig{}}

			// Single call to a helper that wraps Reconcile
			newTick, err := fsmtest.WaitForBenthosManagerStable(
				ctx, manager, emptyConfig, tick,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())
			Expect(manager.GetInstances()).To(BeEmpty())
		})

		It("should create a service in stopped state and remain stable", func() {
			serviceName := "test-stopped-service"
			cfg := config.FullConfig{
				Benthos: []config.BenthosConfig{
					fsmtest.CreateBenthosTestConfig(serviceName, benthosfsm.OperationalStateStopped),
				},
			}

			// Configure the mock service to allow transition to Stopped
			fsmtest.ConfigureBenthosManagerForState(mockService, serviceName, benthosfsm.OperationalStateStopped)

			// Wait for instance creation and stable 'Stopped' state
			newTick, err := fsmtest.WaitForBenthosManagerInstanceState(
				ctx,
				manager,
				cfg,
				serviceName,
				benthosfsm.OperationalStateStopped,
				10,
				tick,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Double-check the manager state
			inst, exists := manager.GetInstance(serviceName)
			Expect(exists).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(benthosfsm.OperationalStateStopped))
		})

		It("should create a service in active state and reach idle or active", func() {
			serviceName := "test-active-service"
			cfg := config.FullConfig{
				Benthos: []config.BenthosConfig{
					fsmtest.CreateBenthosTestConfig(serviceName, benthosfsm.OperationalStateActive),
				},
			}

			// Configure the mock service for transition to Idle (or Active)
			fsmtest.ConfigureBenthosManagerForState(mockService, serviceName, benthosfsm.OperationalStateIdle)

			// Some Benthos FSMs pass through 'Idle' before 'Active', so we might
			// check for either. In your code, you might unify them in a single final "active-like" state.

			newTick, err := fsmtest.WaitForBenthosManagerInstanceState(
				ctx,
				manager,
				cfg,
				serviceName,
				benthosfsm.OperationalStateIdle, // or OperationalStateActive, whichever is stable
				20,
				tick,
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
				Benthos: []config.BenthosConfig{
					fsmtest.CreateBenthosTestConfig(serviceName, benthosfsm.OperationalStateActive),
				},
			}

			// Configure for transition to Idle
			fsmtest.ConfigureBenthosManagerForState(mockService, serviceName, benthosfsm.OperationalStateIdle)

			// 1) Wait for idle
			newTick, err := fsmtest.WaitForBenthosManagerInstanceState(ctx, manager, fullCfg,
				serviceName, benthosfsm.OperationalStateIdle, 20, tick)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// 2) Now configure for degraded state
			fsmtest.ConfigureBenthosManagerForState(mockService, serviceName, benthosfsm.OperationalStateDegraded)

			// Wait for state transition
			newTick, err = fsmtest.WaitForBenthosManagerInstanceState(ctx, manager, fullCfg,
				serviceName, benthosfsm.OperationalStateDegraded, 10, tick)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// 3) Configure for stopped state before removal
			fsmtest.ConfigureBenthosManagerForState(mockService, serviceName, benthosfsm.OperationalStateStopped)

			// 4) Remove from config => instance eventually stops & is removed
			emptyConfig := config.FullConfig{Benthos: []config.BenthosConfig{}}
			newTick, err = fsmtest.WaitForBenthosManagerInstanceRemoval(
				ctx,
				manager,
				emptyConfig,
				serviceName,
				30, // More attempts for removal
				tick,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			Expect(manager.GetInstances()).NotTo(HaveKey(serviceName))
		})

		It("should toggle from active to stopped and back to active with config changes", func() {
			serviceName := "test-toggle"
			// Start from active config
			activeCfg := config.FullConfig{
				Benthos: []config.BenthosConfig{
					fsmtest.CreateBenthosTestConfig(serviceName, benthosfsm.OperationalStateActive),
				},
			}

			// Configure for transition to Idle
			fsmtest.ConfigureBenthosManagerForState(mockService, serviceName, benthosfsm.OperationalStateIdle)

			// Wait for idle or active
			newTick, err := fsmtest.WaitForBenthosManagerInstanceState(ctx, manager, activeCfg,
				serviceName, benthosfsm.OperationalStateIdle, 20, tick)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Configure for transition to Stopped
			fsmtest.ConfigureBenthosManagerForState(mockService, serviceName, benthosfsm.OperationalStateStopped)

			// Switch config to stopped
			stoppedCfg := config.FullConfig{
				Benthos: []config.BenthosConfig{
					fsmtest.CreateBenthosTestConfig(serviceName, benthosfsm.OperationalStateStopped),
				},
			}

			newTick, err = fsmtest.WaitForBenthosManagerInstanceState(ctx, manager, stoppedCfg,
				serviceName, benthosfsm.OperationalStateStopped, 20, tick)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Configure for transition back to Idle
			fsmtest.ConfigureBenthosManagerForState(mockService, serviceName, benthosfsm.OperationalStateIdle)

			// Switch config back to active
			newTick, err = fsmtest.WaitForBenthosManagerInstanceState(ctx, manager, activeCfg,
				serviceName, benthosfsm.OperationalStateIdle, 20, tick)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// -------------------------------------------------------------------------
	//  MULTIPLE SERVICES
	// -------------------------------------------------------------------------
	Context("Multiple Services", func() {
		It("should handle multiple services in parallel, each with its own state", func() {
			svc1 := "benthos1"
			svc2 := "benthos2"

			config1 := fsmtest.CreateBenthosTestConfig(svc1, benthosfsm.OperationalStateActive)
			config2 := fsmtest.CreateBenthosTestConfig(svc2, benthosfsm.OperationalStateActive)
			fullCfg := config.FullConfig{
				Benthos: []config.BenthosConfig{config1, config2},
			}

			// Configure both services for transition to Idle
			fsmtest.ConfigureBenthosManagerForState(mockService, svc1, benthosfsm.OperationalStateIdle)
			fsmtest.ConfigureBenthosManagerForState(mockService, svc2, benthosfsm.OperationalStateStopped)

			// Suppose we want both eventually to be idle
			newTick, err := fsmtest.WaitForBenthosManagerMultiState(
				ctx, manager, fullCfg,
				map[string]string{
					svc1: benthosfsm.OperationalStateIdle,
					svc2: benthosfsm.OperationalStateStopped,
				},
				30,
				tick,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Confirm manager sees both
			Expect(manager.GetInstances()).To(HaveKey(svc1))
			Expect(manager.GetInstances()).To(HaveKey(svc2))

			// Check the states of the instances
			inst1, exists := manager.GetInstance(svc1)
			Expect(exists).To(BeTrue())
			Expect(inst1.GetCurrentFSMState()).To(Equal(benthosfsm.OperationalStateIdle))

			inst2, exists := manager.GetInstance(svc2)
			Expect(exists).To(BeTrue())
			Expect(inst2.GetCurrentFSMState()).To(Equal(benthosfsm.OperationalStateStopped))
		})
	})

	// -------------------------------------------------------------------------
	//  ERROR HANDLING
	// -------------------------------------------------------------------------
	Context("Error Handling", func() {
		It("should recover from transient startup failures", func() {
			serviceName := "transient-error"
			fullCfg := config.FullConfig{
				Benthos: []config.BenthosConfig{
					fsmtest.CreateBenthosTestConfig(serviceName, benthosfsm.OperationalStateActive),
				},
			}

			// Configure the mock for initial "Starting" state
			fsmtest.ConfigureBenthosManagerForState(mockService, serviceName, benthosfsm.OperationalStateStarting)

			// Initial reconcile to create the instance (using helper instead of direct call)
			newTick, err := fsmtest.WaitForBenthosManagerStable(ctx, manager, fullCfg, tick)
			Expect(err).NotTo(HaveOccurred())
			tick = newTick

			// Now configure for transition to Idle eventually
			fsmtest.ConfigureBenthosManagerForState(mockService, serviceName, benthosfsm.OperationalStateIdle)

			// Eventually, it should try again and go to idle
			newTick, err = fsmtest.WaitForBenthosManagerInstanceState(ctx, manager, fullCfg,
				serviceName, benthosfsm.OperationalStateIdle, 25, tick)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())
		})

		It("should remove an instance if it hits a permanent error in stopped state", func() {
			serviceName := "perm-error-test"
			fullCfg := config.FullConfig{
				Benthos: []config.BenthosConfig{
					fsmtest.CreateBenthosTestConfig(serviceName, benthosfsm.OperationalStateActive),
				},
			}

			// Configure for transition to Idle
			fsmtest.ConfigureBenthosManagerForState(mockService, serviceName, benthosfsm.OperationalStateIdle)

			// Wait for idle
			newTick, err := fsmtest.WaitForBenthosManagerInstanceState(ctx, manager, fullCfg,
				serviceName, benthosfsm.OperationalStateIdle, 20, tick)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Configure for transition to Stopped
			fsmtest.ConfigureBenthosManagerForState(mockService, serviceName, benthosfsm.OperationalStateStopped)

			// Set desired state to stopped
			stoppedCfg := config.FullConfig{
				Benthos: []config.BenthosConfig{
					fsmtest.CreateBenthosTestConfig(serviceName, benthosfsm.OperationalStateStopped),
				},
			}

			// Wait for stopped
			newTick, err = fsmtest.WaitForBenthosManagerInstanceState(ctx, manager, stoppedCfg,
				serviceName, benthosfsm.OperationalStateStopped, 20, tick)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Simulate a permanent error by configuring the mock service
			mockService.AddBenthosToS6ManagerError = fmt.Errorf("%s: forced error", backoff.PermanentFailureError)

			// Wait for manager to remove instance due to permanent error
			newTick, err = fsmtest.WaitForBenthosManagerInstanceRemoval(
				ctx, manager,
				config.FullConfig{Benthos: []config.BenthosConfig{}},
				serviceName,
				15,
				tick,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			Expect(manager.GetInstances()).NotTo(HaveKey(serviceName))
		})
	})

	// -------------------------------------------------------------------------
	//  EDGE CASES
	// -------------------------------------------------------------------------
	Context("Edge Cases", func() {
		It("should handle 'service not found' gracefully", func() {
			serviceName := "ghost-service"
			fullCfg := config.FullConfig{
				Benthos: []config.BenthosConfig{
					fsmtest.CreateBenthosTestConfig(serviceName, benthosfsm.OperationalStateActive),
				},
			}

			// Insert a custom mock HTTP client that says "service not found"
			mockHTTPClient := benthossvc.NewMockHTTPClient()
			mockHTTPClient.SetServiceNotFound(serviceName)
			mockService.HTTPClient = mockHTTPClient

			// Configure for initial transition to Stopped
			fsmtest.ConfigureBenthosManagerForState(mockService, serviceName, benthosfsm.OperationalStateStopped)

			// Initial reconcile to create the instance
			newTick, err := fsmtest.WaitForBenthosManagerStable(ctx, manager, fullCfg, tick)
			Expect(err).NotTo(HaveOccurred())
			tick = newTick

			// The manager tries to reconcile, but the service isn't found initially
			newTick, err = fsmtest.WaitForBenthosManagerInstanceState(ctx, manager, fullCfg,
				serviceName, benthosfsm.OperationalStateStopped, 15, tick)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Confirm instance is created but stuck in Stopped
			inst, ok := manager.GetInstance(serviceName)
			Expect(ok).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(benthosfsm.OperationalStateStopped))
		})
	})
})
