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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
)

// GetConnectionServiceConfig returns the template form of the connection config.
// This is used during rendering to access the template that may contain variables like {{ .PORT }}.
// The template will be rendered into a runtime config with proper types during BuildRuntimeConfig.
func (c ProtocolConverterServiceConfigSpec) GetConnectionServiceConfig() connectionserviceconfig.ConnectionServiceConfigTemplate {
	return c.Config.ConnectionServiceConfig
}

// GetDFCReadServiceConfig converts the component config to a full ProtocolConverterServiceConfig
// For a read DFC, the user is not allowed to set its own output config, so we "enforce" the output config
// to be the UNS output config. This ensures protocol converters always write to the unified namespace.
func (c ProtocolConverterServiceConfigSpec) GetDFCReadServiceConfig() dataflowcomponentserviceconfig.DataflowComponentServiceConfig {
	// copy the config
	dfcReadConfig := c.Config.DataflowComponentReadServiceConfig

	// Propagate debug_level: DFC-level OR Spec-level (any true enables debug)
	if !dfcReadConfig.DebugLevel {
		dfcReadConfig.DebugLevel = c.DebugLevel
	}

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

// GetDFCWriteServiceConfig converts the component config to a full ProtocolConverterServiceConfig
// For a write DFC, the user is not allowed to set its own input config, so we "enforce" the input config
// to be the UNS input config. This ensures protocol converters always read from the unified namespace.
func (c ProtocolConverterServiceConfigSpec) GetDFCWriteServiceConfig() dataflowcomponentserviceconfig.DataflowComponentServiceConfig {
	dfcWriteConfig := c.Config.DataflowComponentWriteServiceConfig

	// Propagate debug_level: DFC-level OR Spec-level (any true enables debug)
	if !dfcWriteConfig.DebugLevel {
		dfcWriteConfig.DebugLevel = c.DebugLevel
	}

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

// FromConnectionAndDFCServiceConfig creates a ProtocolConverterServiceConfig
// from a ConnectionServiceConfig and DataFlowComponentConfig.
func FromConnectionAndDFCServiceConfig(
	connection connectionserviceconfig.ConnectionServiceConfig,
	dfcRead dataflowcomponentserviceconfig.DataflowComponentServiceConfig,
	dfcWrite dataflowcomponentserviceconfig.DataflowComponentServiceConfig,
) ProtocolConverterServiceConfigRuntime {
	return ProtocolConverterServiceConfigRuntime{
		ConnectionServiceConfig:             connection,
		DataflowComponentReadServiceConfig:  dfcRead,
		DataflowComponentWriteServiceConfig: dfcWrite,
		DebugLevel:                          dfcRead.DebugLevel,
	}
}
