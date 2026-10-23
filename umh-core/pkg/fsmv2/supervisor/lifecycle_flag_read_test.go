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
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// flagRunningState always runs and ignores shutdown requests, so the stored
// ShutdownRequested flag survives in the store while the worker keeps
// ticking: a spec can observe whether a tick's save wipes it.
type flagRunningState struct {
	nextCalls *atomic.Int64
}

func (flagRunningState) String() string { return "running" }

func (flagRunningState) LifecyclePhase() config.LifecyclePhase {
	return config.PhaseRunningHealthy
}

func (s flagRunningState) Next(any) fsmv2.NextResult[any, any] {
	s.nextCalls.Add(1)

	return fsmv2.NextResult[any, any]{Signal: fsmv2.SignalNone, State: s, Reason: "running"}
}

// flagReadWorker derives a fixed running desired state; its state machine
// counts Next calls and ignores shutdown.
type flagReadWorker struct {
	deriveCalls atomic.Int64
	nextCalls   atomic.Int64
}

func (w *flagReadWorker) CollectObservedState(context.Context, fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	return &supervisor.TestObservedState{ID: "test-worker", CollectedAt: time.Now()}, nil
}

func (w *flagReadWorker) DeriveDesiredState(any) (fsmv2.DesiredState, error) {
	w.deriveCalls.Add(1)

	return &config.DesiredState{State: "running"}, nil
}

func (w *flagReadWorker) GetInitialState() fsmv2.State[any, any] {
	return flagRunningState{nextCalls: &w.nextCalls}
}

// failingLoadStore delegates every call to the wrapped store, but
// LoadDesiredTyped fails once with a plain error when armed.
type failingLoadStore struct {
	storage.TriangularStoreInterface

	failLoadOnce atomic.Bool
}

func (s *failingLoadStore) LoadDesiredTyped(ctx context.Context, workerType string, id string, dest any) error {
	if s.failLoadOnce.CompareAndSwap(true, false) {
		return errors.New("transient store failure")
	}

	return s.TriangularStoreInterface.LoadDesiredTyped(ctx, workerType, id, dest)
}

var _ = Describe("Supervisor lifecycle flag reads", func() {
	It("does not wipe a stored shutdown flag when reading the stored flags fails", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		recorder := newRecordingSentryLogger()
		store := &failingLoadStore{TriangularStoreInterface: createTestTriangularStore()}
		s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
			WorkerType:              "test",
			Store:                   store,
			Logger:                  recorder,
			TickInterval:            20 * time.Millisecond,
			GracefulShutdownTimeout: 500 * time.Millisecond,
		})

		worker := &flagReadWorker{}
		Expect(s.AddWorker(mockIdentity(), worker)).NotTo(HaveOccurred())

		s.Start(ctx)
		defer s.Shutdown()

		// A cached desired state exists after the first ticks.
		Eventually(func() int64 {
			return worker.deriveCalls.Load()
		}, 5*time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 2))

		// Store a shutdown request; the worker ignores it, so the flag sits
		// in the stored document while the supervisor keeps ticking.
		Expect(s.RequestShutdown(ctx, "flag preservation test")).To(Succeed())

		Eventually(func() bool {
			doc, err := store.LoadDesired(ctx, "test", "test-worker")
			if err != nil {
				return false
			}

			return doc.(persistence.Document)["ShutdownRequested"] == true
		}, 5*time.Second, 10*time.Millisecond).Should(BeTrue(),
			"the shutdown request must be stored before the read failure")

		// The next tick's flag read fails: the save must skip, not store a
		// document without the flag.
		store.failLoadOnce.Store(true)

		// Let several ticks pass over the failed read.
		baseline := worker.nextCalls.Load()

		Eventually(func() int64 {
			return worker.nextCalls.Load() - baseline
		}, 5*time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 3))

		doc, err := store.LoadDesired(ctx, "test", "test-worker")
		Expect(err).NotTo(HaveOccurred())
		Expect(doc.(persistence.Document)["ShutdownRequested"]).To(BeTrue(),
			"a failed flag read must not save over the stored shutdown request")
	})
})
