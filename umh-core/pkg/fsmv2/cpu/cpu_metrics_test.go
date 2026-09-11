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

package fsmv2cpu

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// richSample stages a tick whose Details carry a different number in every
// published field, so a mapping that crossed two fields moves at least one of
// them.
func richSample() cpuhealth.Sample {
	return cpuhealth.Sample{
		Timestamp:    time.Now(),
		Quota:        diagnosis.Known(2),
		LogicalCpus:  diagnosis.Known(4),
		HostCpus:     diagnosis.Known(8),
		NrPeriods:    diagnosis.Known(1),
		NrThrottled:  diagnosis.Known(0),
		UsageCores:   diagnosis.Known(1.5),
		Pressure:     diagnosis.Known(0.9),
		Steal:        diagnosis.Known(0.02),
		HostBusy:     diagnosis.Known(0.5),
		Virtualized:  true,
		PsiAvailable: true,
		CpuScope:     cpuhealth.ScopeHost,
	}
}

var _ = Describe("the CPU worker publishes its evidence as worker gauges", func() {
	It("records every measurement under its own gauge name, with the value Decide produced", func() {
		d := newDeps(fixedSampler(richSample()), 4, 2)

		status, err := Poll(context.Background(), d, CPUConfig{})
		Expect(err).NotTo(HaveOccurred())

		want := map[deps.GaugeName]float64{
			deps.GaugeCPUAvgUsageCores:     status.Details.AvgUsageCores,
			deps.GaugeCPUAvgUsageFraction:  status.Details.AvgUsageFraction,
			deps.GaugeCPUThrottleRatio:     status.Details.ThrottleRatio,
			deps.GaugeCPUPressureAvg60:     status.Details.PressureAvg60,
			deps.GaugeCPUHostHeadroomCores: status.Details.HostHeadroomCores,
			deps.GaugeCPUAvgHostBusyCores:  status.Details.AvgHostBusyCores,
			deps.GaugeCPUCapacityCores:     status.Details.CapacityCores,
			deps.GaugeCPUReserveCores:      status.Details.ReserveCores,
			deps.GaugeCPUHostCpus:          status.Details.HostCpus,
		}

		seen := make(map[float64]deps.GaugeName, len(want))
		for name, v := range want {
			Expect(seen).NotTo(HaveKey(v),
				"fixture must stage a distinct value per field, or a crossed mapping passes: %s and %s both hold %v",
				name, seen[v], v)
			seen[v] = name
		}

		gauges := d.MetricsRecorder().Drain().Gauges

		for name, v := range want {
			Expect(gauges).To(HaveKeyWithValue(string(name), v),
				"gauge %s must carry Details' own value %v", name, v)
		}
	})

	It("records each readability flag as 1 or 0, so a consumer can tell an unready signal from a quiet one", func() {
		d := newDeps(fixedSampler(richSample()), 4, 2)

		status, err := Poll(context.Background(), d, CPUConfig{})
		Expect(err).NotTo(HaveOccurred())

		flags := map[deps.GaugeName]bool{
			deps.GaugeCPUUsageRingActive:       status.Details.UsageRingActive,
			deps.GaugeCPUHostBusyRingActive:    status.Details.HostBusyRingActive,
			deps.GaugeCPUHostHeadroomAvailable: status.Details.HostHeadroomAvailable,
			deps.GaugeCPUThrottleSignalReady:   status.Details.ThrottleSignalReady,
			deps.GaugeCPUPressureSignalReady:   status.Details.PressureSignalReady,
		}

		var trues, falses int

		for _, v := range flags {
			if v {
				trues++
			} else {
				falses++
			}
		}

		Expect(trues).To(BeNumerically(">", 0), "this spec needs at least one ready signal to have anything to distinguish")
		Expect(falses).To(BeNumerically(">", 0), "and at least one unready signal")

		gauges := d.MetricsRecorder().Drain().Gauges

		for name, want := range flags {
			expected := 0.0
			if want {
				expected = 1.0
			}

			Expect(gauges).To(HaveKeyWithValue(string(name), expected),
				"flag %s must publish %v as %v", name, want, expected)
		}
	})

	It("publishes an unready signal as a zero measurement beside a zero flag, rather than omitting it", func() {
		// The exporter creates gauges lazily and never deletes one, so a skipped
		// SetGauge leaves the previous value being scraped as though it were
		// current.
		d := newDeps(fixedSampler(richSample()), 4, 2)

		status, err := Poll(context.Background(), d, CPUConfig{})
		Expect(err).NotTo(HaveOccurred())
		Expect(status.Details.ThrottleSignalReady).To(BeFalse(),
			"this spec needs an unready signal; the throttle ratio is a delta between two counter reads, which one tick cannot produce")
		Expect(status.Details.ThrottleRatio).To(Equal(0.0))

		gauges := d.MetricsRecorder().Drain().Gauges

		Expect(gauges).To(HaveKeyWithValue(string(deps.GaugeCPUThrottleRatio), 0.0),
			"the measurement is published even though it was not readable")
		Expect(gauges).To(HaveKeyWithValue(string(deps.GaugeCPUThrottleSignalReady), 0.0),
			"and the flag beside it says the zero is not a measurement")
	})

	It("records nothing on a tick that could not measure", func() {
		d := newDeps(stubSampler{read: func(context.Context) (cpuhealth.Sample, error) {
			return cpuhealth.Sample{}, context.DeadlineExceeded
		}}, 4, 2)

		_, err := Poll(context.Background(), d, CPUConfig{})
		Expect(err).To(HaveOccurred())

		// A statement about the recorder, not about what a scrape sees: the
		// collector re-publishes the previous values from CSE on a failed poll.
		Expect(d.MetricsRecorder().Drain().Gauges).To(BeEmpty(),
			"a failed read publishes no gauge rather than a zero-valued measurement")
	})
})
