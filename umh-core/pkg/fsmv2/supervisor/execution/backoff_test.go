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

package execution_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/execution"
)

var _ = Describe("ExponentialBackoff", func() {
	Describe("NextDelay", func() {
		It("returns base delay on first attempt", func() {
			backoff := execution.NewExponentialBackoff(1*time.Second, 60*time.Second)
			delay := backoff.NextDelay()
			Expect(delay).To(Equal(1 * time.Second))
		})

		It("doubles delay on each attempt", func() {
			backoff := execution.NewExponentialBackoff(1*time.Second, 60*time.Second)

			first := backoff.NextDelay()
			backoff.RecordFailure()

			second := backoff.NextDelay()
			backoff.RecordFailure()

			third := backoff.NextDelay()

			Expect(first).To(Equal(1 * time.Second))
			Expect(second).To(Equal(2 * time.Second))
			Expect(third).To(Equal(4 * time.Second))
		})

		It("caps delay at max", func() {
			backoff := execution.NewExponentialBackoff(1*time.Second, 5*time.Second)
			for i := 0; i < 10; i++ {
				backoff.RecordFailure()
			}
			delay := backoff.NextDelay()
			Expect(delay).To(Equal(5 * time.Second))
		})
	})


	Describe("Reset", func() {
		It("resets delay to base", func() {
			backoff := execution.NewExponentialBackoff(1*time.Second, 60*time.Second)
			backoff.RecordFailure()
			backoff.RecordFailure()
			backoff.Reset()
			delay := backoff.NextDelay()
			Expect(delay).To(Equal(1 * time.Second))
		})
	})
})
