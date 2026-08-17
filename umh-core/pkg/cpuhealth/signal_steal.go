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

// The steal signal answers whether something outside this box is taking our
// CPU; it carries a percentile arm and a mean arm for one question.

package cpuhealth

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// This pair is a ratio on a 0..1 scale with capacity 1.0, and it is shared by
// both steal arms: the mean fallback reuses the p95 bar, which is what lets the
// mean stand in for a percentile at two samples without a second threshold.
var stealMarks = diagnosis.Marks{
	Fire: diagnosis.Mark{At: 0.10}, Clear: diagnosis.Mark{At: 0.06},
	Polarity: diagnosis.HigherIsWorse, Unit: "ratio", Worst: 1.0,
}

// stealSignal is "is something outside this box taking our CPU?" It carries two
// instruments that answer the same question — a percentile once the ring holds
// twenty entries, and a mean before that — so steal is judgeable two seconds
// after a start instead of twenty.
func stealSignal() diagnosis.Signal[Sample] {
	return diagnosis.Signal[Sample]{
		Name:            sigSteal,
		Tier:            tierStarvation,
		External:        true, // Rank's third tie-break; not what sets attribution
		DemoteSpan:      60 * time.Second,
		ReleaseOnAbsent: true,
		Instruments: []diagnosis.Instrument[Sample]{
			{
				Name:     instStealP95,
				Requires: []diagnosis.Capability{HasVirtualization},
				Extract:  func(s Sample) diagnosis.Reading { return s.Steal },
				Span:     60 * time.Second,
				Red:      diagnosis.P95, // the reduction declares its own minimum: 20
				Marks:    stealMarks,
			},
			{
				Name:     instStealMean,
				Requires: []diagnosis.Capability{HasVirtualization},
				Extract:  func(s Sample) diagnosis.Reading { return s.Steal },
				Span:     60 * time.Second,
				Red:      diagnosis.Mean, // minimum 2
				Marks:    stealMarks,     // the mean fallback shares the p95 bar:
				// the question and its unit are the same, only the minimum
				// differs. Do not "fix" this arm back to p95, and do not add a
				// second threshold: one 0.9 spike firing the mean at n=2 is the
				// accepted design, not the defect.
				//
				// Counter stays false, on both steal arms. A steal fraction that
				// falls has fallen. Declare it a counter and the window restarts
				// on the first dip, never reaches p95's twenty samples, and the
				// handover, the mean fallback and the two-second readiness all die silently
				// green — steal simply never fires.
			},
		},
	}
}
