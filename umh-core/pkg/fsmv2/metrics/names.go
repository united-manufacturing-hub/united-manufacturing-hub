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

// TEMPORARY RE-EXPORTS - Remove in Phase 3
// This file re-exports metric types and constants from deps/ for backward compatibility.
package metrics

import "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"

// Type aliases work fine.
type CounterName = deps.CounterName
type GaugeName = deps.GaugeName

// IMPORTANT: Go does NOT allow `const X = pkg.X`.
// Must use var for re-exports of constants.
var (
	// Generic worker counters.
	CounterStateTransitions = deps.CounterStateTransitions
	CounterActionExecutions = deps.CounterActionExecutions
	CounterActionErrors     = deps.CounterActionErrors

	// Generic worker gauges.
	GaugeTimeInCurrentStateMs = deps.GaugeTimeInCurrentStateMs

	// Framework metrics.
	GaugeStateEnteredAtUnix      = deps.GaugeStateEnteredAtUnix
	CounterStateTransitionsTotal = deps.CounterStateTransitionsTotal
	CounterCollectorRestarts     = deps.CounterCollectorRestarts
	CounterStartupCount          = deps.CounterStartupCount

	// Communicator counters.
	CounterPullOps        = deps.CounterPullOps
	CounterPullSuccess    = deps.CounterPullSuccess
	CounterPullFailures   = deps.CounterPullFailures
	CounterMessagesPulled = deps.CounterMessagesPulled
	CounterPushOps        = deps.CounterPushOps
	CounterPushSuccess    = deps.CounterPushSuccess
	CounterPushFailures   = deps.CounterPushFailures
	CounterMessagesPushed = deps.CounterMessagesPushed
	CounterBytesPulled    = deps.CounterBytesPulled
	CounterBytesPushed    = deps.CounterBytesPushed

	// Error counters.
	CounterAuthFailuresTotal           = deps.CounterAuthFailuresTotal
	CounterServerErrorsTotal           = deps.CounterServerErrorsTotal
	CounterNetworkErrorsTotal          = deps.CounterNetworkErrorsTotal
	CounterCloudflareErrorsTotal       = deps.CounterCloudflareErrorsTotal
	CounterProxyBlockErrorsTotal       = deps.CounterProxyBlockErrorsTotal
	CounterBackendRateLimitErrorsTotal = deps.CounterBackendRateLimitErrorsTotal
	CounterInstanceDeletedTotal        = deps.CounterInstanceDeletedTotal

	// Communicator gauges.
	GaugeLastPullLatencyMs = deps.GaugeLastPullLatencyMs
	GaugeLastPushLatencyMs = deps.GaugeLastPushLatencyMs
	GaugeConsecutiveErrors = deps.GaugeConsecutiveErrors
	GaugeQueueDepth        = deps.GaugeQueueDepth
)
