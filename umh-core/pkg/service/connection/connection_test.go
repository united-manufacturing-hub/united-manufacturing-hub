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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
	nmapfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/nmap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/connection"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/nmap"

	basefsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
)

const (
	targetHost = "localhost"
	targetPort = 502
)

func addConnAndReachState(
	ctx context.Context,
	svc *connection.ConnectionService,
	mgr *nmapfsm.NmapManager,
	name string,
	targetFSMState string,
	mockSvc *nmap.MockNmapService,
	tick *uint64,
) {
	cfg := &connectionserviceconfig.ConnectionServiceConfig{
		NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
			Target: targetHost,
			Port:   targetPort,
		},
	}

	Expect(svc.AddConnection(ctx, nil, cfg, name)).To(Succeed())

	// Create the instance
	err, _ := svc.ReconcileManager(ctx, nil, *tick)
	(*tick)++
	Expect(err).NotTo(HaveOccurred())

	currentState, err := mgr.GetCurrentFSMState(name)
	Expect(err).NotTo(HaveOccurred())
	Expect(currentState).To(Equal(basefsm.LifecycleStateToBeCreated))

	// Walk the manager until the instance is in targetFSMState.
	maxAttempts := 10
	attempts := 0
	for currentState != targetFSMState {
		if attempts >= maxAttempts {
			Fail(fmt.Sprintf("Failed to reach target state %s after %d attempts, current state: %s",
				targetFSMState, maxAttempts, currentState))
		}
		err, _ := svc.ReconcileManager(ctx, nil, *tick)
		Expect(err).NotTo(HaveOccurred())
		(*tick)++
		attempts++

		// Update current state for next iteration check
		currentState, err = mgr.GetCurrentFSMState(name)
		Expect(err).NotTo(HaveOccurred())
	}
}

/* ---------- Status table‑driven spec ------------------------------------ */

var _ = Describe("Connection.Status (table‑driven)", func() {
	var (
		ctx             context.Context
		cancel          context.CancelFunc
		mockMgr         *nmapfsm.NmapManager
		mockNmapService *nmap.MockNmapService
		connSvc         *connection.ConnectionService
		tick            uint64
	)

	BeforeEach(func() {
		mockMgr, mockNmapService = nmapfsm.NewNmapManagerWithMockedService("status‑mgr")
		connSvc = connection.NewDefaultConnectionService(
			"status‑svc",
			connection.WithNmapManager(mockMgr),
		)
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		tick = 1
	})

	AfterEach(func() { cancel() })

	type statusCase struct {
		fsmState    string
		scanState   string
		isHealthy   bool
		portOpen    bool
		reachable   bool
		description string
	}

	statusCases := []statusCase{
		{
			description: "FSM=open, port=open → reachable",
			fsmState:    nmapfsm.OperationalStateOpen,
			scanState:   "open",
			isHealthy:   true, portOpen: true, reachable: true,
		},
		{
			description: "FSM=closed, port=closed → down",
			fsmState:    nmapfsm.OperationalStateClosed,
			scanState:   "closed",
			isHealthy:   true, portOpen: false, reachable: false,
		},
		{
			description: "FSM=filtered, port=filtered → down",
			fsmState:    nmapfsm.OperationalStateFiltered,
			scanState:   "filtered",
			isHealthy:   true, portOpen: false, reachable: false,
		},
		// it is not possible to test a stopped connection, as the manager will always set it to up (except during removal)
		{
			description: "FSM=degraded, port=open (ignored) → down",
			fsmState:    nmapfsm.OperationalStateDegraded,
			scanState:   "open",
			isHealthy:   false, portOpen: true, reachable: false,
		},
	}

	DescribeTable("returns correct flags",
		func(c statusCase) {
			const connName = "plc‑1"

			// 1. plant the scan result the Status() call should read
			mockNmapService.SetServicePortState(connName, c.scanState, 10.0)

			// 2. create connection and drive FSM to required state
			addConnAndReachState(ctx, connSvc, mockMgr, connName, c.fsmState, mockNmapService, &tick)

			// 3. call Status
			info, err := connSvc.Status(ctx, nil, connName, tick)
			Expect(err).NotTo(HaveOccurred(), c.description)

			// 4. verify
			Expect(info.IsHealthy).To(Equal(c.isHealthy), c.description)
			Expect(info.PortStateOpen).To(Equal(c.portOpen), c.description)
			Expect(info.IsReachable).To(Equal(c.reachable), c.description)
		},
		Entry(statusCases[0].description, statusCases[0]),
		Entry(statusCases[1].description, statusCases[1]),
		Entry(statusCases[2].description, statusCases[2]),
		Entry(statusCases[3].description, statusCases[3]),
	)
})

