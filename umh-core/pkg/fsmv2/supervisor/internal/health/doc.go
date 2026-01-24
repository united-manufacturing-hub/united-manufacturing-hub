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

// Package health provides freshness detection for FSMv2 observed state.
//
// # Overview
//
// The FreshnessChecker validates that observed state is recent enough to make
// reliable decisions. It implements a 4-layer validation strategy for
// detecting and handling stale data.
//
// # Freshness detection
//
// Observed state freshness matters for three reasons:
//
// 1. Stale data causes wrong decisions. The FSM decides based on observed
// state. Five-minute-old state claiming "process running" may be wrong if
// the process crashed four minutes ago.
//
// 2. Collection failure detection. If the collector goroutine crashes or hangs,
// timestamps stop advancing. Freshness checks detect this.
//
// 3. Network partition awareness. If the monitored system becomes unreachable,
// collection fails and staleness grows.
//
// # Thresholds
//
// The FreshnessChecker uses two thresholds to allow different responses:
//
// Stale threshold (default: 5s):
//   - Data is stale but usable
//   - Triggers warning logs
//   - May trigger degraded state transitions
//   - Tolerates temporary network blips
//
// Timeout threshold (default: 30s):
//   - Data is stale and unreliable
//   - Triggers collector restart
//   - Triggers shutdown escalation
//   - Indicates serious collection failure
//
// Brief staleness (5-30s) logs warnings and may degrade, but keeps running.
// Extended staleness (>30s) triggers corrective action like collector restart.
//
// # Validation layers
//
// Layer 1 - Timestamp detection:
//   - Every ObservedState includes GetTimestamp()
//   - Check() computes age = time.Since(timestamp)
//   - If age > staleThreshold: data is stale
//
// Layer 2 - State transition:
//   - Supervisor uses Check() result in state logic
//   - Stale data triggers transition to degraded state
//   - Workers implement custom staleness handling
//
// Layer 3 - Collector restart:
//   - IsTimeout() detects stale data (>30s)
//   - Supervisor restarts the collector with backoff
//   - Collector restart clears hung goroutines
//
// Layer 4 - Shutdown escalation:
//   - If restart does not fix staleness, escalate to shutdown
//   - Supervisor signals shutdown to the worker
//   - Worker transitions through stopping states
//   - Last resort: full worker restart
//
// # Default threshold rationale
//
// Stale threshold (5s):
//   - Collection interval is typically 100ms-1s
//   - 5s = 5-50 missed collections before considered stale
//   - Allows for brief network blips or resource contention
//   - Low enough to detect real problems quickly
//
// Timeout threshold (30s):
//   - Long enough for transient issues to resolve
//   - Short enough to detect stuck collectors
//   - Aligned with typical container health check intervals
//
// # Threshold tuning
//
// High-frequency workers (real-time control):
//   - Lower stale threshold (1-2s)
//   - Lower timeout (10s)
//   - Faster reaction to failures
//
// Low-frequency workers (batch processing):
//   - Higher stale threshold (30s-60s)
//   - Higher timeout (120s)
//   - Tolerates slower collection
//
// Flaky networks:
//   - Higher stale threshold (10-15s)
//   - Same timeout (30s)
//   - Tolerates brief partitions
//
// # Usage
//
//	checker := health.NewFreshnessChecker(
//	    5 * time.Second,   // staleThreshold
//	    30 * time.Second,  // timeout
//	    logger,
//	)
//
//	if !checker.Check(snapshot) {
//	    // Data is stale, handle degraded transition
//	}
//
//	if checker.IsTimeout(snapshot) {
//	    // Staleness timeout, restart collector
//	}
//
// # Integration with collection
//
// The health package works with the collection package:
//
//	// Collector stores timestamp with each collection
//	observed := &MyObservedState{
//	    Data:        actualSystemState,
//	    CollectedAt: time.Now(),
//	}
//
//	// Supervisor tick loop
//	if !freshnessChecker.Check(snapshot) {
//	    if freshnessChecker.IsTimeout(snapshot) {
//	        collector.Restart()
//	    }
//	    // Transition to degraded
//	}
//
// See supervisor/collection/doc.go for collection details.
package health
