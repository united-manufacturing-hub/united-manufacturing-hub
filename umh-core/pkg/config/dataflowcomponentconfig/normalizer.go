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

package dataflowcomponentconfig

import "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"

// Normalizer handles the normalization of DataflowComponent configurations
// by leveraging benthosserviceconfig.Normalizer
type Normalizer struct {
	benthosNormalizer *benthosserviceconfig.Normalizer
}

// NewNormalizer creates a new configuration normalizer for DataflowComponent
func NewNormalizer() *Normalizer {
	return &Normalizer{
		benthosNormalizer: benthosserviceconfig.NewNormalizer(),
	}
}

// NormalizeConfig applies DataflowComponent defaults to a structured config
// by converting to BenthosServiceConfig, normalizing, and converting back
func (n *Normalizer) NormalizeConfig(cfg DataFlowComponentConfig) DataFlowComponentConfig {
	// Convert to BenthosServiceConfig
	benthosCfg := cfg.GetBenthosServiceConfig()

	// Use BenthosServiceConfig normalizer
	normalizedBenthos := n.benthosNormalizer.NormalizeConfig(benthosCfg)

	// Convert back to DataFlowComponentConfig
	return FromBenthosServiceConfig(normalizedBenthos)
}
