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

package cpuhealth

import "time"

// State is the overall CPU health state.
type State string

const (
	StateHealthy  State = "healthy"
	StateDegraded State = "degraded"
)

// Attribution names the dominant cause class when degraded.
type Attribution string

const (
	AttributionUnknown Attribution = "unknown"
)

// CauseKind enumerates the reason classes that can degrade CPU health.
type CauseKind string

const (
	CauseKindSaturation CauseKind = "saturation"
)

// Cause is a single degradation reason with an associated numeric value.
type Cause struct {
	Kind  CauseKind
	Value float64
}

// Thresholds holds the tunable cutoffs for the Decide verdict.
type Thresholds struct {
	HighUsageFraction float64
}

// DefaultThresholds returns the canonical thresholds (HighUsageFraction 0.70).
func DefaultThresholds() Thresholds {
	return Thresholds{HighUsageFraction: 0.70}
}

// Sample is a single point-in-time CPU usage observation.
type Sample struct {
	Timestamp   time.Time
	UsageCores  float64
	CgroupCores float64
}

// WindowState is reserved for windowed verdicts (saturation-over-N-samples);
// Decide does not read it yet.
type WindowState struct{}

// Signals holds derived intermediate values computed during Decide.
type Signals struct {
	UsageFraction float64
}

// Verdict is the output of Decide.
type Verdict struct {
	State       State
	Attribution Attribution
	Causes      []Cause
}

// Decide computes a pure CPU-health verdict from a sample. When
// thresholds.HighUsageFraction is not a positive finite value (<=0 or NaN),
// it falls back to DefaultThresholds(). The negated-`>0` guard makes a NaN
// threshold route to the default rather than silently disabling detection:
// `NaN <= 0` is false, so a bare `<= 0` check would keep a NaN threshold, and
// `fraction >= NaN` is always false, blinding the monitor.
//
// When Sample.CgroupCores <= 0 (uncapped or host-level sample), Decide cannot
// compute a quota-relative fraction and returns StateHealthy regardless of
// UsageCores; host-contention attribution is not computed by Decide.
func Decide(_ *WindowState, sample Sample, thresholds Thresholds) (Verdict, Signals) {
	if !(thresholds.HighUsageFraction > 0) {
		thresholds = DefaultThresholds()
	}

	var fraction float64
	if sample.CgroupCores > 0 {
		fraction = sample.UsageCores / sample.CgroupCores
	}

	signals := Signals{UsageFraction: fraction}

	if fraction >= thresholds.HighUsageFraction {
		return Verdict{
			State:       StateDegraded,
			Attribution: AttributionUnknown,
			Causes:      []Cause{{Kind: CauseKindSaturation, Value: fraction}},
		}, signals
	}

	return Verdict{State: StateHealthy}, signals
}
