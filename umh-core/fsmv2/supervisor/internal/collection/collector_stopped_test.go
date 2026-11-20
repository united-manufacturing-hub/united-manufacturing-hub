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

package collection_test

import (
	"context"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/supervisor/internal/collection"
)

func newStoppedTestCollector(worker *supervisor.TestWorker, observationInterval time.Duration) *collection.Collector[supervisor.TestObservedState] {
	return collection.NewCollector[supervisor.TestObservedState](collection.CollectorConfig[supervisor.TestObservedState]{
		Worker:              worker,
		Identity:            supervisor.TestIdentity(),
		Store:               supervisor.CreateTestTriangularStore(),
		Logger:              deps.NewNopFSMLogger(),
		ObservationInterval: observationInterval,
		ObservationTimeout:  5 * time.Second,
		DesiredStateProvider: func() (fsmv2.DesiredState, error) {
			return &supervisor.TestDesiredState{}, nil
		},
	})
}

var _ = Describe("Collector.Stopped", func() {
	It("closes only after the collection goroutine exits, also when Stop stopped waiting for it", func() {
		collectionStarted := make(chan struct{})
		releaseCollection := make(chan struct{})

		var startedOnce sync.Once

		worker := &supervisor.TestWorker{
			CollectFunc: func(_ context.Context) (fsmv2.ObservedState, error) {
				startedOnce.Do(func() { close(collectionStarted) })
				<-releaseCollection

				return supervisor.CreateTestObservedStateWithID("test-worker"), nil
			},
		}
		collector := newStoppedTestCollector(worker, 10*time.Millisecond)

		Expect(collector.Start(context.Background())).To(Succeed())
		Eventually(collectionStarted).Should(BeClosed())

		expiredStopCtx, cancelStop := context.WithCancel(context.Background())
		cancelStop()
		collector.Stop(expiredStopCtx)

		Consistently(collector.Stopped(), 100*time.Millisecond).ShouldNot(BeClosed())

		close(releaseCollection)

		Eventually(collector.Stopped()).Should(BeClosed())
	})

	It("stays open while a loop that outlived Restart still collects", func() {
		collectionStarted := make(chan struct{})
		releaseCollection := make(chan struct{})

		var startedOnce sync.Once

		worker := &supervisor.TestWorker{
			CollectFunc: func(_ context.Context) (fsmv2.ObservedState, error) {
				startedOnce.Do(func() { close(collectionStarted) })
				<-releaseCollection

				return supervisor.CreateTestObservedStateWithID("test-worker"), nil
			},
		}
		collector := newStoppedTestCollector(worker, time.Hour)

		Expect(collector.Start(context.Background())).To(Succeed())
		collector.TriggerNow()
		Eventually(collectionStarted).Should(BeClosed())

		collector.Restart()

		expiredStopCtx, cancelStop := context.WithCancel(context.Background())
		cancelStop()
		collector.Stop(expiredStopCtx)

		Consistently(collector.Stopped(), 100*time.Millisecond).ShouldNot(BeClosed())

		close(releaseCollection)

		Eventually(collector.Stopped()).Should(BeClosed())
	})

	It("is closed for a collector that never started", func() {
		collector := newStoppedTestCollector(&supervisor.TestWorker{}, 10*time.Millisecond)

		Expect(collector.Stopped()).To(BeClosed())
	})
})
