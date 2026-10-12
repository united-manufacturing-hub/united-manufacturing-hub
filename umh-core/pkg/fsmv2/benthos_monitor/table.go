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
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// windowSpan bounds the throughput window BY TIME, not by FSMv1's count-based
// ThroughputWindowSize of 600 entries: the window takes one sample per poll and
// an unscheduled poll produces none, so 600 entries can stretch past a minute.
const windowSpan = 60 * time.Second

// Names of the engine's measurements.
const (
	inputRate  = "input_rate"
	outputRate = "output_rate"
)

// sample is one scrape's counters as the engine reads them.
type sample struct {
	inputReceived diagnosis.Reading
	outputSent    diagnosis.Reading
}

// table declares what the engine measures: the input and output rates, each
// the slope of its counter over windowSpan.
func table() diagnosis.Table[sample] {
	return diagnosis.Table[sample]{
		Interval: pollInterval,
		Measurements: []diagnosis.Measurement[sample]{
			{
				Name:      inputRate,
				Extract:   func(s sample) diagnosis.Reading { return s.inputReceived },
				Reduction: diagnosis.Slope,
				Span:      windowSpan,
			},
			{
				Name:      outputRate,
				Extract:   func(s sample) diagnosis.Reading { return s.outputSent },
				Reduction: diagnosis.Slope,
				Span:      windowSpan,
			},
		},
	}
}

// newEngine builds an engine from table. table is fixed at compile time, so an
// error here is a bug in table.
func newEngine() *diagnosis.Engine[sample] {
	e, err := diagnosis.NewEngine(table())
	if err != nil {
		panic(fmt.Sprintf("benthos_monitor: invalid diagnosis table: %v", err))
	}

	return e
}
