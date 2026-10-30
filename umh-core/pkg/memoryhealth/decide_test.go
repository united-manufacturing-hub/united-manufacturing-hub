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
	decimalTotalBytes int64 = 1000000000

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

func (j *memoryJudge) decide(usedPercent int64) (memoryhealth.Verdict, memoryhealth.Details) {
	sample := memoryhealth.Sample{
		Timestamp:  j.sampleTime,
		Source:     memoryhealth.SourceCgroupLimit,
		UsedBytes:  decimalTotalBytes * usedPercent / 100,
		TotalBytes: decimalTotalBytes,
	}
	j.sampleTime = j.sampleTime.Add(time.Second)

	return memoryhealth.Decide(j.engine, sample)
}

func (j *memoryJudge) decideRepeatedly(usedPercent int64, samples int) (memoryhealth.Verdict, memoryhealth.Details) {
	verdict, details := j.decide(usedPercent)
	for range samples - 1 {
		verdict, details = j.decide(usedPercent)
	}

	return verdict, details
}

func usedFractionOf(details memoryhealth.Details) float64 {
	fraction, measured := details.UsedFraction.Get()
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
		Expect(verdict.Causes[0].Kind).To(Equal(memoryhealth.CauseKindLimitFull))
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

	It("recovers once the windowed usage falls below the recovery level", func() {
		judge := newMemoryJudge()
		judge.decideRepeatedly(90, twoMinutesOfSamples)

		verdict, _ := judge.decideRepeatedly(70, twoMinutesOfSamples)

		Expect(verdict.State).To(Equal(memoryhealth.StateHealthy))
	})
})
