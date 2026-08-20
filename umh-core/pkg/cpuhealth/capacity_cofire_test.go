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

// The two capacity signals firing on one tick. The machine can be full while
// this container is also out of its own CPU limit, and the machine can be
// estimated full from our own usage while the same container is out of that
// limit. Both are two causes and one paragraph: the remedies contradict each
// other, so the message speaks with one of the pair. These specs pin which one,
// and that both causes still reach the verdict.

package cpuhealth

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// orderedEngine builds an engine whose two capacity rows appear in the given
// order, independent of cpuTable's production order, so the choice of speaker
// is tested against both declarations and cannot be an artifact of row order.
func orderedEngine(cores, quota float64, limitFirst bool) *diagnosis.Engine[Sample] {
	base := cpuTable(cores, quota)
	sigs := make([]diagnosis.Signal[Sample], 0, len(base.Signals))
	for _, s := range base.Signals {
		if s.Name == sigHostCpuFull || s.Name == sigContainerLimitFull {
			continue
		}
		sigs = append(sigs, s)
	}
	if limitFirst {
		sigs = append(sigs, containerLimitFullSignal(quota), hostCpuFullSignal(cores))
	} else {
		sigs = append(sigs, hostCpuFullSignal(cores), containerLimitFullSignal(quota))
	}
	engine, err := diagnosis.NewEngine(diagnosis.Table[Sample]{
		Signals:      sigs,
		Measurements: base.Measurements,
		Interval:     base.Interval,
	})
	Expect(err).NotTo(HaveOccurred())
	return engine
}

