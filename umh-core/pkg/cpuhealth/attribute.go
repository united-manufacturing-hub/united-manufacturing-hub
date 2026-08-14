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

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// causeOf maps one folded Fired to a Cause. The saturation family always maps
// to CauseKindSaturation. The value is the CURRENT reduction of the arm that
// produced the latch, read back through Engine.Reduction — not Fired.Value,
// which is stamped at the firing tick and stays constant while the latch holds;
// the recording's cause values are the current windowed number, which moves
// while the latch is held (the throttling ratio decays, the settled headroom
// deepens). The unit comes from the mark pair that judged the arm.
func causeOf(engine *diagnosis.Engine[Sample], f diagnosis.Fired) Cause {
	switch f.Identity.Signal {
	case sigThrottling:
		v, _ := engine.Reduction(sigThrottling, instThrottleRatio).Get()
		return Cause{Kind: CauseKindThrottling, Value: v, Unit: Unit(f.Marks.Unit)}
	case sigPressure:
		v, _ := engine.Reduction(sigPressure, instPressureAvg60).Get()
		return Cause{Kind: CauseKindPressure, Value: v, Unit: Unit(f.Marks.Unit)}
	case sigSteal:
		// Selection prefers the p95 whenever its window can supply a value, so
		// the cause value follows the same preference: p95 when it is
		// StateValue, the mean before that.
		v, st := engine.Reduction(sigSteal, instStealP95).Get()
		if st != diagnosis.StateValue {
			v, _ = engine.Reduction(sigSteal, instStealMean).Get()
		}
		return Cause{Kind: CauseKindSteal, Value: v, Unit: Unit(f.Marks.Unit)}
	case sigSaturation:
		if f.Marks.Unit == "fraction" {
			v, _ := engine.Reduction(sigSaturation, instUsageFraction).Get()
			return Cause{Kind: CauseKindSaturation, Value: v, Unit: Unit(f.Marks.Unit)}
		}
		v, _ := engine.Reduction(sigSaturation, instHostHeadroom).Get()
		return Cause{Kind: CauseKindSaturation, Value: v, Unit: Unit(f.Marks.Unit)}
	default: // sigLimitSaturation
		v, _ := engine.Reduction(sigLimitSaturation, instLimitHeadroom).Get()
		return Cause{Kind: CauseKindSaturation, Value: v, Unit: Unit(f.Marks.Unit)}
	}
}

// attributeFor derives the attribution from the dominant cause. The saturation
// kind is ambiguous — it is the fold's one cause — so Decide resolves it with
// the survivor's arm: host-full attributes by the split, the limit arm and the
// no-host-stats fallback are internal (the split cannot run for them).
func attributeFor(dominant Cause, survivor *diagnosis.Fired, splitHost bool) Attribution {
	switch dominant.Kind {
	case CauseKindSteal, CauseKindHostContention:
		return AttributionHost
	case CauseKindThrottling, CauseKindPressure:
		return AttributionUnknown
	case CauseKindSaturation:
		// host-full is the survivor from the "saturation" signal whose unit is
		// "cores"; the limit arm and the no-host-stats fallback are internal.
		hostFull := survivor != nil && survivor.Identity.Signal == sigSaturation && survivor.Marks.Unit == "cores"
		if hostFull && splitHost {
			return AttributionHost
		}
		return AttributionUnknown
	}
	return AttributionUnknown
}
