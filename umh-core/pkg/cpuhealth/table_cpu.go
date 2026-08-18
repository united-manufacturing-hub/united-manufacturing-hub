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

// The CPU table builder: the function that assembles the five signals and two
// tracks table.go's entry points hand to diagnosis.NewEngine.

package cpuhealth

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// cpuTable declares five signals — throttling, pressure, steal, saturation and
// limit-saturation — and two tracks, host-busy and usage-cores.
//
// cpuTable is the CPU declaration, built by a function because two marks and
// one capacity are denominated in quantities that vary per box: the quota and
// the logical CPU count. Both arguments are startup facts, cached across
// ticks, exactly as a Capability is.
//
// quota does not need to be a Reading: HasLimit is present exactly when
// cpu.max names a positive quota, which is the same read that supplies the
// number.
//
// cpuTable omits the limit-saturation signal entirely when quota is not
// positive, and the saturation signal entirely when the core count was never
// readable (cores <= 0) — appending each conditionally is the only
// arrangement that constructs, since leaving either row in place and
// unreached through Requires is not enough (see limitSaturationSignal and
// saturationSignal for why each row's own Marks force the omission).
func cpuTable(cores, quota float64) diagnosis.Table[Sample] {
	t := diagnosis.Table[Sample]{
		Interval: time.Second,
		// The two folds no instrument produces. Both are declared on every box,
		// with no Requires and no quota in sight, which is the whole point:
		// attribution needs both 60s means everywhere, and the instruments that
		// touch these series hold something else.
		Tracks: []diagnosis.Track[Sample]{
			{
				// host-headroom's window holds cores − hostBusy − reserve AND is
				// Unknown() off ScopeHost, so inverting it loses the term on
				// exactly the affinity boxes whose host/container split is still
				// valid.
				Name:      trackHostBusy,
				Extract:   func(s Sample) diagnosis.Reading { return s.HostBusy },
				Span:      60 * time.Second,
				Reduction: diagnosis.Mean, // minimum 2 — Mean's own sample floor
			},
			{
				// limit-headroom's window holds quota − usage − 0.10 × quota and
				// does not exist at all when cpuTable omits limit-saturation,
				// which is every box with no positive quota.
				Name:      trackUsageCores,
				Extract:   func(s Sample) diagnosis.Reading { return s.UsageCores },
				Span:      60 * time.Second,
				Reduction: diagnosis.Mean,
			},
		},
		Signals: []diagnosis.Signal[Sample]{
			// Requires: HasLimit, the same gate limitSaturationSignal has below —
			// but throttling's marks (0.05/0.03) are fixed ratios that don't scale
			// with the quota, so it can sit here unconditionally instead of
			// needing the conditional append limitSaturationSignal gets.
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
