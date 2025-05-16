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

var (
	defaultGenerator  = NewGenerator()
	defaultNormalizer = NewNormalizer()
	defaultComparator = NewComparator()
)

type ProtocolConverterServiceConfigTemplateVariable struct {
	ConnectionServiceConfig             connectionserviceconfig.ConnectionServiceConfig               `yaml:"connection"`
	DataflowComponentReadServiceConfig  dataflowcomponentserviceconfig.DataflowComponentServiceConfig `yaml:"dataflowcomponent_read"`
	DataflowComponentWriteServiceConfig dataflowcomponentserviceconfig.DataflowComponentServiceConfig `yaml:"dataflowcomponent_write"`
}

// ProtocolConverterServiceConfig represents the configuration for a ProtocolConverter
type ProtocolConverterServiceConfig struct {
	Template  ProtocolConverterServiceConfigTemplateVariable `yaml:"template"`
	Variables variables.VariableBundle                       `yaml:"variables"`
}

// Equal checks if two ProtocolConverterServiceConfigs are equal
func (c ProtocolConverterServiceConfig) Equal(other ProtocolConverterServiceConfig) bool {
	return NewComparator().ConfigsEqual(c, other)
}

// NormalizeProtocolConverterConfig is a package-level function for easy config normalization
func NormalizeProtocolConverterConfig(cfg ProtocolConverterServiceConfig) ProtocolConverterServiceConfig {
	return defaultNormalizer.NormalizeConfig(cfg)
}

// ConfigsEqual is a package-level function for easy config comparison
func ConfigsEqual(desired, observed ProtocolConverterServiceConfig) bool {
	return defaultComparator.ConfigsEqual(desired, observed)
}

// ConfigDiff is a package-level function for easy config diff generation
func ConfigDiff(desired, observed ProtocolConverterServiceConfig) string {
	return defaultComparator.ConfigDiff(desired, observed)
}
