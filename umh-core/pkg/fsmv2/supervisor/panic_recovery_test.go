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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/testutil"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

var _ = Describe("Tick Loop Panic Recovery", func() {
	Describe("when tick() panics", func() {
		Context("with a panicking worker state machine", func() {
			It("should recover from panic and log the error", func() {
				observedLogs, logger := createObservedLogger()

				store := supervisor.CreateTestTriangularStore()

				s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
					WorkerType:              "test",
					Store:                   store,
					Logger:                  logger,
					TickInterval:            50 * time.Millisecond,
					GracefulShutdownTimeout: 100 * time.Millisecond,
				})

				identity := supervisor.TestIdentity()
				worker := &panickingWorker{
					panicOnTick:  true,
					panicMessage: "simulated tick panic for testing",
				}
				err := s.AddWorker(identity, worker)
				Expect(err).ToNot(HaveOccurred())
				defer s.Shutdown()

				ctx := context.Background()

				// This should NOT panic (supervisor should recover) and return an error
				err = s.TestTick(ctx)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("tick panic"))

				// Verify panic was logged with ErrorFields pattern
				panicLogs := filterLogs(observedLogs, "tick_panic")
				Expect(panicLogs).ToNot(BeEmpty(), "Expected tick_panic log entry")

				panicLog := panicLogs[0]
				Expect(panicLog.ContextMap()).To(HaveKey("error"))
				Expect(panicLog.ContextMap()).To(HaveKey("stack_trace"))
				Expect(panicLog.ContextMap()).To(HaveKey("feature"))
				Expect(panicLog.ContextMap()["feature"]).To(Equal("fsmv2"))
			})

			It("should continue operating after recovering from panic", func() {
				_, logger := createObservedLogger()

				store := supervisor.CreateTestTriangularStore()

				s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
					WorkerType:              "test",
					Store:                   store,
					Logger:                  logger,
					TickInterval:            50 * time.Millisecond,
					GracefulShutdownTimeout: 100 * time.Millisecond,
				})

				identity := supervisor.TestIdentity()
				worker := &panickingWorker{
					panicOnTick:   true,
					panicMessage:  "simulated tick panic",
					panicOnlyOnce: true,
				}
				err := s.AddWorker(identity, worker)
				Expect(err).ToNot(HaveOccurred())
				defer s.Shutdown()

				ctx := context.Background()

				// First tick panics
				err = s.TestTick(ctx)
				Expect(err).To(HaveOccurred())

				// Second tick should succeed (worker no longer panics)
				err = s.TestTick(ctx)
				Expect(err).ToNot(HaveOccurred(), "Second tick should succeed after panic recovery")
			})

			It("should log panic_type as 'string' for string panics", func() {
				observedLogs, logger := createObservedLogger()

				store := supervisor.CreateTestTriangularStore()

				s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
					WorkerType:              "test",
					Store:                   store,
					Logger:                  logger,
					TickInterval:            50 * time.Millisecond,
					GracefulShutdownTimeout: 100 * time.Millisecond,
				})

				identity := supervisor.TestIdentity()
				worker := &panickingWorker{
					panicOnTick:  true,
					panicMessage: "string panic message",
				}
				err := s.AddWorker(identity, worker)
				Expect(err).ToNot(HaveOccurred())
				defer s.Shutdown()

				err = s.TestTick(context.Background())
				Expect(err).To(HaveOccurred())

				panicLogs := filterLogs(observedLogs, "tick_panic")
				Expect(panicLogs).ToNot(BeEmpty())
				Expect(panicLogs[0].ContextMap()["panic_type"]).To(Equal("string"))
			})

			It("should log panic_type as 'error' and preserve error chain for error panics", func() {
				observedLogs, logger := createObservedLogger()

				store := supervisor.CreateTestTriangularStore()

				s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
					WorkerType:              "test",
					Store:                   store,
					Logger:                  logger,
					TickInterval:            50 * time.Millisecond,
					GracefulShutdownTimeout: 100 * time.Millisecond,
				})

				identity := supervisor.TestIdentity()
				testErr := errors.New("wrapped error for testing")
				worker := &panickingWorker{
					panicOnTick:         true,
					panicValue:          testErr,
					useCustomPanicValue: true,
				}
				err := s.AddWorker(identity, worker)
				Expect(err).ToNot(HaveOccurred())
				defer s.Shutdown()

				err = s.TestTick(context.Background())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("tick panic"))
				Expect(err.Error()).To(ContainSubstring("wrapped error for testing"))

				// Verify error chain is preserved
				Expect(errors.Is(err, testErr)).To(BeTrue(), "Error chain should be preserved")

				panicLogs := filterLogs(observedLogs, "tick_panic")
				Expect(panicLogs).ToNot(BeEmpty())
				Expect(panicLogs[0].ContextMap()["panic_type"]).To(Equal("error"))
			})

			It("should handle panic(nil) gracefully", func() {
				observedLogs, logger := createObservedLogger()

				store := supervisor.CreateTestTriangularStore()

				s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
					WorkerType:              "test",
					Store:                   store,
					Logger:                  logger,
					TickInterval:            50 * time.Millisecond,
					GracefulShutdownTimeout: 100 * time.Millisecond,
				})

				identity := supervisor.TestIdentity()
				worker := &panickingWorker{
					panicOnTick:         true,
					panicValue:          nil,
					useCustomPanicValue: true,
				}
				err := s.AddWorker(identity, worker)
				Expect(err).ToNot(HaveOccurred())
				defer s.Shutdown()

				err = s.TestTick(context.Background())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("tick panic"))

				panicLogs := filterLogs(observedLogs, "tick_panic")
				Expect(panicLogs).ToNot(BeEmpty())
				// In Go 1.21+, panic(nil) creates *runtime.PanicNilError which is an error type
				Expect(panicLogs[0].ContextMap()["panic_type"]).To(Equal("error"))
			})

			It("should log panic_type as 'unknown' for non-error non-string panics", func() {
				observedLogs, logger := createObservedLogger()

				store := supervisor.CreateTestTriangularStore()

				s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
					WorkerType:              "test",
					Store:                   store,
					Logger:                  logger,
					TickInterval:            50 * time.Millisecond,
					GracefulShutdownTimeout: 100 * time.Millisecond,
				})

				identity := supervisor.TestIdentity()
				worker := &panickingWorker{
					panicOnTick:         true,
					panicValue:          42,
					useCustomPanicValue: true,
				}
				err := s.AddWorker(identity, worker)
				Expect(err).ToNot(HaveOccurred())
				defer s.Shutdown()

				err = s.TestTick(context.Background())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("tick panic"))
				Expect(err.Error()).To(ContainSubstring("42"))

				panicLogs := filterLogs(observedLogs, "tick_panic")
				Expect(panicLogs).ToNot(BeEmpty())
				Expect(panicLogs[0].ContextMap()["panic_type"]).To(Equal("unknown"))
			})
		})
	})
})

