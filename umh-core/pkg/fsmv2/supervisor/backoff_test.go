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
			backoff.RecordFailure()
			delay := backoff.NextDelay()
			Expect(delay).To(Equal(2 * time.Second))
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

	Describe("RecordFailure", func() {
		It("increments attempts counter", func() {
			backoff := supervisor.NewExponentialBackoff(1*time.Second, 60*time.Second)
			backoff.RecordFailure()
			backoff.RecordFailure()
			Expect(backoff.Attempts()).To(Equal(2))
		})

		It("records timestamp of last attempt", func() {
			backoff := supervisor.NewExponentialBackoff(1*time.Second, 60*time.Second)
			before := time.Now()
			backoff.RecordFailure()
			after := time.Now()
			Expect(backoff.LastAttempt()).To(BeTemporally(">=", before))
			Expect(backoff.LastAttempt()).To(BeTemporally("<=", after))
		})
	})

	Describe("Reset", func() {
		It("resets attempts to zero", func() {
			backoff := supervisor.NewExponentialBackoff(1*time.Second, 60*time.Second)
			backoff.RecordFailure()
			backoff.RecordFailure()
			backoff.Reset()
			Expect(backoff.Attempts()).To(Equal(0))
		})

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
