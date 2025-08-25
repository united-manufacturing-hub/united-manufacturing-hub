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

package connection_test

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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/connection"
	connectionsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/connection"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

var _ = Describe("ConnectionManager", func() {
	var (
		manager         *connection.ConnectionManager
		mockService     *connectionsvc.MockConnectionService
		ctx             context.Context
		tick            uint64
		cancel          context.CancelFunc
		mockSvcRegistry *serviceregistry.Registry
	)

	AfterEach(func() {
		cancel()
	})

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Minute)
		tick = 0
		mockSvcRegistry = serviceregistry.NewMockRegistry()
		manager, mockService = fsmtest.CreateMockConnectionManager("test-manager")

		mockService.ExistingConnections = make(map[string]bool)
		mockService.ConnectionStates = make(map[string]*connectionsvc.ServiceInfo)
	})

	Context("Initialization", func() {
		It("should handle empty config without errors", func() {
			emptyConfig := config.FullConfig{Internal: config.InternalConfig{
				Connection: []config.ConnectionConfig{},
			}}

			// Single call to a helper that wraps Reconcile
			newTick, err := fsmtest.WaitForConnectionManagerStable(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: emptyConfig, Tick: tick},
				manager,
				mockSvcRegistry,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())
			Expect(manager.GetInstances()).To(BeEmpty())
		})

		It("should create a service in stopped state and remain stable", func() {
			connectionName := "test-stopped-connection"
			cfg := config.FullConfig{
				Internal: config.InternalConfig{
					Connection: []config.ConnectionConfig{
						fsmtest.CreateConnectionTestConfig(connectionName, connection.OperationalStateStopped),
					},
				},
			}

			// Configure the mock service to allow transition to Stopped
			fsmtest.ConfigureConnectionManagerForState(
				mockService,
				connectionName,
				connection.OperationalStateStopped,
			)

			// Wait for instance creation and stable 'Stopped' state
			newTick, err := fsmtest.WaitForConnectionManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick},
				manager,
				mockSvcRegistry,
				connectionName,
				connection.OperationalStateStopped,
				10,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Double-check the manager state
			inst, exists := manager.GetInstance("connection-" + connectionName)
			Expect(exists).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(connection.OperationalStateStopped))
		})

		It("should create a service in active state and reach up ", func() {
			connectionName := "test-active-connection"
			cfg := config.FullConfig{
				Internal: config.InternalConfig{
					Connection: []config.ConnectionConfig{
						fsmtest.CreateConnectionTestConfig(connectionName, connection.OperationalStateUp),
					},
				},
			}

			// Configure the mock service for transition to Up
			fsmtest.ConfigureConnectionManagerForState(
				mockService,
				connectionName,
				connection.OperationalStateUp,
			)

			newTick, err := fsmtest.WaitForConnectionManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: cfg, Tick: tick},
				manager,
				mockSvcRegistry,
				connectionName,
				connection.OperationalStateUp,
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
				Internal: config.InternalConfig{
					Connection: []config.ConnectionConfig{
						fsmtest.CreateConnectionTestConfig(serviceName, connection.OperationalStateUp),
					},
				},
			}

			fsmtest.ConfigureConnectionManagerForState(
				mockService,
				serviceName,
				connection.OperationalStateUp,
			)

			// 1) Wait for Up
			newTick, err := fsmtest.WaitForConnectionManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{
					CurrentConfig: fullCfg,
					Tick:          tick,
				},
				manager,
				mockSvcRegistry,
				serviceName,
				connection.OperationalStateUp,
				20,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// 2) Now configure for degraded state
			fsmtest.ConfigureConnectionManagerForState(
				mockService,
				serviceName,
				connection.OperationalStateDegraded,
			)

			// Wait for state transition
			newTick, err = fsmtest.WaitForConnectionManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{
					CurrentConfig: fullCfg,
					Tick:          tick,
				},
				manager,
				mockSvcRegistry,
				serviceName,
				connection.OperationalStateDegraded,
				20,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())
			Expect(manager.GetInstances()).To(HaveLen(1))

			// 3) Configure for stopped state before removal
			fsmtest.ConfigureConnectionManagerForState(
				mockService,
				serviceName,
				connection.OperationalStateStopped,
			)
			// Service is stopped at this point

			// Wait for removal
			emptyCfg := config.FullConfig{Internal: config.InternalConfig{
				Connection: []config.ConnectionConfig{},
			}}
			newTick, err = fsmtest.WaitForConnectionManagerInstanceRemoval(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: emptyCfg, Tick: tick},
				manager,
				mockSvcRegistry,
				"connection-"+serviceName,
				20,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())
			Expect(manager.GetInstances()).NotTo(HaveKey("connection-" + serviceName))

		})

		It("should toggle from up/down to stopped and back to up/down with config changes", func() {
			connectionName := "test-toggle-connection"
			activeCfg := fsmtest.CreateConnectionTestConfig(connectionName, connection.OperationalStateUp)

			fullCfg := config.FullConfig{
				Internal: config.InternalConfig{
					Connection: []config.ConnectionConfig{activeCfg},
				},
			}

			// Configure for up state
			fsmtest.ConfigureConnectionManagerForState(
				mockService,
				connectionName,
				connection.OperationalStateUp,
			)

			// Wait for idle
			newTick, err := fsmtest.WaitForConnectionManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{
					CurrentConfig: fullCfg,
					Tick:          tick,
				},
				manager,
				mockSvcRegistry,
				connectionName,
				connection.OperationalStateUp,
				20,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			fsmtest.ConfigureConnectionManagerForState(
				mockService,
				connectionName,
				connection.OperationalStateStopped,
			)
			// Toggle to stopped
			stopCfg := fsmtest.CreateConnectionTestConfig(connectionName, connection.OperationalStateStopped)
			fullCfg.Internal.Connection = []config.ConnectionConfig{stopCfg}

			// Wait for stopped
			newTick, err = fsmtest.WaitForConnectionManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{
					CurrentConfig: fullCfg,
					Tick:          tick,
				},
				manager,
				mockSvcRegistry,
				connectionName,
				connection.OperationalStateStopped,
				20,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			fullCfg.Internal.Connection = []config.ConnectionConfig{activeCfg}
			// Toggle back to active
			fsmtest.ConfigureConnectionManagerForState(
				mockService,
				connectionName,
				connection.OperationalStateUp,
			)

			// Wait for active
			newTick, err = fsmtest.WaitForConnectionManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{
					CurrentConfig: fullCfg,
					Tick:          tick,
				},
				manager,
				mockSvcRegistry,
				connectionName,
				connection.OperationalStateUp,
				20,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

		})
	})

	Context("Multiple Services", func() {
		It("should handle multiple components in parallel, each with its own state", func() {
			conn1Name := "connection1"
			conn2Name := "connection2"

			config1 := fsmtest.CreateConnectionTestConfig(conn1Name, connection.OperationalStateUp)
			config2 := fsmtest.CreateConnectionTestConfig(conn2Name, connection.OperationalStateStopped)
			fullCfg := config.FullConfig{
				Internal: config.InternalConfig{
					Connection: []config.ConnectionConfig{config1, config2},
				},
			}

			// Configure connection1 for Idle, connection2 for Stopped
			fsmtest.ConfigureConnectionManagerForState(mockService, conn1Name, connection.OperationalStateUp)
			fsmtest.ConfigureConnectionManagerForState(mockService, conn2Name, connection.OperationalStateStopped)

			// Wait for both connections to reach their target states
			// Note: This requires a helper like WaitForConnectionManagerMultiState in fsmtest
			// Assuming such a helper exists or can be created similar to the Benthos one.
			newTick, err := fsmtest.WaitForConnectionManagerMultiState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick},
				manager,
				mockSvcRegistry,
				map[string]string{
					conn1Name: connection.OperationalStateUp,
					conn2Name: connection.OperationalStateStopped,
				},
				30, // Attempts
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Confirm manager sees both instances
			Expect(manager.GetInstances()).To(HaveLen(2))
			Expect(manager.GetInstances()).To(HaveKey("connection-" + conn1Name))
			Expect(manager.GetInstances()).To(HaveKey("connection-" + conn2Name))

			// Check the states
			inst1, exists := manager.GetInstance("connection-" + conn1Name)
			Expect(exists).To(BeTrue())
			Expect(inst1.GetCurrentFSMState()).To(Equal(connection.OperationalStateUp))

			inst2, exists := manager.GetInstance("connection-" + conn2Name)
			Expect(exists).To(BeTrue())
			Expect(inst2.GetCurrentFSMState()).To(Equal(connection.OperationalStateStopped))
		})
	})

	Context("Error Handling", func() {
		It("should remove an instance if it hits a permanent error in stopped state", func() {
			serviceName := "perm-error-test"
			fullCfg := config.FullConfig{
				Internal: config.InternalConfig{
					Connection: []config.ConnectionConfig{
						fsmtest.CreateConnectionTestConfig(serviceName, connection.OperationalStateUp),
					},
				},
			}

			// Configure for transition to Up
			fsmtest.ConfigureConnectionManagerForState(mockService, serviceName, connection.OperationalStateUp)

			// Wait for Up
			newTick, err := fsmtest.WaitForConnectionManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick},
				manager,
				mockSvcRegistry,
				serviceName,
				connection.OperationalStateUp,
				20,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Configure for transition to Stopped
			fsmtest.ConfigureConnectionManagerForState(mockService, serviceName, connection.OperationalStateStopped)

			// Set desired state to stopped
			stoppedCfg := config.FullConfig{
				Internal: config.InternalConfig{
					Connection: []config.ConnectionConfig{
						fsmtest.CreateConnectionTestConfig(serviceName, connection.OperationalStateStopped),
					},
				},
			}

			// Wait for stopped
			newTick, err = fsmtest.WaitForConnectionManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: stoppedCfg, Tick: tick},
				manager,
				mockSvcRegistry,
				serviceName,
				connection.OperationalStateStopped,
				20,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			// Simulate a permanent error by configuring the mock service
			mockService.AddConnectionToNmapManagerError = fmt.Errorf("%s: forced error", backoff.PermanentFailureError)

			// Wait for manager to remove instance due to permanent error
			newTick, err = fsmtest.WaitForConnectionManagerInstanceRemoval(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{
					Connection: []config.ConnectionConfig{},
				}}},
				manager,
				mockSvcRegistry,
				serviceName,
				15,
			)
			tick = newTick
			Expect(err).NotTo(HaveOccurred())

			Expect(manager.GetInstances()).NotTo(HaveKey(serviceName))
		})

	})
})
