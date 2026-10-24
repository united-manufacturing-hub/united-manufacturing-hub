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
	"errors"
	"strings"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/persistence/memory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/supervisor"
)

// countingShutdownState counts its Next calls and emits SignalNeedsRemoval
// once the snapshot's desired state carries a shutdown request, so a spec
// can prove both that the supervisor keeps ticking a worker while its spec
// is broken and that the worker reaches removal after RequestShutdown.
type countingShutdownState struct {
	nextCalls *atomic.Int64
}

func (countingShutdownState) String() string { return "running" }

func (countingShutdownState) LifecyclePhase() config.LifecyclePhase {
	return config.PhaseRunningHealthy
}

func (s countingShutdownState) Next(snapshot any) fsmv2.NextResult[any, any] {
	s.nextCalls.Add(1)

	if snap, ok := snapshot.(fsmv2.Snapshot); ok {
		if snap.Desired != nil && snap.Desired.IsShutdownRequested() {
			return fsmv2.NextResult[any, any]{Signal: fsmv2.SignalNeedsRemoval, State: s, Reason: "shutdown requested"}
		}
	}

	return fsmv2.NextResult[any, any]{Signal: fsmv2.SignalNone, State: s, Reason: "running"}
}

// configTextShutdownWorker derives a running desired state, with the
// children set on it, unless the user spec's config text contains "{{",
// which fails the derive with a config mistake.
type configTextShutdownWorker struct {
	deriveCalls   atomic.Int64
	nextCalls     atomic.Int64
	childrenSpecs []config.ChildSpec
}

func (w *configTextShutdownWorker) CollectObservedState(context.Context, fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	return &supervisor.TestObservedState{ID: "test-worker", CollectedAt: time.Now()}, nil
}

func (w *configTextShutdownWorker) DeriveDesiredState(spec any) (fsmv2.DesiredState, error) {
	w.deriveCalls.Add(1)

	cfg := ""
	if us, ok := spec.(config.UserSpec); ok {
		cfg = us.Config
	}

	if strings.Contains(cfg, "{{") {
		return nil, &config.ConfigError{Field: "config", Err: errors.New(cfg)}
	}

	return &config.DesiredState{State: "running", ChildrenSpecs: w.childrenSpecs}, nil
}

func (w *configTextShutdownWorker) GetInitialState() fsmv2.State[any, any] {
	return countingShutdownState{nextCalls: &w.nextCalls}
}

// createStoreForWorkerTypes builds a store with the collections every
// worker type in a hierarchy needs, like the suite's createTestTriangularStore
// does for fixed types.
func createStoreForWorkerTypes(workerTypes ...string) *storage.TriangularStore {
	ctx := context.Background()
	basicStore := memory.NewInMemoryStore()

	for _, workerType := range workerTypes {
		for _, suffix := range []string{"_identity", "_desired", "_observed"} {
			Expect(basicStore.CreateCollection(ctx, workerType+suffix, nil)).To(Succeed())
		}
	}

	return storage.NewTriangularStore(basicStore, deps.NewNopFSMLogger())
}