// Common test scaffold for all connection tests
func newConnTestEnv(testName string) (
	ctx context.Context, cancel context.CancelFunc,
	mgr *nmapfsm.NmapManager,
	mockSvc *nmap.MockNmapService,
	connSvc *connection.ConnectionService,
	tick uint64,
) {
	mgr, mockSvc = nmapfsm.NewNmapManagerWithMockedService(testName + "-mgr")
	connSvc = connection.NewDefaultConnectionService(
		testName+"-svc", connection.WithNmapManager(mgr))

	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Minute)
	return ctx, cancel, mgr, mockSvc, connSvc, 1
}

// Helper to reconcile N times
func reconcileN(ctx context.Context, svc *connection.ConnectionService, startTick *uint64, n int) {
	for i := 0; i < n; i++ {
		err, _ := svc.ReconcileManager(ctx, nil, *startTick)
		Expect(err).NotTo(HaveOccurred())
		*startTick++
	}
}

/* ---------- Add Connection tests ---------------------------------------- */

var _ = Describe("Connection.AddConnection", func() {
	var (
		ctx             context.Context
		cancel          context.CancelFunc
		mockMgr         *nmapfsm.NmapManager
		mockNmapService *nmap.MockNmapService
		connSvc         *connection.ConnectionService
		tick            uint64
		connName        string
		connConfig      *connectionserviceconfig.ConnectionServiceConfig
		addErr          error
	)

	BeforeEach(func() {
		ctx, cancel, mockMgr, mockNmapService, connSvc, tick = newConnTestEnv("add-conn")
		connName = "test-conn"
		connConfig = &connectionserviceconfig.ConnectionServiceConfig{
			NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
				Target: targetHost,
				Port:   targetPort,
			},
		}
		addErr = nil
	})

	AfterEach(func() { cancel() })

	Context("with valid config", func() {
		JustBeforeEach(func() {
			mockNmapService.SetServicePortState(connName, "open", 10.0)
			addErr = connSvc.AddConnection(ctx, nil, connConfig, connName)
			reconcileN(ctx, connSvc, &tick, 10)
		})

		It("adds the connection successfully", func() {
			Expect(addErr).NotTo(HaveOccurred())

			// Verify the connection exists
			exists := connSvc.ServiceExists(ctx, nil, connName)
			Expect(exists).To(BeTrue())

			// Verify FSM state after reconciliation
			state, err := mockMgr.GetCurrentFSMState(connName)
			Expect(err).NotTo(HaveOccurred())
			Expect(state).To(Equal(nmapfsm.OperationalStateOpen), "Connection should be in Open state after reconciliation")
		})

		It("rejects duplicate additions", func() {
			// Try to add the same connection again
			err := connSvc.AddConnection(ctx, nil, connConfig, connName)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already exists"))
		})
	})
})

/* ---------- Update Connection tests ------------------------------------- */

