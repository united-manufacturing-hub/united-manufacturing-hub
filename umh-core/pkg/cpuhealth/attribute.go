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

// describeCause maps one fired signal to a Cause. Each signal names its own cause
// kind. The value is the CURRENT reduction of the instrument that produced the
// latch, read back through Engine.Reduction — not Fired.Value,
// which is stamped at the firing tick and stays constant while the latch holds;
// the recording's cause values are the current windowed number, which moves
// while the latch is held (the throttling ratio decays, the settled headroom
// deepens). The unit comes from the mark pair that judged the instrument, and
// the instrument's own name rides along so the message layer can tell two ways
// of measuring one signal apart without asking the engine again.
//
// Which instrument to read is Fired.Instrument and nothing else, for every
// signal. A signal answered two ways therefore reports the arm its episode
// fired on, even after the other arm's window has matured and selection has
// moved to it. Steal is where that is visible: the mean fires within seconds of
// a start, the percentile takes twenty samples, and both arms share one mark
// pair, so nothing re-stamps the latch and the episode reports the mean until
// it releases.
func describeCause(engine *diagnosis.Engine[Sample], f diagnosis.Fired) Cause {
	// Rank does not flatten refinements, so every Fired reaching here is a
	// top-level signal, whose window path is its own name.
	v, _ := engine.Reduction(f.Identity.Signal, f.Instrument).Get()
	cause := Cause{Instrument: f.Instrument, Value: v, Unit: Unit(f.Marks.Unit), Attribution: nameAttribution(declaredBlame(f))}

	// One arm per signal and no default arm: a signal named nowhere below
	// names no kind, rather than borrowing the kind of whichever arm happens to
	// sit last.
	switch f.Identity.Signal {
	case signalThrottling:
		cause.Kind = CauseKindThrottling
	case signalPressure:
		cause.Kind = CauseKindPressure
	case signalSteal:
		cause.Kind = CauseKindSteal
	case signalHostCpuFull:
		cause.Kind = CauseKindHostCpuFull
	case signalContainerLimitFull:
		cause.Kind = CauseKindContainerLimitFull
	}

	return cause
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

// nameAttribution names a blame value. A number no row declared is unknown: the
// alternative is an empty Attribution, which reads as a value rather than as
// the absence of one.
func nameAttribution(blame int) Attribution {
	switch blame {
	case blameHost:
		return AttributionHost
	case blameContainer:
		return AttributionContainer
	default:
		return AttributionUnknown
	}
}
