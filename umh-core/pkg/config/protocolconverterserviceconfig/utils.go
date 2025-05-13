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

// GetConnectionServiceConfig converts the component config to a full ConnectionServiceConfig
func (c *ProtocolConverterServiceConfig) GetConnectionServiceConfig() connectionserviceconfig.ConnectionServiceConfig {
	return c.ConnectionServiceConfig
}

// GetDFCServiceConfig converts the component config to a full ConnectionServiceConfig
func (c *ProtocolConverterServiceConfig) GetDFCServiceConfig() dataflowcomponentserviceconfig.DataflowComponentServiceConfig {
	return c.DataflowComponentServiceConfig
}

// FromConnectionAndDFCServiceConfig creates a ProtocolConverterServiceConfig
// from a ConnectionServiceConfig and DataFlowComponentConfig
func FromConnectionAndDFCServiceConfig(
	connection connectionserviceconfig.ConnectionServiceConfig,
	dfc dataflowcomponentserviceconfig.DataflowComponentServiceConfig,
) ProtocolConverterServiceConfig {
	return ProtocolConverterServiceConfig{
		ConnectionServiceConfig:        connection,
		DataflowComponentServiceConfig: dfc,
	}
}
