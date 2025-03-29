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

import (
	"fmt"
	"reflect"
	"strings"
)

// Comparator handles the comparison of Redpanda configurations
type Comparator struct {
	normalizer *Normalizer
}

// NewComparator creates a new configuration comparator for Redpanda
func NewComparator() *Comparator {
	return &Comparator{
		normalizer: NewNormalizer(),
	}
}

// ConfigsEqual compares two RedpandaServiceConfigs after normalization
func (c *Comparator) ConfigsEqual(desired, observed RedpandaServiceConfig) (isEqual bool) {
	// First normalize both configs
	normDesired := c.normalizer.NormalizeConfig(desired)
	normObserved := c.normalizer.NormalizeConfig(observed)
	defer func() {
		if !isEqual {
			fmt.Printf("Normalized desired: %+v\n", normDesired)
			fmt.Printf("Normalized observed: %+v\n", normObserved)
		}
	}()

	return reflect.DeepEqual(normDesired, normObserved)
}

// ConfigDiff returns a human-readable string describing differences between configs
func (c *Comparator) ConfigDiff(desired, observed RedpandaServiceConfig) string {
	var diff strings.Builder

	// First normalize both configs
	normDesired := c.normalizer.NormalizeConfig(desired)
	normObserved := c.normalizer.NormalizeConfig(observed)

	// Check basic scalar fields
	if normDesired.DefaultTopicRetentionMs != normObserved.DefaultTopicRetentionMs {
		diff.WriteString(fmt.Sprintf("DefaultTopicRetentionMs: Want: %d, Have: %d\n",
			normDesired.DefaultTopicRetentionMs, normObserved.DefaultTopicRetentionMs))
	}

	if normDesired.DefaultTopicRetentionBytes != normObserved.DefaultTopicRetentionBytes {
		diff.WriteString(fmt.Sprintf("DefaultTopicRetentionBytes: Want: %d, Have: %d\n",
			normDesired.DefaultTopicRetentionBytes, normObserved.DefaultTopicRetentionBytes))
	}

	if diff.Len() == 0 {
		return "No significant differences"
	}

	return diff.String()
}
