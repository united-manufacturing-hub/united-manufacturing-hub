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

// The throttling signal answers whether the kernel is capping us against our
// own quota; it is the one CPU signal that reads running totals.

package cpuhealth

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// This pair is a ratio on a 0..1 scale with capacity 1.0, marking the
// throttling signal's fire and clear thresholds.
var throttleMarks = diagnosis.Marks{
	Fire: diagnosis.Mark{At: 0.05}, Clear: diagnosis.Mark{At: 0.03},
	Polarity: diagnosis.HigherIsWorse, Unit: "ratio", Worst: 1.0,
}

// throttlingSignal is "is the kernel capping us against our own quota?" It is
// the one instrument reading running totals, hence the only Counter in the
// table.
func throttlingSignal() diagnosis.Signal[Sample] {
	return diagnosis.Signal[Sample]{
		Name:            sigThrottling,
		Tier:            tierStarvation,
		DemoteSpan:      60 * time.Second,
		ReleaseOnAbsent: true,
		Instruments: []diagnosis.Instrument[Sample]{{
			Name:     instThrottleRatio,
			Requires: []diagnosis.Capability{HasLimit},
			Extract:  func(s Sample) diagnosis.Reading { return s.NrThrottled },
			Against:  func(s Sample) diagnosis.Reading { return s.NrPeriods },
			Span:     60 * time.Second,
			Red:      diagnosis.DeltaRatio,
			// Both cpu.stat counters are running totals: a fall is a cgroup
			// reset, and a delta across it is arithmetic on two origins. This
			// is the only CPU instrument that declares it.
			Counter: true,
			Marks:   throttleMarks,
		}},
	}
}
