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

// Package sentry provides Sentry integration for fsmv2 error reporting.
//
// The SentryHook wraps a zapcore.Core and intercepts warn/error-level log
// entries, forwarding them to Sentry with fingerprinting and debouncing.
//
// Production code uses [deps.FSMLogger] methods (SentryWarn, SentryError)
// which inject "feature", "hierarchy_path", and "error" fields that this hook extracts.
//
// # How Errors Appear in Sentry
//
//   - Title: event_name (e.g., "action_failed")
//   - Subtitle: error message
//   - Tags: feature, event_name, error_types, fsm_version, worker_type, worker_chain, action_name
//   - Contexts["umh_context"]: all remaining fields (reason, duration_ms, capacity, etc.)
//
// # Contexts (umh_context)
//
// All fields not extracted as tags are sent to event.Contexts["umh_context"].
// This includes troubleshooting data like reason, duration_ms, capacity,
// target_worker_id, child_name, and panic_value. These are visible on the
// Sentry issue page but not searchable.
//
// The catch-all excludes:
//   - Keys already handled: feature, stack, panic, action_name, hierarchy_path
//   - The "error" key when a typed error was extracted (avoids duplication with Exception)
//   - Keys matching a sensitive-name denylist (password, secret, token, etc.)
//   - String values are truncated at 1024 characters
//
// # Feature tags
//
// Each worker's Sentry events are tagged with its own type via
// [deps.FeatureForWorker] (e.g., feature:pull, feature:push,
// feature:certfetcher). Supervisor-internal events (tick panics,
// circuit breakers) use feature:fsmv2.
//
// Sentry ownership rules route alerts per worker type
// (Settings > Projects > Ownership Rules):
//
//	tags.feature:certfetcher {certfetcher-owner-email}
//	tags.feature:fsmv2 {fsmv2-owner-email}
//	tags.feature:* {default-owner-email}
//
// # Creating Sentry Alerts
//
//	level:error feature:pull
//	level:error feature:fsmv2 event_name:tick_panic
//	level:error worker_chain:application/communicator
//
// # Tag Design: hierarchy_path vs worker_chain
//
// The raw hierarchy_path (e.g., "app(myapp)/bridge-Factory-PLC-001(protocolconverter)/...")
// is NOT sent as a Sentry tag. Instead, it is decomposed into bounded-cardinality tags:
//
//   - fsm_version: "v2" or "v1" (2 values)
//   - worker_type: leaf segment type, e.g., "pull", "communicator" (~10 values)
//   - worker_chain: type-only path, e.g., "application/communicator/transport/pull" (~10-15 values)
//
// Raw hierarchy paths contain customer-specific data (bridge names, instance IDs)
// that would cause unbounded tag cardinality — each bridge a customer creates adds
// a new unique path. With ~5 bridges per CPU core, a typical deployment would have
// 150-600 unique paths. The worker_chain strips instance IDs and names, keeping only
// the structural type chain, which is bounded to ~10-15 unique values globally.
//
// Tag values are truncated at 200 characters (Sentry's server-side limit) with a
// "..." suffix to make truncation visible. This applies to error_types and
// worker_chain, the only tags with theoretical overflow risk.
//
// # Log-Only vs Sentry Fields
//
// Fields added via FSMLogger.With() (e.g., "worker" identity) are captured by the
// catch-all and appear in Contexts["umh_context"]. High-cardinality identifiers
// like "comm-001(communicator)" are NOT promoted to tags (which would cause
// cardinality issues), but they ARE visible in the Contexts panel on each
// Sentry event for per-incident debugging.
package sentry
