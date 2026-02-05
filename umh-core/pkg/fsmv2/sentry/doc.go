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
// which inject "feature" and "error" fields that this hook extracts.
//
// # How Errors Appear in Sentry
//
//   - Title: event_name (e.g., "action_failed")
//   - Subtitle: error message
//   - Tags: feature, event_name, error_types, fsm_version, worker_type
//
// # Creating Sentry Alerts
//
//	level:error feature:communicator
//	level:error feature:fsmv2 event_name:action_failed
package sentry
