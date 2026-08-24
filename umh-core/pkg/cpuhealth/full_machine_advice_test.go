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

// The advice a full machine earns, by whose load filled it. Every spec here
// drives real ticks through Decide and reads the rendered text: the switch
// being right proves nothing if the tick never reaches it.
//
// A limit is in force in all four scenarios, because that is the mode whose
// paragraph splits. The scenarios differ only in our own usage against the
// machine's busy time, which is the share the two refinements read.

package cpuhealth

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// fullMachineRun drives six one-second ticks on a four-core box with a
// four-core limit, at a fixed machine busy time and usage, and returns the last
// tick's verdict and details.
//
// host-headroom is 4 - hostBusy - 1.0, so a busy time of 3.2 leaves -0.2 and
// the machine fires. The limit is four cores against a usage well under it, so
// container-limit-full stays quiet and the machine cause stands alone. Only
// HasLimit is granted: the estimate from our own usage needs
// HasLimitedVisibility, so host-headroom is the only arm that can answer.
func fullMachineRun(hostBusy float64, usage diagnosis.Reading) (Verdict, Details) {
	engine, err := NewEngine(4, 4.0)
	Expect(err).NotTo(HaveOccurred())
	env := diagnosis.NewEnvironment(HasLimit)
	base := time.Now()

	var verdict Verdict
	var signals Details
	for i := 0; i <= 5; i++ {
		verdict, signals = Decide(engine, Sample{
			Timestamp:   base.Add(time.Duration(i) * time.Second),
			CpuScope:    ScopeHost,
			Quota:       diagnosis.Known(4.0),
			HostBusy:    diagnosis.Known(hostBusy),
			UsageCores:  usage,
			LogicalCpus: diagnosis.Known(4),
			HostCpus:    diagnosis.Known(4),
		}, env)
	}

	return verdict, signals
}

