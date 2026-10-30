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
)

const (
	signalPressure          = "pressure"
	instrumentPressureAvg60 = "pressure-avg60"
)

var pressureMarks = diagnosis.Marks{
	Unit:     unitFraction,
	Fire:     diagnosis.Mark{At: 0.20},
	Clear:    diagnosis.Mark{At: 0.12},
	Polarity: diagnosis.HigherIsWorse,
	Worst:    1.0,
}

func pressureSignal() diagnosis.Signal[Sample] {
	return diagnosis.Signal[Sample]{
		Name:            signalPressure,
		Tier:            tierStarvation,
		DemoteSpan:      averageSpan,
		ReleaseOnAbsent: true,
		Instruments: []diagnosis.Instrument[Sample]{{
			Measurement: diagnosis.Measurement[Sample]{
				Name:      instrumentPressureAvg60,
				Extract:   func(sample Sample) diagnosis.Reading { return sample.Pressure },
				Span:      averageSpan,
				Reduction: diagnosis.Last,
			},
			Marks: pressureMarks,
		}},
	}
}
