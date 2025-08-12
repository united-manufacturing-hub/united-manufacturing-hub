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

package runtime_config_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/streamprocessorserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/streamprocessor/runtime_config"
)

func TestRuntimeConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "StreamProcessor Runtime Config Suite")
}

var _ = Describe("BuildRuntimeConfig", func() {
	var (
		spec          streamprocessorserviceconfig.StreamProcessorServiceConfigSpec
		agentLocation map[string]string
		globalVars    map[string]any
		nodeName      string
		spName        string
	)

	BeforeEach(func() {
		// Set up a basic stream processor config
		spec = streamprocessorserviceconfig.StreamProcessorServiceConfigSpec{
			Config: streamprocessorserviceconfig.StreamProcessorServiceConfigTemplate{
				Model: streamprocessorserviceconfig.ModelRef{
					Name:    "pump",
					Version: "v1",
				},
				Sources: map[string]string{
					"vibration_sensor": "{{ .location_path }}.vibration",
					"count_sensor":     "{{ .location_path }}.count",
				},
				Mapping: map[string]any{
					"count":     "count_sensor.value",
					"vibration": "vibration_sensor",
				},
			},
		}

		agentLocation = map[string]string{
			"0": "factory-A",
			"1": "line-1",
		}

		globalVars = map[string]any{
			"releaseChannel": "stable",
			"version":        "1.0.0",
		}
		nodeName = "test-node"
		spName = "test-sp"
	})

	Describe("BuildRuntimeConfig", func() {
		It("should successfully build runtime config", func() {
			spRuntime, dfcRuntime, err := runtime_config.BuildRuntimeConfig(spec, agentLocation, globalVars, nodeName, spName)
			Expect(err).NotTo(HaveOccurred())

			// Verify the runtime config structure
			Expect(spRuntime).NotTo(BeZero())
			Expect(dfcRuntime).NotTo(BeZero())

			// Check model reference
			Expect(spRuntime.Model.Name).To(Equal("pump"))
			Expect(spRuntime.Model.Version).To(Equal("v1"))

			// Check that sources have been rendered with location_path
			Expect(spRuntime.Sources["vibration_sensor"]).To(Equal("factory-A.line-1.vibration"))
			Expect(spRuntime.Sources["count_sensor"]).To(Equal("factory-A.line-1.count"))

			// Verify DFC config has proper UNS structure
			Expect(dfcRuntime.BenthosConfig.Input).To(HaveKey("uns"))
			Expect(dfcRuntime.BenthosConfig.Output).To(HaveKey("uns"))
			Expect(dfcRuntime.BenthosConfig.Pipeline).To(HaveKey("processors"))

			// Check UNS output has bridged_by value (it should be rendered at this point)
			unsOutput, ok := dfcRuntime.BenthosConfig.Output["uns"].(map[string]any)
			Expect(ok).To(BeTrue(), "UNS output should be a map[string]any")
			Expect(unsOutput).To(HaveKey("bridged_by"))
			Expect(unsOutput["bridged_by"]).To(Equal("stream-processor_test-node_test-sp"))
		})

		It("should generate proper bridged_by header", func() {
			spRuntime, dfcRuntime, err := runtime_config.BuildRuntimeConfig(spec, agentLocation, globalVars, nodeName, spName)
			Expect(err).NotTo(HaveOccurred())

			// Verify that bridged_by is properly rendered in UNS output
			unsOutput, ok := dfcRuntime.BenthosConfig.Output["uns"].(map[string]any)
			Expect(ok).To(BeTrue(), "UNS output should be a map[string]any")
			Expect(unsOutput["bridged_by"]).To(Equal("stream-processor_test-node_test-sp"))

			// Verify the rendered value follows the expected format
			Expect(spRuntime).NotTo(BeZero())
		})

		It("should sanitize bridged_by header correctly", func() {
			// Test with special characters in node name and sp name
			spRuntime, dfcRuntime, err := runtime_config.BuildRuntimeConfig(spec, agentLocation, globalVars, "test@node#1", "test.sp@2")
			Expect(err).NotTo(HaveOccurred())

			// Verify the bridged_by is properly sanitized
			unsOutput := dfcRuntime.BenthosConfig.Output["uns"].(map[string]any)
			Expect(unsOutput["bridged_by"]).To(Equal("stream-processor_test-node-1_test-sp-2"))

			// Verify sanitization removed special characters
			Expect(spRuntime).NotTo(BeZero())
		})

		It("should handle empty node name", func() {
			spRuntime, dfcRuntime, err := runtime_config.BuildRuntimeConfig(spec, agentLocation, globalVars, "", spName)
			Expect(err).NotTo(HaveOccurred())

			// Should default to "unknown" for empty node name
			unsOutput := dfcRuntime.BenthosConfig.Output["uns"].(map[string]any)
			Expect(unsOutput["bridged_by"]).To(Equal("stream-processor_unknown_test-sp"))

			Expect(spRuntime).NotTo(BeZero())
		})

		It("should return error for empty spec", func() {
			emptySpec := streamprocessorserviceconfig.StreamProcessorServiceConfigSpec{}

			spRuntime, dfcRuntime, err := runtime_config.BuildRuntimeConfig(emptySpec, agentLocation, globalVars, nodeName, spName)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("nil spec"))
			Expect(spRuntime).To(Equal(streamprocessorserviceconfig.StreamProcessorServiceConfigRuntime{}))
			Expect(dfcRuntime).To(Equal(dataflowcomponentserviceconfig.DataflowComponentServiceConfig{}))
		})

		It("should handle missing location levels with 'unknown'", func() {
			// Test with incomplete location map
			incompleteLocation := map[string]string{
				"0": "factory-A",
				"3": "workstation-5", // Gap at levels 1,2
			}

			spRuntime, dfcRuntime, err := runtime_config.BuildRuntimeConfig(spec, incompleteLocation, globalVars, nodeName, spName)
			Expect(err).NotTo(HaveOccurred())

			// Should fill gaps with "unknown" and not fail
			Expect(spRuntime.Sources["vibration_sensor"]).To(Equal("factory-A.unknown.unknown.workstation-5.vibration"))
			Expect(dfcRuntime).NotTo(BeZero())
		})

		It("should handle empty global vars", func() {
			spRuntime, dfcRuntime, err := runtime_config.BuildRuntimeConfig(spec, agentLocation, nil, nodeName, spName)
			Expect(err).NotTo(HaveOccurred())
			Expect(spRuntime).NotTo(BeZero())
			Expect(dfcRuntime).NotTo(BeZero())
		})
	})
})
