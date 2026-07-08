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

package supervisor_test

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// These specs lock in the fix for the ENG-4971 TOCTOU race: the per-tick
// derive's load-modify-save of the lifecycle flags (ShutdownRequested,
// Disabled) must not clobber a value written by requestShutdown / setDisabled
// on another goroutine.
//
// The backing store full-replaces on write (matching the real triangular /
// in-memory store), so the preserve block carries the flags forward by
// re-loading them. The clobber happens when an explicit setter commits the new
// value in the window between the derive's load and the derive's save: the
// derive then writes its stale value over the setter's. Because requestShutdown
// fires only once during shutdown, a clobbered flag is never re-asserted and
// the worker never observes shutdown — the captured graceful_shutdown_timeout.
//
// The interleave is forced deterministically via afterLoadDesired: it fires
// the concurrent setter (on its own goroutine) right after the derive's
// preserve-block load reads the stale value, then yields long enough for the
// setter to commit before the derive saves. The derive never waits on the
// setter, so the fix (which serializes the two load-modify-save sequences) does
// not deadlock — the setter blocks on the derive's critical section instead.
var _ = Describe("Derive vs lifecycle-setter race (ENG-4971)", func() {
	const interleaveWindow = 50 * time.Millisecond

	var (
		store      *mockStore
		workerType string
		workerID   string
		ctx        context.Context
	)

	BeforeEach(func() {
		workerType = "test"
		workerID = "test-worker"
		ctx = context.Background()
		store = newMockStore()
	})

	// newSupervisorWithWorker wires a supervisor whose single worker re-derives
	// the lifecycle flags as false every tick (mockWorker returns a plain
	// running desired state). loadSnapshot mirrors whatever the store holds.
	newSupervisorWithWorker := func() *supervisor.Supervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState] {
		store.loadSnapshot = func(_ context.Context, wt string, id string) (*storage.Snapshot, error) {
			desired := persistence.Document{}
			store.mu.RLock()
			if store.desired[wt] != nil && store.desired[wt][id] != nil {
				desired = store.desired[wt][id]
			}
			store.mu.RUnlock()

			return &storage.Snapshot{
				Identity: persistence.Document{"id": id, "name": "Test Worker", "workerType": wt},
				Desired:  desired,
				Observed: persistence.Document{"id": id, "collectedAt": time.Now()},
			}, nil
		}

		worker := &mockWorker{
			observed: &mockObservedState{ID: workerID, CollectedAt: time.Now()},
		}

		s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
			WorkerType: workerType,
			Store:      store,
			Logger:     deps.NewNopFSMLogger(),
			CollectorHealth: supervisor.CollectorHealthConfig{
				StaleThreshold: 10 * time.Second,
			},
		})

		identity := deps.Identity{ID: workerID, Name: "Test Worker", WorkerType: workerType}
		Expect(s.AddWorker(identity, worker)).To(Succeed())

		return s
	}

	// loadFlag returns the persisted bool for the named field, treating an
	// absent field as false (the consumer reads the zero value either way).
	loadFlag := func(field string) bool {
		store.mu.RLock()
		defer store.mu.RUnlock()

		doc := store.desired[workerType][workerID]
		if doc == nil {
			return false
		}

		v, _ := doc[field].(bool)

		return v
	}

	// runRaceTick ticks the supervisor while a concurrent setter commits its
	// flag in the window after the derive's preserve-block load. setter is the
	// explicit lifecycle write under test (requestShutdown / setDisabled).
	//
	// The hook arms once: the first load (the derive's preserve-block load)
	// spawns the setter and yields long enough for it to commit. The setter's
	// own load re-enters the hook, but the armed flag is already cleared so it
	// returns immediately (no re-entrancy stall). The derive never waits on the
	// setter, so a serializing fix blocks the setter on the derive's critical
	// section rather than deadlocking.
	runRaceTick := func(s *supervisor.Supervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState], setter func()) {
		var (
			armed atomic.Bool
			wg    sync.WaitGroup
		)

		armed.Store(true)

		store.afterLoadDesired = func(_ context.Context, _ string, _ string) {
			if !armed.CompareAndSwap(true, false) {
				return
			}

			wg.Add(1)

			go func() {
				defer wg.Done()
				defer GinkgoRecover()

				setter()
			}()

			time.Sleep(interleaveWindow)
		}

		Expect(s.TestTick(ctx)).To(Succeed())

		wg.Wait()
	}

	It("does not let the per-tick derive clobber a concurrent ShutdownRequested write", func() {
		s := newSupervisorWithWorker()

		runRaceTick(s, func() {
			Expect(s.TestRequestShutdown(ctx, workerID, "test-shutdown")).To(Succeed())
		})

		Expect(loadFlag("ShutdownRequested")).To(BeTrue(),
			"requestShutdown set ShutdownRequested=true; the per-tick derive must not overwrite it back to false")
	})

	It("does not let the per-tick derive clobber a concurrent Disabled write", func() {
		s := newSupervisorWithWorker()

		runRaceTick(s, func() {
			Expect(s.SetDisabled(ctx, true)).To(Succeed())
		})

		Expect(loadFlag("Disabled")).To(BeTrue(),
			"setDisabled set Disabled=true; the per-tick derive must not overwrite it back to false")
	})

	It("re-derivation preserves a ShutdownRequested already in the store", func() {
		s := newSupervisorWithWorker()

		Expect(s.TestRequestShutdown(ctx, workerID, "test-shutdown")).To(Succeed())
		Expect(loadFlag("ShutdownRequested")).To(BeTrue())

		Expect(s.TestTick(ctx)).To(Succeed())

		Expect(loadFlag("ShutdownRequested")).To(BeTrue(),
			"re-derivation must carry the persisted ShutdownRequested forward")
	})

	It("re-derivation preserves a Disabled already in the store", func() {
		s := newSupervisorWithWorker()

		Expect(s.SetDisabled(ctx, true)).To(Succeed())
		Expect(loadFlag("Disabled")).To(BeTrue())

		Expect(s.TestTick(ctx)).To(Succeed())

		Expect(loadFlag("Disabled")).To(BeTrue(),
			"re-derivation must carry the persisted Disabled forward")
	})
})
