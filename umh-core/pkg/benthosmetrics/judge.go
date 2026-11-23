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

// What counts as a failure, and how one scrape's failure counters are read.

package benthosmetrics

import (
	"fmt"
	"time"
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

// outputErrorSignal and processorErrorSignal are the signal names of the
// two error counters. They spell the same as the Benthos series they sum,
// but nothing reads them as series names: read sums the parsed fields.
const (
	outputErrorSignal    = "output_error"
	processorErrorSignal = "processor_error"
)

// errorCounters lists the counters the error signals are built from.
var errorCounters = []errorCounter{
	{
		name:  outputErrorSignal,
		label: "output error",
		read:  Metrics.outputErrorTotal,
	},
	{
		name:  processorErrorSignal,
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

// readFailureReading returns what failureTracker.observe reads off m, the
// scrape taken at time at.
func readFailureReading(m Metrics, at time.Time) failureReading {
	return failureReading{
		at:          at,
		errorCounts: readErrors(m),
		connections: readConnections(m),
	}
}
