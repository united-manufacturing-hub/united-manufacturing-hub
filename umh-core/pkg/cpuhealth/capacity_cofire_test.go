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
// this container is also out of its own CPU limit. That is two causes and one
// paragraph: the remedies contradict each other, so the message speaks with one
// of the pair. This spec pins which one, that both causes still reach the
// verdict, and that neither depends on the order the two rows were declared in.
//
// The machine cannot be ESTIMATED full beside a limit. usage-fraction requires
// HasLimitedVisibility, so on a box with a quota the estimate is not capable
// and an unreadable /proc/stat leaves host-cpu-full with nothing to say. That
// box has its own spec below.

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
	It("should reach the verdict as two distinct kinds and render the blended sentence when the machine is full and the container is at its limit, in either declaration order", func() {
		// 4 cores, a 2.0-core quota, host busy 3.8, our usage 1.95.
		// host-headroom is 4 - 3.8 - 1.0 = -0.8 and limit-headroom is
		// 2 - 1.95 - 0.2 = -0.15, so both fire.
		//
		// The two rows are declared both ways round, because nothing about
		// which of the pair speaks may come from where the table happens to
		// list them. Severity decides it — -0.8 against a worst of -1.0 beats
		// -0.15 against a worst of -0.2 — and Rank reaches declaration position
		// only as its last tie-break.
		Expect(declaresArm(sigHostCpuFull, instHostHeadroom)).To(BeTrue(),
			"the host-headroom arm must be in the table for this spec to mean anything")
		Expect(declaresArm(sigContainerLimitFull, instLimitHeadroom)).To(BeTrue(),
			"the limit arm must be in the table for this spec to mean anything")

		for _, limitFirst := range []bool{false, true} {
			engine := orderedEngine(4, 2.0, limitFirst)
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
			Expect(kindsOf(verdict.Causes)).To(ConsistOf(CauseKindHostCpuFull, CauseKindContainerLimitFull),
				"both capacity signals must fire for this spec to mean anything (limit row first: %v)", limitFirst)
			Expect(causeOfKind(verdict.Causes, CauseKindHostCpuFull).Instrument).To(Equal(instHostHeadroom),
				"limit row first: %v", limitFirst)
			Expect(causeOfKind(verdict.Causes, CauseKindContainerLimitFull).Instrument).To(Equal(instLimitHeadroom),
				"limit row first: %v", limitFirst)
			Expect(verdict.Causes[0].Kind).To(Equal(CauseKindHostCpuFull),
				"the machine outranks the limit on severity, whichever row was declared first (limit row first: %v)", limitFirst)

			// One blended sentence, not two paragraphs. Telling a customer to add
			// CPU to a full machine and also to raise their own limit gives them a
			// remedy that cannot work.
			Expect(ComposeMessage(verdict, sig)).To(Equal(
				"CPU running near full"+
					"\nTechnical Details: "+
					"The machine is full and this instance's CPU limit cannot help. "+
					"Add CPU to the machine, or reduce other software running on it. "+
					"(This instance is also at its 2-core limit.)"),
				"limit row first: %v", limitFirst)
			Expect(BlockReason(verdict.Causes, verdict.Attribution, sig)).To(Equal(
				"Can't add another bridge: the machine is full. Add CPU to the machine, or reduce other software running on it, first."),
				"limit row first: %v", limitFirst)
		}
	})

	It("should leave the machine out of it entirely when a container at its own limit cannot read host stats", func() {
		// 4 cores, a 3.5-core quota, 3.5 cores used, /proc/stat unreadable.
		// limit-headroom reduces to
		// 3.5 - 3.5 - 0.35 = -0.35 and fires. The estimate from our own usage
		// reduces to 3.5/4 = 0.875, past its 0.70 fire mark, and does not fire,
		// because a box with a CPU limit has better evidence than an estimate.
		// So the machine gets no verdict and the container's own limit is the
		// only capacity cause on the tick.
		engine, err := NewEngine(4, 3.5)
		Expect(err).NotTo(HaveOccurred())

		base := time.Now()
		sample := func(at time.Time) Sample {
			return Sample{
				Timestamp:   at,
				CpuScope:    ScopeHost,
				Quota:       diagnosis.Known(3.5),
				HostBusy:    diagnosis.Unknown(),
				UsageCores:  diagnosis.Known(3.5),
				LogicalCpus: diagnosis.Known(4),
				HostCpus:    diagnosis.Known(4),
				NrPeriods:   diagnosis.Known(0),
				NrThrottled: diagnosis.Known(0),
			}
		}
		env := DeriveEnvironment(sample(base))

		var verdict Verdict
		var sig Details
		for i := 0; i <= 5; i++ {
			verdict, sig = Decide(engine, sample(base.Add(time.Duration(i)*time.Second)), env)
		}

		// One capacity cause, and it is the limit's.
		Expect(kindsOf(verdict.Causes)).To(Equal([]CauseKind{CauseKindContainerLimitFull}))
		Expect(verdict.Causes[0].Instrument).To(Equal(instLimitHeadroom))

		// The machine question was asked and could not be answered. The
		// estimate is gated off, so host-headroom is the signal's only capable
		// arm and it read nothing.
		sat := signalNamed(cpuTable(4, 3.5), sigHostCpuFull)
		Expect(instrumentNames(sat.Capable(env))).To(Equal([]string{instHostHeadroom}))
		_, _, _, avail := engine.Select(sat, env)
		Expect(avail).To(Equal(diagnosis.AllAbsent))
		fraction, state := engine.Reduction(sigHostCpuFull, instUsageFraction).Get()
		Expect(state).To(Equal(diagnosis.StateValue))
		Expect(fraction).To(BeNumerically("~", 0.875, 1e-9),
			"0.875 is past the 0.70 fire mark, so the gate is what kept the machine silent")

		// Neither customer surface claims anything about the machine's
		// capacity. Both machine-full sentences, detailSatHostFull and
		// detailSatBothAtLimit, open with the same four words.
		msg := ComposeMessage(verdict, sig)
		Expect(msg).NotTo(ContainSubstring("The machine is full"))
		Expect(msg).To(ContainSubstring(detailSatHostUnavail),
			"an unreadable /proc/stat is reported as unreadable, never as a machine that measured fine")
		Expect(BlockReason(verdict.Causes, verdict.Attribution, sig)).
			To(Equal(BlockReason(oneCause(CauseKindContainerLimitFull, instLimitHeadroom), verdict.Attribution, sig)),
				"with one capacity cause there is no pair to blend, so the refusal is the limit's own line")
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
