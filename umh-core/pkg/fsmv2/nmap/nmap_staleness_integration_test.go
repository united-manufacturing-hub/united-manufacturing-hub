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

package fsmv2nmap_test

// Integration test for the NMAP_BACKEND=fsmv2 stale-observation bug (ENG,
// edit-protocol-converter connection-only edit).
//
// Symptom: editing a bridge's connection to a KNOWN-BAD target briefly reports
// a healthy/open connection ONCE, so EditProtocolConverterAction.awaitRollout
// (pkg/communicator/actions/edit-protocolconverter.go, DFCTypeEmpty path) sees
// active/idle and returns success instead of failing. Only reproduces with the
// fsmv2 nmap backend; fsmv1 is fine.
//
// Root cause exercised here: the fsmv2 adapter reads the child observation by
// ref {WorkerType:"nmap", Name}. The name does not change when only the target
// changes, so the ref (and its CSE key) is identical across the edit. The prior
// scan of the GOOD target is still in the store with a recent CollectedAt, so
// fsmv2client.GetFresh classifies it Fresh purely by age
// (pkg/fsmv2/fsmv2client/fsmv2client.go:163) — it never checks whether the
// observation belongs to the current target. AdaptedInstance.resolve therefore
// runs the Fresh rung and mapFresh maps the stale "open" to OperationalStateOpen
// (pkg/fsmv2/adapter/instance.go:230, pkg/fsmv2/nmap/manager.go:76). The
// connection reads healthy until the worker re-polls the bad target.
//
// This test drives the REAL read path: a real TriangularStore over an in-memory
// persistence backend, a real fsmv2client, and the real adapter WorkerManager
// built by NewFsmv2NmapManager. It writes observations exactly as the collector
// does (SaveObserved), so nothing here is stubbed except the poll cadence, which
// is stepped by hand to make the transient window deterministic.

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	publicfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	nmapfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/nmap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/adapter"
	fsmv2config "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	fsmv2nmap "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/nmap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/testutil"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
)

var _ = Describe("NMAP_BACKEND=fsmv2 stale observation across a connection edit", func() {
	const (
		name        = "bridge-conn"
		port uint16 = 502

		goodTarget = "192.0.2.10" // stands in for the healthy target
		badTarget  = "192.0.2.99" // the known-bad target the edit points at
	)

	var (
		ctx   context.Context
		store *storage.TriangularStore
		mgr   *adapter.WorkerManager[config.NmapConfig, simple.Status[fsmv2nmap.NmapStatus]]
	)

	// nmapConfig builds an enabled (running) config.NmapConfig for the given target.
	nmapConfig := func(target string) config.NmapConfig {
		return config.NmapConfig{
			FSMInstanceConfig: config.FSMInstanceConfig{
				Name:            name,
				DesiredFSMState: "running",
			},
			NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
				Target: target,
				Port:   port,
			},
		}
	}

	// snapshotWith puts the configs at the path the manager reads.
	snapshotWith := func(cfgs ...config.NmapConfig) publicfsm.SystemSnapshot {
		return publicfsm.SystemSnapshot{
			CurrentConfig: config.FullConfig{
				Internal: config.InternalConfig{Nmap: cfgs},
			},
		}
	}

	// writeScan persists an observation the way the collector does: an
	// Observation[simple.Status[NmapStatus]] under the ref's CSE key
	// (workerType "nmap", id ChildID(name)), stamped now so it reads Fresh.
	writeScan := func(portState string, running bool) {
		obs := fsmv2.Observation[simple.Status[fsmv2nmap.NmapStatus]]{
			CollectedAt: time.Now(),
			Status: simple.Status[fsmv2nmap.NmapStatus]{
				Result: fsmv2nmap.NmapStatus{
					PortState: portState,
					IsRunning: running,
					Port:      port,
				},
			},
		}

		_, err := store.SaveObserved(ctx, fsmv2nmap.WorkerType, fsmv2config.ChildID(name), obs)
		Expect(err).NotTo(HaveOccurred())
	}

	// currentState reads the fsmv1 operational state the connection FSM would see.
	currentState := func() string {
		state, err := mgr.GetCurrentFSMState(name)
		Expect(err).NotTo(HaveOccurred())

		return state
	}

	BeforeEach(func() {
		ctx = context.Background()

		// Real store + real client (writer for child specs, store as StateReader).
		store = testutil.CreateTriangularStoreForWorkerType(fsmv2nmap.WorkerType)
		writer := dynamicchildren.NewWriter()
		fsmv2client.SetClient(fsmv2client.NewFSMv2Client(writer, store))

		mgr = fsmv2nmap.NewFsmv2NmapManager("staleness-test")
	})

	AfterEach(func() {
		fsmv2client.SetClient(nil)
	})

	It("reports the connection open once after the target is edited to a bad host (BUG)", func() {
		// 1. Deploy the healthy connection: reconcile upserts the worker, and its
		//    first scan of the good target lands open.
		err, _ := mgr.Reconcile(ctx, snapshotWith(nmapConfig(goodTarget)), nil)
		Expect(err).NotTo(HaveOccurred())

		writeScan(string(nmapfsm.PortStateOpen), true)
		Expect(currentState()).To(Equal(nmapfsm.OperationalStateOpen),
			"the healthy connection must read open before the edit")

		// 2. Edit the connection to a known-bad target. Only the target changed,
		//    so the worker name — and therefore the ref/CSE key — is identical.
		//    The worker has not re-polled the bad target yet, so the store still
		//    holds the open scan of the GOOD target.
		err, _ = mgr.Reconcile(ctx, snapshotWith(nmapConfig(badTarget)), nil)
		Expect(err).NotTo(HaveOccurred())

		// 3. A target change must invalidate the prior observation: the manager
		//    must NOT serve the open scan of the OLD target for the new bad
		//    target. Until the bad target is actually scanned this must read
		//    starting/degraded, never open.
		//
		//    This currently FAILS (RED): the stale open scan is still Fresh
		//    (recent CollectedAt) and keyed by the unchanged ref, so the manager
		//    reports open — the single healthy tick that lets awaitRollout's
		//    DFCTypeEmpty path treat the edit as succeeded.
		Expect(currentState()).NotTo(Equal(nmapfsm.OperationalStateOpen),
			"a connection edited to a bad target must not read open from the old target's stale scan")

		// 4. Once the worker actually scans the bad target (closed), the manager
		//    corrects — confirming the earlier open was purely the stale window.
		writeScan(string(nmapfsm.PortStateClosed), false)
		Expect(currentState()).To(Equal(nmapfsm.OperationalStateClosed),
			"after the bad target is scanned the connection reads closed")
	})
})
