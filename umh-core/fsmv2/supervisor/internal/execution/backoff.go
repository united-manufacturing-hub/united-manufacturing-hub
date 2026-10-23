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

package execution

import (
	"math"
	"time"
)

type ExponentialBackoff struct {
	startTime time.Time
	baseDelay time.Duration
	maxDelay  time.Duration
	attempts  int
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

func (b *ExponentialBackoff) GetStartTime() time.Time {
	return b.startTime
}
