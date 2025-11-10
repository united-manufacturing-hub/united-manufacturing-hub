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

import "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"

// Normalizer handles the normalization of Benthos configurations.
type Normalizer struct{}

// NewNormalizer creates a new configuration normalizer for Benthos.
func NewNormalizer() *Normalizer {
	return &Normalizer{}
}

// NormalizeConfig applies DFC defaults to a structured config.
func (n *Normalizer) NormalizeConfig(cfg DataflowComponentServiceConfig) DataflowComponentServiceConfig {
	// create a shallow copy
	normalized := cfg

	// We need to first normalize the underlying BenthosServiceConfig
	normalizer := benthosserviceconfig.NewNormalizer()
	normalizedBenthosConfig := normalizer.NormalizeConfig(normalized.GetBenthosServiceConfig())
	// Then we need to put the normalizedBenthosConfig into the DataFlowComponentConfig
	// Currently the BenthosConfig is the only underlying component of the DFCConfig
	normalized.BenthosConfig = FromBenthosServiceConfig(normalizedBenthosConfig).BenthosConfig

	// Preserve DebugLevel from the original input (lost during FromBenthosServiceConfig conversion)
	normalized.DebugLevel = cfg.DebugLevel

	return normalized
}