var _ = Describe("Connection.UpdateConnection", func() {
	var (
		ctx        context.Context
		cancel     context.CancelFunc
		mockMgr    *nmapfsm.NmapManager
		connSvc    *connection.ConnectionService
		tick       uint64
		connName   string
		connConfig *connectionserviceconfig.ConnectionServiceConfig
		updateErr  error
	)

	BeforeEach(func() {
		ctx, cancel, mockMgr, _, connSvc, tick = newConnTestEnv("update-conn")
		connName = "update-test-conn"
		connConfig = &connectionserviceconfig.ConnectionServiceConfig{
			NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
				Target: targetHost,
				Port:   targetPort,
			},
		}

		// First add the connection
		Expect(connSvc.AddConnection(ctx, nil, connConfig, connName)).To(Succeed())
		reconcileN(ctx, connSvc, &tick, 10)

		// Verify it was added properly
		state, err := mockMgr.GetCurrentFSMState(connName)
		Expect(err).NotTo(HaveOccurred())
		Expect(state).To(Equal(nmapfsm.OperationalStateDegraded))

		updateErr = nil
	})

	AfterEach(func() { cancel() })

	Context("with valid updated config", func() {
		BeforeEach(func() {
			// Update the config
			connConfig.NmapServiceConfig.Port = 503
		})

		JustBeforeEach(func() {
			updateErr = connSvc.UpdateConnection(ctx, nil, connConfig, connName)
			reconcileN(ctx, connSvc, &tick, 10)
		})

		It("updates the connection successfully", func() {
			Expect(updateErr).NotTo(HaveOccurred())

			// Verify the connection still exists
			exists := connSvc.ServiceExists(ctx, nil, connName)
			Expect(exists).To(BeTrue())

			// The manager should have the updated config
			reconcileN(ctx, connSvc, &tick, 10)
			state, err := mockMgr.GetLastObservedState(connName)
			Expect(err).NotTo(HaveOccurred())

			nmapState, ok := state.(nmapfsm.NmapObservedState)
			Expect(ok).To(BeTrue(), "Expected NmapObservedState type")
			Expect(nmapState.ObservedNmapServiceConfig.Port).To(Equal(503))
		})
	})

	Context("for non-existent connection", func() {
		JustBeforeEach(func() {
			updateErr = connSvc.UpdateConnection(ctx, nil, connConfig, "non-existent-conn")
		})

		It("returns an error", func() {
			Expect(updateErr).To(HaveOccurred())
			Expect(updateErr.Error()).To(ContainSubstring("does not exist"))
		})
	})
})

/* ---------- Remove Connection tests ------------------------------------- */

var _ = Describe("Connection.RemoveConnection", func() {
	var (
		ctx        context.Context
		cancel     context.CancelFunc
		mockMgr    *nmapfsm.NmapManager
		connSvc    *connection.ConnectionService
		tick       uint64
		connName   string
		connConfig *connectionserviceconfig.ConnectionServiceConfig
		removeErr  error
	)

	BeforeEach(func() {
		ctx, cancel, mockMgr, _, connSvc, tick = newConnTestEnv("remove-conn")
		connName = "remove-test-conn"
		connConfig = &connectionserviceconfig.ConnectionServiceConfig{
			NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
				Target: targetHost,
				Port:   targetPort,
			},
		}

		// First add the connection
		Expect(connSvc.AddConnection(ctx, nil, connConfig, connName)).To(Succeed())
		reconcileN(ctx, connSvc, &tick, 10)

		// Verify it was added properly
		state, err := mockMgr.GetCurrentFSMState(connName)
		Expect(err).NotTo(HaveOccurred())
		Expect(state).To(Equal(nmapfsm.OperationalStateDegraded))

		removeErr = nil
	})

	AfterEach(func() { cancel() })

	Context("for existing connection", func() {
		JustBeforeEach(func() {
			removeErr = connSvc.RemoveConnection(ctx, nil, connName)
			reconcileN(ctx, connSvc, &tick, 15) // Need more reconciles for removal
		})

		It("removes the connection successfully", func() {
			Expect(removeErr).NotTo(HaveOccurred())

			// Verify the connection no longer exists
			exists := connSvc.ServiceExists(ctx, nil, connName)
			Expect(exists).To(BeFalse())
		})
	})

	Context("for non-existent connection", func() {
		JustBeforeEach(func() {
			removeErr = connSvc.RemoveConnection(ctx, nil, "non-existent-conn")
		})

		It("returns an error", func() {
			Expect(removeErr).To(HaveOccurred())
			Expect(removeErr.Error()).To(ContainSubstring("does not exist"))
		})
	})
})

/* ---------- ServiceExists tests ----------------------------------------- */

