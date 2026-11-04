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
// instance, parses the counters, computes the message rates, and reports
// recent errors, blocked outputs, and inputs or outputs that are not
// connected.
//
// Build one Tracker per Benthos instance with NewTracker, call
// Scrape(ctx, port) every ScrapeInterval from a single goroutine, and read
// the rates, metrics and failures off the returned Sample.
//
// fsmv2/diagnosis computes the time windows; this package says what to
// measure and over how long. Metrics is the parsed counters, Sample is the
// result of one scrape, Failures is what is wrong as of that scrape, and
// Tracker is what is kept about one Benthos instance between scrapes.
//
// FSMv1's pkg/service/benthos_monitor keeps its own copy of the types and
// the parser until FSMv1 is removed; this package is the one new code
// uses. The package imports no
// state machine and has no global state; the caller owns each Tracker.
package benthosmetrics
