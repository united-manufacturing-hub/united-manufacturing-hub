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
	at     time.Time
	input  int
	output int
}

// throughputWindow holds a by-time window of counter samples. Add appends a
// sample stamped with its observed time and prunes samples older than windowSpan
// of the newest, so the window stays bounded to the span. The window computes
// the input and output rates over only the samples inside windowSpan of the
// newest sample. Its zero value is a valid empty window: every method is safe on
// an empty window, so no constructor is needed and a nil window cannot be built.
type throughputWindow struct {
	// port is the scrape port this window's samples belong to. Add full-wipes
	// on a port change (a new endpoint is a new counter series), so the window
	// never delta-ticks across two different benthos monitors. Zero means no
	// sample has been recorded yet.
	port    int
	samples []throughputSample
}

// Add records a sample at the given observed time with the given port and input
// and output counter values, then drops any sample older than windowSpan of the
// newest. The window keys on the scrape port: a different port from the previous
// poll is a new counter series (the child was re-pointed at a different endpoint)
// and full-wipes the window. A restart of the same benthos zeroes the monotonic
// input_received counter, so a drop against the immediately preceding sample
// also full-wipes (D5a's counter-reset detector is the input counter only).
// Production samples arrive in time order (so the aged prefix is normally all
// that is removed), but the scan also drops an out-of-order aged sample, keeping
// the window bounded to the span regardless of arrival order.
func (w *throughputWindow) Add(at time.Time, port, input, output int) {
	if n := len(w.samples); n > 0 && (port != w.port || wipeOnRestart(at, input, w.newest())) {
		w.samples = w.samples[:0]
	}
	w.port = port
	w.samples = append(w.samples, throughputSample{at: at, input: input, output: output})
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

// wipeOnRestart reports whether the process restarted between prev and the new
// sample at at, as seen through benthos' monotonic input_received counter. That
// counter resets to 0 on a process restart, so a drop against the immediately
// preceding sample (D5a's counter-reset detector, input-only) is the signal that
// the old by-time series ended and must be wiped. It deliberately does not also
// require the output counter to drop: a restart where output was already ~0
// (e.g. a backed-up broker) would otherwise never wipe and would keep a stale
// pre-restart baseline in the window. A non-newer sample (an aged, out-of-order
// arrival) is a pruning case, not a restart, so it is skipped.
func wipeOnRestart(at time.Time, input int, prev throughputSample) bool {
	return at.After(prev.at) && input < prev.input
}

// newest returns the sample with the latest observed time. It returns the zero
// sample when the window is empty, so it never panics on an un-populated window.
func (w *throughputWindow) newest() throughputSample {
	if len(w.samples) == 0 {
		return throughputSample{}
	}
	newest := w.samples[0]
	for _, s := range w.samples[1:] {
		if s.at.After(newest.at) {
			newest = s
		}
	}
	return newest
}

// inputRate returns the input rate in messages per second over the in-window
// span: (newest.count - oldest-in-window.count) / elapsedSeconds, where
// elapsedSeconds is the real time between those two samples. With fewer than two
// in-window samples the rate is 0, because a single sample cannot be delta-ed. A
// genuine restart wipes the window in Add, so the newest sample is already the
// post-restart baseline. The newest-vs-oldest guards below cover the mixed cases
// the wipe does not (a one-sided drop, an equal-timestamp arrival, or a restart
// where one counter was already zero), reading 0 instead of a negative delta; an
// elapsed span of zero reads 0 rather than dividing by zero.
func (w *throughputWindow) inputRate() float64 {
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

	if newest.input < oldest.input {
		return 0
	}

	elapsed := newest.at.Sub(oldest.at).Seconds()
	if elapsed <= 0 {
		return 0
	}
	return float64(newest.input-oldest.input) / elapsed
}

// outputRate is inputRate but over the output counter.
func (w *throughputWindow) outputRate() float64 {
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

	if newest.output < oldest.output {
		return 0
	}

	elapsed := newest.at.Sub(oldest.at).Seconds()
	if elapsed <= 0 {
		return 0
	}
	return float64(newest.output-oldest.output) / elapsed
}

// inputCount returns the newest sample's input counter value. It returns 0 on an
// empty window. Because Add prunes, the newest sample is always in-window.
func (w *throughputWindow) inputCount() int {
	return w.newest().input
}

// outputCount returns the newest sample's output counter value. It returns 0 on
// an empty window.
func (w *throughputWindow) outputCount() int {
	return w.newest().output
}
