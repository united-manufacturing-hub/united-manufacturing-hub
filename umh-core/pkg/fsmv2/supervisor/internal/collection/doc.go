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

// Package collection provides the Collector for FSMv2 observed state collection.
//
// # Overview
//
// The Collector manages asynchronous observed state collection with:
//   - Background goroutine for non-blocking collection
//   - Per-collection timeout with context cancellation
//   - Freshness timestamps for staleness detection
//   - Automatic restart with backoff on repeated failures
//
// # Separate goroutine
//
// Observed state collection runs in a separate goroutine for three reasons:
//
// 1. Non-blocking tick loop: The supervisor's tick loop uses the most recently
// collected state without waiting for collection to complete, keeping the
// tick loop fast (<100ms) even if collection is slow.
//
// 2. Decoupled timing: Collection interval (how often the system is polled) is
// independent of tick interval (how often reconciliation runs). Collection might run
// every 100ms while ticks run every second.
//
// 3. Isolation: Collection failures don't block state transitions. The supervisor
// continues operating with stale data (detected via freshness checks) rather
// than halting.
//
// # Freshness timestamps
//
// Every ObservedState includes a timestamp (GetTimestamp()) for three reasons:
//
// 1. Staleness detection: The supervisor's health checker uses timestamps to
// detect when collection has stopped working. If observed state is too old,
// the worker transitions to a degraded state.
//
// 2. Debugging: Operators can see when state was last collected, helping
// diagnose issues like network partitions or hung collectors.
//
// 3. Ordering: If multiple collection results arrive out of order,
// timestamps determine which one is most recent.
//
// # Timeouts
//
// Collection has configurable timeouts (default: 2.2s) for three reasons:
//
// 1. Resource reclamation: A hung collection (e.g., network timeout) is
// cancelled and the goroutine freed. Without timeouts, collectors could
// accumulate indefinitely.
//
// 2. Predictable latency: Operators know the maximum age of observed state
// is bounded by the timeout, which helps SLA calculations.
//
// 3. Cgroup awareness: The default timeout accounts for Docker/Kubernetes
// CPU throttling:
//   - 1s collection interval
//   - 200ms cgroup throttle buffer (100ms period x 2)
//   - 1s safety margin
//     = 2.2s total timeout
//
// # Automatic restart with backoff
//
// The collector restarts automatically with exponential backoff for three reasons:
//
// 1. Transient failures: Network blips, temporary resource exhaustion, or
// brief service outages self-heal without operator intervention.
//
// 2. Thundering herd prevention: If all collectors fail simultaneously (e.g.,
// during a network partition), backoff prevents them from all retrying at once.
//
// 3. Resource protection: Repeated immediate restarts could exhaust resources.
// Backoff gives the system time to recover.
//
// # Thread safety model
//
// The Collector uses a mutex to protect its internal state:
//
//   - Start() and Stop() acquire exclusive locks
//   - GetLatestState() acquires a read lock
//   - The collection goroutine acquires exclusive locks when updating state
//
// The supervisor can call GetLatestState() from the tick loop while
// collection runs in the background.
//
// # Usage
//
//	collector := collection.NewCollector(
//	    worker,
//	    workerID,
//	    store,
//	    logger,
//	    collection.WithInterval(100 * time.Millisecond),
//	    collection.WithTimeout(2 * time.Second),
//	)
//
//	collector.Start(ctx)
//
//	// Get latest state (non-blocking)
//	observed, err := collector.GetLatestState()
//	if err != nil {
//	    // No state collected yet or collection failed
//	}
//
//	// Check staleness
//	if time.Since(observed.GetTimestamp()) > staleness {
//	    // State is stale, trigger degraded transition
//	}
//
//	// Shutdown
//	collector.Stop()
//
// # Metrics
//
// The Collector exposes Prometheus metrics:
//   - fsmv2_collection_duration_seconds: Histogram of collection times
//   - fsmv2_collection_errors_total: Count of collection errors
//   - fsmv2_collection_success_total: Count of successful collections
//   - fsmv2_observed_state_age_seconds: Current age of observed state
//
// # Integration with health
//
// The health package uses collector timestamps to detect staleness:
//
//   - Layer 1: Timestamp too old -> mark data as stale
//   - Layer 2: Stale for too long -> transition to degraded
//   - Layer 3: Degraded for too long -> escalate to stopping
//   - Layer 4: If all else fails -> trigger restart
//
// See supervisor/health/doc.go for the full freshness detection strategy.
//
// # Action-triggered observation
//
// The collector runs observations in two scenarios:
//
// 1. Periodic: Every ObservationInterval (default ~1 second) for regular monitoring.
//    This ensures the FSM always has relatively fresh state even when idle.
//
// 2. On-demand: Immediately after an action completes. When a state returns an
//    action, the FSM waits for fresh observation before re-evaluating. The action
//    executor calls TriggerNow() when done, which signals the collector to collect
//    immediately instead of waiting for the next periodic interval.
//
// This design ensures the FSM sees fresh state after any action that might modify
// the external world (connecting to devices, writing data, etc.).
//
// # TriggerNow vs Restart
//
// TriggerNow() sends a non-blocking signal to trigger immediate collection.
// Multiple calls coalesce into a single collection (buffered channel, capacity 1).
// This is the normal-operation method called by OnActionComplete.
//
// Restart() is an emergency recovery mechanism that actually stops and restarts
// the collector goroutine. Use this when the collector appears hung (not responding
// to TriggerNow signals). This is a last resort - investigate why the collector hung.
//
// # Action-observation gating
//
// When a state returns an action:
//   1. FSM sets an "action gate" and waits for fresh observation
//   2. Action executes asynchronously in worker pool
//   3. Action completes → OnActionComplete callback → TriggerNow()
//   4. Collector wakes up, collects fresh observation
//   5. Fresh observation unblocks the gate, FSM re-evaluates state
//
// This guarantees the FSM always has post-action state before deciding next transition.
package collection
