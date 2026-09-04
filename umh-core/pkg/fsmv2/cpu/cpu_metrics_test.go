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

// richSample fires pressure and reports a host-busy rate on its first tick, so
// Details carries several mutually distinct non-zero numbers without any window
// warm-up. The cumulative counters (UsageUsec, NrThrottled, Steal) need a second
// read to yield a rate, so their reductions stay 0 here; the specs below assert
// those gauges by presence, and a separate spec covers the unready contract.
func richSample() cpuhealth.Sample {
	return cpuhealth.Sample{
		Timestamp:    time.Now(),
		Quota:        diagnosis.Known(2),
		LogicalCpus:  diagnosis.Known(4),
		HostCpus:     diagnosis.Known(8),
		NrPeriods:    diagnosis.Known(1),
		NrThrottled:  diagnosis.Known(0),
		UsageUsec:    diagnosis.Known(5000000),
		Pressure:     diagnosis.Known(0.9),
		Steal:        diagnosis.Known(0),
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

		// Without this the mapping assertions below could all pass against an
		// implementation that wrote 0 everywhere. Four staged numbers that
		// cannot stand in for one another: a dropped or crossed field moves at
		// least one of them.
		Expect(status.Details.CapacityCores).To(Equal(2.0))
		Expect(status.Details.HostCpus).To(Equal(8.0))
		Expect(status.Details.PressureAvg60).To(Equal(0.9))
		Expect(status.Details.ReserveCores).To(BeNumerically(">", 0))

		gauges := d.MetricsRecorder().Drain().Gauges

		Expect(gauges).To(HaveKeyWithValue(string(deps.GaugeCPUCapacityCores), status.Details.CapacityCores))
		Expect(gauges).To(HaveKeyWithValue(string(deps.GaugeCPUReserveCores), status.Details.ReserveCores))
		Expect(gauges).To(HaveKeyWithValue(string(deps.GaugeCPUHostCpus), status.Details.HostCpus))
		Expect(gauges).To(HaveKeyWithValue(string(deps.GaugeCPUPressureAvg60), status.Details.PressureAvg60))
		Expect(gauges).To(HaveKeyWithValue(string(deps.GaugeCPUAvgHostBusyCores), status.Details.AvgHostBusyCores))
		Expect(gauges).To(HaveKeyWithValue(string(deps.GaugeCPUHostHeadroomCores), status.Details.HostHeadroomCores))
		Expect(gauges).To(HaveKeyWithValue(string(deps.GaugeCPUAvgUsageCores), status.Details.AvgUsageCores))
		Expect(gauges).To(HaveKeyWithValue(string(deps.GaugeCPUAvgUsageFraction), status.Details.AvgUsageFraction))
		Expect(gauges).To(HaveKeyWithValue(string(deps.GaugeCPUThrottleRatio), status.Details.ThrottleRatio))
		Expect(gauges).To(HaveKeyWithValue(string(deps.GaugeCPUStealP95), status.Details.StealP95))
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
			deps.GaugeCPUStealSignalReady:      status.Details.StealSignalReady,
		}

		// An implementation that wrote 1 everywhere, or 0 everywhere, would pass
		// the per-flag assertions below if this tick happened to be unanimous.
		// Assert the staged evidence is mixed, so it cannot be.
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
		// current. Publishing 0 and saying so in the flag is the contract; this
		// spec is what fails if someone later skips the set to "avoid a
		// misleading zero".
		d := newDeps(fixedSampler(richSample()), 4, 2)

		status, err := Poll(context.Background(), d, CPUConfig{})
		Expect(err).NotTo(HaveOccurred())
		Expect(status.Details.StealSignalReady).To(BeFalse(),
			"this spec needs an unready signal; steal p95 needs a window this tick cannot have filled")

		gauges := d.MetricsRecorder().Drain().Gauges

		Expect(gauges).To(HaveKeyWithValue(string(deps.GaugeCPUStealP95), 0.0),
			"the measurement is published even though it was not readable")
		Expect(gauges).To(HaveKeyWithValue(string(deps.GaugeCPUStealSignalReady), 0.0),
			"and the flag beside it says the zero is not a measurement")
	})

	It("records nothing on a tick that could not measure", func() {
		d := newDeps(stubSampler{read: func(context.Context) (cpuhealth.Sample, error) {
			return cpuhealth.Sample{}, context.DeadlineExceeded
		}}, 4, 2)

		_, err := Poll(context.Background(), d, CPUConfig{})
		Expect(err).To(HaveOccurred())

		Expect(d.MetricsRecorder().Drain().Gauges).To(BeEmpty(),
			"a failed read publishes no gauge rather than a zero-valued measurement")
	})
})
