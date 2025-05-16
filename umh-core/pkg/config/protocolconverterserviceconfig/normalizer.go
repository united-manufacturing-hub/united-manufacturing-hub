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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/variables"
)

// Normalizer handles the normalization of ProtocolConverter configurations
type Normalizer struct{}

// NewNormalizer creates a new configuration normalizer for ProtocolConverter
func NewNormalizer() *Normalizer {
	return &Normalizer{}
}

// NormalizeConfig applies ProtocolConverter defaults to a structured config
func (n *Normalizer) NormalizeConfig(cfg ProtocolConverterServiceConfigSpec) ProtocolConverterServiceConfigSpec {

	// create a copy
	normalized := cfg

	// We need to first normalize the underlying DFCServiceConfig
	dfcNormalizer := dataflowcomponentserviceconfig.NewNormalizer()
	normalized.Template.DataflowComponentReadServiceConfig = dfcNormalizer.NormalizeConfig(normalized.GetDFCReadServiceConfig())
	normalized.Template.DataflowComponentWriteServiceConfig = dfcNormalizer.NormalizeConfig(normalized.GetDFCWriteServiceConfig())

	// Then we  need to normalize the underlying ConnectionServiceConfig
	connectionNormalizer := connectionserviceconfig.NewNormalizer()
	normalized.Template.ConnectionServiceConfig = connectionNormalizer.NormalizeConfig(normalized.GetConnectionServiceConfig())

	// Then we need to normalize the variables
	variablesNormalizer := variables.NewNormalizer()
	normalized.Variables = variablesNormalizer.NormalizeConfig(normalized.Variables)

	// no need to normalize the location

	return normalized
}
