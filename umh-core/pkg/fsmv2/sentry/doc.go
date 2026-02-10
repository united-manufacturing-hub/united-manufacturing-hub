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
//   - Tags: feature, event_name, error_types, fsm_version, worker_type, worker_chain
//
// # Creating Sentry Alerts
//
//	level:error feature:communicator
//	level:error feature:fsmv2 event_name:action_failed
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
// # Log-Only vs Sentry Fields
//
// Fields added via FSMLogger.With() (e.g., "worker" identity) are visible to
// the SentryHook as per-call fields. However, the hook only extracts specific
// keys ("feature", "hierarchy_path", "error", "stack", "panic", "action_name")
// and ignores the rest. High-cardinality identifiers like "comm-001(communicator)"
// appear in the "worker" field which the hook intentionally does not tag, keeping
// Sentry tags bounded. These identifiers remain available in structured log output
// for grep.
package sentry
