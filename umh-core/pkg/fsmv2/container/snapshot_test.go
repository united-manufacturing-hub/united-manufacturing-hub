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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/container"
)

var _ = Describe("ContainerObservedState", func() {
	Describe("GetTimestamp", func() {
		It("should return the CollectedAt timestamp", func() {
			expectedTime := time.Date(2025, 10, 18, 12, 0, 0, 0, time.UTC)
			observed := &container.ContainerObservedState{
				CollectedAt: expectedTime, ObservedThresholds: standardThresholds(),
			}
			Expect(observed.GetTimestamp()).To(Equal(expectedTime))
		})

		It("should return zero time when not set", func() {
			observed := &container.ContainerObservedState{ObservedThresholds: standardThresholds()}
			Expect(observed.GetTimestamp()).To(Equal(time.Time{}))
		})
	})

	Describe("GetObservedDesiredState", func() {
		It("should return an empty ContainerDesiredState", func() {
			observed := &container.ContainerObservedState{ObservedThresholds: standardThresholds()}
			desiredState := observed.GetObservedDesiredState()
			Expect(desiredState).NotTo(BeNil())

			containerDesiredState, ok := desiredState.(*container.ContainerDesiredState)
			Expect(ok).To(BeTrue())
			Expect(containerDesiredState.ShutdownRequested()).To(BeFalse())
		})

		It("should return thresholds from observed state", func() {
			observed := &container.ContainerObservedState{
				ObservedThresholds: container.HealthThresholds{
					CPUHighPercent:        70.0,
					CPUMediumPercent:      60.0,
					MemoryHighPercent:     80.0,
					MemoryMediumPercent:   70.0,
					DiskHighPercent:       85.0,
					DiskMediumPercent:     75.0,
					CPUThrottleRatioLimit: 0.05,
				},
			}

			desired := observed.GetObservedDesiredState().(*container.ContainerDesiredState)
			thresholds := desired.HealthThresholds()

			Expect(thresholds.CPUHighPercent).To(Equal(70.0))
			Expect(thresholds.CPUMediumPercent).To(Equal(60.0))
			Expect(thresholds.MemoryHighPercent).To(Equal(80.0))
			Expect(thresholds.MemoryMediumPercent).To(Equal(70.0))
			Expect(thresholds.DiskHighPercent).To(Equal(85.0))
			Expect(thresholds.DiskMediumPercent).To(Equal(75.0))
			Expect(thresholds.CPUThrottleRatioLimit).To(Equal(0.05))
		})

		It("should return empty thresholds when not set", func() {
			observed := &container.ContainerObservedState{}

			desired := observed.GetObservedDesiredState().(*container.ContainerDesiredState)
			thresholds := desired.HealthThresholds()

			Expect(thresholds.CPUHighPercent).To(Equal(0.0))
		})
	})
})

var _ = Describe("HealthThresholds", func() {
	Context("DesiredState HealthThresholds getter", func() {
		It("should return configured thresholds", func() {
			desired := &container.ContainerDesiredState{}

			thresholds := desired.HealthThresholds()
			Expect(thresholds).To(Equal(container.HealthThresholds{}))
		})
	})
})
