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
				DiskHealth:    models.Active,
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
				DiskHealth:    models.Active,
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
				DiskHealth:    models.Active,
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
				DiskHealth:    models.Active,
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
				DiskHealth:    models.Degraded,
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
				DiskHealth:    models.Active,
			}
			Expect(container.IsFullyHealthy(observed)).To(BeFalse())
		})
	})
})

var _ = Describe("BuildDegradedReason", func() {
	Context("when CPU is degraded", func() {
		It("should include CPU in reason", func() {
			observed := &container.ContainerObservedState{
				OverallHealth: models.Degraded,
				CPUHealth:     models.Degraded,
				MemoryHealth:  models.Active,
				DiskHealth:    models.Active,
			}
			reason := container.BuildDegradedReason(observed)
			Expect(reason).To(ContainSubstring("CPU"))
			Expect(reason).To(ContainSubstring("degraded"))
		})

		It("should not mention healthy metrics", func() {
			observed := &container.ContainerObservedState{
				OverallHealth: models.Degraded,
				CPUHealth:     models.Degraded,
				MemoryHealth:  models.Active,
				DiskHealth:    models.Active,
			}
			reason := container.BuildDegradedReason(observed)
			Expect(reason).NotTo(ContainSubstring("Memory"))
			Expect(reason).NotTo(ContainSubstring("Disk"))
		})
	})

	Context("when Memory is degraded", func() {
		It("should include Memory in reason", func() {
			observed := &container.ContainerObservedState{
				OverallHealth: models.Degraded,
				CPUHealth:     models.Active,
				MemoryHealth:  models.Degraded,
				DiskHealth:    models.Active,
			}
			reason := container.BuildDegradedReason(observed)
			Expect(reason).To(ContainSubstring("Memory"))
			Expect(reason).To(ContainSubstring("degraded"))
		})

		It("should not mention healthy metrics", func() {
			observed := &container.ContainerObservedState{
				OverallHealth: models.Degraded,
				CPUHealth:     models.Active,
				MemoryHealth:  models.Degraded,
				DiskHealth:    models.Active,
			}
			reason := container.BuildDegradedReason(observed)
			Expect(reason).NotTo(ContainSubstring("CPU"))
			Expect(reason).NotTo(ContainSubstring("Disk"))
		})
	})

	Context("when Disk is degraded", func() {
		It("should include Disk in reason", func() {
			observed := &container.ContainerObservedState{
				OverallHealth: models.Degraded,
				CPUHealth:     models.Active,
				MemoryHealth:  models.Active,
				DiskHealth:    models.Degraded,
			}
			reason := container.BuildDegradedReason(observed)
			Expect(reason).To(ContainSubstring("Disk"))
			Expect(reason).To(ContainSubstring("degraded"))
		})

		It("should not mention healthy metrics", func() {
			observed := &container.ContainerObservedState{
				OverallHealth: models.Degraded,
				CPUHealth:     models.Active,
				MemoryHealth:  models.Active,
				DiskHealth:    models.Degraded,
			}
			reason := container.BuildDegradedReason(observed)
			Expect(reason).NotTo(ContainSubstring("CPU"))
			Expect(reason).NotTo(ContainSubstring("Memory"))
		})
	})

	Context("when multiple metrics are unhealthy", func() {
		It("should include all unhealthy metrics", func() {
			observed := &container.ContainerObservedState{
				OverallHealth: models.Degraded,
				CPUHealth:     models.Degraded,
				MemoryHealth:  models.Degraded,
				DiskHealth:    models.Active,
			}
			reason := container.BuildDegradedReason(observed)
			Expect(reason).To(ContainSubstring("CPU"))
			Expect(reason).To(ContainSubstring("Memory"))
		})

		It("should not mention healthy metrics", func() {
			observed := &container.ContainerObservedState{
				OverallHealth: models.Degraded,
				CPUHealth:     models.Degraded,
				MemoryHealth:  models.Degraded,
				DiskHealth:    models.Active,
			}
			reason := container.BuildDegradedReason(observed)
			Expect(reason).NotTo(ContainSubstring("Disk"))
		})
	})

	Context("when all metrics are unhealthy", func() {
		It("should include all metrics in reason", func() {
			observed := &container.ContainerObservedState{
				OverallHealth: models.Degraded,
				CPUHealth:     models.Degraded,
				MemoryHealth:  models.Degraded,
				DiskHealth:    models.Degraded,
			}
			reason := container.BuildDegradedReason(observed)
			Expect(reason).To(ContainSubstring("CPU"))
			Expect(reason).To(ContainSubstring("Memory"))
			Expect(reason).To(ContainSubstring("Disk"))
		})
	})

	Context("when OverallHealth is degraded but individual metrics are healthy", func() {
		It("should handle edge case gracefully", func() {
			observed := &container.ContainerObservedState{
				OverallHealth: models.Degraded,
				CPUHealth:     models.Active,
				MemoryHealth:  models.Active,
				DiskHealth:    models.Active,
			}
			reason := container.BuildDegradedReason(observed)
			Expect(reason).NotTo(BeEmpty())
		})

		It("should not claim individual metrics are degraded", func() {
			observed := &container.ContainerObservedState{
				OverallHealth: models.Degraded,
				CPUHealth:     models.Active,
				MemoryHealth:  models.Active,
				DiskHealth:    models.Active,
			}
			reason := container.BuildDegradedReason(observed)
			Expect(reason).NotTo(MatchRegexp("CPU.*degraded"))
			Expect(reason).NotTo(MatchRegexp("Memory.*degraded"))
			Expect(reason).NotTo(MatchRegexp("Disk.*degraded"))
		})
	})
})
