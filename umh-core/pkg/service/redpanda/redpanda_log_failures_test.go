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

package redpanda_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda"
)

var _ = Describe("Redpanda Log Failures", func() {
	Describe("AddressAlreadyInUseFailure", func() {
		var failure *redpanda.AddressAlreadyInUseFailure

		BeforeEach(func() {
			failure = &redpanda.AddressAlreadyInUseFailure{}
		})

		It("should detect address already in use failure", func() {
			Expect(failure.IsFailure("Address already in use")).To(BeTrue())
		})

		It("should not detect failure in normal log line", func() {
			Expect(failure.IsFailure("Normal log line")).To(BeFalse())
		})

		It("should be case sensitive", func() {
			Expect(failure.IsFailure("address already in use")).To(BeFalse())
		})
	})

	Describe("ReactorStalledFailure", func() {
		var failure *redpanda.ReactorStalledFailure

		BeforeEach(func() {
			failure = &redpanda.ReactorStalledFailure{}
		})

		It("should detect reactor stall over 500ms", func() {
			Expect(failure.IsFailure("Reactor stalled for 501 ms")).To(BeTrue())
		})

		It("should not detect reactor stall under 500ms", func() {
			Expect(failure.IsFailure("Reactor stalled for 499 ms")).To(BeFalse())
		})

		It("should not detect failure in normal log line", func() {
			Expect(failure.IsFailure("Normal log line")).To(BeFalse())
		})

		It("should handle malformed log lines", func() {
			Expect(failure.IsFailure("Reactor stalled for ms")).To(BeFalse())
			Expect(failure.IsFailure("Reactor stalled for abc ms")).To(BeFalse())
		})

		It("should be case sensitive", func() {
			Expect(failure.IsFailure("reactor stalled for 501 ms")).To(BeFalse())
		})
	})

	Describe("RedpandaFailures", func() {
		It("should contain all failure types", func() {
			Expect(redpanda.RedpandaFailures).To(HaveLen(2))
			Expect(redpanda.RedpandaFailures[0]).To(BeAssignableToTypeOf(&redpanda.AddressAlreadyInUseFailure{}))
			Expect(redpanda.RedpandaFailures[1]).To(BeAssignableToTypeOf(&redpanda.ReactorStalledFailure{}))
		})
	})
})
