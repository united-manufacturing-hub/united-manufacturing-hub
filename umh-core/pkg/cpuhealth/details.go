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
// carry. buildDetails is its sole producer; buildVerdict returns only the
// Verdict. Flat, like Sample, because one signal already spans more than one
// of the groups below — throttle is ThrottleRatio and ThrottleSignalReady — so
// splitting by signal or by group would splinter the other.
type Details struct {
	// The metrics. ThrottleRatio, PressureAvg60 and StealP95 are filled every
	// tick, independent of their latch's fired state, so the number stays
	// observable even when the latch has not fired.
	P95UsageCores diagnosis.Reading `json:"p95UsageCores"` // the usage-cores measurement's 60s p95; 0 is a legitimate usage figure, so below the reduction's floor of 20 readings it answers absent, never a measured zero
	ThrottleRatio float64           `json:"throttleRatio"` // 60s nr_throttled/nr_periods delta; buildDetails discards State, so absent or untrusted reads 0
	PressureAvg60 float64           `json:"pressureAvg60"` // PSI avg60; NaN/±Inf are never stored (rejected at window ingest), and buildDetails discards State, so absent reads 0
	StealP95      float64           `json:"stealP95"`      // 60s p95; buildDetails discards State, so bare metal or below the reduction's floor of 20 reads 0

	// Observability only: neither of these two changes a verdict. Both are
	// filled every tick from their own signal's reduction.
	AvgUsageFraction float64 `json:"avgUsageFraction"` // usage-fraction's own 60s reduction — the number the host-cpu-full latch was judged on, not the usage-cores measurement divided by anything, so a mid-run core-count change cannot split them
	AvgUsageCores    float64 `json:"avgUsageCores"`    // ABSOLUTE cores; limit-mode headroom and the wire's avgMCpu read this one value — the usage-cores measurement's 60s mean

	// The headroom arithmetic. Neither headroom is clamped: a full box yields
	// a negative number, not a 0. buildDetails sets all four of them.
	HostHeadroomCores float64 `json:"hostHeadroomCores"` // host-headroom's own 60s reduction, in cores: cores − hostBusy − reserve
	AvgHostBusyCores  float64 `json:"avgHostBusyCores"`  // the host-busy measurement's 60s mean
	CapacityCores     float64 `json:"capacityCores"`     // the quota when set and positive, else LogicalCpus
	ReserveCores      float64 `json:"reserveCores"`      // the reserve subtracted from CapacityCores: limitReserveFraction x quota when limited, else the fixed cpuReserveCores

	// The two core counts. They differ whenever this container may use fewer
	// CPUs than the machine has.
	LogicalCpus float64 `json:"logicalCpus"` // the "2" — the CPUs this process may use
	HostCpus    float64 `json:"hostCpus"`    // the "8" — the machine's count, from the per-CPU lines of /proc/stat

	UsageRingActive bool `json:"usageRingActive"` // usage-cores reduced to StateValue — the healthy headline's LIMIT-mode floor gate
	// HostBusyRingActive: host-busy reduced to StateValue — the healthy
	// headline's NO-LIMIT floor gate. Two measurements, two floors: an outage
	// can leave one thin while the other fills.
	HostBusyRingActive bool `json:"hostBusyRingActive"`

	// LimitedVisibility is an annotation on a healthy verdict, never a State.
	// buildDetails sets it only when quota is absent or non-positive, to
	// !PsiAvailable — so it reads true exactly where there is no quota to
	// reason about and no PSI to fall back on, and false whenever a quota
	// applies.
	LimitedVisibility bool `json:"limitedVisibility"`

	HostBusyCoresAvailable bool `json:"hostBusyCoresAvailable"` // the sample's own readability flag; a ==0 proxy cannot tell unreadable from idle

	// Capability, NOT readability. The healthy message's budget lines are
	// forbidden from reading these three as "the reading succeeded". The three
	// *SignalReady fields below are the readability half, and the two families
	// are never interchangeable. buildDetails sets all three below.
	LimitApplies    bool `json:"limitApplies"`    // true when a positive quota applies (env.Has(HasLimit))
	PressureApplies bool `json:"pressureApplies"` // true when PSI is available on this box (the sample's own PsiAvailable)
	StealApplies    bool `json:"stealApplies"`    // true under virtualization (env.Has(HasVirtualization))

	// HostHeadroomAvailable is the SECOND field ending "Available" and it puts
	// this struct AT the cap of two this package allows before a family of
	// near-duplicate booleans has to be modelled as a set instead.
	HostHeadroomAvailable bool `json:"hostHeadroomAvailable"` // false when the scope is not ScopeHost: withheld, not failed

	// ---- per-signal readiness. buildDetails fills all three below from
	// Observe's second return. Each is Availability == Ready for that signal —
	// named for what it holds, not the signal name plus "Available", to stay
	// under the two-field cap above. ----
	ThrottleSignalReady bool `json:"throttleSignalReady"` // true when the throttling signal's readiness this tick is Ready; NoInstrument (bare metal) and NoneReady (a window too thin) both read false
	PressureSignalReady bool `json:"pressureSignalReady"` // true when the pressure signal's readiness this tick is Ready, set the same way
	StealSignalReady    bool `json:"stealSignalReady"`    // true when the steal signal's readiness this tick is Ready, set the same way
}
