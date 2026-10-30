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

package fsmv2memory

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
)

func drainedGaugesAfterPolls(files map[string]string, polls int) map[string]float64 {
	memoryDeps := newTestDeps(fixtureFilesystem(files), hostMemoryOf(hostUsedBytes, hostTotalBytes))

	for range polls {
		_, err := Poll(context.Background(), memoryDeps, MemoryConfig{})
		Expect(err).ToNot(HaveOccurred())
	}

	return memoryDeps.MetricsRecorder().Drain().Gauges
}

var _ = Describe("the memory worker's gauges", func() {
	It("publishes the averages of a cgroup reading as a fraction", func() {
		gauges := drainedGaugesAfterPolls(cgroupAtPercent(50), 2)

		Expect(gauges).To(HaveKeyWithValue(string(deps.GaugeMemoryAvgUsedBytes), float64(decimalLimitBytes/2)))
		Expect(gauges).To(HaveKeyWithValue(string(deps.GaugeMemoryTotalBytes), float64(decimalLimitBytes)))
		Expect(gauges).To(HaveKeyWithValue(string(deps.GaugeMemoryAvgUsedFraction), 0.5))
		Expect(gauges).To(HaveKeyWithValue(string(deps.GaugeMemoryUsedRingActive), 1.0))
		Expect(gauges).To(HaveKeyWithValue(string(deps.GaugeMemoryUsedFromCgroup), 1.0))
		Expect(gauges).To(HaveKeyWithValue(string(deps.GaugeMemoryTotalIsCgroupLimit), 1.0))
		Expect(gauges[string(deps.GaugeMemoryLastSampleUnix)]).To(BeNumerically("~", float64(time.Now().Unix()), 5))
	})

	It("marks the averages as not ready after a single poll", func() {
		gauges := drainedGaugesAfterPolls(cgroupAtPercent(50), 1)

		Expect(gauges).To(HaveKeyWithValue(string(deps.GaugeMemoryUsedRingActive), 0.0))
		Expect(gauges).To(HaveKeyWithValue(string(deps.GaugeMemoryTotalBytes), float64(decimalLimitBytes)))
	})

	DescribeTable("marks which source each number came from",
		func(files map[string]string, expectedUsedFromCgroup, expectedTotalIsCgroupLimit float64) {
			gauges := drainedGaugesAfterPolls(files, 1)

			Expect(gauges).To(HaveKeyWithValue(string(deps.GaugeMemoryUsedFromCgroup), expectedUsedFromCgroup))
			Expect(gauges).To(HaveKeyWithValue(string(deps.GaugeMemoryTotalIsCgroupLimit), expectedTotalIsCgroupLimit))
		},
		Entry("a cgroup without a limit", unlimitedCgroup, 1.0, 0.0),
		Entry("an unreadable cgroup", missingCgroup, 0.0, 0.0),
	)

	It("publishes nothing when the poll fails", func() {
		memoryDeps := newTestDeps(fixtureFilesystem(missingCgroup), unreadableHostMemory)

		_, err := Poll(context.Background(), memoryDeps, MemoryConfig{})
		Expect(err).To(HaveOccurred())

		Expect(memoryDeps.MetricsRecorder().Drain().Gauges).To(BeEmpty())
	})
})
