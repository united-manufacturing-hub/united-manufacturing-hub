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

func readCgroupMemory(files map[string]string) (memoryhealth.CgroupMemory, error) {
	return memoryhealth.ReadCgroupMemory(context.Background(), fixtureFilesystem(files), fixtureCgroupBase)
}

var _ = Describe("ReadCgroupMemory", func() {
	DescribeTable("reads the limit and the current usage",
		func(memoryMax, memoryCurrent string, expected memoryhealth.CgroupMemory) {
			memory, err := readCgroupMemory(cgroupFiles(memoryMax, memoryCurrent))

			Expect(err).ToNot(HaveOccurred())
			Expect(memory).To(Equal(expected))
		},
		Entry("a numeric limit", bytesText(eightGiBBytes), bytesText(twoGiBBytes),
			memoryhealth.CgroupMemory{LimitBytes: eightGiBBytes, CurrentBytes: twoGiBBytes}),
		Entry("values without a trailing newline", "4294967296", "1073741824",
			memoryhealth.CgroupMemory{LimitBytes: fourGiBBytes, CurrentBytes: oneGiBBytes}),
		Entry("max as the limit", "max\n", bytesText(twoGiBBytes),
			memoryhealth.CgroupMemory{CurrentBytes: twoGiBBytes, Unlimited: true}),
	)

	DescribeTable("rejects a value that does not parse",
		func(memoryMax, memoryCurrent string) {
			_, err := readCgroupMemory(cgroupFiles(memoryMax, memoryCurrent))

			Expect(err).To(HaveOccurred())
		},
		Entry("an empty memory.max", "", bytesText(twoGiBBytes)),
		Entry("a non-numeric memory.max", "notanumber\n", bytesText(twoGiBBytes)),
		Entry("a negative memory.max", "-1\n", bytesText(twoGiBBytes)),
		Entry("an empty memory.current", "max\n", ""),
		Entry("a non-numeric memory.current", "max\n", "abc\n"),
		Entry("a negative memory.current", "max\n", "-100\n"),
	)

	DescribeTable("fails when a file is missing",
		func(files map[string]string) {
			_, err := readCgroupMemory(files)

			Expect(err).To(MatchError(fs.ErrNotExist))
		},
		Entry("memory.max", map[string]string{fixtureCgroupBase + "/memory.current": bytesText(twoGiBBytes)}),
		Entry("memory.current", map[string]string{fixtureCgroupBase + "/memory.max": "max\n"}),
	)
})
