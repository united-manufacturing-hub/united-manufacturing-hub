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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/internal/collection"
)

// logEntry records the level and message of a single log call.
type logEntry struct {
	level string
	msg   string
}

// severityCapturingLogger is a deps.FSMLogger test double that records the
// (level, message) of every log call. Unlike deps.NewNopFSMLogger() (which
// discards the level), it lets a spec assert the severity a specific event
// was logged at. With() returns the same recorder so child-logger output is
// captured too.
type severityCapturingLogger struct {
	mu      sync.Mutex
	entries []logEntry
}

func (l *severityCapturingLogger) record(level, msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, logEntry{level: level, msg: msg})
}

func (l *severityCapturingLogger) Debug(msg string, _ ...deps.Field) { l.record("debug", msg) }

func (l *severityCapturingLogger) Info(msg string, _ ...deps.Field) { l.record("info", msg) }

func (l *severityCapturingLogger) SentryWarn(_ deps.Feature, _ string, msg string, _ ...deps.Field) {
	l.record("sentrywarn", msg)
}

func (l *severityCapturingLogger) SentryError(_ deps.Feature, _ string, _ error, msg string, _ ...deps.Field) {
	l.record("sentryerror", msg)
}

func (l *severityCapturingLogger) With(_ ...deps.Field) deps.FSMLogger { return l }

// levelFor returns the level the given message was logged at, or "" if it was
// never logged.
func (l *severityCapturingLogger) levelFor(msg string) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range l.entries {
		if e.msg == msg {
			return e.level
		}
	}
	return ""
}

// has reports whether the given (level, message) pair was ever recorded.
func (l *severityCapturingLogger) has(level, msg string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range l.entries {
		if e.level == level && e.msg == msg {
			return true
		}
	}
	return false
}

