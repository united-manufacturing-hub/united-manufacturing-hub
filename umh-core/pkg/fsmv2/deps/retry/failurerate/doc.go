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

// Package failurerate tracks the failure rate of the last N outcomes using a
// fixed-size circular buffer. The caller records each outcome as success or
// failure; the [Tracker] computes the rolling failure rate and fires a one-shot
// escalation when the rate crosses a configurable threshold.
//
// # Why This Exists
//
// Transient transport errors (network timeouts, HTTP 5xx, rate limits) should
// not fire SentryError immediately. They self-resolve. But if they persist
// continuously at a high rate, something is genuinely wrong (server issue,
// broken network path) and needs attention. This package provides the filter:
// it tracks the rolling failure rate and signals when transient errors cross
// from "expected noise" to "sustained problem."
//
// [Tracker.RecordOutcome] returns true exactly once when the failure rate
// first crosses the threshold. Callers (push/pull dependencies) use this
// signal to fire a one-shot SentryWarn.
//
// Persistent errors (invalid tokens, deleted instances, proxy blocks) are not
// tracked here. They fire SentryError immediately and always require human
// intervention.
//
// # Design
//
// The Tracker uses a fixed-size circular buffer over the last N outcomes.
// If push fails for 5 minutes, succeeds once, then fails for another
// 5 minutes, the window still sees ~99% failure and keeps the alert. One
// brief success doesn't reset anything.
//
// The window size is outcome-count-based, not time-based. At a 100 ms tick
// rate, WindowSize=6000 covers roughly 10 minutes. Under backoff the
// effective duration stretches because fewer outcomes are recorded per unit
// of time.
//
// # Transient and Persistent Errors
//
// Transport errors fall into two categories:
//
//   - Transient errors self-resolve without human intervention: network
//     timeouts, DNS failures, HTTP 5xx responses, rate limits, and full
//     channels.
//
//   - Persistent errors require human intervention: invalid tokens, deleted
//     instances, proxy blocks, and Cloudflare challenges.
//
// All error types (transient and persistent) feed the rolling window.
// Persistent errors also fire SentryError immediately. The suppression
// logic suppresses SentryError for transient errors while still updating
// metrics and DegradedState.
//
// # Escalation Lifecycle
//
// When the failure rate exceeds the configured threshold over the rolling
// window, the Tracker fires a one-shot escalation.
// The caller fires a SentryWarn to alert operators. After enough successes
// bring the rate below the threshold, the Tracker rearms and can fire again
// on the next crossing.
//
// # Why a Rolling Window
//
// A timer-based approach (escalate when degraded longer than N minutes) has a
// blind spot: a single success in a 99% failure pattern resets the timer
// completely (ENG-4565). The rolling window tracks rate, not duration. A single
// success shifts the window from 100/100 failures to 99/100 — the rate barely
// changes, and escalation holds.
//
// # Integration
//
// Error classification (ErrorType, IsTransient) lives in the
// communicator/transport/http package. Rate tracking lives in this package.
// Push and pull dependencies each hold a *[Tracker] and call
// [Tracker.RecordOutcome] after every real HTTP operation (success or
// failure). Idle ticks — where no HTTP request was made — must NOT record
// an outcome, as this would dilute the failure rate with phantom data.
package failurerate
