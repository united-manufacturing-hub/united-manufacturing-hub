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

// Turning a fired signal into the cause a customer is shown, and attributing it.
// Attribution in verdict.go names the classes.

package cpuhealth

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// causeOf maps one chosen Fired to a Cause. The saturation family always maps
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
		// The p95 is used whenever its window can supply a value, so
		// the cause value follows the same rule: p95 when it is
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

// attributeFor derives the attribution from the dominant cause. Throttling is
// the kernel capping this container against its own quota, so it is a container
// cause whatever the host is doing. Pressure names no side: it counts stalled
// time without saying whose load caused the stall.
//
// The saturation kind is ambiguous — it is the one cause kept for the whole
// family — so Decide resolves it with the survivor's arm. The limit arm is this
// container spending its own budget. The no-host-stats fallback has no host
// reading to compare against. Host-full is the only arm the split can settle,
// and it needs two trusted 60s means to settle it.
func attributeFor(dominant Cause, survivor *diagnosis.Fired, split attributionSplit) Attribution {
	switch dominant.Kind {
	case CauseKindSteal, CauseKindHostContention:
		return AttributionHost
	case CauseKindThrottling:
		return AttributionContainer
	case CauseKindPressure:
		return AttributionUnknown
	case CauseKindSaturation:
		if survivor == nil {
			return AttributionUnknown
		}
		switch saturationArmOf(*survivor) {
		case hostFullArm:
			if split.HostBusyState != diagnosis.StateValue || split.OurUsageState != diagnosis.StateValue {
				return AttributionUnknown
			}
			// HostDominates compares strictly greater — hostBusyMean >
			// 2*ourUsageMean — so a host share exactly double our own is ours.
			if split.HostDominates {
				return AttributionHost
			}
			return AttributionContainer
		case limitArm:
			return AttributionContainer
		}
		return AttributionUnknown
	}
	return AttributionUnknown
}
