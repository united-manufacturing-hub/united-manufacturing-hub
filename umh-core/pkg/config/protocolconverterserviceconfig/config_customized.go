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

// GetConnectionServiceConfig converts the component config to a full ProtocolConverterServiceConfig
// no customization needed
func (c *ProtocolConverterServiceConfigSpec) GetConnectionServiceConfig() connectionserviceconfig.ConnectionServiceConfig {
	return c.Template.ConnectionServiceConfig
}

// GetDFCReadServiceConfig converts the component config to a full ProtocolConverterServiceConfig
// For a read DFC, the user is not allowed to set its own output config, so we "enforce" the output config
// to be the uns output config.
func (c *ProtocolConverterServiceConfigSpec) GetDFCReadServiceConfig() dataflowcomponentserviceconfig.DataflowComponentServiceConfig {
	// copy the config
	dfcReadConfig := c.Template.DataflowComponentReadServiceConfig

	// enforce the output config to be the uns output config
	dfcReadConfig.BenthosConfig.Output = map[string]any{
		"uns": map[string]any{
			"bridged_by": "{{ .bridged_by }}",
		},
	}

	return dfcReadConfig
}

// GetDFCWriteServiceConfig converts the component config to a full ProtocolConverterServiceConfig
// For a write DFC, the user is not allowed to set its own input config, so we "enforce" the input config
// to be the uns input config.
func (c *ProtocolConverterServiceConfigSpec) GetDFCWriteServiceConfig() dataflowcomponentserviceconfig.DataflowComponentServiceConfig {
	dfcWriteConfig := c.Template.DataflowComponentWriteServiceConfig

	dfcWriteConfig.BenthosConfig.Input = map[string]any{
		"uns": map[string]any{
			"consumer_group": "{{ .consumer_group }}",
			"umh_topic":      "{{ .umh_topic }}",
		},
	}
	return dfcWriteConfig
}

// FromConnectionAndDFCServiceConfig creates a ProtocolConverterServiceConfig
// from a ConnectionServiceConfig and DataFlowComponentConfig
func FromConnectionAndDFCServiceConfig(
	connection connectionserviceconfig.ConnectionServiceConfig,
	dfcRead dataflowcomponentserviceconfig.DataflowComponentServiceConfig,
	dfcWrite dataflowcomponentserviceconfig.DataflowComponentServiceConfig,
) ProtocolConverterServiceConfigRuntime {
	return ProtocolConverterServiceConfigRuntime{
		ConnectionServiceConfig:             connection,
		DataflowComponentReadServiceConfig:  dfcRead,
		DataflowComponentWriteServiceConfig: dfcWrite,
	}
}
