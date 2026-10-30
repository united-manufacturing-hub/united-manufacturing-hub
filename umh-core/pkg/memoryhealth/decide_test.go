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

package memoryhealth_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/diagnosis"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/memoryhealth"
)

const (
	decimalTotalBytes   int64 = 1000000000
	decimalMachineBytes int64 = 8000000000

	twoMinutesOfSamples = 120
)

var firstSampleTime = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

type memoryJudge struct {
	engine     *diagnosis.Engine[memoryhealth.Sample]
	sampleTime time.Time
}

func newMemoryJudge() *memoryJudge {
	engine, err := memoryhealth.NewEngine()
	Expect(err).ToNot(HaveOccurred())

	return &memoryJudge{engine: engine, sampleTime: firstSampleTime}
}

func percentOf(totalBytes, percent int64) int64 {
	return totalBytes * percent / 100
}

func containerAtPercent(usedPercent int64) memoryhealth.Sample {
	return memoryhealth.Sample{
		Source:     memoryhealth.SourceCgroupLimit,
		UsedBytes:  percentOf(decimalTotalBytes, usedPercent),
		TotalBytes: decimalTotalBytes,
	}
}

func containerAndMachineAtPercent(containerPercent, machinePercent int64) memoryhealth.Sample {
	sample := containerAtPercent(containerPercent)
	sample.HostUsedBytes = percentOf(decimalMachineBytes, machinePercent)
	sample.HostTotalBytes = decimalMachineBytes

	return sample
}

func unlimitedContainerOnMachineAtPercent(containerPercent, machinePercent int64) memoryhealth.Sample {
	return memoryhealth.Sample{
		Source:         memoryhealth.SourceHostTotalNoCgroupLimit,
		UsedBytes:      percentOf(decimalMachineBytes, containerPercent),
		TotalBytes:     decimalMachineBytes,
		HostUsedBytes:  percentOf(decimalMachineBytes, machinePercent),
		HostTotalBytes: decimalMachineBytes,
	}
}

func (j *memoryJudge) decideSample(sample memoryhealth.Sample) (memoryhealth.Verdict, memoryhealth.Details) {
	sample.Timestamp = j.sampleTime
	j.sampleTime = j.sampleTime.Add(time.Second)

	return memoryhealth.Decide(j.engine, sample)
}

func (j *memoryJudge) decideSampleRepeatedly(sample memoryhealth.Sample, samples int) (memoryhealth.Verdict, memoryhealth.Details) {
	verdict, details := j.decideSample(sample)
	for range samples - 1 {
		verdict, details = j.decideSample(sample)
	}

	return verdict, details
}

func (j *memoryJudge) decide(usedPercent int64) (memoryhealth.Verdict, memoryhealth.Details) {
	return j.decideSample(containerAtPercent(usedPercent))
}

func causeKindsOf(verdict memoryhealth.Verdict) []memoryhealth.CauseKind {
	kinds := make([]memoryhealth.CauseKind, len(verdict.Causes))
	for index, cause := range verdict.Causes {
		kinds[index] = cause.Kind
	}

	return kinds
}

func (j *memoryJudge) decideRepeatedly(usedPercent int64, samples int) (memoryhealth.Verdict, memoryhealth.Details) {
	verdict, details := j.decide(usedPercent)
	for range samples - 1 {
		verdict, details = j.decide(usedPercent)
	}

	return verdict, details
}

func usedFractionOf(details memoryhealth.Details) float64 {
	fraction, measured := details.AvgUsedFraction.Get()
	Expect(measured).To(BeTrue())

	return fraction
}

