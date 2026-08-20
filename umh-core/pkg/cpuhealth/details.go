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

// The per-tick values and flags the message layer interpolates; a ranked cause
// list cannot carry them.

package cpuhealth

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// Details carries the per-tick values and flags ComposeMessage and
// BlockReason read to render a sentence a ranked cause list alone cannot
// carry. detailsFor is its sole producer; buildVerdict returns only the
// Verdict. Flat, like
// Sample, because a single signal already spans three of the groups below —
// throttle is ThrottleRatio, ThrottleFired, and ThrottleSignalReady — so
// splitting by signal or by group would splinter the other.
type Details struct {
	// The metrics. ThrottleRatio, PressureAvg60 and StealP95 are filled every
	// tick, independent of their latch's fired state, so the number stays
	// observable even when the latch has not fired.
	UsageFraction diagnosis.Reading // absent in every mode; declared for a future frontend projection
	ThrottleRatio float64           // 60s nr_throttled/nr_periods delta; detailsFor discards State, so absent or untrusted reads 0
	PressureAvg60 float64           // PSI avg60; NaN/±Inf are never stored (rejected at window ingest), and detailsFor discards State, so absent reads 0
	StealP95      float64           // 60s p95; detailsFor discards State, so bare metal or below the reduction's floor of 20 reads 0

	// Observability only: none of the six below changes a verdict. The four
	// Readings are declared for a future frontend projection and nothing fills
	// them; AvgUsageFraction and AvgUsageCores are the two exceptions, filled
	// every tick from their own signal's reduction.
	AvgUsageFraction float64           // usage-fraction's own 60s reduction — the number the host-cpu-full latch was judged on, not the usage track divided by anything, so a mid-run core-count change cannot split them
	P95UsageFraction diagnosis.Reading // declared for a future frontend projection; nothing fills it
	P99UsageFraction diagnosis.Reading // declared for a future frontend projection; nothing fills it
	AvgUsageCores    float64           // ABSOLUTE cores; limit-mode headroom and the wire's avgMCpu read this one value — the usage-cores track's 60s mean
	P95UsageCores    diagnosis.Reading // declared for a future frontend projection; nothing fills it
	P99UsageCores    diagnosis.Reading // declared for a future frontend projection; nothing fills it
	UsageRingActive  bool              // usage-cores reduced to StateValue — the healthy headline's LIMIT-mode floor gate
	// HostBusyRingActive: host-busy reduced to StateValue — the healthy
	// headline's NO-LIMIT floor gate. Two tracks, two floors: an outage can
	// leave one thin while the other fills.
	HostBusyRingActive bool

	// The latches. detailsFor sets ThrottleFired, PressureFired and StealFired
	// from the fired set. The two capacity signals have no flag here: their
	// causes name the kind and the instrument.
	ThrottleFired bool // true when the throttling signal fired this tick
	PressureFired bool // true when the pressure signal fired this tick
	StealFired    bool // true when the steal signal fired this tick
	// HostContentionFired is reserved: no signal sets it, so it reads false —
	// Details' zero value — on every tick.
	HostContentionFired bool
	// LimitedVisibility is the dead-zone annotation, never a State.
	// detailsFor sets it only when quota is absent or non-positive, to
	// !PsiAvailable — so it reads true exactly in the dead zone (no quota to
	// reason about, no PSI to fall back on) and stays false whenever a quota
	// applies.
	LimitedVisibility bool

	// The headroom arithmetic. Neither headroom is clamped: a full box yields
	// a negative number, not a 0. detailsFor sets all four below.
	HostHeadroomCores      float64           // host-headroom's own 60s reduction, in cores: cores − hostBusy − reserve
	HostBusyCoresAvailable bool              // the sample's own readability flag; a ==0 proxy cannot tell unreadable from idle
	AvgHostBusyCores       float64           // the host-busy track's 60s mean
	HeadroomCores          diagnosis.Reading // declared for a future frontend projection; nothing fills it
	CapacityCores          float64           // the quota when set and positive, else LogicalCpus
	ReserveCores           float64           // the reserve subtracted from CapacityCores: limitReserveFraction x quota when limited, else the fixed cpuReserveCores

	// Capability, NOT readability. The healthy message's budget lines are
	// forbidden from reading these three as "the reading succeeded". The three
	// *SignalReady fields below are the readability half, and the two families
	// are never interchangeable. detailsFor sets all three below.
	LimitApplies    bool // true when a positive quota applies (env.Has(HasLimit))
	PressureApplies bool // true when PSI is available on this box (the sample's own PsiAvailable)
	StealApplies    bool // true under virtualization (env.Has(HasVirtualization))

	// ---- append-only from here, in the order each field was added, not a
	// fixed API. detailsFor fills all three below. ----
	//
	// HostHeadroomAvailable is the SECOND field ending "Available" and it puts
	// this struct AT the cap of two this package allows before a family of
	// near-duplicate booleans has to be modelled as a set instead.
	HostHeadroomAvailable bool    // false when the scope is not ScopeHost: withheld, not failed
	LogicalCpus           float64 // the "2" — the CPUs this process may use
	HostCpus              float64 // the "8" — the machine's count, from the per-CPU lines of /proc/stat

	// ---- per-signal readiness. detailsFor fills all three below from
	// Observe's second return. Each is Availability == Ready for that signal —
	// named for what it holds, not the signal name plus "Available", to stay
	// under the two-field cap above. ----
	ThrottleSignalReady bool // true when the throttling signal's readiness this tick is Ready; NoInstrument (bare metal) and NoneReady (a window too thin) both read false
	PressureSignalReady bool // true when the pressure signal's readiness this tick is Ready, set the same way
	StealSignalReady    bool // true when the steal signal's readiness this tick is Ready, set the same way
}
