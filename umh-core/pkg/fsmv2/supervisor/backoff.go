// Copyright 2025 UMH Systems GmbH
package supervisor

import (
	"math"
	"time"
)

type ExponentialBackoff struct {
	baseDelay time.Duration
	maxDelay  time.Duration
	attempts  int
	startTime time.Time
}

func NewExponentialBackoff(baseDelay, maxDelay time.Duration) *ExponentialBackoff {
	return &ExponentialBackoff{
		baseDelay: baseDelay,
		maxDelay:  maxDelay,
		startTime: time.Now(),
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
	if b.attempts == 0 {
		b.startTime = time.Now()
	}
	b.attempts++
}

func (b *ExponentialBackoff) Reset() {
	b.attempts = 0
	b.startTime = time.Now()
}

func (b *ExponentialBackoff) GetTotalDowntime() time.Duration {
	if b.attempts == 0 {
		return 0
	}
	return time.Since(b.startTime)
}

func (b *ExponentialBackoff) GetAttempts() int {
	return b.attempts
}
