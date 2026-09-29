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
	"errors"
	"io/fs"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

const (
	oneGiB      = int64(1073741824)
	oneGiBText  = "1073741824\n"
	halfGiBText = "536870912\n"

	decimalLimitText = "1000000000\n"
	fiftyPercent     = "500000000\n"
	seventyPercent   = "700000000\n"
	eightyPercent    = "800000000\n"
	ninetyPercent    = "900000000\n"
)

var errHostUnreadable = errors.New("host memory unreadable")

func hostMemoryOf(usedBytes, totalBytes uint64) HostMemoryReader {
	return func(context.Context) (uint64, uint64, error) {
		return usedBytes, totalBytes, nil
	}
}

func unreadableHostMemory(context.Context) (uint64, uint64, error) {
	return 0, 0, errHostUnreadable
}

func newTestDeps(fixture filesystem.Service, hostMemory HostMemoryReader) *MemoryDeps {
	identity := deps.Identity{ID: "memory-test", WorkerType: WorkerType}

	return &MemoryDeps{
		BaseDependencies: deps.NewBaseDependencies(deps.NewNopFSMLogger(), nil, identity),
		filesystem:       fixture,
		hostMemory:       hostMemory,
	}
}

func pollAndJudge(fixture filesystem.Service) (MemoryStatus, simple.Health) {
	status, err := Poll(context.Background(), newTestDeps(fixture, unreadableHostMemory), MemoryConfig{})
	Expect(err).ToNot(HaveOccurred())

	return status, healthFromStatus(MemoryConfig{}, status)
}

var _ = Describe("the memory worker's poll", func() {
	It("reports a cgroup with a limit", func() {
		status, health := pollAndJudge(fixtureFilesystem(cgroupFiles(oneGiBText, halfGiBText)))

		Expect(status.Source).To(Equal(SourceCgroup))
		Expect(status.UsedBytes).To(Equal(oneGiB / 2))
		Expect(status.TotalBytes).To(Equal(oneGiB))
		Expect(status.Unlimited).To(BeFalse())
		Expect(status.UsedPercent).To(BeNumerically("~", 50.0, 0.001))
		Expect(health.Degraded).To(BeFalse())
	})

	DescribeTable("judges usage against the thresholds",
		func(memoryCurrent string, expectedMessage string, expectedDegraded bool) {
			status, health := pollAndJudge(fixtureFilesystem(cgroupFiles(decimalLimitText, memoryCurrent)))

			Expect(status.Message).To(Equal(expectedMessage))
			Expect(health.Reason).To(Equal(expectedMessage))
			Expect(health.Degraded).To(Equal(expectedDegraded))
		},
		Entry("below the warning level", fiftyPercent, messageNormal, false),
		Entry("at exactly the warning level", seventyPercent, messageWarning, false),
		Entry("at exactly the degraded level", eightyPercent, messageCritical, true),
		Entry("above the degraded level", ninetyPercent, messageCritical, true),
	)

	It("reads through the filesystem published under FilesystemDepsKey", func() {
		register.SetDeps[filesystem.Service](FilesystemDepsKey, fixtureFilesystem(cgroupFiles(oneGiBText, halfGiBText)))
		DeferCleanup(func() { register.ClearDeps(FilesystemDepsKey) })

		identity := deps.Identity{ID: "memory-injection", WorkerType: WorkerType}
		memoryDeps := NewDeps(identity, deps.NewBaseDependencies(deps.NewNopFSMLogger(), nil, identity))

		status, err := Poll(context.Background(), memoryDeps, MemoryConfig{})

		Expect(err).ToNot(HaveOccurred())
		Expect(status.TotalBytes).To(Equal(oneGiB))
	})
})

const (
	eightGiBHost = uint64(8589934592)
	threeGiBHost = uint64(3221225472)
	halfGiBBytes = int64(536870912)
)

var _ = Describe("the memory worker's fallbacks", func() {
	It("measures an unlimited cgroup against the host total", func() {
		memoryDeps := newTestDeps(fixtureFilesystem(cgroupFiles("max\n", halfGiBText)), hostMemoryOf(threeGiBHost, eightGiBHost))

		status, err := Poll(context.Background(), memoryDeps, MemoryConfig{})

		Expect(err).ToNot(HaveOccurred())
		Expect(status.Source).To(Equal(SourceCgroup))
		Expect(status.Unlimited).To(BeTrue())
		Expect(status.UsedBytes).To(Equal(halfGiBBytes))
		Expect(status.TotalBytes).To(Equal(int64(eightGiBHost)))
	})

	It("fails an unlimited cgroup when the host total is unreadable", func() {
		memoryDeps := newTestDeps(fixtureFilesystem(cgroupFiles("max\n", halfGiBText)), unreadableHostMemory)

		_, err := Poll(context.Background(), memoryDeps, MemoryConfig{})

		Expect(err).To(MatchError(errHostUnreadable))
	})

	DescribeTable("uses host values when the cgroup is unreadable",
		func(files map[string]string) {
			memoryDeps := newTestDeps(fixtureFilesystem(files), hostMemoryOf(threeGiBHost, eightGiBHost))

			status, err := Poll(context.Background(), memoryDeps, MemoryConfig{})

			Expect(err).ToNot(HaveOccurred())
			Expect(status.Source).To(Equal(SourceHost))
			Expect(status.UsedBytes).To(Equal(int64(threeGiBHost)))
			Expect(status.TotalBytes).To(Equal(int64(eightGiBHost)))
			Expect(status.UsedPercent).To(BeNumerically("~", 37.5, 0.001))
		},
		Entry("no cgroup files", map[string]string{}),
		Entry("an unparsable memory.current", cgroupFiles(oneGiBText, "abc\n")),
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
		memoryDeps := newTestDeps(fixtureFilesystem(cgroupFiles(oneGiBText, halfGiBText)), unreadableHostMemory)

		status, err := Poll(context.Background(), memoryDeps, MemoryConfig{})

		Expect(err).ToNot(HaveOccurred())
		Expect(status.TotalBytes).To(Equal(oneGiB))
	})

	It("returns the context error on a cancelled tick", func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		memoryDeps := newTestDeps(fixtureFilesystem(cgroupFiles(oneGiBText, halfGiBText)), hostMemoryOf(threeGiBHost, eightGiBHost))

		_, err := Poll(ctx, memoryDeps, MemoryConfig{})

		Expect(err).To(MatchError(context.Canceled))
	})
})
