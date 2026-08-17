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

// The per-tick fact bag: the numbers and flags the message layer needs that a
// ranked cause list cannot carry.

package cpuhealth

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// Signals is the per-tick fact bag ComposeMessage and BlockReason read to render
// a sentence the ranked cause list alone cannot carry. The first 31 fields keep
// their declaration order; everything after them is appended. Two of the three
// frozen signatures take a Signals, so Decide is its sole producer.
//
// ⚠️ Exactly two fields here end in "Available" — HostBusyCoresAvailable and
// HostHeadroomAvailable — and that is the cap this package allows before a
// family of near-duplicate booleans has to be modelled as a set instead, which
// is why the readiness trio below is named for what it holds.
//
// Flat for the same reasons as Sample, plus one of its own: a single signal
// already spans three of the groups below — throttle appears as ThrottleRatio,
// as ThrottleFired and as ThrottleSignalReady — so a per-signal type splinters
// the groups and a per-group type splinters the signals. Grouping would also
// multiply the value-beside-its-own-bool pairs the cap above holds to two.
type Signals struct {
	// The metrics. ThrottleRatio, PressureAvg60 and StealP95 are populated
	// independent of their latch state, so the number stays observable when the
	// latch has not fired.
	UsageFraction diagnosis.Reading // absent in every mode; declared for a future frontend projection
	ThrottleRatio float64           // 60s nr_throttled/nr_periods delta; fillSignals discards State, so absent or untrusted reads 0
	PressureAvg60 float64           // PSI avg60; NaN/±Inf are never stored (rejected at window ingest), and fillSignals discards State, so absent reads 0
	StealP95      float64           // 60s p95; fillSignals discards State, so bare metal or below the reduction's floor of 20 reads 0

	// Observability only: none of the six changes a verdict. The four Readings
	// are declared for a future frontend projection and nothing fills them.
	AvgUsageFraction float64 // 60s mean of usage against the logical CPU count
	P95UsageFraction diagnosis.Reading
	P99UsageFraction diagnosis.Reading
	AvgUsageCores    float64 // ABSOLUTE cores; limit-mode headroom and the wire's avgMCpu read this one value
	P95UsageCores    diagnosis.Reading
	P99UsageCores    diagnosis.Reading
	UsageRingActive  bool // usage-cores reduced to StateValue — the healthy headline's LIMIT-mode floor gate
	// HostBusyRingActive: host-busy reduced to StateValue — the healthy
	// headline's NO-LIMIT floor gate. Two tracks, two floors: an outage can
	// leave one thin while the other fills.
	HostBusyRingActive bool

	// The latches.
	ThrottleFired       bool
	PressureFired       bool
	StealFired          bool
	HostContentionFired bool // reserved; Decide sets it false unconditionally
	LimitedVisibility   bool // the dead-zone annotation, never a State

	// The saturation family. SaturationFired is the OR of the arms; the four
	// are what causeDetails and BlockReason dispatch on. Fired.Identity carries
	// the signal name only, so Decide recovers the instrument from
	// Fired.Marks.Unit: "cores" is host-headroom, "fraction" is usage-fraction.
	SaturationFired            bool
	LimitSaturationFired       bool
	HostFullFired              bool
	NoHostStatsSaturationFired bool
	NoLimitHostFired           bool

	// The headroom arithmetic. Neither headroom is clamped: a full box yields a
	// negative number, not a 0.
	HostHeadroomCores      float64
	HostBusyCoresAvailable bool // the sample's own readability flag; a ==0 proxy cannot tell unreadable from idle
	AvgHostBusyCores       float64
	HeadroomCores          diagnosis.Reading // declared for a future frontend projection; nothing fills it
	CapacityCores          float64           // the quota when set and positive, else LogicalCpus
	ReserveCores           float64

	// Capability, NOT readability. The healthy message's budget lines are
	// forbidden from reading these three as "the reading succeeded". The three
	// *SignalReady fields below are the readability half, and the two families
	// are never interchangeable.
	LimitApplies    bool
	PressureApplies bool
	StealApplies    bool

	// ---- past the first 31. Decide fills all three. ----
	//
	// HostHeadroomAvailable is the SECOND field ending "Available" and it puts
	// this struct AT the cap of two.
	HostHeadroomAvailable bool    // false when the scope is not ScopeHost: withheld, not failed
	LogicalCpus           float64 // the "2" — the CPUs this process may use
	HostCpus              float64 // the "8" — the machine's count, from the per-CPU lines of /proc/stat

	// ---- per-signal readiness. Decide fills all three from Observe's
	// second return. Each is Availability == Ready for that signal. ----
	ThrottleSignalReady bool
	PressureSignalReady bool
	StealSignalReady    bool
}
