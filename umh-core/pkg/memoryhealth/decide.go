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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/diagnosis"
)

var causeKinds = map[string]CauseKind{
	signalContainerLimitFull: CauseKindContainerLimitFull,
	signalHostMemoryFull:     CauseKindHostMemoryFull,
	signalPressure:           CauseKindPressure,
	signalOomKill:            CauseKindOomKill,
}

// Details holds the numbers behind a verdict.
type Details struct {
	AvgUsedFraction     diagnosis.Reading `json:"avgUsedFraction"`
	AvgUsedBytes        diagnosis.Reading `json:"avgUsedBytes"`
	AvgHostUsedFraction diagnosis.Reading `json:"avgHostUsedFraction"`
	PressureAvg60       diagnosis.Reading `json:"pressureAvg60"`
	OomKillsLast60s     diagnosis.Reading `json:"oomKillsLast60s"`

	AvgLimitUsedFraction diagnosis.Reading `json:"avgLimitUsedFraction"`
	LimitApplies         bool              `json:"limitApplies"`
	TotalBytes           float64           `json:"totalBytes"`
	HostTotalBytes       float64           `json:"hostTotalBytes"`
	PressureApplies      bool              `json:"pressureApplies"`
	OomKillsApply        bool              `json:"oomKillsApply"`
}

// Measured reports whether the window holds enough samples to judge the memory.
func (d Details) Measured() bool {
	_, measured := d.AvgUsedFraction.Get()

	return measured
}

// Decide feeds one sample to the engine and returns the verdict and the numbers
// behind it.
func Decide(engine *diagnosis.Engine[Sample], sample Sample) (Verdict, Details) {
	fired, _ := engine.Observe(sample, diagnosis.NewEnvironment(), sample.Timestamp)

	return buildVerdict(engine, fired), buildDetails(engine, sample)
}

func buildVerdict(engine *diagnosis.Engine[Sample], fired []diagnosis.Fired) Verdict {
	ranked := diagnosis.Rank(fired)
	if len(ranked) == 0 {
		return Verdict{State: StateHealthy}
	}

	causes := make([]Cause, len(ranked))
	for index, firedSignal := range ranked {
		causes[index] = describeCause(engine, firedSignal)
	}

	return Verdict{State: StateDegraded, Causes: causes}
}

func describeCause(engine *diagnosis.Engine[Sample], firedSignal diagnosis.Fired) Cause {
	value, _ := engine.Reduction(firedSignal.Signal, firedSignal.Instrument).Get()

	return Cause{Kind: causeKinds[firedSignal.Signal], Value: value}
}

func buildDetails(engine *diagnosis.Engine[Sample], sample Sample) Details {
	_, pressureRead := sample.Pressure.Get()

	return Details{
		AvgUsedFraction:     readingFromReduced(engine.Measurement(measurementUsedFraction).Get()),
		AvgUsedBytes:        readingFromReduced(engine.Measurement(measurementUsedBytes).Get()),
		AvgHostUsedFraction: readingFromReduced(engine.Reduction(signalHostMemoryFull, instrumentHostUsedFraction).Get()),
		PressureAvg60:       readingFromReduced(engine.Reduction(signalPressure, instrumentPressureAvg60).Get()),
		OomKillsLast60s:     readingFromReduced(engine.Reduction(signalOomKill, instrumentOomKillsLast60s).Get()),

		AvgLimitUsedFraction: readingFromReduced(engine.Reduction(signalContainerLimitFull, instrumentContainerUsedFraction).Get()),
		LimitApplies:         sample.Source == SourceCgroupLimit,
		TotalBytes:           float64(sample.TotalBytes),
		HostTotalBytes:       float64(sample.HostTotalBytes),
		PressureApplies:      pressureRead,
		OomKillsApply:        sample.OomCounterRead,
	}
}

func readingFromReduced(value float64, state diagnosis.State) diagnosis.Reading {
	if state != diagnosis.StateValue {
		return diagnosis.Unknown()
	}

	return diagnosis.Known(value)
}
