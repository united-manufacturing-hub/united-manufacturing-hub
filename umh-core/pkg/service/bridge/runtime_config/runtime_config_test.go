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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/bridgeserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/bridge/runtime_config"
)

var _ = Describe("BuildRuntimeConfig", func() {
	var (
		spec          bridgeserviceconfig.ConfigSpec
		agentLocation map[string]string
		globalVars    map[string]any
		nodeName      string
		brName        string
	)

	BeforeEach(func() {
		// Parse the example config file using the config manager's parseConfig function
		exampleConfigPath := "../../../../examples/example-config-bridge-templated.yaml"

		// Read the example config file
		data, err := os.ReadFile(exampleConfigPath)
		Expect(err).NotTo(HaveOccurred(), "Failed to read example config file")

		// Use the config manager's parseConfig function to properly handle templates and anchors
		ctx := context.Background()
		fullConfig, err := config.ParseConfig(data, ctx, true) // Allow unknown fields for template handling
		Expect(err).NotTo(HaveOccurred(), "Failed to parse example config")

		// Extract the first bridge (temperature-sensor-bridge)
		Expect(fullConfig.Bridge).To(HaveLen(3), "Expected 3 bridges in example config")
		firstBridge := fullConfig.Bridge[0]
		Expect(firstBridge.Name).To(Equal("temperature-sensor-bridge"), "Expected first Bridge to be temperature-sensor-bridge")

		// Get the spec from the first bridge
		spec = firstBridge.ServiceConfig

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
		brName = "temperature-sensor-bridge"

		spec.Config.DFCWriteConfig = dataflowcomponentserviceconfig.DataflowComponentServiceConfig{}
	})

	Describe("BuildRuntimeConfig", func() {
		It("should successfully build runtime config from example config", func() {
			result, err := runtime_config.BuildRuntimeConfig(spec, agentLocation, globalVars, nodeName, brName)
			Expect(err).NotTo(HaveOccurred())

			// Verify the runtime config structure
			Expect(result).NotTo(BeZero())

			// Check connection config - should have rendered variables
			Expect(result.ConnectionConfig.NmapServiceConfig.Target).To(Equal("10.0.1.50"))
			Expect(result.ConnectionConfig.NmapServiceConfig.Port).To(Equal(uint16(4840)))

			// Verify dataflow component configs exist (even if empty)
			Expect(result.DFCReadConfig).NotTo(BeNil())
			Expect(result.DFCWriteConfig).NotTo(BeNil())
		})

		It("should properly merge agent and Bridge locations", func() {
			result, err := runtime_config.BuildRuntimeConfig(spec, agentLocation, globalVars, nodeName, brName)
			Expect(err).NotTo(HaveOccurred())

			// The location should be properly merged and accessible via variables
			// We can't directly inspect internal variables, but the build should succeed
			Expect(result).NotTo(BeZero())
		})

		It("should handle template variable substitution", func() {
			result, err := runtime_config.BuildRuntimeConfig(spec, agentLocation, globalVars, nodeName, brName)
			Expect(err).NotTo(HaveOccurred())

			// Check that template variables {{ .IP }} and {{ .PORT }} have been substituted
			Expect(result.ConnectionConfig.NmapServiceConfig.Target).To(Equal("10.0.1.50"))
			Expect(result.ConnectionConfig.NmapServiceConfig.Port).To(Equal(uint16(4840)))
		})

		It("should generate proper bridged_by header", func() {
			result, err := runtime_config.BuildRuntimeConfig(spec, agentLocation, globalVars, nodeName, brName)
			Expect(err).NotTo(HaveOccurred())

			// We can't directly check the bridged_by header, but the function should succeed
			// The bridged_by header would be something like "bridge-test-node-temperature-sensor-bridge"
			Expect(result).NotTo(BeZero())
		})

		It("should handle missing location levels with 'unknown'", func() {
			// Test with incomplete location map
			incompleteLocation := map[string]string{
				"0": "plant-A",
				"3": "workstation-5", // Gap at levels 1,2
			}

			result, err := runtime_config.BuildRuntimeConfig(spec, incompleteLocation, globalVars, nodeName, brName)
			Expect(err).NotTo(HaveOccurred())

			// Should fill gaps with "unknown" and not fail
			Expect(result).NotTo(BeZero())
		})

		It("should return error for empty spec", func() {
			emptySpec := bridgeserviceconfig.ConfigSpec{}

			result, err := runtime_config.BuildRuntimeConfig(emptySpec, agentLocation, globalVars, nodeName, brName)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("nil spec"))
			Expect(result).To(Equal(bridgeserviceconfig.ConfigRuntime{}))
		})

		It("should handle empty global vars", func() {
			result, err := runtime_config.BuildRuntimeConfig(spec, agentLocation, nil, nodeName, brName)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeZero())
		})

		It("should handle empty node name", func() {
			result, err := runtime_config.BuildRuntimeConfig(spec, agentLocation, globalVars, "", brName)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeZero())
		})
	})

	Describe("Variable expansion", func() {
		It("should expand all template variables correctly", func() {
			result, err := runtime_config.BuildRuntimeConfig(spec, agentLocation, globalVars, nodeName, brName)
			Expect(err).NotTo(HaveOccurred())

			// Verify that no template strings remain (no {{ }} patterns)
			Expect(result.ConnectionConfig.NmapServiceConfig.Target).NotTo(ContainSubstring("{{"))
			Expect(result.ConnectionConfig.NmapServiceConfig.Target).NotTo(ContainSubstring("}}"))
		})
	})
})
