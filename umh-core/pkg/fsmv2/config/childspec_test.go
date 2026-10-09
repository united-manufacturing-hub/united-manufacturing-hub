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
	"encoding/json"
	"reflect"

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
				Enabled: true,
			}

			data, err := yaml.Marshal(spec)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(data)).To(ContainSubstring("name: connection"))
			Expect(string(data)).To(ContainSubstring("workerType: mqtt_connection"))
			Expect(string(data)).To(ContainSubstring("enabled: true"))
		})

		It("should deserialize from YAML correctly", func() {
			yamlData := `
name: connection
workerType: mqtt_connection
userSpec:
  config: "mqtt:\n  url: tcp://localhost:1883"
enabled: true
`
			var spec config.ChildSpec
			err := yaml.Unmarshal([]byte(yamlData), &spec)

			Expect(err).ToNot(HaveOccurred())
			Expect(spec.Name).To(Equal("connection"))
			Expect(spec.WorkerType).To(Equal("mqtt_connection"))
			Expect(spec.UserSpec.Config).To(ContainSubstring("tcp://localhost:1883"))
			Expect(spec.Enabled).To(BeTrue())
		})

		It("should handle missing optional fields correctly", func() {
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
			Expect(spec.Enabled).To(BeFalse())
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

			data, err := json.Marshal(spec)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(data)).To(ContainSubstring(`"name":"dataflow"`))
			Expect(string(data)).To(ContainSubstring(`"workerType":"kafka_consumer"`))
		})
	})
})

var _ = Describe("ChildSpec Clone", func() {
	Describe("Clone()", func() {
		It("should create a deep copy that is independent from the original", func() {
			// Create original ChildSpec with a UserSpec carrying Variables.
			original := config.ChildSpec{
				Name:       "original-worker",
				WorkerType: "mqtt_connection",
				UserSpec: config.UserSpec{
					Config: "original-config",
					Variables: map[string]any{
						"key1": "value1",
					},
				},
				Enabled: true,
			}

			// Clone it
			cloned := original.Clone()

			// Verify clone has same values
			Expect(cloned.Name).To(Equal(original.Name))
			Expect(cloned.WorkerType).To(Equal(original.WorkerType))
			Expect(cloned.UserSpec.Config).To(Equal(original.UserSpec.Config))
			Expect(cloned.Enabled).To(Equal(original.Enabled))

			// Modify the clone's deep-copied UserSpec variables
			cloned.UserSpec.Variables["key1"] = "modified"

			// Verify the original is NOT affected
			Expect(original.UserSpec.Variables["key1"]).To(Equal("value1"))
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
					Variables: map[string]interface{}{
						"key": "value",
					},
				},
				Enabled: true,
			}

			spec2 := config.ChildSpec{
				Name:       "child-1",
				WorkerType: "mqtt_connection",
				UserSpec: config.UserSpec{
					Config: "test-config",
					Variables: map[string]interface{}{
						"key": "value",
					},
				},
				Enabled: true,
			}

			hash1, err1 := spec1.Hash()
			Expect(err1).NotTo(HaveOccurred())
			hash2, err2 := spec2.Hash()
			Expect(err2).NotTo(HaveOccurred())
			Expect(hash1).To(Equal(hash2))
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

			hash1, err1 := spec1.Hash()
			Expect(err1).NotTo(HaveOccurred())
			hash2, err2 := spec2.Hash()
			Expect(err2).NotTo(HaveOccurred())
			Expect(hash1).NotTo(Equal(hash2))
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

			hash1, err1 := spec1.Hash()
			Expect(err1).NotTo(HaveOccurred())
			hash2, err2 := spec2.Hash()
			Expect(err2).NotTo(HaveOccurred())
			Expect(hash1).NotTo(Equal(hash2))
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

			hash1, err1 := spec1.Hash()
			Expect(err1).NotTo(HaveOccurred())
			hash2, err2 := spec2.Hash()
			Expect(err2).NotTo(HaveOccurred())
			Expect(hash1).NotTo(Equal(hash2))
		})

		It("should produce different hashes when Enabled changes", func() {
			spec1 := config.ChildSpec{
				Name:       "child-1",
				WorkerType: "mqtt_connection",
				UserSpec:   config.UserSpec{Config: "config"},
				Enabled:    false,
			}

			spec2 := config.ChildSpec{
				Name:       "child-1",
				WorkerType: "mqtt_connection",
				UserSpec:   config.UserSpec{Config: "config"},
				Enabled:    true,
			}

			hash1, err1 := spec1.Hash()
			Expect(err1).NotTo(HaveOccurred())
			hash2, err2 := spec2.Hash()
			Expect(err2).NotTo(HaveOccurred())
			Expect(hash1).NotTo(Equal(hash2))
		})

		It("should produce a 16-character hex string", func() {
			spec := config.ChildSpec{
				Name:       "child-1",
				WorkerType: "mqtt_connection",
				UserSpec:   config.UserSpec{Config: "config"},
			}

			hash, err := spec.Hash()
			Expect(err).NotTo(HaveOccurred())
			Expect(hash).To(HaveLen(16))
			// All characters should be hex digits
			for _, c := range hash {
				Expect(string(c)).To(MatchRegexp("[0-9a-f]"))
			}
		})
	})
})

