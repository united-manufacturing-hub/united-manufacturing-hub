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

package protocolconverterserviceconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestProtocolConverterServiceConfigSpec_ParseYAML_DebugLevelTrue(t *testing.T) {
	yamlData := `
config:
  debug_level: true
  connection:
    name: "test-connection"
  dataflowcomponent_read:
    benthos:
      input:
        generate:
          mapping: 'root = ""'
      output:
        stdout: {}
  dataflowcomponent_write:
    benthos:
      input:
        generate:
          mapping: 'root = ""'
      output:
        stdout: {}
`

	var spec ProtocolConverterServiceConfigSpec

	err := yaml.Unmarshal([]byte(yamlData), &spec)
	require.NoError(t, err)

	assert.True(t, spec.Config.DebugLevel)
}

func TestProtocolConverterServiceConfigSpec_ParseYAML_DebugLevelFalse(t *testing.T) {
	yamlData := `
config:
  debug_level: false
  connection:
    name: "test-connection"
  dataflowcomponent_read:
    benthos:
      input:
        generate:
          mapping: 'root = ""'
      output:
        stdout: {}
  dataflowcomponent_write:
    benthos:
      input:
        generate:
          mapping: 'root = ""'
      output:
        stdout: {}
`

	var spec ProtocolConverterServiceConfigSpec

	err := yaml.Unmarshal([]byte(yamlData), &spec)
	require.NoError(t, err)

	assert.False(t, spec.Config.DebugLevel)
}

func TestProtocolConverterServiceConfigSpec_ParseYAML_DebugLevelOmitted(t *testing.T) {
	yamlData := `
config:
  connection:
    name: "test-connection"
  dataflowcomponent_read:
    benthos:
      input:
        generate:
          mapping: 'root = ""'
      output:
        stdout: {}
  dataflowcomponent_write:
    benthos:
      input:
        generate:
          mapping: 'root = ""'
      output:
        stdout: {}
`

	var spec ProtocolConverterServiceConfigSpec

	err := yaml.Unmarshal([]byte(yamlData), &spec)
	require.NoError(t, err)

	assert.False(t, spec.Config.DebugLevel)
}
