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

package redpandaserviceconfig

// Normalizer handles the normalization of Redpanda configurations
type Normalizer struct{}

// NewNormalizer creates a new configuration normalizer for Redpanda
func NewNormalizer() *Normalizer {
	return &Normalizer{}
}

// NormalizeConfig applies Redpanda defaults to a structured config
func (n *Normalizer) NormalizeConfig(cfg RedpandaServiceConfig) RedpandaServiceConfig {
	// Create a copy
	normalized := cfg

	if normalized.Topic.DefaultTopicRetentionMs == 0 {
		normalized.Topic.DefaultTopicRetentionMs = 604800000 // Redpanda by default sets this to 7 days when set to 0
	}

	if normalized.Topic.DefaultTopicRetentionBytes == 0 {
		normalized.Topic.DefaultTopicRetentionBytes = 0
	}

	if normalized.Topic.DefaultTopicCompressionType == "" {
		normalized.Topic.DefaultTopicCompressionType = "snappy"
	}

	if normalized.Resources.MaxCores == 0 {
		normalized.Resources.MaxCores = 1
	}

	if normalized.Resources.MemoryPerCoreInBytes == 0 {
		normalized.Resources.MemoryPerCoreInBytes = 2147483648 // 2GB
	}

	return normalized
}
