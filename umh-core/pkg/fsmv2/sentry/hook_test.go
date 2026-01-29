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

package sentry_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sync"
	"time"

	sentrygo "github.com/getsentry/sentry-go"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/sentry"

	//nolint:revive // dot import for Ginkgo DSL
	. "github.com/onsi/ginkgo/v2"
	//nolint:revive // dot import for Gomega matchers
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var _ = Describe("SentryHook", func() {

	Describe("extractErrorTypes", func() {
		It("should return type chain for wrapped errors", func() {
			// Create a wrapped error chain
			innerErr := errors.New("inner error")
			wrappedErr := fmt.Errorf("outer: %w", innerErr)

			result := sentry.ExtractErrorTypes(wrappedErr)

			// Should contain both error types in chain
			Expect(result).To(ContainSubstring("*fmt.wrapError"))
			Expect(result).To(ContainSubstring("*errors.errorString"))
		})

		It("should return single type for unwrapped error", func() {
			err := errors.New("simple error")

			result := sentry.ExtractErrorTypes(err)

			Expect(result).To(Equal("*errors.errorString"))
		})

		It("should return empty string for nil error", func() {
			result := sentry.ExtractErrorTypes(nil)

			Expect(result).To(BeEmpty())
		})

		It("should handle deeply nested wrapped errors", func() {
			err1 := errors.New("level 1")
			err2 := fmt.Errorf("level 2: %w", err1)
			err3 := fmt.Errorf("level 3: %w", err2)

			result := sentry.ExtractErrorTypes(err3)

			// Should contain all three levels
			Expect(result).To(ContainSubstring("*fmt.wrapError"))
			Expect(result).To(ContainSubstring("*errors.errorString"))
			// Separator should be used
			Expect(result).To(ContainSubstring("|"))
		})
	})

	Describe("fieldsToMap", func() {
		It("should convert zap fields to map", func() {
			fields := []zapcore.Field{
				zap.String("feature", "communicator"),
				zap.String("worker_id", "worker-123"),
			}

			result := sentry.FieldsToMap(fields)

			Expect(result).To(HaveKey("feature"))
			Expect(result["feature"]).To(Equal("communicator"))
			Expect(result).To(HaveKey("worker_id"))
			Expect(result["worker_id"]).To(Equal("worker-123"))
		})

		It("should handle error fields", func() {
			testErr := errors.New("test error")
			fields := []zapcore.Field{
				zap.Error(testErr),
			}

			result := sentry.FieldsToMap(fields)

			Expect(result).To(HaveKey("error"))
		})

		It("should handle empty fields", func() {
			fields := []zapcore.Field{}

			result := sentry.FieldsToMap(fields)

			Expect(result).To(BeEmpty())
		})
	})

	Describe("zapLevelToSentry", func() {
		It("should convert debug level", func() {
			result := sentry.ZapLevelToSentry(zapcore.DebugLevel)
			Expect(string(result)).To(Equal("debug"))
		})

		It("should convert info level", func() {
			result := sentry.ZapLevelToSentry(zapcore.InfoLevel)
			Expect(string(result)).To(Equal("info"))
		})

		It("should convert warn level", func() {
			result := sentry.ZapLevelToSentry(zapcore.WarnLevel)
			Expect(string(result)).To(Equal("warning"))
		})

		It("should convert error level", func() {
			result := sentry.ZapLevelToSentry(zapcore.ErrorLevel)
			Expect(string(result)).To(Equal("error"))
		})

		It("should convert fatal level", func() {
			result := sentry.ZapLevelToSentry(zapcore.FatalLevel)
			Expect(string(result)).To(Equal("fatal"))
		})
	})

	Describe("BuildFingerprint", func() {
		It("should build fingerprint from level, feature, and event_name", func() {
			result := sentry.BuildFingerprint(zapcore.ErrorLevel, "communicator", "connection_failed", "")

			Expect(result).To(ContainElement("level: error"))
			Expect(result).To(ContainElement("feature: communicator"))
			Expect(result).To(ContainElement("event_name: connection_failed"))
		})

		It("should include error_types when provided", func() {
			result := sentry.BuildFingerprint(zapcore.ErrorLevel, "fsm", "transition_error", "*fmt.wrapError|*errors.errorString")

			Expect(result).To(HaveLen(4))
			Expect(result).To(ContainElement("error_types: *fmt.wrapError|*errors.errorString"))
		})

		It("should not include error_types when empty", func() {
			result := sentry.BuildFingerprint(zapcore.WarnLevel, "test", "test_event", "")

			Expect(result).To(HaveLen(3))
			Expect(result).NotTo(ContainElement(ContainSubstring("error_types")))
		})
	})

	Describe("ExtractFeature", func() {
		It("should extract feature field from map", func() {
			fieldMap := map[string]interface{}{
				"feature": "communicator",
				"other":   "value",
			}

			result := sentry.ExtractFeature(fieldMap)

			Expect(result).To(Equal("communicator"))
		})

		It("should return 'unknown' when feature is missing", func() {
			fieldMap := map[string]interface{}{
				"other": "value",
			}

			result := sentry.ExtractFeature(fieldMap)

			Expect(result).To(Equal("unknown"))
		})

		It("should return 'unknown' when feature is empty string", func() {
			fieldMap := map[string]interface{}{
				"feature": "",
			}

			result := sentry.ExtractFeature(fieldMap)

			Expect(result).To(Equal("unknown"))
		})

		It("should return 'unknown' when feature is not a string", func() {
			fieldMap := map[string]interface{}{
				"feature": 123,
			}

			result := sentry.ExtractFeature(fieldMap)

			Expect(result).To(Equal("unknown"))
		})
	})

	Describe("NewSentryHook", func() {
		It("should create hook with debouncer", func() {
			hook := sentry.NewSentryHook(5 * time.Minute)

			Expect(hook).NotTo(BeNil())
			Expect(hook.Debouncer()).NotTo(BeNil())
		})
	})

	Describe("ShouldCapture integration", func() {
		It("should call debouncer's ShouldCapture with correct fingerprint key", func() {
			hook := sentry.NewSentryHook(5 * time.Minute)

			// First capture should succeed
			fp := []string{"level: error", "feature: test", "event_name: test_event"}
			result1 := hook.Debouncer().ShouldCapture("level: error|feature: test|event_name: test_event")
			Expect(result1).To(BeTrue())

			// Immediate second capture with same fingerprint should be debounced
			result2 := hook.Debouncer().ShouldCapture("level: error|feature: test|event_name: test_event")
			Expect(result2).To(BeFalse())

			// Different fingerprint should succeed
			_ = fp // suppress unused warning
			result3 := hook.Debouncer().ShouldCapture("level: warn|feature: other|event_name: other_event")
			Expect(result3).To(BeTrue())
		})
	})
})

