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

// What counts as a failure, and how one scrape becomes Failures.

package benthosmetrics

import (
	"fmt"
	"sort"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/diagnosis"
)

// errorCounter is one cumulative Benthos error counter the failure window
// judges.
type errorCounter struct {
	// name identifies the counter everywhere, including FailureEntry.Signal.
	name string
	// label is the wording the entry's reason shows for a count of 1.
	label string
	// read sums the counter off one scrape's Metrics.
	read func(Metrics) int64
}

// errorCounters lists the counters the error signals are built from. The
// names are the Benthos series names from names.go.
var errorCounters = []errorCounter{
	{
		name:  outputError,
		label: "output error",
		read:  Metrics.outputErrorTotal,
	},
	{
		name:  processorError,
		label: "processor error",
		read:  Metrics.processorErrorTotal,
	},
}

// errorCounterOf returns the error counter named name. Every fired error
// signal is built from an errorCounters entry, so no entry is a bug here.
func errorCounterOf(name string) errorCounter {
	for _, c := range errorCounters {
		if c.name == name {
			return c
		}
	}

	panic(fmt.Sprintf("benthosmetrics: no error counter named %q", name))
}

// connectionCheck counts, for the inputs or for the outputs of one Benthos
// instance, how many are not connected.
type connectionCheck struct {
	// name is the check's name in the observation's connections and in the
	// failure entry's Signal.
	name string
	// label is the wording the entry's reason shows for one input or output.
	label string
	// notConnected counts the inputs or outputs that are not connected.
	notConnected func(Metrics) int64
}

// inputConnectionSignal and outputConnectionSignal are our own signal names
// for the derived not-connected counts; they are not Benthos series names.
const (
	inputConnectionSignal  = "input_connection"
	outputConnectionSignal = "output_connection"
)

// connectionChecks has one check for the inputs and one for the outputs.
var connectionChecks = []connectionCheck{
	{name: inputConnectionSignal, label: "input", notConnected: inputsNotConnected},
	{name: outputConnectionSignal, label: "output", notConnected: outputsNotConnected},
}

// endpointDown reports whether one input or output is down: up <= lost (its
// last connect did not succeed) and failed or lost is above 0, so a first
// connect still in progress counts as up. Benthos sets these counters in
// input/async_reader.go and output/async_writer.go; names.go links both.
func endpointDown(up, lost, failed int64) bool {
	return up <= lost && (failed > 0 || lost > 0)
}

// inputsNotConnected counts the inputs of m that are not connected.
func inputsNotConnected(m Metrics) int64 {
	var n int64
	for _, in := range m.Inputs {
		if endpointDown(in.ConnectionUp, in.ConnectionLost, in.ConnectionFailed) {
			n++
		}
	}

	return n
}

// outputsNotConnected counts the outputs of m that are not connected.
func outputsNotConnected(m Metrics) int64 {
	var n int64
	for _, out := range m.Outputs {
		if endpointDown(out.ConnectionUp, out.ConnectionLost, out.ConnectionFailed) {
			n++
		}
	}

	return n
}

// readConnections returns, for each connection check, how many inputs or
// outputs of m are not connected, keyed by the check's name.
func readConnections(m Metrics) map[string]int64 {
	out := make(map[string]int64, len(connectionChecks))
	for _, c := range connectionChecks {
		out[c.name] = c.notConnected(m)
	}

	return out
}

// readErrors returns each error counter's sum over m, keyed by the counter's
// name.
func readErrors(m Metrics) map[string]int64 {
	out := make(map[string]int64, len(errorCounters))
	for _, c := range errorCounters {
		out[c.name] = c.read(m)
	}

	return out
}

// newIncrease returns the diagnosis reduction for error signals: how much a
// counter went up from the oldest to the newest reading in its window. It
// needs at least two readings, and panics on a constructor error, a bug here.
func newIncrease() diagnosis.Reduction {
	r, err := diagnosis.NewReduction("increase", 2, func(p []diagnosis.Point) float64 {
		return p[len(p)-1].Value - p[0].Value
	})
	if err != nil {
		panic(fmt.Sprintf("benthosmetrics: invalid increase reduction: %v", err))
	}

	return r
}

