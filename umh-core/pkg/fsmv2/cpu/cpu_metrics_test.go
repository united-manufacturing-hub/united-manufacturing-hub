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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// sampleAt is the instant every sample below is read at. A fixed instant rather
// than time.Now(), so a spec can assert the exact unix second the freshness
// gauge carries. Nothing in cpuhealth.Decide compares a sample against the wall
// clock: the engine's windows are ordered only against each other.
var sampleAt = time.Date(2025, 9, 16, 10, 0, 0, 0, time.UTC)

// richSample stages a tick whose Details carry a different number in every
// published field, so a mapping that crossed two fields moves at least one of
// them.
func richSample() cpuhealth.Sample {
	return cpuhealth.Sample{
		Timestamp:    sampleAt,
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

// tickSampler hands back one staged sample per Read, in order. fixedSampler
// cannot drive a sequence: it repeats one reading at one instant, and the
// engine's 60s windows would never see a second distinct tick.
type tickSampler struct {
	samples []cpuhealth.Sample
	next    int
}

func newTickSampler(samples ...cpuhealth.Sample) *tickSampler {
	return &tickSampler{samples: samples}
}

func (s *tickSampler) Read(context.Context) (cpuhealth.Sample, error) {
	Expect(s.next).To(BeNumerically("<", len(s.samples)),
		"the spec polled more often than it staged ticks; repeating the last sample would publish values no staged tick asked for")

	sample := s.samples[s.next]
	s.next++

	return sample, nil
}

// flagTicks stages the sequence the flag spec polls, oldest first. Each flag
// ends up with its own pattern of values across these ticks, which is what lets
// the spec catch two flags published under each other's names.
//
// Two ticks offer four patterns and there are six flags, and within a single
// tick there is no equivalent of the distinct number per field richSample
// stages, because a boolean has two values. Three ticks still leave two flags
// sharing a pattern, which is what the fourth is for.
func flagTicks() []cpuhealth.Sample {
	// Tick 1 is the engine's first. The two ring flags and the throttle flag
	// all read false here whatever is staged: a mean needs two readings and a
	// counter delta needs two counter reads. It is also the only tick whose
	// cpuset covers the whole machine, which is the one thing
	// cpu_host_headroom_available reports.
	first := richSample()

	// Tick 2 gives both means their second reading, so both ring flags turn
	// true. Its nr_throttled read failed, so the throttle window stores nothing
	// and the throttle flag stays false — that is what separates the throttle
	// flag from the two rings. The cpuset is now a subset of the machine.
	second := richSample()
	second.Timestamp = first.Timestamp.Add(time.Second)
	second.CpuScope = cpuhealth.ScopeAffinity
	second.UsageCores = diagnosis.Known(1.6)
	second.HostBusy = diagnosis.Known(0.6)
	second.NrThrottled = diagnosis.Unknown()

	// Tick 3 gives the throttle window its second counter read, with both
	// counters advanced past tick 1's, so the throttle flag is true here and
	// nowhere else. Its /proc/stat and cpu.pressure reads failed, which drops
	// the host-busy ring and the pressure flag back to false; the usage ring
	// keeps reading, which is what separates those three from each other.
	third := richSample()
	third.Timestamp = second.Timestamp.Add(time.Second)
	third.CpuScope = cpuhealth.ScopeAffinity
	third.UsageCores = diagnosis.Known(1.4)
	third.HostBusy = diagnosis.Unknown()
	third.Pressure = diagnosis.Unknown()
	third.NrPeriods = diagnosis.Known(300)
	third.NrThrottled = diagnosis.Known(5)

	// Tick 4 restores the /proc/stat read while cpu.pressure stays unreadable.
	// Over the first three ticks cpu_host_busy_cores_available and
	// cpu_pressure_signal_ready read the same three values; this is the tick
	// that separates them, because the host-busy figure is readable here and
	// the pressure figure is not. Both throttle counters advance past tick 3's,
	// so the throttle window keeps its points rather than restarting.
	fourth := richSample()
	fourth.Timestamp = third.Timestamp.Add(time.Second)
	fourth.CpuScope = cpuhealth.ScopeAffinity
	fourth.UsageCores = diagnosis.Known(1.3)
	fourth.HostBusy = diagnosis.Known(0.7)
	fourth.Pressure = diagnosis.Unknown()
	fourth.NrPeriods = diagnosis.Known(600)
	fourth.NrThrottled = diagnosis.Known(10)

	return []cpuhealth.Sample{first, second, third, fourth}
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

	It("publishes every boolean as 1 or 0 under the gauge name that reports it", func() {
		// The expected value per staged tick, in flagTicks' order. Every row
		// differs from every other row, so publishing one flag's value under
		// another flag's name changes at least one tick's number and fails
		// here. Counting trues and falses instead would pass with any two of
		// the three flags that read false on a single tick swapped.
		want := map[deps.GaugeName][]float64{
			deps.GaugeCPUUsageRingActive:        {0, 1, 1, 1},
			deps.GaugeCPUHostBusyRingActive:     {0, 1, 0, 1},
			deps.GaugeCPUHostHeadroomAvailable:  {1, 0, 0, 0},
			deps.GaugeCPUThrottleSignalReady:    {0, 0, 1, 1},
			deps.GaugeCPUPressureSignalReady:    {1, 1, 0, 0},
			deps.GaugeCPUHostBusyCoresAvailable: {1, 1, 0, 1},
		}

		rows := make(map[string]deps.GaugeName, len(want))
		for name, row := range want {
			key := fmt.Sprint(row)
			Expect(rows).NotTo(HaveKey(key),
				"flagTicks must give every flag its own row, or a crossed mapping passes: %s and %s both read %v",
				name, rows[key], row)
			rows[key] = name
		}

		ticks := flagTicks()
		d := newDeps(newTickSampler(ticks...), 4, 2)

		for i := range ticks {
			_, err := Poll(context.Background(), d, CPUConfig{})
			Expect(err).NotTo(HaveOccurred())

			// Drain per tick, not once at the end: a gauge keeps its previous
			// value until the next SetGauge, so a single drain after the last
			// poll would only ever show the last tick's row.
			gauges := d.MetricsRecorder().Drain().Gauges

			for name, row := range want {
				Expect(gauges).To(HaveKeyWithValue(string(name), row[i]),
					"on tick %d, %s must publish %v", i+1, name, row[i])
			}
		}
	})

	It("publishes each series under the name an operator's dashboard already queries", func() {
		// Every other spec here keys its expectation off the same constant the
		// code publishes under, so renaming a constant's value keeps them all
		// green while the dashboards and alerts built on these names go blank.
		// The literals are spelled out rather than derived from the constants,
		// which is the whole point: an expectation derived from the thing under
		// test cannot see it change.
		want := map[deps.GaugeName]string{
			deps.GaugeCPUAvgUsageCores:          "cpu_avg_usage_cores",
			deps.GaugeCPUAvgUsageFraction:       "cpu_avg_usage_fraction",
			deps.GaugeCPUThrottleRatio:          "cpu_throttle_ratio",
			deps.GaugeCPUPressureAvg60:          "cpu_pressure_avg60_ratio",
			deps.GaugeCPUHostHeadroomCores:      "cpu_host_headroom_cores",
			deps.GaugeCPUAvgHostBusyCores:       "cpu_avg_host_busy_cores",
			deps.GaugeCPUCapacityCores:          "cpu_capacity_cores",
			deps.GaugeCPUReserveCores:           "cpu_reserve_cores",
			deps.GaugeCPUHostCpus:               "cpu_host_cpus",
			deps.GaugeCPULastSampleUnix:         "cpu_last_sample_unix",
			deps.GaugeCPUUsageRingActive:        "cpu_usage_ring_active",
			deps.GaugeCPUHostBusyRingActive:     "cpu_host_busy_ring_active",
			deps.GaugeCPUHostHeadroomAvailable:  "cpu_host_headroom_available",
			deps.GaugeCPUThrottleSignalReady:    "cpu_throttle_signal_ready",
			deps.GaugeCPUPressureSignalReady:    "cpu_pressure_signal_ready",
			deps.GaugeCPUHostBusyCoresAvailable: "cpu_host_busy_cores_available",
		}

		for constant, literal := range want {
			Expect(string(constant)).To(Equal(literal),
				"%s names a prometheus series an operator queries; renaming it is a breaking change, not a refactor", literal)
		}

		d := newDeps(fixedSampler(richSample()), 4, 2)

		_, err := Poll(context.Background(), d, CPUConfig{})
		Expect(err).NotTo(HaveOccurred())

		gauges := d.MetricsRecorder().Drain().Gauges

		Expect(gauges).To(HaveLen(len(want)),
			"a measured tick publishes exactly the series named above; a gauge added without a name here, or one dropped, changes the count")

		for _, literal := range want {
			Expect(gauges).To(HaveKey(literal),
				"%s must be published on a measured tick", literal)
		}
	})

	It("stamps a measured tick with the sample's own read time, not the clock at publish time", func() {
		// Reading the clock here instead would report a fresh tick after a read
		// that returned a stale sample, which is the one thing this series
		// exists to reveal.
		sample := richSample()
		d := newDeps(fixedSampler(sample), 4, 2)

		_, err := Poll(context.Background(), d, CPUConfig{})
		Expect(err).NotTo(HaveOccurred())

		gauges := d.MetricsRecorder().Drain().Gauges

		Expect(gauges).To(HaveKeyWithValue(string(deps.GaugeCPULastSampleUnix), float64(sample.Timestamp.Unix())),
			"the stamp must be the sample's own unix second, %d", sample.Timestamp.Unix())
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

	It("records nothing on a tick that could not measure, the freshness stamp included", func() {
		d := newDeps(stubSampler{read: func(context.Context) (cpuhealth.Sample, error) {
			return cpuhealth.Sample{}, context.DeadlineExceeded
		}}, 4, 2)

		_, err := Poll(context.Background(), d, CPUConfig{})
		Expect(err).To(HaveOccurred())

		// A statement about the recorder, not about what a scrape sees: the
		// collector re-publishes the previous values from CSE on a failed poll.
		// cpu_last_sample_unix is how a reader tells that apart from a fresh
		// tick, so writing it here would erase the evidence of the freeze.
		Expect(d.MetricsRecorder().Drain().Gauges).To(BeEmpty(),
			"a failed read publishes no gauge rather than a zero-valued measurement, and no freshness stamp either")
	})
})
