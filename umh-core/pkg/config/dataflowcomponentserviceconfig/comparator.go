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

// Comparator handles the comparison of DFC configurations
type Comparator struct {
	normalizer *Normalizer
}

// NewComparator creates a new configuration comparator for DFC's
func NewComparator() *Comparator {
	return &Comparator{
		normalizer: NewNormalizer(),
	}
}

// ConfigsEqual compares two DataFlowComponentConfigs for equality
// by converting to BenthosServiceConfig and using the existing comparison utilities
func (c *Comparator) ConfigsEqual(a, b *DataflowComponentServiceConfig) bool {
	if a == nil || b == nil {
		return false
	}
	benthosA := a.GetBenthosServiceConfig()
	benthosB := b.GetBenthosServiceConfig()

	comparator := benthosserviceconfig.NewComparator()
	return comparator.ConfigsEqual(benthosA, benthosB)
}

// ConfigDiff returns a human-readable string describing differences between configs
func (c *Comparator) ConfigDiff(a, b *DataflowComponentServiceConfig) string {
	if a == nil || b == nil {
		return "one or both configurations are nil"
	}
	benthosA := a.GetBenthosServiceConfig()
	benthosB := b.GetBenthosServiceConfig()

	comparator := benthosserviceconfig.NewComparator()
	return comparator.ConfigDiff(benthosA, benthosB)
}
