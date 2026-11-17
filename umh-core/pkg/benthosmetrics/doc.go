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

// Package benthosmetrics scrapes the metrics endpoints of one Benthos
// instance, parses the counters, and computes the message rates.
//
// Build one Tracker per Benthos instance with NewTracker, call
// Scrape(ctx, port) every ScrapeInterval from a single goroutine, and read
// the probe answers off the returned Probes and the metrics and message
// throughput off the returned Sample.
//
// FSMv1's pkg/service/benthos_monitor keeps its own copy of the types and
// the parser until FSMv1 is removed; this package is the one new code
// uses. The package imports no state machine and has no global state; the
// caller owns each Tracker.
package benthosmetrics
