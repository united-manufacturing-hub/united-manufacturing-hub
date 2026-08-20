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

// causeOf maps one fired signal to a Cause. Each signal names its own cause
// kind. The value is the CURRENT reduction of the instrument that produced the
// latch, read back through Engine.Reduction — not Fired.Value,
// which is stamped at the firing tick and stays constant while the latch holds;
// the recording's cause values are the current windowed number, which moves
// while the latch is held (the throttling ratio decays, the settled headroom
// deepens). The unit comes from the mark pair that judged the instrument, and
// the instrument's own name rides along so the message layer can tell two ways
// of measuring one signal apart without asking the engine again.
func causeOf(engine *diagnosis.Engine[Sample], f diagnosis.Fired) Cause {
	switch f.Identity.Signal {
	case sigThrottling:
		v, _ := engine.Reduction(sigThrottling, instThrottleRatio).Get()
		return Cause{Kind: CauseKindThrottling, Instrument: f.Instrument, Value: v, Unit: Unit(f.Marks.Unit)}
	case sigPressure:
		v, _ := engine.Reduction(sigPressure, instPressureAvg60).Get()
		return Cause{Kind: CauseKindPressure, Instrument: f.Instrument, Value: v, Unit: Unit(f.Marks.Unit)}
	case sigSteal:
		// The p95 is used whenever its window can supply a value, so
		// the cause value follows the same rule: p95 when it is
		// StateValue, the mean before that.
		v, st := engine.Reduction(sigSteal, instStealP95).Get()
		if st != diagnosis.StateValue {
			v, _ = engine.Reduction(sigSteal, instStealMean).Get()
		}
		return Cause{Kind: CauseKindSteal, Instrument: f.Instrument, Value: v, Unit: Unit(f.Marks.Unit)}
	case sigHostCpuFull:
		if f.Instrument == instUsageFraction {
			v, _ := engine.Reduction(sigHostCpuFull, instUsageFraction).Get()
			return Cause{Kind: CauseKindHostCpuFull, Instrument: f.Instrument, Value: v, Unit: Unit(f.Marks.Unit)}
		}
		v, _ := engine.Reduction(sigHostCpuFull, instHostHeadroom).Get()
		return Cause{Kind: CauseKindHostCpuFull, Instrument: f.Instrument, Value: v, Unit: Unit(f.Marks.Unit)}
	case sigContainerLimitFull:
		v, _ := engine.Reduction(sigContainerLimitFull, instLimitHeadroom).Get()
		return Cause{Kind: CauseKindContainerLimitFull, Instrument: f.Instrument, Value: v, Unit: Unit(f.Marks.Unit)}
	default:
		// A signal with no case above names no kind, rather than borrowing the
		// kind of whichever case happened to sit last. Its value is the one
		// stamped at the fire tick, because no case here says which reduction
		// to read instead.
		return Cause{Instrument: f.Instrument, Value: f.Value, Unit: Unit(f.Marks.Unit)}
	}
}

// declaredBlame reads the blame the table declared for one fired signal. A
// refinement narrows the signal it hangs under, so a fired refinement answers
// in its parent's place. Refinements arrive most urgent first, which makes
// index 0 the narrowing that applies.
func declaredBlame(f diagnosis.Fired) int {
	if len(f.Refinements) > 0 {
		return f.Refinements[0].Attribution
	}

	return f.Identity.Attribution
}

// attributionOf names a blame value. A number no row declared is unknown: the
// alternative is an empty Attribution, which reads as a value rather than as
// the absence of one.
func attributionOf(blame int) Attribution {
	switch blame {
	case blameHost:
		return AttributionHost
	case blameContainer:
		return AttributionContainer
	default:
		return AttributionUnknown
	}
}