var _ = Describe("Connection.ServiceExists", func() {
	var (
		ctx             context.Context
		cancel          context.CancelFunc
		mockNmapService *nmap.MockNmapService
		connSvc         *connection.ConnectionService
		tick            uint64
	)

	BeforeEach(func() {
		ctx, cancel, _, mockNmapService, connSvc, tick = newConnTestEnv("exists-conn")
	})

	AfterEach(func() { cancel() })

	Context("with existing connection", func() {
		const connName = "exists-test-conn"

		BeforeEach(func() {
			cfg := &connectionserviceconfig.ConnectionServiceConfig{
				NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
					Target: targetHost,
					Port:   targetPort,
				},
			}
			Expect(connSvc.AddConnection(ctx, nil, cfg, connName)).To(Succeed())
			reconcileN(ctx, connSvc, &tick, 10)
		})

		It("returns true", func() {
			exists := connSvc.ServiceExists(ctx, nil, connName)
			Expect(exists).To(BeTrue())
		})
	})

	Context("with non-existent connection", func() {
		It("returns false", func() {
			exists := connSvc.ServiceExists(ctx, nil, "non-existent-conn")
			Expect(exists).To(BeFalse())
		})
	})

	// Error injection test
	Context("when service check fails", func() {
		BeforeEach(func() {
			mockNmapService.ExistsError = true
		})

		It("propagates the error", func() {
			exists := connSvc.ServiceExists(ctx, nil, "any-conn")
			Expect(exists).To(BeFalse())
		})
	})
})

/* ---------- ReconcileManager tests -------------------------------------- */

var _ = Describe("Connection.ReconcileManager", func() {
	var (
		ctx     context.Context
		cancel  context.CancelFunc
		mockMgr *nmapfsm.NmapManager
		connSvc *connection.ConnectionService
		tick    uint64
	)

	BeforeEach(func() {
		ctx, cancel, mockMgr, _, connSvc, tick = newConnTestEnv("reconcile-conn")
	})

	AfterEach(func() { cancel() })

	Context("with existing connection", func() {
		const connName = "reconcile-test-conn"

		BeforeEach(func() {
			cfg := &connectionserviceconfig.ConnectionServiceConfig{
				NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
					Target: targetHost,
					Port:   targetPort,
				},
			}
			Expect(connSvc.AddConnection(ctx, nil, cfg, connName)).To(Succeed())
		})

		It("reconciles successfully", func() {
			// First reconcile call
			err, did := connSvc.ReconcileManager(ctx, nil, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(did).To(BeTrue(), "First reconcile should do something")
			tick++

			// Second reconcile call
			err, _ = connSvc.ReconcileManager(ctx, nil, tick)
			Expect(err).NotTo(HaveOccurred())
			tick++

			// Third reconcile call
			err, _ = connSvc.ReconcileManager(ctx, nil, tick)
			Expect(err).NotTo(HaveOccurred())
			tick++

			// Verify connection is in expected state after reconciliation
			state, err := mockMgr.GetCurrentFSMState(connName)
			Expect(err).NotTo(HaveOccurred())
			Expect(state).To(Equal(nmapfsm.OperationalStateStopped))
		})
	})
})

/* ---------- Flaky detection tests --------------------------------------- */