var _ = Describe("Panic Recovery Unit Tests", func() {
	Describe("panicRecovery sliding window", func() {
		It("should not reach threshold after a single panic", func() {
			tracker := supervisor.NewTestPanicRecoveryTracker(5*time.Minute, 3)

			shouldEscalate := tracker.RecordPanic()
			Expect(shouldEscalate).To(BeFalse())
			Expect(tracker.PanicCount()).To(Equal(1))
		})

		It("should reach threshold after maxPanics within window", func() {
			tracker := supervisor.NewTestPanicRecoveryTracker(5*time.Minute, 3)

			Expect(tracker.RecordPanic()).To(BeFalse())
			Expect(tracker.RecordPanic()).To(BeFalse())
			shouldEscalate := tracker.RecordPanic()
			Expect(shouldEscalate).To(BeTrue())
			Expect(tracker.PanicCount()).To(Equal(3))
		})

		It("should prune panics outside the window", func() {
			tracker := supervisor.NewTestPanicRecoveryTracker(100*time.Millisecond, 3)

			tracker.RecordPanic()
			tracker.RecordPanic()
			Expect(tracker.PanicCount()).To(Equal(2))

			time.Sleep(150 * time.Millisecond)

			Expect(tracker.PanicCount()).To(Equal(0))

			Expect(tracker.RecordPanic()).To(BeFalse())
			Expect(tracker.PanicCount()).To(Equal(1))
		})

		It("should reset all recorded panics", func() {
			tracker := supervisor.NewTestPanicRecoveryTracker(5*time.Minute, 3)

			tracker.RecordPanic()
			tracker.RecordPanic()
			Expect(tracker.PanicCount()).To(Equal(2))

			tracker.Reset()
			Expect(tracker.PanicCount()).To(Equal(0))
		})
	})
})

