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

import (
	"fmt"
	"sort"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
	benthosmonitorservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
)

// errorCounter is one cumulative Benthos error counter the worker judges.
type errorCounter struct {
	// name is the counter's name everywhere: the table's signal, the
	// sample's key, and the verdict's entry.
	name string
	// label is the wording the entry's reason shows for a count of 1.
	label string
	// read sums the counter off one scrape's Metrics.
	read func(benthosmonitorservice.Metrics) int64
}

// errorCounters lists the counters the error signals are built from.
var errorCounters = []errorCounter{
	{
		name:  "output_error",
		label: "output error",
		read:  benthosmonitorservice.Metrics.OutputErrorTotal,
	},
	{
		name:  "processor_error",
		label: "processor error",
		read:  benthosmonitorservice.Metrics.ProcessorErrorTotal,
	},
}

// connectionCheck counts, for the inputs or for the outputs of one benthos
// instance, how many are not connected.
type connectionCheck struct {
	// name is the check's name in observation.connections and in the
	// verdict's entry.
	name string
	// label is the wording the entry's reason shows for one input or output.
	label string
	// notConnected counts the inputs or outputs that are not connected.
	notConnected func(benthosmonitorservice.Metrics) int64
}

// connectionChecks has one check for the inputs and one for the outputs.
var connectionChecks = []connectionCheck{
	{name: "input_connection", label: "input", notConnected: inputsNotConnected},
	{name: "output_connection", label: "output", notConnected: outputsNotConnected},
}

// notConnected reports whether one Benthos input or output is down now.
// Benthos raises connection_up on every successful connect, and
// connection_lost on every drop (benthos v4, internal/component/input
// async_reader.go and internal/component/output async_writer.go). So up <= lost
// means the last connect did not succeed. A first connect still in progress
// also has up == lost == 0. It counts as down only once a connect failed or a
// connection was lost.
func notConnected(up, lost, failed int64) bool {
	return up <= lost && (failed > 0 || lost > 0)
}

func inputsNotConnected(m benthosmonitorservice.Metrics) int64 {
	var n int64
	for _, in := range m.Inputs {
		if notConnected(in.ConnectionUp, in.ConnectionLost, in.ConnectionFailed) {
			n++
		}
	}

	return n
}

func outputsNotConnected(m benthosmonitorservice.Metrics) int64 {
	var n int64
	for _, out := range m.Outputs {
		if notConnected(out.ConnectionUp, out.ConnectionLost, out.ConnectionFailed) {
			n++
		}
	}

	return n
}

func readConnections(m benthosmonitorservice.Metrics) map[string]int64 {
	out := make(map[string]int64, len(connectionChecks))
	for _, c := range connectionChecks {
		out[c.name] = c.notConnected(m)
	}

	return out
}

// increase reports how much a counter rose between the oldest and newest
// reading in the window. It needs at least 2 readings.
var increase = mustIncrease()

func mustIncrease() diagnosis.Reduction {
	r, err := diagnosis.NewReduction("increase", 2, func(p []diagnosis.Point) float64 {
		return p[len(p)-1].Value - p[0].Value
	})
	if err != nil {
		panic(fmt.Sprintf("benthos_monitor: invalid increase reduction: %v", err))
	}

	return r
}

func readErrors(m benthosmonitorservice.Metrics) map[string]int64 {
	out := make(map[string]int64, len(errorCounters))
	for _, c := range errorCounters {
		out[c.name] = c.read(m)
	}

	return out
}

// sampleFrom turns one observation into the sample the engine reads: the
// throughput counters and one reading per error counter.
func sampleFrom(cur observation) sample {
	s := sample{
		inputReceived: diagnosis.Known(float64(cur.inputReceived)),
		outputSent:    diagnosis.Known(float64(cur.outputSent)),
		errors:        make(map[string]diagnosis.Reading, len(cur.errors)),
	}
	for name, v := range cur.errors {
		s.errors[name] = diagnosis.Known(float64(v))
	}

	return s
}

// notConnectedSince returns, for each connection check whose count is above 0,
// the scrape at which that count went above 0: prev's entry, or at when prev
// has none. A check whose count is 0 has no entry.
func notConnectedSince(prev map[string]time.Time, counts map[string]int64, at time.Time) map[string]time.Time {
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

// failureVerdict builds the verdict Poll reports for cur. It lists the fired
// error signals first, then the connection checks whose count is above 0, each
// group with the larger Count first.
func failureVerdict(e *diagnosis.Engine[sample], fired []diagnosis.Fired, cur observation) benthosmonitorservice.FailureVerdict {
	v := benthosmonitorservice.FailureVerdict{ObservedAt: cur.at, LastIncreaseAt: cur.lastIncreaseAt}
	for _, f := range fired {
		// table() builds Signals[i] from errorCounters[i].
		c := errorCounters[f.Index]
		n, _ := e.Reduction(f.Signal, f.Signal).Get() // 0 until the window holds 2 readings
		v.Fired = append(v.Fired, benthosmonitorservice.FailureEntry{
			Signal: c.name,
			Label:  c.label,
			Count:  int64(n),
			Since:  f.Since,
			Window: constants.BenthosLogWindow,
		})
	}
	errorEntries := len(v.Fired)
	for _, c := range connectionChecks {
		n := cur.connections[c.name]
		if n == 0 {
			continue
		}
		v.Fired = append(v.Fired, benthosmonitorservice.FailureEntry{
			Signal: c.name,
			Label:  c.label,
			Count:  n,
			Since:  cur.notConnectedSince[c.name],
		})
	}
	byCount(v.Fired[:errorEntries])
	byCount(v.Fired[errorEntries:])

	return v
}

// byCount orders entries by Count, largest first.
func byCount(entries []benthosmonitorservice.FailureEntry) {
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Count > entries[j].Count })
}
