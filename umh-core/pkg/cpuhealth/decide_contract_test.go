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

// The whole-Details contract. Decide's stages hand 27 fields between two
// helper functions without changing any exported signature, so no existing
// test edits when those fields are wired to the wrong source: a mis-wire
// still returns a Details of the right shape, just with a value read from
// the wrong place. Every other test in this package asserts a handful of
// fields relevant to its own scenario; this one asserts every field Decide
// returns, for a fixed set of scenarios, against a struct literal with every
// field named — so a reviewer can read what Decide promises without running
// it, and a wrong wire anywhere in the 37 fields turns one line of this file
// red.

package cpuhealth

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

var _ = Describe("Decide's whole Details contract", func() {
	It("should return this exact Details on a healthy tick, with nothing fired", func() {
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasVirtualization, HasLimit, HasPressureStats)
		base := time.Now()

		var sig Details
		for i := 0; i <= 65; i++ {
			smp := Sample{
				Timestamp:    base.Add(time.Duration(i) * time.Second),
				CpuScope:     ScopeHost,
				Virtualized:  true,
				Pressure:     diagnosis.Known(0.1),
				PsiAvailable: true,
				Steal:        diagnosis.Known(0),
				HostBusy:     diagnosis.Known(0.5),
				UsageCores:   diagnosis.Known(0.2),
				NrPeriods:    diagnosis.Known(100 * float64(i)),
				NrThrottled:  diagnosis.Known(2 * float64(i)), // steady 0.02, below the 0.05 fire mark
				Quota:        diagnosis.Known(2.0),
				LogicalCpus:  diagnosis.Known(4),
				HostCpus:     diagnosis.Known(8),
			}
			_, sig = Decide(engine, smp, env)
		}

		Expect(sig).To(Equal(Details{
			UsageFraction:    diagnosis.Unknown(), // declared for a future projection; never filled
			ThrottleRatio:    0.02,
			PressureAvg60:    0.1,
			StealP95:         0,
			AvgUsageFraction: 0.049999999999999954,
			P95UsageFraction: diagnosis.Unknown(),
			P99UsageFraction: diagnosis.Unknown(),
			AvgUsageCores:    0.19999999999999982,
			P95UsageCores:    diagnosis.Unknown(),
			P99UsageCores:    diagnosis.Unknown(),
			UsageRingActive:  true,

			HostBusyRingActive: true,

			ThrottleFired:       false,
			PressureFired:       false,
			StealFired:          false,
			HostContentionFired: false,
			LimitedVisibility:   false,

			SaturationFired:            false,
			LimitSaturationFired:       false,
			HostFullFired:              false,
			NoHostStatsSaturationFired: false,
			NoLimitHostFired:           false,

			HostHeadroomCores:      2.5,
			HostBusyCoresAvailable: true,
			AvgHostBusyCores:       0.5,
			HeadroomCores:          diagnosis.Unknown(),
			CapacityCores:          2,
			ReserveCores:           0.2,

			LimitApplies:    true,
			PressureApplies: true,
			StealApplies:    true,

			HostHeadroomAvailable: true,
			LogicalCpus:           4,
			HostCpus:              8,

			ThrottleSignalReady: true,
			PressureSignalReady: true,
			StealSignalReady:    true,
		}))
	})

	It("should return this exact Details when the throttling signal fires", func() {
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasVirtualization, HasLimit, HasPressureStats)
		base := time.Now()

		var sig Details
		for i := 0; i <= 65; i++ {
			smp := Sample{
				Timestamp:    base.Add(time.Duration(i) * time.Second),
				CpuScope:     ScopeHost,
				Virtualized:  true,
				Pressure:     diagnosis.Known(0.1),
				PsiAvailable: true,
				Steal:        diagnosis.Known(0),
				HostBusy:     diagnosis.Known(0.5),
				UsageCores:   diagnosis.Known(0.2),
				NrPeriods:    diagnosis.Known(100 * float64(i)),
				NrThrottled:  diagnosis.Known(6 * float64(i)), // steady 0.06, above the 0.05 fire mark
				Quota:        diagnosis.Known(2.0),
				LogicalCpus:  diagnosis.Known(4),
				HostCpus:     diagnosis.Known(8),
			}
			_, sig = Decide(engine, smp, env)
		}

		Expect(sig).To(Equal(Details{
			UsageFraction:    diagnosis.Unknown(),
			ThrottleRatio:    0.06,
			PressureAvg60:    0.1,
			StealP95:         0,
			AvgUsageFraction: 0.049999999999999954,
			P95UsageFraction: diagnosis.Unknown(),
			P99UsageFraction: diagnosis.Unknown(),
			AvgUsageCores:    0.19999999999999982,
			P95UsageCores:    diagnosis.Unknown(),
			P99UsageCores:    diagnosis.Unknown(),
			UsageRingActive:  true,

			HostBusyRingActive: true,

			ThrottleFired:       true,
			PressureFired:       false,
			StealFired:          false,
			HostContentionFired: false,
			LimitedVisibility:   false,

			SaturationFired:            false,
			LimitSaturationFired:       false,
			HostFullFired:              false,
			NoHostStatsSaturationFired: false,
			NoLimitHostFired:           false,

			HostHeadroomCores:      2.5,
			HostBusyCoresAvailable: true,
			AvgHostBusyCores:       0.5,
			HeadroomCores:          diagnosis.Unknown(),
			CapacityCores:          2,
			ReserveCores:           0.2,

			LimitApplies:    true,
			PressureApplies: true,
			StealApplies:    true,

			HostHeadroomAvailable: true,
			LogicalCpus:           4,
			HostCpus:              8,

			ThrottleSignalReady: true,
			PressureSignalReady: true,
			StealSignalReady:    true,
		}))
	})

	It("should return this exact Details when the pressure signal fires", func() {
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasVirtualization, HasLimit, HasPressureStats)
		base := time.Now()

		var sig Details
		for i := 0; i <= 65; i++ {
			smp := Sample{
				Timestamp:    base.Add(time.Duration(i) * time.Second),
				CpuScope:     ScopeHost,
				Virtualized:  true,
				Pressure:     diagnosis.Known(0.35), // above the 0.20 fire mark
				PsiAvailable: true,
				Steal:        diagnosis.Known(0),
				HostBusy:     diagnosis.Known(0.5),
				UsageCores:   diagnosis.Known(0.2),
				NrPeriods:    diagnosis.Known(100 * float64(i)),
				NrThrottled:  diagnosis.Known(2 * float64(i)),
				Quota:        diagnosis.Known(2.0),
				LogicalCpus:  diagnosis.Known(4),
				HostCpus:     diagnosis.Known(8),
			}
			_, sig = Decide(engine, smp, env)
		}

		Expect(sig).To(Equal(Details{
			UsageFraction:    diagnosis.Unknown(),
			ThrottleRatio:    0.02,
			PressureAvg60:    0.35,
			StealP95:         0,
			AvgUsageFraction: 0.049999999999999954,
			P95UsageFraction: diagnosis.Unknown(),
			P99UsageFraction: diagnosis.Unknown(),
			AvgUsageCores:    0.19999999999999982,
			P95UsageCores:    diagnosis.Unknown(),
			P99UsageCores:    diagnosis.Unknown(),
			UsageRingActive:  true,

			HostBusyRingActive: true,

			ThrottleFired:       false,
			PressureFired:       true,
			StealFired:          false,
			HostContentionFired: false,
			LimitedVisibility:   false,

			SaturationFired:            false,
			LimitSaturationFired:       false,
			HostFullFired:              false,
			NoHostStatsSaturationFired: false,
			NoLimitHostFired:           false,

			HostHeadroomCores:      2.5,
			HostBusyCoresAvailable: true,
			AvgHostBusyCores:       0.5,
			HeadroomCores:          diagnosis.Unknown(),
			CapacityCores:          2,
			ReserveCores:           0.2,

			LimitApplies:    true,
			PressureApplies: true,
			StealApplies:    true,

			HostHeadroomAvailable: true,
			LogicalCpus:           4,
			HostCpus:              8,

			ThrottleSignalReady: true,
			PressureSignalReady: true,
			StealSignalReady:    true,
		}))
	})

	It("should return this exact Details when the steal signal fires", func() {
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasVirtualization, HasLimit, HasPressureStats)
		base := time.Now()

		var sig Details
		for i := 0; i <= 65; i++ {
			smp := Sample{
				Timestamp:    base.Add(time.Duration(i) * time.Second),
				CpuScope:     ScopeHost,
				Virtualized:  true,
				Pressure:     diagnosis.Known(0.1),
				PsiAvailable: true,
				Steal:        diagnosis.Known(0.9), // above the 0.10 fire mark
				HostBusy:     diagnosis.Known(0.5),
				UsageCores:   diagnosis.Known(0.2),
				NrPeriods:    diagnosis.Known(100 * float64(i)),
				NrThrottled:  diagnosis.Known(2 * float64(i)),
				Quota:        diagnosis.Known(2.0),
				LogicalCpus:  diagnosis.Known(4),
				HostCpus:     diagnosis.Known(8),
			}
			_, sig = Decide(engine, smp, env)
		}

		Expect(sig).To(Equal(Details{
			UsageFraction:    diagnosis.Unknown(),
			ThrottleRatio:    0.02,
			PressureAvg60:    0.1,
			StealP95:         0.9,
			AvgUsageFraction: 0.049999999999999954,
			P95UsageFraction: diagnosis.Unknown(),
			P99UsageFraction: diagnosis.Unknown(),
			AvgUsageCores:    0.19999999999999982,
			P95UsageCores:    diagnosis.Unknown(),
			P99UsageCores:    diagnosis.Unknown(),
			UsageRingActive:  true,

			HostBusyRingActive: true,

			ThrottleFired:       false,
			PressureFired:       false,
			StealFired:          true,
			HostContentionFired: false,
			LimitedVisibility:   false,

			SaturationFired:            false,
			LimitSaturationFired:       false,
			HostFullFired:              false,
			NoHostStatsSaturationFired: false,
			NoLimitHostFired:           false,

			HostHeadroomCores:      2.5,
			HostBusyCoresAvailable: true,
			AvgHostBusyCores:       0.5,
			HeadroomCores:          diagnosis.Unknown(),
			CapacityCores:          2,
			ReserveCores:           0.2,

			LimitApplies:    true,
			PressureApplies: true,
			StealApplies:    true,

			HostHeadroomAvailable: true,
			LogicalCpus:           4,
			HostCpus:              8,

			ThrottleSignalReady: true,
			PressureSignalReady: true,
			StealSignalReady:    true,
		}))
	})

	It("should return this exact Details when host-full and the container's own limit both fire on the same tick", func() {
		// Same drive as attribution_test.go's host-full-AND-limit scenario:
		// quota 2.0, 4 cores, usage 0.2/hostBusy 0.1 until tick 40, then
		// 1.95/3.8. Both saturation arms fire; the fold keeps the host arm.
		// The Sample here carries no Quota/LogicalCpus/HostCpus (matching the
		// reused scenario), so CapacityCores and LogicalCpus/HostCpus read as
		// their zero value and LimitedVisibility reads true — Details fields
		// that read the Sample, not the engine's quota parameter.
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasVirtualization, HasLimit)
		base := time.Now()

		var sig Details
		for i := 0; i <= 100; i++ {
			usage, hb := 0.2, 0.1
			if i >= 40 {
				usage, hb = 1.95, 3.8
			}
			smp := Sample{
				Timestamp:   base.Add(time.Duration(i) * time.Second),
				CpuScope:    ScopeHost,
				Virtualized: true,
				UsageCores:  diagnosis.Known(usage),
				HostBusy:    diagnosis.Known(hb),
			}
			_, sig = Decide(engine, smp, env)
		}

		Expect(sig).To(Equal(Details{
			UsageFraction:    diagnosis.Unknown(),
			ThrottleRatio:    0,
			PressureAvg60:    0,
			StealP95:         0,
			AvgUsageFraction: 0.4875000000000005,
			P95UsageFraction: diagnosis.Unknown(),
			P99UsageFraction: diagnosis.Unknown(),
			AvgUsageCores:    1.950000000000002,
			P95UsageCores:    diagnosis.Unknown(),
			P99UsageCores:    diagnosis.Unknown(),
			UsageRingActive:  true,

			HostBusyRingActive: true,

			ThrottleFired:       false,
			PressureFired:       false,
			StealFired:          false,
			HostContentionFired: false,
			LimitedVisibility:   true,

			SaturationFired:            true,
			LimitSaturationFired:       true,
			HostFullFired:              true,
			NoHostStatsSaturationFired: false,
			NoLimitHostFired:           false,

			HostHeadroomCores:      -0.7999999999999993,
			HostBusyCoresAvailable: true,
			AvgHostBusyCores:       3.800000000000004,
			HeadroomCores:          diagnosis.Unknown(),
			CapacityCores:          0,
			ReserveCores:           1,

			LimitApplies:    true,
			PressureApplies: false,
			StealApplies:    true,

			HostHeadroomAvailable: true,
			LogicalCpus:           0,
			HostCpus:              0,

			ThrottleSignalReady: false,
			PressureSignalReady: false,
			StealSignalReady:    false,
		}))
	})

	It("should return this exact Details when host stats are unreadable and the usage-fraction fallback fires", func() {
		// Same drive as unit_coupling_test.go's fallback scenario: host busy
		// unreadable throughout, usage 3.0/4 cores = 0.75 fires the fallback
		// arm. Quota 8.0 keeps the limit arm quiet (8 - 3 - 0.8 = 4.2).
		engine, err := NewEngine(4, 8.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasLimit)
		base := time.Now()

		var sig Details
		for i := 0; i <= 5; i++ {
			smp := Sample{
				Timestamp:   base.Add(time.Duration(i) * time.Second),
				CpuScope:    ScopeHost,
				HostBusy:    diagnosis.Unknown(),
				UsageCores:  diagnosis.Known(3.0),
				NrPeriods:   diagnosis.Known(0),
				NrThrottled: diagnosis.Known(0),
			}
			_, sig = Decide(engine, smp, env)
		}

		Expect(sig).To(Equal(Details{
			UsageFraction:    diagnosis.Unknown(),
			ThrottleRatio:    0,
			PressureAvg60:    0,
			StealP95:         0,
			AvgUsageFraction: 0.75,
			P95UsageFraction: diagnosis.Unknown(),
			P99UsageFraction: diagnosis.Unknown(),
			AvgUsageCores:    3,
			P95UsageCores:    diagnosis.Unknown(),
			P99UsageCores:    diagnosis.Unknown(),
			UsageRingActive:  true,

			HostBusyRingActive: false,

			ThrottleFired:       false,
			PressureFired:       false,
			StealFired:          false,
			HostContentionFired: false,
			LimitedVisibility:   true,

			SaturationFired:            true,
			LimitSaturationFired:       false,
			HostFullFired:              false,
			NoHostStatsSaturationFired: true,
			NoLimitHostFired:           false,

			HostHeadroomCores:      0,
			HostBusyCoresAvailable: false,
			AvgHostBusyCores:       0,
			HeadroomCores:          diagnosis.Unknown(),
			CapacityCores:          0,
			ReserveCores:           1,

			LimitApplies:    true,
			PressureApplies: false,
			StealApplies:    false,

			HostHeadroomAvailable: true,
			LogicalCpus:           0,
			HostCpus:              0,

			ThrottleSignalReady: false,
			PressureSignalReady: false,
			StealSignalReady:    false,
		}))
	})

	It("should return this exact Details in no-limit mode, when the host-headroom arm fires as NoLimitHostFired", func() {
		// Same drive as unit_coupling_test.go's no-limit scenario: 4 cores, no
		// quota, host busy 3.5 -> headroom 4 - 3.5 - 1.0 = -0.5 fires. With no
		// HasLimit capability the same instrument reports under
		// NoLimitHostFired rather than HostFullFired.
		engine, err := NewEngine(4, 0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment()
		base := time.Now()

		var sig Details
		for i := 0; i <= 5; i++ {
			smp := Sample{
				Timestamp:  base.Add(time.Duration(i) * time.Second),
				CpuScope:   ScopeHost,
				HostBusy:   diagnosis.Known(3.5),
				UsageCores: diagnosis.Known(0.5),
			}
			_, sig = Decide(engine, smp, env)
		}

		Expect(sig).To(Equal(Details{
			UsageFraction:    diagnosis.Unknown(),
			ThrottleRatio:    0,
			PressureAvg60:    0,
			StealP95:         0,
			AvgUsageFraction: 0.125,
			P95UsageFraction: diagnosis.Unknown(),
			P99UsageFraction: diagnosis.Unknown(),
			AvgUsageCores:    0.5,
			P95UsageCores:    diagnosis.Unknown(),
			P99UsageCores:    diagnosis.Unknown(),
			UsageRingActive:  true,

			HostBusyRingActive: true,

			ThrottleFired:       false,
			PressureFired:       false,
			StealFired:          false,
			HostContentionFired: false,
			LimitedVisibility:   true,

			SaturationFired:            true,
			LimitSaturationFired:       false,
			HostFullFired:              false,
			NoHostStatsSaturationFired: false,
			NoLimitHostFired:           true,

			HostHeadroomCores:      -0.5,
			HostBusyCoresAvailable: true,
			AvgHostBusyCores:       3.5,
			HeadroomCores:          diagnosis.Unknown(),
			CapacityCores:          0,
			ReserveCores:           1,

			LimitApplies:    false,
			PressureApplies: false,
			StealApplies:    false,

			HostHeadroomAvailable: true,
			LogicalCpus:           0,
			HostCpus:              0,

			ThrottleSignalReady: false,
			PressureSignalReady: false,
			StealSignalReady:    false,
		}))
	})

	It("should return this exact Details on a cold first tick, before any window has enough samples to answer", func() {
		// One sample, nothing warmed up: the usage and host-busy tracks both
		// need a second sample before their Mean floor is met, so
		// UsageRingActive and HostBusyRingActive read false. CpuScope is
		// ScopeUnknown — the scope a sampler reports before its cpuset read
		// has ever succeeded — so HostHeadroomAvailable reads false too, and
		// the host-headroom arm's own scope guard withholds a value, leaving
		// HostHeadroomCores at its zero value. Both HostHeadroomAvailable and
		// UsageRingActive read true in every other scenario in this file, so
		// this is the one scenario where a mis-wire that hardcodes either to
		// true turns red.
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasVirtualization, HasLimit, HasPressureStats)

		smp := Sample{
			Timestamp:    time.Now(),
			CpuScope:     ScopeUnknown,
			Virtualized:  true,
			Pressure:     diagnosis.Known(0.1),
			PsiAvailable: true,
			Steal:        diagnosis.Known(0),
			HostBusy:     diagnosis.Known(0.5),
			UsageCores:   diagnosis.Known(0.2),
			NrPeriods:    diagnosis.Known(0),
			NrThrottled:  diagnosis.Known(0),
			Quota:        diagnosis.Known(2.0),
			LogicalCpus:  diagnosis.Known(4),
			HostCpus:     diagnosis.Known(8),
		}
		_, sig := Decide(engine, smp, env)

		Expect(sig).To(Equal(Details{
			UsageFraction:    diagnosis.Unknown(),
			ThrottleRatio:    0,
			PressureAvg60:    0.1,
			StealP95:         0,
			AvgUsageFraction: 0.05,
			P95UsageFraction: diagnosis.Unknown(),
			P99UsageFraction: diagnosis.Unknown(),
			AvgUsageCores:    0.2,
			P95UsageCores:    diagnosis.Unknown(),
			P99UsageCores:    diagnosis.Unknown(),
			UsageRingActive:  false,

			HostBusyRingActive: false,

			ThrottleFired:       false,
			PressureFired:       false,
			StealFired:          false,
			HostContentionFired: false,
			LimitedVisibility:   false,

			SaturationFired:            false,
			LimitSaturationFired:       false,
			HostFullFired:              false,
			NoHostStatsSaturationFired: false,
			NoLimitHostFired:           false,

			HostHeadroomCores:      0,
			HostBusyCoresAvailable: true,
			AvgHostBusyCores:       0.5,
			HeadroomCores:          diagnosis.Unknown(),
			CapacityCores:          2,
			ReserveCores:           0.2,

			LimitApplies:    true,
			PressureApplies: true,
			StealApplies:    true,

			HostHeadroomAvailable: false,
			LogicalCpus:           4,
			HostCpus:              8,

			ThrottleSignalReady: false,
			PressureSignalReady: true,
			StealSignalReady:    false,
		}))
	})
})
