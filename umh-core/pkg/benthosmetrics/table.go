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

// What the engine measures: the diagnosis table and the readings it reads.

package benthosmetrics

import (
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/diagnosis"
)

// throughputWindow is how far back the throughput looks. It is a duration,
// not a readings count, so a skipped poll does not stretch the window past
// one minute.
const throughputWindow = 60 * time.Second

// Names of the engine's measurements.
const (
	inputMessagesPerSecond  = "input_messages_per_second"
	outputMessagesPerSecond = "output_messages_per_second"
)

// readings is one scrape's counters as the engine reads them.
type readings struct {
	inputReceived diagnosis.Reading
	outputSent    diagnosis.Reading
}

// table declares what the engine measures: the input and output messages per
// second, each the slope of its counter over throughputWindow. fsmv2/diagnosis
// computes the time windows; this table says what to measure and over how
// long.
func table() diagnosis.Table[readings] {
	return diagnosis.Table[readings]{
		Interval: ScrapeInterval,
		Measurements: []diagnosis.Measurement[readings]{
			{
				Name:      inputMessagesPerSecond,
				Extract:   func(s readings) diagnosis.Reading { return s.inputReceived },
				Reduction: diagnosis.Slope,
				Span:      throughputWindow,
			},
			{
				Name:      outputMessagesPerSecond,
				Extract:   func(s readings) diagnosis.Reading { return s.outputSent },
				Reduction: diagnosis.Slope,
				Span:      throughputWindow,
			},
		},
	}
}

// mustNewEngine builds an engine from table and panics on error: table is
// fixed at compile time, so an error is a bug in this package.
func mustNewEngine() *diagnosis.Engine[readings] {
	e, err := diagnosis.NewEngine(table())
	if err != nil {
		panic(fmt.Sprintf("benthosmetrics: invalid diagnosis table: %v", err))
	}

	return e
}
