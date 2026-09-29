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

const (
	eightGiB = int64(8589934592)
	twoGiB   = int64(2147483648)
)

var _ = Describe("ReadCgroupMemory", func() {
	It("returns the limit and the current usage", func() {
		filesystem := fixtureFilesystem(cgroupFiles("8589934592\n", "2147483648\n"))

		memory, err := ReadCgroupMemory(context.Background(), filesystem, fixtureCgroupBase)

		Expect(err).ToNot(HaveOccurred())
		Expect(memory).To(Equal(CgroupMemory{LimitBytes: eightGiB, CurrentBytes: twoGiB}))
	})

	It("reports an unlimited cgroup", func() {
		filesystem := fixtureFilesystem(cgroupFiles("max\n", "2147483648\n"))

		memory, err := ReadCgroupMemory(context.Background(), filesystem, fixtureCgroupBase)

		Expect(err).ToNot(HaveOccurred())
		Expect(memory).To(Equal(CgroupMemory{CurrentBytes: twoGiB, Unlimited: true}))
	})

	It("fails when memory.max is missing", func() {
		filesystem := fixtureFilesystem(map[string]string{fixtureCgroupBase + "/memory.current": "2147483648\n"})

		_, err := ReadCgroupMemory(context.Background(), filesystem, fixtureCgroupBase)

		Expect(err).To(MatchError(fs.ErrNotExist))
	})

	It("fails when memory.current is missing", func() {
		filesystem := fixtureFilesystem(map[string]string{fixtureCgroupBase + "/memory.max": "max\n"})

		_, err := ReadCgroupMemory(context.Background(), filesystem, fixtureCgroupBase)

		Expect(err).To(MatchError(fs.ErrNotExist))
	})

	It("fails when memory.current does not parse", func() {
		filesystem := fixtureFilesystem(cgroupFiles("max\n", "abc\n"))

		_, err := ReadCgroupMemory(context.Background(), filesystem, fixtureCgroupBase)

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
		Entry("a numeric limit", "8589934592\n", eightGiB, false),
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
		Entry("a numeric value", "2147483648\n", twoGiB),
		Entry("a value without trailing newline", "1073741824", int64(1073741824)),
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
