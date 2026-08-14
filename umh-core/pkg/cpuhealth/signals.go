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

// Signals is the per-tick fact bag ComposeMessage and BlockReason read to render
// a sentence the ranked cause list alone cannot carry. The first 31 fields are
// the parked shape in declaration order; everything after them is appended. Two
// of the three frozen signatures take a Signals, so Decide is its sole producer.
//
// ⚠️ C1 scores this struct at exactly 2 (*Available fields: HostBusyCoresAvailable
// and HostHeadroomAvailable) against a cap of 2 — a third fails the gate, which
// is why the readiness trio below is named for what it holds instead.
type Signals struct {
	// The metrics. Each is populated independent of its latch state, so the
	// number stays observable when the latch has not fired.
	UsageFraction    float64 // quota-relative; collapses to 0 in no-limit mode
	ThrottleRatio    float64 // 60s nr_throttled/nr_periods delta; negatives clamped to 0
	PressureAvg60Out float64 // PSI avg60; NaN/negative/+Inf clamped to 0
	StealP95         float64 // 60s p95; 0 on bare metal and below 2 samples

	// Observability only: none of the six changes a verdict.
	AvgUsageFraction float64
	P95UsageFraction float64
	P99UsageFraction float64
	AvgUsageCores    float64 // ABSOLUTE cores; limit-mode headroom and the wire's avgMCpu read this one value
	P95UsageCores    float64
	P99UsageCores    float64
	UsageRingActive  bool // usage-cores reduced to StateValue — S4 R2's LIMIT-mode floor gate
	// HostBusyRingActive: host-busy reduced to StateValue — S4 R2's NO-LIMIT
	// floor gate. Two tracks, two floors: an outage can leave one thin while
	// the other fills.
	HostBusyRingActive bool

	// The latches.
	ThrottleFired       bool
	PressureFired       bool
	StealFired          bool
	HostContentionFired bool // reserved; Decide sets it false unconditionally
	LimitedVisibility   bool // the dead-zone annotation, never a State

	// The saturation family. SaturationFired is the OR of the arms; the four
	// are what S4 R5 dispatches on. Fired.Identity carries the signal name only,
	// so Decide recovers the instrument from Fired.Marks.Unit: "cores" is
	// host-headroom, "fraction" is usage-fraction.
	SaturationFired               bool
	LimitSaturationFired          bool
	HostFullFired                 bool
	NoHostStatsSaturationFired    bool
	NoLimitHostFired              bool
	NoHostStatsSaturationFraction float64

	// The headroom arithmetic. Neither headroom is clamped: a full box yields a
	// negative number, not a 0.
	HostHeadroomCores      float64
	HostBusyCoresAvailable bool // the sample's own readability flag; a ==0 proxy cannot tell unreadable from idle
	HostBusyCores60sMean   float64
	HeadroomCores          float64 // the saturation decision variable
	CapacityCores          float64 // the quota when set and positive, else LogicalCpus
	ReserveCores           float64

	// Capability, NOT readability. F1 is that distinction, and S4 R3 is the rung
	// forbidden from reading these three as "the reading succeeded". The three
	// *SignalReady fields below are the readability half, and the two families
	// are never interchangeable.
	LimitApplies bool
	PsiApplies   bool
	StealApplies bool

	// ---- past the parked 31. S3 R5 fills all three. ----
	//
	// HostHeadroomAvailable is the SECOND field matching C1's pattern and it
	// puts this struct AT the cap of two.
	HostHeadroomAvailable bool    // false when the scope is not ScopeHost: withheld, not failed
	LogicalCpus           float64 // the "2" — the CPUs this process may use
	HostCpus              float64 // the "8" — the machine's count, from S2 R4b

	// ---- per-signal readiness. S3 R8 spec 4 fills all three from Observe's
	// second return. Each is Availability == Ready for that signal. ----
	ThrottleSignalReady bool
	PressureSignalReady bool
	StealSignalReady    bool
}
