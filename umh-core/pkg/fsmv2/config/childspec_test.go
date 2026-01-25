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
	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

var _ = Describe("ChildSpec", func() {
	Describe("YAML serialization", func() {
		It("should serialize to YAML correctly", func() {
			spec := config.ChildSpec{
				Name:       "connection",
				WorkerType: "mqtt_connection",
				UserSpec: config.UserSpec{
					Config: "mqtt:\n  url: tcp://localhost:1883",
				},
				ChildStartStates: []string{"Running", "TryingToStart"},
			}

			data, err := yaml.Marshal(spec)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(data)).To(ContainSubstring("name: connection"))
			Expect(string(data)).To(ContainSubstring("workerType: mqtt_connection"))
			Expect(string(data)).To(ContainSubstring("childStartStates:"))
			Expect(string(data)).To(ContainSubstring("- Running"))
		})

		It("should deserialize from YAML correctly", func() {
			yamlData := `
name: connection
workerType: mqtt_connection
userSpec:
  config: "mqtt:\n  url: tcp://localhost:1883"
childStartStates:
  - Running
  - TryingToStart
`
			var spec config.ChildSpec
			err := yaml.Unmarshal([]byte(yamlData), &spec)

			Expect(err).ToNot(HaveOccurred())
			Expect(spec.Name).To(Equal("connection"))
			Expect(spec.WorkerType).To(Equal("mqtt_connection"))
			Expect(spec.UserSpec.Config).To(ContainSubstring("tcp://localhost:1883"))
			Expect(spec.ChildStartStates).To(ConsistOf("Running", "TryingToStart"))
		})

		It("should handle optional ChildStartStates correctly", func() {
			yamlData := `
name: simple-worker
workerType: basic
userSpec:
  config: "simple config"
`
			var spec config.ChildSpec
			err := yaml.Unmarshal([]byte(yamlData), &spec)

			Expect(err).ToNot(HaveOccurred())
			Expect(spec.Name).To(Equal("simple-worker"))
			Expect(spec.ChildStartStates).To(BeNil())
		})
	})

	Describe("JSON serialization", func() {
		It("should serialize to JSON correctly", func() {
			spec := config.ChildSpec{
				Name:       "dataflow",
				WorkerType: "kafka_consumer",
				UserSpec: config.UserSpec{
					Config: "topic: sensor-data",
				},
			}

			data, err := spec.MarshalJSON()
			Expect(err).ToNot(HaveOccurred())
			Expect(string(data)).To(ContainSubstring(`"name":"dataflow"`))
			Expect(string(data)).To(ContainSubstring(`"workerType":"kafka_consumer"`))
		})
	})
})

var _ = Describe("ChildSpec Clone", func() {
	Describe("Clone()", func() {
		It("should create a deep copy that is independent from the original", func() {
			// Create original ChildSpec with UserSpec and ChildStartStates
			original := config.ChildSpec{
				Name:       "original-worker",
				WorkerType: "mqtt_connection",
				UserSpec: config.UserSpec{
					Config: "original-config",
					Variables: config.VariableBundle{
						User: map[string]interface{}{
							"key1": "value1",
						},
					},
				},
				ChildStartStates: []string{"Running", "TryingToStart"},
			}

			// Clone it
			cloned := original.Clone()

			// Verify clone has same values
			Expect(cloned.Name).To(Equal(original.Name))
			Expect(cloned.WorkerType).To(Equal(original.WorkerType))
			Expect(cloned.UserSpec.Config).To(Equal(original.UserSpec.Config))
			Expect(cloned.ChildStartStates).To(Equal(original.ChildStartStates))

			// Modify the clone's ChildStartStates slice
			cloned.ChildStartStates[0] = "Modified"
			cloned.ChildStartStates = append(cloned.ChildStartStates, "NewState")

			// Verify the original is NOT affected
			Expect(original.ChildStartStates).To(Equal([]string{"Running", "TryingToStart"}))
			Expect(original.ChildStartStates).ToNot(ContainElement("Modified"))
			Expect(original.ChildStartStates).ToNot(ContainElement("NewState"))
		})

		It("should handle nil ChildStartStates", func() {
			original := config.ChildSpec{
				Name:             "worker-no-states",
				WorkerType:       "basic",
				ChildStartStates: nil,
			}

			cloned := original.Clone()

			Expect(cloned.ChildStartStates).To(BeNil())
		})

		It("should handle empty ChildStartStates slice", func() {
			original := config.ChildSpec{
				Name:             "worker-empty-states",
				WorkerType:       "basic",
				ChildStartStates: []string{},
			}

			cloned := original.Clone()

			Expect(cloned.ChildStartStates).ToNot(BeNil())
			Expect(cloned.ChildStartStates).To(BeEmpty())
		})
	})
})