var _ = Describe("two capacity causes on one tick", func() {
	It("should let the container's own limit speak, not the estimate from our usage, when both fire in either declaration order", func() {
		// The scenario: /proc/stat unreadable so host-headroom cannot answer,
		// usage 3.0/4 = 0.75 fires the usage-fraction estimate, and
		// quota 2.0 - 3.0 - 0.2 = -1.2 fires the container's own limit on the
		// SAME tick. The limit is a measured ceiling and the estimate is not,
		// so the limit writes the paragraph.
		Expect(declaredUnit(sigHostCpuFull, instUsageFraction)).NotTo(BeEmpty(),
			"the usage-fraction arm must be in the table for this spec to mean anything")
		Expect(declaredUnit(sigContainerLimitFull, instLimitHeadroom)).NotTo(BeEmpty(),
			"the limit arm must be in the table for this spec to mean anything")

		for _, limitFirst := range []bool{false, true} {
			engine := orderedEngine(4, 2.0, limitFirst)
			env := diagnosis.NewEnvironment(HasLimit)
			base := time.Now()

			for i := 0; i <= 5; i++ {
				smp := Sample{
					Timestamp:   base.Add(time.Duration(i) * time.Second),
					CpuScope:    ScopeHost,
					Quota:       diagnosis.Known(2.0),
					HostBusy:    diagnosis.Unknown(),
					UsageCores:  diagnosis.Known(3.0),
					NrPeriods:   diagnosis.Known(0),
					NrThrottled: diagnosis.Known(0),
				}
				verdict, sig := Decide(engine, smp, env)
				if i < 5 {
					continue
				}

				// Positive control: both latches fired on this tick, so there
				// were two causes to choose a speaker from.
				Expect(kindsOf(verdict.Causes)).To(ConsistOf(CauseKindHostCpuFull, CauseKindContainerLimitFull),
					"both capacity signals must fire for this spec to mean anything (limit row first: %v)", limitFirst)
				Expect(causeOfKind(verdict.Causes, CauseKindHostCpuFull).Instrument).To(Equal(instUsageFraction),
					"an unreadable /proc/stat leaves usage-fraction as the only machine reading (limit row first: %v)", limitFirst)

				// The limit is the more severe of the two, so it also ranks
				// first: -1.2 against a worst of -0.2, versus 0.75 against a
				// worst of 1.0.
				limitHeadroom, _ := engine.Reduction(sigContainerLimitFull, instLimitHeadroom).Get()
				Expect(verdict.Causes[0].Kind).To(Equal(CauseKindContainerLimitFull),
					"the limit outranks the estimate on severity (limit row first: %v)", limitFirst)
				Expect(verdict.Causes[0].Value).To(BeNumerically("~", limitHeadroom, 1e-9),
					"the cause value is the limit-headroom reduction, not the usage fraction (limit row first: %v)", limitFirst)
				Expect(verdict.Causes[0].Unit).To(Equal(Unit("cores")),
					"the limit arm is denominated in cores (limit row first: %v)", limitFirst)

				// One paragraph, and it is the limit's.
				msg := ComposeMessage(verdict, sig)
				Expect(msg).NotTo(ContainSubstring("\n\n"),
					"two capacity causes rendered two paragraphs (limit row first: %v)", limitFirst)
				Expect(msg).To(ContainSubstring("Raise its CPU limit, or reduce the load on it."),
					"the limit paragraph is the one that must be printed (limit row first: %v)", limitFirst)
				Expect(msg).NotTo(ContainSubstring("Host contention is not visible here"),
					"the usage-fraction paragraph must stay silent (limit row first: %v)", limitFirst)
			}
		}
	})

	It("should reach the verdict as two distinct kinds and render the blended sentence when the machine is full and the container is at its limit", func() {
		// 4 cores, a 2.0-core quota, host busy 3.8, our usage 1.95.
		// host-headroom is 4 - 3.8 - 1.0 = -0.8 and limit-headroom is
		// 2 - 1.95 - 0.2 = -0.15, so both fire. usage-fraction sits at
		// 1.95 / 4 = 0.4875, below its 0.70 mark, so the machine reading is
		// host-headroom's.
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasLimit)
		base := time.Now()

		var verdict Verdict
		var sig Details
		for i := 0; i <= 5; i++ {
			smp := Sample{
				Timestamp:   base.Add(time.Duration(i) * time.Second),
				CpuScope:    ScopeHost,
				Quota:       diagnosis.Known(2.0),
				HostBusy:    diagnosis.Known(3.8),
				UsageCores:  diagnosis.Known(1.95),
				LogicalCpus: diagnosis.Known(4),
				HostCpus:    diagnosis.Known(4),
				NrPeriods:   diagnosis.Known(0),
				NrThrottled: diagnosis.Known(0),
			}
			verdict, sig = Decide(engine, smp, env)
		}

		// The two kinds are distinct all the way to the verdict. Nothing
		// collapsed them before the ranking, and neither is reported as the
		// other.
		Expect(kindsOf(verdict.Causes)).To(ConsistOf(CauseKindHostCpuFull, CauseKindContainerLimitFull))
		Expect(causeOfKind(verdict.Causes, CauseKindHostCpuFull).Instrument).To(Equal(instHostHeadroom))
		Expect(causeOfKind(verdict.Causes, CauseKindContainerLimitFull).Instrument).To(Equal(instLimitHeadroom))

		// One blended sentence, not two paragraphs. Telling a customer to add
		// CPU to a full machine and also to raise their own limit gives them a
		// remedy that cannot work.
		Expect(ComposeMessage(verdict, sig)).To(Equal(
			"CPU running near full" +
				"\nTechnical Details: " +
				"The machine is full and this instance's CPU limit cannot help. " +
				"Add CPU to the machine, or reduce other software running on it. " +
				"(This instance is also at its 2-core limit.)"))
		Expect(BlockReason(verdict.Causes, sig)).To(Equal(
			"Can't add another bridge: the machine is full. Add CPU to the machine, or reduce other software running on it, first."))
	})
})

// kindsOf lists the cause kinds a verdict carries, for a set comparison that
// says nothing about their order.
func kindsOf(causes []Cause) []CauseKind {
	kinds := make([]CauseKind, 0, len(causes))
	for _, c := range causes {
		kinds = append(kinds, c.Kind)
	}

	return kinds
}

// causeOfKind returns the cause of this kind, or the zero Cause when the
// verdict carries none.
func causeOfKind(causes []Cause, kind CauseKind) Cause {
	for _, c := range causes {
		if c.Kind == kind {
			return c
		}
	}

	return Cause{}
}
