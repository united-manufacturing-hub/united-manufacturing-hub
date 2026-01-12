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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"go.uber.org/zap"
)

// Comparator handles the comparison of Topic Browser configurations.
type Comparator struct {
	normalizer              *Normalizer
	benthosConfigComparator *benthosserviceconfig.Comparator
}

// NewComparator creates a new configuration comparator for Topic Browser.
func NewComparator() *Comparator {
	return &Comparator{
		normalizer:              NewNormalizer(),
		benthosConfigComparator: benthosserviceconfig.NewComparator(),
	}
}

// ConfigsEqual compares two Topic Browser Configs after normalization.
func (c *Comparator) ConfigsEqual(desired, observed Config) (isEqual bool) {
	// First normalize both configs
	normDesired := c.normalizer.NormalizeConfig(desired)
	normObserved := c.normalizer.NormalizeConfig(observed)

	defer func(normDesired, normObserved *Config) {
		if !isEqual {
			zap.S().Debugf("Normalized desired:  %+v", normDesired)
			zap.S().Debugf("Normalized observed: %+v", normObserved)
		}
	}(&normDesired, &normObserved) // configs are passed as pointers to make the copy cheap

	// Since Config is currently empty, they are always equal
	// When fields are added to Config, add comparison logic here
	return c.benthosConfigComparator.ConfigsEqual(normDesired.BenthosConfig, normObserved.BenthosConfig)
}

// ConfigDiff returns a human-readable string describing differences between configs.
func (c *Comparator) ConfigDiff(desired, observed Config) string {
	// First normalize both configs
	normDesired := c.normalizer.NormalizeConfig(desired)
	normObserved := c.normalizer.NormalizeConfig(observed)

	return c.benthosConfigComparator.ConfigDiff(normDesired.BenthosConfig, normObserved.BenthosConfig)
}
