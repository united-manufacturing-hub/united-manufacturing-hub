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

package protocolconverterserviceconfig_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
)

func TestProtocolConverterDebugLevel_EndToEnd_DebugEnabled(t *testing.T) {
	// Test the complete flow with debug_level: true
	yamlData := `
config:
  debug_level: true
  connection:
    name: "test-connection"
    nmap:
      target: "127.0.0.1"
      port: "502"
  dataflowcomponent_read:
    benthos:
      input:
        generate:
          mapping: 'root = "test"'
      output:
        drop: {}
  dataflowcomponent_write:
    benthos:
      input:
        drop: {}
      output:
        drop: {}
`

	// 1. Parse YAML to Spec
	var spec protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec
	err := yaml.Unmarshal([]byte(yamlData), &spec)
	require.NoError(t, err)

	// 2. Verify debug_level parsed correctly
	require.True(t, spec.Config.DebugLevel)

	// 3. Convert Spec to Runtime
	runtime, err := protocolconverterserviceconfig.SpecToRuntime(spec)
	require.NoError(t, err)

	// 4. Verify debug_level preserved in Runtime
	require.True(t, runtime.DebugLevel)

	// 5. Extract DataflowComponentServiceConfigs
	readDFC := runtime.DataflowComponentReadServiceConfig
	writeDFC := runtime.DataflowComponentWriteServiceConfig

	// 6. Verify debug_level propagated to DataflowComponentServiceConfigs
	require.True(t, readDFC.DebugLevel)
	require.True(t, writeDFC.DebugLevel)

	// 7. Convert DataflowComponentServiceConfigs to BenthosServiceConfigs
	readBenthos := readDFC.GetBenthosServiceConfig()
	writeBenthos := writeDFC.GetBenthosServiceConfig()

	// 8. Verify LogLevel set to DEBUG in BenthosServiceConfigs
	assert.Equal(t, "DEBUG", readBenthos.LogLevel)
	assert.Equal(t, "DEBUG", writeBenthos.LogLevel)

	// 9. Generate S6 configs using BenthosService
	benthosService := benthos.NewDefaultBenthosService("integration-test")

	readS6, err := benthosService.GenerateS6ConfigForBenthos(&readBenthos, "test-read")
	require.NoError(t, err)

	writeS6, err := benthosService.GenerateS6ConfigForBenthos(&writeBenthos, "test-write")
	require.NoError(t, err)

	// 10. Verify OPC_DEBUG environment variable set in S6 configs
	assert.Equal(t, "true", readS6.Env["OPC_DEBUG"])
	assert.Equal(t, "true", writeS6.Env["OPC_DEBUG"])
}

func TestProtocolConverterDebugLevel_EndToEnd_DebugDisabled(t *testing.T) {
	// Test the complete flow with debug_level: false
	yamlData := `
config:
  debug_level: false
  connection:
    name: "test-connection"
    nmap:
      target: "127.0.0.1"
      port: "502"
  dataflowcomponent_read:
    benthos:
      input:
        generate:
          mapping: 'root = "test"'
      output:
        drop: {}
  dataflowcomponent_write:
    benthos:
      input:
        drop: {}
      output:
        drop: {}
`

	// 1. Parse YAML to Spec
	var spec protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec
	err := yaml.Unmarshal([]byte(yamlData), &spec)
	require.NoError(t, err)

	// 2. Verify debug_level parsed correctly
	require.False(t, spec.Config.DebugLevel)

	// 3. Convert Spec to Runtime
	runtime, err := protocolconverterserviceconfig.SpecToRuntime(spec)
	require.NoError(t, err)

	// 4. Verify debug_level preserved in Runtime
	require.False(t, runtime.DebugLevel)

	// 5. Extract DataflowComponentServiceConfigs
	readDFC := runtime.DataflowComponentReadServiceConfig
	writeDFC := runtime.DataflowComponentWriteServiceConfig

	// 6. Verify debug_level propagated to DataflowComponentServiceConfigs
	require.False(t, readDFC.DebugLevel)
	require.False(t, writeDFC.DebugLevel)

	// 7. Convert DataflowComponentServiceConfigs to BenthosServiceConfigs
	readBenthos := readDFC.GetBenthosServiceConfig()
	writeBenthos := writeDFC.GetBenthosServiceConfig()

	// 8. Verify LogLevel set to INFO (default) in BenthosServiceConfigs
	assert.Equal(t, "INFO", readBenthos.LogLevel)
	assert.Equal(t, "INFO", writeBenthos.LogLevel)

	// 9. Generate S6 configs using BenthosService
	benthosService := benthos.NewDefaultBenthosService("integration-test")

	readS6, err := benthosService.GenerateS6ConfigForBenthos(&readBenthos, "test-read")
	require.NoError(t, err)

	writeS6, err := benthosService.GenerateS6ConfigForBenthos(&writeBenthos, "test-write")
	require.NoError(t, err)

	// 10. Verify OPC_DEBUG environment variable NOT set in S6 configs
	_, readOpcDebugExists := readS6.Env["OPC_DEBUG"]
	_, writeOpcDebugExists := writeS6.Env["OPC_DEBUG"]
	assert.False(t, readOpcDebugExists, "OPC_DEBUG should not be set when LogLevel is INFO")
	assert.False(t, writeOpcDebugExists, "OPC_DEBUG should not be set when LogLevel is INFO")
}

