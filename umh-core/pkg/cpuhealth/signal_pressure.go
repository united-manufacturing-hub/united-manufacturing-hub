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

// The pressure signal answers whether our tasks are waiting for a core, from
// PSI's own 60-second average. PSI is the Linux kernel's pressure stall
// information: the share of wall-clock time in which tasks were runnable and
// waiting on a resource. The kernel defines it, and the avg60 field this
// signal reads, at https://docs.kernel.org/accounting/psi.html.

package cpuhealth

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// This pair is a ratio on a 0..1 scale with capacity 1.0, marking the pressure
// signal's fire and clear thresholds. It is the one home for both numbers: the
// engine judges against it and the Technical Details table states it, so the
// threshold the customer reads is the threshold that fires.
var pressureMarks = diagnosis.Marks{
	Fire: diagnosis.Mark{At: 0.20}, Clear: diagnosis.Mark{At: 0.12},
	Polarity: diagnosis.HigherIsWorse, Unit: unitRatio, Worst: 1.0,
}

// pressureSignal is "are our tasks waiting for a core?" PSI's avg60 is already
// a 60-second average, so the reduction is Last and the instrument can fire on
// tick 0.
func pressureSignal() diagnosis.Signal[Sample] {
	return diagnosis.Signal[Sample]{
		Name: signalPressure,
		// Stalled time names no side: our tasks can wait because the machine is
		// busy, or because we asked for more CPU than we may use, and the
		// pressure number alone does not separate those.
		Attribution:     blameUnknown,
		Tier:            tierStarvation,
		DemoteSpan:      60 * time.Second,
		ReleaseOnAbsent: true,
		Instruments: []diagnosis.Instrument[Sample]{{
			Measurement: diagnosis.Measurement[Sample]{
				Name:      instrumentPressureAvg60,
				Requires:  []diagnosis.Capability{HasPressureStats},
				Extract:   func(s Sample) diagnosis.Reading { return s.Pressure },
				Span:      60 * time.Second,
				Reduction: diagnosis.Last,
			},
			Marks: pressureMarks,
		}},
	}
}
