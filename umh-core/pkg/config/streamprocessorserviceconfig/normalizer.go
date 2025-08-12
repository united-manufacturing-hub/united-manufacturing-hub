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

package streamprocessorserviceconfig

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/variables"
)

// Normalizer handles the normalization of StreamProcessor configurations.
type Normalizer struct{}

// NewNormalizer creates a new configuration normalizer for StreamProcessor.
func NewNormalizer() *Normalizer {
	return &Normalizer{}
}

// NormalizeConfig applies StreamProcessor defaults to a structured config.
func (n *Normalizer) NormalizeConfig(cfg StreamProcessorServiceConfigSpec) StreamProcessorServiceConfigSpec {
	// create a shallow copy
	normalized := cfg

	// Normalize the variables
	variablesNormalizer := variables.NewNormalizer()
	normalized.Variables = variablesNormalizer.NormalizeConfig(normalized.Variables)

	// Normalize the model reference - ensure version is set
	if normalized.Config.Model.Version == "" {
		normalized.Config.Model.Version = "v1"
	}

	// Normalize sources - ensure they exist even if empty
	if normalized.Config.Sources == nil {
		normalized.Config.Sources = make(SourceMapping)
	}

	// Normalize mapping - ensure both dynamic and static exist even if empty
	if normalized.Config.Mapping == nil {
		normalized.Config.Mapping = make(map[string]interface{})
	}

	// No need to normalize the location

	return normalized
}
