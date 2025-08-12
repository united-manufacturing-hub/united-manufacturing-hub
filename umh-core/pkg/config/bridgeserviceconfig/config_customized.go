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

package bridgeserviceconfig

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
)

// GetConnectionConfig returns the template form of the connection config.
// This is used during rendering to access the template that may contain variables like {{ .PORT }}.
// The template will be rendered into a runtime config with proper types during BuildRuntimeConfig.
func (c ConfigSpec) GetConnectionConfig() connectionserviceconfig.ConnectionServiceConfigTemplate {
	return c.Config.ConnectionConfig
}

// GetDFCReadConfig converts the component config to a full ProtocolConverterServiceConfig
// For a read DFC, the user is not allowed to set its own output config, so we "enforce" the output config
// to be the UNS output config. This ensures protocol converters always write to the unified namespace.
func (c ConfigSpec) GetDFCReadConfig() dataflowcomponentserviceconfig.DataflowComponentServiceConfig {
	// copy the config
	dfcReadConfig := c.Config.DFCReadConfig

	// Only append UNS output if there's an input config
	if len(dfcReadConfig.BenthosConfig.Input) > 0 {
		dfcReadConfig.BenthosConfig.Output = map[string]any{
			"uns": map[string]any{
				"bridged_by": "{{ .internal.bridged_by }}",
			},
		}
	}

	return dfcReadConfig
}

// GetDFCWriteConfig converts the component config to a full ProtocolConverterServiceConfig
// For a write DFC, the user is not allowed to set its own input config, so we "enforce" the input config
// to be the UNS input config. This ensures protocol converters always read from the unified namespace.
func (c ConfigSpec) GetDFCWriteConfig() dataflowcomponentserviceconfig.DataflowComponentServiceConfig {
	dfcWriteConfig := c.Config.DFCWriteConfig

	// Only append UNS input if there's an output config
	if len(dfcWriteConfig.BenthosConfig.Output) > 0 {
		dfcWriteConfig.BenthosConfig.Input = map[string]any{
			"uns": map[string]any{
				"consumer_group": "{{ .internal.bridged_by }}", // use bridged_by as consumer group
				"umh_topic":      "{{ .internal.umh_topic }}",  // this needs to come from some value set by the user
			},
		}
	}
	return dfcWriteConfig
}

// FromConfigs creates a Config
// from a ConnectionConfig and DataFlowComponentConfig
func FromConfigs(
	connection connectionserviceconfig.ConnectionServiceConfig,
	dfcRead dataflowcomponentserviceconfig.DataflowComponentServiceConfig,
	dfcWrite dataflowcomponentserviceconfig.DataflowComponentServiceConfig,
) ConfigRuntime {
	return ConfigRuntime{
		ConnectionConfig: connection,
		DFCReadConfig:    dfcRead,
		DFCWriteConfig:   dfcWrite,
	}
}
