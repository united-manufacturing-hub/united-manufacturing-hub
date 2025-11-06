// Copyright 2025 UMH Systems GmbH
package supervisor_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

var _ = Describe("ExponentialBackoff", func() {
	Describe("NextDelay", func() {
		It("returns base delay on first attempt", func() {
			backoff := supervisor.NewExponentialBackoff(1*time.Second, 60*time.Second)
			delay := backoff.NextDelay()
			Expect(delay).To(Equal(1 * time.Second))
		})

		It("doubles delay on each attempt", func() {
			backoff := supervisor.NewExponentialBackoff(1*time.Second, 60*time.Second)

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
			backoff := supervisor.NewExponentialBackoff(1*time.Second, 5*time.Second)
			for i := 0; i < 10; i++ {
				backoff.RecordFailure()
			}
			delay := backoff.NextDelay()
			Expect(delay).To(Equal(5 * time.Second))
		})
	})


	Describe("Reset", func() {
		It("resets delay to base", func() {
			backoff := supervisor.NewExponentialBackoff(1*time.Second, 60*time.Second)
			backoff.RecordFailure()
			backoff.RecordFailure()
			backoff.Reset()
			delay := backoff.NextDelay()
			Expect(delay).To(Equal(1 * time.Second))
		})
	})
})
