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
				CollectedAt: expectedTime,
			}
			Expect(observed.GetTimestamp()).To(Equal(expectedTime))
		})

		It("should return zero time when not set", func() {
			observed := &container.ContainerObservedState{}
			Expect(observed.GetTimestamp()).To(Equal(time.Time{}))
		})
	})

	Describe("GetObservedDesiredState", func() {
		It("should return an empty ContainerDesiredState", func() {
			observed := &container.ContainerObservedState{}
			desiredState := observed.GetObservedDesiredState()
			Expect(desiredState).NotTo(BeNil())

			containerDesiredState, ok := desiredState.(*container.ContainerDesiredState)
			Expect(ok).To(BeTrue())
			Expect(containerDesiredState.ShutdownRequested()).To(BeFalse())
		})
	})
})
