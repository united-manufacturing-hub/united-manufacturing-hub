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

// The CPU declaration's names and entry points: which signals exist, where
// each one ranks, and the constructor that builds an engine from them. Each
// signal's declaration and the shared mark pairs live in a file of their own
// (signal_*.go); the builder that assembles them into a table lives in
// table_cpu.go. This file keeps the names, the tiers, and the entry points.

package cpuhealth

import (
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

	// The two refinements of saturation. Each names its single instrument the
	// same as itself: a refinement is one narrowing, read one way.
	refHostShare      = "host-share"
	refContainerShare = "container-share"
)

// The tiers. Every signal declares one. Starvation means something outside
// this container is taking CPU away from it: the throttling, pressure and
// steal signals. Saturation means the CPU is being used up: the saturation
// and limit-saturation signals. Starvation outranks saturation regardless of
// severity, because "you are being throttled" is actionable and "you are
// busy" is not. The words are CPU's; diagnosis only ever sees the number.
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
// per-signal Availability, and cpuTable is unexported. Build the engine from the
// returned value rather than from a second Table call — two calls with different
// cores or quota key their windows differently.
func Table(cores, quota float64) diagnosis.Table[Sample] {
	return cpuTable(cores, quota)
}