var _ = Describe("ChildSpec Hash", func() {
	Describe("Hash()", func() {
		It("should produce deterministic hashes for identical specs", func() {
			spec1 := config.ChildSpec{
				Name:       "child-1",
				WorkerType: "mqtt_connection",
				UserSpec: config.UserSpec{
					Config: "test-config",
					Variables: config.VariableBundle{
						User: map[string]interface{}{
							"key": "value",
						},
					},
				},
				ChildStartStates: []string{"Running", "TryingToStart"},
			}

			spec2 := config.ChildSpec{
				Name:       "child-1",
				WorkerType: "mqtt_connection",
				UserSpec: config.UserSpec{
					Config: "test-config",
					Variables: config.VariableBundle{
						User: map[string]interface{}{
							"key": "value",
						},
					},
				},
				ChildStartStates: []string{"Running", "TryingToStart"},
			}

			Expect(spec1.Hash()).To(Equal(spec2.Hash()))
		})

		It("should produce different hashes when Name changes", func() {
			spec1 := config.ChildSpec{
				Name:       "child-1",
				WorkerType: "mqtt_connection",
				UserSpec:   config.UserSpec{Config: "config"},
			}

			spec2 := config.ChildSpec{
				Name:       "child-2",
				WorkerType: "mqtt_connection",
				UserSpec:   config.UserSpec{Config: "config"},
			}

			Expect(spec1.Hash()).NotTo(Equal(spec2.Hash()))
		})

		It("should produce different hashes when WorkerType changes", func() {
			spec1 := config.ChildSpec{
				Name:       "child-1",
				WorkerType: "mqtt_connection",
				UserSpec:   config.UserSpec{Config: "config"},
			}

			spec2 := config.ChildSpec{
				Name:       "child-1",
				WorkerType: "modbus_connection",
				UserSpec:   config.UserSpec{Config: "config"},
			}

			Expect(spec1.Hash()).NotTo(Equal(spec2.Hash()))
		})

		It("should produce different hashes when UserSpec.Config changes", func() {
			spec1 := config.ChildSpec{
				Name:       "child-1",
				WorkerType: "mqtt_connection",
				UserSpec:   config.UserSpec{Config: "config-a"},
			}

			spec2 := config.ChildSpec{
				Name:       "child-1",
				WorkerType: "mqtt_connection",
				UserSpec:   config.UserSpec{Config: "config-b"},
			}

			Expect(spec1.Hash()).NotTo(Equal(spec2.Hash()))
		})

		It("should produce different hashes when ChildStartStates changes", func() {
			spec1 := config.ChildSpec{
				Name:             "child-1",
				WorkerType:       "mqtt_connection",
				UserSpec:         config.UserSpec{Config: "config"},
				ChildStartStates: []string{"Running"},
			}

			spec2 := config.ChildSpec{
				Name:             "child-1",
				WorkerType:       "mqtt_connection",
				UserSpec:         config.UserSpec{Config: "config"},
				ChildStartStates: []string{"Running", "TryingToStart"},
			}

			Expect(spec1.Hash()).NotTo(Equal(spec2.Hash()))
		})

		It("should produce a 16-character hex string", func() {
			spec := config.ChildSpec{
				Name:       "child-1",
				WorkerType: "mqtt_connection",
				UserSpec:   config.UserSpec{Config: "config"},
			}

			hash := spec.Hash()
			Expect(len(hash)).To(Equal(16))
			// All characters should be hex digits
			for _, c := range hash {
				Expect(string(c)).To(MatchRegexp("[0-9a-f]"))
			}
		})
	})
})

var _ = Describe("DesiredState", func() {
	Describe("YAML serialization", func() {
		It("should serialize to YAML correctly", func() {
			desired := config.DesiredState{
				BaseDesiredState: config.BaseDesiredState{State: "running"},
				ChildrenSpecs: []config.ChildSpec{
					{
						Name:       "child1",
						WorkerType: "worker_type_a",
						UserSpec: config.UserSpec{
							Config: "config1",
						},
					},
				},
			}

			data, err := yaml.Marshal(desired)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(data)).To(ContainSubstring("state: running"))
			Expect(string(data)).To(ContainSubstring("childrenSpecs:"))
			Expect(string(data)).To(ContainSubstring("name: child1"))
		})

		It("should deserialize from YAML correctly", func() {
			yamlData := `
state: active
childrenSpecs:
  - name: connection
    workerType: mqtt
    userSpec:
      config: "mqtt config"
  - name: dataflow
    workerType: processor
    userSpec:
      config: "processor config"
`
			var desired config.DesiredState
			err := yaml.Unmarshal([]byte(yamlData), &desired)

			Expect(err).ToNot(HaveOccurred())
			Expect(desired.State).To(Equal("active"))
			Expect(desired.ChildrenSpecs).To(HaveLen(2))
			Expect(desired.ChildrenSpecs[0].Name).To(Equal("connection"))
			Expect(desired.ChildrenSpecs[1].Name).To(Equal("dataflow"))
		})

		It("should handle empty ChildrenSpecs correctly", func() {
			yamlData := `
state: stopped
`
			var desired config.DesiredState
			err := yaml.Unmarshal([]byte(yamlData), &desired)

			Expect(err).ToNot(HaveOccurred())
			Expect(desired.State).To(Equal("stopped"))
			Expect(desired.ChildrenSpecs).To(BeEmpty())
		})
	})

	Describe("ShutdownRequested interface", func() {
		It("should return false when State is not 'shutdown'", func() {
			desired := config.DesiredState{
				BaseDesiredState: config.BaseDesiredState{State: "running"},
			}

			Expect(desired.IsShutdownRequested()).To(BeFalse())
		})

		It("should return true when ShutdownRequested is set", func() {
			desired := config.DesiredState{}
			desired.SetShutdownRequested(true)

			Expect(desired.IsShutdownRequested()).To(BeTrue())
		})
	})
})