var _ = Describe("Decide", func() {
	It("has not measured the memory after a single sample", func() {
		verdict, details := newMemoryJudge().decide(95)

		Expect(details.Measured()).To(BeFalse())
		Expect(verdict.State).To(Equal(memoryhealth.StateHealthy))
	})

	It("is healthy below the degraded level", func() {
		verdict, details := newMemoryJudge().decideRepeatedly(79, 10)

		Expect(verdict.State).To(Equal(memoryhealth.StateHealthy))
		Expect(verdict.Causes).To(BeEmpty())
		Expect(usedFractionOf(details)).To(BeNumerically("~", 0.79, 1e-9))
	})

	It("degrades once the windowed usage reaches the degraded level", func() {
		verdict, details := newMemoryJudge().decideRepeatedly(80, 2)

		Expect(details.Measured()).To(BeTrue())
		Expect(verdict.State).To(Equal(memoryhealth.StateDegraded))
		Expect(verdict.Causes).To(HaveLen(1))
		Expect(verdict.Causes[0].Kind).To(Equal(memoryhealth.CauseKindContainerLimitFull))
		Expect(verdict.Causes[0].Value).To(BeNumerically("~", 0.80, 1e-9))
	})

	It("does not degrade on one sample above the degraded level inside a lower window", func() {
		judge := newMemoryJudge()
		judge.decideRepeatedly(50, twoMinutesOfSamples)

		verdict, details := judge.decide(99)

		Expect(verdict.State).To(Equal(memoryhealth.StateHealthy))
		Expect(usedFractionOf(details)).To(BeNumerically("<", 0.80))
	})

	It("does not degrade on a 15-second burst above the degraded level", func() {
		judge := newMemoryJudge()
		judge.decideRepeatedly(50, twoMinutesOfSamples)

		verdict, details := judge.decideRepeatedly(99, 15)

		Expect(verdict.State).To(Equal(memoryhealth.StateHealthy))
		Expect(usedFractionOf(details)).To(BeNumerically("<", 0.80))
	})

	It("stays degraded while the windowed usage sits between the recovery and degraded levels", func() {
		judge := newMemoryJudge()
		judge.decideRepeatedly(90, twoMinutesOfSamples)

		verdict, details := judge.decideRepeatedly(77, twoMinutesOfSamples)

		Expect(verdict.State).To(Equal(memoryhealth.StateDegraded))
		Expect(usedFractionOf(details)).To(BeNumerically("~", 0.77, 1e-9))
	})

	It("averages the used bytes over the window", func() {
		judge := newMemoryJudge()
		judge.decideRepeatedly(90, twoMinutesOfSamples)

		_, details := judge.decideRepeatedly(30, twoMinutesOfSamples)

		avgUsedBytes, measured := details.AvgUsedBytes.Get()
		Expect(measured).To(BeTrue())
		Expect(avgUsedBytes).To(BeNumerically("~", float64(decimalTotalBytes)*0.30, 1))
	})

	It("has no average of the used bytes after a single sample", func() {
		_, details := newMemoryJudge().decide(50)

		_, measured := details.AvgUsedBytes.Get()
		Expect(measured).To(BeFalse())
	})

	It("recovers once the windowed usage falls below the recovery level", func() {
		judge := newMemoryJudge()
		judge.decideRepeatedly(90, twoMinutesOfSamples)

		verdict, _ := judge.decideRepeatedly(70, twoMinutesOfSamples)

		Expect(verdict.State).To(Equal(memoryhealth.StateHealthy))
	})
})

var _ = Describe("Decide for the container and the machine", func() {
	It("degrades with host-memory-full when the machine is full and the container is below its limit", func() {
		verdict, details := newMemoryJudge().decideSampleRepeatedly(containerAndMachineAtPercent(50, 90), twoMinutesOfSamples)

		Expect(verdict.State).To(Equal(memoryhealth.StateDegraded))
		Expect(causeKindsOf(verdict)).To(ConsistOf(memoryhealth.CauseKindHostMemoryFull))
		Expect(verdict.Causes[0].Value).To(BeNumerically("~", 0.90, 1e-9))

		avgHostUsedFraction, measured := details.AvgHostUsedFraction.Get()
		Expect(measured).To(BeTrue())
		Expect(avgHostUsedFraction).To(BeNumerically("~", 0.90, 1e-9))
	})

	It("degrades with both causes when the container and the machine are full", func() {
		verdict, _ := newMemoryJudge().decideSampleRepeatedly(containerAndMachineAtPercent(90, 90), twoMinutesOfSamples)

		Expect(causeKindsOf(verdict)).To(ConsistOf(memoryhealth.CauseKindContainerLimitFull, memoryhealth.CauseKindHostMemoryFull))
	})

	It("is healthy when the container and the machine are below the degraded level", func() {
		verdict, _ := newMemoryJudge().decideSampleRepeatedly(containerAndMachineAtPercent(50, 50), twoMinutesOfSamples)

		Expect(verdict.State).To(Equal(memoryhealth.StateHealthy))
	})

	It("judges a container without a limit only against the full machine", func() {
		verdict, _ := newMemoryJudge().decideSampleRepeatedly(unlimitedContainerOnMachineAtPercent(10, 90), twoMinutesOfSamples)

		Expect(causeKindsOf(verdict)).To(ConsistOf(memoryhealth.CauseKindHostMemoryFull))
	})
})

