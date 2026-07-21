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

// Package supervisor internal test for ENG-4971: Run(ctx) must drain a LIVE
// tick loop when the caller's ctx is cancelled. Production wires Run to a
// signal.NotifyContext root ctx (cmd/main.go), so the first SIGTERM cancels
// the caller ctx; if the tick loop dies with it, Shutdown's drain waits on
// worker removals only the dead loop could perform — every level burns its
// full budget, fires graceful_shutdown_timeout, and docker SIGKILLs the
// process mid-drain. This test is the ticket's acceptance gate: ctx-cancel
// combined with live workers nested across supervisor levels.
package supervisor_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
)

// Drain arithmetic: every worker honors shutdown promptly
// (shutdownHonoringState emits SignalNeedsRemoval on the first post-request
// tick), so against a LIVE tick loop the whole depth-3 tree drains in a few
// tick intervals — far inside the budgets. Only a dead tick loop can make a
// level burn its full budget, so a graceful_shutdown_timeout here is the
// dead-loop signature, not a tight-margin flake. The base is kept small so
// the dead-loop failure mode (≈3×base: root samples 3×base, the cascaded
// child drains spend 2×base of it) stays fast.
const (
	runDetachDrainBase    = 1 * time.Second
	runDetachTickInterval = 50 * time.Millisecond // ≪ base

	runDetachRootType = "rundetach_root"
	runDetachMidType  = "rundetach_mid"
	runDetachLeafType = "rundetach_leaf"
)

