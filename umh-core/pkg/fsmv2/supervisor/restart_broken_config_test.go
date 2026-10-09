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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

// restartSignalState signals NeedsRestart on its first Next call, then
// NeedsRemoval once the desired state carries a shutdown request, so a spec
// can drive the full restart flow: signal, graceful shutdown, re-add.
type restartSignalState struct {
	restarts *atomic.Int64
}

func (restartSignalState) String() string { return "running" }

func (restartSignalState) LifecyclePhase() config.LifecyclePhase {
	return config.PhaseRunningHealthy
}

func (s restartSignalState) Next(snapshot any) fsmv2.NextResult[any, any] {
	if snap, ok := snapshot.(fsmv2.Snapshot); ok {
		if snap.Desired != nil && snap.Desired.IsShutdownRequested() {
			return fsmv2.NextResult[any, any]{Signal: fsmv2.SignalNeedsRemoval, State: s, Reason: "shutdown requested"}
		}
	}

	if s.restarts != nil && s.restarts.Load() == 0 {
		return fsmv2.NextResult[any, any]{Signal: fsmv2.SignalNeedsRestart, State: s, Reason: "unrecoverable"}
	}

	return fsmv2.NextResult[any, any]{Signal: fsmv2.SignalNone, State: s, Reason: "running"}
}

// restartBrokenConfigWorker derives a running desired state unless the user
// spec's config text contains "{{", which fails the derive with a config
// error. Its state signals NeedsRestart once, so the supervisor walks the
// full restart flow while the config is broken.
type restartBrokenConfigWorker struct {
	deriveCalls atomic.Int64
	restarts    atomic.Int64
}

func (w *restartBrokenConfigWorker) CollectObservedState(context.Context, fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	return &supervisor.TestObservedState{ID: "test-worker", CollectedAt: time.Now()}, nil
}

func (w *restartBrokenConfigWorker) DeriveDesiredState(spec any) (fsmv2.DesiredState, error) {
	w.deriveCalls.Add(1)

	cfg := ""
	if us, ok := spec.(config.UserSpec); ok {
		cfg = us.Config
	}

	if strings.Contains(cfg, "{{") {
		return nil, &config.ConfigError{Field: "config", Err: errors.New(cfg)}
	}

	return &config.DesiredState{State: "running"}, nil
}

func (w *restartBrokenConfigWorker) GetInitialState() fsmv2.State[any, any] {
	return restartSignalState{restarts: &w.restarts}
}

var _ = Describe("Supervisor restart with a broken config", func() {
	const brokenWorkerType = "restartbroken"

	BeforeEach(func() {
		Expect(factory.RegisterFactoryByType(brokenWorkerType,
			func(_ deps.Identity, _ deps.FSMLogger, _ deps.StateReader, _ map[string]any) fsmv2.Worker {
				return &restartBrokenConfigWorker{}
			})).To(Succeed())

		Expect(factory.RegisterSupervisorFactoryByType(brokenWorkerType, func(cfg any) any {
			return supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](cfg.(supervisor.Config))
		})).To(Succeed())
	})

	It("re-adds a worker whose restart aborted on a broken config once the config renders", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		recorder := newRecordingSentryLogger()
		s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
			WorkerType:              brokenWorkerType,
			Store:                   createStoreForWorkerTypes(brokenWorkerType),
			Logger:                  recorder,
			TickInterval:            20 * time.Millisecond,
			GracefulShutdownTimeout: 500 * time.Millisecond,
		})

		identity := deps.Identity{
			ID:         "test-worker",
			Name:       "Test Worker",
			WorkerType: brokenWorkerType,
		}

		worker := &restartBrokenConfigWorker{}
		Expect(s.AddWorker(identity, worker)).NotTo(HaveOccurred())

		// The real tick loop and collectors drive the multi-tick restart flow
		// (signal, graceful shutdown, removal, retry).
		s.Start(ctx)
		defer s.Shutdown()

		// One successful tick before the break, so the worker has a cached
		// desired state.
		Eventually(func() int64 {
			return worker.deriveCalls.Load()
		}, 5*time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 2),
			"AddWorker derives once and the tick loop derives again; one means the loop is not running")

		// Break the config and let the worker signal NeedsRestart. Ticks keep
		// running: the worker's state signals the restart, the graceful
		// shutdown completes, and the re-derive fails with the config error,
		// so the restart aborts after the removal.
		s.TestUpdateUserSpec(config.UserSpec{Config: "{{ broken"})

		// The state signals NeedsRestart on its first call after the break,
		// so the next tick starts the restart flow. The old worker runs its
		// graceful shutdown and the re-derive fails with the config error,
		// so the restart aborts after the removal.
		Eventually(func() int {
			return len(s.ListWorkers())
		}, 5*time.Second, 10*time.Millisecond).Should(Equal(0),
			"the restart flow removes the old worker while the config is broken")

		// While the config is broken, no Sentry event is logged for the
		// restart's config error.
		Expect(recorder.sentryErrorEvents()).To(BeEmpty(),
			"the restart's config error must not reach Sentry")
		Expect(recorder.sentryWarnEvents()).To(BeEmpty())

		// The config mistake warned exactly once.
		Expect(recorder.warnCount("config_error")).To(Equal(1))

		// Fixing the config brings the worker back: the supervisor retries
		// the pending re-add on a later tick and ticks the worker again.
		s.TestUpdateUserSpec(config.UserSpec{Config: "state: running"})

		Eventually(func() int {
			return len(s.ListWorkers())
		}, 5*time.Second, 10*time.Millisecond).Should(Equal(1),
			"a worker whose restart aborted on a broken config returns once the config renders")

		// The re-added worker is ticked.
		Eventually(func() bool {
			workers := s.ListWorkers()

			return len(workers) == 1 && workers[0] == "test-worker"
		}, 5*time.Second, 10*time.Millisecond).Should(BeTrue())
	})
})