func containerUnderPressure(pressure float64) memoryhealth.Sample {
	sample := containerAtPercent(50)
	sample.Pressure = diagnosis.Known(pressure)

	return sample
}

var _ = Describe("Decide for memory pressure", func() {
	It("degrades with pressure once some avg60 passes 20 percent", func() {
		verdict, details := newMemoryJudge().decideSample(containerUnderPressure(0.25))

		Expect(causeKindsOf(verdict)).To(ConsistOf(memoryhealth.CauseKindPressure))
		Expect(verdict.Causes[0].Value).To(BeNumerically("~", 0.25, 1e-9))

		pressure, measured := details.PressureAvg60.Get()
		Expect(measured).To(BeTrue())
		Expect(pressure).To(BeNumerically("~", 0.25, 1e-9))
	})

	It("stays healthy below 20 percent", func() {
		verdict, _ := newMemoryJudge().decideSample(containerUnderPressure(0.15))

		Expect(verdict.State).To(Equal(memoryhealth.StateHealthy))
	})

	It("stays degraded until the pressure falls below 12 percent", func() {
		judge := newMemoryJudge()
		judge.decideSample(containerUnderPressure(0.25))

		stillDegraded, _ := judge.decideSampleRepeatedly(containerUnderPressure(0.15), twoMinutesOfSamples)
		Expect(causeKindsOf(stillDegraded)).To(ConsistOf(memoryhealth.CauseKindPressure))

		recovered, _ := judge.decideSampleRepeatedly(containerUnderPressure(0.10), twoMinutesOfSamples)
		Expect(recovered.State).To(Equal(memoryhealth.StateHealthy))
	})
})

func containerWithNewOomKills(newOomKills float64) memoryhealth.Sample {
	sample := containerAtPercent(50)
	sample.NewOomKills = diagnosis.Known(newOomKills)

	return sample
}

var _ = Describe("Decide for OOM kills", func() {
	It("degrades with oom-kill on a new OOM kill", func() {
		judge := newMemoryJudge()
		judge.decideSample(containerWithNewOomKills(0))

		verdict, details := judge.decideSample(containerWithNewOomKills(1))

		Expect(causeKindsOf(verdict)).To(ConsistOf(memoryhealth.CauseKindOomKill))

		oomKills, measured := details.OomKillsLast60s.Get()
		Expect(measured).To(BeTrue())
		Expect(oomKills).To(Equal(1.0))
	})

	It("counts every OOM kill in the window", func() {
		judge := newMemoryJudge()
		judge.decideSample(containerWithNewOomKills(2))

		_, details := judge.decideSample(containerWithNewOomKills(1))

		oomKills, _ := details.OomKillsLast60s.Get()
		Expect(oomKills).To(Equal(3.0))
	})

	It("recovers once a whole window passes without an OOM kill", func() {
		judge := newMemoryJudge()
		judge.decideSample(containerWithNewOomKills(1))

		verdict, _ := judge.decideSampleRepeatedly(containerWithNewOomKills(0), twoMinutesOfSamples)

		Expect(verdict.State).To(Equal(memoryhealth.StateHealthy))
	})

	It("is healthy without OOM kills", func() {
		verdict, _ := newMemoryJudge().decideSampleRepeatedly(containerWithNewOomKills(0), twoMinutesOfSamples)

		Expect(verdict.State).To(Equal(memoryhealth.StateHealthy))
	})
})
