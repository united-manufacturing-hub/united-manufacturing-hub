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
//
// Quota is the cgroup CPU quota as a pointer so that nil distinguishes "no
// quota set" from a read-failed / first-call / genuinely-idle zero. When Quota
// is non-nil, Decide uses it exclusively: a positive value derives the usage
// fraction (UsageCores/Quota), and a non-positive value (zero, negative, or
// NaN — the `> 0` guard rejects all three, mirroring the legacy CgroupCores
// guard) means uncapped, so a zero quota — the unlimited-cgroup case returned
// by parseCPUMax for `cpu.max = "max <period>"` — cannot produce a +Inf/NaN
// poison fraction and stays healthy. When Quota is nil, Decide falls back to
// CgroupCores when it is positive, and otherwise treats the sample as uncapped
// (StateHealthy, no fraction computed).
// CgroupCores is the legacy non-pointer representation retained for callers
// that still populate it (it remains the live production path).
type Sample struct {
	Timestamp   time.Time
	UsageCores  float64
	CgroupCores float64
	Quota       *float64
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
// When Quota is non-nil, Decide uses it exclusively: a positive Quota yields
// UsageCores/Quota, and a non-positive Quota (zero/negative/NaN) means
// uncapped (the `> 0` guard rejects all three), so the unlimited-cgroup case
// (parseCPUMax quotaCores==0) is treated as uncapped instead of poisoning the
// fraction with +Inf/NaN — there is no fallback to CgroupCores when Quota is
// non-nil. When Quota is nil, the fraction is derived from CgroupCores when
// positive. When neither is available the sample is uncapped and Decide
// returns StateHealthy regardless of UsageCores (uncapped cannot be
// quota-saturated); host-contention attribution is not computed by Decide.
func Decide(_ *WindowState, sample Sample, thresholds Thresholds) (Verdict, Signals) {
	if !(thresholds.HighUsageFraction > 0) {
		thresholds = DefaultThresholds()
	}

	var fraction float64
	if sample.Quota != nil {
		if *sample.Quota > 0 {
			fraction = sample.UsageCores / *sample.Quota
		}
	} else if sample.CgroupCores > 0 {
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
