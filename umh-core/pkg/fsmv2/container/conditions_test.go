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

package container_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/container"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

var _ = Describe("isFullyHealthy", func() {
	Context("when all health categories are Active", func() {
		It("should return true", func() {
			observed := &container.ContainerObservedState{
				OverallHealth: models.Active,
				CPUHealth:     models.Active,
				MemoryHealth:  models.Active,
				DiskHealth:    models.Active, ObservedThresholds: standardThresholds(),
			}
			Expect(container.IsFullyHealthy(observed)).To(BeTrue())
		})
	})

	Context("when OverallHealth is Degraded", func() {
		It("should return false", func() {
			observed := &container.ContainerObservedState{
				OverallHealth: models.Degraded,
				CPUHealth:     models.Active,
				MemoryHealth:  models.Active,
				DiskHealth:    models.Active, ObservedThresholds: standardThresholds(),
			}
			Expect(container.IsFullyHealthy(observed)).To(BeFalse())
		})
	})

	Context("when CPUHealth is Degraded", func() {
		It("should return false", func() {
			observed := &container.ContainerObservedState{
				OverallHealth: models.Active,
				CPUHealth:     models.Degraded,
				MemoryHealth:  models.Active,
				DiskHealth:    models.Active, ObservedThresholds: standardThresholds(),
			}
			Expect(container.IsFullyHealthy(observed)).To(BeFalse())
		})
	})

	Context("when MemoryHealth is Degraded", func() {
		It("should return false", func() {
			observed := &container.ContainerObservedState{
				OverallHealth: models.Active,
				CPUHealth:     models.Active,
				MemoryHealth:  models.Degraded,
				DiskHealth:    models.Active, ObservedThresholds: standardThresholds(),
			}
			Expect(container.IsFullyHealthy(observed)).To(BeFalse())
		})
	})

	Context("when DiskHealth is Degraded", func() {
		It("should return false", func() {
			observed := &container.ContainerObservedState{
				OverallHealth: models.Active,
				CPUHealth:     models.Active,
				MemoryHealth:  models.Active,
				DiskHealth:    models.Degraded, ObservedThresholds: standardThresholds(),
			}
			Expect(container.IsFullyHealthy(observed)).To(BeFalse())
		})
	})

	Context("when multiple health categories are Degraded", func() {
		It("should return false", func() {
			observed := &container.ContainerObservedState{
				OverallHealth: models.Degraded,
				CPUHealth:     models.Degraded,
				MemoryHealth:  models.Active,
				DiskHealth:    models.Active, ObservedThresholds: standardThresholds(),
			}
			Expect(container.IsFullyHealthy(observed)).To(BeFalse())
		})
	})
})

