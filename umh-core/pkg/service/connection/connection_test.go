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
	tick uint64,
) {
	cfg := &connectionserviceconfig.ConnectionServiceConfig{
		NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
			Target: targetHost,
			Port:   targetPort,
		},
	}

	Expect(svc.AddConnection(ctx, nil, cfg, name)).To(Succeed())

	// Create the instance
	err, _ := svc.ReconcileManager(ctx, nil, tick)
	tick++
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
		err, _ := svc.ReconcileManager(ctx, nil, tick)
		Expect(err).NotTo(HaveOccurred())
		tick++
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
			addConnAndReachState(ctx, connSvc, mockMgr, connName, c.fsmState, mockNmapService, tick)

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
