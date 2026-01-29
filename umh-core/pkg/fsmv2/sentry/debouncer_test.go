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
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/sentry"

	//nolint:revive // dot import for Ginkgo DSL
	. "github.com/onsi/ginkgo/v2"
	//nolint:revive // dot import for Gomega matchers
	. "github.com/onsi/gomega"
)

var _ = Describe("FingerprintDebouncer", func() {

	Describe("ShouldCapture", func() {

		It("should return true for first call with any fingerprint", func() {
			debouncer := sentry.NewFingerprintDebouncer(100 * time.Millisecond)

			result := debouncer.ShouldCapture("fingerprint-1")

			Expect(result).To(BeTrue())
		})

		It("should return false for immediate second call with same fingerprint", func() {
			debouncer := sentry.NewFingerprintDebouncer(100 * time.Millisecond)

			// First call - should capture
			_ = debouncer.ShouldCapture("fingerprint-1")

			// Immediate second call - should be debounced
			result := debouncer.ShouldCapture("fingerprint-1")

			Expect(result).To(BeFalse())
		})

		It("should return true after debounce window expires", func() {
			debouncer := sentry.NewFingerprintDebouncer(50 * time.Millisecond)

			// First call
			_ = debouncer.ShouldCapture("fingerprint-1")

			// Wait for debounce window to expire
			time.Sleep(60 * time.Millisecond)

			// Call after window expired - should capture again
			result := debouncer.ShouldCapture("fingerprint-1")

			Expect(result).To(BeTrue())
		})

		It("should return true for different fingerprints (independent debouncing)", func() {
			debouncer := sentry.NewFingerprintDebouncer(100 * time.Millisecond)

			// First call with fingerprint-1
			result1 := debouncer.ShouldCapture("fingerprint-1")
			Expect(result1).To(BeTrue())

			// Call with different fingerprint - should capture independently
			result2 := debouncer.ShouldCapture("fingerprint-2")
			Expect(result2).To(BeTrue())

			// Second call with fingerprint-1 - should be debounced
			result3 := debouncer.ShouldCapture("fingerprint-1")
			Expect(result3).To(BeFalse())

			// Second call with fingerprint-2 - should also be debounced
			result4 := debouncer.ShouldCapture("fingerprint-2")
			Expect(result4).To(BeFalse())
		})
	})
})
