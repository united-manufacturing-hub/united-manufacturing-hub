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

package streamprocessorserviceconfig

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
)

// GetDFCReadServiceConfig converts the component config to a full ProtocolConverterServiceConfig
// For a read DFC, the user is not allowed to set its own output config, so we "enforce" the output config
// to be the UNS output config. This ensures protocol converters always write to the unified namespace.
func (c ConfigSpec) GetDFCConfig() dataflowcomponentserviceconfig.DataflowComponentServiceConfig {
	// copy the config
	dfcReadConfig := c.Config.DFCConfig

	// Only append UNS output if there's an input config
	if len(dfcReadConfig.BenthosConfig.Input) > 0 {
		dfcReadConfig.BenthosConfig.Output = map[string]any{
			"uns": map[string]any{},
		}
	}

	return dfcReadConfig
}

// FromConfigs creates a StremProcessorConfig
func FromConfigs(
	dfc dataflowcomponentserviceconfig.DataflowComponentServiceConfig,
) RuntimeConfig {
	return RuntimeConfig{
		DFCConfig: dfc,
	}
}
