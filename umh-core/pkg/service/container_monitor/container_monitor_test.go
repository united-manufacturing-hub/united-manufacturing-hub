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

package container_monitor_test

import (
	"context"
	"errors"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/container_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

var _ = Describe("Container Monitor Service", func() {
	var (
		mockFS      *filesystem.MockService
		service     container_monitor.Service
		ctx         context.Context
		defaultHWID = "hwid-12345"
	)

	BeforeEach(func() {
		mockFS = filesystem.NewMockService()
		service = container_monitor.NewContainerMonitorService(mockFS)
		ctx = context.Background()
	})

	Describe("GetStatus", func() {
		Context("when hardware ID file exists", func() {
			BeforeEach(func() {
				mockFS.On("FileExists", ctx, constants.HWIDFilePath).Return(true, nil)
				mockFS.On("ReadFile", ctx, constants.HWIDFilePath).Return([]byte(defaultHWID), nil)
			})

			It("should return metrics with the hardware ID from file", func() {
				metrics, err := service.GetStatus(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(metrics).ToNot(BeNil())
				Expect(metrics.HWID).To(Equal(defaultHWID))
			})

			It("should contain CPU metrics", func() {
				metrics, err := service.GetStatus(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(metrics.CPU).ToNot(BeNil())
				Expect(metrics.CPU.CoreCount).To(BeNumerically(">", 0))
			})

			It("should contain memory metrics", func() {
				metrics, err := service.GetStatus(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(metrics.Memory).ToNot(BeNil())
				Expect(metrics.Memory.CGroupTotalBytes).To(BeNumerically(">", 0))
			})

			It("should contain disk metrics", func() {
				metrics, err := service.GetStatus(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(metrics.Disk).ToNot(BeNil())
				Expect(metrics.Disk.DataPartitionTotalBytes).To(BeNumerically(">", 0))
			})

			It("should set architecture from runtime", func() {
				metrics, err := service.GetStatus(ctx)
				Expect(err).ToNot(HaveOccurred())
				expectedArch := models.ContainerArchitecture(os.Getenv("GOARCH"))
				if expectedArch == "" {
					expectedArch = models.ArchitectureAmd64 // Default in most test environments
				}
				Expect(metrics.Architecture).To(Equal(expectedArch))
			})
		})

		Context("when hardware ID file does not exist", func() {
			BeforeEach(func() {
				mockFS.On("FileExists", ctx, constants.HWIDFilePath).Return(false, nil)
			})

			It("should return metrics with fallback hardware ID", func() {
				metrics, err := service.GetStatus(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(metrics).ToNot(BeNil())
				Expect(metrics.HWID).To(Equal("hwid-12345")) // Fallback value
			})
		})

		Context("when hardware ID file read fails", func() {
			BeforeEach(func() {
				mockFS.On("FileExists", ctx, constants.HWIDFilePath).Return(true, nil)
				mockFS.On("ReadFile", ctx, constants.HWIDFilePath).Return(nil, errors.New("read error"))
			})

			It("should return metrics with empty hardware ID", func() {
				metrics, err := service.GetStatus(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(metrics).ToNot(BeNil())
				Expect(metrics.HWID).To(Equal(""))
			})
		})
	})

	Describe("GetHealth", func() {
		Context("when all metrics are normal", func() {
			BeforeEach(func() {
				mockFS.On("FileExists", ctx, constants.HWIDFilePath).Return(true, nil)
				mockFS.On("ReadFile", ctx, constants.HWIDFilePath).Return([]byte(defaultHWID), nil)
			})

			It("should return healthy status", func() {
				health, cpu, disk, memory, err := service.GetHealth(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(health).ToNot(BeNil())
				Expect(health.Category).To(Equal(models.Active))
				Expect(health.ObservedState).To(Equal(constants.ContainerStateRunning))

				Expect(cpu).ToNot(BeNil())
				Expect(cpu.Health.Category).To(Equal(models.Active))

				Expect(disk).ToNot(BeNil())
				Expect(disk.Health.Category).To(Equal(models.Active))

				Expect(memory).ToNot(BeNil())
				Expect(memory.Health.Category).To(Equal(models.Active))
			})
		})

		// Additional tests for critical conditions would be added here
		// using a real implementation that can simulate high load
	})
})