var _ = Describe("Supervisor shutdown with a config mistake", func() {
	const (
		parentWorkerType = "test"
		childWorkerType  = "shutdownchild"
		childName        = "shutdownchild"
	)

	var childNextCalls *atomic.Int64

	newSupervisor := func(recorder *recordingSentryLogger, store *storage.TriangularStore) *supervisor.Supervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState] {
		return supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
			WorkerType:              parentWorkerType,
			Store:                   store,
			Logger:                  recorder,
			TickInterval:            20 * time.Millisecond,
			GracefulShutdownTimeout: 500 * time.Millisecond,
		})
	}

	BeforeEach(func() {
		childNextCalls = &atomic.Int64{}

		Expect(factory.RegisterFactoryByType(childWorkerType,
			func(_ deps.Identity, _ deps.FSMLogger, _ deps.StateReader, _ map[string]any) fsmv2.Worker {
				return &shutdownChildWorker{nextCalls: childNextCalls}
			})).To(Succeed())

		Expect(factory.RegisterSupervisorFactoryByType(childWorkerType, func(cfg any) any {
			return supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](cfg.(supervisor.Config))
		})).To(Succeed())
	})

	It("removes a running worker whose derive fails with a config mistake after RequestShutdown", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// The worker must keep being ticked on the desired state derived from
		// its last good config while its spec is broken, so it still reads the
		// shutdown request and reaches removal.
		recorder := newRecordingSentryLogger()
		store := createTestTriangularStore()
		s := newSupervisor(recorder, store)

		worker := &configTextShutdownWorker{}
		Expect(s.AddWorker(mockIdentity(), worker)).NotTo(HaveOccurred())
		Expect(s.ListWorkers()).To(HaveLen(1))

		s.Start(ctx)
		defer s.Shutdown()

		// Two derive calls mean the worker derived once in AddWorker and once
		// in a tick, so the supervisor's cached desired state
		// (cachedDesiredState) holds a desired state derived from a good
		// config.
		Eventually(func() int64 {
			return worker.deriveCalls.Load()
		}, 5*time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 2))

		// The stored desired state is the one from the good config. A tick on
		// the cached state must not save over it.
		goodDoc, err := store.LoadDesired(ctx, "test", "test-worker") //nolint:staticcheck // the raw document is the comparison
		Expect(err).NotTo(HaveOccurred())

		// Break the spec and wait until a tick derived it and logged the
		// config mistake, so the checks below run while the error lasts.
		s.TestUpdateUserSpec(config.UserSpec{Config: "{{ broken"})
		Eventually(func() int {
			return recorder.infoCount("config_error")
		}, 5*time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

		// The worker must stay listed while the spec is broken: a regression
		// that removes it on the config mistake would pass the removal check
		// below vacuously, because RequestShutdown on zero workers returns nil.
		Consistently(func() int {
			return len(s.ListWorkers())
		}, 300*time.Millisecond, 20*time.Millisecond).Should(Equal(1))

		// The stored desired state stays the one from before the error: the
		// broken config never reaches the store.
		storedDoc, err := store.LoadDesired(ctx, "test", "test-worker") //nolint:staticcheck // the raw document is the comparison
		Expect(err).NotTo(HaveOccurred())
		Expect(storedDoc).To(Equal(goodDoc),
			"a tick on the cached desired state must not save a new one")

		Expect(s.RequestShutdown(ctx, "test shutdown")).To(Succeed())

		// Removal is a state the worker stays in, so waiting on it cannot pass
		// on a window: the worker is only gone once the tick that read the
		// shutdown request removed it.
		Eventually(func() []string {
			return s.ListWorkers()
		}, 5*time.Second, 10*time.Millisecond).Should(BeEmpty(),
			"a worker with a config mistake must still be ticked on the desired state derived from its last good config, so it reaches removal after RequestShutdown")

		// One Info line and no Sentry event across the broken window: the
		// config mistake is a user mistake, logged once, never escalated.
		Expect(recorder.infoCount("config_error")).To(Equal(1))
		Expect(recorder.sentryErrorEvents()).To(BeEmpty())
		Expect(recorder.sentryWarnEvents()).To(BeEmpty())
	})

	It("keeps calling the worker's state machine while the config mistake lasts", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		recorder := newRecordingSentryLogger()
		s := newSupervisor(recorder, createTestTriangularStore())

		worker := &configTextShutdownWorker{}
		Expect(s.AddWorker(mockIdentity(), worker)).NotTo(HaveOccurred())

		s.Start(ctx)
		defer s.Shutdown()

		Eventually(func() int64 {
			return worker.deriveCalls.Load()
		}, 5*time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 2))

		s.TestUpdateUserSpec(config.UserSpec{Config: "{{ broken"})
		Eventually(func() int {
			return recorder.infoCount("config_error")
		}, 5*time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

		// The spec-hash cache is not updated on the config error, so every
		// tick derives again: an implementation that cached the broken spec
		// would stop calling the worker's derive while it stays broken.
		deriveBaseline := worker.deriveCalls.Load()

		Eventually(func() int64 {
			return worker.deriveCalls.Load() - deriveBaseline
		}, 5*time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 3))

		// The worker's Next keeps being called while the error lasts. An
		// implementation that ticks the worker only once a shutdown request is
		// stored keeps the removal spec above green, but cannot pass here.
		baseline := worker.nextCalls.Load()

		Eventually(func() int64 {
			return worker.nextCalls.Load() - baseline
		}, 5*time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 3))
	})

	Describe("a parent with a child", func() {
		It("keeps ticking the child while the parent's own spec is broken", func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			recorder := newRecordingSentryLogger()
			s := newSupervisor(recorder, createStoreForWorkerTypes(parentWorkerType, childWorkerType))

			parent := &configTextShutdownWorker{
				childrenSpecs: []config.ChildSpec{{
					Name:       childName,
					WorkerType: childWorkerType,
					UserSpec:   config.UserSpec{},
					Enabled:    true,
				}},
			}
			Expect(s.AddWorker(mockIdentity(), parent)).NotTo(HaveOccurred())

			s.Start(ctx)
			defer s.Shutdown()

			// The child must exist and be ticked before the parent's spec
			// breaks, so the growth check below measures the broken window.
			Eventually(func() bool {
				child, ok := s.GetChildren()[childName]
				if !ok {
					return false
				}

				return len(child.ListWorkers()) == 1 && childNextCalls.Load() > 0
			}, 5*time.Second, 20*time.Millisecond).Should(BeTrue())

			// Break the parent's own spec: its derive fails with a config
			// mistake, its cached desired state still names the child.
			s.TestUpdateUserSpec(config.UserSpec{Config: "{{ broken"})
			Eventually(func() int {
				return recorder.infoCount("config_error")
			}, 5*time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

			// The child's Next keeps being called while the parent's spec is
			// broken: the parent's tick keeps reconciling and ticking it.
			baseline := childNextCalls.Load()

			Eventually(func() int64 {
				return childNextCalls.Load() - baseline
			}, 5*time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 3))
		})

		It("drops the child when a broken parent shuts down, and no child reappears", func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			recorder := newRecordingSentryLogger()
			s := newSupervisor(recorder, createStoreForWorkerTypes(parentWorkerType, childWorkerType))

			parent := &configTextShutdownWorker{
				childrenSpecs: []config.ChildSpec{{
					Name:       childName,
					WorkerType: childWorkerType,
					UserSpec:   config.UserSpec{},
					Enabled:    true,
				}},
			}
			Expect(s.AddWorker(mockIdentity(), parent)).NotTo(HaveOccurred())

			s.Start(ctx)
			defer s.Shutdown()

			Eventually(func() bool {
				child, ok := s.GetChildren()[childName]
				if !ok {
					return false
				}

				return len(child.ListWorkers()) == 1 && childNextCalls.Load() > 0
			}, 5*time.Second, 20*time.Millisecond).Should(BeTrue())

			s.TestUpdateUserSpec(config.UserSpec{Config: "{{ broken"})
			Eventually(func() int {
				return recorder.infoCount("config_error")
			}, 5*time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

			Expect(s.RequestShutdown(ctx, "test shutdown")).To(Succeed())

			// Both the worker and its child go: the child must be dropped on
			// the shutdown of a broken parent, not respawned from the cached
			// children specs. Removing a parent drains its child first, so the
			// wait allows 15s, more than the 2 × 5s budget that the "Graceful
			// shutdown budget" section of supervisor/doc.go gives two levels.
			Eventually(func() bool {
				children := s.GetChildren()

				return len(s.ListWorkers()) == 0 && len(children) == 0
			}, 15*time.Second, 20*time.Millisecond).Should(BeTrue(),
				"a parent with a config error must drop its children on shutdown")

			// No child supervisor reappears: the tick after the removal does
			// not recreate one from the cached children specs.
			Consistently(func() int {
				return len(s.GetChildren())
			}, 300*time.Millisecond, 20*time.Millisecond).Should(Equal(0))
		})
	})

	Describe("a disabled worker", func() {
		It("stays disabled across the broken window and its stored flag is unchanged", func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			recorder := newRecordingSentryLogger()
			store := createStoreForWorkerTypes(parentWorkerType)
			s := newSupervisor(recorder, store)

			worker := &configTextShutdownWorker{}
			Expect(s.AddWorker(mockIdentity(), worker)).NotTo(HaveOccurred())

			s.Start(ctx)
			defer s.Shutdown()

			Eventually(func() int64 {
				return worker.deriveCalls.Load()
			}, 5*time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 2))

			// Disable the worker through the same path the disable-mapping
			// pass uses: the stored desired state carries Disabled=true and
			// the state machine holds it resident in Stopped.
			Expect(s.SetDisabled(ctx, true)).To(Succeed())

			// Break the spec while the worker is disabled.
			s.TestUpdateUserSpec(config.UserSpec{Config: "{{ broken"})
			Eventually(func() int {
				return recorder.infoCount("config_error")
			}, 5*time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

			// The stored Disabled bit survives the broken window: a tick that
			// continues on the cached state must not read or write lifecycle
			// flags through a document it did not save.
			baseline := worker.nextCalls.Load()

			Eventually(func() int64 {
				return worker.nextCalls.Load() - baseline
			}, 5*time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 3))

			var stored config.DesiredState

			err := store.LoadDesiredTyped(ctx, parentWorkerType, "test-worker", &stored)
			Expect(err).NotTo(HaveOccurred())
			Expect(stored.IsDisabled()).To(BeTrue(),
				"a disabled worker's flag must survive a broken-spec window")
		})
	})
})

// shutdownChildWorker is the child of the parent-with-child spec: its state
// counts Next calls, so a spec can prove the parent keeps ticking the child
// while the parent's own spec is broken.
type shutdownChildWorker struct {
	nextCalls *atomic.Int64
}

func (w *shutdownChildWorker) CollectObservedState(context.Context, fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	return &supervisor.TestObservedState{ID: "shutdownchild-worker", CollectedAt: time.Now()}, nil
}

func (w *shutdownChildWorker) DeriveDesiredState(any) (fsmv2.DesiredState, error) {
	return &config.DesiredState{State: "running"}, nil
}

func (w *shutdownChildWorker) GetInitialState() fsmv2.State[any, any] {
	return countingShutdownState{nextCalls: w.nextCalls}
}