// Integration tests using mock transport to verify full Sentry event capture.
var _ = Describe("SentryHook Integration with Mock Transport", func() {
	var (
		hook      *sentry.SentryHook
		store     *eventStore
		transport *mockTransport
		logger    *zap.SugaredLogger
	)

	BeforeEach(func() {
		// Create fresh event store for each test
		store = newEventStore()

		// Create mock transport that captures events
		transport = &mockTransport{store: store}

		// Initialize Sentry with mock transport
		err := sentrygo.Init(sentrygo.ClientOptions{
			Dsn:       "https://test@sentry.io/123",
			Transport: transport,
		})
		Expect(err).NotTo(HaveOccurred())

		// Create underlying core (writes to /dev/null for tests)
		encoderConfig := zap.NewProductionEncoderConfig()
		core := zapcore.NewCore(
			zapcore.NewJSONEncoder(encoderConfig),
			zapcore.AddSync(&discardWriter{}),
			zapcore.DebugLevel,
		)

		// Create the SentryHook with 1-hour window (will reset in specific tests)
		hook = sentry.NewSentryHook(time.Hour)
		wrappedCore := hook.Wrap(core)

		// Create logger with the hook
		logger = zap.New(wrappedCore).Sugar()
	})

	AfterEach(func() {
		// Flush any pending events
		sentrygo.Flush(time.Second)
		time.Sleep(50 * time.Millisecond)
	})

	Describe("Error as Exception", func() {
		It("captures error as Exception via SetException", func() {
			// Given: wrapped error chain
			rootErr := io.EOF
			wrappedErr := fmt.Errorf("connection failed: %w", rootErr)

			// When: logged with ErrorFields
			logger.Errorw("action_failed", sentry.ErrorFields{
				Feature: "communicator",
				Err:     wrappedErr,
			}.ZapFields()...)

			// Then: event has Exception (not just Message)
			Eventually(func() int {
				return store.Len()
			}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

			event := store.GetLast()
			Expect(event).NotTo(BeNil())
			Expect(event.Exception).NotTo(BeEmpty(), "Error should be captured as Exception")
		})

		It("unwraps error chain capturing all types in fingerprint", func() {
			// Given: deeply nested error
			err1 := errors.New("root cause")
			err2 := fmt.Errorf("layer 2: %w", err1)
			err3 := fmt.Errorf("layer 3: %w", err2)

			// When: logged
			logger.Errorw("action_failed", sentry.ErrorFields{
				Feature: "fsmv2",
				Err:     err3,
			}.ZapFields()...)

			// Then: all error types in fingerprint
			Eventually(func() int {
				return store.Len()
			}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

			event := store.GetLast()
			Expect(event).NotTo(BeNil())

			// Find the error_types fingerprint element
			var errorTypesElement string

			for _, fp := range event.Fingerprint {
				if len(fp) > 13 && fp[:13] == "error_types: " {
					errorTypesElement = fp

					break
				}
			}

			Expect(errorTypesElement).NotTo(BeEmpty(), "Fingerprint should contain error_types")
			Expect(errorTypesElement).To(ContainSubstring("*fmt.wrapError"))
		})

		It("handles nil error gracefully", func() {
			// When: logged without error
			logger.Warnw("warning_event", sentry.ErrorFields{
				Feature: "fsmv2",
			}.ZapFields()...)

			// Then: event captured with no exception
			Eventually(func() int {
				return store.Len()
			}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

			event := store.GetLast()
			Expect(event).NotTo(BeNil())
			Expect(event.Exception).To(BeEmpty())
			Expect(event.Message).To(Equal("warning_event"))
		})

		It("captures error message in event", func() {
			// Given: error with specific message
			err := errors.New("specific error message")

			// When: logged
			logger.Errorw("action_failed", sentry.ErrorFields{
				Feature: "fsmv2",
				Err:     err,
			}.ZapFields()...)

			// Then: event message is the log message, not error message
			Eventually(func() int {
				return store.Len()
			}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

			event := store.GetLast()
			Expect(event).NotTo(BeNil())
			Expect(event.Message).To(Equal("action_failed"))
		})
	})

	Describe("Fingerprint Stability", func() {
		It("groups same error type with different URLs", func() {
			// Create new hook with very short debounce to allow both events
			store = newEventStore()
			transport.store = store
			hook = sentry.NewSentryHook(time.Nanosecond) // Effectively no debouncing
			wrappedCore := hook.Wrap(zapcore.NewCore(
				zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
				zapcore.AddSync(&discardWriter{}),
				zapcore.DebugLevel,
			))
			logger = zap.New(wrappedCore).Sugar()

			// Given: two errors with same type but different URLs
			err1 := &url.Error{Op: "Post", URL: "http://192.168.1.100:8090/api", Err: io.EOF}

			// Wait a bit to ensure separate events
			time.Sleep(10 * time.Millisecond)

			err2 := &url.Error{Op: "Post", URL: "http://10.0.0.50:8090/api", Err: io.EOF}

			// When: both logged
			logger.Errorw("push_failed", sentry.ErrorFields{Feature: "communicator", Err: err1}.ZapFields()...)
			time.Sleep(10 * time.Millisecond)
			logger.Errorw("push_failed", sentry.ErrorFields{Feature: "communicator", Err: err2}.ZapFields()...)

			// Then: same fingerprint (URLs excluded)
			Eventually(func() int {
				return store.Len()
			}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 2))

			events := store.GetAll()
			Expect(len(events)).To(BeNumerically(">=", 2))
			Expect(events[0].Fingerprint).To(Equal(events[1].Fingerprint),
				"Same error type with different URLs should produce same fingerprint")
		})

		It("separates different error types", func() {
			// Create new hook with no debouncing
			store = newEventStore()
			transport.store = store
			hook = sentry.NewSentryHook(time.Nanosecond)
			wrappedCore := hook.Wrap(zapcore.NewCore(
				zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
				zapcore.AddSync(&discardWriter{}),
				zapcore.DebugLevel,
			))
			logger = zap.New(wrappedCore).Sugar()

			// Given: two different error types
			err1 := io.EOF
			err2 := context.DeadlineExceeded

			// When: both logged
			logger.Errorw("action_failed", sentry.ErrorFields{Feature: "fsmv2", Err: err1}.ZapFields()...)
			time.Sleep(10 * time.Millisecond)
			logger.Errorw("action_failed", sentry.ErrorFields{Feature: "fsmv2", Err: err2}.ZapFields()...)

			// Then: different fingerprints
			Eventually(func() int {
				return store.Len()
			}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 2))

			events := store.GetAll()
			Expect(len(events)).To(BeNumerically(">=", 2))
			Expect(events[0].Fingerprint).NotTo(Equal(events[1].Fingerprint),
				"Different error types should produce different fingerprints")
		})

		It("includes feature in fingerprint", func() {
			// Create new hook with no debouncing
			store = newEventStore()
			transport.store = store
			hook = sentry.NewSentryHook(time.Nanosecond)
			wrappedCore := hook.Wrap(zapcore.NewCore(
				zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
				zapcore.AddSync(&discardWriter{}),
				zapcore.DebugLevel,
			))
			logger = zap.New(wrappedCore).Sugar()

			// Given: same error, different features
			err := io.EOF

			// When: both logged
			logger.Errorw("action_failed", sentry.ErrorFields{Feature: "fsmv2", Err: err}.ZapFields()...)
			time.Sleep(10 * time.Millisecond)
			logger.Errorw("action_failed", sentry.ErrorFields{Feature: "communicator", Err: err}.ZapFields()...)

			// Then: different fingerprints due to different features
			Eventually(func() int {
				return store.Len()
			}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 2))

			events := store.GetAll()
			Expect(len(events)).To(BeNumerically(">=", 2))
			Expect(events[0].Fingerprint).NotTo(Equal(events[1].Fingerprint),
				"Different features should produce different fingerprints")
		})
	})

	Describe("Per-Fingerprint Debouncing", func() {
		It("captures first event", func() {
			logger.Errorw("action_failed", sentry.ErrorFields{Feature: "fsmv2", Err: io.EOF}.ZapFields()...)

			Eventually(func() int {
				return store.Len()
			}, time.Second, 10*time.Millisecond).Should(Equal(1))
		})

		It("debounces duplicate within window", func() {
			// Both use same feature, same error, same message → same fingerprint
			logger.Errorw("action_failed", sentry.ErrorFields{Feature: "fsmv2", Err: io.EOF}.ZapFields()...)
			logger.Errorw("action_failed", sentry.ErrorFields{Feature: "fsmv2", Err: io.EOF}.ZapFields()...)

			// Wait a bit for any async processing
			time.Sleep(100 * time.Millisecond)

			// Second event should be debounced
			Expect(store.Len()).To(Equal(1))
		})

		It("captures event after window expires", func() {
			// Create hook with very short debounce window
			store = newEventStore()
			transport.store = store
			hook = sentry.NewSentryHook(50 * time.Millisecond) // 50ms window
			wrappedCore := hook.Wrap(zapcore.NewCore(
				zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
				zapcore.AddSync(&discardWriter{}),
				zapcore.DebugLevel,
			))
			logger = zap.New(wrappedCore).Sugar()

			// First event
			logger.Errorw("action_failed", sentry.ErrorFields{Feature: "fsmv2", Err: io.EOF}.ZapFields()...)

			Eventually(func() int {
				return store.Len()
			}, time.Second, 10*time.Millisecond).Should(Equal(1))

			// Wait for window to expire
			time.Sleep(100 * time.Millisecond)

			// Second event after window
			logger.Errorw("action_failed", sentry.ErrorFields{Feature: "fsmv2", Err: io.EOF}.ZapFields()...)

			Eventually(func() int {
				return store.Len()
			}, time.Second, 10*time.Millisecond).Should(Equal(2))
		})

		It("debounces different fingerprints independently", func() {
			// Different features = different fingerprints = both captured
			logger.Errorw("action_failed", sentry.ErrorFields{Feature: "fsmv2", Err: io.EOF}.ZapFields()...)
			logger.Errorw("action_failed", sentry.ErrorFields{Feature: "communicator", Err: io.EOF}.ZapFields()...)

			Eventually(func() int {
				return store.Len()
			}, time.Second, 10*time.Millisecond).Should(Equal(2))
		})
	})

	Describe("Hierarchy Path Auto-Tagging", func() {
		It("extracts fsm_version=v2 and worker_type from fsmv2 path", func() {
			logger.Errorw("action_failed", sentry.ErrorFields{
				Feature:       "communicator",
				HierarchyPath: "app(application)/worker(communicator)",
				Err:           io.EOF,
			}.ZapFields()...)

			Eventually(func() int {
				return store.Len()
			}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

			event := store.GetLast()
			Expect(event).NotTo(BeNil())
			Expect(event.Tags["fsm_version"]).To(Equal("v2"))
			Expect(event.Tags["worker_type"]).To(Equal("communicator"))
		})

		It("extracts fsm_version=v1 from legacy path", func() {
			logger.Errorw("action_failed", sentry.ErrorFields{
				Feature:       "legacy",
				HierarchyPath: "Enterprise.Site.Area.WorkCell",
				Err:           io.EOF,
			}.ZapFields()...)

			Eventually(func() int {
				return store.Len()
			}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

			event := store.GetLast()
			Expect(event).NotTo(BeNil())
			Expect(event.Tags["fsm_version"]).To(Equal("v1"))
			Expect(event.Tags["worker_type"]).To(Equal("WorkCell"))
		})

		It("handles empty hierarchy path gracefully", func() {
			logger.Errorw("action_failed", sentry.ErrorFields{
				Feature: "fsmv2",
				Err:     io.EOF,
			}.ZapFields()...)

			Eventually(func() int {
				return store.Len()
			}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

			event := store.GetLast()
			Expect(event).NotTo(BeNil())
			// Tags should not have fsm_version or worker_type if no hierarchy_path
			_, hasFsmVersion := event.Tags["fsm_version"]
			Expect(hasFsmVersion).To(BeFalse())
		})
	})

	Describe("Tag Extraction", func() {
		It("sets feature tag", func() {
			logger.Errorw("action_failed", sentry.ErrorFields{
				Feature: "communicator",
				Err:     io.EOF,
			}.ZapFields()...)

			Eventually(func() int {
				return store.Len()
			}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

			event := store.GetLast()
			Expect(event).NotTo(BeNil())
			Expect(event.Tags["feature"]).To(Equal("communicator"))
		})

		It("sets event_name tag from message", func() {
			logger.Errorw("custom_event_name", sentry.ErrorFields{
				Feature: "fsmv2",
				Err:     io.EOF,
			}.ZapFields()...)

			Eventually(func() int {
				return store.Len()
			}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

			event := store.GetLast()
			Expect(event).NotTo(BeNil())
			Expect(event.Tags["event_name"]).To(Equal("custom_event_name"))
		})

		It("uses 'unknown' for missing feature", func() {
			// Log without using ErrorFields (should still capture but use 'unknown')
			logger.Errorw("some_error", "some_field", "some_value")

			Eventually(func() int {
				return store.Len()
			}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

			event := store.GetLast()
			Expect(event).NotTo(BeNil())
			Expect(event.Tags["feature"]).To(Equal("unknown"))
		})
	})

	Describe("Level Filtering", func() {
		It("captures Error level logs to Sentry", func() {
			logger.Errorw("test_error", sentry.ErrorFields{Feature: "fsmv2"}.ZapFields()...)

			Eventually(func() int {
				return store.Len()
			}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

			event := store.GetLast()
			Expect(event.Level).To(Equal(sentrygo.LevelError))
		})

		It("captures Warn level logs to Sentry", func() {
			logger.Warnw("test_warning", sentry.ErrorFields{Feature: "fsmv2"}.ZapFields()...)

			Eventually(func() int {
				return store.Len()
			}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

			event := store.GetLast()
			Expect(event.Level).To(Equal(sentrygo.LevelWarning))
		})

		It("does NOT capture Info level logs to Sentry", func() {
			logger.Infow("test_info", sentry.ErrorFields{Feature: "fsmv2"}.ZapFields()...)

			time.Sleep(100 * time.Millisecond)

			Expect(store.Len()).To(Equal(0))
		})

		It("does NOT capture Debug level logs to Sentry", func() {
			logger.Debugw("test_debug", sentry.ErrorFields{Feature: "fsmv2"}.ZapFields()...)

			time.Sleep(100 * time.Millisecond)

			Expect(store.Len()).To(Equal(0))
		})
	})
})

// eventStore provides thread-safe storage for captured Sentry events.
type eventStore struct {
	events []*sentrygo.Event
	mutex  sync.Mutex
}

func newEventStore() *eventStore {
	return &eventStore{
		events: make([]*sentrygo.Event, 0),
	}
}

func (s *eventStore) Add(event *sentrygo.Event) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.events = append(s.events, event)
}

func (s *eventStore) Len() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return len(s.events)
}

func (s *eventStore) GetAll() []*sentrygo.Event {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Return a copy to avoid race conditions
	result := make([]*sentrygo.Event, len(s.events))
	copy(result, s.events)

	return result
}

func (s *eventStore) GetLast() *sentrygo.Event {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if len(s.events) == 0 {
		return nil
	}

	return s.events[len(s.events)-1]
}

// mockTransport captures Sentry events for testing.
type mockTransport struct {
	store *eventStore
}

func (t *mockTransport) Configure(_ sentrygo.ClientOptions) {}

func (t *mockTransport) Flush(_ time.Duration) bool {
	return true
}

func (t *mockTransport) FlushWithContext(_ context.Context) bool {
	return true
}

func (t *mockTransport) Close() {}

func (t *mockTransport) SendEvent(event *sentrygo.Event) {
	t.store.Add(event)
}

// discardWriter discards all writes (like /dev/null).
type discardWriter struct{}

func (d *discardWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func (d *discardWriter) Sync() error {
	return nil
}
