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

// failureCounter is one cumulative error counter the worker judges.
type failureCounter struct {
	// name is the counter's name everywhere: the table's signal, the
	// sample's key, and the verdict's entry.
	name string
	// label is the wording the entry's reason shows for a count of 1.
	label string
	// tier is the counter's rank class in the verdict's order, lower first.
	tier int
	// read sums the counter off one scrape's Metrics.
	read func(benthosmonitorservice.Metrics) int64
}

// failureCounters lists every counter the failure signals are built from.
var failureCounters = []failureCounter{
	{
		name:  "output_error",
		label: "output error",
		tier:  0,
		read:  benthosmonitorservice.Metrics.OutputErrorTotal,
	},
	{
		name:  "processor_error",
		label: "processor error",
		tier:  0,
		read:  benthosmonitorservice.Metrics.ProcessorErrorTotal,
	},
}

// connectionSide is one side of a benthos instance whose connection the
// worker judges: its inputs or its outputs.
type connectionSide struct {
	// name is the side's name everywhere: the table's signal, the sample's
	// key, and the verdict's entry.
	name string
	// label is the wording the entry's reason shows for one path.
	label string
	// notConnected counts the paths on this side that are not connected.
	notConnected func(benthosmonitorservice.Metrics) int64
}

// connectionTier ranks the connection signals after the failure counters.
const connectionTier = 1

// connectionSides lists the sides the connection signals are built from.
var connectionSides = []connectionSide{
	{name: "input_connection", label: "input", notConnected: inputsNotConnected},
	{name: "output_connection", label: "output", notConnected: outputsNotConnected},
}

// notConnected reports whether one path's connection counters say it is not
// connected now. Benthos raises connection_up at the first connect and again
// after each connection_lost (benthos v4, internal/component/input
// async_reader.go and internal/component/output async_writer.go), so up not
// above lost means the path is connecting. A failed or lost connection says an
// attempt already went wrong; a first connect still in progress has neither
// and is not reported.
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
	out := make(map[string]int64, len(connectionSides))
	for _, c := range connectionSides {
		out[c.name] = c.notConnected(m)
	}

	return out
}

// increase is the reduction the failure signals judge by: how much a counter
// rose between the window's oldest and newest sample. Min 2, because a single
// sample has no rise to read.
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

func readFailures(m benthosmonitorservice.Metrics) map[string]int64 {
	out := make(map[string]int64, len(failureCounters))
	for _, c := range failureCounters {
		out[c.name] = c.read(m)
	}

	return out
}

// sampleFrom turns one observation into the sample the engine reads: the
// throughput counters, one reading per failure counter, and one per
// connection side.
func sampleFrom(cur observation) sample {
	s := sample{
		inputReceived: diagnosis.Known(float64(cur.inputReceived)),
		outputSent:    diagnosis.Known(float64(cur.outputSent)),
		failures:      make(map[string]diagnosis.Reading, len(cur.failures)),
		connections:   make(map[string]diagnosis.Reading, len(cur.connections)),
	}
	for name, v := range cur.failures {
		s.failures[name] = diagnosis.Known(float64(v))
	}
	for name, v := range cur.connections {
		s.connections[name] = diagnosis.Known(float64(v))
	}

	return s
}

// entrySpec is what the verdict needs about one signal of the table.
type entrySpec struct {
	label  string
	tier   int
	window time.Duration
}

// specByName finds the failureCounters or connectionSides entry a signal name
// was built from. Every signal in the table is built from one of them, so the
// not-found arm has no reachable caller.
func specByName(name string) (entrySpec, bool) {
	for _, c := range failureCounters {
		if c.name == name {
			return entrySpec{label: c.label, tier: c.tier, window: constants.BenthosLogWindow}, true
		}
	}
	for _, c := range connectionSides {
		if c.name == name {
			return entrySpec{label: c.label, tier: connectionTier}, true
		}
	}

	return entrySpec{}, false
}

// failureVerdict turns the fired set of one Observe call into the verdict Poll
// reports, most urgent entry first. A window with fewer readings than the
// reduction needs reports 0.
func failureVerdict(e *diagnosis.Engine[sample], fired []diagnosis.Fired, at time.Time) benthosmonitorservice.FailureVerdict {
	v := benthosmonitorservice.FailureVerdict{ObservedAt: at}
	for _, f := range fired {
		spec, ok := specByName(f.Signal)
		if !ok {
			continue
		}

		now, _ := e.Reduction(f.Signal, f.Signal).Get()
		// A connection signal can stay fired after the newest scrape shows
		// every path connected, until its window has collected for its span.
		// The verdict follows the newest scrape, so it leaves the entry out.
		if spec.window == 0 && now == 0 {
			continue
		}

		v.Fired = append(v.Fired, benthosmonitorservice.FailureEntry{
			Signal: f.Signal,
			Label:  spec.label,
			Count:  now,
			Since:  f.Since,
			Window: spec.window,
		})
	}

	sort.SliceStable(v.Fired, func(i, j int) bool {
		a, _ := specByName(v.Fired[i].Signal)
		b, _ := specByName(v.Fired[j].Signal)
		if a.tier != b.tier {
			return a.tier < b.tier
		}

		return v.Fired[i].Count > v.Fired[j].Count
	})

	return v
}
