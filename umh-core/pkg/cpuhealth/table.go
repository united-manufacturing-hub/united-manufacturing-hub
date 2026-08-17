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

// The CPU declaration: which signals exist, what each one measures, where its
// thresholds sit, and the constructor that builds an engine from it. Each
// signal's declaration and the shared mark pairs now live in a file of their
// own (signal_*.go); this file keeps the names, the tiers, and the assembly.

package cpuhealth

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// The signal names. Every window CPU reads back is named by one of these, in
// the table that declares it and at the call site that reads it. An unnamed
// pair reduces to StateAbsent forever, so a string literal at either end is a
// typo that reads as a permanent absence — a constant makes it a compile error
// instead.
const (
	sigSaturation      = "saturation"
	sigLimitSaturation = "limit-saturation"
	sigThrottling      = "throttling"
	sigSteal           = "steal"
	sigPressure        = "pressure"

	instHostHeadroom  = "host-headroom"
	instUsageFraction = "usage-fraction"
	instLimitHeadroom = "limit-headroom"
	instThrottleRatio = "throttle-ratio"
	instStealP95      = "steal-p95"
	instStealMean     = "steal-mean"
	instPressureAvg60 = "pressure-avg60"

	trackHostBusy   = "host-busy"
	trackUsageCores = "usage-cores"
)

// The tiers. Starvation outranks saturation regardless of severity, because
// "you are being throttled" is actionable and "you are busy" is not. The words
// are CPU's; diagnosis only ever sees the number.
const (
	tierStarvation = 0
	tierSaturation = 1
)

// NewEngine builds the engine once, at construction. Both arguments are startup
// facts, cached across ticks, exactly as a Capability is; a quota that changes
// at runtime needs a rebuilt table, and rebuilding drops every window.
func NewEngine(cores, quota float64) (*diagnosis.Engine[Sample], error) {
	return diagnosis.NewEngine(Table(cores, quota))
}

// Table builds the CPU signal table for a caller that needs the Signal values
// themselves: a worker outside this package walks them to ask Engine.Select for
// per-signal Availability, and cpuTable is unexported.
//
// What this guarantees is narrow and exact. NewEngine delegates through Table,
// so the table a caller walks and the table that caller's engine was built from
// come from the same call — a signal the worker polls is a signal the engine
// keyed windows under. It is not a package-wide guarantee: RunSuite still builds
// its own table from cpuTable directly, so "nothing in cpuhealth can drift" is
// not a claim this function is in a position to make.
func Table(cores, quota float64) diagnosis.Table[Sample] {
	return cpuTable(cores, quota)
}

// cpuTable is the CPU declaration, built by a function because two marks and
// one capacity are denominated in quantities that vary per box: the quota and
// the logical CPU count. Both arguments are startup facts, cached across ticks,
// exactly as a Capability is. host-headroom's capacity is the fixed reserve
// rather than the core count, so only the limit arm's capacity scales.
//
// 🔥 quota is a float64 and not a Reading. Marks.Worst and Mark.At are both
// float64, so a table cannot be built from an absence — and it does not need to
// be: HasLimit is present exactly when cpu.max names a positive quota, which is
// the same read that supplies the number.
//
// When there is no positive quota, cpuTable omits the limit-saturation signal
// entirely. It is not enough to leave the row unreachable through Requires: at
// quota = 0 the pair is Fire{At: 0} against Clear{At: 0.05 × 0}, which
// NewEngine refuses under LowerIsWorse — so a box with no limit could not build
// a table at all. Omitting the row is the only arrangement that constructs.
// throttling stays, Requires: HasLimit and all, because its marks are ratios
// and do not scale with the quota.
//
// The same conditional omission applies to the core count: a box whose core
// count was never readable (cores <= 0) declares no saturation row at all —
// there is no capacity to be full, only a count that was never taken.
func cpuTable(cores, quota float64) diagnosis.Table[Sample] {
	t := diagnosis.Table[Sample]{
		Interval: time.Second,
		// The two folds no instrument produces. Both are declared on every box,
		// with no Requires and no quota in sight, which is the whole point:
		// attribution needs both 60s means everywhere, and the instruments that
		// touch these series hold something else.
		//
		// host-busy: host-headroom's window holds cores − hostBusy − reserve AND
		// is Unknown() off ScopeHost, so inverting it loses the term on exactly
		// the affinity boxes whose host/container split is still valid.
		//
		// usage-cores: limit-headroom's window holds quota − usage − 0.10 × quota
		// and does not exist at all when cpuTable omits limit-saturation, which
		// is every box with no positive quota.
		Tracks: []diagnosis.Track[Sample]{
			{
				Name:    trackHostBusy,
				Extract: func(s Sample) diagnosis.Reading { return s.HostBusy },
				Span:    60 * time.Second,
				Red:     diagnosis.Mean, // minimum 2 — the parked 2-sample ring floor
			},
			{
				Name:    trackUsageCores,
				Extract: func(s Sample) diagnosis.Reading { return s.UsageCores },
				Span:    60 * time.Second,
				Red:     diagnosis.Mean,
			},
		},
		Signals: []diagnosis.Signal[Sample]{
			throttlingSignal(),
			pressureSignal(),
			stealSignal(),
		},
	}
	if cores > 0 {
		t.Signals = append(t.Signals, saturationSignal(cores))
	}
	if quota > 0 {
		t.Signals = append(t.Signals, limitSaturationSignal(quota))
	}
	return t
}
