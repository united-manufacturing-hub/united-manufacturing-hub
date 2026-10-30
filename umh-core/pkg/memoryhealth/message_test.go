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

package memoryhealth

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/diagnosis"
)

const (
	twoGiB     = 2 * bytesPerGiB
	sixteenGiB = 16 * bytesPerGiB
)

func limitDetails(avgLimitUsedFraction float64) Details {
	return Details{
		LimitApplies:         true,
		TotalBytes:           twoGiB,
		HostTotalBytes:       sixteenGiB,
		AvgUsedFraction:      diagnosis.Known(avgLimitUsedFraction),
		AvgUsedBytes:         diagnosis.Known(avgLimitUsedFraction * twoGiB),
		AvgLimitUsedFraction: diagnosis.Known(avgLimitUsedFraction),
		AvgHostUsedFraction:  diagnosis.Known(0.41),
		PressureApplies:      true,
		PressureAvg60:        diagnosis.Known(0.03),
		OomKillsApply:        true,
		OomKillsLast60s:      diagnosis.Known(0),
	}
}

func noLimitDetails(avgHostUsedFraction float64) Details {
	details := limitDetails(0)
	details.LimitApplies = false
	details.TotalBytes = sixteenGiB
	details.AvgLimitUsedFraction = diagnosis.Unknown()
	details.AvgHostUsedFraction = diagnosis.Known(avgHostUsedFraction)

	return details
}

func degradedBy(causes ...Cause) Verdict {
	return Verdict{State: StateDegraded, Causes: causes}
}

func messageLines(message string) []string {
	return strings.Split(message, "\n")
}

func technicalDetailsLines(message string) []string {
	_, table, found := strings.Cut(message, technicalDetailsLabel)
	Expect(found).To(BeTrue())

	return strings.Split(table, "\n")
}

var _ = Describe("ComposeMessage for a degraded verdict", func() {
	DescribeTable("opens with the headline of the top-ranked cause",
		func(kind CauseKind, expectedHeadline string) {
			message := ComposeMessage(degradedBy(Cause{Kind: kind, Value: 0.9}), limitDetails(0.9))

			Expect(messageLines(message)[0]).To(Equal(expectedHeadline))
		},
		Entry("oom-kill", CauseKindOomKill, headlineOomKill),
		Entry("pressure", CauseKindPressure, headlinePressure),
		Entry("container-limit-full", CauseKindContainerLimitFull, headlineLimitFull),
		Entry("host-memory-full", CauseKindHostMemoryFull, headlineHostFull),
	)

	It("explains a full container limit with its numbers and a remedy", func() {
		message := ComposeMessage(degradedBy(Cause{Kind: CauseKindContainerLimitFull, Value: 0.86}), limitDetails(0.86))

		Expect(messageLines(message)[1]).To(Equal(fmt.Sprintf(detailLimitFull, 86, "2.0 GiB")))
	})

	It("explains a full machine with its numbers and a remedy", func() {
		message := ComposeMessage(degradedBy(Cause{Kind: CauseKindHostMemoryFull, Value: 0.92}), noLimitDetails(0.92))

		Expect(messageLines(message)[1]).To(Equal(fmt.Sprintf(detailHostFull, 92, "16.0 GiB")))
	})

	It("explains pressure with the share of the minute spent waiting", func() {
		message := ComposeMessage(degradedBy(Cause{Kind: CauseKindPressure, Value: 0.25}), limitDetails(0.5))

		Expect(messageLines(message)[1]).To(Equal(fmt.Sprintf(detailPressure, 25)))
	})

	DescribeTable("explains OOM kills with their count",
		func(kills float64, expectedCount string) {
			details := limitDetails(0.5)
			details.OomKillsLast60s = diagnosis.Known(kills)

			message := ComposeMessage(degradedBy(Cause{Kind: CauseKindOomKill, Value: kills}), details)

			Expect(messageLines(message)[1]).To(Equal(fmt.Sprintf(detailOomKill, expectedCount)))
		},
		Entry("one kill", 1.0, "1 process"),
		Entry("several kills", 3.0, "3 processes"),
	)

	It("writes one paragraph per cause, the top-ranked first", func() {
		message := ComposeMessage(degradedBy(Cause{Kind: CauseKindOomKill, Value: 1}, Cause{Kind: CauseKindPressure, Value: 0.25}), limitDetails(0.5))

		body, _, _ := strings.Cut(message, technicalDetailsLabel)
		Expect(body).To(Equal(headlineOomKill + "\n" + fmt.Sprintf(detailOomKill, "1 process") + "\n\n" + fmt.Sprintf(detailPressure, 25)))
	})

	It("writes one blended paragraph when the container and the machine are both full", func() {
		causes := degradedBy(Cause{Kind: CauseKindContainerLimitFull, Value: 0.9}, Cause{Kind: CauseKindHostMemoryFull, Value: 0.85})
		details := limitDetails(0.9)
		details.AvgHostUsedFraction = diagnosis.Known(0.85)

		body, _, _ := strings.Cut(ComposeMessage(causes, details), technicalDetailsLabel)

		Expect(body).To(Equal(headlineLimitFull + "\n" + fmt.Sprintf(detailBothFull, 90, "2.0 GiB")))
	})
})

