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
	"errors"
	"io/fs"
	"maps"

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

	It("reads the machine's memory beside a cgroup with a limit", func() {
		sample, err := readSample(cgroupFiles(bytesText(oneGiBBytes), bytesText(halfGiBBytes)), hostMemoryOf(threeGiBBytes, eightGiBBytes))

		Expect(err).ToNot(HaveOccurred())
		Expect(sample.Source).To(Equal(memoryhealth.SourceCgroupLimit))
		Expect(sample.HostUsedBytes).To(Equal(threeGiBBytes))
		Expect(sample.HostTotalBytes).To(Equal(eightGiBBytes))
	})

	It("keeps a cgroup with a limit when the machine's memory is unreadable", func() {
		sample, err := readSample(cgroupFiles(bytesText(oneGiBBytes), bytesText(halfGiBBytes)), unreadableHostMemory)

		Expect(err).ToNot(HaveOccurred())
		Expect(sample.HostReadErr).To(MatchError(errHostUnreadable))
		Expect(sample.HostTotalBytes).To(BeZero())
	})

	It("reads the some avg60 of memory.pressure as a fraction", func() {
		files := withFile(cgroupFiles(bytesText(oneGiBBytes), bytesText(halfGiBBytes)), "memory.pressure", memoryPressureText("25.00"))

		sample, err := readSample(files, hostMemoryOf(threeGiBBytes, eightGiBBytes))

		Expect(err).ToNot(HaveOccurred())
		pressure, measured := sample.Pressure.Get()
		Expect(measured).To(BeTrue())
		Expect(pressure).To(BeNumerically("~", 0.25, 1e-9))
	})

	DescribeTable("leaves the pressure unknown without a readable memory.pressure",
		func(files map[string]string) {
			sample, err := readSample(files, hostMemoryOf(threeGiBBytes, eightGiBBytes))

			Expect(err).ToNot(HaveOccurred())
			_, measured := sample.Pressure.Get()
			Expect(measured).To(BeFalse())
		},
		Entry("no memory.pressure", cgroupFiles(bytesText(oneGiBBytes), bytesText(halfGiBBytes))),
		Entry("no some line", withFile(cgroupFiles(bytesText(oneGiBBytes), bytesText(halfGiBBytes)), "memory.pressure", "full avg10=0.00 avg60=1.00 avg300=0.00 total=0\n")),
		Entry("an unparsable avg60", withFile(cgroupFiles(bytesText(oneGiBBytes), bytesText(halfGiBBytes)), "memory.pressure", memoryPressureText("abc"))),
		Entry("cgroup v1", withFile(cgroupV1Files(bytesText(oneGiBBytes), bytesText(halfGiBBytes)), "memory.pressure", memoryPressureText("25.00"))),
	)

	DescribeTable("counts the OOM kills since the previous read",
		func(baseFiles map[string]string, countFile string, countText func(int) string) {
			files := withFile(baseFiles, countFile, countText(3))
			sampler := memoryhealth.NewSampler(fixtureFilesystem(files), fixtureCgroupBase, hostMemoryOf(threeGiBBytes, eightGiBBytes))

			firstSample, err := sampler.Read(context.Background())
			Expect(err).ToNot(HaveOccurred())
			_, measured := firstSample.NewOomKills.Get()
			Expect(measured).To(BeFalse())

			files[fixtureCgroupBase+"/"+countFile] = countText(5)

			secondSample, err := sampler.Read(context.Background())
			Expect(err).ToNot(HaveOccurred())
			newOomKills, measured := secondSample.NewOomKills.Get()
			Expect(measured).To(BeTrue())
			Expect(newOomKills).To(Equal(2.0))
		},
		Entry("cgroup v2 memory.events", cgroupFiles(bytesText(oneGiBBytes), bytesText(halfGiBBytes)), "memory.events", memoryEventsText),
		Entry("cgroup v1 memory.oom_control", cgroupV1Files(bytesText(oneGiBBytes), bytesText(halfGiBBytes)), "memory/memory.oom_control", oomControlText),
	)

	It("leaves the new OOM kills unknown when the counter goes down", func() {
		files := withFile(cgroupFiles(bytesText(oneGiBBytes), bytesText(halfGiBBytes)), "memory.events", memoryEventsText(5))
		sampler := memoryhealth.NewSampler(fixtureFilesystem(files), fixtureCgroupBase, hostMemoryOf(threeGiBBytes, eightGiBBytes))

		_, err := sampler.Read(context.Background())
		Expect(err).ToNot(HaveOccurred())

		files[fixtureCgroupBase+"/memory.events"] = memoryEventsText(1)

		sample, err := sampler.Read(context.Background())
		Expect(err).ToNot(HaveOccurred())
		_, measured := sample.NewOomKills.Get()
		Expect(measured).To(BeFalse())
	})

	It("leaves the new OOM kills unknown without memory.events", func() {
		sampler := memoryhealth.NewSampler(fixtureFilesystem(cgroupFiles(bytesText(oneGiBBytes), bytesText(halfGiBBytes))), fixtureCgroupBase, hostMemoryOf(threeGiBBytes, eightGiBBytes))

		_, _ = sampler.Read(context.Background())
		sample, err := sampler.Read(context.Background())

		Expect(err).ToNot(HaveOccurred())
		_, measured := sample.NewOomKills.Get()
		Expect(measured).To(BeFalse())
	})

	It("reads a cgroup v1 with a limit without the host", func() {
		sample, err := readSample(cgroupV1Files(bytesText(oneGiBBytes), bytesText(halfGiBBytes)), unreadableHostMemory)

		Expect(err).ToNot(HaveOccurred())
		Expect(sample.Source).To(Equal(memoryhealth.SourceCgroupLimit))
		Expect(sample.UsedBytes).To(Equal(halfGiBBytes))
		Expect(sample.TotalBytes).To(Equal(oneGiBBytes))
	})

	DescribeTable("measures a cgroup v1 without a limit against the host total",
		func(limitInBytes string) {
			sample, err := readSample(cgroupV1Files(limitInBytes, bytesText(halfGiBBytes)), hostMemoryOf(threeGiBBytes, eightGiBBytes))

			Expect(err).ToNot(HaveOccurred())
			Expect(sample.Source).To(Equal(memoryhealth.SourceHostTotalNoCgroupLimit))
			Expect(sample.UsedBytes).To(Equal(halfGiBBytes))
			Expect(sample.TotalBytes).To(Equal(eightGiBBytes))
		},
		Entry("4 KiB pages", "9223372036854771712\n"),
		Entry("16 KiB pages", "9223372036854759424\n"),
		Entry("64 KiB pages", "9223372036854710272\n"),
	)

	It("names the v1 file that could not be parsed", func() {
		sample, err := readSample(cgroupV1Files(bytesText(oneGiBBytes), "abc\n"), hostMemoryOf(threeGiBBytes, eightGiBBytes))

		Expect(err).ToNot(HaveOccurred())
		Expect(sample.Source).To(Equal(memoryhealth.SourceHostFallbackCgroupUnreadable))

		var cgroupErr *memoryhealth.CgroupReadError
		Expect(errors.As(sample.CgroupReadErr, &cgroupErr)).To(BeTrue())
		Expect(cgroupErr.File).To(Equal("memory/memory.usage_in_bytes"))
		Expect(cgroupErr.Outcome).To(Equal(memoryhealth.ReadUnparsable))
	})

	It("detects the cgroup version again on the next read while no version was found", func() {
		files := map[string]string{}
		sampler := memoryhealth.NewSampler(fixtureFilesystem(files), fixtureCgroupBase, hostMemoryOf(threeGiBBytes, eightGiBBytes))

		firstSample, err := sampler.Read(context.Background())
		Expect(err).ToNot(HaveOccurred())
		Expect(firstSample.Source).To(Equal(memoryhealth.SourceHostFallbackCgroupUnreadable))

		maps.Copy(files, cgroupV1Files(bytesText(oneGiBBytes), bytesText(halfGiBBytes)))

		secondSample, err := sampler.Read(context.Background())
		Expect(err).ToNot(HaveOccurred())
		Expect(secondSample.Source).To(Equal(memoryhealth.SourceCgroupLimit))
		Expect(secondSample.TotalBytes).To(Equal(oneGiBBytes))
	})

	It("reports the missing v2 file when neither cgroup version is readable", func() {
		sample, err := readSample(map[string]string{}, hostMemoryOf(threeGiBBytes, eightGiBBytes))

		Expect(err).ToNot(HaveOccurred())

		var cgroupErr *memoryhealth.CgroupReadError
		Expect(errors.As(sample.CgroupReadErr, &cgroupErr)).To(BeTrue())
		Expect(cgroupErr.File).To(Equal("memory.max"))
		Expect(cgroupErr.Outcome).To(Equal(memoryhealth.ReadMissing))
	})

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