var _ = Describe("Connection.FlakyDetection", func() {
	var (
		ctx             context.Context
		cancel          context.CancelFunc
		mockNmapService *nmap.MockNmapService
		connSvc         *connection.ConnectionService
		tick            uint64
		connName        string
	)

	BeforeEach(func() {
		ctx, cancel, _, mockNmapService, connSvc, tick = newConnTestEnv("flaky-conn")
		connName = "flaky-test-conn"

		// Add a connection for testing
		cfg := &connectionserviceconfig.ConnectionServiceConfig{
			NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
				Target: targetHost,
				Port:   targetPort,
			},
		}
		Expect(connSvc.AddConnection(ctx, nil, cfg, connName)).To(Succeed())
		reconcileN(ctx, connSvc, &tick, 10)
	})

	AfterEach(func() { cancel() })

	Context("with stable connection history", func() {
		BeforeEach(func() {
			// Plant consistent "open" state in the history
			for i := 0; i < 5; i++ {
				mockNmapService.SetServicePortState(connName, "open", 10.0)
			}
		})

		It("reports not flaky", func() {
			info, err := connSvc.Status(ctx, nil, connName, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.IsFlaky).To(BeFalse(), "Connection with stable history should not be flaky")
		})
	})

	Context("with mixed connection history", func() {
		BeforeEach(func() {
			// Plant mixed state history (alternating open/closed)
			mockNmapService.SetServicePortState(connName, "open", 10.0)
			reconcileN(ctx, connSvc, &tick, 1)
			_, err := connSvc.Status(ctx, nil, connName, tick)
			Expect(err).NotTo(HaveOccurred())
			tick++
			mockNmapService.SetServicePortState(connName, "closed", 10.0)
			reconcileN(ctx, connSvc, &tick, 1)
			_, err = connSvc.Status(ctx, nil, connName, tick)
			Expect(err).NotTo(HaveOccurred())
			tick++
			mockNmapService.SetServicePortState(connName, "open", 10.0)
			reconcileN(ctx, connSvc, &tick, 1)
			_, err = connSvc.Status(ctx, nil, connName, tick)
			Expect(err).NotTo(HaveOccurred())
			tick++
			mockNmapService.SetServicePortState(connName, "closed", 10.0)
			reconcileN(ctx, connSvc, &tick, 1)
			_, err = connSvc.Status(ctx, nil, connName, tick)
			Expect(err).NotTo(HaveOccurred())
			tick++
			mockNmapService.SetServicePortState(connName, "open", 10.0)
			reconcileN(ctx, connSvc, &tick, 1)
			_, err = connSvc.Status(ctx, nil, connName, tick)
			Expect(err).NotTo(HaveOccurred())
			tick++
		})

		It("reports flaky", func() {
			info, err := connSvc.Status(ctx, nil, connName, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.IsFlaky).To(BeTrue(), "Connection with mixed history should be flaky")
		})
	})
})

/* ---------- History size trimming tests --------------------------------- */

var _ = Describe("Connection.HistorySizeTrimming", func() {
	var (
		ctx             context.Context
		cancel          context.CancelFunc
		mockMgr         *nmapfsm.NmapManager
		mockNmapService *nmap.MockNmapService
		connSvc         *connection.ConnectionService
		tick            uint64
		connName        string
	)

	const maxHistorySize = 3

	BeforeEach(func() {
		ctx, cancel, mockMgr, mockNmapService, connSvc, tick = newConnTestEnv("history-conn")
		mockMgr, mockNmapService = nmapfsm.NewNmapManagerWithMockedService("history-mgr")

		// Create service with limited history size
		connSvc = connection.NewDefaultConnectionService(
			"history-svc",
			connection.WithNmapManager(mockMgr),
			connection.WithMaxRecentScans(maxHistorySize),
		)

		tick = 1
		connName = "history-test-conn"

		// Add a connection for testing
		cfg := &connectionserviceconfig.ConnectionServiceConfig{
			NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
				Target: targetHost,
				Port:   targetPort,
			},
		}
		Expect(connSvc.AddConnection(ctx, nil, cfg, connName)).To(Succeed())
		reconcileN(ctx, connSvc, &tick, 10)
	})

	AfterEach(func() { cancel() })

	It("maintains limited history size", func() {
		// Add 5 scan results (more than maxHistorySize)
		mockNmapService.SetServicePortState(connName, "open", 10.0)
		reconcileN(ctx, connSvc, &tick, 1)
		_, err := connSvc.Status(ctx, nil, connName, tick)
		Expect(err).NotTo(HaveOccurred())
		tick++
		mockNmapService.SetServicePortState(connName, "open", 20.0)
		reconcileN(ctx, connSvc, &tick, 1)
		_, err = connSvc.Status(ctx, nil, connName, tick)
		Expect(err).NotTo(HaveOccurred())
		tick++
		mockNmapService.SetServicePortState(connName, "open", 30.0)
		reconcileN(ctx, connSvc, &tick, 1)
		_, err = connSvc.Status(ctx, nil, connName, tick)
		Expect(err).NotTo(HaveOccurred())
		tick++
		mockNmapService.SetServicePortState(connName, "open", 40.0)
		reconcileN(ctx, connSvc, &tick, 1)
		_, err = connSvc.Status(ctx, nil, connName, tick)
		Expect(err).NotTo(HaveOccurred())
		tick++
		mockNmapService.SetServicePortState(connName, "open", 50.0)
		reconcileN(ctx, connSvc, &tick, 1)
		_, err = connSvc.Status(ctx, nil, connName, tick)
		Expect(err).NotTo(HaveOccurred())
		tick++

		Expect(connSvc.GetRecentScansCount(connName)).To(Equal(maxHistorySize),
			"History should be limited to maxHistorySize")

		// The most recent scans should be kept (highest time values)
		scan, ok := connSvc.GetRecentScanAtIndex(connName, 0)
		Expect(ok).To(BeTrue())
		Expect(scan.NmapStatus.LastScan.PortResult.LatencyMs).To(BeNumerically(">=", 30.0),
			"Only the most recent scans should be kept")
	})
})

