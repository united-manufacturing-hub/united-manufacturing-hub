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

package retry

import "time"

// ErrorInfo contains details about the last recorded error.
type ErrorInfo struct {
	OccurredAt time.Time
	Class      string        // Protocol-specific: "network", "rate_limit", "crc_mismatch", etc.
	RetryAfter time.Duration // Server-suggested retry delay (from Retry-After header)
}

// Recorder handles recording errors and successes.
// Called by Actions during execution.
type Recorder interface {
	// RecordError records an error occurrence with optional classification.
	RecordError(opts ...ErrorOption)

	// RecordSuccess resets all error state (counter, timestamps, classification).
	RecordSuccess()

	// Attempt increments the retry counter without recording a new error.
	// Used by recovery actions (like ResetTransport) to advance past modulo-N triggers.
	Attempt()
}

// State provides read access to retry state.
// Called by Collector during CollectObservedState.
type State interface {
	// ConsecutiveErrors returns the current error count.
	ConsecutiveErrors() int

	// DegradedSince returns when degraded mode started, or (zero, false) if not degraded.
	DegradedSince() (time.Time, bool)

	// LastError returns details about the most recent error.
	LastError() ErrorInfo
}

// Policy determines retry timing decisions.
// Called by States during Next() evaluation.
type Policy interface {
	// ShouldReset returns true if counter is at a reset threshold.
	ShouldReset(threshold int) bool

	// BackoffElapsed returns true if enough time has passed since degraded entry.
	BackoffElapsed(delay time.Duration) bool
}

// Tracker combines all retry-related interfaces.
// Most workers will use this composed interface.
type Tracker interface {
	Recorder
	State
	Policy
}
