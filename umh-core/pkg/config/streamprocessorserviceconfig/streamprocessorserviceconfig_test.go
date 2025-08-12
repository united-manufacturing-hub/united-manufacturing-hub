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

package streamprocessorserviceconfig_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/streamprocessorserviceconfig"
)

func TestYAML(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "StreamProcessor YAML Suite")
}

var _ = Describe("StreamProcessorServiceConfig", func() {
	Describe("SpecToRuntime", func() {
		It("should convert spec config to runtime correctly", func() {
			spec := streamprocessorserviceconfig.StreamProcessorServiceConfigSpec{
				Config: streamprocessorserviceconfig.StreamProcessorServiceConfigTemplate{
					Model: streamprocessorserviceconfig.ModelRef{
						Name:    "test-model",
						Version: "1.0.0",
					},
					Sources: map[string]string{
						"sensor1": "umh.v1.factory.line1.sensor1",
						"sensor2": "umh.v1.factory.line1.sensor2",
					},
					Mapping: map[string]interface{}{
						"field1": "value1",
						"field2": "{{ .sensor1.value }}",
					},
				},
			}

			runtime := streamprocessorserviceconfig.SpecToRuntime(spec)
			Expect(runtime.Model.Name).To(Equal("test-model"))
			Expect(runtime.Model.Version).To(Equal("1.0.0"))
			Expect(runtime.Sources["sensor1"]).To(Equal("umh.v1.factory.line1.sensor1"))
			Expect(runtime.Mapping["field1"]).To(Equal("value1"))
		})
	})

	Describe("ConfigsEqual", func() {
		It("should return true for identical configs", func() {
			config1 := streamprocessorserviceconfig.StreamProcessorServiceConfigSpec{
				Config: streamprocessorserviceconfig.StreamProcessorServiceConfigTemplate{
					Model: streamprocessorserviceconfig.ModelRef{
						Name:    "test-model",
						Version: "1.0.0",
					},
					Sources: map[string]string{
						"sensor1": "umh.v1.factory.line1.sensor1",
					},
				},
			}

			config2 := streamprocessorserviceconfig.StreamProcessorServiceConfigSpec{
				Config: streamprocessorserviceconfig.StreamProcessorServiceConfigTemplate{
					Model: streamprocessorserviceconfig.ModelRef{
						Name:    "test-model",
						Version: "1.0.0",
					},
					Sources: map[string]string{
						"sensor1": "umh.v1.factory.line1.sensor1",
					},
				},
			}

			Expect(streamprocessorserviceconfig.ConfigsEqual(config1, config2)).To(BeTrue())
		})

		It("should return false for different configs", func() {
			config1 := streamprocessorserviceconfig.StreamProcessorServiceConfigSpec{
				Config: streamprocessorserviceconfig.StreamProcessorServiceConfigTemplate{
					Model: streamprocessorserviceconfig.ModelRef{
						Name:    "test-model",
						Version: "1.0.0",
					},
				},
			}

			config2 := streamprocessorserviceconfig.StreamProcessorServiceConfigSpec{
				Config: streamprocessorserviceconfig.StreamProcessorServiceConfigTemplate{
					Model: streamprocessorserviceconfig.ModelRef{
						Name:    "different-model",
						Version: "1.0.0",
					},
				},
			}

			Expect(streamprocessorserviceconfig.ConfigsEqual(config1, config2)).To(BeFalse())
		})
	})

	Describe("ConfigsEqualRuntime", func() {
		It("should correctly compare runtime configs", func() {
			runtime1 := streamprocessorserviceconfig.StreamProcessorServiceConfigRuntime{
				Model: streamprocessorserviceconfig.ModelRef{
					Name:    "test-model",
					Version: "1.0.0",
				},
				Sources: map[string]string{
					"sensor1": "umh.v1.factory.line1.sensor1",
				},
			}

			runtime2 := streamprocessorserviceconfig.StreamProcessorServiceConfigRuntime{
				Model: streamprocessorserviceconfig.ModelRef{
					Name:    "test-model",
					Version: "1.0.0",
				},
				Sources: map[string]string{
					"sensor1": "umh.v1.factory.line1.sensor1",
				},
			}

			Expect(streamprocessorserviceconfig.ConfigsEqualRuntime(runtime1, runtime2)).To(BeTrue())
		})
	})

	Describe("FromDFCServiceConfig", func() {
		It("should extract stream processor config from DFC config", func() {
			// Create a DFC config that matches the provided YAML structure
			dfcConfig := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{}
			dfcConfig.BenthosConfig.Input = map[string]any{
				"uns": map[string]any{
					"umh_topics": []string{
						"umh.v1.corpA.plant-A.aawd._raw.press",
						"umh.v1.corpA.plant-A.aawd._raw.tempF",
						"umh.v1.corpA.plant-A.aawd._raw.run",
					},
				},
			}

			dfcConfig.BenthosConfig.Pipeline = map[string]any{
				"processors": []any{map[string]any{
					"stream_processor": map[string]any{
						"mode": "timeseries",
						"model": map[string]any{
							"name":    "pump",
							"version": "v1",
						},
						"output_topic": "umh.v1.corpA.plant-A.aawd",
						"sources": map[string]any{
							"press": "umh.v1.corpA.plant-A.aawd._raw.press",
							"tF":    "umh.v1.corpA.plant-A.aawd._raw.tempF",
							"r":     "umh.v1.corpA.plant-A.aawd._raw.run",
						},
						"mapping": map[string]any{
							"pressure":    "press+4.00001",
							"temperature": "tF*69/31",
							"motor": map[string]any{
								"rpm": "press/4",
							},
							"serialNumber": `"SN-P42-008"`,
						},
					},
				}},
			}

			dfcConfig.BenthosConfig.Output = map[string]any{
				"uns": map[string]any{},
			}

			// Convert back to StreamProcessor runtime config
			runtime := streamprocessorserviceconfig.FromDFCServiceConfig(dfcConfig)

			// Verify the extracted values
			Expect(runtime.Model.Name).To(Equal("pump"))
			Expect(runtime.Model.Version).To(Equal("v1"))
			Expect(runtime.Sources["press"]).To(Equal("corpA.plant-A.aawd._raw.press"))
			Expect(runtime.Sources["tF"]).To(Equal("corpA.plant-A.aawd._raw.tempF"))
			Expect(runtime.Sources["r"]).To(Equal("corpA.plant-A.aawd._raw.run"))
			Expect(runtime.Mapping["pressure"]).To(Equal("press+4.00001"))
			Expect(runtime.Mapping["temperature"]).To(Equal("tF*69/31"))
			motor, ok := runtime.Mapping["motor"].(map[string]any)
			Expect(ok).To(BeTrue(), "motor mapping should be a map[string]any")
			Expect(motor["rpm"]).To(Equal("press/4"))
			Expect(runtime.Mapping["serialNumber"]).To(Equal(`"SN-P42-008"`))
		})
	})
})