/* ---------- LastChange semantics tests ---------------------------------- */

var _ = Describe("Connection.LastChangeSemantics", func() {
	var (
		ctx             context.Context
		cancel          context.CancelFunc
		mockNmapService *nmap.MockNmapService
		connSvc         *connection.ConnectionService
		tick            uint64
		connName        string
	)

	BeforeEach(func() {
		ctx, cancel, _, mockNmapService, connSvc, tick = newConnTestEnv("lastchange-conn")
		connName = "lastchange-test-conn"

		// Add a connection for testing
		cfg := &connectionserviceconfig.ConnectionServiceConfig{
			NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
				Target: targetHost,
				Port:   targetPort,
			},
		}
		Expect(connSvc.AddConnection(ctx, nil, cfg, connName)).To(Succeed())
		reconcileN(ctx, connSvc, &tick, 3)

		// Ensure we're in open state initially
		mockNmapService.SetServicePortState(connName, "open", 10.0)
		reconcileN(ctx, connSvc, &tick, 1)
		info, err := connSvc.Status(ctx, nil, connName, tick)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.PortStateOpen).To(BeTrue())
	})

	AfterEach(func() { cancel() })

	It("updates LastChange only on actual state transitions", func() {
		reconcileN(ctx, connSvc, &tick, 1)
		tick++

		// Initial state capture
		initialInfo, err := connSvc.Status(ctx, nil, connName, tick)
		Expect(err).NotTo(HaveOccurred())
		initialLastChange := initialInfo.LastChange

		// Change the state to closed
		mockNmapService.SetServicePortState(connName, "closed", 20.0)
		reconcileN(ctx, connSvc, &tick, 1)
		tick++

		// Check status after state change
		afterChangeInfo, err := connSvc.Status(ctx, nil, connName, tick)
		Expect(err).NotTo(HaveOccurred())
		Expect(afterChangeInfo.PortStateOpen).To(BeFalse(), "Port state should have changed")
		Expect(afterChangeInfo.LastChange).To(Equal(tick),
			"LastChange should be updated on state transition")
		Expect(afterChangeInfo.LastChange).To(BeNumerically(">", initialLastChange),
			"New LastChange should be greater than initial")

		// Record the timestamp after change
		stateChangeTimestamp := afterChangeInfo.LastChange

		// Update with the same state (closed)
		mockNmapService.SetServicePortState(connName, "closed", 30.0)
		reconcileN(ctx, connSvc, &tick, 1)
		tick++

		// Check status after same-state update
		afterSameInfo, err := connSvc.Status(ctx, nil, connName, tick)
		Expect(err).NotTo(HaveOccurred())
		Expect(afterSameInfo.PortStateOpen).To(BeFalse(), "Port should still be closed")
		Expect(afterSameInfo.LastChange).To(Equal(stateChangeTimestamp),
			"LastChange should not update when state didn't change")
	})
})
