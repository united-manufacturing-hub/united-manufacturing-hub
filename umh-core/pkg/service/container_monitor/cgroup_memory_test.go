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

package container_monitor

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

var _ = Describe("Cgroup Memory", func() {
	Describe("Fallback behavior", func() {
		It("should return valid memory metrics even when cgroup files are unavailable", func() {
			// On macOS and non-container Linux, /sys/fs/cgroup/memory.max does not exist.
			// getMemoryMetrics() should fall back to gopsutil host-level values.
			mockFS := filesystem.NewMockFileSystem()
			service := NewContainerMonitorServiceWithPath(mockFS, GinkgoT().TempDir())

			status, err := service.GetStatus(context.Background())
			Expect(err).ToNot(HaveOccurred())
			Expect(status.Memory).ToNot(BeNil())
			Expect(status.Memory.CGroupTotalBytes).To(BeNumerically(">", 0))
			Expect(status.Memory.CGroupUsedBytes).To(BeNumerically(">", 0))
		})
	})

	Describe("parseMemoryMax", func() {
		It("should parse a numeric memory limit", func() {
			data := []byte("8589934592\n") // 8 GiB
			limit, unlimited, err := parseMemoryMax(data)
			Expect(err).ToNot(HaveOccurred())
			Expect(unlimited).To(BeFalse())
			Expect(limit).To(Equal(int64(8589934592)))
		})

		It("should handle 'max' as unlimited", func() {
			data := []byte("max\n")
			_, unlimited, err := parseMemoryMax(data)
			Expect(err).ToNot(HaveOccurred())
			Expect(unlimited).To(BeTrue())
		})

		It("should handle value without trailing newline", func() {
			data := []byte("4294967296")
			limit, unlimited, err := parseMemoryMax(data)
			Expect(err).ToNot(HaveOccurred())
			Expect(unlimited).To(BeFalse())
			Expect(limit).To(Equal(int64(4294967296)))
		})

		It("should return error for empty data", func() {
			data := []byte("")
			_, _, err := parseMemoryMax(data)
			Expect(err).To(HaveOccurred())
		})

		It("should return error for non-numeric data", func() {
			data := []byte("notanumber\n")
			_, _, err := parseMemoryMax(data)
			Expect(err).To(HaveOccurred())
		})

		It("should return error for negative values", func() {
			data := []byte("-1\n")
			_, _, err := parseMemoryMax(data)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("parseMemoryCurrent", func() {
		It("should parse current memory usage", func() {
			data := []byte("2147483648\n") // 2 GiB
			current, err := parseMemoryCurrent(data)
			Expect(err).ToNot(HaveOccurred())
			Expect(current).To(Equal(int64(2147483648)))
		})

		It("should handle value without trailing newline", func() {
			data := []byte("1073741824")
			current, err := parseMemoryCurrent(data)
			Expect(err).ToNot(HaveOccurred())
			Expect(current).To(Equal(int64(1073741824)))
		})

		It("should return error for empty data", func() {
			data := []byte("")
			_, err := parseMemoryCurrent(data)
			Expect(err).To(HaveOccurred())
		})

		It("should return error for non-numeric data", func() {
			data := []byte("abc\n")
			_, err := parseMemoryCurrent(data)
			Expect(err).To(HaveOccurred())
		})

		It("should return error for negative values", func() {
			data := []byte("-100\n")
			_, err := parseMemoryCurrent(data)
			Expect(err).To(HaveOccurred())
		})
	})
})
