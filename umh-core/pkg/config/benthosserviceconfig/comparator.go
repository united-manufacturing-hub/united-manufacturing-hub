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

import (
	"fmt"
	"reflect"
	"strings"

	"go.uber.org/zap"
)

// Comparator handles the comparison of Benthos configurations.
type Comparator struct {
	normalizer *Normalizer
}

// NewComparator creates a new configuration comparator for Benthos.
func NewComparator() *Comparator {
	return &Comparator{
		normalizer: NewNormalizer(),
	}
}

// ConfigsEqual compares two BenthosServiceConfigs after normalization.
//
// This runs for every component on every tick and canonicalization (see
// canonicalize) is the expensive part, so the cheap checks go first. Each step below
// is sufficient on its own; the later ones only widen what counts as equal.
func (c *Comparator) ConfigsEqual(desired, observed BenthosServiceConfig) (isEqual bool) {
	normDesired := c.normalizer.NormalizeConfig(desired)
	normObserved := c.normalizer.NormalizeConfig(observed)

	defer func(normDesired, normObserved *BenthosServiceConfig) {
		if !isEqual {
			zap.S().Debugf("Normalized desired:  %+v", normDesired)
			zap.S().Debugf("Normalized observed: %+v", normObserved)
		}
	}(&normDesired, &normObserved) // configs are passed as pointers to make the copy cheap

	// 1. Both sides as they stand.
	if configsIdentical(normDesired, normObserved) {
		return true
	}

	// 2. Desired canonicalized; observed came off disk already canonical.
	normDesired = canonicalize(normDesired)
	if configsEqualNormalized(normDesired, normObserved) {
		return true
	}

	// 3. Observed canonicalized too, for callers comparing two Go-built configs.
	normObserved = canonicalize(normObserved)

	return configsEqualNormalized(normDesired, normObserved)
}

// configsIdentical reports whether two normalized configs hold the same values, with
// no allowance for how a value is spelled. MetricsPort is excluded as everywhere
// else, since the port manager allocates it.
//
// Step 1 of ConfigsEqual needs this rather than configsEqualNormalized, whose
// leniency about processors only holds once both sides are canonicalized:
// getProcessors reports "no processors" for any list not held as []interface{}, so
// two Go-built configs holding []map[string]interface{} would both look
// processor-less and never have their processors compared.
func configsIdentical(a, b BenthosServiceConfig) bool {
	return a.DebugLevel == b.DebugLevel &&
		reflect.DeepEqual(a.Input, b.Input) &&
		reflect.DeepEqual(a.Output, b.Output) &&
		reflect.DeepEqual(a.Pipeline, b.Pipeline) &&
		reflect.DeepEqual(a.Buffer, b.Buffer) &&
		isResourcesEqual(a.CacheResources, b.CacheResources) &&
		isResourcesEqual(a.RateLimitResources, b.RateLimitResources)
}

// configsEqualNormalized compares two configs that are already normalized and
// canonicalized.
func configsEqualNormalized(normDesired, normObserved BenthosServiceConfig) bool {
	// Compare essential fields that must match exactly
	// Ignoring MetricsPort since it's allocated by the port manager
	if normDesired.DebugLevel != normObserved.DebugLevel {
		return false
	}

	// Compare maps with deep equality
	if !reflect.DeepEqual(normDesired.Input, normObserved.Input) ||
		!reflect.DeepEqual(normDesired.Output, normObserved.Output) ||
		!isResourcesEqual(normDesired.CacheResources, normObserved.CacheResources) ||
		!isResourcesEqual(normDesired.RateLimitResources, normObserved.RateLimitResources) {
		return false
	}

	// Special handling for pipeline processors
	desiredProcs := getProcessors(normDesired.Pipeline)

	observedProcs := getProcessors(normObserved.Pipeline)
	if len(desiredProcs) == 0 && len(observedProcs) == 0 {
		// Both have empty processors, now compare the rest of pipeline
		pipeDesiredCopy := copyMap(normDesired.Pipeline)
		pipeObservedCopy := copyMap(normObserved.Pipeline)

		delete(pipeDesiredCopy, "processors")
		delete(pipeObservedCopy, "processors")

		if !reflect.DeepEqual(pipeDesiredCopy, pipeObservedCopy) {
			return false
		}
	} else if !reflect.DeepEqual(desiredProcs, observedProcs) {
		return false
	}

	// Special handling for buffer
	if len(normDesired.Buffer) == 1 && len(normObserved.Buffer) == 1 {
		if _, hasNoneDesired := normDesired.Buffer["none"]; hasNoneDesired {
			if _, hasNoneObserved := normObserved.Buffer["none"]; hasNoneObserved {
				// Both have "none" buffer, consider them equal
				return true
			}
		}
	}

	return reflect.DeepEqual(normDesired.Buffer, normObserved.Buffer)
}