var _ = Describe("Panic Escalation", func() {
	Describe("tick panic circuit breaker", func() {
		It("should not open circuit after a single panic", func() {
			_, logger := createObservedLogger()
			store := supervisor.CreateTestTriangularStore()

			s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
				WorkerType:              "test",
				Store:                   store,
				Logger:                  logger,
				TickInterval:            50 * time.Millisecond,
				GracefulShutdownTimeout: 100 * time.Millisecond,
			})

			identity := supervisor.TestIdentity()
			worker := &panickingWorker{
				panicOnTick:  true,
				panicMessage: "escalation test panic",
			}
			err := s.AddWorker(identity, worker)
			Expect(err).ToNot(HaveOccurred())
			defer s.Shutdown()

			ctx := context.Background()

			err = s.TestTick(ctx)
			Expect(err).To(HaveOccurred())

			Expect(s.TestIsPanicCircuitOpen()).To(BeFalse(), "Single panic should not open circuit")
		})

		It("should open circuit after 3 panics within window", func() {
			observedLogs, logger := createObservedLogger()
			store := supervisor.CreateTestTriangularStore()

			s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
				WorkerType:              "test",
				Store:                   store,
				Logger:                  logger,
				TickInterval:            50 * time.Millisecond,
				GracefulShutdownTimeout: 100 * time.Millisecond,
			})

			identity := supervisor.TestIdentity()
			worker := &panickingWorker{
				panicOnTick:  true,
				panicMessage: "escalation test panic",
			}
			err := s.AddWorker(identity, worker)
			Expect(err).ToNot(HaveOccurred())
			defer s.Shutdown()

			ctx := context.Background()

			for i := 0; i < 3; i++ {
				err = s.TestTick(ctx)
				Expect(err).To(HaveOccurred())
			}

			Expect(s.TestIsPanicCircuitOpen()).To(BeTrue(), "3 panics should open panic circuit")

			circuitLogs := filterLogs(observedLogs, "panic_circuit_open")
			Expect(circuitLogs).ToNot(BeEmpty(), "Expected panic_circuit_open log entry")
			circuitLog := circuitLogs[0]
			Expect(circuitLog.ContextMap()["feature"]).To(Equal("fsmv2"))
			Expect(circuitLog.ContextMap()["panic_count"]).To(BeEquivalentTo(3))
		})

		It("should skip tick processing when panic circuit is open", func() {
			_, logger := createObservedLogger()
			store := supervisor.CreateTestTriangularStore()

			s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
				WorkerType:              "test",
				Store:                   store,
				Logger:                  logger,
				TickInterval:            50 * time.Millisecond,
				GracefulShutdownTimeout: 100 * time.Millisecond,
			})

			identity := supervisor.TestIdentity()
			worker := &panickingWorker{
				panicOnTick:  true,
				panicMessage: "escalation test panic",
			}
			err := s.AddWorker(identity, worker)
			Expect(err).ToNot(HaveOccurred())
			defer s.Shutdown()

			ctx := context.Background()

			// Trigger 3 panics to open the circuit
			for i := 0; i < 3; i++ {
				err = s.TestTick(ctx)
				Expect(err).To(HaveOccurred())
			}

			Expect(s.TestIsPanicCircuitOpen()).To(BeTrue())

			// 4th tick should be skipped (no panic, no error)
			err = s.TestTick(ctx)
			Expect(err).ToNot(HaveOccurred(), "Tick should be skipped when panic circuit is open")
		})

		It("should report panic circuit via IsCircuitOpen()", func() {
			_, logger := createObservedLogger()
			store := supervisor.CreateTestTriangularStore()

			s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
				WorkerType:              "test",
				Store:                   store,
				Logger:                  logger,
				TickInterval:            50 * time.Millisecond,
				GracefulShutdownTimeout: 100 * time.Millisecond,
			})

			identity := supervisor.TestIdentity()
			worker := &panickingWorker{
				panicOnTick:  true,
				panicMessage: "escalation test panic",
			}
			err := s.AddWorker(identity, worker)
			Expect(err).ToNot(HaveOccurred())
			defer s.Shutdown()

			ctx := context.Background()

			Expect(s.IsCircuitOpen()).To(BeFalse())

			// Trigger 3 panics to open circuit
			for i := 0; i < 3; i++ {
				err = s.TestTick(ctx)
				Expect(err).To(HaveOccurred())
			}

			Expect(s.IsCircuitOpen()).To(BeTrue(), "IsCircuitOpen should reflect panic circuit state")
		})
	})
})

