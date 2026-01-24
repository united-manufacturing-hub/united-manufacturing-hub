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

// Package backoff provides exponential backoff calculation for communicator retry delays.
package backoff

import (
	"math"
	"time"

	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
)

const (
	MaxDelay                = 60 * time.Second
	BaseDelay               = time.Second
	TransportResetThreshold = 5 // Consecutive errors before calling Transport.Reset()
)


// CalculateDelay returns 2^consecutiveErrors seconds, capped at MaxDelay.
func CalculateDelay(consecutiveErrors int) time.Duration {
	if consecutiveErrors <= 0 {
		return 0
	}

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

// CalculateDelayForErrorType returns backoff based on error type. Retry-After header overrides default strategy.
func CalculateDelayForErrorType(errType httpTransport.ErrorType, consecutiveErrors int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		return retryAfter
	}

	return calculateDefaultDelay(errType, consecutiveErrors)
}

func calculateDefaultDelay(errType httpTransport.ErrorType, consecutiveErrors int) time.Duration {
	switch errType {
	case httpTransport.ErrorTypeCloudflareChallenge, httpTransport.ErrorTypeProxyBlock:
		return 60 * time.Second

	case httpTransport.ErrorTypeInvalidToken:
		return 60 * time.Second

	case httpTransport.ErrorTypeBackendRateLimit:
		delays := []time.Duration{30 * time.Second, 60 * time.Second, 120 * time.Second, 300 * time.Second}
		idx := consecutiveErrors - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= len(delays) {
			idx = len(delays) - 1
		}
		return delays[idx]

	case httpTransport.ErrorTypeServerError, httpTransport.ErrorTypeNetwork:
		return CalculateDelay(consecutiveErrors)

	case httpTransport.ErrorTypeInstanceDeleted:
		return 0

	default:
		return CalculateDelay(consecutiveErrors)
	}
}

// ShouldStopRetrying always returns false; the communicator never gives up.
func ShouldStopRetrying(_ httpTransport.ErrorType, _ int) bool {
	return false
}

// ShouldResetTransport returns true when connection pool flush may help recover from errors.
// Network errors reset every 5 consecutive errors; server/Cloudflare errors every 10.
func ShouldResetTransport(errType httpTransport.ErrorType, consecutiveErrors int) bool {
	if consecutiveErrors == 0 {
		return false
	}

	switch errType {
	case httpTransport.ErrorTypeNetwork:
		return consecutiveErrors%TransportResetThreshold == 0
	case httpTransport.ErrorTypeServerError, httpTransport.ErrorTypeCloudflareChallenge:
		return consecutiveErrors%(TransportResetThreshold*2) == 0
	default:
		return false
	}
}
