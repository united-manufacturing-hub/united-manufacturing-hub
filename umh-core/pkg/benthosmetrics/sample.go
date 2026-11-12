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

// What one scrape returns to the caller.

package benthosmetrics

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/diagnosis"
)

// Probes is what the probe endpoints /ping, /ready and /version answered.
// Scrape returns it whether or not the scrape failed.
type Probes struct {
	// At is when the scrape began.
	At time.Time

	// PingAlive is whether /ping answered pong.
	PingAlive bool

	// Ready is whether /ready reported every connection up.
	Ready bool

	// Version is the version string /version reported, or empty when
	// /version failed.
	Version string
}

// Sample is the measurement of one successful scrape: the parsed /metrics and
// the message throughput computed over throughputWindow (60 s).
type Sample struct {
	// At is when /metrics was read.
	At time.Time

	// Metrics is the parsed /metrics, kept whole: consumers read the
	// connection counters off it.
	Metrics Metrics

	// InputMessagesPerSecond and OutputMessagesPerSecond are Unknown until
	// the window holds two scrapes of the same port.
	InputMessagesPerSecond  diagnosis.Reading
	OutputMessagesPerSecond diagnosis.Reading

	// IsActive is whether InputMessagesPerSecond is known and above 0.
	// Output traffic does not count.
	IsActive bool
}