// errorReadings turns one observation's error counters into the readings the
// error engine reads.
func errorReadings(cur observation) readings {
	s := readings{
		inputReceived: diagnosis.Unknown(),
		outputSent:    diagnosis.Unknown(),
		errors:        make(map[string]diagnosis.Reading, len(cur.errors)),
	}
	for name, v := range cur.errors {
		s.errors[name] = diagnosis.Known(float64(v))
	}

	return s
}

// seedErrors is the readings a restarted error engine observes first: each
// error counter at its value in errorCounts, or 0 when errorCounts has none.
func seedErrors(errorCounts map[string]int64) readings {
	s := readings{
		inputReceived: diagnosis.Unknown(),
		outputSent:    diagnosis.Unknown(),
		errors:        make(map[string]diagnosis.Reading, len(errorCounters)),
	}
	for _, c := range errorCounters {
		s.errors[c.name] = diagnosis.Known(float64(errorCounts[c.name]))
	}

	return s
}

// firstNotConnectedSince returns, per connection check with something not
// connected, when that started: the time from prev, or at if prev has none.
func firstNotConnectedSince(prev map[string]time.Time, counts map[string]int64, at time.Time) map[string]time.Time {
	out := make(map[string]time.Time, len(counts))
	for name, n := range counts {
		if n == 0 {
			continue
		}

		since, ok := prev[name]
		if !ok {
			since = at
		}
		out[name] = since
	}

	return out
}

// judgeFailures builds the Failures for one scrape: error entries first, then
// connection entries, each group with the larger Count first. An error
// entry's Count is how much its counter went up within ErrorWindow, and its
// LastErrorAt is the last scrape at which that counter rose.
func judgeFailures(e *diagnosis.Engine[readings], fired []diagnosis.Fired, cur observation) Failures {
	v := Failures{ObservedAt: cur.at}
	for _, f := range fired {
		c := errorCounterOf(f.Signal)
		n, _ := e.Reduction(f.Signal, f.Signal).Get() // 0 until the window holds 2 readings
		v.Entries = append(v.Entries, FailureEntry{
			Signal:      c.name,
			Label:       c.label,
			Count:       int64(n),
			Since:       f.Since,
			LastErrorAt: lastRise(cur, c.name, f.Since),
		})
	}
	errorEntries := len(v.Entries)
	for _, c := range connectionChecks {
		n := cur.connections[c.name]
		if n == 0 {
			continue
		}
		v.Entries = append(v.Entries, FailureEntry{
			Signal:     c.name,
			Label:      c.label,
			Count:      n,
			Since:      cur.notConnectedSince[c.name],
			Connection: true,
		})
	}
	sortEntriesByUrgency(v.Entries[:errorEntries])
	sortEntriesByUrgency(v.Entries[errorEntries:])

	return v
}

// lastRise returns when the error counter named signal last rose, for the
// entry that opened at since: cur.at when it rose this scrape (a counter
// missing from cur.before counts as 0, so after a restart every non-zero
// counter rose), the previous scrape's entry for the same episode when it
// did not, and the episode's first rise when there is no previous entry.
func lastRise(cur observation, signal string, since time.Time) time.Time {
	if cur.errors[signal] > cur.before[signal] {
		return cur.at
	}

	pe, ok := prevEpisode(cur.prevEntries, signal, since)
	if !ok {
		return since
	}

	// Re-dated from cur.at by the monotonic elapsed time, so NotCleared
	// measures real time even after a wall-clock step and the store's JSON
	// round-trip, which drops monotonic readings.
	return cur.at.Add(-cur.at.Sub(pe.LastErrorAt))
}

// prevEpisode returns the previous scrape's entry for the same episode: the
// entry with the same signal and the same Since.
func prevEpisode(prevEntries []FailureEntry, signal string, since time.Time) (FailureEntry, bool) {
	for _, pe := range prevEntries {
		if pe.Signal == signal && pe.Since.Equal(since) {
			return pe, true
		}
	}

	return FailureEntry{}, false
}

// sortEntriesByUrgency sorts entries in place by the larger Count first,
// keeping equals in list order.
func sortEntriesByUrgency(entries []FailureEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Count > entries[j].Count
	})
}
