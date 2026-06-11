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

// Package supervisor internal test for the graceful-drain budget cascading
// contract (pkg/fsmv2/CLAUDE.md §"Graceful Shutdown Cascading"): each level
// samples base × subtree height at Shutdown entry and its child drains spend
// from that same budget, so per-level budgets do not sum — a chain of depth N
// drains within N×base total (depth 1 = base, depth 2 = 2×base, depth 3 =
// 3×base).
package supervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
)

// Budget arithmetic for the cascade test. The slow worker's stop duration is
// derived from the base so the outcome is forced by arithmetic, not
// scheduling: stop = 1.1×base sits strictly between the flat budget (1×base,
// would warn) and the cascaded depth-2 budget (2×base, must complete
// warn-free). Scheduler and observation lag delay the stop relative to the
// already-armed timer — away from the flat 1×base budget (so a flat-budget
// regression cannot falsely PASS) but TOWARD the cascaded 2×base expiry. The
// 0.9×base warn-free margin between the stop and that expiry IS the flake
// protection: it absorbs tick/poll lag and the leaf drain's spend, which
// would otherwise present as a spurious warn — indistinguishable from a
// product bug.
// The documented total for a depth-3 chain is 3×base.
const (
	cascadeDrainBase    = 2 * time.Second
	cascadeSlowStop     = 11 * cascadeDrainBase / 10 // 1.1×base
	cascadeTickInterval = 50 * time.Millisecond      // ≪ base

	// perLevelTeardownSlop widens wall-clock bounds for what the drain
	// budget does not cover, once per supervisor torn down in sequence:
	// Phase-4 teardown (tickLoop join, metrics reporter, collector/executor
	// stops), the 100ms drain-ticker granularity, and scheduler delays on
	// loaded CI runners. Each bound multiplies it by the number of
	// supervisors that tear down sequentially in its scenario, keeping
	// margins uniform. Never widen the budget constants instead — the
	// forcing arithmetic rides on them.
	perLevelTeardownSlop = 500 * time.Millisecond

	// cascadeTruncationBase keeps the truncation case fast: no forcing
	// arithmetic rides on it because the stuck worker never stops at any
	// budget; the tick interval just has to stay ≪ base.
	cascadeTruncationBase = 1 * time.Second

	cascadeDrainRootType = "cascadedrain_root"
	cascadeDrainMidType  = "cascadedrain_mid"
	cascadeDrainLeafType = "cascadedrain_leaf"
	cascadeStuckMidType  = "cascadedrain_stuckmid"
	cascadeStuckLeafType = "cascadedrain_stuckleaf"
)

// findLogEvents returns every JSON log entry with the given msg, in buffer
// order. The single JSON-line parse site: containsLogEvent and findLogEvent
// (lifecycle_shutdown_test.go) wrap it, so a log-format drift breaks every
// helper at once instead of leaving an absence-assertion vacuously green.
func findLogEvents(logOutput, msg string) []map[string]interface{} {
	var entries []map[string]interface{}

	for _, line := range strings.Split(logOutput, "\n") {
		if line == "" {
			continue
		}

		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		if entry["msg"] == msg {
			entries = append(entries, entry)
		}
	}

	return entries
}

// slowRemovalState honors ShutdownRequested like shutdownHonoringState, but
// only emits SignalNeedsRemoval once stopAfter has elapsed since it first
// observed the shutdown request. This models a graceful drain that needs
// 1.1×base — longer than one flat budget.
type slowRemovalState struct {
	stopSeen  time.Time
	stopAfter time.Duration
	mu        sync.Mutex
}

func (s *slowRemovalState) String() string { return "running" }

func (s *slowRemovalState) LifecyclePhase() config.LifecyclePhase {
	return config.PhaseRunningHealthy
}

