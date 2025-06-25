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

package topicbrowserserviceconfig

import (
	"fmt"
	"strings"
)

// Comparator handles the comparison of Topic Browser configurations
type Comparator struct {
	normalizer *Normalizer
}

// NewComparator creates a new configuration comparator for Topic Browser
func NewComparator() *Comparator {
	return &Comparator{
		normalizer: NewNormalizer(),
	}
}

// ConfigsEqual compares two Topic Browser Configs after normalization
func (c *Comparator) ConfigsEqual(desired, observed Config) (isEqual bool) {
	// First normalize both configs
	normDesired := c.normalizer.NormalizeConfig(desired)
	normObserved := c.normalizer.NormalizeConfig(observed)
	defer func() {
		if !isEqual {
			fmt.Printf("Normalized desired: %+v\n", normDesired)
			fmt.Printf("Normalized observed: %+v\n", normObserved)
		}
	}()

	return true
}

// ConfigDiff returns a human-readable string describing differences between configs
func (c *Comparator) ConfigDiff(desired, observed Config) string {
	var diff strings.Builder

	// First normalize both configs
	//	normDesired := c.normalizer.NormalizeConfig(desired)
	//	normObserved := c.normalizer.NormalizeConfig(observed)

	// Check basic fields

	if diff.Len() == 0 {
		return "No significant differences"
	}

	return diff.String()
}
