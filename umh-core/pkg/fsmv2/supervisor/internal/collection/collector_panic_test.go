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
	"errors"
	"strings"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/internal/collection"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

var _ = Describe("Collector Panic Recovery", func() {
	Context("when CollectObservedState panics", func() {
		It("should recover and continue the observation loop", func() {
			var callCount atomic.Int32

			worker := &supervisor.TestWorker{
				CollectFunc: func(ctx context.Context) (fsmv2.ObservedState, error) {
					count := callCount.Add(1)
					if count == 1 {
						panic("simulated collector panic")
					}

					return supervisor.CreateTestObservedStateWithID("test-worker"), nil
				},
			}

			observedCore, observedLogs := observer.New(zapcore.DebugLevel)
			logger := deps.NewFSMLogger(zap.New(observedCore).Sugar())

			collector := collection.NewCollector[supervisor.TestObservedState](collection.CollectorConfig[supervisor.TestObservedState]{
				Worker:              worker,
				Identity:            supervisor.TestIdentity(),
				Store:               supervisor.CreateTestTriangularStore(),
				Logger:              logger,
				DesiredStateProvider: func() fsmv2.DesiredState {
					return &supervisor.TestDesiredState{State: "running"}
				},
				ObservationInterval: 50 * time.Millisecond,
				ObservationTimeout:  1 * time.Second,
			})

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			err := collector.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Wait for at least 2 collection cycles (first panics, second succeeds)
			time.Sleep(200 * time.Millisecond)

			// Collector should still be running after recovering from panic
			Expect(collector.IsRunning()).To(BeTrue(),
				"Collector should still be running after panic recovery")

			// CollectFunc should have been called more than once (panic didn't kill the loop)
			Expect(callCount.Load()).To(BeNumerically(">", 1),
				"CollectFunc should be called multiple times after panic recovery")

			// Verify panic was logged
			panicLogs := filterCollectorLogs(observedLogs, "collector_panic")
			Expect(panicLogs).ToNot(BeEmpty(), "Expected collector_panic log entry")

			cancel()
			time.Sleep(100 * time.Millisecond)
			Expect(collector.IsRunning()).To(BeFalse())
		})

		It("should log stack trace when recovering from panic", func() {
			var panicked atomic.Bool

			worker := &supervisor.TestWorker{
				CollectFunc: func(ctx context.Context) (fsmv2.ObservedState, error) {
					if !panicked.Load() {
						panicked.Store(true)
						panic("panic with stack trace test")
					}

					return supervisor.CreateTestObservedStateWithID("test-worker"), nil
				},
			}

			observedCore, observedLogs := observer.New(zapcore.DebugLevel)
			logger := deps.NewFSMLogger(zap.New(observedCore).Sugar())

			collector := collection.NewCollector[supervisor.TestObservedState](collection.CollectorConfig[supervisor.TestObservedState]{
				Worker:              worker,
				Identity:            supervisor.TestIdentity(),
				Store:               supervisor.CreateTestTriangularStore(),
				Logger:              logger,
				DesiredStateProvider: func() fsmv2.DesiredState {
					return &supervisor.TestDesiredState{State: "running"}
				},
				ObservationInterval: 50 * time.Millisecond,
				ObservationTimeout:  1 * time.Second,
			})

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			err := collector.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			time.Sleep(200 * time.Millisecond)

			panicLogs := filterCollectorLogs(observedLogs, "collector_panic")
			Expect(panicLogs).ToNot(BeEmpty())

			panicLog := panicLogs[0]
			Expect(panicLog.ContextMap()).To(HaveKey("stack_trace"))
			Expect(panicLog.ContextMap()).To(HaveKey("panic_value"))

			cancel()
			time.Sleep(100 * time.Millisecond)
		})
	})
})

