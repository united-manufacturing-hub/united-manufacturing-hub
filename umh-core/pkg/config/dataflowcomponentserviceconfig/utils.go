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
	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
)

// GetBenthosServiceConfig converts the component config to a full BenthosServiceConfig
func (c DataflowComponentServiceConfig) GetBenthosServiceConfig() benthosserviceconfig.BenthosServiceConfig {
	return c.BenthosConfig.ToBenthosServiceConfig()
}

// ToBenthosServiceConfig converts the simplified BenthosConfig to a full BenthosServiceConfig
// with default advanced configuration
func (bc BenthosConfig) ToBenthosServiceConfig() benthosserviceconfig.BenthosServiceConfig {
	return benthosserviceconfig.BenthosServiceConfig{
		Input:              bc.Input,
		Pipeline:           bc.Pipeline,
		Output:             bc.Output,
		CacheResources:     bc.CacheResources,
		RateLimitResources: bc.RateLimitResources,
		Buffer:             bc.Buffer,
		Logger:             bc.Logger,
		// Default values for advanced configuration
		MetricsPort: 0, // Will be assigned dynamically by the port manager
		LogLevel:    constants.DefaultBenthosLogLevel,
	}
}

// FromBenthosServiceConfig creates a DataFlowComponentConfig from a BenthosServiceConfig,
// ignoring advanced configuration fields
func FromBenthosServiceConfig(benthos benthosserviceconfig.BenthosServiceConfig) DataflowComponentServiceConfig {
	return DataflowComponentServiceConfig{
		BenthosConfig: BenthosConfig{
			Input:              benthos.Input,
			Pipeline:           benthos.Pipeline,
			Output:             benthos.Output,
			CacheResources:     benthos.CacheResources,
			RateLimitResources: benthos.RateLimitResources,
			Buffer:             benthos.Buffer,
			Logger:             benthos.Logger,
		},
	}
}

// generate the uuid from the name
func GenerateUUIDFromName(name string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceDNS, []byte(name))
}
