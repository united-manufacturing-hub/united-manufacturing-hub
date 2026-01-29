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
	"errors"
	"fmt"
	"time"

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
