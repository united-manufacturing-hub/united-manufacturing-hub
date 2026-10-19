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

package memoryhealth

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

const (
	signalLimitFull        = "limit-full"
	instrumentUsedFraction = "used-fraction"
	unitFraction           = "fraction"

	sampleInterval = time.Second
	limitFullSpan  = 10 * time.Second

	limitFullFireFraction  = constants.MemoryHighThresholdPercent / 100
	limitFullClearFraction = limitFullFireFraction - 0.05
)

var limitFullMarks = diagnosis.Marks{
	Unit:     unitFraction,
	Fire:     diagnosis.Mark{At: limitFullFireFraction, Inclusive: true},
	Clear:    diagnosis.Mark{At: limitFullClearFraction},
	Polarity: diagnosis.HigherIsWorse,
	Worst:    1.0,
}

// NewEngine builds the engine that judges memory samples. Each engine keeps its
// own windows, so a worker builds one and feeds it every sample.
func NewEngine() (*diagnosis.Engine[Sample], error) {
	return diagnosis.NewEngine(memoryTable())
}

func memoryTable() diagnosis.Table[Sample] {
	return diagnosis.Table[Sample]{
		Interval: sampleInterval,
		Signals:  []diagnosis.Signal[Sample]{limitFullSignal()},
	}
}

func limitFullSignal() diagnosis.Signal[Sample] {
	return diagnosis.Signal[Sample]{
		Name:            signalLimitFull,
		DemoteSpan:      limitFullSpan,
		ReleaseOnAbsent: true,
		Instruments: []diagnosis.Instrument[Sample]{{
			Measurement: diagnosis.Measurement[Sample]{
				Name:      instrumentUsedFraction,
				Extract:   usedFraction,
				Span:      limitFullSpan,
				Reduction: diagnosis.Mean,
			},
			Marks: limitFullMarks,
		}},
	}
}

func usedFraction(sample Sample) diagnosis.Reading {
	if sample.TotalBytes <= 0 {
		return diagnosis.Unknown()
	}

	return diagnosis.Known(float64(sample.UsedBytes) / float64(sample.TotalBytes))
}