var _ = Describe("the advice on a full machine follows whose load filled it", func() {
	It("should tell a customer to reduce other software when the rest of the box filled the machine", func() {
		// Machine busy 3.2 against our 1.0 is a share of 0.3125, past
		// host-share's 0.49 fire mark, so the rest of the box accounts for most
		// of the busy time.
		verdict, signals := fullMachineRun(3.2, diagnosis.Known(1.0))

		Expect(kindsOf(verdict.Causes)).To(Equal([]CauseKind{CauseKindHostCpuFull}),
			"the machine cause must stand alone, or the blend answers instead of the split")
		Expect(verdict.Causes[0].Instrument).To(Equal(instrumentHostHeadroom))
		Expect(signals.LimitApplies).To(BeTrue(), "the paragraph that splits is the limit-mode one")
		Expect(verdict.Attribution).To(Equal(AttributionHost))

		Expect(ComposeMessage(verdict, signals)).To(ContainSubstring(
			"The machine is full. Add CPU to the machine, or reduce other software running on it."))
		Expect(BlockReason(verdict.Causes, verdict.Attribution, signals)).To(Equal(
			"Can't add another bridge: the machine is full. Add CPU to the machine, or reduce other software running on it, first."))
	})

	It("should tell a customer to reduce their own load when this instance filled the machine", func() {
		// Machine busy 3.2 against our 2.0 is a share of 0.625, past
		// container-share's 0.51 fire mark, so the load is ours. Usage 2.0 sits
		// well inside the four-core limit, so only the machine fires.
		verdict, signals := fullMachineRun(3.2, diagnosis.Known(2.0))

		Expect(kindsOf(verdict.Causes)).To(Equal([]CauseKind{CauseKindHostCpuFull}),
			"the machine cause must stand alone, or the blend answers instead of the split")
		Expect(verdict.Causes[0].Instrument).To(Equal(instrumentHostHeadroom))
		Expect(signals.LimitApplies).To(BeTrue())
		Expect(verdict.Attribution).To(Equal(AttributionContainer))

		msg := ComposeMessage(verdict, signals)
		Expect(msg).To(ContainSubstring(
			"The machine is full, and this instance is using most of it. Reduce the load on this instance, or add CPU to the machine."))
		Expect(msg).NotTo(ContainSubstring("reduce other software running on it"),
			"the load is our own, so sending the customer after other people's software is wrong advice")
		Expect(BlockReason(verdict.Causes, verdict.Attribution, signals)).To(Equal(
			"Can't add another bridge: the machine is full, and this instance is using most of it. Reduce the load on this instance, or add CPU to the machine, first."))
	})

	It("should name nobody when the machine is full and the share cannot be measured", func() {
		// Our own usage is absent while the machine's busy time answers, so the
		// share has no numerator and neither refinement can fire. This is the
		// cleaner of the two unknown routes: it holds on every tick, where the
		// dead band between 0.49 and 0.51 holds only until one refinement has
		// fired once.
		verdict, signals := fullMachineRun(3.2, diagnosis.Unknown())

		Expect(kindsOf(verdict.Causes)).To(Equal([]CauseKind{CauseKindHostCpuFull}),
			"the machine cause must stand alone, or the blend answers instead of the split")
		Expect(verdict.Causes[0].Instrument).To(Equal(instrumentHostHeadroom))
		Expect(signals.LimitApplies).To(BeTrue())
		Expect(verdict.Attribution).To(Equal(AttributionUnknown))

		msg := ComposeMessage(verdict, signals)
		Expect(msg).To(ContainSubstring(
			"The machine is full. Add CPU to the machine, or reduce what is running on it."))
		Expect(msg).NotTo(ContainSubstring("other software"),
			"nothing placed the load on either side, so the sentence claims nothing about who")
		Expect(BlockReason(verdict.Causes, verdict.Attribution, signals)).To(Equal(
			"Can't add another bridge: the machine is full. Add CPU to the machine, or reduce what is running on it, first."))
	})

	It("should keep the blended sentence when the container is also at its limit, whatever the share says", func() {
		// A four-core box with a 2.0-core limit, machine busy 3.8, our usage
		// 1.95: host-headroom is -0.8 and limit-headroom is -0.15, so both
		// capacity causes fire. The share is 1.95 / 3.80 = 0.5132, past
		// container-share's fire mark, so the attribution says container and
		// the split would change the paragraph if it ran first.
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasLimit)
		base := time.Now()

		var verdict Verdict
		var signals Details
		for i := 0; i <= 5; i++ {
			verdict, signals = Decide(engine, Sample{
				Timestamp:   base.Add(time.Duration(i) * time.Second),
				CpuScope:    ScopeHost,
				Quota:       diagnosis.Known(2.0),
				HostBusy:    diagnosis.Known(3.8),
				UsageCores:  diagnosis.Known(1.95),
				LogicalCpus: diagnosis.Known(4),
				HostCpus:    diagnosis.Known(4),
			}, env)
		}

		Expect(kindsOf(verdict.Causes)).To(ConsistOf(CauseKindHostCpuFull, CauseKindContainerLimitFull),
			"both capacity causes must fire, or this spec is not the blend")
		Expect(verdict.Attribution).To(Equal(AttributionContainer),
			"the attribution must say container, or the blend is not being asked to beat it")

		msg := ComposeMessage(verdict, signals)
		Expect(msg).To(ContainSubstring(
			"The machine is full and this instance's CPU limit cannot help. Add CPU to the machine, or reduce other software running on it. (This instance is also at its 2-core limit.)"))
		Expect(msg).NotTo(ContainSubstring("this instance is using most of it"),
			"the pair is answered by one blended sentence; the attribution split applies to a machine cause standing alone")
		Expect(BlockReason(verdict.Causes, verdict.Attribution, signals)).To(Equal(
			"Can't add another bridge: the machine is full. Add CPU to the machine, or reduce other software running on it, first."),
			"the refusal stays in step with the blended paragraph it is shown beside")
	})
})
