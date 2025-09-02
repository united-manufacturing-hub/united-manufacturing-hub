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
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6/s6_default"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsmtest"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/backoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthosfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	benthos_monitor_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos_monitor"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/portmanager"
	benthossvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
	benthos_monitor_service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// Following the CursorRules, we never call manager.Reconcile(...) directly in loops.
// Instead, we use the fsmtest helpers.

var _ = Describe("BenthosManager", func() {
	var (
		manager         *benthosfsm.BenthosManager
		mockService     *benthossvc.MockBenthosService
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
			emptyConfig := config.FullConfig{Internal: config.InternalConfig{Benthos: []config.BenthosConfig{}}}

			// Single call to a helper that wraps Reconcile
			newTick, err := fsmtest.WaitForBenthosManagerStable(
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
					Benthos: []config.BenthosConfig{
						fsmtest.CreateBenthosTestConfig(serviceName, benthosfsm.OperationalStateStopped),
					},
				},
			}

			// Configure the mock service to allow transition to Stopped
			fsmtest.ConfigureBenthosManagerForState(mockService, serviceName, benthosfsm.OperationalStateStopped)

			// Wait for instance creation and stable 'Stopped' state
			newTick, err := fsmtest.WaitForBenthosManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick},
				manager,
				mockSvcRegistry,
				serviceName,
				benthosfsm.OperationalStateStopped,
				10,
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
				Internal: config.InternalConfig{
					Benthos: []config.BenthosConfig{
						fsmtest.CreateBenthosTestConfig(serviceName, benthosfsm.OperationalStateActive),
					},
				},
			}

			// Configure the mock service for transition to Idle (or Active)
			fsmtest.ConfigureBenthosManagerForState(mockService, serviceName, benthosfsm.OperationalStateIdle)

			// Some Benthos FSMs pass through 'Idle' before 'Active', so we might
			// check for either. In your code, you might unify them in a single final "active-like" state.

			newTick, err := fsmtest.WaitForBenthosManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick, SnapshotTime: time.Now()},
				manager,
				mockSvcRegistry,
				serviceName,
				benthosfsm.OperationalStateIdle, // or OperationalStateActive, whichever is stable
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
					Benthos: []config.BenthosConfig{
						fsmtest.CreateBenthosTestConfig(serviceName, benthosfsm.OperationalStateActive),
					},
				},
			}

			// Configure for transition to Idle
			fsmtest.ConfigureBenthosManagerForState(mockService, serviceName, benthosfsm.OperationalStateIdle)

			// 1) Wait for idle
			newTick, err := fsmtest.WaitForBenthosManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick, SnapshotTime: time.Now()}, manager, mockSvcRegistry,
				serviceName,
				benthosfsm.OperationalStateIdle,
				90,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// 2) Now configure for degraded state
			fsmtest.ConfigureBenthosManagerForState(mockService, serviceName, benthosfsm.OperationalStateDegraded)

			// Wait for state transition
			newTick, err = fsmtest.WaitForBenthosManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick, SnapshotTime: time.Now()}, manager, mockSvcRegistry,
				serviceName,
				benthosfsm.OperationalStateDegraded,
				90,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// 3) Configure for stopped state before removal
			fsmtest.ConfigureBenthosManagerForState(mockService, serviceName, benthosfsm.OperationalStateStopped)

			// 4) Remove from config => instance eventually stops & is removed
			emptyConfig := config.FullConfig{Internal: config.InternalConfig{Benthos: []config.BenthosConfig{}}}
			newTick, err = fsmtest.WaitForBenthosManagerInstanceRemoval(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: emptyConfig, Tick: tick, SnapshotTime: time.Now()},
				manager,
				mockSvcRegistry,
				serviceName,
				30, // More attempts for removal
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			Expect(manager.GetInstances()).NotTo(HaveKey(serviceName))
		})

		It("should toggle from active to stopped and back to active with config changes", func() {
			serviceName := "test-toggle"
			// Start from active config
			activeCfg := config.FullConfig{
				Internal: config.InternalConfig{
					Benthos: []config.BenthosConfig{
						fsmtest.CreateBenthosTestConfig(serviceName, benthosfsm.OperationalStateActive),
					},
				},
			}

			// Configure for transition to Idle
			fsmtest.ConfigureBenthosManagerForState(mockService, serviceName, benthosfsm.OperationalStateIdle)

			// Wait for idle or active
			newTick, err := fsmtest.WaitForBenthosManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: activeCfg, Tick: tick, SnapshotTime: time.Now()}, manager, mockSvcRegistry,
				serviceName,
				benthosfsm.OperationalStateIdle,
				90,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Configure for transition to Stopped
			fsmtest.ConfigureBenthosManagerForState(mockService, serviceName, benthosfsm.OperationalStateStopped)

			// Switch config to stopped
			stoppedCfg := config.FullConfig{
				Internal: config.InternalConfig{
					Benthos: []config.BenthosConfig{
						fsmtest.CreateBenthosTestConfig(serviceName, benthosfsm.OperationalStateStopped),
					},
				},
			}

			newTick, err = fsmtest.WaitForBenthosManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: stoppedCfg, Tick: tick}, manager, mockSvcRegistry,
				serviceName,
				benthosfsm.OperationalStateStopped,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Configure for transition back to Idle
			fsmtest.ConfigureBenthosManagerForState(mockService, serviceName, benthosfsm.OperationalStateIdle)

			// Switch config back to active
			// We need to pass the snapshot time as we are passing through the healthcheck waiting state and it needs to be passing for at least constants.benthosStartingConfigLoadingDuration
			newTick, err = fsmtest.WaitForBenthosManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: activeCfg, Tick: tick, SnapshotTime: time.Now()}, manager, mockSvcRegistry,
				serviceName,
				benthosfsm.OperationalStateIdle,
				90,
			)
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
				Internal: config.InternalConfig{
					Benthos: []config.BenthosConfig{config1, config2},
				},
			}

			// Configure both services for transition to Idle
			fsmtest.ConfigureBenthosManagerForState(mockService, svc1, benthosfsm.OperationalStateIdle)
			fsmtest.ConfigureBenthosManagerForState(mockService, svc2, benthosfsm.OperationalStateStopped)

			// Suppose we want both eventually to be idle
			newTick, err := fsmtest.WaitForBenthosManagerMultiState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick, SnapshotTime: time.Now()},
				manager,
				mockSvcRegistry,
				map[string]string{
					svc1: benthosfsm.OperationalStateIdle,
					svc2: benthosfsm.OperationalStateStarting, // target state is active, but the state is set to stopped, so it will hang in starting state
				},
				100,
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
			Expect(inst2.GetCurrentFSMState()).To(Equal(benthosfsm.OperationalStateStarting))
		})
	})

	// -------------------------------------------------------------------------
	//  ERROR HANDLING
	// -------------------------------------------------------------------------
	Context("Error Handling", func() {
		It("should recover from transient startup failures", func() {
			serviceName := "transient-error"
			fullCfg := config.FullConfig{
				Internal: config.InternalConfig{
					Benthos: []config.BenthosConfig{
						fsmtest.CreateBenthosTestConfig(serviceName, benthosfsm.OperationalStateActive),
					},
				},
			}

			// Configure the mock for initial "Starting" state
			fsmtest.ConfigureBenthosManagerForState(mockService, serviceName, benthosfsm.OperationalStateStarting)

			// Initial reconcile to create the instance (using helper instead of direct call)
			newTick, err := fsmtest.WaitForBenthosManagerStable(ctx, fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick}, manager, mockSvcRegistry)
			Expect(err).NotTo(HaveOccurred())
			tick = newTick

			// Now configure for transition to Idle eventually
			fsmtest.ConfigureBenthosManagerForState(mockService, serviceName, benthosfsm.OperationalStateIdle)

			// Eventually, it should try again and go to idle
			newTick, err = fsmtest.WaitForBenthosManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick, SnapshotTime: time.Now()}, manager, mockSvcRegistry,
				serviceName,
				benthosfsm.OperationalStateIdle,
				90,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())
		})

		It("should remove an instance if it hits a permanent error in stopped state", func() {
			serviceName := "perm-error-test"
			fullCfg := config.FullConfig{
				Internal: config.InternalConfig{
					Benthos: []config.BenthosConfig{
						fsmtest.CreateBenthosTestConfig(serviceName, benthosfsm.OperationalStateActive),
					},
				},
			}

			// Configure for transition to Idle
			fsmtest.ConfigureBenthosManagerForState(mockService, serviceName, benthosfsm.OperationalStateIdle)

			// Wait for idle
			newTick, err := fsmtest.WaitForBenthosManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick, SnapshotTime: time.Now()}, manager, mockSvcRegistry,
				serviceName,
				benthosfsm.OperationalStateIdle,
				90,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Configure for transition to Stopped
			fsmtest.ConfigureBenthosManagerForState(mockService, serviceName, benthosfsm.OperationalStateStopped)

			// Set desired state to stopped
			stoppedCfg := config.FullConfig{
				Internal: config.InternalConfig{
					Benthos: []config.BenthosConfig{
						fsmtest.CreateBenthosTestConfig(serviceName, benthosfsm.OperationalStateStopped),
					},
				},
			}

			// Wait for stopped
			newTick, err = fsmtest.WaitForBenthosManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: stoppedCfg, Tick: tick}, manager, mockSvcRegistry,
				serviceName,
				benthosfsm.OperationalStateStopped,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Simulate a permanent error by configuring the mock service
			mockService.AddBenthosToS6ManagerError = fmt.Errorf("%s: forced error", backoff.PermanentFailureError)

			// Wait for manager to remove instance due to permanent error
			newTick, err = fsmtest.WaitForBenthosManagerInstanceRemoval(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Benthos: []config.BenthosConfig{}}}},
				manager,
				mockSvcRegistry,
				serviceName,
				30,
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
			Skip("TODO: until I understand how to mock the benthos monitor service")
			serviceName := "ghost-service"
			fullCfg := config.FullConfig{
				Internal: config.InternalConfig{
					Benthos: []config.BenthosConfig{
						fsmtest.CreateBenthosTestConfig(serviceName, benthosfsm.OperationalStateActive),
					},
				},
			}

			// Insert a custom mock HTTP client that says "service not found"
			//mockHTTPClient := benthossvc.NewMockHTTPClient()
			//mockHTTPClient.SetServiceNotFound(serviceName)
			//mockService.HTTPClient = mockHTTPClient

			// Configure for initial transition to Stopped
			fsmtest.ConfigureBenthosManagerForState(mockService, serviceName, benthosfsm.OperationalStateStopped)

			// Initial reconcile to create the instance
			newTick, err := fsmtest.WaitForBenthosManagerStable(ctx, fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick}, manager, mockSvcRegistry)
			Expect(err).NotTo(HaveOccurred())
			tick = newTick

			// The manager tries to reconcile, but the service isn't found initially
			newTick, err = fsmtest.WaitForBenthosManagerInstanceState(ctx, fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick}, manager, mockSvcRegistry,
				serviceName,
				benthosfsm.OperationalStateStopped,
				30,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Confirm instance is created but stuck in Stopped
			inst, ok := manager.GetInstance(serviceName)
			Expect(ok).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(benthosfsm.OperationalStateStopped))
		})
	})

	Context("Port Management", func() {
		It("should allocate ports before base reconciliation", func() {
			serviceName := "test-service-port-alloc"

			// Create a BenthosConfig that desires an Active state
			benthosCfg := fsmtest.CreateBenthosTestConfig(serviceName, benthosfsm.OperationalStateActive)
			fullCfg := config.FullConfig{
				Internal: config.InternalConfig{
					Benthos: []config.BenthosConfig{benthosCfg},
				},
			}

			// Get the mock port manager from the registry
			mockPortMgr := mockSvcRegistry.GetPortManager().(*portmanager.MockPortManager)

			// Perform a single manager reconcile using a helper (not a for-loop)
			newTick, err, reconciled := fsmtest.ReconcileOnceBenthosManager(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick},
				manager,
				mockSvcRegistry,
			)
			tick = newTick

			// Check results
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue(), "Expected a change during the first reconcile")
			Expect(mockPortMgr.PreReconcileCalled).To(BeTrue(), "Manager should call PreReconcile first")
			Expect(mockPortMgr.PostReconcileCalled).To(BeTrue(), "Manager should call PostReconcile after the base reconcile")

			// Optionally verify the instance's port was allocated
			inst, found := manager.GetInstance(serviceName)
			Expect(found).To(BeTrue(), "Instance should be created after reconcile")
			_, ok := inst.(*benthosfsm.BenthosInstance)
			Expect(ok).To(BeTrue(), "Instance should be a BenthosInstance")

			port, exists := mockPortMgr.GetPort(serviceName)
			Expect(exists).To(BeTrue(), "Port should be allocated for the service")
			Expect(port).To(BeNumerically(">", 0), "Expected a valid (>0) port to be allocated")
		})

		It("should handle port allocation failures gracefully", func() {
			serviceName := "test-service-port-error"

			// Create config for the service
			benthosCfg := fsmtest.CreateBenthosTestConfig(serviceName, benthosfsm.OperationalStateActive)
			fullCfg := config.FullConfig{
				Internal: config.InternalConfig{
					Benthos: []config.BenthosConfig{benthosCfg},
				},
			}

			// Get the mock port manager from the registry and configure it to return an error
			mockPortMgr := mockSvcRegistry.GetPortManager().(*portmanager.MockPortManager)
			mockPortMgr.PreReconcileError = fmt.Errorf("test port allocation error")

			// Reconcile once
			newTick, err, reconciled := fsmtest.ReconcileOnceBenthosManager(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick},
				manager,
				mockSvcRegistry,
			)
			tick = newTick

			// Verify we got an error and no changes occurred
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("test port allocation error"))
			Expect(reconciled).To(BeFalse(), "No changes should be recorded if port allocation fails")

			// The manager should not create any instance
			Expect(manager.GetInstances()).To(BeEmpty(), "Expected zero instances due to port allocation failure")
		})

		It("should call post-reconciliation after base reconciliation", func() {
			serviceName := "test-service-port-post"
			benthosCfg := fsmtest.CreateBenthosTestConfig(serviceName, benthosfsm.OperationalStateActive)
			fullCfg := config.FullConfig{
				Internal: config.InternalConfig{
					Benthos: []config.BenthosConfig{benthosCfg},
				},
			}

			// Get the mock port manager from the registry
			mockPortMgr := mockSvcRegistry.GetPortManager().(*portmanager.MockPortManager)

			// Single reconcile
			newTick, err, reconciled := fsmtest.ReconcileOnceBenthosManager(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick},
				manager,
				mockSvcRegistry,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())

			// Verify post was called
			Expect(mockPortMgr.PostReconcileCalled).To(BeTrue())
		})
	})

	Context("Benthos-to-S6 remove hook", func() {
		It("calls the S6 mock’s Remove() when a Benthos instance disappears from config", func() {

			//----------------------------------------------------------------------
			// 0. plumbing & constants
			//----------------------------------------------------------------------
			const benthosName = "remove-me"

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			var tick uint64

			// Get the config and the resulting S6 config
			benthosCfg := fsmtest.CreateBenthosTestConfig(benthosName, benthosfsm.OperationalStateStopped)
			realBenthosService := benthossvc.NewDefaultBenthosService(benthosName)
			s6Config, err := realBenthosService.GenerateS6ConfigForBenthos(&benthosCfg.BenthosServiceConfig, benthosName)
			Expect(err).NotTo(HaveOccurred())

			//----------------------------------------------------------------------
			// 1. Build the dependency graph
			// ---------------------------------------------------------------------
			//    – real BenthosManager
			//    – BUT:  mock S6Manager  (so FSMs are mocks)
			//      AND:  mock S6Service  (so ad-hoc calls are mocks)
			//----------------------------------------------------------------------
			benthosMgr := benthosfsm.NewBenthosManager("rm-test-mgr")
			mockS6Mgr := s6fsm.NewS6ManagerWithMockedServices("rm-test-mgr")

			// ---------- mock S6 *service* injected into BenthosService ----------
			myMockS6Svc := s6service.NewMockService()
			// a minimal, valid YAML so Unmarshal succeeds
			myMockS6Svc.GetS6ConfigFileResult = []byte("logger:\n  level: info\n")
			// we also return a stub S6ServiceConfig for GetConfig()
			myMockS6Svc.GetConfigResult = s6Config

			mockBenthosMonitorSvc := benthos_monitor_service.NewMockBenthosMonitorService()
			mockBenthosMonitorMgr := benthos_monitor_fsm.NewBenthosMonitorManagerWithMockedService("rm-test-mgr", *mockBenthosMonitorSvc)

			// ---------- assemble BenthosService with both injections -----------

			benthosSvc := benthossvc.NewDefaultBenthosService(
				benthosName,
				benthossvc.WithS6Manager(mockS6Mgr),   // << manager mock
				benthossvc.WithS6Service(myMockS6Svc), // << service mock
				benthossvc.WithMonitorManager(mockBenthosMonitorMgr),
			)

			// ---------- craft the BenthosInstance & register it -----------------
			benthosInst := benthosfsm.NewBenthosInstanceWithService(benthosCfg, benthosSvc)
			benthosMgr.AddInstanceForTest(benthosName, benthosInst)

			//----------------------------------------------------------------------
			// 2. Reconcile until the Benthos FSM settles in “Stopped”.
			//----------------------------------------------------------------------
			startCfg := config.FullConfig{
				Internal: config.InternalConfig{
					Benthos: []config.BenthosConfig{benthosCfg},
				},
			}

			tick, err = fsmtest.WaitForBenthosManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: startCfg, Tick: tick},
				benthosMgr,
				mockSvcRegistry,
				benthosName,
				benthosfsm.OperationalStateStopped,
				10,
			)
			Expect(err).NotTo(HaveOccurred())

			//----------------------------------------------------------------------
			// 3. Grab the *mock* S6Service nested inside the S6Instance
			//    (we’ll check its RemoveCalled flag at the end).
			//----------------------------------------------------------------------

			s6Name := benthosSvc.GetS6ServiceName(benthosName)
			s6InstIfc, ok := mockS6Mgr.GetInstance(s6Name)
			Expect(ok).To(BeTrue(), "S6 instance should exist after first reconcile")

			s6Inst, ok := s6InstIfc.(*s6fsm.S6Instance)
			Expect(ok).To(BeTrue())

			innerMockS6Svc, ok := s6Inst.GetService().(*s6service.MockService)
			Expect(ok).To(BeTrue(), "internal service is not a MockService")

			//----------------------------------------------------------------------
			// 4. Hand the manager an *empty* Benthos section → instance must vanish
			//----------------------------------------------------------------------
			emptyCfg := config.FullConfig{
				Internal: config.InternalConfig{Benthos: []config.BenthosConfig{}},
			}

			tick, err = fsmtest.WaitForBenthosManagerInstanceRemoval(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: emptyCfg, Tick: tick},
				benthosMgr,
				mockSvcRegistry,
				benthosName,
				15,
			)
			Expect(err).NotTo(HaveOccurred())

			//----------------------------------------------------------------------
			// 5. Assertion – did the S6 FSM actually invoke Remove() ?
			//----------------------------------------------------------------------
			Expect(innerMockS6Svc.RemoveCalled).To(BeTrue(),
				"S6Instance.Remove() was never called")

		})
	})

})
