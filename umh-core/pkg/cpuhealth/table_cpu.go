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

// The CPU table builder: the function that assembles the signals and
// measurements table.go's entry points hand to diagnosis.NewEngine.

package cpuhealth

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// cpuTable declares the CPU signals and measurements. Add a row here and the
// engine picks it up.
//
// It is a function rather than a value because the rows below are denominated
// in quantities that vary per box: the quota and the logical CPU count.
func cpuTable(cores, quota float64) diagnosis.Table[Sample] {
	t := diagnosis.Table[Sample]{
		Interval: time.Second,
		// Read only by detailsFor, which puts each one's mean and window state
		// on Details for the wire and the healthy message.
		Measurements: []diagnosis.Measurement[Sample]{
			{
				Name:      measHostBusy,
				Extract:   func(s Sample) diagnosis.Reading { return s.HostBusy },
				Span:      60 * time.Second,
				Reduction: diagnosis.Mean, // minimum 2 — Mean's own sample floor
			},
			{
				Name:      measUsageCores,
				Extract:   func(s Sample) diagnosis.Reading { return s.UsageCores },
				Span:      60 * time.Second,
				Reduction: diagnosis.Mean,
			},
		},
		Signals: []diagnosis.Signal[Sample]{
			stealSignal(),
			throttlingSignal(),
			pressureSignal(),
		},
	}
	// Only when the core count was readable. cores <= 0 means both /proc/cpuinfo
	// and the cpuset failed, so there is no capacity to be full against.
	if cores > 0 {
		t.Signals = append(t.Signals, hostCpuFullSignal(cores))
	}
	// Only when a positive quota exists; with no limit there is nothing to saturate.
	if quota > 0 {
		t.Signals = append(t.Signals, containerLimitFullSignal(quota))
	}
	return t
}
