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
	"time"
)

// signal names one thing that can fail: an error counter the failure window
// judges, or a connection check. It keys the failureTracker's state and names
// the error engine's signals; the FailureReport has a field for each instead.
type signal string

// errorCounter is one cumulative Benthos error counter the failure window
// judges.
type errorCounter struct {
	// name keys the counter in the failureTracker and the error engine.
	name signal
	// label is the wording the reason shows for a count of 1.
	label string
	// read sums the counter off one scrape's Metrics.
	read func(Metrics) int64
	// field points at this counter's field in a report: judge sets it,
	// ErrorFree reads it.
	field func(*FailureReport) **RecentErrors
}

// The signals of the two error counters. They spell the same as the Benthos
// series they sum, but nothing reads them as series names: read sums the
// parsed fields.
const (
	signalOutputError    signal = "output_error"
	signalProcessorError signal = "processor_error"
)

// errorCounters lists the counters the error signals are built from.
var errorCounters = []errorCounter{
	{
		name:  signalOutputError,
		label: "output error",
		read:  Metrics.outputErrorTotal,
		field: func(r *FailureReport) **RecentErrors { return &r.OutputErrors },
	},
	{
		name:  signalProcessorError,
		label: "processor error",
		read:  Metrics.processorErrorTotal,
		field: func(r *FailureReport) **RecentErrors { return &r.ProcessorErrors },
	},
}

// connectionCheck counts, for the inputs or for the outputs of one Benthos
// instance, how many are not connected.
type connectionCheck struct {
	// name keys the check in the failureTracker.
	name signal
	// label is the wording the reason shows for one input or output.
	label string
	// notConnected counts the inputs or outputs that are not connected.
	notConnected func(Metrics) int64
	// field points at this check's field in a report: judge sets it,
	// ErrorFree reads it.
	field func(*FailureReport) **NotConnected
}

// The signals of the two connection checks. These are our own names for the
// derived not-connected counts; they are not Benthos series names.
const (
	signalInputConnection  signal = "input_connection"
	signalOutputConnection signal = "output_connection"
)

// connectionChecks has one check for the inputs and one for the outputs.
var connectionChecks = []connectionCheck{
	{
		name:         signalInputConnection,
		label:        "input",
		notConnected: inputsNotConnected,
		field:        func(r *FailureReport) **NotConnected { return &r.InputsNotConnected },
	},
	{
		name:         signalOutputConnection,
		label:        "output",
		notConnected: outputsNotConnected,
		field:        func(r *FailureReport) **NotConnected { return &r.OutputsNotConnected },
	},
}

// inputsNotConnected counts the inputs of m that are not connected.
func inputsNotConnected(m Metrics) int64 {
	var n int64

	for _, in := range m.Inputs {
		if in.down() {
			n++
		}
	}

	return n
}

// outputsNotConnected counts the outputs of m that are not connected.
func outputsNotConnected(m Metrics) int64 {
	var n int64

	for _, out := range m.Outputs {
		if out.down() {
			n++
		}
	}

	return n
}

// readConnections returns, for each connection check, how many inputs or
// outputs of m are not connected, keyed by the check's name.
func readConnections(m Metrics) map[signal]int64 {
	out := make(map[signal]int64, len(connectionChecks))
	for _, c := range connectionChecks {
		out[c.name] = c.notConnected(m)
	}

	return out
}

// readErrors returns each error counter's sum over m, keyed by the counter's
// name.
func readErrors(m Metrics) map[signal]int64 {
	out := make(map[signal]int64, len(errorCounters))
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
