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

package tools

import "time"

type BackoffPolicy int

const (
	BackoffPolicyExponential BackoffPolicy = iota
	BackoffPolicyLinear
)

type Backoff struct {
	lastBackoff time.Duration
	start       time.Duration
	step        time.Duration
	max         time.Duration
	policy      BackoffPolicy
}

func NewBackoff(start, step, max time.Duration, policy BackoffPolicy) *Backoff {
	if start <= 0 {
		start = 1 * time.Millisecond
	}
	return &Backoff{
		lastBackoff: start,
		start:       start,
		step:        step,
		max:         max,
		policy:      policy,
	}
}

// Reset resets the backoff to its initial state.
func (b *Backoff) Reset() {
	b.lastBackoff = b.start
}

// Next returns the next backoff duration.
func (b *Backoff) Next() time.Duration {
	backoff := b.lastBackoff

	if b.policy == BackoffPolicyLinear {
		backoff += b.step
	} else {
		backoff *= b.step
	}

	if backoff > b.max {
		backoff = b.max
	}

	b.lastBackoff = backoff
	return backoff
}

// IncrementAndSleep increments the backoff and sleeps for the duration (in milliseconds).
func (b *Backoff) IncrementAndSleep() {
	backoff := b.Next()
	time.Sleep(backoff)
}

func (b *Backoff) GetCycleTime() time.Duration {
	return b.lastBackoff
}

type NonBlockingBackoff struct {
	backoff     *Backoff  // The underlying backoff
	nextAllowed time.Time // Next timestamp we’re allowed to attempt
}

// NewNonBlockingBackoff initializes a NonBlockingBackoff with your desired parameters.
// Example usage:  tools.NewNonBlockingBackoff(1*time.Second, 2, 1*time.Minute, BackoffPolicyExponential)
// start: The initial backoff duration
// step: The step increment for each failure, for example 2 means double the backoff each time
// max: The maximum backoff duration
// policy: The backoff policy (BackoffPolicyExponential or BackoffPolicyLinear)
func NewNonBlockingBackoff(start, step, max time.Duration, policy BackoffPolicy) *NonBlockingBackoff {
	return &NonBlockingBackoff{
		backoff:     NewBackoff(start, step, max, policy),
		nextAllowed: time.Time{}, // zero means “no restriction yet”
	}
}

// ShouldRunNow returns true if the current time is >= nextAllowed.
func (n *NonBlockingBackoff) ShouldRunNow() bool {
	return time.Now().After(n.nextAllowed)
}

// MarkFailed increments the underlying backoff and updates nextAllowed accordingly.
func (n *NonBlockingBackoff) MarkFailed() {
	delay := n.backoff.Next()
	n.nextAllowed = time.Now().Add(delay)
}

// MarkSucceeded resets the underlying backoff and clears nextAllowed.
func (n *NonBlockingBackoff) MarkSucceeded() {
	n.backoff.Reset()
	n.nextAllowed = time.Time{}
}

// GetBackoffDuration returns the current backoff duration (useful for logging).
func (n *NonBlockingBackoff) GetBackoffDuration() time.Duration {
	return n.backoff.lastBackoff
}
