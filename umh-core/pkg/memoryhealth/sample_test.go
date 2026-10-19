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
	"context"
	"io/fs"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/memoryhealth"
)

func readSample(files map[string]string, hostMemory memoryhealth.HostMemoryReader) (memoryhealth.Sample, error) {
	sampler := memoryhealth.NewSampler(fixtureFilesystem(files), fixtureCgroupBase, hostMemory)

	return sampler.Read(context.Background())
}

var _ = Describe("Sampler", func() {
	It("reads a cgroup with a limit without the host", func() {
		sample, err := readSample(cgroupFiles(bytesText(oneGiBBytes), bytesText(halfGiBBytes)), unreadableHostMemory)

		Expect(err).ToNot(HaveOccurred())
		Expect(sample.Source).To(Equal(memoryhealth.SourceCgroupLimit))
		Expect(sample.UsedBytes).To(Equal(halfGiBBytes))
		Expect(sample.TotalBytes).To(Equal(oneGiBBytes))
		Expect(sample.Timestamp).ToNot(BeZero())
	})

	DescribeTable("measures a cgroup without a limit against the host total",
		func(memoryMax string) {
			sample, err := readSample(cgroupFiles(memoryMax, bytesText(halfGiBBytes)), hostMemoryOf(threeGiBBytes, eightGiBBytes))

			Expect(err).ToNot(HaveOccurred())
			Expect(sample.Source).To(Equal(memoryhealth.SourceHostTotalNoCgroupLimit))
			Expect(sample.UsedBytes).To(Equal(halfGiBBytes))
			Expect(sample.TotalBytes).To(Equal(eightGiBBytes))
		},
		Entry("memory.max is max", "max\n"),
		Entry("memory.max is 0", "0\n"),
	)

	It("fails a cgroup without a limit when the host total is unreadable", func() {
		sample, err := readSample(cgroupFiles("max\n", bytesText(halfGiBBytes)), unreadableHostMemory)

		Expect(err).To(MatchError(errHostUnreadable))
		Expect(sample.HostReadErr).To(MatchError(errHostUnreadable))
	})

	DescribeTable("uses the host's memory when the cgroup is unreadable",
		func(files map[string]string) {
			sample, err := readSample(files, hostMemoryOf(threeGiBBytes, eightGiBBytes))

			Expect(err).ToNot(HaveOccurred())
			Expect(sample.Source).To(Equal(memoryhealth.SourceHostFallbackCgroupUnreadable))
			Expect(sample.UsedBytes).To(Equal(threeGiBBytes))
			Expect(sample.TotalBytes).To(Equal(eightGiBBytes))
			Expect(sample.CgroupReadErr).To(HaveOccurred())
		},
		Entry("no cgroup files", map[string]string{}),
		Entry("an unparsable memory.current", cgroupFiles(bytesText(oneGiBBytes), "abc\n")),
	)

	It("fails when both the cgroup and the host are unreadable", func() {
		sample, err := readSample(map[string]string{}, unreadableHostMemory)

		Expect(err).To(MatchError(errHostUnreadable))
		Expect(err).To(MatchError(fs.ErrNotExist))
		Expect(sample.CgroupReadErr).To(MatchError(fs.ErrNotExist))
		Expect(sample.HostReadErr).To(MatchError(errHostUnreadable))
	})

	It("fails when the host reports a zero total", func() {
		sample, err := readSample(map[string]string{}, hostMemoryOf(0, 0))

		Expect(err).To(MatchError(memoryhealth.ErrZeroTotal))
		Expect(sample.Source).To(Equal(memoryhealth.SourceHostFallbackCgroupUnreadable))
	})

	It("returns the context error on a cancelled read", func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		sampler := memoryhealth.NewSampler(fixtureFilesystem(cgroupFiles(bytesText(oneGiBBytes), bytesText(halfGiBBytes))), fixtureCgroupBase, hostMemoryOf(threeGiBBytes, eightGiBBytes))

		_, err := sampler.Read(ctx)

		Expect(err).To(MatchError(context.Canceled))
	})
})