// restartOnceState signals NeedsRestart on its very first Next call and
// honors a shutdown request afterwards, so one worker instance walks the
// restart flow exactly once no matter how often it is re-added.
type restartOnceState struct {
	signaled *atomic.Bool
}

func (restartOnceState) String() string { return "running" }

func (restartOnceState) LifecyclePhase() config.LifecyclePhase {
	return config.PhaseRunningHealthy
}

func (s restartOnceState) Next(snapshot any) fsmv2.NextResult[any, any] {
	if !s.signaled.Swap(true) {
		return fsmv2.NextResult[any, any]{Signal: fsmv2.SignalNeedsRestart, State: s, Reason: "unrecoverable"}
	}

	if snap, ok := snapshot.(fsmv2.Snapshot); ok {
		if snap.Desired != nil && snap.Desired.IsShutdownRequested() {
			return fsmv2.NextResult[any, any]{Signal: fsmv2.SignalNeedsRemoval, State: s, Reason: "shutdown requested"}
		}
	}

	return fsmv2.NextResult[any, any]{Signal: fsmv2.SignalNone, State: s, Reason: "running"}
}

// retryFlakyCOSWorker derives a running desired state; its CollectObservedState
// fails while its switch is set, so AddWorker's initial collection fails with a
// plain error that is not a config error.
type retryFlakyCOSWorker struct {
	deriveCalls atomic.Int64
	signaled    atomic.Bool
	cosFails    atomic.Bool
}

func (w *retryFlakyCOSWorker) CollectObservedState(context.Context, fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	if w.cosFails.Load() {
		return nil, errors.New("collection temporarily broken")
	}

	return &supervisor.TestObservedState{ID: "test-worker", CollectedAt: time.Now()}, nil
}

func (w *retryFlakyCOSWorker) DeriveDesiredState(any) (fsmv2.DesiredState, error) {
	w.deriveCalls.Add(1)

	return &config.DesiredState{State: "running"}, nil
}

func (w *retryFlakyCOSWorker) GetInitialState() fsmv2.State[any, any] {
	return restartOnceState{signaled: &w.signaled}
}

var _ = Describe("Supervisor pending re-add retries", func() {
	const flakyWorkerType = "flakycos"

	It("keeps retrying a re-add whose failure is not a config error, and recovers once the cause clears", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		flaky := &retryFlakyCOSWorker{}

		Expect(factory.RegisterFactoryByType(flakyWorkerType,
			func(_ deps.Identity, _ deps.FSMLogger, _ deps.StateReader, _ map[string]any) fsmv2.Worker {
				return flaky
			})).To(Succeed())

		Expect(factory.RegisterSupervisorFactoryByType(flakyWorkerType, func(cfg any) any {
			return supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](cfg.(supervisor.Config))
		})).To(Succeed())

		recorder := newRecordingSentryLogger()
		s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
			WorkerType:              flakyWorkerType,
			Store:                   createStoreForWorkerTypes(flakyWorkerType),
			Logger:                  recorder,
			TickInterval:            20 * time.Millisecond,
			GracefulShutdownTimeout: 500 * time.Millisecond,
		})

		identity := deps.Identity{
			ID:         "test-worker",
			Name:       "Test Worker",
			WorkerType: flakyWorkerType,
		}

		Expect(s.AddWorker(identity, flaky)).NotTo(HaveOccurred())

		s.Start(ctx)
		defer s.Shutdown()

		// The worker signals NeedsRestart on its first tick, so the restart
		// flow removes it and re-adds it.
		Eventually(func() int64 {
			return flaky.deriveCalls.Load()
		}, 5*time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

		// The replacement's collection fails, so the re-add fails with a
		// plain error; the restart leaves the re-add pending.
		flaky.cosFails.Store(true)

		// Several retries fail while the collection is broken.
		Eventually(func() int {
			return sentryErrorCount(recorder, "worker_restart_retry_add_failed")
		}, 5*time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 2),
			"a non-config re-add failure must keep being retried, not attempted once")

		// Once the collection works again the worker comes back.
		flaky.cosFails.Store(false)

		Eventually(func() int {
			return len(s.ListWorkers())
		}, 5*time.Second, 10*time.Millisecond).Should(Equal(1),
			"a pending re-add whose failure was not a config error must survive until it succeeds")
	})
})
