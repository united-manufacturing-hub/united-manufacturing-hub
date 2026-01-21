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

// TODO: doesnt this make sense to move the metric nbames into speratep ackages? so specify communicatorm etrics in communicator package, etc.?

// Package metrics provides typed metric name constants for FSMv2 workers.
//
// This package ensures compile-time safety for metric names. Using typed
// constants prevents typos that would otherwise silently create incorrect
// metrics (e.g., "mesages_pulled" instead of "messages_pulled").
//
// # Usage
//
// Actions record metrics using typed constants:
//
//	deps.Metrics().IncrementCounter(metrics.CounterMessagesPulled, 5)
//	deps.Metrics().SetGauge(metrics.GaugeLastPullLatencyMs, 42.0)
//
// Typos become compile errors:
//
//	deps.Metrics().IncrementCounter(metrics.CounterMessagessPulled, 1) // COMPILE ERROR
package metrics

// CounterName is a type-safe metric name for counters.
// Counters are cumulative values that only increase.
// The supervisor computes deltas for Prometheus export.
type CounterName string

// GaugeName is a type-safe metric name for gauges.
// Gauges are point-in-time values that can go up or down.
// These are exported directly to Prometheus.
type GaugeName string

// Generic worker counter names.
// These can be used by any worker type for common operations.
const (
	// CounterStateTransitions tracks total state transitions.
	CounterStateTransitions CounterName = "state_transitions"

	// CounterActionExecutions tracks total action executions.
	CounterActionExecutions CounterName = "action_executions"

	// CounterActionErrors tracks total action execution errors.
	CounterActionErrors CounterName = "action_errors"
)

// Generic worker gauge names.
// These can be used by any worker type for common monitoring.
const (
	// GaugeTimeInCurrentStateMs tracks time spent in the current state in milliseconds.
	// NOTE: Also available via FrameworkMetrics.TimeInCurrentStateMs (supervisor-injected).
	GaugeTimeInCurrentStateMs GaugeName = "time_in_current_state_ms"
)

// Framework metric names (supervisor-provided).
// These are automatically injected into FrameworkMetrics by the supervisor.
// Workers access them via snap.Observed.GetFrameworkMetrics() in State.Next().
//
// NOTE: These constants are for Prometheus export consistency. The actual values
// are available via FrameworkMetrics struct fields (not map-based metrics).
const (
	// GaugeStateEnteredAtUnix tracks when the current state was entered (Unix timestamp).
	GaugeStateEnteredAtUnix GaugeName = "state_entered_at_unix"

	// CounterStateTransitionsTotal tracks total state transitions this session.
	// Different from CounterStateTransitions which is the per-tick counter.
	CounterStateTransitionsTotal CounterName = "state_transitions_total"

	// CounterCollectorRestarts tracks per-worker collector restart count.
	CounterCollectorRestarts CounterName = "collector_restarts"

	// CounterStartupCount tracks how many times this worker has been started (persistent).
	// This value survives restarts - loaded from CSE and incremented on each AddWorker().
	CounterStartupCount CounterName = "startup_count"
)

// Communicator worker counter names.
// These track cumulative operations for the bidirectional sync.
const (
	// CounterPullOps tracks total pull operations attempted.
	CounterPullOps CounterName = "pull_ops"

	// CounterPullSuccess tracks successful pull operations.
	CounterPullSuccess CounterName = "pull_success"

	// CounterPullFailures tracks failed pull operations.
	CounterPullFailures CounterName = "pull_failures"

	// CounterMessagesPulled tracks total messages pulled from backend.
	CounterMessagesPulled CounterName = "messages_pulled"

	// CounterPushOps tracks total push operations attempted.
	CounterPushOps CounterName = "push_ops"

	// CounterPushSuccess tracks successful push operations.
	CounterPushSuccess CounterName = "push_success"

	// CounterPushFailures tracks failed push operations.
	CounterPushFailures CounterName = "push_failures"

	// CounterMessagesPushed tracks total messages pushed to backend.
	CounterMessagesPushed CounterName = "messages_pushed"

	// CounterBytesPulled tracks total bytes pulled from backend.
	CounterBytesPulled CounterName = "bytes_pulled"

	// CounterBytesPushed tracks total bytes pushed to backend.
	CounterBytesPushed CounterName = "bytes_pushed"
)

// Error type counter names.
// These track errors by classification for intelligent backoff and monitoring.
const (
	// CounterAuthFailuresTotal tracks authentication failures (401/403).
	CounterAuthFailuresTotal CounterName = "auth_failures_total"

	// CounterServerErrorsTotal tracks server errors (5xx).
	CounterServerErrorsTotal CounterName = "server_errors_total"

	// CounterNetworkErrorsTotal tracks network/connection errors.
	CounterNetworkErrorsTotal CounterName = "network_errors_total"

	// CounterCloudflareErrorsTotal tracks Cloudflare challenge responses (429 + HTML).
	CounterCloudflareErrorsTotal CounterName = "cloudflare_errors_total"

	// CounterProxyBlockErrorsTotal tracks proxy block responses (Zscaler, etc.).
	CounterProxyBlockErrorsTotal CounterName = "proxy_block_errors_total"

	// CounterBackendRateLimitErrorsTotal tracks backend rate limit errors (429 + JSON).
	CounterBackendRateLimitErrorsTotal CounterName = "backend_rate_limit_errors_total"

	// CounterInstanceDeletedTotal tracks instance deleted errors (404).
	CounterInstanceDeletedTotal CounterName = "instance_deleted_total"
)

// Communicator worker gauge names.
// These track point-in-time values for monitoring.
const (
	// GaugeLastPullLatencyMs tracks the latency of the most recent pull operation in milliseconds.
	GaugeLastPullLatencyMs GaugeName = "last_pull_latency_ms"

	// GaugeLastPushLatencyMs tracks the latency of the most recent push operation in milliseconds.
	GaugeLastPushLatencyMs GaugeName = "last_push_latency_ms"

	// GaugeConsecutiveErrors tracks the number of consecutive errors.
	// Resets to 0 on successful operation.
	GaugeConsecutiveErrors GaugeName = "consecutive_errors"

	// GaugeQueueDepth tracks the depth of the outbound message queue.
	GaugeQueueDepth GaugeName = "queue_depth"
)
