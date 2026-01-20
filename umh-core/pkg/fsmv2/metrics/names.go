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
