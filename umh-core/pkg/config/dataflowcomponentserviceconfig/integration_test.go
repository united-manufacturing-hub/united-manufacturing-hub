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

package dataflowcomponentserviceconfig_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
)

func TestDataFlowDebugLevel_EndToEnd_DebugEnabled(t *testing.T) {
	// Test standalone data flow with debug_level: true
	dfc := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
		DebugLevel: true,
		BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
			Input: map[string]interface{}{
				"generate": map[string]interface{}{
					"mapping": `root = "test"`,
				},
			},
			Output: map[string]interface{}{
				"drop": map[string]interface{}{},
			},
		},
	}

	// 1. Verify DebugLevel flag set
	assert.True(t, dfc.DebugLevel)

	// 2. Convert to BenthosServiceConfig
	benthosConfig := dfc.GetBenthosServiceConfig()

	// 3. Verify LogLevel set to DEBUG
	assert.Equal(t, "DEBUG", benthosConfig.LogLevel)

	// 4. Generate S6 config
	benthosService := benthos.NewDefaultBenthosService("integration-test")
	s6Config, err := benthosService.GenerateS6ConfigForBenthos(&benthosConfig, "test-dataflow")
	require.NoError(t, err)

	// 5. Verify OPC_DEBUG environment variable set
	assert.Equal(t, "true", s6Config.Env["OPC_DEBUG"])
}

func TestDataFlowDebugLevel_EndToEnd_DebugDisabled(t *testing.T) {
	// Test standalone data flow with debug_level: false
	dfc := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
		DebugLevel: false,
		BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
			Input: map[string]interface{}{
				"generate": map[string]interface{}{
					"mapping": `root = "test"`,
				},
			},
			Output: map[string]interface{}{
				"drop": map[string]interface{}{},
			},
		},
	}

	// 1. Verify DebugLevel flag set to false
	assert.False(t, dfc.DebugLevel)

	// 2. Convert to BenthosServiceConfig
	benthosConfig := dfc.GetBenthosServiceConfig()

	// 3. Verify LogLevel set to INFO (default)
	assert.Equal(t, "INFO", benthosConfig.LogLevel)

	// 4. Generate S6 config
	benthosService := benthos.NewDefaultBenthosService("integration-test")
	s6Config, err := benthosService.GenerateS6ConfigForBenthos(&benthosConfig, "test-dataflow")
	require.NoError(t, err)

	// 5. Verify OPC_DEBUG environment variable NOT set
	_, opcDebugExists := s6Config.Env["OPC_DEBUG"]
	assert.False(t, opcDebugExists, "OPC_DEBUG should not be set when LogLevel is INFO")
}

func TestDataFlowDebugLevel_EndToEnd_DebugOmitted(t *testing.T) {
	// Test standalone data flow without debug_level field (should default to false/INFO)
	dfc := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
		BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
			Input: map[string]interface{}{
				"generate": map[string]interface{}{
					"mapping": `root = "test"`,
				},
			},
			Output: map[string]interface{}{
				"drop": map[string]interface{}{},
			},
		},
	}

	// 1. Verify DebugLevel defaults to false
	assert.False(t, dfc.DebugLevel)

	// 2. Convert to BenthosServiceConfig
	benthosConfig := dfc.GetBenthosServiceConfig()

	// 3. Verify LogLevel set to INFO (default)
	assert.Equal(t, "INFO", benthosConfig.LogLevel)

	// 4. Generate S6 config
	benthosService := benthos.NewDefaultBenthosService("integration-test")
	s6Config, err := benthosService.GenerateS6ConfigForBenthos(&benthosConfig, "test-dataflow")
	require.NoError(t, err)

	// 5. Verify OPC_DEBUG environment variable NOT set
	_, opcDebugExists := s6Config.Env["OPC_DEBUG"]
	assert.False(t, opcDebugExists, "OPC_DEBUG should not be set when LogLevel is INFO")
}

func TestDataFlowDebugLevel_EndToEnd_LogLevelMapping(t *testing.T) {
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
			// Create data flow component with programmatic debug_level
			dfc := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
				DebugLevel: tc.debugLevel,
				BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
					Input: map[string]interface{}{
						"generate": map[string]interface{}{
							"mapping": `root = "test"`,
						},
					},
					Output: map[string]interface{}{
						"drop": map[string]interface{}{},
					},
				},
			}

			// Convert to BenthosServiceConfig
			benthosConfig := dfc.GetBenthosServiceConfig()
			assert.Equal(t, tc.expectedLogLevel, benthosConfig.LogLevel)

			// Generate S6 config
			benthosService := benthos.NewDefaultBenthosService("integration-test")
			s6Config, err := benthosService.GenerateS6ConfigForBenthos(&benthosConfig, "test-dataflow")
			require.NoError(t, err)

			// Verify OPC_DEBUG environment variable
			opcDebugValue, opcDebugExists := s6Config.Env["OPC_DEBUG"]
			assert.Equal(t, tc.expectOpcDebugSet, opcDebugExists, "OPC_DEBUG existence should match expected")

			if tc.expectOpcDebugSet {
				assert.Equal(t, tc.expectedOpcDebug, opcDebugValue, "OPC_DEBUG value should match expected")
			}
		})
	}
}
