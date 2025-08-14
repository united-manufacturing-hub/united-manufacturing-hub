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

	"go.uber.org/zap"
)

// Comparator handles the comparison of Redpanda configurations.
type Comparator struct {
	normalizer *Normalizer
}

// NewComparator creates a new configuration comparator for Redpanda.
func NewComparator() *Comparator {
	return &Comparator{
		normalizer: NewNormalizer(),
	}
}

// ConfigsEqual compares two RedpandaServiceConfigs after normalization.
func (c *Comparator) ConfigsEqual(desired, observed RedpandaServiceConfig) bool {
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

	isEqual = reflect.DeepEqual(normDesired, normObserved)

	return isEqual
}

// ConfigDiff returns a human-readable string describing differences between configs.
func (c *Comparator) ConfigDiff(desired, observed RedpandaServiceConfig) string {
	var diff strings.Builder

	// First normalize both configs
	normDesired := c.normalizer.NormalizeConfig(desired)
	normObserved := c.normalizer.NormalizeConfig(observed)

	// Check topic configuration
	if normDesired.Topic.DefaultTopicRetentionMs != normObserved.Topic.DefaultTopicRetentionMs {
		diff.WriteString(fmt.Sprintf("Topic.DefaultTopicRetentionMs: Want: %d, Have: %d\n",
			normDesired.Topic.DefaultTopicRetentionMs, normObserved.Topic.DefaultTopicRetentionMs))
	}

	if normDesired.Topic.DefaultTopicRetentionBytes != normObserved.Topic.DefaultTopicRetentionBytes {
		diff.WriteString(fmt.Sprintf("Topic.DefaultTopicRetentionBytes: Want: %d, Have: %d\n",
			normDesired.Topic.DefaultTopicRetentionBytes, normObserved.Topic.DefaultTopicRetentionBytes))
	}

	if normDesired.Topic.DefaultTopicCompressionAlgorithm != normObserved.Topic.DefaultTopicCompressionAlgorithm {
		diff.WriteString(fmt.Sprintf("Topic.DefaultTopicCompressionAlgorithm: Want: %s, Have: %s\n",
			normDesired.Topic.DefaultTopicCompressionAlgorithm, normObserved.Topic.DefaultTopicCompressionAlgorithm))
	}

	if normDesired.Topic.DefaultTopicCleanupPolicy != normObserved.Topic.DefaultTopicCleanupPolicy {
		diff.WriteString(fmt.Sprintf("Topic.DefaultTopicCleanupPolicy: Want: %s, Have: %s\n",
			normDesired.Topic.DefaultTopicCleanupPolicy, normObserved.Topic.DefaultTopicCleanupPolicy))
	}

	if normDesired.Topic.DefaultTopicSegmentMs != normObserved.Topic.DefaultTopicSegmentMs {
		diff.WriteString(fmt.Sprintf("Topic.DefaultTopicSegmentMs: Want: %d, Have: %d\n",
			normDesired.Topic.DefaultTopicSegmentMs, normObserved.Topic.DefaultTopicSegmentMs))
	}

	// Check resources configuration
	if normDesired.Resources.MaxCores != normObserved.Resources.MaxCores {
		diff.WriteString(fmt.Sprintf("Resources.MaxCores: Want: %d, Have: %d\n",
			normDesired.Resources.MaxCores, normObserved.Resources.MaxCores))
	}

	if normDesired.Resources.MemoryPerCoreInBytes != normObserved.Resources.MemoryPerCoreInBytes {
		diff.WriteString(fmt.Sprintf("Resources.MemoryPerCoreInBytes: Want: %d, Have: %d\n",
			normDesired.Resources.MemoryPerCoreInBytes, normObserved.Resources.MemoryPerCoreInBytes))
	}

	if diff.Len() == 0 {
		return "No significant differences"
	}

	return diff.String()
}
