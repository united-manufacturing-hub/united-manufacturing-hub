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
