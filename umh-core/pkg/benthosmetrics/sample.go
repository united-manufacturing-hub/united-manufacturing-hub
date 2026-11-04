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

// Sample, what one scrape returns to the caller.

package benthosmetrics

import "time"

// rate carries the rate for one direction of the Benthos pipeline.
type rate struct {
	// MessagesPerSecond is averaged over rateWindow (60 s).
	MessagesPerSecond float64
}

// rates is the input and output message rates, and whether the Benthos
// instance is active.
type rates struct {
	Input  rate
	Output rate

	// IsActive is Input.MessagesPerSecond > 0. Output traffic does not count.
	IsActive bool
}

// Sample is what one scrape observed: when it began, the parsed metrics,
// the rates, the Failures, and the three probe answers (/ping, /ready,
// /version).
//
// After a failed scrape, At is set (a Tracker rejected by Scrape's guard
// returns a zero Sample, so its At is the zero time); PingAlive and Ready
// say whether /ping and /ready answered; Version is set only when the
// failure was /metrics; Metrics, the rates and the Failures stay zero or
// nil.
type Sample struct {
	// At is when the scrape began. It is set before the first request, so a
	// failed scrape also carries a fresh value.
	At time.Time

	// Metrics is the parsed /metrics, kept whole: consumers read the
	// connection counters off it.
	Metrics Metrics

	// rates is the input and output rates over rateWindow, and whether the
	// Benthos instance is active.
	rates

	// Failures is what is wrong as of this scrape, or nil after a failed
	// scrape.
	Failures *Failures

	// PingAlive is whether /ping answered pong.
	PingAlive bool

	// Ready is whether /ready reported every connection up.
	Ready bool

	// Version is the version string /version reported.
	Version string
}
