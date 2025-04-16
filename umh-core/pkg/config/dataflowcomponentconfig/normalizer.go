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

// Normalizer handles the normalization of Dataflowcomponent configurations
type Normalizer struct{}

// NewNormalizer creates a new configuration normalizer for DataflowComponent
func NewNormalizer() *Normalizer {
	return &Normalizer{}
}

// NormalizeConfig applies DataflowComponent defaults to a structured config
func (n *Normalizer) NormalizeConfig(cfg DataFlowComponentConfig) DataFlowComponentConfig {
	// Right now dataflowcomponentConfig only has the BenthosConfig. In future
	// if a new field is added to dataflowComponentConfig, it should be normalizedBenthos by this method
	normalized := cfg
	normalizedBenthos := cfg.BenthosConfig

	// Ensure Input map exists
	if normalizedBenthos.Input == nil {
		normalizedBenthos.Input = make(map[string]interface{})
	}

	// Ensure Output map exists
	if normalizedBenthos.Output == nil {
		normalizedBenthos.Output = make(map[string]interface{})
	}

	// Ensure Pipeline map exists with processors
	if normalizedBenthos.Pipeline == nil {
		normalizedBenthos.Pipeline = map[string]interface{}{
			"processors": []interface{}{},
		}
	} else if _, exists := normalizedBenthos.Pipeline["processors"]; !exists {
		normalizedBenthos.Pipeline["processors"] = []interface{}{}
	}

	// Set default buffer if missing
	if normalizedBenthos.Buffer == nil || len(normalizedBenthos.Buffer) == 0 {
		normalizedBenthos.Buffer = map[string]interface{}{
			"none": map[string]interface{}{},
		}
	}

	normalized.BenthosConfig = normalizedBenthos

	return normalized
}
