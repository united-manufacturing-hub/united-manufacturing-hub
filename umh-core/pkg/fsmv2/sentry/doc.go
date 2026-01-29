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
// # Quick Start
//
// Log errors using ErrorFields:
//
//	logger.Errorw("action_failed", sentry.ErrorFields{
//	    Feature:       "communicator",  // Required: your component
//	    Err:           err,             // Pass error, not err.Error()!
//	    HierarchyPath: "app(umh-core)/worker(communicator)",
//	}.ZapFields()...)
//
// # Feature Field
//
// Identifies which component owns the error. Common values:
//   - "fsmv2" - FSMv2 framework
//   - "communicator" - Backend communication
//   - "benthos" - Benthos process management
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
//
// # Best Practices
//
//   - Always set Feature
//   - Pass err (error), never err.Error() (string)
//   - Include HierarchyPath for auto-tagging
package sentry
