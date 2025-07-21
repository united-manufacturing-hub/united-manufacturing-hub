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
	"fmt"
	"reflect"
	"strings"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/variables"
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
	var diff strings.Builder

	// Normalize both configs before diffing
	normalizer := NewNormalizer()
	normalizedDesired := normalizer.NormalizeConfig(desired)
	normalizedObserved := normalizer.NormalizeConfig(observed)

	// Compare model references
	if !reflect.DeepEqual(normalizedDesired.Config.Model, normalizedObserved.Config.Model) {
		diff.WriteString(fmt.Sprintf("Model references differ. Want: %v, Have: %v\n",
			normalizedDesired.Config.Model, normalizedObserved.Config.Model))
	}

	// Compare sources
	if !reflect.DeepEqual(normalizedDesired.Config.Sources, normalizedObserved.Config.Sources) {
		diff.WriteString(fmt.Sprintf("Sources differ. Want: %v, Have: %v\n",
			normalizedDesired.Config.Sources, normalizedObserved.Config.Sources))
	}

	// Compare mapping
	if !reflect.DeepEqual(normalizedDesired.Config.Mapping, normalizedObserved.Config.Mapping) {
		diff.WriteString(fmt.Sprintf("Mapping differs. Want: %v, Have: %v\n",
			normalizedDesired.Config.Mapping, normalizedObserved.Config.Mapping))
	}

	// Compare variables using the variables comparator
	variablesComparator := variables.NewComparator()
	if !variablesComparator.ConfigsEqual(normalizedDesired.Variables, normalizedObserved.Variables) {
		diff.WriteString("Variables differ\n")
	}

	// Compare location
	if !reflect.DeepEqual(normalizedDesired.Location, normalizedObserved.Location) {
		diff.WriteString(fmt.Sprintf("Location differs. Want: %v, Have: %v\n",
			normalizedDesired.Location, normalizedObserved.Location))
	}

	// Compare templateRef
	if normalizedDesired.TemplateRef != normalizedObserved.TemplateRef {
		diff.WriteString(fmt.Sprintf("TemplateRef differs. Want: %s, Have: %s\n",
			normalizedDesired.TemplateRef, normalizedObserved.TemplateRef))
	}

	if diff.Len() == 0 {
		return "No significant differences\n"
	}

	return diff.String()
}
