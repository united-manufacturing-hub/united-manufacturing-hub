// Copyright 2025 UMH Systems GmbH
package supervisor

import (
	"math"
	"time"
)

type ExponentialBackoff struct {
	baseDelay   time.Duration
	maxDelay    time.Duration
	attempts    int
	lastAttempt time.Time
}

func NewExponentialBackoff(baseDelay, maxDelay time.Duration) *ExponentialBackoff {
	return &ExponentialBackoff{
		baseDelay: baseDelay,
		maxDelay:  maxDelay,
	}
}

func (b *ExponentialBackoff) NextDelay() time.Duration {
	if b.attempts == 0 {
		return b.baseDelay
	}

	delay := time.Duration(math.Pow(2, float64(b.attempts))) * b.baseDelay

	if delay > b.maxDelay {
		return b.maxDelay
	}

	return delay
}

func (b *ExponentialBackoff) RecordFailure() {
	b.attempts++
	b.lastAttempt = time.Now()
}

func (b *ExponentialBackoff) Reset() {
	b.attempts = 0
	b.lastAttempt = time.Time{}
}

func (b *ExponentialBackoff) Attempts() int {
	return b.attempts
}

func (b *ExponentialBackoff) LastAttempt() time.Time {
	return b.lastAttempt
}
