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

package fsmv2benthosmonitor

import "time"

// windowSpan is the width of the throughput window in seconds. Throughput is
// held BY TIME over this span, not by FSMv1's count-based ThroughputWindowSize
// of 600 entries, so a one-minute average does not drift when ticks are dropped.
const windowSpan = 60 * time.Second

// throughputSample is one poll's counter snapshot, stamped with the time it was
// observed. The time is injected (not obtained from a clock inside the window)
// so callers can replay deterministic sample sequences.
type throughputSample struct {
	at    time.Time
	input int
}

// throughputWindow holds a by-time window of counter samples. Add appends a
// sample stamped with its observed time and prunes samples older than windowSpan
// of the newest, so the window stays bounded to the span. The window computes the
// input rate over only the samples inside windowSpan of the newest sample.
type throughputWindow struct {
	samples []throughputSample
}

// Add records a sample at the given observed time with the given input counter
// value, then drops any sample older than windowSpan of the newest. Production
// samples arrive in time order (so the aged prefix is normally all that is
// removed), but the scan also drops an out-of-order aged sample, keeping the
// window bounded to the span regardless of arrival order.
func (w *throughputWindow) Add(at time.Time, input int) {
	w.samples = append(w.samples, throughputSample{at: at, input: input})
	if len(w.samples) < 2 {
		return
	}
	newest := w.newest()
	cutoff := newest.at.Add(-windowSpan)

	kept := w.samples[:0]
	for _, s := range w.samples {
		if !s.at.Before(cutoff) {
			kept = append(kept, s)
		}
	}
	w.samples = kept
}

// newest returns the sample with the latest observed time. Callers guarantee
// the window holds at least one sample.
func (w *throughputWindow) newest() throughputSample {
	newest := w.samples[0]
	for _, s := range w.samples[1:] {
		if s.at.After(newest.at) {
			newest = s
		}
	}
	return newest
}

// messagesPerSecond returns the input rate in messages per second over the
// in-window span: (newest.count - oldest-in-window.count) / elapsedSeconds,
// where elapsedSeconds is the real time between those two samples. With fewer
// than two in-window samples the rate is 0, because a single sample cannot be
// delta-ed.
func (w *throughputWindow) messagesPerSecond() float64 {
	if len(w.samples) < 2 {
		return 0
	}
	newest := w.newest()
	cutoff := newest.at.Add(-windowSpan)

	oldest := newest
	for _, s := range w.samples {
		if !s.at.Before(cutoff) && s.at.Before(oldest.at) {
			oldest = s
		}
	}

	// A counter reset makes the newest sample lower than the oldest in-window
	// sample: benthos' input_received is a Prometheus counter that zeroes when
	// the process restarts, so a reset landing inside the window yields a
	// negative delta. The rate is undefined there, so report 0. The misreport
	// lasts up to the window span (~60s) until the pre-restart samples age out.
	if newest.input < oldest.input {
		return 0
	}

	elapsed := newest.at.Sub(oldest.at).Seconds()
	return float64(newest.input-oldest.input) / elapsed
}

// lastCount returns the newest sample's input counter value. Because Add prunes,
// the newest sample is always in-window.
func (w *throughputWindow) lastCount() int {
	return w.newest().input
}
