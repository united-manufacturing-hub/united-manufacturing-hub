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
})
