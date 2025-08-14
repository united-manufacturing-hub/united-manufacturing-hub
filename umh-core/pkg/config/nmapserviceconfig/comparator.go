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

package nmapserviceconfig

import (
	"fmt"
	"strings"

	"go.uber.org/zap"
)

// Comparator handles the comparison of Nmap configurations.
type Comparator struct {
	normalizer *Normalizer
}

// NewComparator creates a new configuration comparator for Nmap.
func NewComparator() *Comparator {
	return &Comparator{
		normalizer: NewNormalizer(),
	}
}

// ConfigsEqual compares two NmapServiceConfigs after normalization.
func (c *Comparator) ConfigsEqual(desired, observed NmapServiceConfig) bool {
	// First normalize both configs
	normDesired := c.normalizer.NormalizeConfig(desired)
	normObserved := c.normalizer.NormalizeConfig(observed)

	var isEqual bool

	defer func() {
		if !isEqual {
			zap.S().Infof("Normalized desired:  %+v", normDesired)
			zap.S().Infof("Normalized observed: %+v", normObserved)
		}
	}()

	// Compare essential fields that must match exactly
	if normDesired.Target != normObserved.Target {
		isEqual = false

		return isEqual
	}

	if normDesired.Port != normObserved.Port {
		isEqual = false

		return isEqual
	}

	isEqual = true

	return isEqual
}

// ConfigDiff returns a human-readable string describing differences between configs.
func (c *Comparator) ConfigDiff(desired, observed NmapServiceConfig) string {
	var diff strings.Builder

	// First normalize both configs
	normDesired := c.normalizer.NormalizeConfig(desired)
	normObserved := c.normalizer.NormalizeConfig(observed)

	// Check basic fields
	if normDesired.Target != normObserved.Target {
		diff.WriteString(fmt.Sprintf("Target: Want: %s, Have: %s\n",
			normDesired.Target, normObserved.Target))
	}

	if normDesired.Port != normObserved.Port {
		diff.WriteString(fmt.Sprintf("Port: Want: %d, Have: %d\n",
			normDesired.Port, normObserved.Port))
	}

	if diff.Len() == 0 {
		return "No significant differences"
	}

	return diff.String()
}
