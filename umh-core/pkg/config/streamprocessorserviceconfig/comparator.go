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
	"reflect"

	"github.com/kylelemons/godebug/diff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/variables"
	"gopkg.in/yaml.v3"
)

// Comparator handles the comparison of StreamProcessor configurations
type Comparator struct{}

// NewComparator creates a new configuration comparator for StreamProcessor
func NewComparator() *Comparator {
	return &Comparator{}
}

// ConfigsEqual checks if two StreamProcessorServiceConfigSpecs are equal
func (c *Comparator) ConfigsEqual(desired, observed StreamProcessorServiceConfigSpec) bool {
	// Normalize both configs before comparison
	normalizer := NewNormalizer()
	normalizedDesired := normalizer.NormalizeConfig(desired)
	normalizedObserved := normalizer.NormalizeConfig(observed)

	// Compare model references
	if !reflect.DeepEqual(normalizedDesired.Config.Model, normalizedObserved.Config.Model) {
		return false
	}

	// Compare sources
	if !reflect.DeepEqual(normalizedDesired.Config.Sources, normalizedObserved.Config.Sources) {
		return false
	}

	// Compare mapping
	if !reflect.DeepEqual(normalizedDesired.Config.Mapping, normalizedObserved.Config.Mapping) {
		return false
	}

	// Compare variables using the variables comparator
	variablesComparator := variables.NewComparator()
	if !variablesComparator.ConfigsEqual(normalizedDesired.Variables, normalizedObserved.Variables) {
		return false
	}

	// Compare location
	if !reflect.DeepEqual(normalizedDesired.Location, normalizedObserved.Location) {
		return false
	}

	// Compare templateRef
	if normalizedDesired.TemplateRef != normalizedObserved.TemplateRef {
		return false
	}

	return true
}

// ConfigDiff generates a diff between two StreamProcessorServiceConfigSpecs
func (c *Comparator) ConfigDiff(desired, observed StreamProcessorServiceConfigSpec) string {
	// Normalize both configs before diffing
	normalizer := NewNormalizer()
	normalizedDesired := normalizer.NormalizeConfig(desired)
	normalizedObserved := normalizer.NormalizeConfig(observed)

	// Convert to YAML for diffing
	desiredYAML, err := yaml.Marshal(normalizedDesired)
	if err != nil {
		return "Error marshaling desired config: " + err.Error()
	}

	observedYAML, err := yaml.Marshal(normalizedObserved)
	if err != nil {
		return "Error marshaling observed config: " + err.Error()
	}

	return diff.Diff(string(observedYAML), string(desiredYAML))
}
