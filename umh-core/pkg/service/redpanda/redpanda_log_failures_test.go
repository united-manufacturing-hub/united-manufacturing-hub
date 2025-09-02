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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda"
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6/s6_default"
)

var _ = Describe("Redpanda Log Failures", func() {
	Describe("AddressAlreadyInUseFailure", func() {
		var failure *redpanda.AddressAlreadyInUseFailure
		var transitionTime time.Time

		BeforeEach(func() {
			failure = &redpanda.AddressAlreadyInUseFailure{}
			transitionTime = time.Now()
		})

		It("should detect address already in use failure", func() {
			log := s6service.LogEntry{
				Timestamp: time.Now(),
				Content:   "Address already in use",
			}
			Expect(failure.IsFailure(log, transitionTime)).To(BeTrue())
		})

		It("should not detect failure in normal log line", func() {
			log := s6service.LogEntry{
				Timestamp: time.Now(),
				Content:   "Normal log line",
			}
			Expect(failure.IsFailure(log, transitionTime)).To(BeFalse())
		})

		It("should be case sensitive", func() {
			log := s6service.LogEntry{
				Timestamp: time.Now(),
				Content:   "address already in use",
			}
			Expect(failure.IsFailure(log, transitionTime)).To(BeFalse())
		})
	})

	Describe("ReactorStalledFailure", func() {
		var failure *redpanda.ReactorStalledFailure
		var transitionTime time.Time

		BeforeEach(func() {
			failure = &redpanda.ReactorStalledFailure{}
			transitionTime = time.Now()
		})

		It("should detect reactor stall over 500ms", func() {
			log := s6service.LogEntry{
				Timestamp: time.Now(),
				Content:   "Reactor stalled for 501 ms",
			}
			Expect(failure.IsFailure(log, transitionTime)).To(BeTrue())
		})

		It("should not detect reactor stall under 500ms", func() {
			log := s6service.LogEntry{
				Timestamp: time.Now(),
				Content:   "Reactor stalled for 499 ms",
			}
			Expect(failure.IsFailure(log, transitionTime)).To(BeFalse())
		})

		It("should not detect failure in normal log line", func() {
			log := s6service.LogEntry{
				Timestamp: time.Now(),
				Content:   "Normal log line",
			}
			Expect(failure.IsFailure(log, transitionTime)).To(BeFalse())
		})

		It("should handle malformed log lines", func() {
			log1 := s6service.LogEntry{
				Timestamp: time.Now(),
				Content:   "Reactor stalled for ms",
			}
			log2 := s6service.LogEntry{
				Timestamp: time.Now(),
				Content:   "Reactor stalled for abc ms",
			}
			Expect(failure.IsFailure(log1, transitionTime)).To(BeFalse())
			Expect(failure.IsFailure(log2, transitionTime)).To(BeFalse())
		})

		It("should be case sensitive", func() {
			log := s6service.LogEntry{
				Timestamp: time.Now(),
				Content:   "reactor stalled for 501 ms",
			}
			Expect(failure.IsFailure(log, transitionTime)).To(BeFalse())
		})

		It("should ignore stalls before transition time", func() {
			transitionTime := time.Now()
			log := s6service.LogEntry{
				Timestamp: transitionTime.Add(-1 * time.Second), // 1 second before transition
				Content:   "Reactor stalled for 501 ms",
			}
			Expect(failure.IsFailure(log, transitionTime)).To(BeFalse())
		})

		It("should detect stalls after transition time", func() {
			transitionTime := time.Now()
			log := s6service.LogEntry{
				Timestamp: transitionTime.Add(1 * time.Second), // 1 second after transition
				Content:   "Reactor stalled for 501 ms",
			}
			Expect(failure.IsFailure(log, transitionTime)).To(BeTrue())
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
