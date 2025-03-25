package yaml

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
)

// Comparator handles the comparison of Benthos configurations
type Comparator struct {
	normalizer *Normalizer
}

// NewComparator creates a new configuration comparator for Benthos
func NewComparator() *Comparator {
	return &Comparator{
		normalizer: NewNormalizer(),
	}
}

// ConfigsEqual compares two BenthosServiceConfigs after normalization
func (c *Comparator) ConfigsEqual(desired, observed config.BenthosServiceConfig) bool {
	// First normalize both configs
	normDesired := c.normalizer.NormalizeConfig(desired)
	normObserved := c.normalizer.NormalizeConfig(observed)

	// Compare essential fields that must match exactly
	// Ignoring MetricsPort since it's allocated by the port manager
	if normDesired.LogLevel != normObserved.LogLevel {
		return false
	}

	// Compare maps with deep equality
	if !reflect.DeepEqual(normDesired.Input, normObserved.Input) ||
		!reflect.DeepEqual(normDesired.Output, normObserved.Output) ||
		!reflect.DeepEqual(normDesired.CacheResources, normObserved.CacheResources) ||
		!reflect.DeepEqual(normDesired.RateLimitResources, normObserved.RateLimitResources) {
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

// ConfigDiff returns a human-readable string describing differences between configs
func (c *Comparator) ConfigDiff(desired, observed config.BenthosServiceConfig) string {
	var diff strings.Builder

	// First normalize both configs
	normDesired := c.normalizer.NormalizeConfig(desired)
	normObserved := c.normalizer.NormalizeConfig(observed)

	// Check basic scalar fields
	if normDesired.MetricsPort != normObserved.MetricsPort {
		// Metrics port differences are expected and managed by the port manager,
		// but we still log them for diagnostic purposes
		diff.WriteString(fmt.Sprintf("MetricsPort: Want: %d, Have: %d (Note: Difference is expected and handled by port manager)\n",
			normDesired.MetricsPort, normObserved.MetricsPort))
	}

	if normDesired.LogLevel != normObserved.LogLevel {
		diff.WriteString(fmt.Sprintf("LogLevel: Want: %s, Have: %s\n",
			normDesired.LogLevel, normObserved.LogLevel))
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
			diff.WriteString("  - Processors differ\n")
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
		if !(isNoneBuffer(normDesired.Buffer) && isNoneBuffer(normObserved.Buffer)) {
			diff.WriteString("Buffer config differences:\n")
			compareMapKeys(normDesired.Buffer, normObserved.Buffer, "Buffer", &diff)
		}
	}

	// Compare cache resources
	if !reflect.DeepEqual(normDesired.CacheResources, normObserved.CacheResources) {
		diff.WriteString("Cache resources differ\n")
	}

	// Compare rate limit resources
	if !reflect.DeepEqual(normDesired.RateLimitResources, normObserved.RateLimitResources) {
		diff.WriteString("Rate limit resources differ\n")
	}

	if diff.Len() == 0 {
		return "No significant differences"
	}

	return diff.String()
}

// Helper functions

// getProcessors extracts the processors array from a pipeline config
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

// copyMap creates a shallow copy of a map
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

// isNoneBuffer checks if a buffer config is the default "none" buffer
func isNoneBuffer(buffer map[string]interface{}) bool {
	if len(buffer) != 1 {
		return false
	}

	if _, hasNone := buffer["none"]; hasNone {
		return true
	}
	return false
}

// compareMapKeys compares keys in two maps and logs differences
func compareMapKeys(desired, observed map[string]interface{}, prefix string, diff *strings.Builder) {
	// Check keys in desired that don't exist or are different in observed
	for k, v := range desired {
		if observedVal, ok := observed[k]; !ok {
			diff.WriteString(fmt.Sprintf("  - %s.%s: exists in desired but missing in observed\n", prefix, k))
		} else if !reflect.DeepEqual(v, observedVal) {
			diff.WriteString(fmt.Sprintf("  - %s.%s differs\n", prefix, k))
		}
	}

	// Check for keys in observed that don't exist in desired
	for k := range observed {
		if _, ok := desired[k]; !ok {
			diff.WriteString(fmt.Sprintf("  - %s.%s: exists in observed but missing in desired\n", prefix, k))
		}
	}
}