var _ = Describe("UserSpec Clone", func() {
	It("should copy every field deeply, and keep the original when the clone changes", func() {
		spec := config.UserSpec{
			Variables:    map[string]any{"vars": map[string]any{"nested": "original"}},
			Config:       "config: {{ .x }}",
			ConfigValues: map[string]any{"historian": map[string]any{"host": "original"}},
			Names:        map[string]any{"x": map[string]any{"nested": "original"}},
		}

		// Every field of the spec must be non-zero, so a field added to
		// UserSpec later fails this test until the test sets it.
		rv := reflect.ValueOf(spec)
		for i := range rv.NumField() {
			field := rv.Type().Field(i)
			Expect(rv.Field(i).IsZero()).To(BeFalse(),
				"UserSpec field %s must be non-zero so the test keeps covering it", field.Name)
		}

		clone := spec.Clone()

		clone.Variables["vars"].(map[string]any)["nested"] = "changed"
		clone.ConfigValues["historian"].(map[string]any)["host"] = "changed"
		clone.Names["x"].(map[string]any)["nested"] = "changed"

		Expect(spec.Variables["vars"].(map[string]any)["nested"]).To(Equal("original"))
		Expect(spec.ConfigValues["historian"].(map[string]any)["host"]).To(Equal("original"))
		Expect(spec.Names["x"].(map[string]any)["nested"]).To(Equal("original"))
	})
})

var _ = Describe("UserSpec serialization", func() {
	It("should keep the handed-down maps through a JSON round trip", func() {
		spec := config.UserSpec{
			Variables:    map[string]any{"a": "b"},
			Config:       "config",
			ConfigValues: map[string]any{"historian": map[string]any{"host": "db"}},
			Names:        map[string]any{"x": "from-parent"},
		}

		data, err := json.Marshal(spec)
		Expect(err).ToNot(HaveOccurred())

		var back config.UserSpec
		err = json.Unmarshal(data, &back)
		Expect(err).ToNot(HaveOccurred())
		Expect(back.ConfigValues).To(Equal(map[string]any{"historian": map[string]any{"host": "db"}}))
		Expect(back.Names).To(Equal(map[string]any{"x": "from-parent"}))
	})

	It("should leave the handed-down maps nil when a user sets their keys in YAML", func() {
		yamlData := `
config: "user config"
config_values:
  a: 1
names:
  a: 1
`

		var spec config.UserSpec
		err := yaml.Unmarshal([]byte(yamlData), &spec)

		Expect(err).ToNot(HaveOccurred())
		Expect(spec.Config).To(Equal("user config"))
		Expect(spec.ConfigValues).To(BeNil())
		Expect(spec.Names).To(BeNil())
	})

	It("should decode a user's variables section straight into Variables", func() {
		yamlData := `
variables:
  IP: 10.0.0.1
`

		var spec config.UserSpec
		err := yaml.Unmarshal([]byte(yamlData), &spec)

		Expect(err).ToNot(HaveOccurred())
		Expect(spec.Variables).To(Equal(map[string]any{"IP": "10.0.0.1"}))
	})
})

var _ = Describe("DesiredState", func() {
	Describe("YAML serialization", func() {
		It("should serialize to YAML correctly", func() {
			desired := config.DesiredState{
				State: "running",
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
				State: "running",
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
