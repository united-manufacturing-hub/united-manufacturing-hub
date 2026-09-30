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
	"io/fs"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

func pollWithoutHost(fileSystem filesystem.Service) (MemoryStatus, simple.Health) {
	status, err := Poll(context.Background(), newTestDeps(fileSystem, unreadableHostMemory), MemoryConfig{})
	Expect(err).ToNot(HaveOccurred())

	return status, healthFromStatus(MemoryConfig{}, status)
}

var _ = Describe("the memory worker's poll", func() {
	It("reports a cgroup with a limit", func() {
		status, health := pollWithoutHost(fixtureFilesystem(cgroupFiles(bytesText(oneGiBBytes), bytesText(halfGiBBytes))))

		Expect(status.Measured).To(BeTrue())
		Expect(status.Source).To(Equal(SourceCgroupLimit))
		Expect(status.UsedBytes).To(Equal(halfGiBBytes))
		Expect(status.TotalBytes).To(Equal(oneGiBBytes))
		Expect(status.UsedPercent).To(BeNumerically("~", 50.0, 0.001))
		Expect(health.Degraded).To(BeFalse())
	})

	DescribeTable("judges usage against the thresholds",
		func(usedPercent int64, expectedMessage string, expectedDegraded bool) {
			fileSystem := fixtureFilesystem(cgroupFiles(bytesText(decimalLimitBytes), percentOfDecimalLimitText(usedPercent)))

			status, health := pollWithoutHost(fileSystem)

			Expect(status.Message).To(Equal(expectedMessage))
			Expect(health.Reason).To(Equal(expectedMessage))
			Expect(health.Degraded).To(Equal(expectedDegraded))
		},
		Entry("below the warning level", int64(50), messageNormal, false),
		Entry("at exactly the warning level", int64(70), messageWarning, false),
		Entry("at exactly the degraded level", int64(80), messageCritical, true),
		Entry("above the degraded level", int64(90), messageCritical, true),
	)

	It("reads through the filesystem published under FilesystemDepsKey", func() {
		register.SetDeps[filesystem.Service](FilesystemDepsKey, fixtureFilesystem(cgroupFiles(bytesText(oneGiBBytes), bytesText(halfGiBBytes))))
		DeferCleanup(func() { register.ClearDeps(FilesystemDepsKey) })

		identity := deps.Identity{ID: "memory-injection", WorkerType: WorkerType}
		memoryDeps := NewDeps(identity, deps.NewBaseDependencies(deps.NewNopFSMLogger(), nil, identity))

		status, err := Poll(context.Background(), memoryDeps, MemoryConfig{})

		Expect(err).ToNot(HaveOccurred())
		Expect(status.TotalBytes).To(Equal(oneGiBBytes))
	})
})

var _ = Describe("the memory worker's fallbacks", func() {
	DescribeTable("measures a cgroup without a limit against the host total",
		func(memoryMax string) {
			memoryDeps := newTestDeps(fixtureFilesystem(cgroupFiles(memoryMax, bytesText(halfGiBBytes))), hostMemoryOf(threeGiBBytes, eightGiBBytes))

			status, err := Poll(context.Background(), memoryDeps, MemoryConfig{})

			Expect(err).ToNot(HaveOccurred())
			Expect(status.Source).To(Equal(SourceHostTotalNoCgroupLimit))
			Expect(status.UsedBytes).To(Equal(halfGiBBytes))
			Expect(status.TotalBytes).To(Equal(eightGiBBytes))
		},
		Entry("memory.max is max", "max\n"),
		Entry("memory.max is 0", "0\n"),
	)

	It("fails an unlimited cgroup when the host total is unreadable", func() {
		memoryDeps := newTestDeps(fixtureFilesystem(cgroupFiles("max\n", bytesText(halfGiBBytes))), unreadableHostMemory)

		_, err := Poll(context.Background(), memoryDeps, MemoryConfig{})

		Expect(err).To(MatchError(errHostUnreadable))
	})

	DescribeTable("uses host values when the cgroup is unreadable",
		func(files map[string]string) {
			memoryDeps := newTestDeps(fixtureFilesystem(files), hostMemoryOf(threeGiBBytes, eightGiBBytes))

			status, err := Poll(context.Background(), memoryDeps, MemoryConfig{})

			Expect(err).ToNot(HaveOccurred())
			Expect(status.Source).To(Equal(SourceHostFallbackCgroupUnreadable))
			Expect(status.UsedBytes).To(Equal(threeGiBBytes))
			Expect(status.TotalBytes).To(Equal(eightGiBBytes))
			Expect(status.UsedPercent).To(BeNumerically("~", 37.5, 0.001))
		},
		Entry("no cgroup files", map[string]string{}),
		Entry("an unparsable memory.current", cgroupFiles(bytesText(oneGiBBytes), "abc\n")),
	)

	It("fails when both the cgroup and the host are unreadable", func() {
		memoryDeps := newTestDeps(fixtureFilesystem(map[string]string{}), unreadableHostMemory)

		_, err := Poll(context.Background(), memoryDeps, MemoryConfig{})

		Expect(err).To(MatchError(errHostUnreadable))
		Expect(err).To(MatchError(fs.ErrNotExist))
	})

	It("fails when the host reports a zero total", func() {
		memoryDeps := newTestDeps(fixtureFilesystem(map[string]string{}), hostMemoryOf(0, 0))

		_, err := Poll(context.Background(), memoryDeps, MemoryConfig{})

		Expect(err).To(MatchError(errZeroTotal))
	})

	It("does not need the host for a cgroup with a limit", func() {
		memoryDeps := newTestDeps(fixtureFilesystem(cgroupFiles(bytesText(oneGiBBytes), bytesText(halfGiBBytes))), unreadableHostMemory)

		status, err := Poll(context.Background(), memoryDeps, MemoryConfig{})

		Expect(err).ToNot(HaveOccurred())
		Expect(status.TotalBytes).To(Equal(oneGiBBytes))
	})

	It("returns the context error on a cancelled tick", func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		memoryDeps := newTestDeps(fixtureFilesystem(cgroupFiles(bytesText(oneGiBBytes), bytesText(halfGiBBytes))), hostMemoryOf(threeGiBBytes, eightGiBBytes))

		_, err := Poll(ctx, memoryDeps, MemoryConfig{})

		Expect(err).To(MatchError(context.Canceled))
	})
})