var _ = Describe("Collector Panic Type Classification", func() {
	It("should classify error-type panics with panic_type=error", func() {
		var panicked atomic.Bool

		worker := &supervisor.TestWorker{
			CollectFunc: func(ctx context.Context) (fsmv2.ObservedState, error) {
				if !panicked.Load() {
					panicked.Store(true)
					panic(errors.New("typed error panic"))
				}

				return supervisor.CreateTestObservedStateWithID("test-worker"), nil
			},
		}

		observedCore, observedLogs := observer.New(zapcore.DebugLevel)
		logger := deps.NewFSMLogger(zap.New(observedCore).Sugar())

		collector := collection.NewCollector[supervisor.TestObservedState](collection.CollectorConfig[supervisor.TestObservedState]{
			Worker:              worker,
			Identity:            supervisor.TestIdentity(),
			Store:               supervisor.CreateTestTriangularStore(),
			Logger:              logger,
			DesiredStateProvider: func() fsmv2.DesiredState {
				return &supervisor.TestDesiredState{State: "running"}
			},
			ObservationInterval: 50 * time.Millisecond,
			ObservationTimeout:  1 * time.Second,
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		err := collector.Start(ctx)
		Expect(err).ToNot(HaveOccurred())

		time.Sleep(200 * time.Millisecond)

		panicLogs := filterCollectorLogs(observedLogs, "collector_panic")
		Expect(panicLogs).ToNot(BeEmpty())

		panicLog := panicLogs[0]
		Expect(panicLog.ContextMap()["panic_type"]).To(Equal("error_panic"))

		cancel()
		time.Sleep(100 * time.Millisecond)
	})

	It("should classify non-string non-error panics with panic_type=unknown", func() {
		var panicked atomic.Bool

		worker := &supervisor.TestWorker{
			CollectFunc: func(ctx context.Context) (fsmv2.ObservedState, error) {
				if !panicked.Load() {
					panicked.Store(true)
					panic(42)
				}

				return supervisor.CreateTestObservedStateWithID("test-worker"), nil
			},
		}

		observedCore, observedLogs := observer.New(zapcore.DebugLevel)
		logger := deps.NewFSMLogger(zap.New(observedCore).Sugar())

		collector := collection.NewCollector[supervisor.TestObservedState](collection.CollectorConfig[supervisor.TestObservedState]{
			Worker:              worker,
			Identity:            supervisor.TestIdentity(),
			Store:               supervisor.CreateTestTriangularStore(),
			Logger:              logger,
			DesiredStateProvider: func() fsmv2.DesiredState {
				return &supervisor.TestDesiredState{State: "running"}
			},
			ObservationInterval: 50 * time.Millisecond,
			ObservationTimeout:  1 * time.Second,
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		err := collector.Start(ctx)
		Expect(err).ToNot(HaveOccurred())

		time.Sleep(200 * time.Millisecond)

		panicLogs := filterCollectorLogs(observedLogs, "collector_panic")
		Expect(panicLogs).ToNot(BeEmpty())

		panicLog := panicLogs[0]
		Expect(panicLog.ContextMap()["panic_type"]).To(Equal("unknown_panic"))

		cancel()
		time.Sleep(100 * time.Millisecond)
	})
})

var _ = Describe("Collector Double Panic", func() {
	It("should recover when the recovery handler itself panics", func() {
		var panicked atomic.Bool

		worker := &supervisor.TestWorker{
			CollectFunc: func(ctx context.Context) (fsmv2.ObservedState, error) {
				if !panicked.Load() {
					panicked.Store(true)
					panic("trigger collector double panic")
				}

				return supervisor.CreateTestObservedStateWithID("test-worker"), nil
			},
		}

		logger := &panicOnSentryErrorCollectorLogger{}

		collector := collection.NewCollector[supervisor.TestObservedState](collection.CollectorConfig[supervisor.TestObservedState]{
			Worker:              worker,
			Identity:            supervisor.TestIdentity(),
			Store:               supervisor.CreateTestTriangularStore(),
			Logger:              logger,
			DesiredStateProvider: func() fsmv2.DesiredState {
				return &supervisor.TestDesiredState{State: "running"}
			},
			ObservationInterval: 50 * time.Millisecond,
			ObservationTimeout:  1 * time.Second,
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		err := collector.Start(ctx)
		Expect(err).ToNot(HaveOccurred())

		time.Sleep(200 * time.Millisecond)

		Expect(collector.IsRunning()).To(BeTrue(),
			"Collector should still be running after double panic recovery")

		cancel()
		time.Sleep(100 * time.Millisecond)
		Expect(collector.IsRunning()).To(BeFalse())
	})
})

// panicOnSentryErrorCollectorLogger panics on the first SentryError call only.
// After the first call, subsequent SentryError calls are no-ops.
// This tests the double-panic path inside collectAndSaveObservedState without
// crashing observationLoop's error handler (which also calls SentryError).
type panicOnSentryErrorCollectorLogger struct {
	panicked atomic.Bool
}

func (p *panicOnSentryErrorCollectorLogger) Debug(msg string, fields ...deps.Field)     {}
func (p *panicOnSentryErrorCollectorLogger) Info(msg string, fields ...deps.Field)       {}
func (p *panicOnSentryErrorCollectorLogger) SentryWarn(_ deps.Feature, _ string, _ string, _ ...deps.Field) {
}
func (p *panicOnSentryErrorCollectorLogger) SentryError(_ deps.Feature, _ string, _ error, _ string, _ ...deps.Field) {
	if !p.panicked.Load() {
		p.panicked.Store(true)
		panic("logger SentryError panicked in collector")
	}
}
func (p *panicOnSentryErrorCollectorLogger) With(fields ...deps.Field) deps.FSMLogger { return p }

func filterCollectorLogs(logs *observer.ObservedLogs, message string) []observer.LoggedEntry {
	var filtered []observer.LoggedEntry

	for _, entry := range logs.All() {
		if strings.Contains(entry.Message, message) {
			filtered = append(filtered, entry)
		}
	}

	return filtered
}