var _ = Describe("Run(ctx) cancel with live nested workers (ENG-4971)", func() {
	var buf *shutdownTestSyncBuffer

	BeforeEach(func() {
		buf = &shutdownTestSyncBuffer{}

		// Mid worker: prompt-stopping, declares the leaf child so the
		// supervisor builds a real nested tree through reconcileChildren.
		_ = factory.RegisterFactoryByType(runDetachMidType, func(identity deps.Identity, _ deps.FSMLogger, _ deps.StateReader, _ map[string]any) fsmv2.Worker {
			return &cascadeTreeWorker{
				id:           identity.ID,
				initialState: shutdownHonoringState{},
				childrenSpecs: []config.ChildSpec{
					{Name: "leaf", WorkerType: runDetachLeafType, UserSpec: config.UserSpec{}, Enabled: true},
				},
			}
		})

		// Leaf worker: prompt-stopping.
		_ = factory.RegisterFactoryByType(runDetachLeafType, func(identity deps.Identity, _ deps.FSMLogger, _ deps.StateReader, _ map[string]any) fsmv2.Worker {
			return &cascadeTreeWorker{
				id:           identity.ID,
				initialState: shutdownHonoringState{},
			}
		})

		for _, wt := range []string{runDetachMidType, runDetachLeafType} {
			_ = factory.RegisterSupervisorFactoryByType(wt, func(cfg interface{}) interface{} {
				return supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](cfg.(supervisor.Config))
			})
		}
	})

	AfterEach(func() {
		factory.ResetRegistry()
	})

	// ENG-4971 acceptance gate, verbatim from the ticket: "a test that
	// combines ctx-cancel-then-Shutdown with LIVE workers". Run's public
	// contract — run until ctx cancels, shut down gracefully, return when
	// done — requires the graceful drain to execute against a tick loop that
	// is still alive when the caller's ctx is cancelled.
	It("drains the live tree gracefully when the caller ctx is cancelled, without graceful_shutdown_timeout", func() {
		ctx := context.Background()

		basicStore := memory.NewInMemoryStore()
		for _, wt := range []string{runDetachRootType, runDetachMidType, runDetachLeafType} {
			for _, suffix := range []string{"_identity", "_desired", "_observed"} {
				Expect(basicStore.CreateCollection(ctx, wt+suffix, nil)).To(Succeed())
			}
		}
		triangularStore := storage.NewTriangularStore(basicStore, deps.NewNopFSMLogger())

		logger := deps.NewJSONFSMLogger(buf, deps.LevelDebug)

		root := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
			WorkerType:              runDetachRootType,
			Store:                   triangularStore,
			Logger:                  logger,
			TickInterval:            runDetachTickInterval,
			GracefulShutdownTimeout: runDetachDrainBase,
		})

		rootIdentity := deps.Identity{
			ID:            "rundetach-root-worker",
			Name:          "Run Detach Root Worker",
			WorkerType:    runDetachRootType,
			HierarchyPath: "rundetach-root-worker(" + runDetachRootType + ")",
		}
		rootWorker := &cascadeTreeWorker{
			id:           rootIdentity.ID,
			initialState: shutdownHonoringState{},
			childrenSpecs: []config.ChildSpec{
				{Name: "mid", WorkerType: runDetachMidType, UserSpec: config.UserSpec{}, Enabled: true},
			},
		}
		Expect(root.AddWorker(rootIdentity, rootWorker)).To(Succeed())

		desiredDoc := persistence.Document{
			supervisor.FieldID:  rootIdentity.ID,
			"ShutdownRequested": false,
			"state":             "running",
		}
		_, err := triangularStore.SaveDesired(ctx, runDetachRootType, rootIdentity.ID, desiredDoc)
		Expect(err).ToNot(HaveOccurred())

		// Production composition (cmd/main.go): a cancellable root ctx
		// (signal.NotifyContext) handed to Run; the first SIGTERM cancels it.
		runCtx, cancel := context.WithCancel(context.Background())
		defer cancel()

		runResult := make(chan error, 1)

		go func() {
			runResult <- root.Run(runCtx)
		}()

		defer root.Shutdown() // Idempotent; reaps the tree if an assertion fails early.

		// Settle gate: the full depth-3 tree is alive AND every level's worker
		// is observably running — polled from supervisor state, never a blind
		// sleep (a blind sleep was a flake source here before).
		var midSup, leafSup supervisor.SupervisorInterface
		Eventually(func() bool {
			if root.GetCurrentStateName() != "running" {
				return false
			}
			children := root.GetChildren()
			mid, ok := children["mid"]
			if !ok || len(mid.ListWorkers()) != 1 || mid.GetCurrentStateName() != "running" {
				return false
			}
			grandchildren := mid.GetChildren()
			leaf, ok := grandchildren["leaf"]
			if !ok || len(leaf.ListWorkers()) != 1 || leaf.GetCurrentStateName() != "running" {
				return false
			}
			midSup = mid
			leafSup = leaf

			return true
		}, 5*time.Second, 20*time.Millisecond).Should(BeTrue(),
			"depth-3 tree (root worker → mid supervisor → leaf supervisor) never fully spawned and settled into running")

		// The trigger under test: caller-ctx cancellation (production's first
		// SIGTERM). The orderly drain must now run against the LIVE tick loop.
		cancel()

		// Run must return once the graceful shutdown completes. A live drain of
		// prompt-stopping workers needs only a few ticks; the bound leaves room
		// for the dead-loop failure mode (≈3×base) to complete so the log
		// assertions below report the real signature instead of a timeout here.
		Eventually(runResult, 3*runDetachDrainBase+2*time.Second).Should(Receive(BeNil()),
			"Run did not return after ctx cancellation")

		logOutput := buf.String()

		// THE dead-loop signature: with the tick loop killed by the caller-ctx
		// cancel, no level can reap its workers, so every level burns its full
		// budget and warns. A live drain of prompt-stopping workers never
		// reaches any budget.
		Expect(containsLogEvent(logOutput, "graceful_shutdown_timeout")).To(BeFalse(),
			"graceful_shutdown_timeout was logged: Run's ctx cancel killed the tick loop before Shutdown drained, "+
				"so the drain waited on worker removals only the dead loop could perform (ENG-4971 dead-loop drain)")
		Expect(containsLogEvent(logOutput, "graceful_shutdown_budget_exhausted")).To(BeFalse(),
			"graceful_shutdown_budget_exhausted was logged: child drains burned the whole budget, "+
				"which a live drain of prompt-stopping workers never does")

		// Orderly-drain markers: Shutdown ran, and the root's own worker drain
		// completed via live-loop reaping (the workers-removed debug line only
		// logs when the drain loop observes zero workers before any budget fires).
		Expect(containsLogEvent(logOutput, "supervisor_shutting_down")).To(BeTrue(),
			"supervisor_shutting_down was never logged: ctx cancellation did not trigger an orderly Shutdown")
		Expect(containsLogEvent(logOutput, "graceful_shutdown_workers_removed")).To(BeTrue(),
			"graceful_shutdown_workers_removed was never logged: the drain never saw the tick loop reap the workers — "+
				"the loop was already dead when Shutdown's drain ran (ENG-4971)")

		// Every level fully drained — no worker left behind a dead-loop drain.
		rootRemaining := root.TestWorkerCount()
		Expect(rootRemaining).To(BeZero(), "root supervisor leaked workers after Run returned")
		Expect(midSup.ListWorkers()).To(BeEmpty(), "mid supervisor leaked workers after Run returned")
		Expect(leafSup.ListWorkers()).To(BeEmpty(), "leaf supervisor leaked workers after Run returned")
	})
})
