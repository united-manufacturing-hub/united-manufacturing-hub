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
// # Problem
//
// Transport push and pull workers each need to detect sustained failure
// patterns and alert operators via Sentry. Without a shared abstraction,
// each worker would implement its own escalation infrastructure: a mutex,
// a flag, a threshold check, and a timer or counter. This duplication is
// error-prone and makes the escalation behavior hard to review, test, and
// reason about across workers.
//
// The naive approach — escalate when degraded longer than N minutes — has a
// known blind spot (ENG-4565): a single success in a 99% failure pattern
// resets the timer completely. The system never escalates even though it is
// failing 99% of the time.
//
// # Design
//
// The Tracker uses a fixed-size circular buffer to compute a rolling failure
// rate. A single success shifts the window from 100/100 failures to 99/100.
// The rate barely changes, and escalation holds.
//
// A rolling window was chosen over an exponentially weighted moving average
// (EWMA) because the window provides exact, bounded semantics: "90% of the
// last 6000 outcomes failed" is easy to reason about and configure. EWMA
// gives a fuzzy decay where the effective window size depends on the smoothing
// factor, making threshold tuning less intuitive. The circular buffer also
// makes tests fully deterministic (no time dependency, no floating-point
// drift from repeated multiplication).
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
// Persistent errors also fire SentryError immediately. The downstream
// suppression logic (wired in subsequent PRs) suppresses SentryError for
// transient errors while still updating metrics and DegradedState.
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
// Error classification lives in the communicator/transport/http package
// (ErrorType constants). Rate tracking lives in this package. Push and pull
// dependencies each hold a *[Tracker] and call [Tracker.RecordOutcome]
// after every real HTTP operation (success or failure). Idle ticks — where
// no HTTP request was made — must NOT record an outcome, as this would
// dilute the failure rate with phantom data.
package failurerate
