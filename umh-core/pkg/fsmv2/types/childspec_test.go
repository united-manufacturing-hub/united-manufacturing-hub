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


package types_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/types"
)

func TestTypes(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "FSMv2 Types Suite")
}

var _ = Describe("ChildSpec", func() {
	Describe("YAML serialization", func() {
		It("should serialize to YAML correctly", func() {
			spec := types.ChildSpec{
				Name:       "connection",
				WorkerType: "mqtt_connection",
				UserSpec: types.UserSpec{
					Config: "mqtt:\n  url: tcp://localhost:1883",
				},
				StateMapping: map[string]string{
					"idle":   "stopped",
					"active": "running",
				},
			}

			data, err := yaml.Marshal(spec)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(data)).To(ContainSubstring("name: connection"))
			Expect(string(data)).To(ContainSubstring("workerType: mqtt_connection"))
			Expect(string(data)).To(ContainSubstring("stateMapping:"))
			Expect(string(data)).To(ContainSubstring("idle: stopped"))
		})

		It("should deserialize from YAML correctly", func() {
			yamlData := `
name: connection
workerType: mqtt_connection
userSpec:
  config: "mqtt:\n  url: tcp://localhost:1883"
stateMapping:
  idle: stopped
  active: running
`
			var spec types.ChildSpec
			err := yaml.Unmarshal([]byte(yamlData), &spec)

			Expect(err).ToNot(HaveOccurred())
			Expect(spec.Name).To(Equal("connection"))
			Expect(spec.WorkerType).To(Equal("mqtt_connection"))
			Expect(spec.UserSpec.Config).To(ContainSubstring("tcp://localhost:1883"))
			Expect(spec.StateMapping["idle"]).To(Equal("stopped"))
			Expect(spec.StateMapping["active"]).To(Equal("running"))
		})

		It("should handle optional StateMapping correctly", func() {
			yamlData := `
name: simple-worker
workerType: basic
userSpec:
  config: "simple config"
`
			var spec types.ChildSpec
			err := yaml.Unmarshal([]byte(yamlData), &spec)

			Expect(err).ToNot(HaveOccurred())
			Expect(spec.Name).To(Equal("simple-worker"))
			Expect(spec.StateMapping).To(BeNil())
		})
	})

	Describe("JSON serialization", func() {
		It("should serialize to JSON correctly", func() {
			spec := types.ChildSpec{
				Name:       "dataflow",
				WorkerType: "kafka_consumer",
				UserSpec: types.UserSpec{
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

var _ = Describe("DesiredState", func() {
	Describe("YAML serialization", func() {
		It("should serialize to YAML correctly", func() {
			desired := types.DesiredState{
				State: "running",
				ChildrenSpecs: []types.ChildSpec{
					{
						Name:       "child1",
						WorkerType: "worker_type_a",
						UserSpec: types.UserSpec{
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
			var desired types.DesiredState
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
			var desired types.DesiredState
			err := yaml.Unmarshal([]byte(yamlData), &desired)

			Expect(err).ToNot(HaveOccurred())
			Expect(desired.State).To(Equal("stopped"))
			Expect(desired.ChildrenSpecs).To(BeEmpty())
		})
	})

	Describe("ShutdownRequested interface", func() {
		It("should return false when State is not 'shutdown'", func() {
			desired := types.DesiredState{
				State: "running",
			}

			Expect(desired.ShutdownRequested()).To(BeFalse())
		})

		It("should return true when State is 'shutdown'", func() {
			desired := types.DesiredState{
				State: "shutdown",
			}

			Expect(desired.ShutdownRequested()).To(BeTrue())
		})
	})
})
