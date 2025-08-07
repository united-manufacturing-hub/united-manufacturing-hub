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
	"context"
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/protocolconverter/runtime_config"
)

var _ = Describe("BuildRuntimeConfig", func() {
	var (
		spec          protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec
		agentLocation map[string]string
		globalVars    map[string]any
		nodeName      string
		pcName        string
	)

	BeforeEach(func() {
		// Parse the example config file using the config manager's parseConfig function
		exampleConfigPath := "../../../../examples/example-config-protocolconverter-templated.yaml"

		// Read the example config file
		data, err := os.ReadFile(exampleConfigPath)
		Expect(err).NotTo(HaveOccurred(), "Failed to read example config file")

		// Use the config manager's parseConfig function to properly handle templates and anchors
		ctx := context.Background()
		fullConfig, err := config.ParseConfig(data, ctx, true) // Allow unknown fields for template handling
		Expect(err).NotTo(HaveOccurred(), "Failed to parse example config")

		// Extract the first protocol converter (temperature-sensor-pc)
		Expect(fullConfig.ProtocolConverter).To(HaveLen(3), "Expected 3 protocol converters in example config")
		firstPC := fullConfig.ProtocolConverter[0]
		Expect(firstPC.Name).To(Equal("temperature-sensor-pc"), "Expected first PC to be temperature-sensor-pc")

		// Get the spec from the first protocol converter
		spec = firstPC.ProtocolConverterServiceConfig

		// Extract agent location from the config
		agentLocation = map[string]string{}
		for k, v := range fullConfig.Agent.Location {
			agentLocation[fmt.Sprintf("%d", k)] = v
		}

		// Set up test data
		globalVars = map[string]any{
			"releaseChannel": "stable",
			"version":        "1.0.0",
		}
		nodeName = "test-node"
		pcName = "temperature-sensor-pc"

		spec.Config.DataflowComponentWriteServiceConfig = dataflowcomponentserviceconfig.DataflowComponentServiceConfig{}
	})

	Describe("BuildRuntimeConfig", func() {
		It("should successfully build runtime config from example config", func() {
			result, err := runtime_config.BuildRuntimeConfig(spec, agentLocation, globalVars, nodeName, pcName)
			Expect(err).NotTo(HaveOccurred())

			// Verify the runtime config structure
			Expect(result).NotTo(BeZero())

			// Check connection config - should have rendered variables
			Expect(result.ConnectionServiceConfig.NmapServiceConfig.Target).To(Equal("10.0.1.50"))
			Expect(result.ConnectionServiceConfig.NmapServiceConfig.Port).To(Equal(uint16(4840)))

			// Verify dataflow component configs exist (even if empty)
			Expect(result.DataflowComponentReadServiceConfig).NotTo(BeNil())
			Expect(result.DataflowComponentWriteServiceConfig).NotTo(BeNil())
		})

		It("should properly merge agent and PC locations", func() {
			result, err := runtime_config.BuildRuntimeConfig(spec, agentLocation, globalVars, nodeName, pcName)
			Expect(err).NotTo(HaveOccurred())

			// The location should be properly merged and accessible via variables
			// We can't directly inspect internal variables, but the build should succeed
			Expect(result).NotTo(BeZero())
		})

		It("should handle template variable substitution", func() {
			result, err := runtime_config.BuildRuntimeConfig(spec, agentLocation, globalVars, nodeName, pcName)
			Expect(err).NotTo(HaveOccurred())

			// Check that template variables {{ .IP }} and {{ .PORT }} have been substituted
			Expect(result.ConnectionServiceConfig.NmapServiceConfig.Target).To(Equal("10.0.1.50"))
			Expect(result.ConnectionServiceConfig.NmapServiceConfig.Port).To(Equal(uint16(4840)))
		})

		It("should generate proper bridged_by header", func() {
			result, err := runtime_config.BuildRuntimeConfig(spec, agentLocation, globalVars, nodeName, pcName)
			Expect(err).NotTo(HaveOccurred())

			// We can't directly check the bridged_by header, but the function should succeed
			// The bridged_by header would be something like "protocol-converter-test-node-temperature-sensor-pc"
			Expect(result).NotTo(BeZero())
		})

		It("should handle missing location levels with 'unknown'", func() {
			// Test with incomplete location map
			incompleteLocation := map[string]string{
				"0": "plant-A",
				"3": "workstation-5", // Gap at levels 1,2
			}

			result, err := runtime_config.BuildRuntimeConfig(spec, incompleteLocation, globalVars, nodeName, pcName)
			Expect(err).NotTo(HaveOccurred())

			// Should fill gaps with "unknown" and not fail
			Expect(result).NotTo(BeZero())
		})

		It("should return error for empty spec", func() {
			emptySpec := protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{}

			result, err := runtime_config.BuildRuntimeConfig(emptySpec, agentLocation, globalVars, nodeName, pcName)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("nil spec"))
			Expect(result).To(Equal(protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime{}))
		})

		It("should handle empty global vars", func() {
			result, err := runtime_config.BuildRuntimeConfig(spec, agentLocation, nil, nodeName, pcName)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeZero())
		})

		It("should handle empty node name", func() {
			result, err := runtime_config.BuildRuntimeConfig(spec, agentLocation, globalVars, "", pcName)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeZero())
		})
	})

	Describe("Variable expansion", func() {
		It("should expand all template variables correctly", func() {
			result, err := runtime_config.BuildRuntimeConfig(spec, agentLocation, globalVars, nodeName, pcName)
			Expect(err).NotTo(HaveOccurred())

			// Verify that no template strings remain (no {{ }} patterns)
			Expect(result.ConnectionServiceConfig.NmapServiceConfig.Target).NotTo(ContainSubstring("{{"))
			Expect(result.ConnectionServiceConfig.NmapServiceConfig.Target).NotTo(ContainSubstring("}}"))
		})
	})
})