func TestProtocolConverterDebugLevel_EndToEnd_DebugOmitted(t *testing.T) {
	// Test the complete flow without debug_level field (should default to false/INFO)
	yamlData := `
config:
  connection:
    name: "test-connection"
    nmap:
      target: "127.0.0.1"
      port: "502"
  dataflowcomponent_read:
    benthos:
      input:
        generate:
          mapping: 'root = "test"'
      output:
        drop: {}
  dataflowcomponent_write:
    benthos:
      input:
        drop: {}
      output:
        drop: {}
`

	// 1. Parse YAML to Spec
	var spec protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec
	err := yaml.Unmarshal([]byte(yamlData), &spec)
	require.NoError(t, err)

	// 2. Verify debug_level defaults to false
	require.False(t, spec.Config.DebugLevel)

	// 3. Convert Spec to Runtime
	runtime, err := protocolconverterserviceconfig.SpecToRuntime(spec)
	require.NoError(t, err)

	// 4. Verify debug_level preserved in Runtime
	require.False(t, runtime.DebugLevel)

	// 5. Extract DataflowComponentServiceConfigs
	readDFC := runtime.DataflowComponentReadServiceConfig
	writeDFC := runtime.DataflowComponentWriteServiceConfig

	// 6. Verify debug_level defaults to false in DataflowComponentServiceConfigs
	require.False(t, readDFC.DebugLevel)
	require.False(t, writeDFC.DebugLevel)

	// 7. Convert DataflowComponentServiceConfigs to BenthosServiceConfigs
	readBenthos := readDFC.GetBenthosServiceConfig()
	writeBenthos := writeDFC.GetBenthosServiceConfig()

	// 8. Verify LogLevel set to INFO (default) in BenthosServiceConfigs
	assert.Equal(t, "INFO", readBenthos.LogLevel)
	assert.Equal(t, "INFO", writeBenthos.LogLevel)

	// 9. Generate S6 configs using BenthosService
	benthosService := benthos.NewDefaultBenthosService("integration-test")

	readS6, err := benthosService.GenerateS6ConfigForBenthos(&readBenthos, "test-read")
	require.NoError(t, err)

	writeS6, err := benthosService.GenerateS6ConfigForBenthos(&writeBenthos, "test-write")
	require.NoError(t, err)

	// 10. Verify OPC_DEBUG environment variable NOT set in S6 configs
	_, readOpcDebugExists := readS6.Env["OPC_DEBUG"]
	_, writeOpcDebugExists := writeS6.Env["OPC_DEBUG"]
	assert.False(t, readOpcDebugExists, "OPC_DEBUG should not be set when LogLevel is INFO")
	assert.False(t, writeOpcDebugExists, "OPC_DEBUG should not be set when LogLevel is INFO")
}