func (s *slowRemovalState) Next(snapshot any) fsmv2.NextResult[any, any] {
	snap, ok := snapshot.(fsmv2.Snapshot)
	if !ok {
		return fsmv2.NextResult[any, any]{Signal: fsmv2.SignalNone, State: s, Reason: "no snapshot"}
	}

	ds, ok := snap.Desired.(fsmv2.DesiredState)
	if !ok || !ds.IsShutdownRequested() {
		return fsmv2.NextResult[any, any]{Signal: fsmv2.SignalNone, State: s, Reason: "running"}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopSeen.IsZero() {
		s.stopSeen = time.Now()
	}

	if time.Since(s.stopSeen) >= s.stopAfter {
		return fsmv2.NextResult[any, any]{Signal: fsmv2.SignalNeedsRemoval, State: s, Reason: "slow stop complete"}
	}

	return fsmv2.NextResult[any, any]{
		Signal: fsmv2.SignalNone,
		State:  s,
		Reason: fmt.Sprintf("stopping slowly: %s of %s elapsed", time.Since(s.stopSeen).Round(time.Millisecond), s.stopAfter),
	}
}

// ignoresShutdownState never emits SignalNeedsRemoval: it models a stuck
// worker whose level must exhaust its cascaded budget, warn, and break out.
type ignoresShutdownState struct{}

func (ignoresShutdownState) String() string { return "running" }

func (ignoresShutdownState) LifecyclePhase() config.LifecyclePhase {
	return config.PhaseRunningHealthy
}

func (s ignoresShutdownState) Next(_ any) fsmv2.NextResult[any, any] {
	return fsmv2.NextResult[any, any]{Signal: fsmv2.SignalNone, State: s, Reason: "ignoring shutdown"}
}

// cascadeTreeWorker is a minimal fsmv2.Worker that declares a static child set
// so the supervisor builds a real nested tree through reconcileChildren.
type cascadeTreeWorker struct {
	initialState  fsmv2.State[any, any]
	id            string
	childrenSpecs []config.ChildSpec
}

func (w *cascadeTreeWorker) CollectObservedState(_ context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	return &TestObservedState{
		ID:          w.id,
		CollectedAt: time.Now(),
		Desired:     &TestDesiredState{},
	}, nil
}

func (w *cascadeTreeWorker) DeriveDesiredState(_ interface{}) (fsmv2.DesiredState, error) {
	return &config.DesiredState{
		State:         "running",
		ChildrenSpecs: w.childrenSpecs,
	}, nil
}

func (w *cascadeTreeWorker) GetInitialState() fsmv2.State[any, any] {
	return w.initialState
}

var _ = Describe("Graceful drain budget cascading (CLAUDE.md §Graceful Shutdown Cascading)", func() {
	var buf *shutdownTestSyncBuffer

	BeforeEach(func() {
		buf = &shutdownTestSyncBuffer{}

		// Mid worker: slow-stopping. Its graceful stop needs 1.1×base — more
		// than the flat budget, comfortably less than its cascaded 2×base
		// budget (it has the leaf supervisor below it, subtree height 2).
		_ = factory.RegisterFactoryByType(cascadeDrainMidType, func(identity deps.Identity, _ deps.FSMLogger, _ deps.StateReader, _ map[string]any) fsmv2.Worker {
			return &cascadeTreeWorker{
				id:           identity.ID,
				initialState: &slowRemovalState{stopAfter: cascadeSlowStop},
				childrenSpecs: []config.ChildSpec{
					{Name: "leaf", WorkerType: cascadeDrainLeafType, UserSpec: config.UserSpec{}, Enabled: true},
				},
			}
		})

		// Stuck mid worker: same position in the tree, but never honors
		// ShutdownRequested — its level must exhaust the cascaded budget.
		_ = factory.RegisterFactoryByType(cascadeStuckMidType, func(identity deps.Identity, _ deps.FSMLogger, _ deps.StateReader, _ map[string]any) fsmv2.Worker {
			return &cascadeTreeWorker{
				id:           identity.ID,
				initialState: ignoresShutdownState{},
				childrenSpecs: []config.ChildSpec{
					{Name: "leaf", WorkerType: cascadeDrainLeafType, UserSpec: config.UserSpec{}, Enabled: true},
				},
			}
		})

		// Leaf worker: stops promptly when ShutdownRequested arrives.
		_ = factory.RegisterFactoryByType(cascadeDrainLeafType, func(identity deps.Identity, _ deps.FSMLogger, _ deps.StateReader, _ map[string]any) fsmv2.Worker {
			return &cascadeTreeWorker{
				id:           identity.ID,
				initialState: shutdownHonoringState{},
			}
		})

		for _, wt := range []string{cascadeDrainMidType, cascadeDrainLeafType, cascadeStuckMidType} {
			_ = factory.RegisterSupervisorFactoryByType(wt, func(cfg interface{}) interface{} {
				return NewSupervisor[*TestObservedState, *TestDesiredState](cfg.(Config))
			})
		}
	})

	AfterEach(func() {
		factory.ResetRegistry()
	})

	It("drains a depth-3 tree warn-free within the documented 3×base total", func() {
		ctx := context.Background()

		// One shared store with collections for every level's worker type.
		basicStore := memory.NewInMemoryStore()
		for _, wt := range []string{cascadeDrainRootType, cascadeDrainMidType, cascadeDrainLeafType} {
			for _, suffix := range []string{"_identity", "_desired", "_observed"} {
				Expect(basicStore.CreateCollection(ctx, wt+suffix, nil)).To(Succeed())
			}
		}
		triangularStore := storage.NewTriangularStore(basicStore, deps.NewNopFSMLogger())

		logger := deps.NewJSONFSMLogger(buf, deps.LevelDebug)

		root := NewSupervisor[*TestObservedState, *TestDesiredState](Config{
			WorkerType:              cascadeDrainRootType,
			Store:                   triangularStore,
			Logger:                  logger,
			TickInterval:            cascadeTickInterval,
			GracefulShutdownTimeout: cascadeDrainBase,
		})

		rootIdentity := deps.Identity{
			ID:            "cascade-root-worker",
			Name:          "Cascade Root Worker",
			WorkerType:    cascadeDrainRootType,
			HierarchyPath: "cascade-root-worker(" + cascadeDrainRootType + ")",
		}
		rootWorker := &cascadeTreeWorker{
			id:           rootIdentity.ID,
			initialState: shutdownHonoringState{},
			childrenSpecs: []config.ChildSpec{
				{Name: "mid", WorkerType: cascadeDrainMidType, UserSpec: config.UserSpec{}, Enabled: true},
			},
		}
		Expect(root.AddWorker(rootIdentity, rootWorker)).To(Succeed())

		desiredDoc := persistence.Document{
			FieldID:             rootIdentity.ID,
			"ShutdownRequested": false,
			"state":             "running",
		}
		_, err := triangularStore.SaveDesired(ctx, cascadeDrainRootType, rootIdentity.ID, desiredDoc)
		Expect(err).ToNot(HaveOccurred())

		runCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		_ = root.Start(runCtx)
		defer root.Shutdown() // Idempotent; reaps the tree if an assertion fails early.

		// Wait until the full depth-3 tree is alive: root worker, mid
		// supervisor + worker, leaf supervisor + worker.
		var midSup, leafSup SupervisorInterface
		Eventually(func() bool {
			children := root.GetChildren()
			mid, ok := children["mid"]
			if !ok || len(mid.ListWorkers()) != 1 {
				return false
			}
			grandchildren := mid.GetChildren()
			leaf, ok := grandchildren["leaf"]
			if !ok || len(leaf.ListWorkers()) != 1 {
				return false
			}
			midSup = mid
			leafSup = leaf

			return true
		}, 5*time.Second, 20*time.Millisecond).Should(BeTrue(),
			"depth-3 tree (root worker → mid supervisor → leaf supervisor) never fully spawned")

		// Drain the whole tree. Contract under test (pkg/fsmv2/CLAUDE.md
		// §"Graceful Shutdown Cascading"): each level samples base × subtree
		// HEIGHT at Shutdown entry — leaf base, mid 2×base, root 3×base — and
		// its child drains spend from that same budget, so the mid worker's
		// 1.1×base graceful stop completes warn-free and the whole chain
		// drains within 3×base total.
		start := time.Now()
		root.Shutdown()
		elapsed := time.Since(start)

		logOutput := buf.String()

		// Assertion 1: an in-progress graceful drain must not be truncated.
		// Under a flat budget the mid level WOULD arm 1×base while its worker
		// needs 1.1×base, so graceful_shutdown_timeout would fire spuriously.
		Expect(containsLogEvent(logOutput, "graceful_shutdown_timeout")).To(BeFalse(),
			"graceful_shutdown_timeout was logged: a level armed a flat %v budget instead of the cascaded "+
				"height-based budget, truncating a drain that still needed 0.1×base to emit SignalNeedsRemoval", cascadeDrainBase)

		// Assertion 2: every level fully drained — no worker leaked behind a
		// truncated drain.
		root.mu.RLock()
		rootRemaining := len(root.workers)
		root.mu.RUnlock()
		Expect(rootRemaining).To(BeZero(), "root supervisor leaked workers after Shutdown")
		Expect(midSup.ListWorkers()).To(BeEmpty(),
			"mid supervisor leaked its slow-stopping worker: its drain budget was exhausted before the worker's 1.1×base graceful stop finished")
		Expect(leafSup.ListWorkers()).To(BeEmpty(), "leaf supervisor leaked workers after Shutdown")

		// Assertion 3: the documented total holds — depth 3 = 3×base — plus
		// teardown slop for the three sequentially-torn-down supervisors
		// (root, mid, leaf), which the drain budget does not cover.
		Expect(elapsed).To(BeNumerically("<=", 3*cascadeDrainBase+3*perLevelTeardownSlop),
			"Shutdown took %v; the documented cascading contract bounds a depth-3 chain at 3×base = %v (+%v teardown slop for 3 supervisors)",
			elapsed, 3*cascadeDrainBase, 3*perLevelTeardownSlop)
	})

	It("warns per stuck level with that level's cascaded budget and stays within the documented total", func() {
		ctx := context.Background()

		basicStore := memory.NewInMemoryStore()
		for _, wt := range []string{cascadeDrainRootType, cascadeStuckMidType, cascadeDrainLeafType} {
			for _, suffix := range []string{"_identity", "_desired", "_observed"} {
				Expect(basicStore.CreateCollection(ctx, wt+suffix, nil)).To(Succeed())
			}
		}
		triangularStore := storage.NewTriangularStore(basicStore, deps.NewNopFSMLogger())

		logger := deps.NewJSONFSMLogger(buf, deps.LevelDebug)

		root := NewSupervisor[*TestObservedState, *TestDesiredState](Config{
			WorkerType:              cascadeDrainRootType,
			Store:                   triangularStore,
			Logger:                  logger,
			TickInterval:            cascadeTickInterval,
			GracefulShutdownTimeout: cascadeTruncationBase,
		})

		rootIdentity := deps.Identity{
			ID:            "truncation-root-worker",
			Name:          "Truncation Root Worker",
			WorkerType:    cascadeDrainRootType,
			HierarchyPath: "truncation-root-worker(" + cascadeDrainRootType + ")",
		}
		// The root worker is stuck too, so the root's own Phase-3 drain rides
		// its timer to expiry. That makes the elapsed bound below sensitive to
		// the Phase-2 deduction: the root must arm 3×base MINUS the ≈2×base mid
		// drain, not a fresh 3×base window on top of it.
		rootWorker := &cascadeTreeWorker{
			id:           rootIdentity.ID,
			initialState: ignoresShutdownState{},
			childrenSpecs: []config.ChildSpec{
				{Name: "mid", WorkerType: cascadeStuckMidType, UserSpec: config.UserSpec{}, Enabled: true},
			},
		}
		Expect(root.AddWorker(rootIdentity, rootWorker)).To(Succeed())

		desiredDoc := persistence.Document{
			FieldID:             rootIdentity.ID,
			"ShutdownRequested": false,
			"state":             "running",
		}
		_, err := triangularStore.SaveDesired(ctx, cascadeDrainRootType, rootIdentity.ID, desiredDoc)
		Expect(err).ToNot(HaveOccurred())

		runCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		_ = root.Start(runCtx)
		defer root.Shutdown() // Idempotent; reaps the tree if an assertion fails early.

		Eventually(func() bool {
			children := root.GetChildren()
			mid, ok := children["mid"]
			if !ok || len(mid.ListWorkers()) != 1 {
				return false
			}
			grandchildren := mid.GetChildren()
			leaf, ok := grandchildren["leaf"]

			return ok && len(leaf.ListWorkers()) == 1
		}, 5*time.Second, 20*time.Millisecond).Should(BeTrue(),
			"depth-3 tree (root worker → stuck mid supervisor → leaf supervisor) never fully spawned")

		start := time.Now()
		root.Shutdown()
		elapsed := time.Since(start)

		logOutput := buf.String()

		// Assertion 1: exactly the two stuck levels warn, each with its own
		// cascaded budget in the timeout field — the mid first (it exhausts
		// its 2×base during the root's child-drain phase, before the root's
		// own drain starts), then the root with 3×base. Any further entry —
		// a leaf warn (1×base) or a duplicate — is a spurious warn: a level
		// armed a flat 1×base budget instead of its cascaded one.
		warns := findLogEvents(logOutput, "graceful_shutdown_timeout")
		Expect(warns).To(HaveLen(2),
			"expected exactly two graceful_shutdown_timeout warns (stuck mid, stuck root), got %d: %v", len(warns), warns)
		Expect(warns[0]["timeout"]).To(Equal((2 * cascadeTruncationBase).String()),
			"expected the first warn's timeout field to carry the mid level's cascaded budget (2×base)")
		Expect(warns[1]["timeout"]).To(Equal((3 * cascadeTruncationBase).String()),
			"expected the second warn's timeout field to carry the root level's cascaded budget (3×base)")

		// Assertion 2: stuck levels must not stall the documented total. The
		// root's stuck worker rides the Phase-3 timer to expiry, so this bound
		// pins the Phase-2 deduction: without it the root would arm a fresh
		// 3×base window after the ≈2×base mid drain and elapsed would land
		// near 5×base. Slop covers the three sequentially-torn-down
		// supervisors (root, mid, leaf).
		Expect(elapsed).To(BeNumerically("<=", 3*cascadeTruncationBase+3*perLevelTeardownSlop),
			"Shutdown took %v; the documented cascading contract bounds a depth-3 chain at 3×base = %v (+%v teardown slop for 3 supervisors)",
			elapsed, 3*cascadeTruncationBase, 3*perLevelTeardownSlop)
	})

	// Sibling-sequential exhaustion (CLAUDE.md §Graceful Shutdown Cascading,
	// wide-tree paragraph): two stuck depth-2 subtrees spend ≈2×base each from
	// the root's 3×base budget, so the root's own workers reach Phase 3 with
	// the budget already overdrawn. The root must warn immediately and break
	// out — a regression that regrants a fresh window on a non-positive
	// remainder (e.g. `if remainder <= 0 { remainder = drainBudget }`) restores
	// per-level budget summing and is invisible to the chain tests above.
	It("warns and breaks out immediately when sibling child drains pre-spend the whole budget", func() {
		ctx := context.Background()

		basicStore := memory.NewInMemoryStore()
		for _, wt := range []string{cascadeDrainRootType, cascadeStuckMidType, cascadeDrainLeafType} {
			for _, suffix := range []string{"_identity", "_desired", "_observed"} {
				Expect(basicStore.CreateCollection(ctx, wt+suffix, nil)).To(Succeed())
			}
		}
		triangularStore := storage.NewTriangularStore(basicStore, deps.NewNopFSMLogger())

		logger := deps.NewJSONFSMLogger(buf, deps.LevelDebug)

		root := NewSupervisor[*TestObservedState, *TestDesiredState](Config{
			WorkerType:              cascadeDrainRootType,
			Store:                   triangularStore,
			Logger:                  logger,
			TickInterval:            cascadeTickInterval,
			GracefulShutdownTimeout: cascadeTruncationBase,
		})

		rootIdentity := deps.Identity{
			ID:            "prespent-root-worker",
			Name:          "Prespent Root Worker",
			WorkerType:    cascadeDrainRootType,
			HierarchyPath: "prespent-root-worker(" + cascadeDrainRootType + ")",
		}
		// The root worker itself honors shutdown promptly — the warn this test
		// demands can only come from the exhausted budget, never from the
		// root's own worker being slow.
		rootWorker := &cascadeTreeWorker{
			id:           rootIdentity.ID,
			initialState: shutdownHonoringState{},
			childrenSpecs: []config.ChildSpec{
				{Name: "mid1", WorkerType: cascadeStuckMidType, UserSpec: config.UserSpec{}, Enabled: true},
				{Name: "mid2", WorkerType: cascadeStuckMidType, UserSpec: config.UserSpec{}, Enabled: true},
			},
		}
		Expect(root.AddWorker(rootIdentity, rootWorker)).To(Succeed())

		desiredDoc := persistence.Document{
			FieldID:             rootIdentity.ID,
			"ShutdownRequested": false,
			"state":             "running",
		}
		_, err := triangularStore.SaveDesired(ctx, cascadeDrainRootType, rootIdentity.ID, desiredDoc)
		Expect(err).ToNot(HaveOccurred())

		runCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		_ = root.Start(runCtx)
		defer root.Shutdown() // Idempotent; reaps the tree if an assertion fails early.

		Eventually(func() bool {
			children := root.GetChildren()
			for _, name := range []string{"mid1", "mid2"} {
				mid, ok := children[name]
				if !ok || len(mid.ListWorkers()) != 1 {
					return false
				}

				leaf, ok := mid.GetChildren()["leaf"]
				if !ok || len(leaf.ListWorkers()) != 1 {
					return false
				}
			}

			return true
		}, 5*time.Second, 20*time.Millisecond).Should(BeTrue(),
			"wide tree (root worker → two stuck mid supervisors → leaf supervisors) never fully spawned")

		start := time.Now()
		root.Shutdown()
		elapsed := time.Since(start)

		logOutput := buf.String()

		// Each stuck mid exhausts its own 2×base budget with its own worker
		// stuck — those two warn graceful_shutdown_timeout. The root's budget
		// was pre-spent by the two mid drains before its own (prompt) worker
		// had a window, so the root warns the distinct
		// graceful_shutdown_budget_exhausted event: a Sentry consumer must be
		// able to tell sibling-drain exhaustion from a genuinely stuck worker
		// by fingerprint, not by reading fields. The mids drain in
		// map-iteration order, but they are identical, so both timeout
		// entries carry 2×base.
		warns := findLogEvents(logOutput, "graceful_shutdown_timeout")
		Expect(warns).To(HaveLen(2),
			"expected exactly two graceful_shutdown_timeout warns (the two stuck mids), got %d: %v", len(warns), warns)
		Expect(warns[0]["timeout"]).To(Equal((2 * cascadeTruncationBase).String()),
			"expected the first warn to carry a mid level's cascaded budget (2×base)")
		Expect(warns[1]["timeout"]).To(Equal((2 * cascadeTruncationBase).String()),
			"expected the second warn to carry a mid level's cascaded budget (2×base)")

		exhausted := findLogEvents(logOutput, "graceful_shutdown_budget_exhausted")
		Expect(exhausted).To(HaveLen(1),
			"expected exactly one graceful_shutdown_budget_exhausted warn (the pre-spent root), got %d: %v", len(exhausted), exhausted)

		rootWarn := exhausted[0]
		Expect(rootWarn["timeout"]).To(Equal((3 * cascadeTruncationBase).String()),
			"expected the root to warn with its 3×base budget even though its own worker honors shutdown: "+
				"the child drains spent ≈4×base, so Phase 3 arms a non-positive remainder that fires before the worker can drain")
		Expect(rootWarn["remaining_worker_count"]).To(BeNumerically("==", 1),
			"expected the root's worker to still be present when the exhausted budget fired")

		// The warn must say where the budget went: child_drain_elapsed carries
		// the Phase-2 spend (≥3×base here), so on-call can tell sibling-drain
		// exhaustion from a stuck worker that ate the window itself.
		spentField, ok := rootWarn["child_drain_elapsed"].(string)
		Expect(ok).To(BeTrue(), "expected the warn to carry child_drain_elapsed, got %v", rootWarn)
		spent, parseErr := time.ParseDuration(spentField)
		Expect(parseErr).ToNot(HaveOccurred())
		Expect(spent).To(BeNumerically(">=", 3*cascadeTruncationBase),
			"expected child_drain_elapsed to show the Phase-2 child drains spent at least the root's whole 3×base budget")

		// Total wall clock = sum of the sibling subtree budgets (2×base each)
		// plus an immediate root break-out — bounded, not stalled by a fresh
		// root window on top. Slop covers the five sequentially-torn-down
		// supervisors (root, two mids, two leafs).
		Expect(elapsed).To(BeNumerically("<=", 4*cascadeTruncationBase+5*perLevelTeardownSlop),
			"Shutdown took %v; two sequential 2×base sibling drains plus an immediate root break-out must stay within 4×base = %v (+%v teardown slop for 5 supervisors)",
			elapsed, 4*cascadeTruncationBase, 5*perLevelTeardownSlop)
	})

	// Zero-worker exhaustion (CLAUDE.md §Graceful Shutdown Cascading: "A level
	// that exhausts its budget always warns and breaks out"): zero workers on
	// a started supervisor is a normal transient (reconciliation.go, tick's
	// no-worker skip), and such a level still has children whose drains can
	// overrun its budget. The warn must not hide inside the worker-drain
	// branch.
	It("warns on a zero-worker level whose child drains exhaust its budget", func() {
		ctx := context.Background()

		basicStore := memory.NewInMemoryStore()
		for _, wt := range []string{cascadeDrainRootType, cascadeStuckLeafType} {
			for _, suffix := range []string{"_identity", "_desired", "_observed"} {
				Expect(basicStore.CreateCollection(ctx, wt+suffix, nil)).To(Succeed())
			}
		}
		triangularStore := storage.NewTriangularStore(basicStore, deps.NewNopFSMLogger())

		logger := deps.NewJSONFSMLogger(buf, deps.LevelDebug)

		root := NewSupervisor[*TestObservedState, *TestDesiredState](Config{
			WorkerType:              cascadeDrainRootType,
			Store:                   triangularStore,
			Logger:                  logger,
			TickInterval:            cascadeTickInterval,
			GracefulShutdownTimeout: cascadeTruncationBase,
		})

		runCtx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Three stuck single-worker children, linked directly (the spawn path
		// needs a parent worker, which this level deliberately lacks). Each
		// burns its full 1×base budget, so the root's 2×base budget (height 2)
		// is exhausted with a full 1×base of margin — the comparison is not
		// riding the boundary.
		for i := range 3 {
			name := fmt.Sprintf("stuck-%d", i)
			child := NewSupervisor[*TestObservedState, *TestDesiredState](Config{
				WorkerType:              cascadeStuckLeafType,
				Store:                   triangularStore,
				Logger:                  logger,
				TickInterval:            cascadeTickInterval,
				GracefulShutdownTimeout: cascadeTruncationBase,
			})
			childIdentity := deps.Identity{
				ID:            name + "-worker",
				Name:          "Stuck Child Worker " + name,
				WorkerType:    cascadeStuckLeafType,
				HierarchyPath: name + "-worker(" + cascadeStuckLeafType + ")",
			}
			Expect(child.AddWorker(childIdentity, &cascadeTreeWorker{
				id:           childIdentity.ID,
				initialState: ignoresShutdownState{},
			})).To(Succeed())

			childDoc := persistence.Document{
				FieldID:             childIdentity.ID,
				"ShutdownRequested": false,
				"state":             "running",
			}
			_, saveErr := triangularStore.SaveDesired(ctx, cascadeStuckLeafType, childIdentity.ID, childDoc)
			Expect(saveErr).ToNot(HaveOccurred())

			_ = child.Start(runCtx)

			root.mu.Lock()
			root.children[name] = child
			root.mu.Unlock()
		}

		_ = root.Start(runCtx)
		defer root.Shutdown() // Idempotent; reaps the children if an assertion fails early.

		start := time.Now()
		root.Shutdown()
		elapsed := time.Since(start)

		logOutput := buf.String()

		// Each stuck child exhausts its 1×base budget with its own worker
		// stuck — those three warn graceful_shutdown_timeout. The zero-worker
		// root's 2×base budget was spent entirely by its children, so it
		// warns the distinct graceful_shutdown_budget_exhausted event.
		warns := findLogEvents(logOutput, "graceful_shutdown_timeout")
		Expect(warns).To(HaveLen(3),
			"expected exactly three graceful_shutdown_timeout warns (the three stuck children), got %d: %v", len(warns), warns)

		exhausted := findLogEvents(logOutput, "graceful_shutdown_budget_exhausted")
		Expect(exhausted).To(HaveLen(1),
			"expected exactly one graceful_shutdown_budget_exhausted warn (the zero-worker root), got %d: %v", len(exhausted), exhausted)

		rootWarn := exhausted[0]
		Expect(rootWarn["timeout"]).To(Equal((2 * cascadeTruncationBase).String()),
			"expected the root's warn to carry its cascaded budget (2×base)")
		Expect(rootWarn["remaining_worker_count"]).To(BeNumerically("==", 0),
			"expected the zero-worker root to warn with remaining_worker_count=0 — a silent break-out leaves no Sentry signal for the overrun")

		// The warn must say where the budget went: the three sequential
		// 1×base child drains spent at least the root's whole 2×base budget.
		spentField, ok := rootWarn["child_drain_elapsed"].(string)
		Expect(ok).To(BeTrue(), "expected the warn to carry child_drain_elapsed, got %v", rootWarn)
		spent, parseErr := time.ParseDuration(spentField)
		Expect(parseErr).ToNot(HaveOccurred())
		Expect(spent).To(BeNumerically(">=", 2*cascadeTruncationBase),
			"expected child_drain_elapsed to show the Phase-2 child drains spent at least the root's whole 2×base budget")

		// Slop covers the four sequentially-torn-down supervisors (root plus
		// three children).
		Expect(elapsed).To(BeNumerically("<=", 3*cascadeTruncationBase+4*perLevelTeardownSlop),
			"Shutdown took %v; three sequential 1×base child drains plus an immediate root break-out must stay within 3×base = %v (+%v teardown slop for 4 supervisors)",
			elapsed, 3*cascadeTruncationBase, 4*perLevelTeardownSlop)
	})

	// Firing-direction counterpart to the exhaustion case above: a
	// zero-worker level whose child drains complete WITHIN budget must exit
	// warn-free. Zero workers on a started supervisor is a documented normal
	// transient (reconciliation.go, tick's no-worker skip), so a loosened
	// exhaustion condition (childDrainElapsed > 0, or no comparison at all)
	// would fire spurious SentryWarns fleet-wide on every shutdown.
	It("exits warn-free on a zero-worker level whose child drains complete within budget", func() {
		ctx := context.Background()

		basicStore := memory.NewInMemoryStore()
		for _, wt := range []string{cascadeDrainRootType, cascadeDrainLeafType} {
			for _, suffix := range []string{"_identity", "_desired", "_observed"} {
				Expect(basicStore.CreateCollection(ctx, wt+suffix, nil)).To(Succeed())
			}
		}
		triangularStore := storage.NewTriangularStore(basicStore, deps.NewNopFSMLogger())

		logger := deps.NewJSONFSMLogger(buf, deps.LevelDebug)

		root := NewSupervisor[*TestObservedState, *TestDesiredState](Config{
			WorkerType:              cascadeDrainRootType,
			Store:                   triangularStore,
			Logger:                  logger,
			TickInterval:            cascadeTickInterval,
			GracefulShutdownTimeout: cascadeTruncationBase,
		})

		runCtx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// One fast-draining child, linked directly (the spawn path needs a
		// parent worker, which this level deliberately lacks).
		child := NewSupervisor[*TestObservedState, *TestDesiredState](Config{
			WorkerType:              cascadeDrainLeafType,
			Store:                   triangularStore,
			Logger:                  logger,
			TickInterval:            cascadeTickInterval,
			GracefulShutdownTimeout: cascadeTruncationBase,
		})
		childIdentity := deps.Identity{
			ID:            "prompt-child-worker",
			Name:          "Prompt Child Worker",
			WorkerType:    cascadeDrainLeafType,
			HierarchyPath: "prompt-child-worker(" + cascadeDrainLeafType + ")",
		}
		Expect(child.AddWorker(childIdentity, &cascadeTreeWorker{
			id:           childIdentity.ID,
			initialState: shutdownHonoringState{},
		})).To(Succeed())

		childDoc := persistence.Document{
			FieldID:             childIdentity.ID,
			"ShutdownRequested": false,
			"state":             "running",
		}
		_, saveErr := triangularStore.SaveDesired(ctx, cascadeDrainLeafType, childIdentity.ID, childDoc)
		Expect(saveErr).ToNot(HaveOccurred())

		_ = child.Start(runCtx)

		root.mu.Lock()
		root.children["prompt"] = child
		root.mu.Unlock()

		_ = root.Start(runCtx)
		defer root.Shutdown() // Idempotent; reaps the child if an assertion fails early.

		Eventually(func() int {
			return len(child.ListWorkers())
		}, 5*time.Second, 20*time.Millisecond).Should(Equal(1),
			"the prompt child's worker never spawned")

		root.Shutdown()

		logOutput := buf.String()

		Expect(findLogEvents(logOutput, "graceful_shutdown_timeout")).To(BeEmpty(),
			"graceful_shutdown_timeout was logged: a zero-worker level whose child drained within budget must exit warn-free")
		Expect(findLogEvents(logOutput, "graceful_shutdown_budget_exhausted")).To(BeEmpty(),
			"graceful_shutdown_budget_exhausted was logged: the child's prompt drain left budget to spare, so the exhaustion warn fired on a non-exhausted budget")
	})
})

var _ = Describe("calculateSubtreeHeight", func() {
	newBareSupervisor := func() *Supervisor[*TestObservedState, *TestDesiredState] {
		return NewSupervisor[*TestObservedState, *TestDesiredState](Config{
			WorkerType: "heighttest",
			Store:      CreateTestTriangularStore(),
			Logger:     deps.NewNopFSMLogger(),
		})
	}

	link := func(parent, child *Supervisor[*TestObservedState, *TestDesiredState], name string) {
		parent.mu.Lock()
		defer parent.mu.Unlock()

		parent.children[name] = child
	}

	It("returns 1 for a supervisor with no children", func() {
		Expect(newBareSupervisor().calculateSubtreeHeight()).To(Equal(1))
	})

	It("takes the max over sibling subtrees, not the sum", func() {
		root := newBareSupervisor()
		tall := newBareSupervisor()
		tallLeaf := newBareSupervisor()
		short := newBareSupervisor()

		link(tall, tallLeaf, "leaf") // tall subtree: height 2
		link(root, tall, "tall")
		link(root, short, "short") // short subtree: height 1

		// 1 + max(2, 1) = 3. A max→sum regression would return 4 and arm an
		// oversized drain budget for every wide tree, invisibly to the
		// warn-free cascade test above.
		Expect(root.calculateSubtreeHeight()).To(Equal(3))
	})
})
