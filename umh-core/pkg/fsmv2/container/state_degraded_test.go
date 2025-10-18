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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/container"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

var _ = Describe("DegradedState", func() {
	var (
		snapshot fsmv2.Snapshot
		desired  *container.ContainerDesiredState
		observed *container.ContainerObservedState
	)

	BeforeEach(func() {
		desired = &container.ContainerDesiredState{
			MonitoringEnabled:    true,
			CollectionIntervalMs: 5000,
		}
		observed = &container.ContainerObservedState{
			OverallHealth: models.Degraded,
			CPUHealth:     models.Degraded,
			MemoryHealth:  models.Active,
			DiskHealth:    models.Active,
			CollectedAt:   time.Now(),
		}
	})

	Describe("Next", func() {
		Context("when shutdown is requested", func() {
			var state *container.DegradedState

			BeforeEach(func() {
				state = &container.DegradedState{}
				desired.SetShutdownRequested(true)
				snapshot = fsmv2.Snapshot{
					Desired:  desired,
					Observed: observed,
				}
			})

			It("should transition to StoppingState", func() {
				nextState, _, _ := state.Next(snapshot)
				Expect(nextState).To(BeAssignableToTypeOf(&container.StoppingState{}))
			})

			It("should not signal anything", func() {
				_, signal, _ := state.Next(snapshot)
				Expect(signal).To(Equal(fsmv2.SignalNone))
			})

			It("should not return an action", func() {
				_, _, action := state.Next(snapshot)
				Expect(action).To(BeNil())
			})
		})

		Context("when metrics recovered", func() {
			var state *container.DegradedState

			BeforeEach(func() {
				state = &container.DegradedState{}
				desired.SetShutdownRequested(false)
				observed.CollectedAt = time.Now()
				observed.OverallHealth = models.Active
				observed.CPUHealth = models.Active
				observed.MemoryHealth = models.Active
				observed.DiskHealth = models.Active
				snapshot = fsmv2.Snapshot{
					Desired:  desired,
					Observed: observed,
				}
			})

			It("should transition to ActiveState", func() {
				nextState, _, _ := state.Next(snapshot)
				Expect(nextState).To(BeAssignableToTypeOf(&container.ActiveState{}))
			})

			It("should not signal anything", func() {
				_, signal, _ := state.Next(snapshot)
				Expect(signal).To(Equal(fsmv2.SignalNone))
			})

			It("should not return an action", func() {
				_, _, action := state.Next(snapshot)
				Expect(action).To(BeNil())
			})
		})

		Context("when metrics still unhealthy", func() {
			var state *container.DegradedState

			BeforeEach(func() {
				state = &container.DegradedState{}
				desired.SetShutdownRequested(false)
				observed.CollectedAt = time.Now()
				observed.OverallHealth = models.Degraded
				observed.CPUHealth = models.Degraded
				snapshot = fsmv2.Snapshot{
					Desired:  desired,
					Observed: observed,
				}
			})

			It("should stay in DegradedState", func() {
				nextState, _, _ := state.Next(snapshot)
				Expect(nextState).To(BeAssignableToTypeOf(&container.DegradedState{}))
			})

			It("should not signal anything", func() {
				_, signal, _ := state.Next(snapshot)
				Expect(signal).To(Equal(fsmv2.SignalNone))
			})

			It("should not return an action", func() {
				_, _, action := state.Next(snapshot)
				Expect(action).To(BeNil())
			})
		})
	})

	Describe("String", func() {
		It("should return state name", func() {
			state := &container.DegradedState{}
			Expect(state.String()).To(Equal("Degraded"))
		})
	})

	Describe("Reason", func() {
		Context("when reason is not set", func() {
			It("should return generic reason", func() {
				state := &container.DegradedState{}
				Expect(state.Reason()).To(Equal("Monitoring degraded"))
			})
		})
	})
})