func TestProtocolConverterDebugLevel_EndToEnd_VerifyIntegrationChain(t *testing.T) {
	// Additional test to verify the complete integration chain explicitly
	yamlData := `
config:
  debug_level: true
  connection:
    name: "integration-test"
    nmap:
      target: "127.0.0.1"
      port: "502"
  dataflowcomponent_read:
    benthos:
      input:
        generate:
          mapping: 'root = "integration"'
      output:
        drop: {}
  dataflowcomponent_write:
    benthos:
      input:
        drop: {}
      output:
        drop: {}
`

	// Parse YAML
	var spec protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec
	err := yaml.Unmarshal([]byte(yamlData), &spec)
	require.NoError(t, err)

	// Convert to Runtime
	runtime, err := protocolconverterserviceconfig.SpecToRuntime(spec)
	require.NoError(t, err)

	// Verify the complete chain for read DFC
	t.Run("ReadDataflowComponent", func(t *testing.T) {
		readDFC := runtime.DataflowComponentReadServiceConfig

		// Step 1: DebugLevel flag set
		assert.True(t, readDFC.DebugLevel, "DebugLevel should be true in DataflowComponentServiceConfig")

		// Step 2: GetBenthosServiceConfig() returns config with DEBUG LogLevel
		benthosConfig := readDFC.GetBenthosServiceConfig()
		assert.Equal(t, "DEBUG", benthosConfig.LogLevel, "LogLevel should be DEBUG in BenthosServiceConfig")

		// Step 3: GenerateS6ConfigForBenthos() sets OPC_DEBUG environment variable
		benthosService := benthos.NewDefaultBenthosService("integration-test")
		s6Config, err := benthosService.GenerateS6ConfigForBenthos(&benthosConfig, "read-test")
		require.NoError(t, err)
		assert.Equal(t, "true", s6Config.Env["OPC_DEBUG"], "OPC_DEBUG should be set to true in S6ServiceConfig")
	})

	// Verify the complete chain for write DFC
	t.Run("WriteDataflowComponent", func(t *testing.T) {
		writeDFC := runtime.DataflowComponentWriteServiceConfig

		// Step 1: DebugLevel flag set
		assert.True(t, writeDFC.DebugLevel, "DebugLevel should be true in DataflowComponentServiceConfig")

		// Step 2: GetBenthosServiceConfig() returns config with DEBUG LogLevel
		benthosConfig := writeDFC.GetBenthosServiceConfig()
		assert.Equal(t, "DEBUG", benthosConfig.LogLevel, "LogLevel should be DEBUG in BenthosServiceConfig")

		// Step 3: GenerateS6ConfigForBenthos() sets OPC_DEBUG environment variable
		benthosService := benthos.NewDefaultBenthosService("integration-test")
		s6Config, err := benthosService.GenerateS6ConfigForBenthos(&benthosConfig, "write-test")
		require.NoError(t, err)
		assert.Equal(t, "true", s6Config.Env["OPC_DEBUG"], "OPC_DEBUG should be set to true in S6ServiceConfig")
	})
}

func TestProtocolConverterDebugLevel_EndToEnd_LogLevelMapping(t *testing.T) {
	// Test explicit mapping between debug_level boolean and BenthosServiceConfig LogLevel string
	testCases := []struct {
		name               string
		debugLevel         bool
		expectedLogLevel   string
		expectOpcDebugSet  bool
		expectedOpcDebug   string
	}{
		{
			name:              "debug_level true maps to LogLevel DEBUG",
			debugLevel:        true,
			expectedLogLevel:  "DEBUG",
			expectOpcDebugSet: true,
			expectedOpcDebug:  "true",
		},
		{
			name:              "debug_level false maps to LogLevel INFO",
			debugLevel:        false,
			expectedLogLevel:  "INFO",
			expectOpcDebugSet: false,
			expectedOpcDebug:  "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			yamlData := `
config:
  connection:
    name: "test-connection"
    nmap:
      target: "127.0.0.1"
      port: "502"
  dataflowcomponent_read:
    benthos:
      input:
        generate:
          mapping: 'root = "test"'
      output:
        drop: {}
  dataflowcomponent_write:
    benthos:
      input:
        drop: {}
      output:
        drop: {}
`
			// Parse base YAML
			var spec protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec
			err := yaml.Unmarshal([]byte(yamlData), &spec)
			require.NoError(t, err)

			// Set debug_level programmatically
			spec.Config.DebugLevel = tc.debugLevel

			// Convert to Runtime
			runtime, err := protocolconverterserviceconfig.SpecToRuntime(spec)
			require.NoError(t, err)

			// Test read DFC
			readBenthos := runtime.DataflowComponentReadServiceConfig.GetBenthosServiceConfig()
			assert.Equal(t, tc.expectedLogLevel, readBenthos.LogLevel)

			benthosService := benthos.NewDefaultBenthosService("integration-test")
			readS6, err := benthosService.GenerateS6ConfigForBenthos(&readBenthos, "test-read")
			require.NoError(t, err)

			opcDebugValue, opcDebugExists := readS6.Env["OPC_DEBUG"]
			assert.Equal(t, tc.expectOpcDebugSet, opcDebugExists, "OPC_DEBUG existence should match expected")
			if tc.expectOpcDebugSet {
				assert.Equal(t, tc.expectedOpcDebug, opcDebugValue, "OPC_DEBUG value should match expected")
			}

			// Test write DFC
			writeBenthos := runtime.DataflowComponentWriteServiceConfig.GetBenthosServiceConfig()
			assert.Equal(t, tc.expectedLogLevel, writeBenthos.LogLevel)

			writeS6, err := benthosService.GenerateS6ConfigForBenthos(&writeBenthos, "test-write")
			require.NoError(t, err)

			opcDebugValue, opcDebugExists = writeS6.Env["OPC_DEBUG"]
			assert.Equal(t, tc.expectOpcDebugSet, opcDebugExists, "OPC_DEBUG existence should match expected")
			if tc.expectOpcDebugSet {
				assert.Equal(t, tc.expectedOpcDebug, opcDebugValue, "OPC_DEBUG value should match expected")
			}
		})
	}
}
