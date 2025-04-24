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

package benthosserviceconfig

// Normalizer handles the normalization of Benthos configurations
type Normalizer struct{}

// NewNormalizer creates a new configuration normalizer for Benthos
func NewNormalizer() *Normalizer {
	return &Normalizer{}
}

// NormalizeConfig applies Benthos defaults to a structured config
func (n *Normalizer) NormalizeConfig(cfg BenthosServiceConfig) BenthosServiceConfig {
	// Create a copy
	normalized := cfg

	// Ensure Input map exists
	if normalized.Input == nil {
		normalized.Input = make(map[string]interface{})
	}

	// Ensure Output map exists
	if normalized.Output == nil {
		normalized.Output = make(map[string]interface{})
	}

	// Ensure Pipeline map exists with processors
	if normalized.Pipeline == nil {
		normalized.Pipeline = map[string]interface{}{
			"processors": []interface{}{},
		}
	} else if _, exists := normalized.Pipeline["processors"]; !exists {
		normalized.Pipeline["processors"] = []interface{}{}
	}

	// Set default buffer if missing
	if len(normalized.Buffer) == 0 {
		normalized.Buffer = map[string]interface{}{
			"none": map[string]interface{}{},
		}
	}

	// Set default metrics port if not specified
	if normalized.MetricsPort == 0 {
		normalized.MetricsPort = 4195
	}

	// Set default log level if not specified
	if normalized.LogLevel == "" {
		normalized.LogLevel = "INFO"
	}

	return normalized
}
