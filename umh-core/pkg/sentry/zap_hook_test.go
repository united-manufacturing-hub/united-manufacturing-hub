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

package sentry

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getsentry/sentry-go"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var _ = Describe("SentryHook", func() {
	var (
		hook       *SentryHook
		eventStore *eventStore
		core       zapcore.Core
		logger     *zap.Logger
	)

	BeforeEach(func() {
		// Create a new isolated event store for each test
		eventStore = newEventStore()

		// Create a mock transport that captures events
		transport := &mockTransport{
			store: eventStore,
		}

		// Initialize Sentry with mock transport
		err := sentry.Init(sentry.ClientOptions{
			Dsn:       "https://test@sentry.io/123",
			Transport: transport,
		})
		Expect(err).NotTo(HaveOccurred())

		// Create underlying core (writes to /dev/null for tests)
		encoderConfig := zap.NewProductionEncoderConfig()
		core = zapcore.NewCore(
			zapcore.NewJSONEncoder(encoderConfig),
			zapcore.AddSync(&discardWriter{}),
			zapcore.DebugLevel,
		)

		// Create the SentryHook wrapping the core
		hook = NewSentryHook(core)

		// Create logger with the hook
		logger = zap.New(hook)
	})

	AfterEach(func() {
		// Flush any pending events and wait for goroutines to complete
		sentry.Flush(time.Second)
		// Give time for any in-flight goroutines to complete
		time.Sleep(100 * time.Millisecond)
	})

	Describe("Level filtering", func() {
		It("captures Error level logs to Sentry", func() {
			logger.Error("test error message")

			// Wait for async goroutine to complete
			Eventually(func() int {
				return eventStore.Len()
			}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

			events := eventStore.GetAll()
			Expect(events).To(HaveLen(1))
			Expect(events[0].Message).To(Equal("test error message"))
			Expect(events[0].Level).To(Equal(sentry.LevelError))
		})

		It("captures Warn level logs to Sentry", func() {
			logger.Warn("test warning message")

			Eventually(func() int {
				return eventStore.Len()
			}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

			events := eventStore.GetAll()
			Expect(events).To(HaveLen(1))
			Expect(events[0].Message).To(Equal("test warning message"))
			Expect(events[0].Level).To(Equal(sentry.LevelWarning))
		})

		It("does NOT capture Info level logs to Sentry", func() {
			logger.Info("test info message")

			// Wait a bit to ensure no event is captured
			time.Sleep(100 * time.Millisecond)

			Expect(eventStore.Len()).To(Equal(0))
		})

		It("does NOT capture Debug level logs to Sentry", func() {
			logger.Debug("test debug message")

			time.Sleep(100 * time.Millisecond)

			Expect(eventStore.Len()).To(Equal(0))
		})
	})

	Describe("Field extraction", func() {
		It("extracts fields as Sentry tags", func() {
			logger.Error("test error",
				zap.String("service_id", "benthos-123"),
				zap.String("operation", "reconcile"),
			)

			Eventually(func() int {
				return eventStore.Len()
			}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

			events := eventStore.GetAll()
			Expect(events).To(HaveLen(1))
			Expect(events[0].Tags).To(HaveKeyWithValue("service_id", "benthos-123"))
			Expect(events[0].Tags).To(HaveKeyWithValue("operation", "reconcile"))
		})

		It("extracts fingerprint keys for proper grouping", func() {
			logger.Error("test error",
				zap.String("operation", "create_failure"),
				zap.String("fsm_type", "benthosfsm"),
				zap.String("service_type", "benthos"),
				zap.String("trigger", "IsBenthosS6Stopped_empty_state"),
			)

			Eventually(func() int {
				return eventStore.Len()
			}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

			events := eventStore.GetAll()
			Expect(events).To(HaveLen(1))
			Expect(events[0].Fingerprint).To(ContainElement("operation: create_failure"))
			Expect(events[0].Fingerprint).To(ContainElement("fsm_type: benthosfsm"))
			Expect(events[0].Fingerprint).To(ContainElement("service_type: benthos"))
			Expect(events[0].Fingerprint).To(ContainElement("trigger: IsBenthosS6Stopped_empty_state"))
		})

		It("handles integer fields", func() {
			logger.Error("test error",
				zap.Int("retry_count", 5),
				zap.Int64("duration_ms", 1500),
			)

			Eventually(func() int {
				return eventStore.Len()
			}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

			events := eventStore.GetAll()
			Expect(events).To(HaveLen(1))
			Expect(events[0].Tags).To(HaveKeyWithValue("retry_count", "5"))
			Expect(events[0].Tags).To(HaveKeyWithValue("duration_ms", "1500"))
		})

		It("handles boolean fields", func() {
			logger.Error("test error",
				zap.Bool("is_retry", true),
			)

			Eventually(func() int {
				return eventStore.Len()
			}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

			events := eventStore.GetAll()
			Expect(events).To(HaveLen(1))
			Expect(events[0].Tags).To(HaveKeyWithValue("is_retry", "true"))
		})
	})

	Describe("Non-blocking behavior", func() {
		It("does not block the logger on Sentry calls", func() {
			// Use atomic counter for this test to avoid race with goroutines
			var counter atomic.Int64

			// Create a separate transport with atomic counter for this test
			atomicTransport := &atomicCounterTransport{counter: &counter}
			err := sentry.Init(sentry.ClientOptions{
				Dsn:       "https://test@sentry.io/123",
				Transport: atomicTransport,
			})
			Expect(err).NotTo(HaveOccurred())

			// Log many messages quickly - should not block
			start := time.Now()
			for range 100 {
				logger.Error("rapid error message", zap.Int("index", 1))
			}

			elapsed := time.Since(start)

			// Logging 100 messages should be fast (< 100ms) even if Sentry is slow
			Expect(elapsed).To(BeNumerically("<", 100*time.Millisecond))
		})
	})

	Describe("Core delegation", func() {
		It("delegates Enabled check to underlying core", func() {
			Expect(hook.Enabled(zapcore.DebugLevel)).To(BeTrue())
			Expect(hook.Enabled(zapcore.InfoLevel)).To(BeTrue())
			Expect(hook.Enabled(zapcore.WarnLevel)).To(BeTrue())
			Expect(hook.Enabled(zapcore.ErrorLevel)).To(BeTrue())
		})

		It("delegates With to create new wrapped core", func() {
			newHook := hook.With([]zapcore.Field{zap.String("component", "test")})
			Expect(newHook).NotTo(BeNil())

			// The new hook should be a SentryHook
			_, ok := newHook.(*SentryHook)
			Expect(ok).To(BeTrue())
		})

		It("delegates Check to underlying core", func() {
			entry := hook.Check(zapcore.Entry{Level: zapcore.ErrorLevel}, nil)
			Expect(entry).NotTo(BeNil())
		})
	})

	Describe("SugaredLogger integration", func() {
		It("works with SugaredLogger.Errorw", func() {
			sugar := logger.Sugar()
			sugar.Errorw("sugared error message",
				"operation", "test_op",
				"fsm_type", "testfsm",
			)

			Eventually(func() int {
				return eventStore.Len()
			}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

			events := eventStore.GetAll()
			Expect(events).To(HaveLen(1))
			Expect(events[0].Message).To(Equal("sugared error message"))
			Expect(events[0].Tags).To(HaveKeyWithValue("operation", "test_op"))
			Expect(events[0].Tags).To(HaveKeyWithValue("fsm_type", "testfsm"))
			Expect(events[0].Fingerprint).To(ContainElement("operation: test_op"))
			Expect(events[0].Fingerprint).To(ContainElement("fsm_type: testfsm"))
		})

		It("works with SugaredLogger.Warnw", func() {
			sugar := logger.Sugar()
			sugar.Warnw("sugared warning message",
				"trigger", "some_trigger",
			)

			Eventually(func() int {
				return eventStore.Len()
			}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

			events := eventStore.GetAll()
			Expect(events).To(HaveLen(1))
			Expect(events[0].Message).To(Equal("sugared warning message"))
			Expect(events[0].Level).To(Equal(sentry.LevelWarning))
			Expect(events[0].Fingerprint).To(ContainElement("trigger: some_trigger"))
		})
	})
})

// eventStore provides thread-safe storage for captured Sentry events.
type eventStore struct {
	events []*sentry.Event
	mutex  sync.Mutex
}

func newEventStore() *eventStore {
	return &eventStore{
		events: make([]*sentry.Event, 0),
	}
}

func (s *eventStore) Add(event *sentry.Event) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.events = append(s.events, event)
}

func (s *eventStore) Len() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return len(s.events)
}

func (s *eventStore) GetAll() []*sentry.Event {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Return a copy to avoid race conditions
	result := make([]*sentry.Event, len(s.events))
	copy(result, s.events)

	return result
}

// mockTransport captures Sentry events for testing.
type mockTransport struct {
	store *eventStore
}

func (t *mockTransport) Configure(options sentry.ClientOptions)    {}
func (t *mockTransport) Flush(timeout time.Duration) bool          { return true }
func (t *mockTransport) FlushWithContext(ctx context.Context) bool { return true }
func (t *mockTransport) Close()                                    {}

func (t *mockTransport) SendEvent(event *sentry.Event) {
	t.store.Add(event)
}

// atomicCounterTransport just counts events without storing them (for race-free perf tests).
type atomicCounterTransport struct {
	counter *atomic.Int64
}

func (t *atomicCounterTransport) Configure(options sentry.ClientOptions)    {}
func (t *atomicCounterTransport) Flush(timeout time.Duration) bool          { return true }
func (t *atomicCounterTransport) FlushWithContext(ctx context.Context) bool { return true }
func (t *atomicCounterTransport) Close()                                    {}

func (t *atomicCounterTransport) SendEvent(event *sentry.Event) {
	t.counter.Add(1)
}

// discardWriter discards all writes (like /dev/null).
type discardWriter struct{}

func (d *discardWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func (d *discardWriter) Sync() error {
	return nil
}
