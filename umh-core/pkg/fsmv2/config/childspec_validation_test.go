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

package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// MockRegistry implements WorkerTypeChecker for testing.
type MockRegistry struct {
	registeredTypes []string
}

func NewMockRegistry(types ...string) *MockRegistry {
	return &MockRegistry{
		registeredTypes: types,
	}
}

func (m *MockRegistry) ListRegisteredTypes() []string {
	return m.registeredTypes
}

var _ = Describe("ChildSpec Validation", func() {
	var registry config.WorkerTypeChecker

	BeforeEach(func() {
		registry = NewMockRegistry("communicator", "test-worker")
	})

	Describe("ValidateChildSpec", func() {
		It("should pass validation for a valid ChildSpec", func() {
			spec := config.ChildSpec{
				Name:       "valid-child",
				WorkerType: "communicator",
				UserSpec: config.UserSpec{
					Config: "host: localhost\nport: 8080",
				},
			}

			err := config.ValidateChildSpec(spec, registry)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should pass validation for a ChildSpec with ChildStartStates", func() {
			spec := config.ChildSpec{
				Name:       "valid-child-with-states",
				WorkerType: "test-worker",
				UserSpec: config.UserSpec{
					Config: "config-data",
				},
				ChildStartStates: []string{"Running", "TryingToStart"},
			}

			err := config.ValidateChildSpec(spec, registry)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should fail when Name is empty", func() {
			spec := config.ChildSpec{
				Name:       "",
				WorkerType: "communicator",
				UserSpec: config.UserSpec{
					Config: "config-data",
				},
			}

			err := config.ValidateChildSpec(spec, registry)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("name cannot be empty"))
		})

		It("should fail when WorkerType is empty", func() {
			spec := config.ChildSpec{
				Name:       "missing-type",
				WorkerType: "",
				UserSpec: config.UserSpec{
					Config: "config-data",
				},
			}

			err := config.ValidateChildSpec(spec, registry)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("worker type cannot be empty"))
			Expect(err.Error()).To(ContainSubstring("missing-type"))
		})

		It("should fail when WorkerType is unknown", func() {
			spec := config.ChildSpec{
				Name:       "unknown-type-child",
				WorkerType: "unknown-worker",
				UserSpec: config.UserSpec{
					Config: "config-data",
				},
			}

			err := config.ValidateChildSpec(spec, registry)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unknown worker type"))
			Expect(err.Error()).To(ContainSubstring("unknown-worker"))
			Expect(err.Error()).To(ContainSubstring("unknown-type-child"))
			Expect(err.Error()).To(ContainSubstring("available:"))
		})

		It("should list available types in error message", func() {
			spec := config.ChildSpec{
				Name:       "test-child",
				WorkerType: "invalid-type",
				UserSpec: config.UserSpec{
					Config: "config",
				},
			}

			err := config.ValidateChildSpec(spec, registry)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("communicator"))
			Expect(err.Error()).To(ContainSubstring("test-worker"))
		})

		It("should fail when UserSpec cannot be marshaled to JSON", func() {
			spec := config.ChildSpec{
				Name:       "invalid-spec",
				WorkerType: "communicator",
				UserSpec: config.UserSpec{
					Config:    "config-data",
					Variables: createUnmarshalableVariables(),
				},
			}

			err := config.ValidateChildSpec(spec, registry)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid user spec"))
			Expect(err.Error()).To(ContainSubstring("invalid-spec"))
		})

		It("should validate even with nil ChildStartStates", func() {
			spec := config.ChildSpec{
				Name:       "no-states",
				WorkerType: "test-worker",
				UserSpec: config.UserSpec{
					Config: "config",
				},
				ChildStartStates: nil,
			}

			err := config.ValidateChildSpec(spec, registry)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should validate with empty ChildStartStates", func() {
			spec := config.ChildSpec{
				Name:       "empty-states",
				WorkerType: "test-worker",
				UserSpec: config.UserSpec{
					Config: "config",
				},
				ChildStartStates: []string{},
			}

			err := config.ValidateChildSpec(spec, registry)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject ChildStartStates with empty state name", func() {
			spec := config.ChildSpec{
				Name:             "test-child",
				WorkerType:       "test-worker",
				ChildStartStates: []string{"Running", ""},
				UserSpec: config.UserSpec{
					Config: "config",
				},
			}
			err := config.ValidateChildSpec(spec, registry)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot be empty"))
		})

		It("should reject ChildStartStates with duplicate state names", func() {
			spec := config.ChildSpec{
				Name:             "test-child",
				WorkerType:       "test-worker",
				ChildStartStates: []string{"Running", "Running"},
				UserSpec: config.UserSpec{
					Config: "config",
				},
			}
			err := config.ValidateChildSpec(spec, registry)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("duplicate"))
		})

		It("should accept valid ChildStartStates with multiple unique states", func() {
			spec := config.ChildSpec{
				Name:             "test-child",
				WorkerType:       "test-worker",
				ChildStartStates: []string{"Running", "TryingToStart", "Degraded"},
				UserSpec: config.UserSpec{
					Config: "config",
				},
			}
			err := config.ValidateChildSpec(spec, registry)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should use correct name in all error messages", func() {
			spec := config.ChildSpec{
				Name:       "specific-child-name",
				WorkerType: "bad-type",
				UserSpec: config.UserSpec{
					Config: "config",
				},
			}

			err := config.ValidateChildSpec(spec, registry)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("specific-child-name"))
		})

		It("should validate successfully with JSON variables", func() {
			spec := config.ChildSpec{
				Name:       "valid-json-spec",
				WorkerType: "communicator",
				UserSpec: config.UserSpec{
					Config: `{"host": "localhost", "port": 8080}`,
					Variables: config.VariableBundle{
						User: map[string]any{
							"IP":   "192.168.1.100",
							"PORT": 502,
						},
					},
				},
			}

			err := config.ValidateChildSpec(spec, registry)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("ValidateChildSpecs", func() {
		It("should pass validation for a single valid ChildSpec in a slice", func() {
			specs := []config.ChildSpec{
				{
					Name:       "child-1",
					WorkerType: "communicator",
					UserSpec: config.UserSpec{
						Config: "config-1",
					},
				},
			}

			err := config.ValidateChildSpecs(specs, registry)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should pass validation for multiple valid ChildSpecs", func() {
			specs := []config.ChildSpec{
				{
					Name:       "child-1",
					WorkerType: "communicator",
					UserSpec: config.UserSpec{
						Config: "config-1",
					},
				},
				{
					Name:       "child-2",
					WorkerType: "test-worker",
					UserSpec: config.UserSpec{
						Config: "config-2",
					},
				},
				{
					Name:       "child-3",
					WorkerType: "communicator",
					UserSpec: config.UserSpec{
						Config: "config-3",
					},
				},
			}

			err := config.ValidateChildSpecs(specs, registry)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should pass validation for empty slice", func() {
			specs := []config.ChildSpec{}

			err := config.ValidateChildSpecs(specs, registry)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should fail when any individual spec is invalid", func() {
			specs := []config.ChildSpec{
				{
					Name:       "valid-child",
					WorkerType: "communicator",
					UserSpec: config.UserSpec{
						Config: "config",
					},
				},
				{
					Name:       "invalid-child",
					WorkerType: "",
					UserSpec: config.UserSpec{
						Config: "config",
					},
				},
			}

			err := config.ValidateChildSpecs(specs, registry)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("child spec [1]"))
			Expect(err.Error()).To(ContainSubstring("worker type cannot be empty"))
		})

		It("should report correct index when validation fails", func() {
			specs := []config.ChildSpec{
				{
					Name:       "valid-1",
					WorkerType: "communicator",
					UserSpec: config.UserSpec{
						Config: "config",
					},
				},
				{
					Name:       "valid-2",
					WorkerType: "test-worker",
					UserSpec: config.UserSpec{
						Config: "config",
					},
				},
				{
					Name:       "invalid-child",
					WorkerType: "unknown-type",
					UserSpec: config.UserSpec{
						Config: "config",
					},
				},
			}

			err := config.ValidateChildSpecs(specs, registry)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("child spec [2]"))
		})

		It("should fail on duplicate names", func() {
			specs := []config.ChildSpec{
				{
					Name:       "duplicate-name",
					WorkerType: "communicator",
					UserSpec: config.UserSpec{
						Config: "config-1",
					},
				},
				{
					Name:       "duplicate-name",
					WorkerType: "test-worker",
					UserSpec: config.UserSpec{
						Config: "config-2",
					},
				},
			}

			err := config.ValidateChildSpecs(specs, registry)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("duplicate child spec name"))
			Expect(err.Error()).To(ContainSubstring("duplicate-name"))
		})

		It("should catch duplicate names even when other validations fail", func() {
			specs := []config.ChildSpec{
				{
					Name:       "child-1",
					WorkerType: "communicator",
					UserSpec: config.UserSpec{
						Config: "config",
					},
				},
				{
					Name:       "child-1",
					WorkerType: "unknown-type",
					UserSpec: config.UserSpec{
						Config: "config",
					},
				},
			}

			err := config.ValidateChildSpecs(specs, registry)
			Expect(err).To(HaveOccurred())
			// The individual validation should fail first
			Expect(err.Error()).To(ContainSubstring("unknown worker type"))
		})

		It("should detect duplicate names on the second occurrence", func() {
			specs := []config.ChildSpec{
				{
					Name:       "unique-1",
					WorkerType: "communicator",
					UserSpec: config.UserSpec{
						Config: "config",
					},
				},
				{
					Name:       "unique-2",
					WorkerType: "test-worker",
					UserSpec: config.UserSpec{
						Config: "config",
					},
				},
				{
					Name:       "unique-1",
					WorkerType: "communicator",
					UserSpec: config.UserSpec{
						Config: "config",
					},
				},
			}

			err := config.ValidateChildSpecs(specs, registry)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("duplicate child spec name"))
			Expect(err.Error()).To(ContainSubstring("unique-1"))
		})

		It("should pass validation with multiple specs having different ChildStartStates", func() {
			specs := []config.ChildSpec{
				{
					Name:       "child-with-states",
					WorkerType: "communicator",
					UserSpec: config.UserSpec{
						Config: "config",
					},
					ChildStartStates: []string{"Running"},
				},
				{
					Name:       "child-without-states",
					WorkerType: "test-worker",
					UserSpec: config.UserSpec{
						Config: "config",
					},
				},
			}

			err := config.ValidateChildSpecs(specs, registry)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should fail with proper context when first spec is invalid", func() {
			specs := []config.ChildSpec{
				{
					Name:       "",
					WorkerType: "communicator",
					UserSpec: config.UserSpec{
						Config: "config",
					},
				},
			}

			err := config.ValidateChildSpecs(specs, registry)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("child spec [0]"))
			Expect(err.Error()).To(ContainSubstring("name cannot be empty"))
		})

		It("should validate many specs without early termination", func() {
			specs := []config.ChildSpec{}
			for i := range 10 {
				specs = append(specs, config.ChildSpec{
					Name:       "child-" + string(rune(i+'0')),
					WorkerType: "communicator",
					UserSpec: config.UserSpec{
						Config: "config-" + string(rune(i+'0')),
					},
				})
			}

			err := config.ValidateChildSpecs(specs, registry)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Registry types", func() {
		It("should work with empty registry", func() {
			emptyRegistry := NewMockRegistry()

			spec := config.ChildSpec{
				Name:       "child-1",
				WorkerType: "any-type",
				UserSpec: config.UserSpec{
					Config: "config",
				},
			}

			err := config.ValidateChildSpec(spec, emptyRegistry)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unknown worker type"))
		})

		It("should work with single type in registry", func() {
			singleRegistry := NewMockRegistry("only-type")

			spec := config.ChildSpec{
				Name:       "child-1",
				WorkerType: "only-type",
				UserSpec: config.UserSpec{
					Config: "config",
				},
			}

			err := config.ValidateChildSpec(spec, singleRegistry)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should work with many types in registry", func() {
			manyTypes := []string{
				"type-1", "type-2", "type-3", "type-4", "type-5",
				"communicator", "test-worker",
			}
			manyRegistry := NewMockRegistry(manyTypes...)

			spec := config.ChildSpec{
				Name:       "child-1",
				WorkerType: "type-3",
				UserSpec: config.UserSpec{
					Config: "config",
				},
			}

			err := config.ValidateChildSpec(spec, manyRegistry)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

// createUnmarshalableVariables creates a VariableBundle that cannot be marshaled to JSON.
// This simulates invalid configuration that should be caught during validation.
func createUnmarshalableVariables() config.VariableBundle {
	// Create a channel which is not JSON serializable
	bundle := config.VariableBundle{
		User: map[string]any{
			"invalid": make(chan int),
		},
	}

	return bundle
}