var _ = Describe("Collector log severity", func() {
	Context("Stop() on a not-running collector", func() {
		// J1: a not-running Stop is benign on every teardown caller — the
		// supervisor shutdown path (context.Background, Err()==nil), a cancelled
		// teardown ctx, and the worker-reap / RemoveWorker paths (a live
		// reconcile ctx, Err()==nil). collector_stop_skipped must be Debug for
		// all of them. A regression re-introducing a ctx.Err() discriminator
		// that routes any non-cancelled or live ctx back to SentryWarn must fail
		// this table.
		DescribeTable("logs collector_stop_skipped at Debug regardless of the passed context",
			func(makeCtx func() (context.Context, context.CancelFunc)) {
				logger := &severityCapturingLogger{}
				collector := collection.NewCollector[supervisor.TestObservedState](collection.CollectorConfig[supervisor.TestObservedState]{
					Worker:              &supervisor.TestWorker{},
					Identity:            supervisor.TestIdentity(),
					Store:               supervisor.CreateTestTriangularStore(),
					Logger:              logger,
					ObservationInterval: 1 * time.Second,
					ObservationTimeout:  3 * time.Second,
				})

				ctx, cancel := makeCtx()
				defer cancel()
				collector.Stop(ctx)

				Expect(logger.levelFor("collector_stop_skipped")).To(Equal("debug"),
					"collector_stop_skipped must be Debug on every benign teardown caller, not SentryWarn")
				Expect(logger.has("sentrywarn", "collector_stop_skipped")).To(BeFalse(),
					"collector_stop_skipped must never be SentryWarn for a benign not-running Stop")
			},
			Entry("background context (shutdown path, Err()==nil)", func() (context.Context, context.CancelFunc) {
				return context.Background(), func() {}
			}),
			Entry("cancelled context (Err()==context.Canceled)", func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx, func() {}
			}),
			Entry("deadline-exceeded context (Err()==context.DeadlineExceeded)", func() (context.Context, context.CancelFunc) {
				return context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
			}),
			Entry("live uncancelled context (reap/RemoveWorker path, Err()==nil)", func() (context.Context, context.CancelFunc) {
				return context.WithCancel(context.Background())
			}),
		)
	})

	Context("CollectFinalObservation() that times out", func() {
		// CollectFinalObservation runs from the reap on every worker
		// removal (reconciliation.go:1131) with a context whose deadline is
		// already expired wrapped in a local WithTimeout. A genuinely stuck
		// collector produces context.DeadlineExceeded,
		// which is a real fault and must stay visible in Sentry — only
		// context.Canceled (the shutdown race) is downgraded to Debug. A regression
		// that downgrades DeadlineExceeded to Debug must fail this spec.
		It("logs collector_final_observation_failed at SentryWarn on DeadlineExceeded", func() {
			logger := &severityCapturingLogger{}
			collector := collection.NewCollector[supervisor.TestObservedState](collection.CollectorConfig[supervisor.TestObservedState]{
				Worker:              &supervisor.TestWorker{},
				Identity:            supervisor.TestIdentity(),
				Store:               supervisor.CreateTestTriangularStore(),
				Logger:              logger,
				ObservationInterval: 1 * time.Second,
				ObservationTimeout:  3 * time.Second,
				DesiredStateProvider: func() (fsmv2.DesiredState, error) {
					return &supervisor.TestDesiredState{}, nil
				},
			})

			Expect(collector.Start(context.Background())).To(Succeed())
			defer collector.Stop(context.Background())

			// A context whose deadline is already in the past: the local
			// context.WithTimeout(ctx, ObservationTimeout) inside
			// CollectFinalObservation inherits the expired deadline, so the
			// collection fails with context.DeadlineExceeded — a genuinely stuck
			// collector, not a cancellation.
			expiredCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
			defer cancel()

			err := collector.CollectFinalObservation(expiredCtx)
			Expect(err).To(MatchError(context.DeadlineExceeded))

			Expect(logger.levelFor("collector_final_observation_failed")).To(Equal("sentrywarn"),
				"a DeadlineExceeded final observation is a genuinely stuck collector and must stay visible in Sentry, not Debug")
			Expect(logger.has("debug", "collector_final_observation_failed")).To(BeFalse(),
				"only context.Canceled (the shutdown race) is downgraded to Debug; DeadlineExceeded must not be")
		})

		// The Canceled->Debug arm is the shutdown-race downgrade this change
		// exists to preserve. Without this spec a regression dropping the
		// errors.Is(err, context.Canceled) check would route clean-shutdown
		// cancellations back to SentryWarn — the exact Sentry noise the split
		// suppresses — and the suite would stay green.
		It("logs collector_final_observation_failed at Debug on context.Canceled", func() {
			logger := &severityCapturingLogger{}
			collector := collection.NewCollector[supervisor.TestObservedState](collection.CollectorConfig[supervisor.TestObservedState]{
				Worker:              &supervisor.TestWorker{},
				Identity:            supervisor.TestIdentity(),
				Store:               supervisor.CreateTestTriangularStore(),
				Logger:              logger,
				ObservationInterval: 1 * time.Second,
				ObservationTimeout:  3 * time.Second,
				DesiredStateProvider: func() (fsmv2.DesiredState, error) {
					return &supervisor.TestDesiredState{}, nil
				},
			})

			Expect(collector.Start(context.Background())).To(Succeed())
			defer collector.Stop(context.Background())

			// A context cancelled before the call: the local WithTimeout inside
			// CollectFinalObservation inherits the cancellation, so the collection
			// fails with context.Canceled — the benign shutdown race that is
			// deliberately downgraded to Debug.
			cancelledCtx, cancel := context.WithCancel(context.Background())
			cancel()

			err := collector.CollectFinalObservation(cancelledCtx)
			Expect(err).To(MatchError(context.Canceled))

			Expect(logger.levelFor("collector_final_observation_failed")).To(Equal("debug"),
				"context.Canceled is the benign shutdown race and must be downgraded to Debug")
			Expect(logger.has("sentrywarn", "collector_final_observation_failed")).To(BeFalse(),
				"context.Canceled must not raise a SentryWarn (that would re-noise Sentry on clean shutdown)")
		})
	})
})
