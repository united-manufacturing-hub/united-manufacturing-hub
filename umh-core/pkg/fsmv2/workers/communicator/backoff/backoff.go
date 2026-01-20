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

// Package backoff provides exponential backoff calculation utilities for the communicator FSM.
// This package is used by both SyncingState and DegradedState to calculate retry delays.
package backoff

import (
	"math"
	"time"
)

const (
	// MaxDelay caps the exponential backoff to prevent excessive delays.
	MaxDelay = 60 * time.Second

	// BaseDelay is the starting delay for backoff calculation.
	BaseDelay = time.Second
)

// CalculateDelay calculates exponential backoff based on consecutive errors.
// Returns 0 if no errors or negative errors. Caps at MaxDelay (60 seconds).
//
// Formula: 2^consecutiveErrors seconds, capped at 60 seconds.
// This implements a simple circuit breaker pattern: more errors = longer delay.
func CalculateDelay(consecutiveErrors int) time.Duration {
	if consecutiveErrors <= 0 {
		return 0
	}

	// Exponential backoff: 2^errors seconds, capped at 60s
	// Cap early to prevent overflow for large error counts
	// 2^6 = 64 seconds, so anything >= 6 errors would exceed 60s cap
	if consecutiveErrors >= 6 {
		return MaxDelay
	}

	delaySeconds := math.Pow(2, float64(consecutiveErrors))
	delay := time.Duration(delaySeconds) * BaseDelay

	if delay > MaxDelay {
		return MaxDelay
	}

	return delay
}