var _ = Describe("Tick Double Panic", func() {
	It("should recover when the recovery handler itself panics", func() {
		logger := &panicOnSentryErrorLogger{}

		store := supervisor.CreateTestTriangularStore()

		s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
			WorkerType:              "test",
			Store:                   store,
			Logger:                  logger,
			TickInterval:            50 * time.Millisecond,
			GracefulShutdownTimeout: 100 * time.Millisecond,
		})

		identity := supervisor.TestIdentity()
		worker := &panickingWorker{
			panicOnTick:  true,
			panicMessage: "trigger double panic",
		}
		err := s.AddWorker(identity, worker)
		Expect(err).ToNot(HaveOccurred())
		defer s.Shutdown()

		err = s.TestTick(context.Background())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("recovery handler also panicked"))
		Expect(s.TestIsPanicCircuitOpen()).To(BeTrue(), "Double panic should open panic circuit")
	})
})

var _ = Describe("IsCircuitOpen Infra Path", func() {
	It("should return true when infrastructure circuit is open", func() {
		_, logger := createObservedLogger()
		store := supervisor.CreateTestTriangularStore()

		s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
			WorkerType:              "test",
			Store:                   store,
			Logger:                  logger,
			TickInterval:            50 * time.Millisecond,
			GracefulShutdownTimeout: 100 * time.Millisecond,
		})

		Expect(s.IsCircuitOpen()).To(BeFalse())

		s.TestSetCircuitOpen(true)
		Expect(s.IsCircuitOpen()).To(BeTrue(), "IsCircuitOpen should reflect infra circuit state")
		Expect(s.TestIsPanicCircuitOpen()).To(BeFalse(), "Panic circuit should remain closed")

		s.TestSetCircuitOpen(false)
		Expect(s.IsCircuitOpen()).To(BeFalse())
	})
})

// panicOnSentryErrorLogger panics when SentryError is called, used to test double-panic recovery.
type panicOnSentryErrorLogger struct{}

func (p *panicOnSentryErrorLogger) Debug(msg string, fields ...deps.Field)                         {}
func (p *panicOnSentryErrorLogger) Info(msg string, fields ...deps.Field)                          {}
func (p *panicOnSentryErrorLogger) SentryWarn(_ deps.Feature, _ string, _ string, _ ...deps.Field) {}
func (p *panicOnSentryErrorLogger) SentryError(_ deps.Feature, _ string, _ error, _ string, _ ...deps.Field) {
	panic("logger SentryError panicked")
}
func (p *panicOnSentryErrorLogger) With(fields ...deps.Field) deps.FSMLogger { return p }

// panickingWorker is a test worker that panics during state machine execution.
// This simulates a bug in a worker's state machine that causes a panic.
type panickingWorker struct {
	panicOnTick         bool
	panicMessage        string
	panicValue          any
	useCustomPanicValue bool
	panicOnlyOnce       bool
	panicTriggered      bool
}

func (w *panickingWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	return &testutil.ObservedState{
		ID:          "panic-worker",
		CollectedAt: time.Now(),
		Desired:     &testutil.DesiredState{},
	}, nil
}

func (w *panickingWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	return &config.DesiredState{BaseDesiredState: config.BaseDesiredState{State: "running"}}, nil
}

func (w *panickingWorker) GetInitialState() fsmv2.State[any, any] {
	return &panickingState{worker: w}
}

// panickingState is a state that panics when Next() is called.
type panickingState struct {
	worker *panickingWorker
}

func (s *panickingState) Next(snapshot any) fsmv2.NextResult[any, any] {
	if s.worker.panicOnTick {
		if s.worker.panicOnlyOnce && s.worker.panicTriggered {
			return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil, "staying in state")
		}

		s.worker.panicTriggered = true

		if s.worker.useCustomPanicValue {
			panic(s.worker.panicValue)
		}

		panic(s.worker.panicMessage)
	}

	return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil, "staying in state")
}

func (s *panickingState) String() string { return "PanickingState" }

func (s *panickingState) LifecyclePhase() config.LifecyclePhase { return config.PhaseRunningHealthy }

func createObservedLogger() (*observer.ObservedLogs, deps.FSMLogger) {
	core, logs := observer.New(zapcore.DebugLevel)
	logger := deps.NewFSMLogger(zap.New(core).Sugar())

	return logs, logger
}

func filterLogs(logs *observer.ObservedLogs, message string) []observer.LoggedEntry {
	var filtered []observer.LoggedEntry

	for _, entry := range logs.All() {
		if strings.Contains(entry.Message, message) {
			filtered = append(filtered, entry)
		}
	}

	return filtered
}
