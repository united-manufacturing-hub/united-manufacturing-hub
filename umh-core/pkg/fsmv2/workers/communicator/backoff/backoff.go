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

	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
)

const (
	// MaxDelay caps the exponential backoff to prevent excessive delays.
	MaxDelay = 60 * time.Second

	// BaseDelay is the starting delay for backoff calculation.
	BaseDelay = time.Second

	// TransportResetThreshold is the number of consecutive errors that triggers a transport reset.
	// When this threshold is reached (and at every multiple of the threshold), the transport's
	// Reset() method is called to flush stale connections and potentially resolve connection-level
	// issues like DNS caching, corrupted TCP state, or stale pooled connections.
	TransportResetThreshold = 5
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

// CalculateDelayForErrorType returns backoff based on error type.
// If retryAfter > 0 (from Retry-After header), it overrides the default strategy.
// This enables intelligent backoff: Cloudflare/proxy blocks get fixed delays,
// rate limits get progressive delays, network errors get exponential backoff.
func CalculateDelayForErrorType(errType httpTransport.ErrorType, consecutiveErrors int, retryAfter time.Duration) time.Duration {
	// Respect server's Retry-After header if provided
	if retryAfter > 0 {
		return retryAfter
	}

	return calculateDefaultDelay(errType, consecutiveErrors)
}

// calculateDefaultDelay returns backoff without Retry-After override.
func calculateDefaultDelay(errType httpTransport.ErrorType, consecutiveErrors int) time.Duration {
	switch errType {
	case httpTransport.ErrorTypeCloudflareChallenge,
		httpTransport.ErrorTypeProxyBlock:
		// Fixed 60s for challenges/blocks - waiting longer doesn't help
		return 60 * time.Second

	case httpTransport.ErrorTypeInvalidToken:
		// Fixed 60s for auth errors - credentials won't magically change
		return 60 * time.Second

	case httpTransport.ErrorTypeBackendRateLimit:
		// Progressive: 30s, 60s, 120s, 300s (5 min max)
		delays := []time.Duration{30 * time.Second, 60 * time.Second, 120 * time.Second, 300 * time.Second}

		idx := consecutiveErrors - 1
		if idx < 0 {
			idx = 0
		}

		if idx >= len(delays) {
			idx = len(delays) - 1
		}

		return delays[idx]

	case httpTransport.ErrorTypeServerError:
		// Exponential backoff: 2s, 4s, 8s, 16s, 32s (server may recover)
		return CalculateDelay(consecutiveErrors)

	case httpTransport.ErrorTypeNetwork:
		// Exponential backoff: 1s, 2s, 4s, 8s, 16s (network may recover)
		return CalculateDelay(consecutiveErrors)

	case httpTransport.ErrorTypeInstanceDeleted:
		// Instance deleted - no point retrying
		return 0

	default:
		// Unknown errors - use exponential backoff
		return CalculateDelay(consecutiveErrors)
	}
}

// ShouldStopRetrying always returns false - the communicator should never stop retrying.
// Only the backoff time changes based on error type via CalculateDelayForErrorType.
// This ensures the communicator remains resilient and keeps attempting to reconnect.
func ShouldStopRetrying(_ httpTransport.ErrorType, _ int) bool {
	// Never stop retrying - only backoff time changes
	return false
}

// ShouldResetTransport determines if the HTTP transport should be reset based on error type.
// Transport reset flushes connection pools, clears DNS cache, and re-establishes TLS.
// This helps recover from connection-level issues that accumulate over time.
//
// Reset patterns:
//   - Network errors: Reset every TransportResetThreshold (5) consecutive errors
//   - Server/Cloudflare errors: Reset every 2*TransportResetThreshold (10) consecutive errors
//   - Other errors: Never reset (auth errors, rate limits won't benefit from transport reset)
//
// Workers use this by checking after each error:
//
//	if backoff.ShouldResetTransport(errType, consecutiveErrors) {
//	    deps.Transport.Reset()
//	}
func ShouldResetTransport(errType httpTransport.ErrorType, consecutiveErrors int) bool {
	if consecutiveErrors == 0 {
		return false
	}

	switch errType {
	case httpTransport.ErrorTypeNetwork:
		// Network errors benefit most from transport reset - reset every 5 errors
		return consecutiveErrors%TransportResetThreshold == 0

	case httpTransport.ErrorTypeServerError, httpTransport.ErrorTypeCloudflareChallenge:
		// Server/Cloudflare errors might benefit, but less frequently - reset every 10 errors
		return consecutiveErrors%(TransportResetThreshold*2) == 0

	default:
		// Auth errors, rate limits, proxy blocks, etc. won't benefit from transport reset
		return false
	}
}
