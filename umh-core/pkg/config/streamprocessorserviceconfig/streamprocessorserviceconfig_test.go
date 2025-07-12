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
})