var _ = Describe("BuildDegradedReason", func() {
	Context("when CPU is degraded by usage", func() {
		It("should show CPU percentage with threshold", func() {
			observed := &container.ContainerObservedState{
				OverallHealth:  models.Degraded,
				CPUHealth:      models.Degraded,
				MemoryHealth:   models.Active,
				DiskHealth:     models.Active,
				CPUUsageMCores: 1800, // 90% of 2 cores
				CgroupCores:    2.0,
				IsThrottled:    false, ObservedThresholds: standardThresholds(),
			}
			reason := container.BuildDegradedReason(observed)
			Expect(reason).To(MatchRegexp(`CPU at \d+% \(threshold 70%\)`))
		})

		It("should not mention healthy metrics", func() {
			observed := &container.ContainerObservedState{
				OverallHealth:  models.Degraded,
				CPUHealth:      models.Degraded,
				MemoryHealth:   models.Active,
				DiskHealth:     models.Active,
				CPUUsageMCores: 1800,
				CgroupCores:    2.0,
				IsThrottled:    false, ObservedThresholds: standardThresholds(),
			}
			reason := container.BuildDegradedReason(observed)
			Expect(reason).NotTo(ContainSubstring("Memory"))
			Expect(reason).NotTo(ContainSubstring("Disk"))
		})
	})

	Context("when CPU is throttled", func() {
		It("should show throttle percentage instead of usage", func() {
			observed := &container.ContainerObservedState{
				OverallHealth: models.Degraded,
				CPUHealth:     models.Degraded,
				MemoryHealth:  models.Active,
				DiskHealth:    models.Active,
				IsThrottled:   true,
				ThrottleRatio: 0.125, ObservedThresholds: // 12.5%
				standardThresholds(),
			}
			reason := container.BuildDegradedReason(observed)
			Expect(reason).To(ContainSubstring("CPU throttled"))
			Expect(reason).To(MatchRegexp(`\d+\.\d+% periods`))
		})
	})

	Context("when Memory is degraded", func() {
		It("should show Memory percentage with threshold", func() {
			observed := &container.ContainerObservedState{
				OverallHealth:    models.Degraded,
				CPUHealth:        models.Active,
				MemoryHealth:     models.Degraded,
				DiskHealth:       models.Active,
				MemoryUsedBytes:  8_500_000_000,                      // 8.5GB
				MemoryTotalBytes: 10_000_000_000, ObservedThresholds: // 10GB = 85%
				standardThresholds(),
			}
			reason := container.BuildDegradedReason(observed)
			Expect(reason).To(MatchRegexp(`Memory at \d+% \(threshold 80%\)`))
		})

		It("should not mention healthy metrics", func() {
			observed := &container.ContainerObservedState{
				OverallHealth:    models.Degraded,
				CPUHealth:        models.Active,
				MemoryHealth:     models.Degraded,
				DiskHealth:       models.Active,
				MemoryUsedBytes:  8_500_000_000,
				MemoryTotalBytes: 10_000_000_000, ObservedThresholds: standardThresholds(),
			}
			reason := container.BuildDegradedReason(observed)
			Expect(reason).NotTo(ContainSubstring("CPU"))
			Expect(reason).NotTo(ContainSubstring("Disk"))
		})
	})

	Context("when Disk is degraded", func() {
		It("should show Disk percentage with threshold", func() {
			observed := &container.ContainerObservedState{
				OverallHealth:  models.Degraded,
				CPUHealth:      models.Active,
				MemoryHealth:   models.Active,
				DiskHealth:     models.Degraded,
				DiskUsedBytes:  90_000_000_000,                      // 90GB
				DiskTotalBytes: 100_000_000_000, ObservedThresholds: // 100GB = 90%
				standardThresholds(),
			}
			reason := container.BuildDegradedReason(observed)
			Expect(reason).To(MatchRegexp(`Disk at \d+% \(threshold 85%\)`))
		})

		It("should not mention healthy metrics", func() {
			observed := &container.ContainerObservedState{
				OverallHealth:  models.Degraded,
				CPUHealth:      models.Active,
				MemoryHealth:   models.Active,
				DiskHealth:     models.Degraded,
				DiskUsedBytes:  90_000_000_000,
				DiskTotalBytes: 100_000_000_000, ObservedThresholds: standardThresholds(),
			}
			reason := container.BuildDegradedReason(observed)
			Expect(reason).NotTo(ContainSubstring("CPU"))
			Expect(reason).NotTo(ContainSubstring("Memory"))
		})
	})

	Context("when multiple metrics are unhealthy", func() {
		It("should include all unhealthy metrics with percentages", func() {
			observed := &container.ContainerObservedState{
				OverallHealth:    models.Degraded,
				CPUHealth:        models.Degraded,
				MemoryHealth:     models.Degraded,
				DiskHealth:       models.Active,
				CPUUsageMCores:   1600,
				CgroupCores:      2.0,
				IsThrottled:      false,
				MemoryUsedBytes:  8_500_000_000,
				MemoryTotalBytes: 10_000_000_000, ObservedThresholds: standardThresholds(),
			}
			reason := container.BuildDegradedReason(observed)
			Expect(reason).To(MatchRegexp(`CPU at \d+%`))
			Expect(reason).To(MatchRegexp(`Memory at \d+%`))
		})

		It("should not mention healthy metrics", func() {
			observed := &container.ContainerObservedState{
				OverallHealth:    models.Degraded,
				CPUHealth:        models.Degraded,
				MemoryHealth:     models.Degraded,
				DiskHealth:       models.Active,
				CPUUsageMCores:   1600,
				CgroupCores:      2.0,
				IsThrottled:      false,
				MemoryUsedBytes:  8_500_000_000,
				MemoryTotalBytes: 10_000_000_000, ObservedThresholds: standardThresholds(),
			}
			reason := container.BuildDegradedReason(observed)
			Expect(reason).NotTo(ContainSubstring("Disk"))
		})
	})

	Context("when all metrics are unhealthy", func() {
		It("should include all metrics with percentages", func() {
			observed := &container.ContainerObservedState{
				OverallHealth:    models.Degraded,
				CPUHealth:        models.Degraded,
				MemoryHealth:     models.Degraded,
				DiskHealth:       models.Degraded,
				CPUUsageMCores:   1600,
				CgroupCores:      2.0,
				IsThrottled:      false,
				MemoryUsedBytes:  8_500_000_000,
				MemoryTotalBytes: 10_000_000_000,
				DiskUsedBytes:    90_000_000_000,
				DiskTotalBytes:   100_000_000_000, ObservedThresholds: standardThresholds(),
			}
			reason := container.BuildDegradedReason(observed)
			Expect(reason).To(MatchRegexp(`CPU at \d+%`))
			Expect(reason).To(MatchRegexp(`Memory at \d+%`))
			Expect(reason).To(MatchRegexp(`Disk at \d+%`))
		})
	})

	Context("when OverallHealth is degraded but individual metrics are healthy", func() {
		It("should handle edge case gracefully", func() {
			observed := &container.ContainerObservedState{
				OverallHealth: models.Degraded,
				CPUHealth:     models.Active,
				MemoryHealth:  models.Active,
				DiskHealth:    models.Active, ObservedThresholds: standardThresholds(),
			}
			reason := container.BuildDegradedReason(observed)
			Expect(reason).NotTo(BeEmpty())
		})

		It("should not show percentage patterns since no metrics degraded", func() {
			observed := &container.ContainerObservedState{
				OverallHealth: models.Degraded,
				CPUHealth:     models.Active,
				MemoryHealth:  models.Active,
				DiskHealth:    models.Active, ObservedThresholds: standardThresholds(),
			}
			reason := container.BuildDegradedReason(observed)
			Expect(reason).NotTo(MatchRegexp(`\d+%`))
			Expect(reason).NotTo(ContainSubstring("threshold"))
		})
	})

	Context("UX Standards compliance", func() {
		It("should show exact CPU percentage", func() {
			observed := &container.ContainerObservedState{
				CPUHealth:      models.Degraded,
				MemoryHealth:   models.Active,
				DiskHealth:     models.Active,
				CPUUsageMCores: 1800,
				CgroupCores:    2.0,
				IsThrottled:    false, ObservedThresholds: standardThresholds(),
			}
			reason := container.BuildDegradedReason(observed)
			Expect(reason).To(Equal("CPU at 90% (threshold 70%)"))
		})

		It("should show exact throttle percentage", func() {
			observed := &container.ContainerObservedState{
				CPUHealth:     models.Degraded,
				MemoryHealth:  models.Active,
				DiskHealth:    models.Active,
				IsThrottled:   true,
				ThrottleRatio: 0.125, ObservedThresholds: standardThresholds(),
			}
			reason := container.BuildDegradedReason(observed)
			Expect(reason).To(Equal("CPU throttled (12.5% periods)"))
		})

		It("should format multiple components with commas", func() {
			observed := &container.ContainerObservedState{
				CPUHealth:        models.Degraded,
				MemoryHealth:     models.Degraded,
				DiskHealth:       models.Active,
				CPUUsageMCores:   1600,
				CgroupCores:      2.0,
				IsThrottled:      false,
				MemoryUsedBytes:  8_500_000_000,
				MemoryTotalBytes: 10_000_000_000, ObservedThresholds: standardThresholds(),
			}
			reason := container.BuildDegradedReason(observed)
			Expect(reason).To(ContainSubstring("CPU at"))
			Expect(reason).To(ContainSubstring("Memory at"))
			Expect(reason).To(ContainSubstring(", "))
		})
	})
})
