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
		// Read only by buildDetails, which puts each one's reduced value and
		// window state on Details for the wire and the healthy message.
		Measurements: []diagnosis.Measurement[Sample]{
			{
				Name:      measurementHostBusy,
				Extract:   func(s Sample) diagnosis.Reading { return s.HostBusy },
				Span:      60 * time.Second,
				Reduction: diagnosis.Mean, // minimum 2 — Mean's own sample floor
			},
			{
				Name:      measurementUsageCores,
				Extract:   func(s Sample) diagnosis.Reading { return s.UsageCores },
				Span:      60 * time.Second,
				Reduction: diagnosis.Mean,
			},
			{
				Name:      measurementUsageCoresP95,
				Extract:   func(s Sample) diagnosis.Reading { return s.UsageCores },
				Span:      60 * time.Second,
				Reduction: diagnosis.P95, // minimum 20 — P95's own sample floor
				//
				// No sixty-second p99 row can sit beside this one: a p99 needs
				// a hundred readings, and a sixty-second window at this table's
				// one-second interval holds sixty-one, so NewEngine would
				// refuse the whole table — a p99 row here would need a span of
				// ninety-nine seconds or more at this interval.
			},
		},
		Signals: []diagnosis.Signal[Sample]{
			stealSignal(),
			throttlingSignal(),
			pressureSignal(),
		},
	}
	if hostCpuFullDeclared(cores) {
		t.Signals = append(t.Signals, hostCpuFullSignal(cores))
	}
	if containerLimitFullDeclared(quota) {
		t.Signals = append(t.Signals, containerLimitFullSignal(quota))
	}
	return t
}

// hostCpuFullDeclared reports whether the table holds the machine-full signal.
// It does only when the core count was readable: cores <= 0 means the cgroup's
// cpuset could not be read, so there is no capacity to be full against. Why a
// machine-wide busy time may be subtracted from cores, which is a
// container-scoped count: see host_source.go's header.
//
// message.go reads this too, spelling the condition once but not making the
// two sides agree: the table is built from the startup snapshot, the message
// runs on the current tick. A cpuset that failed at startup and reads later
// makes this true while the engine holds no signal (ENG-5752).
func hostCpuFullDeclared(cores float64) bool {
	return cores > 0
}

// containerLimitFullDeclared reports whether the table holds the
// out-of-our-own-limit signal. It does only when a positive quota exists: with
// no limit there is nothing to saturate, and Fire{At: 0} against
// Clear{At: 0.05 x 0} is a pair NewEngine rejects.
//
// hostCpuFullDeclared's header says why this is a function rather than an
// inline comparison.
func containerLimitFullDeclared(quota float64) bool {
	return quota > 0
}
