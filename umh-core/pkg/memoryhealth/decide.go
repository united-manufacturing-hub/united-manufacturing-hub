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
	AvgUsedBytes    float64 `json:"avgUsedBytes"`
	AvgUsedFraction float64 `json:"avgUsedFraction"`
	UsedRingActive  bool    `json:"usedRingActive"`

	LimitApplies         bool    `json:"limitApplies"`
	TotalBytes           float64 `json:"totalBytes"`
	AvgLimitUsedFraction float64 `json:"avgLimitUsedFraction"`
	LimitRingActive      bool    `json:"limitRingActive"`

	HostTotalBytes      float64 `json:"hostTotalBytes"`
	AvgHostUsedFraction float64 `json:"avgHostUsedFraction"`
	HostRingActive      bool    `json:"hostRingActive"`

	PressureAvg60       float64 `json:"pressureAvg60"`
	PressureApplies     bool    `json:"pressureApplies"`
	PressureSignalReady bool    `json:"pressureSignalReady"`

	OomKillsLast60s float64 `json:"oomKillsLast60s"`
	OomKillsApply   bool    `json:"oomKillsApply"`
	OomSignalReady  bool    `json:"oomSignalReady"`
}

// Measured reports whether the window holds enough samples to judge the memory.
func (d Details) Measured() bool {
	return d.UsedRingActive
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

	avgUsedBytes, usedBytesReady := reducedValue(engine.Measurement(measurementUsedBytes).Get())
	avgUsedFraction, usedFractionReady := reducedValue(engine.Measurement(measurementUsedFraction).Get())
	avgLimitUsedFraction, limitReady := reducedValue(engine.Reduction(signalContainerLimitFull, instrumentContainerUsedFraction).Get())
	avgHostUsedFraction, hostReady := reducedValue(engine.Reduction(signalHostMemoryFull, instrumentHostUsedFraction).Get())
	pressureAvg60, pressureReady := reducedValue(engine.Reduction(signalPressure, instrumentPressureAvg60).Get())
	oomKillsLast60s, oomReady := reducedValue(engine.Reduction(signalOomKill, instrumentOomKillsLast60s).Get())

	return Details{
		AvgUsedBytes:    avgUsedBytes,
		AvgUsedFraction: avgUsedFraction,
		UsedRingActive:  usedBytesReady && usedFractionReady,

		LimitApplies:         sample.Source == SourceCgroupLimit,
		TotalBytes:           float64(sample.TotalBytes),
		AvgLimitUsedFraction: avgLimitUsedFraction,
		LimitRingActive:      limitReady,

		HostTotalBytes:      float64(sample.HostTotalBytes),
		AvgHostUsedFraction: avgHostUsedFraction,
		HostRingActive:      hostReady,

		PressureAvg60:       pressureAvg60,
		PressureApplies:     pressureRead,
		PressureSignalReady: pressureReady,

		OomKillsLast60s: oomKillsLast60s,
		OomKillsApply:   sample.OomCounterRead,
		OomSignalReady:  oomReady,
	}
}

func reducedValue(value float64, state diagnosis.State) (float64, bool) {
	if state != diagnosis.StateValue {
		return 0, false
	}

	return value, true
}
