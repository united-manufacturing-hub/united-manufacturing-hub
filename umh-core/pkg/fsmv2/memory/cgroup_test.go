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
)

var _ = Describe("ReadCgroupMemory", func() {
	It("returns the limit and the current usage", func() {
		fileSystem := fixtureFilesystem(cgroupFiles(bytesText(eightGiBBytes), bytesText(twoGiBBytes)))

		memory, err := ReadCgroupMemory(context.Background(), fileSystem, fixtureCgroupBase)

		Expect(err).ToNot(HaveOccurred())
		Expect(memory).To(Equal(CgroupMemory{LimitBytes: eightGiBBytes, CurrentBytes: twoGiBBytes}))
	})

	It("reports an unlimited cgroup", func() {
		fileSystem := fixtureFilesystem(cgroupFiles("max\n", bytesText(twoGiBBytes)))

		memory, err := ReadCgroupMemory(context.Background(), fileSystem, fixtureCgroupBase)

		Expect(err).ToNot(HaveOccurred())
		Expect(memory).To(Equal(CgroupMemory{CurrentBytes: twoGiBBytes, Unlimited: true}))
	})

	It("fails when memory.max is missing", func() {
		fileSystem := fixtureFilesystem(map[string]string{fixtureCgroupBase + "/memory.current": bytesText(twoGiBBytes)})

		_, err := ReadCgroupMemory(context.Background(), fileSystem, fixtureCgroupBase)

		Expect(err).To(MatchError(fs.ErrNotExist))
	})

	It("fails when memory.current is missing", func() {
		fileSystem := fixtureFilesystem(map[string]string{fixtureCgroupBase + "/memory.max": "max\n"})

		_, err := ReadCgroupMemory(context.Background(), fileSystem, fixtureCgroupBase)

		Expect(err).To(MatchError(fs.ErrNotExist))
	})

	It("fails when memory.current does not parse", func() {
		fileSystem := fixtureFilesystem(cgroupFiles("max\n", "abc\n"))

		_, err := ReadCgroupMemory(context.Background(), fileSystem, fixtureCgroupBase)

		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("parseMemoryMax", func() {
	DescribeTable("accepted values",
		func(data string, expectedLimit int64, expectedUnlimited bool) {
			limit, unlimited, err := parseMemoryMax([]byte(data))

			Expect(err).ToNot(HaveOccurred())
			Expect(limit).To(Equal(expectedLimit))
			Expect(unlimited).To(Equal(expectedUnlimited))
		},
		Entry("a numeric limit", bytesText(eightGiBBytes), eightGiBBytes, false),
		Entry("a limit without trailing newline", "4294967296", int64(4294967296), false),
		Entry("max", "max\n", int64(0), true),
	)

	DescribeTable("rejected values",
		func(data string) {
			_, _, err := parseMemoryMax([]byte(data))

			Expect(err).To(HaveOccurred())
		},
		Entry("empty", ""),
		Entry("non-numeric", "notanumber\n"),
		Entry("negative", "-1\n"),
	)
})

var _ = Describe("parseMemoryCurrent", func() {
	DescribeTable("accepted values",
		func(data string, expected int64) {
			current, err := parseMemoryCurrent([]byte(data))

			Expect(err).ToNot(HaveOccurred())
			Expect(current).To(Equal(expected))
		},
		Entry("a numeric value", bytesText(twoGiBBytes), twoGiBBytes),
		Entry("a value without trailing newline", "1073741824", oneGiBBytes),
	)

	DescribeTable("rejected values",
		func(data string) {
			_, err := parseMemoryCurrent([]byte(data))

			Expect(err).To(HaveOccurred())
		},
		Entry("empty", ""),
		Entry("non-numeric", "abc\n"),
		Entry("negative", "-100\n"),
	)
})
