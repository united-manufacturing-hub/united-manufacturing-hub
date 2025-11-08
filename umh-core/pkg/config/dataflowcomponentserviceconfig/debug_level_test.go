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

package dataflowcomponentserviceconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestDataflowComponentServiceConfig_ParseYAML_DebugLevelTrue(t *testing.T) {
	yamlData := `
debug_level: true
benthos:
  input:
    generate:
      mapping: 'root = ""'
  output:
    stdout: {}
`

	var config DataflowComponentServiceConfig

	err := yaml.Unmarshal([]byte(yamlData), &config)
	require.NoError(t, err)

	assert.True(t, config.DebugLevel)
}

func TestDataflowComponentServiceConfig_ParseYAML_DebugLevelFalse(t *testing.T) {
	yamlData := `
debug_level: false
benthos:
  input:
    generate:
      mapping: 'root = ""'
  output:
    stdout: {}
`

	var config DataflowComponentServiceConfig

	err := yaml.Unmarshal([]byte(yamlData), &config)
	require.NoError(t, err)

	assert.False(t, config.DebugLevel)
}

func TestDataflowComponentServiceConfig_ParseYAML_DebugLevelOmitted(t *testing.T) {
	yamlData := `
benthos:
  input:
    generate:
      mapping: 'root = ""'
  output:
    stdout: {}
`

	var config DataflowComponentServiceConfig

	err := yaml.Unmarshal([]byte(yamlData), &config)
	require.NoError(t, err)

	assert.False(t, config.DebugLevel)
}

func TestDataflowComponentServiceConfig_GetBenthosServiceConfig_DebugLevelTrue(t *testing.T) {
	cfg := DataflowComponentServiceConfig{
		DebugLevel: true,
		BenthosConfig: BenthosConfig{
			Input: map[string]any{
				"generate": map[string]any{
					"mapping": `root = ""`,
				},
			},
			Output: map[string]any{
				"stdout": map[string]any{},
			},
		},
	}

	benthosConfig := cfg.GetBenthosServiceConfig()
	assert.Equal(t, "DEBUG", benthosConfig.LogLevel)
}

func TestDataflowComponentServiceConfig_GetBenthosServiceConfig_DebugLevelFalse(t *testing.T) {
	cfg := DataflowComponentServiceConfig{
		DebugLevel: false,
		BenthosConfig: BenthosConfig{
			Input: map[string]any{
				"generate": map[string]any{
					"mapping": `root = ""`,
				},
			},
			Output: map[string]any{
				"stdout": map[string]any{},
			},
		},
	}

	benthosConfig := cfg.GetBenthosServiceConfig()
	assert.Equal(t, "INFO", benthosConfig.LogLevel)
}
