// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package memoryhealth

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/diagnosis"
)

const (
	measurementUsedBytes    = "used-bytes"
	measurementUsedFraction = "used-fraction"
	unitFraction            = "fraction"

	sampleInterval = time.Second
	averageSpan    = 60 * time.Second
)

// NewEngine builds the engine that judges memory samples. Each engine keeps its
// own windows, so a worker builds one and feeds it every sample.
func NewEngine() (*diagnosis.Engine[Sample], error) {
	return diagnosis.NewEngine(memoryTable())
}

func memoryTable() diagnosis.Table[Sample] {
	return diagnosis.Table[Sample]{
		Interval: sampleInterval,
		Measurements: []diagnosis.Measurement[Sample]{
			{
				Name:      measurementUsedBytes,
				Extract:   usedBytes,
				Span:      averageSpan,
				Reduction: diagnosis.Mean,
			},
			{
				Name:      measurementUsedFraction,
				Extract:   usedFraction,
				Span:      averageSpan,
				Reduction: diagnosis.Mean,
			},
		},
		Signals: []diagnosis.Signal[Sample]{
			containerLimitFullSignal(),
			hostMemoryFullSignal(),
			pressureSignal(),
		},
	}
}

func usedBytes(sample Sample) diagnosis.Reading {
	return diagnosis.Known(float64(sample.UsedBytes))
}

func usedFraction(sample Sample) diagnosis.Reading {
	return fractionOf(sample.UsedBytes, sample.TotalBytes)
}

func fractionOf(usedBytes, totalBytes int64) diagnosis.Reading {
	if totalBytes <= 0 {
		return diagnosis.Unknown()
	}

	return diagnosis.Known(float64(usedBytes) / float64(totalBytes))
}
