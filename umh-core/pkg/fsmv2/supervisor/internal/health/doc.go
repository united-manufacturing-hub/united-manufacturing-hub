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
// reliable decisions. It implements a 4-layer defense-in-depth strategy for
// detecting and handling stale data.
//
// # Why Freshness Detection?
//
// Observed state freshness is critical because:
//
// 1. Stale Data = Wrong Decisions: The FSM makes decisions based on observed
// state. If that state is from 5 minutes ago, the decision may be wrong.
// Example: Observed says "process running" but it crashed 4 minutes ago.
//
// 2. Collection Failure Detection: If the collector goroutine crashes or hangs,
// timestamps stop advancing. Freshness checks detect this.
//
// 3. Network Partition Awareness: If the monitored system becomes unreachable,
// collection will fail. Freshness checks detect the growing staleness.
//
// # Why Two Thresholds (Stale vs Timeout)?
//
// The FreshnessChecker uses two separate thresholds:
//
// Stale Threshold (default: 5s):
//   - Data is considered "stale" but still usable
//   - Triggers warning logs for debugging
//   - May trigger degraded state transitions
//   - Allows for temporary network blips without overreacting
//
// Timeout Threshold (default: 30s):
//   - Data is considered "critically old" and unreliable
//   - Triggers collector restart
//   - May trigger shutdown escalation
//   - Indicates serious collection failure
//
// Why two thresholds? Graceful degradation:
//   - Brief staleness (5-30s): Log warning, maybe degrade, but keep running
//   - Extended staleness (>30s): Take corrective action (restart collector)
//
// # 4-Layer Defense-in-Depth Strategy
//
// Layer 1: Timestamp Detection
//   - Every ObservedState includes GetTimestamp()
//   - Check() computes age = time.Since(timestamp)
//   - If age > staleThreshold: data is stale
//
// Layer 2: State Transition
//   - Supervisor uses Check() result in state logic
//   - Stale data may trigger transition to degraded state
//   - Worker can implement custom staleness handling
//
// Layer 3: Collector Restart
//   - IsTimeout() detects critically old data (>30s)
//   - Supervisor restarts the collector with backoff
//   - Collector restart clears hung goroutines
//
// Layer 4: Shutdown Escalation
//   - If restart doesn't fix staleness, escalate to shutdown
//   - Supervisor signals shutdown to the worker
//   - Worker transitions through stopping states
//   - Last resort: full worker restart
//
// # Why These Default Thresholds?
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
//   - Gives operators ~30s to notice and investigate
//
// # Threshold Tuning Rationale
//
// For high-frequency workers (e.g., real-time control):
//   - Lower stale threshold (1-2s)
//   - Lower timeout (10s)
//   - Faster reaction to failures
//
// For low-frequency workers (e.g., batch processing):
//   - Higher stale threshold (30s-60s)
//   - Higher timeout (120s)
//   - Tolerates slower collection
//
// For flaky networks:
//   - Higher stale threshold (10-15s)
//   - Same timeout (30s)
//   - Tolerates brief partitions without false alarms
//
// # Usage
//
//	checker := health.NewFreshnessChecker(
//	    5 * time.Second,   // staleThreshold
//	    30 * time.Second,  // timeout
//	    logger,
//	)
//
//	// Check if data is fresh
//	if !checker.Check(snapshot) {
//	    // Data is stale, consider degraded transition
//	}
//
//	// Check if data has timed out
//	if checker.IsTimeout(snapshot) {
//	    // Critical staleness, restart collector
//	}
//
// # Integration with Collection
//
// The health package works with the collection package:
//
//	// Collector stores timestamp with each collection
//	observed := &MyObservedState{
//	    Data:        actualSystemState,
//	    CollectedAt: time.Now(),  // Timestamp for freshness
//	}
//
//	// Supervisor tick loop
//	if !freshnessChecker.Check(snapshot) {
//	    if freshnessChecker.IsTimeout(snapshot) {
//	        collector.Restart()
//	    }
//	    // Consider transitioning to degraded
//	}
//
// See supervisor/collection/doc.go for collection implementation details.
package health
