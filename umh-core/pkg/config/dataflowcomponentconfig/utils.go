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

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
)

// CompareConfigs compares two DataFlowComponentConfigs for equality
// by converting to BenthosServiceConfig and using the existing comparison utilities
func CompareConfigs(a, b *DataFlowComponentConfig) bool {
	benthosA := a.GetBenthosServiceConfig()
	benthosB := b.GetBenthosServiceConfig()

	comparator := benthosserviceconfig.NewComparator()
	return comparator.ConfigsEqual(benthosA, benthosB)
}

// GetConfigDiff returns a human-readable string describing differences between configs
func GetConfigDiff(a, b *DataFlowComponentConfig) string {
	benthosA := a.GetBenthosServiceConfig()
	benthosB := b.GetBenthosServiceConfig()

	comparator := benthosserviceconfig.NewComparator()
	return comparator.ConfigDiff(benthosA, benthosB)
}

// GenerateYAML generates YAML configuration for a DataFlowComponentConfig
// using the existing benthos config generator
func GenerateYAML(config *DataFlowComponentConfig) ([]byte, error) {
	benthosConfig := config.GetBenthosServiceConfig()

	generator := benthosserviceconfig.NewGenerator()
	yaml, err := generator.RenderConfig(benthosConfig)
	if err != nil {
		return nil, err
	}
	return []byte(yaml), nil
}

// NormalizeConfig applies normalization to a DataFlowComponentConfig
// by leveraging the existing normalizer for BenthosServiceConfig
func NormalizeConfig(config *DataFlowComponentConfig) {
	// Convert to BenthosServiceConfig
	benthosConfig := config.GetBenthosServiceConfig()

	// Normalize
	normalizer := benthosserviceconfig.NewNormalizer()
	normalized := normalizer.NormalizeConfig(benthosConfig)

	// Update the simplified config with normalized values
	config.BenthosConfig = FromBenthosServiceConfig(normalized)
}