// ConfigDiff returns a human-readable string describing differences between configs.
func (c *Comparator) ConfigDiff(desired, observed BenthosServiceConfig) string {
	var diff strings.Builder

	// Normalize both configs, then canonicalize both (see ConfigsEqual). This runs
	// only after a difference has been reported, so skipping the observed side saves
	// nothing that matters, and a diff listing a representational difference would
	// send the reader after a phantom.
	normDesired := canonicalize(c.normalizer.NormalizeConfig(desired))
	normObserved := canonicalize(c.normalizer.NormalizeConfig(observed))

	// Check basic scalar fields
	if normDesired.MetricsPort != normObserved.MetricsPort {
		// Metrics port differences are expected and managed by the port manager,
		// but we still log them for diagnostic purposes
		diff.WriteString(fmt.Sprintf("MetricsPort: Want: %d, Have: %d (Note: Difference is expected and handled by port manager)\n",
			normDesired.MetricsPort, normObserved.MetricsPort))
	}

	if normDesired.DebugLevel != normObserved.DebugLevel {
		diff.WriteString(fmt.Sprintf("DebugLevel: Want: %v, Have: %v\n",
			normDesired.DebugLevel, normObserved.DebugLevel))
	}

	// Compare Input sections
	if !reflect.DeepEqual(normDesired.Input, normObserved.Input) {
		diff.WriteString("Input config differences:\n")
		compareMapKeys(normDesired.Input, normObserved.Input, "Input", &diff)
	}

	// Compare Output sections
	if !reflect.DeepEqual(normDesired.Output, normObserved.Output) {
		diff.WriteString("Output config differences:\n")
		compareMapKeys(normDesired.Output, normObserved.Output, "Output", &diff)
	}

	// Compare Pipeline sections
	if !reflect.DeepEqual(normDesired.Pipeline, normObserved.Pipeline) {
		diff.WriteString("Pipeline config differences:\n")

		// Special handling for processors
		desiredProcs := getProcessors(normDesired.Pipeline)

		observedProcs := getProcessors(normObserved.Pipeline)
		if !reflect.DeepEqual(desiredProcs, observedProcs) {
			fmt.Fprintf(&diff, "  - Processors differ (desired %d, observed %d)\n    desired:  %v\n    observed: %v\n",
				len(desiredProcs), len(observedProcs), desiredProcs, observedProcs)
		}

		// Compare other pipeline keys
		pipeDesiredCopy := copyMap(normDesired.Pipeline)
		pipeObservedCopy := copyMap(normObserved.Pipeline)

		delete(pipeDesiredCopy, "processors")
		delete(pipeObservedCopy, "processors")
		compareMapKeys(pipeDesiredCopy, pipeObservedCopy, "Pipeline", &diff)
	}

	// Compare Buffer sections
	if !reflect.DeepEqual(normDesired.Buffer, normObserved.Buffer) {
		// Skip comparing if both are effectively "none" buffer
		if !isNoneBuffer(normDesired.Buffer) || !isNoneBuffer(normObserved.Buffer) {
			diff.WriteString("Buffer config differences:\n")
			compareMapKeys(normDesired.Buffer, normObserved.Buffer, "Buffer", &diff)
		}
	}

	// Compare cache resources
	if !isResourcesEqual(normDesired.CacheResources, normObserved.CacheResources) {
		diff.WriteString(fmt.Sprintf("Cache resources differ. Want: %v, Have: %v\n",
			normDesired.CacheResources, normObserved.CacheResources))
	}

	// Compare rate limit resources
	if !isResourcesEqual(normDesired.RateLimitResources, normObserved.RateLimitResources) {
		diff.WriteString(fmt.Sprintf("Rate limit resources differ. Want: %v, Have: %v\n",
			normDesired.RateLimitResources, normObserved.RateLimitResources))
	}

	if diff.Len() == 0 {
		return "No significant differences\n"
	}

	return diff.String()
}

// Helper functions

// getProcessors extracts the processors array from a pipeline config.
func getProcessors(pipeline map[string]interface{}) []interface{} {
	if pipeline == nil {
		return []interface{}{}
	}

	if procs, ok := pipeline["processors"]; ok {
		if procsArray, ok := procs.([]interface{}); ok {
			return procsArray
		}
	}

	return []interface{}{}
}

// copyMap creates a shallow copy of a map.
func copyMap(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}

	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		result[k] = v
	}

	return result
}

// isNoneBuffer checks if a buffer config is the default "none" buffer.
func isNoneBuffer(buffer map[string]interface{}) bool {
	if len(buffer) != 1 {
		return false
	}

	if _, hasNone := buffer["none"]; hasNone {
		return true
	}

	return false
}

// compareMapKeys compares keys in two maps and logs differences.
func compareMapKeys(desired, observed map[string]interface{}, prefix string, diff *strings.Builder) {
	// Check keys in desired that don't exist or are different in observed
	for k, v := range desired {
		if observedVal, ok := observed[k]; !ok {
			fmt.Fprintf(diff, "  - %s.%s: exists in desired but missing in observed\n", prefix, k)
		} else if !reflect.DeepEqual(v, observedVal) {
			fmt.Fprintf(diff, "  - %s.%s differs\n", prefix, k)
		}
	}

	// Check for keys in observed that don't exist in desired
	for k := range observed {
		if _, ok := desired[k]; !ok {
			fmt.Fprintf(diff, "  - %s.%s: exists in observed but missing in desired\n", prefix, k)
		}
	}
}

// isResourcesEqual handles comparison of resource slices, properly handling nil and empty slices.
func isResourcesEqual(a, b interface{}) bool {
	// For nil or empty slices
	aIsNilOrEmpty := isNilOrEmpty(a)
	bIsNilOrEmpty := isNilOrEmpty(b)

	// If both are nil or empty, consider them equal
	if aIsNilOrEmpty && bIsNilOrEmpty {
		return true
	}

	// Otherwise, use standard deep equality
	return reflect.DeepEqual(a, b)
}

// isNilOrEmpty checks if a value is nil or an empty slice.
func isNilOrEmpty(v interface{}) bool {
	if v == nil {
		return true
	}

	// Check if it's a slice
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice {
		return rv.Len() == 0
	}

	return false
}
