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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/sentry"

	//nolint:revive // dot import for Ginkgo DSL
	. "github.com/onsi/ginkgo/v2"
	//nolint:revive // dot import for Gomega matchers
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var _ = Describe("SentryHook", func() {

	Describe("ParseStackTrace", func() {
		It("should parse a valid Go stack trace", func() {
			// Sample stack trace from debug.Stack()
			stack := `goroutine 1 [running]:
runtime/debug.Stack()
	/usr/local/go/src/runtime/debug/stack.go:24 +0x5e
main.handlePanic()
	/app/handler.go:42 +0x1a
main.main()
	/app/main.go:15 +0x25
`
			result := sentry.ParseStackTrace(stack)

			Expect(result).NotTo(BeNil())
			Expect(result.Frames).NotTo(BeEmpty())
			Expect(len(result.Frames)).To(BeNumerically(">=", 2))

			// Verify frame structure
			var foundHandler bool

			for _, frame := range result.Frames {
				if frame.Function == "main.handlePanic" {
					foundHandler = true
					Expect(frame.Filename).To(Equal("handler.go"))
					Expect(frame.Lineno).To(Equal(42))
					Expect(frame.AbsPath).To(Equal("/app/handler.go"))
				}
			}

			Expect(foundHandler).To(BeTrue(), "Should find handlePanic frame")
		})

		It("should return nil for empty stack", func() {
			result := sentry.ParseStackTrace("")

			Expect(result).To(BeNil())
		})

		It("should return nil for invalid stack trace", func() {
			result := sentry.ParseStackTrace("not a valid stack trace")

			Expect(result).To(BeNil())
		})

		It("should handle multi-goroutine stack traces", func() {
			// Stack with multiple goroutines - should use first one
			stack := `goroutine 1 [running]:
main.first()
	/app/first.go:10 +0x1a

goroutine 2 [runnable]:
main.second()
	/app/second.go:20 +0x2b
`
			result := sentry.ParseStackTrace(stack)

			Expect(result).NotTo(BeNil())
			// Should only parse first goroutine
			var foundFirst bool

			for _, frame := range result.Frames {
				if frame.Function == "main.first" {
					foundFirst = true
				}
			}

			Expect(foundFirst).To(BeTrue())
		})
	})

	Describe("isInternalFrame", func() {
		It("should filter sentry package frames", func() {
			frame := sentrygo.Frame{
				Module:   "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry",
				Function: "reportError",
			}
			Expect(sentry.IsInternalFrame(frame)).To(BeTrue())
		})

		It("should filter fsmv2 sentry package frames", func() {
			frame := sentrygo.Frame{
				Module:   "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/sentry",
				Function: "captureToSentry",
			}
			Expect(sentry.IsInternalFrame(frame)).To(BeTrue())
		})

		It("should filter zap frames", func() {
			frame := sentrygo.Frame{
				Module:   "go.uber.org/zap",
				Function: "(*Logger).Error",
			}
			Expect(sentry.IsInternalFrame(frame)).To(BeTrue())
		})

		It("should filter runtime frames", func() {
			frame := sentrygo.Frame{
				Module:   "runtime/debug",
				Function: "Stack",
			}
			Expect(sentry.IsInternalFrame(frame)).To(BeTrue())
		})

		It("should NOT filter application frames", func() {
			frame := sentrygo.Frame{
				Module:   "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/agent",
				Function: "Agent.Run",
			}
			Expect(sentry.IsInternalFrame(frame)).To(BeFalse())
		})

		It("should NOT filter communicator frames", func() {
			frame := sentrygo.Frame{
				Module:   "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2",
				Function: "NewLogin",
			}
			Expect(sentry.IsInternalFrame(frame)).To(BeFalse())
		})
	})

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

	Describe("TruncateTag", func() {
		It("returns value unchanged when under limit", func() {
			result := sentry.TruncateTag("short", 200)
			Expect(result).To(Equal("short"))
		})

		It("truncates with ... when over limit", func() {
			long := ""
			for i := 0; i < 210; i++ {
				long += "a"
			}
			result := sentry.TruncateTag(long, 200)
			Expect(len(result)).To(Equal(200))
			Expect(result).To(HaveSuffix("..."))
		})

		It("handles exact limit length", func() {
			exact := ""
			for i := 0; i < 200; i++ {
				exact += "x"
			}
			result := sentry.TruncateTag(exact, 200)
			Expect(result).To(Equal(exact))
		})

		It("handles empty string", func() {
			result := sentry.TruncateTag("", 200)
			Expect(result).To(Equal(""))
		})
	})

	Describe("IsSensitiveKey", func() {
		It("returns true for each denylist entry", func() {
			Expect(sentry.IsSensitiveKey("password")).To(BeTrue())
			Expect(sentry.IsSensitiveKey("secret")).To(BeTrue())
			Expect(sentry.IsSensitiveKey("token")).To(BeTrue())
			Expect(sentry.IsSensitiveKey("credential")).To(BeTrue())
			Expect(sentry.IsSensitiveKey("auth_token")).To(BeTrue())
			Expect(sentry.IsSensitiveKey("api_key")).To(BeTrue())
			Expect(sentry.IsSensitiveKey("private_key")).To(BeTrue())
		})

		It("returns false for non-sensitive keys", func() {
			Expect(sentry.IsSensitiveKey("reason")).To(BeFalse())
			Expect(sentry.IsSensitiveKey("duration_ms")).To(BeFalse())
			Expect(sentry.IsSensitiveKey("worker_id")).To(BeFalse())
		})

		It("is case-insensitive", func() {
			Expect(sentry.IsSensitiveKey("PASSWORD")).To(BeTrue())
			Expect(sentry.IsSensitiveKey("Token")).To(BeTrue())
		})

		It("uses exact match not substring", func() {
			Expect(sentry.IsSensitiveKey("cache_key")).To(BeFalse())
			Expect(sentry.IsSensitiveKey("worker_token_count")).To(BeFalse())
			Expect(sentry.IsSensitiveKey("primary_key")).To(BeFalse())
		})
	})

	Describe("NewSentryHook", func() {
		It("should create hook with debouncer", func() {
			hook := sentry.NewSentryHook(5 * time.Minute)
			defer hook.Stop()

			Expect(hook).NotTo(BeNil())
			Expect(hook.Debouncer()).NotTo(BeNil())
		})
	})

	Describe("ShouldCapture integration", func() {
		It("should call debouncer's ShouldCapture with correct fingerprint key", func() {
			hook := sentry.NewSentryHook(5 * time.Minute)
			defer hook.Stop()

			// First capture should succeed
			result1 := hook.Debouncer().ShouldCapture("level: error|feature: test|event_name: test_event")
			Expect(result1).To(BeTrue())

			// Immediate second capture with same fingerprint should be debounced
			result2 := hook.Debouncer().ShouldCapture("level: error|feature: test|event_name: test_event")
			Expect(result2).To(BeFalse())

			// Different fingerprint should succeed
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

		// Stop hook to release cleanup goroutine
		if hook != nil {
			hook.Stop()
		}
	})

	Describe("Error as Exception", func() {
		It("captures error as Exception", func() {
			// Given: wrapped error chain
			rootErr := io.EOF
			wrappedErr := fmt.Errorf("connection failed: %w", rootErr)

			// When: logged with error fields
			logger.Errorw("action_failed",
				"feature", "communicator",
				"error", wrappedErr)

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
			logger.Errorw("action_failed",
				"feature", "fsmv2",
				"error", err3)

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
			logger.Warnw("warning_event",
				"feature", "fsmv2")

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
			logger.Errorw("action_failed",
				"feature", "fsmv2",
				"error", err)

			// Then: event message is the log message, not error message
			Eventually(func() int {
				return store.Len()
			}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

			event := store.GetLast()
			Expect(event).NotTo(BeNil())
			Expect(event.Message).To(Equal("action_failed"))
		})

		It("attaches stacktrace to exception when stack field is present", func() {
			// Given: error with a stack trace (simulating panic recovery)
			err := errors.New("action panicked")
			stack := `goroutine 1 [running]:
runtime/debug.Stack()
	/usr/local/go/src/runtime/debug/stack.go:24 +0x5e
github.com/example/pkg/executor.executeWork()
	/app/executor.go:142 +0x1a
`
			// When: logged with stack field (like action_executor does for panics)
			logger.Errorw("action_panic",
				"feature", "fsmv2",
				"error", err)
			// Add stack field separately to match action_executor pattern
			logger.Errorw("action_panic_with_stack",
				"feature", "fsmv2",
				"error", err,
				"stack", stack,
				"panic", "test panic value",
				"action_name", "test_action",
			)

			// Then: exception should have stacktrace attached
			Eventually(func() int {
				return store.Len()
			}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 2))

			events := store.GetAll()
			// Find the event with stack
			var eventWithStack *sentrygo.Event

			for _, evt := range events {
				if evt.Message == "action_panic_with_stack" {
					eventWithStack = evt

					break
				}
			}

			Expect(eventWithStack).NotTo(BeNil())
			Expect(eventWithStack.Exception).NotTo(BeEmpty())
			Expect(eventWithStack.Exception[0].Stacktrace).NotTo(BeNil(),
				"Exception should have parsed stacktrace attached")
			Expect(eventWithStack.Exception[0].Stacktrace.Frames).NotTo(BeEmpty(),
				"Stacktrace should have parsed frames")
		})
	})

	Describe("With() Derived Logger", func() {
		It("maintains Sentry capture through logger.With() derived loggers", func() {
			// This tests a critical fix: when supervisors call logger.With("worker", "...")
			// the derived logger must still capture errors to Sentry.

			// Create a derived logger using With()
			derivedLogger := logger.With("worker", "test-worker-123")

			// Log an error through the derived logger
			derivedLogger.Errorw("action_failed",
				"feature", "fsmv2",
				"error", io.EOF)

			// Event should be captured to Sentry
			Eventually(func() int {
				return store.Len()
			}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

			event := store.GetLast()
			Expect(event).NotTo(BeNil())
			Expect(event.Message).To(Equal("action_failed"))
			Expect(event.Exception).NotTo(BeEmpty(), "Error should be captured as Exception")
		})

		It("maintains Sentry capture through multiple With() calls", func() {
			// Test chained With() calls
			derivedLogger := logger.With("worker", "test-worker").With("action", "test-action")

			derivedLogger.Errorw("nested_error",
				"feature", "fsmv2",
				"error", errors.New("nested test error"))

			Eventually(func() int {
				return store.Len()
			}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

			event := store.GetLast()
			Expect(event).NotTo(BeNil())
			Expect(event.Message).To(Equal("nested_error"))
		})

		It("shares debouncer across derived loggers", func() {
			// Both loggers should share the same debouncer, so duplicate events are suppressed
			derivedLogger1 := logger.With("worker", "worker-1")
			derivedLogger2 := logger.With("worker", "worker-2")

			// Log same error from both derived loggers
			derivedLogger1.Errorw("shared_error",
				"feature", "fsmv2",
				"error", io.EOF)

			derivedLogger2.Errorw("shared_error",
				"feature", "fsmv2",
				"error", io.EOF)

			// Wait a bit for any async processing
			time.Sleep(100 * time.Millisecond)

			// Only one event should be captured (second is debounced due to same fingerprint)
			Expect(store.Len()).To(Equal(1), "Debouncer should suppress duplicate from derived logger")
		})
	})

	Describe("Fingerprint Stability", func() {
		It("groups same error type with different URLs", func() {
			// Create new hook with very short debounce to allow both events
			store = newEventStore()
			transport.store = store
			hook.Stop()
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
			logger.Errorw("push_failed", "feature", "communicator", "error", err1)
			time.Sleep(10 * time.Millisecond)
			logger.Errorw("push_failed", "feature", "communicator", "error", err2)

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
			hook.Stop()
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
			logger.Errorw("action_failed", "feature", "fsmv2", "error", err1)
			time.Sleep(10 * time.Millisecond)
			logger.Errorw("action_failed", "feature", "fsmv2", "error", err2)

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
			hook.Stop()
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
			logger.Errorw("action_failed", "feature", "fsmv2", "error", err)
			time.Sleep(10 * time.Millisecond)
			logger.Errorw("action_failed", "feature", "communicator", "error", err)

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
			logger.Errorw("action_failed", "feature", "fsmv2", "error", io.EOF)

			Eventually(func() int {
				return store.Len()
			}, time.Second, 10*time.Millisecond).Should(Equal(1))
		})

		It("debounces duplicate within window", func() {
			// Both use same feature, same error, same message → same fingerprint
			logger.Errorw("action_failed", "feature", "fsmv2", "error", io.EOF)
			logger.Errorw("action_failed", "feature", "fsmv2", "error", io.EOF)

			// Wait a bit for any async processing
			time.Sleep(100 * time.Millisecond)

			// Second event should be debounced
			Expect(store.Len()).To(Equal(1))
		})

		It("captures event after window expires", func() {
			// Create hook with very short debounce window
			store = newEventStore()
			transport.store = store
			hook.Stop()
			hook = sentry.NewSentryHook(50 * time.Millisecond) // 50ms window
			wrappedCore := hook.Wrap(zapcore.NewCore(
				zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
				zapcore.AddSync(&discardWriter{}),
				zapcore.DebugLevel,
			))
			logger = zap.New(wrappedCore).Sugar()

			// First event
			logger.Errorw("action_failed", "feature", "fsmv2", "error", io.EOF)

			Eventually(func() int {
				return store.Len()
			}, time.Second, 10*time.Millisecond).Should(Equal(1))

			// Wait for window to expire
			time.Sleep(100 * time.Millisecond)

			// Second event after window
			logger.Errorw("action_failed", "feature", "fsmv2", "error", io.EOF)

			Eventually(func() int {
				return store.Len()
			}, time.Second, 10*time.Millisecond).Should(Equal(2))
		})

		It("debounces different fingerprints independently", func() {
			// Different features = different fingerprints = both captured
			logger.Errorw("action_failed", "feature", "fsmv2", "error", io.EOF)
			logger.Errorw("action_failed", "feature", "communicator", "error", io.EOF)

			Eventually(func() int {
				return store.Len()
			}, time.Second, 10*time.Millisecond).Should(Equal(2))
		})
	})

	Describe("Hierarchy Path Auto-Tagging", func() {
		It("extracts fsm_version=v2, worker_type, and worker_chain from fsmv2 path", func() {
			logger.Errorw("action_failed",
				"feature", "communicator",
				"error", io.EOF,
				"hierarchy_path", "app(application)/worker(communicator)")

			Eventually(func() int {
				return store.Len()
			}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

			event := store.GetLast()
			Expect(event).NotTo(BeNil())
			Expect(event.Tags["fsm_version"]).To(Equal("v2"))
			Expect(event.Tags["worker_type"]).To(Equal("communicator"))
			Expect(event.Tags["worker_chain"]).To(Equal("application/communicator"))
		})

		It("extracts fsm_version=v1 from legacy path", func() {
			logger.Errorw("action_failed",
				"feature", "legacy",
				"error", io.EOF,
				"hierarchy_path", "Enterprise.Site.Area.WorkCell")

			Eventually(func() int {
				return store.Len()
			}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

			event := store.GetLast()
			Expect(event).NotTo(BeNil())
			Expect(event.Tags["fsm_version"]).To(Equal("v1"))
			Expect(event.Tags["worker_type"]).To(Equal("WorkCell"))
			Expect(event.Tags["worker_chain"]).To(Equal("Enterprise/Site/Area/WorkCell"))
		})

		It("handles empty hierarchy path gracefully", func() {
			logger.Errorw("action_failed",
				"feature", "fsmv2",
				"error", io.EOF)

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
			logger.Errorw("action_failed",
				"feature", "communicator",
				"error", io.EOF)

			Eventually(func() int {
				return store.Len()
			}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

			event := store.GetLast()
			Expect(event).NotTo(BeNil())
			Expect(event.Tags["feature"]).To(Equal("communicator"))
		})

		It("sets event_name tag from message", func() {
			logger.Errorw("custom_event_name",
				"feature", "fsmv2",
				"error", io.EOF)

			Eventually(func() int {
				return store.Len()
			}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

			event := store.GetLast()
			Expect(event).NotTo(BeNil())
			Expect(event.Tags["event_name"]).To(Equal("custom_event_name"))
		})

		It("uses 'unknown' for missing feature", func() {
			// Log without feature field (should still capture but use 'unknown')
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
			logger.Errorw("test_error", "feature", "fsmv2")

			Eventually(func() int {
				return store.Len()
			}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

			event := store.GetLast()
			Expect(event.Level).To(Equal(sentrygo.LevelError))
		})

		It("captures Warn level logs to Sentry", func() {
			logger.Warnw("test_warning", "feature", "fsmv2")

			Eventually(func() int {
				return store.Len()
			}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

			event := store.GetLast()
			Expect(event.Level).To(Equal(sentrygo.LevelWarning))
		})

		It("does NOT capture Info level logs to Sentry", func() {
			logger.Infow("test_info", "feature", "fsmv2")

			time.Sleep(100 * time.Millisecond)

			Expect(store.Len()).To(Equal(0))
		})

		It("does NOT capture Debug level logs to Sentry", func() {
			logger.Debugw("test_debug", "feature", "fsmv2")

			time.Sleep(100 * time.Millisecond)

			Expect(store.Len()).To(Equal(0))
		})
	})
})

// Contexts catch-all tests verify that extra fields reach Sentry via event.Contexts["umh_context"].
var _ = Describe("Contexts Catch-All", func() {
	var (
		hook      *sentry.SentryHook
		store     *eventStore
		transport *mockTransport
		logger    *zap.SugaredLogger
	)

	BeforeEach(func() {
		store = newEventStore()
		transport = &mockTransport{store: store}
		err := sentrygo.Init(sentrygo.ClientOptions{
			Dsn:       "https://test@sentry.io/123",
			Transport: transport,
		})
		Expect(err).NotTo(HaveOccurred())
		hook = sentry.NewSentryHook(time.Hour)
		wrappedCore := hook.Wrap(zapcore.NewCore(
			zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
			zapcore.AddSync(&discardWriter{}),
			zapcore.DebugLevel,
		))
		logger = zap.New(wrappedCore).Sugar()
	})

	AfterEach(func() {
		sentrygo.Flush(time.Second)
		time.Sleep(50 * time.Millisecond)
		if hook != nil {
			hook.Stop()
		}
	})

	It("captures extra fields in Contexts['umh_context']", func() {
		logger.Errorw("action_failed",
			"feature", "fsmv2",
			"error", io.EOF,
			"reason", "timeout",
			"duration_ms", 500)

		Eventually(func() int {
			return store.Len()
		}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

		event := store.GetLast()
		Expect(event).NotTo(BeNil())
		ctx, ok := event.Contexts["umh_context"]
		Expect(ok).To(BeTrue(), "should have umh_context")
		Expect(ctx["reason"]).To(Equal("timeout"))
	})

	It("excludes handled keys from Contexts", func() {
		logger.Errorw("action_panic",
			"feature", "fsmv2",
			"error", io.EOF,
			"hierarchy_path", "app(application)/w(communicator)",
			"stack", "goroutine 1 [running]:\n",
			"panic", "test panic",
			"action_name", "connect",
			"reason", "should_appear")

		Eventually(func() int {
			return store.Len()
		}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

		event := store.GetLast()
		ctx, ok := event.Contexts["umh_context"]
		Expect(ok).To(BeTrue())
		// "reason" should be in context
		Expect(ctx).To(HaveKey("reason"))
		// Handled keys should NOT be in context (except panic_value which is remapped)
		Expect(ctx).NotTo(HaveKey("feature"))
		Expect(ctx).NotTo(HaveKey("stack"))
		Expect(ctx).NotTo(HaveKey("action_name"))
		Expect(ctx).NotTo(HaveKey("hierarchy_path"))
		// panic is excluded but panic_value is explicitly added
		Expect(ctx).NotTo(HaveKey("panic"))
		Expect(ctx).To(HaveKey("panic_value"))
	})

	It("excludes 'error' from Contexts when typed error is present", func() {
		logger.Errorw("action_failed",
			"feature", "fsmv2",
			"error", io.EOF,
			"reason", "test")

		Eventually(func() int {
			return store.Len()
		}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

		event := store.GetLast()
		ctx, ok := event.Contexts["umh_context"]
		Expect(ok).To(BeTrue())
		Expect(ctx).NotTo(HaveKey("error"), "error should be excluded when typed error present")
		Expect(event.Exception).NotTo(BeEmpty(), "typed error should be in Exception")
	})

	It("includes 'error' string in Contexts when no typed error (SentryWarn pattern)", func() {
		// Simulate SentryWarn + deps.Err: error becomes a string field, not typed
		logger.Warnw("shutdown_failed",
			"feature", "fsmv2",
			"error", "connection reset by peer")

		Eventually(func() int {
			return store.Len()
		}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

		event := store.GetLast()
		ctx, ok := event.Contexts["umh_context"]
		Expect(ok).To(BeTrue())
		Expect(ctx["error"]).To(Equal("connection reset by peer"))
		Expect(event.Exception).To(BeEmpty())
	})

	It("does not create Contexts when no extra fields exist", func() {
		logger.Warnw("simple_warning",
			"feature", "fsmv2")

		Eventually(func() int {
			return store.Len()
		}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

		event := store.GetLast()
		_, ok := event.Contexts["umh_context"]
		Expect(ok).To(BeFalse(), "no umh_context when no extra fields")
	})

	It("captures panic_value in Contexts instead of Extra", func() {
		logger.Errorw("action_panic",
			"feature", "fsmv2",
			"error", errors.New("panicked"),
			"panic", "runtime error: nil pointer",
			"stack", "goroutine 1 [running]:\n",
			"action_name", "test_action")

		Eventually(func() int {
			return store.Len()
		}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

		event := store.GetLast()
		ctx, ok := event.Contexts["umh_context"]
		Expect(ok).To(BeTrue())
		Expect(ctx["panic_value"]).To(Equal("runtime error: nil pointer"))
		// Extra should not have panic_value
		Expect(event.Extra).To(BeEmpty())
	})

	It("filters sensitive keys from Contexts", func() {
		logger.Errorw("auth_error",
			"feature", "fsmv2",
			"error", io.EOF,
			"password", "secret123",
			"reason", "timeout")

		Eventually(func() int {
			return store.Len()
		}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

		event := store.GetLast()
		ctx, ok := event.Contexts["umh_context"]
		Expect(ok).To(BeTrue())
		Expect(ctx).NotTo(HaveKey("password"))
		Expect(ctx["reason"]).To(Equal("timeout"))
	})

	It("does not filter non-sensitive keys with sensitive substrings", func() {
		logger.Warnw("test_event",
			"feature", "fsmv2",
			"cache_key", "abc",
			"worker_token_count", "5")

		Eventually(func() int {
			return store.Len()
		}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

		event := store.GetLast()
		ctx, ok := event.Contexts["umh_context"]
		Expect(ok).To(BeTrue())
		Expect(ctx).To(HaveKey("cache_key"))
		Expect(ctx).To(HaveKey("worker_token_count"))
	})

	It("truncates long string values in Contexts at 1024 chars", func() {
		longVal := ""
		for i := 0; i < 2000; i++ {
			longVal += "x"
		}
		logger.Warnw("test_event",
			"feature", "fsmv2",
			"long_field", longVal)

		Eventually(func() int {
			return store.Len()
		}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

		event := store.GetLast()
		ctx, ok := event.Contexts["umh_context"]
		Expect(ok).To(BeTrue())
		val, ok := ctx["long_field"].(string)
		Expect(ok).To(BeTrue())
		Expect(len(val)).To(BeNumerically("<=", 1024+len("...[truncated]")))
		Expect(val).To(HaveSuffix("...[truncated]"))
	})

	It("does not truncate short string values", func() {
		logger.Warnw("test_event",
			"feature", "fsmv2",
			"short_field", "hello")

		Eventually(func() int {
			return store.Len()
		}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

		event := store.GetLast()
		ctx, ok := event.Contexts["umh_context"]
		Expect(ok).To(BeTrue())
		Expect(ctx["short_field"]).To(Equal("hello"))
	})
})

// Tag truncation tests verify that long tag values are safely truncated.
var _ = Describe("Tag Truncation", func() {
	var (
		hook      *sentry.SentryHook
		store     *eventStore
		transport *mockTransport
		logger    *zap.SugaredLogger
	)

	BeforeEach(func() {
		store = newEventStore()
		transport = &mockTransport{store: store}
		err := sentrygo.Init(sentrygo.ClientOptions{
			Dsn:       "https://test@sentry.io/123",
			Transport: transport,
		})
		Expect(err).NotTo(HaveOccurred())
		hook = sentry.NewSentryHook(time.Hour)
		wrappedCore := hook.Wrap(zapcore.NewCore(
			zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
			zapcore.AddSync(&discardWriter{}),
			zapcore.DebugLevel,
		))
		logger = zap.New(wrappedCore).Sugar()
	})

	AfterEach(func() {
		sentrygo.Flush(time.Second)
		time.Sleep(50 * time.Millisecond)
		if hook != nil {
			hook.Stop()
		}
	})

	It("truncates error_types exceeding 200 chars", func() {
		// Build a deeply wrapped error chain to exceed 200 chars
		var err error = errors.New("root")
		for i := 0; i < 15; i++ {
			err = fmt.Errorf("wrap_%d: %w", i, err)
		}
		// error_types for 15 wraps + 1 base: ~16 types * ~20 chars = ~320 chars

		logger.Errorw("action_failed",
			"feature", "fsmv2",
			"error", err)

		Eventually(func() int {
			return store.Len()
		}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

		event := store.GetLast()
		Expect(len(event.Tags["error_types"])).To(BeNumerically("<=", 200))
		Expect(event.Tags["error_types"]).To(HaveSuffix("..."))
	})

	It("does not truncate error_types under 200 chars", func() {
		err := fmt.Errorf("wrap: %w", io.EOF)

		logger.Errorw("action_failed",
			"feature", "fsmv2",
			"error", err)

		Eventually(func() int {
			return store.Len()
		}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

		event := store.GetLast()
		Expect(event.Tags["error_types"]).NotTo(HaveSuffix("..."))
	})
})

// FSMLogger-level tests verify the full pipeline: FSMLogger → zap → SentryHook → Sentry event.
// This complements the raw-zap tests above by proving the FSMLogger wrapper produces
// identical Sentry events to hand-written Errorw/Warnw calls.
var _ = Describe("FSMLogger to Sentry Event Mapping", func() {
	var (
		hook      *sentry.SentryHook
		store     *eventStore
		transport *mockTransport
		fsmLogger deps.FSMLogger
	)

	BeforeEach(func() {
		store = newEventStore()
		transport = &mockTransport{store: store}

		err := sentrygo.Init(sentrygo.ClientOptions{
			Dsn:       "https://test@sentry.io/123",
			Transport: transport,
		})
		Expect(err).NotTo(HaveOccurred())

		encoderConfig := zap.NewProductionEncoderConfig()
		core := zapcore.NewCore(
			zapcore.NewJSONEncoder(encoderConfig),
			zapcore.AddSync(&discardWriter{}),
			zapcore.DebugLevel,
		)

		hook = sentry.NewSentryHook(time.Hour)
		wrappedCore := hook.Wrap(core)
		sugar := zap.New(wrappedCore).Sugar()
		fsmLogger = deps.NewFSMLogger(sugar)
	})

	AfterEach(func() {
		sentrygo.Flush(time.Second)
		time.Sleep(50 * time.Millisecond)
		if hook != nil {
			hook.Stop()
		}
	})

	It("SentryError produces exception with correct type and value", func() {
		testErr := fmt.Errorf("connection refused: %w", io.EOF)
		fsmLogger.SentryError(deps.FeatureFSMv2, "app(application)/w1(helloworld)", testErr, "action_failed",
			deps.ActionName("connect"))

		Eventually(func() int {
			return store.Len()
		}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

		event := store.GetLast()
		Expect(event.Exception).NotTo(BeEmpty())
		Expect(event.Exception[0].Type).To(Equal("action_failed"))
		Expect(event.Exception[0].Value).To(ContainSubstring("connection refused"))
	})

	It("SentryError sets feature tag", func() {
		fsmLogger.SentryError(deps.FeatureFSMv2, "", io.EOF, "action_failed")

		Eventually(func() int {
			return store.Len()
		}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

		event := store.GetLast()
		Expect(event.Tags["feature"]).To(Equal("fsmv2"))
	})

	It("SentryError sets event_name tag from message", func() {
		fsmLogger.SentryError(deps.FeatureFSMv2, "", io.EOF, "action_failed")

		Eventually(func() int {
			return store.Len()
		}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

		event := store.GetLast()
		Expect(event.Tags["event_name"]).To(Equal("action_failed"))
	})

	It("SentryError extracts error types from wrapped chain", func() {
		rootErr := errors.New("root cause")
		wrapped := fmt.Errorf("layer: %w", rootErr)
		fsmLogger.SentryError(deps.FeatureFSMv2, "", wrapped, "action_failed")

		Eventually(func() int {
			return store.Len()
		}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

		event := store.GetLast()
		Expect(event.Tags["error_types"]).To(ContainSubstring("*fmt.wrapError"))
		Expect(event.Tags["error_types"]).To(ContainSubstring("*errors.errorString"))
	})

	It("SentryError with hierarchy_path derives fsm_version, worker_type, and worker_chain", func() {
		fsmLogger.SentryError(deps.FeatureForWorker("communicator"), "app(application)/worker(communicator)", io.EOF, "action_failed")

		Eventually(func() int {
			return store.Len()
		}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

		event := store.GetLast()
		Expect(event.Tags["feature"]).To(Equal("communicator"))
		Expect(event.Tags["fsm_version"]).To(Equal("v2"))
		Expect(event.Tags["worker_type"]).To(Equal("communicator"))
		Expect(event.Tags["worker_chain"]).To(Equal("application/communicator"))
	})

	It("SentryWarn captures at warn level without exception", func() {
		fsmLogger.SentryWarn(deps.FeatureFSMv2, "", "collector_unresponsive",
			deps.Attempts(3))

		Eventually(func() int {
			return store.Len()
		}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

		event := store.GetLast()
		Expect(event.Level).To(Equal(sentrygo.LevelWarning))
		Expect(event.Exception).To(BeEmpty())
		Expect(event.Tags["feature"]).To(Equal("fsmv2"))
		Expect(event.Tags["event_name"]).To(Equal("collector_unresponsive"))
	})

	It("With() context is preserved through FSMLogger", func() {
		scoped := fsmLogger.With(deps.WorkerID("test-worker"))
		scoped.SentryError(deps.FeatureFSMv2, "", io.EOF, "worker_failed")

		Eventually(func() int {
			return store.Len()
		}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

		event := store.GetLast()
		Expect(event.Tags["feature"]).To(Equal("fsmv2"))
	})

	It("SentryWarn with deps.Err still extracts typed error into Exception", func() {
		// deps.Err() creates a typed error field that zap preserves as ErrorType.
		// ExtractErrorFromFields finds it, so even SentryWarn produces an Exception.
		fsmLogger.SentryWarn(deps.FeatureFSMv2, "", "shutdown_failed",
			deps.Err(errors.New("connection reset")))

		Eventually(func() int {
			return store.Len()
		}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

		event := store.GetLast()
		Expect(event.Exception).NotTo(BeEmpty(), "deps.Err creates typed error, extracted as Exception")
		Expect(event.Exception[0].Value).To(ContainSubstring("connection reset"))
	})

	It("SentryError with extra fields captures them in Contexts", func() {
		fsmLogger.SentryError(deps.FeatureFSMv2, "", io.EOF, "action_failed",
			deps.Attempts(3), deps.String("reason", "timeout"))

		Eventually(func() int {
			return store.Len()
		}, time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

		event := store.GetLast()
		ctx, ok := event.Contexts["umh_context"]
		Expect(ok).To(BeTrue())
		Expect(ctx["reason"]).To(Equal("timeout"))
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
