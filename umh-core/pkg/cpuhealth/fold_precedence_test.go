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

// The saturation fold's precedence. The fold keeps ONE member of the saturation
// family; when the usage-fraction fallback and the limit arm co-fire on the
// same tick (host stats unreadable and the container at its limit), the decided
// order — host-full, then limit, then no-host-stats — says the limit arm
// survives. This spec pins that against BOTH declaration orders, so a fold that
// "fixed" the bug by reordering the rows instead of by rank is caught too.

package cpuhealth

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// orderedEngine builds an engine whose saturation rows appear in the given
// order, independent of cpuTable's production order, so the fold's choice is
// tested against both declarations and cannot be an artifact of row order.
func orderedEngine(cores, quota float64, limitFirst bool) *diagnosis.Engine[Sample] {
	base := cpuTable(cores, quota)
	sigs := make([]diagnosis.Signal[Sample], 0, len(base.Signals))
	for _, s := range base.Signals {
		if s.Name == sigSaturation || s.Name == sigLimitSaturation {
			continue
		}
		sigs = append(sigs, s)
	}
	if limitFirst {
		sigs = append(sigs, limitSaturationSignal(quota), saturationSignal(cores))
	} else {
		sigs = append(sigs, saturationSignal(cores), limitSaturationSignal(quota))
	}
	engine, err := diagnosis.NewEngine(diagnosis.Table[Sample]{
		Signals:      sigs,
		Measurements: base.Measurements,
		Interval:     base.Interval,
	})
	Expect(err).NotTo(HaveOccurred())
	return engine
}

var _ = Describe("the saturation fold's precedence", func() {
	It("should keep the limit arm, not the usage-fraction fallback, as the fold's survivor when both fire on one tick, in either declaration order", func() {
		// The scenario: /proc/stat unreadable so host-headroom cannot answer,
		// usage 3.0/4 = 0.75 fires the usage-fraction fallback, and
		// quota 2.0 - 3.0 - 0.2 = -1.2 fires the limit arm on the SAME tick.
		// The decided precedence (Jeremy, 2026-08-14) is host-full, limit,
		// no-host-stats, so the limit arm must survive the fold.
		Expect(declaredUnit(sigSaturation, instUsageFraction)).NotTo(BeEmpty(),
			"the usage-fraction arm must be in the table for this spec to mean anything")
		Expect(declaredUnit(sigLimitSaturation, instLimitHeadroom)).NotTo(BeEmpty(),
			"the limit arm must be in the table for this spec to mean anything")

		for _, limitFirst := range []bool{false, true} {
			engine := orderedEngine(4, 2.0, limitFirst)
			env := diagnosis.NewEnvironment(HasLimit)
			base := time.Now()

			for i := 0; i <= 5; i++ {
				smp := Sample{
					Timestamp:   base.Add(time.Duration(i) * time.Second),
					CpuScope:    ScopeHost,
					HostBusy:    diagnosis.Unknown(),
					UsageCores:  diagnosis.Known(3.0),
					NrPeriods:   diagnosis.Known(0),
					NrThrottled: diagnosis.Known(0),
				}
				verdict, sig := Decide(engine, smp, env)
				if i < 5 {
					continue
				}

				// Positive control: BOTH latches fired on this tick, so the fold
				// had two members to choose from. The flags record what FIRED,
				// not what won the fold, so both are expected to be raised.
				Expect(sig.LimitSaturationFired).To(BeTrue(),
					"limit-headroom 2.0 - 3.0 - 0.2 = -1.2 fires on the same tick (limit row first: %v)", limitFirst)
				Expect(sig.NoHostStatsSaturationFired).To(BeTrue(),
					"usage-fraction 3.0/4 = 0.75 fires the fallback latch (limit row first: %v)", limitFirst)
				Expect(sig.HostFullFired).To(BeFalse(),
					"an unreadable /proc/stat keeps the host-full arm out of it")
				fraction, _ := engine.Reduction(sigSaturation, instUsageFraction).Get()
				Expect(fraction).To(BeNumerically(">=", 0.70),
					"the fallback latch judged on a fired fraction (limit row first: %v)", limitFirst)

				// The assertion: the limit arm survives the fold, whatever the
				// row order, so the cause value is limit-headroom's number, not
				// the usage fraction.
				limitHeadroom, _ := engine.Reduction(sigLimitSaturation, instLimitHeadroom).Get()
				Expect(verdict.Causes[0].Value).To(BeNumerically("~", limitHeadroom, 1e-9),
					"the survivor's cause value is the limit-headroom reduction, not the usage fraction (limit row first: %v)", limitFirst)
				Expect(verdict.Causes[0].Unit).To(Equal(Unit("cores")),
					"the limit arm is denominated in cores (limit row first: %v)", limitFirst)
			}
		}
	})
})