var _ = Describe("ComposeMessage for a healthy verdict", func() {
	It("states the container's budget against its limit", func() {
		message := ComposeMessage(Verdict{State: StateHealthy}, limitDetails(0.55))

		Expect(messageLines(message)[0]).To(Equal(fmt.Sprintf(headlineHealthyLimit, "1.1", "2.0 GiB", 55, "0.5 GiB")))
	})

	It("states the machine's budget without a limit", func() {
		message := ComposeMessage(Verdict{State: StateHealthy}, noLimitDetails(0.41))

		Expect(messageLines(message)[0]).To(Equal(fmt.Sprintf(headlineHealthyMachine, "6.6", "16.0 GiB", "6.2 GiB")))
	})

	It("says the container is close to degraded within the display band", func() {
		message := ComposeMessage(Verdict{State: StateHealthy}, limitDetails(0.78))

		Expect(messageLines(message)[0]).To(Equal(fmt.Sprintf(headlineHealthyLimitClose, "1.6", "2.0 GiB", 78)))
	})

	It("says it is starting up until the usage is measured", func() {
		details := limitDetails(0.5)
		details.AvgUsedBytes = diagnosis.Unknown()
		details.AvgLimitUsedFraction = diagnosis.Unknown()

		message := ComposeMessage(Verdict{State: StateHealthy}, details)

		Expect(messageLines(message)[0]).To(Equal(memoryStartingUp))
		Expect(message).To(ContainSubstring(technicalDetailsLabel))
	})
})

var _ = Describe("the Technical Details table", func() {
	It("lists every rule with its value and the mark that would degrade it", func() {
		lines := technicalDetailsLines(ComposeMessage(Verdict{State: StateHealthy}, limitDetails(0.55)))

		Expect(lines).To(Equal([]string{
			labelInstanceMemory + " 55% of 2.0 GiB limit (degrades at 80%).",
			labelMachineMemory + " 41% of 16.0 GiB (degrades at 80%).",
			labelPressure + " 3% (degrades above 20%).",
			labelOomKills + " 0 in the last minute (degrades at 1).",
		}))
	})

	It("states the mark that would clear a fired rule", func() {
		lines := technicalDetailsLines(ComposeMessage(degradedBy(Cause{Kind: CauseKindContainerLimitFull, Value: 0.86}), limitDetails(0.86)))

		Expect(lines[0]).To(Equal(labelInstanceMemory + " 86% of 2.0 GiB limit (recovers below 75%)."))
	})

	It("says why a rule does not run on this host", func() {
		details := noLimitDetails(0.41)
		details.PressureApplies = false
		details.PressureAvg60 = diagnosis.Unknown()

		lines := technicalDetailsLines(ComposeMessage(Verdict{State: StateHealthy}, details))

		Expect(lines[0]).To(Equal(labelInstanceMemory + " not measured (" + reasonNoMemoryLimit + ")."))
		Expect(lines[2]).To(Equal(labelPressure + " not measured (" + reasonNoPressureStats + ")."))
	})

	It("says a rule is still measuring while its window fills", func() {
		details := limitDetails(0.55)
		details.OomKillsLast60s = diagnosis.Unknown()

		lines := technicalDetailsLines(ComposeMessage(Verdict{State: StateHealthy}, details))

		Expect(lines[3]).To(Equal(labelOomKills + " not available (measuring)."))
	})
})
