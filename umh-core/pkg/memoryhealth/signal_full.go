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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/diagnosis"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
)

const (
	signalContainerLimitFull = "container-limit-full"
	signalHostMemoryFull     = "host-memory-full"

	instrumentContainerUsedFraction = "container-used-fraction"
	instrumentHostUsedFraction      = "host-used-fraction"

	fullFireFraction  = constants.MemoryHighThresholdPercent / 100
	fullClearFraction = fullFireFraction - 0.05
)

var fullMarks = diagnosis.Marks{
	Unit:     unitFraction,
	Fire:     diagnosis.Mark{At: fullFireFraction, Inclusive: true},
	Clear:    diagnosis.Mark{At: fullClearFraction},
	Polarity: diagnosis.HigherIsWorse,
	Worst:    1.0,
}

func containerLimitFullSignal() diagnosis.Signal[Sample] {
	return fullSignal(signalContainerLimitFull, instrumentContainerUsedFraction, containerUsedFraction)
}

func hostMemoryFullSignal() diagnosis.Signal[Sample] {
	return fullSignal(signalHostMemoryFull, instrumentHostUsedFraction, hostUsedFraction)
}

func fullSignal(name, instrument string, extract func(Sample) diagnosis.Reading) diagnosis.Signal[Sample] {
	return diagnosis.Signal[Sample]{
		Name:            name,
		DemoteSpan:      averageSpan,
		ReleaseOnAbsent: true,
		Instruments: []diagnosis.Instrument[Sample]{{
			Measurement: diagnosis.Measurement[Sample]{
				Name:      instrument,
				Extract:   extract,
				Span:      averageSpan,
				Reduction: diagnosis.Mean,
			},
			Marks: fullMarks,
		}},
	}
}

func containerUsedFraction(sample Sample) diagnosis.Reading {
	if sample.Source != SourceCgroupLimit {
		return diagnosis.Unknown()
	}

	return fractionOf(sample.UsedBytes, sample.TotalBytes)
}

func hostUsedFraction(sample Sample) diagnosis.Reading {
	return fractionOf(sample.HostUsedBytes, sample.HostTotalBytes)
}
