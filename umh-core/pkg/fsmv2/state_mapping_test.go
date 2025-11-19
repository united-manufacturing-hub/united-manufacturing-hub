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

package fsmv2_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
)

var _ = Describe("StateMappingRegistry", func() {
	var registry *fsmv2.StateMappingRegistry

	BeforeEach(func() {
		registry = fsmv2.NewStateMappingRegistry()
	})

	Describe("NewStateMappingRegistry", func() {
		It("should create a new empty registry", func() {
			Expect(registry).NotTo(BeNil())
		})
	})

	Describe("Register", func() {
		It("should register a single mapping", func() {
			mapping := fsmv2.StateMapping{
				ParentState: "StartingChildren",
				ChildDesired: map[string]string{
					"child-1": "running",
				},
			}
			registry.Register(mapping)

			states := registry.DeriveChildDesiredStates("StartingChildren", fsmv2.Snapshot{})
			Expect(states).To(HaveLen(1))
			Expect(states["child-1"]).To(Equal("running"))
		})

		It("should register multiple mappings for same parent state", func() {
			mapping1 := fsmv2.StateMapping{
				ParentState: "StartingChildren",
				ChildDesired: map[string]string{
					"child-1": "running",
				},
			}
			mapping2 := fsmv2.StateMapping{
				ParentState: "StartingChildren",
				ChildDesired: map[string]string{
					"child-2": "running",
				},
			}
			registry.Register(mapping1)
			registry.Register(mapping2)

			states := registry.DeriveChildDesiredStates("StartingChildren", fsmv2.Snapshot{})
			Expect(states).To(HaveLen(2))
			Expect(states["child-1"]).To(Equal("running"))
			Expect(states["child-2"]).To(Equal("running"))
		})

		It("should register mappings for different parent states", func() {
			mappingStart := fsmv2.StateMapping{
				ParentState: "StartingChildren",
				ChildDesired: map[string]string{
					"child-1": "running",
				},
			}
			mappingStop := fsmv2.StateMapping{
				ParentState: "StoppingChildren",
				ChildDesired: map[string]string{
					"child-1": "stopped",
				},
			}
			registry.Register(mappingStart)
			registry.Register(mappingStop)

			startStates := registry.DeriveChildDesiredStates("StartingChildren", fsmv2.Snapshot{})
			Expect(startStates["child-1"]).To(Equal("running"))

			stopStates := registry.DeriveChildDesiredStates("StoppingChildren", fsmv2.Snapshot{})
			Expect(stopStates["child-1"]).To(Equal("stopped"))
		})
	})

	Describe("DeriveChildDesiredStates", func() {
		It("should return correct states for matching parent state", func() {
			registry.Register(fsmv2.StateMapping{
				ParentState: "StartingChildren",
				ChildDesired: map[string]string{
					"s6-service-1": "running",
					"s6-service-2": "running",
				},
			})

			states := registry.DeriveChildDesiredStates("StartingChildren", fsmv2.Snapshot{})
			Expect(states).To(HaveLen(2))
			Expect(states["s6-service-1"]).To(Equal("running"))
			Expect(states["s6-service-2"]).To(Equal("running"))
		})

		It("should return empty map for unknown parent state", func() {
			registry.Register(fsmv2.StateMapping{
				ParentState: "StartingChildren",
				ChildDesired: map[string]string{
					"child-1": "running",
				},
			})

			states := registry.DeriveChildDesiredStates("UnknownState", fsmv2.Snapshot{})
			Expect(states).To(BeEmpty())
		})

		It("should return empty map for empty registry", func() {
			states := registry.DeriveChildDesiredStates("AnyState", fsmv2.Snapshot{})
			Expect(states).To(BeEmpty())
		})

		It("should merge child desired states from multiple mappings", func() {
			registry.Register(fsmv2.StateMapping{
				ParentState: "Running",
				ChildDesired: map[string]string{
					"child-1": "active",
				},
			})
			registry.Register(fsmv2.StateMapping{
				ParentState: "Running",
				ChildDesired: map[string]string{
					"child-2": "active",
					"child-3": "active",
				},
			})

			states := registry.DeriveChildDesiredStates("Running", fsmv2.Snapshot{})
			Expect(states).To(HaveLen(3))
			Expect(states["child-1"]).To(Equal("active"))
			Expect(states["child-2"]).To(Equal("active"))
			Expect(states["child-3"]).To(Equal("active"))
		})

		It("should allow later mappings to override earlier ones for same child", func() {
			registry.Register(fsmv2.StateMapping{
				ParentState: "Running",
				ChildDesired: map[string]string{
					"child-1": "initial",
				},
			})
			registry.Register(fsmv2.StateMapping{
				ParentState: "Running",
				ChildDesired: map[string]string{
					"child-1": "overridden",
				},
			})

			states := registry.DeriveChildDesiredStates("Running", fsmv2.Snapshot{})
			Expect(states["child-1"]).To(Equal("overridden"))
		})
	})

	Describe("Condition functions", func() {
		It("should apply mapping when condition returns true", func() {
			registry.Register(fsmv2.StateMapping{
				ParentState: "Running",
				ChildDesired: map[string]string{
					"child-1": "running",
				},
				Condition: func(parentSnapshot fsmv2.Snapshot) bool {
					return true
				},
			})

			states := registry.DeriveChildDesiredStates("Running", fsmv2.Snapshot{})
			Expect(states["child-1"]).To(Equal("running"))
		})

		It("should skip mapping when condition returns false", func() {
			registry.Register(fsmv2.StateMapping{
				ParentState: "Running",
				ChildDesired: map[string]string{
					"child-1": "running",
				},
				Condition: func(parentSnapshot fsmv2.Snapshot) bool {
					return false
				},
			})

			states := registry.DeriveChildDesiredStates("Running", fsmv2.Snapshot{})
			Expect(states).To(BeEmpty())
		})

		It("should apply mapping when condition is nil", func() {
			registry.Register(fsmv2.StateMapping{
				ParentState: "Running",
				ChildDesired: map[string]string{
					"child-1": "running",
				},
				Condition: nil,
			})

			states := registry.DeriveChildDesiredStates("Running", fsmv2.Snapshot{})
			Expect(states["child-1"]).To(Equal("running"))
		})

		It("should pass parent snapshot to condition function", func() {
			var capturedSnapshot fsmv2.Snapshot
			registry.Register(fsmv2.StateMapping{
				ParentState: "Running",
				ChildDesired: map[string]string{
					"child-1": "running",
				},
				Condition: func(parentSnapshot fsmv2.Snapshot) bool {
					capturedSnapshot = parentSnapshot
					return true
				},
			})

			expectedSnapshot := fsmv2.Snapshot{
				Identity: fsmv2.Identity{
					ID:   "parent-id",
					Name: "parent-name",
				},
			}
			registry.DeriveChildDesiredStates("Running", expectedSnapshot)
			Expect(capturedSnapshot.Identity.ID).To(Equal("parent-id"))
			Expect(capturedSnapshot.Identity.Name).To(Equal("parent-name"))
		})

		It("should evaluate conditions independently for each mapping", func() {
			callCount := 0
			registry.Register(fsmv2.StateMapping{
				ParentState: "Running",
				ChildDesired: map[string]string{
					"child-1": "running",
				},
				Condition: func(parentSnapshot fsmv2.Snapshot) bool {
					callCount++
					return callCount == 1 // true on first call
				},
			})
			registry.Register(fsmv2.StateMapping{
				ParentState: "Running",
				ChildDesired: map[string]string{
					"child-2": "running",
				},
				Condition: func(parentSnapshot fsmv2.Snapshot) bool {
					callCount++
					return callCount == 2 // true on second call
				},
			})

			states := registry.DeriveChildDesiredStates("Running", fsmv2.Snapshot{})
			Expect(states).To(HaveLen(2))
			Expect(states["child-1"]).To(Equal("running"))
			Expect(states["child-2"]).To(Equal("running"))
		})
	})

	Describe("GetChildDesiredState", func() {
		BeforeEach(func() {
			registry.Register(fsmv2.StateMapping{
				ParentState: "StartingChildren",
				ChildDesired: map[string]string{
					"s6-service-1": "running",
					"s6-service-2": "running",
				},
			})
		})

		It("should return desired state for existing child", func() {
			state, ok := registry.GetChildDesiredState("StartingChildren", "s6-service-1", fsmv2.Snapshot{})
			Expect(ok).To(BeTrue())
			Expect(state).To(Equal("running"))
		})

		It("should return false for non-existing child", func() {
			state, ok := registry.GetChildDesiredState("StartingChildren", "non-existent", fsmv2.Snapshot{})
			Expect(ok).To(BeFalse())
			Expect(state).To(BeEmpty())
		})

		It("should return false for non-existing parent state", func() {
			state, ok := registry.GetChildDesiredState("UnknownState", "s6-service-1", fsmv2.Snapshot{})
			Expect(ok).To(BeFalse())
			Expect(state).To(BeEmpty())
		})

		It("should respect conditions", func() {
			conditionRegistry := fsmv2.NewStateMappingRegistry()
			conditionRegistry.Register(fsmv2.StateMapping{
				ParentState: "Running",
				ChildDesired: map[string]string{
					"child-1": "running",
				},
				Condition: func(parentSnapshot fsmv2.Snapshot) bool {
					return false
				},
			})

			state, ok := conditionRegistry.GetChildDesiredState("Running", "child-1", fsmv2.Snapshot{})
			Expect(ok).To(BeFalse())
			Expect(state).To(BeEmpty())
		})
	})

	Describe("Example usage from plan", func() {
		It("should support the example from the plan document", func() {
			// Create registry
			registry := fsmv2.NewStateMappingRegistry()

			// When parent is "StartingChildren", children should be "running"
			registry.Register(fsmv2.StateMapping{
				ParentState: "StartingChildren",
				ChildDesired: map[string]string{
					"s6-service-1": "running",
					"s6-service-2": "running",
				},
			})

			// When parent is "StoppingChildren", children should be "stopped"
			registry.Register(fsmv2.StateMapping{
				ParentState: "StoppingChildren",
				ChildDesired: map[string]string{
					"s6-service-1": "stopped",
					"s6-service-2": "stopped",
				},
			})

			// Query desired states for StartingChildren
			states := registry.DeriveChildDesiredStates("StartingChildren", fsmv2.Snapshot{})
			Expect(states).To(Equal(map[string]string{
				"s6-service-1": "running",
				"s6-service-2": "running",
			}))

			// Query desired states for StoppingChildren
			states = registry.DeriveChildDesiredStates("StoppingChildren", fsmv2.Snapshot{})
			Expect(states).To(Equal(map[string]string{
				"s6-service-1": "stopped",
				"s6-service-2": "stopped",
			}))
		})
	})
})
