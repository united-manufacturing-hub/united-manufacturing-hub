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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

var _ = Describe("the memory worker's gauges", func() {
	It("publishes a cgroup reading", func() {
		memoryDeps := newTestDeps(fixtureFilesystem(cgroupFiles(oneGiBText, halfGiBText)), unreadableHostMemory)

		_, err := Poll(context.Background(), memoryDeps, MemoryConfig{})
		Expect(err).ToNot(HaveOccurred())

		gauges := memoryDeps.MetricsRecorder().Drain().Gauges

		Expect(gauges).To(HaveKeyWithValue(string(deps.GaugeMemoryUsedBytes), float64(oneGiB/2)))
		Expect(gauges).To(HaveKeyWithValue(string(deps.GaugeMemoryTotalBytes), float64(oneGiB)))
		Expect(gauges).To(HaveKeyWithValue(string(deps.GaugeMemoryUsedPercent), 50.0))
		Expect(gauges).To(HaveKeyWithValue(string(deps.GaugeMemorySourceIsCgroup), 1.0))
		Expect(gauges[string(deps.GaugeMemoryLastSampleUnix)]).To(BeNumerically("~", float64(time.Now().Unix()), 5))
	})

	It("marks a host reading", func() {
		memoryDeps := newTestDeps(fixtureFilesystem(map[string]string{}), hostMemoryOf(threeGiBHost, eightGiBHost))

		_, err := Poll(context.Background(), memoryDeps, MemoryConfig{})
		Expect(err).ToNot(HaveOccurred())

		Expect(memoryDeps.MetricsRecorder().Drain().Gauges).To(HaveKeyWithValue(string(deps.GaugeMemorySourceIsCgroup), 0.0))
	})

	It("publishes nothing when the poll fails", func() {
		memoryDeps := newTestDeps(fixtureFilesystem(map[string]string{}), unreadableHostMemory)

		_, err := Poll(context.Background(), memoryDeps, MemoryConfig{})
		Expect(err).To(HaveOccurred())

		Expect(memoryDeps.MetricsRecorder().Drain().Gauges).To(BeEmpty())
	})
})
