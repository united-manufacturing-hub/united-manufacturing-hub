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

package dataflowcomponentconfig

import (
	"fmt"
	"reflect"
)

// Comparator handles the comparison of DataflowComponent configurations
type Comparator struct {
	normalizer *Normalizer
}

// NewComparator creates a new configuration comparator for DataflowComponent
func NewComparator() *Comparator {
	return &Comparator{
		normalizer: NewNormalizer(),
	}
}

// ConfigsEqual compares two DataflowComponentConfig after normalization
func (c *Comparator) ConfigsEqual(desired, observed DataFlowComponentConfig) (isEqual bool) {
	// First normalize both configs
	normDesired := c.normalizer.NormalizeConfig(desired)
	normObserved := c.normalizer.NormalizeConfig(observed)
	defer func() {
		if !isEqual {
			fmt.Printf("Normalized desired: %+v\n", normDesired)
			fmt.Printf("Normalized observed: %+v\n", normObserved)
		}
	}()

	// BethosConfig is the only field for DataflowComponentConfig. In future if a new field is added, then that should compared here
	// Compare maps with deep equality
	if !reflect.DeepEqual(normDesired.BenthosConfig.Input, normObserved.BenthosConfig.Input) ||
		!reflect.DeepEqual(normDesired.BenthosConfig.Output, normObserved.BenthosConfig.Output) ||
		!isResourcesEqual(normDesired.BenthosConfig.CacheResources, normObserved.BenthosConfig.CacheResources) ||
		!isResourcesEqual(normDesired.BenthosConfig.RateLimitResources, normObserved.BenthosConfig.RateLimitResources) {
		return false
	}

	// Special handling for pipeline processors
	desiredProcs := getProcessors(normDesired.BenthosConfig.Pipeline)
	observedProcs := getProcessors(normObserved.BenthosConfig.Pipeline)
	if len(desiredProcs) == 0 && len(observedProcs) == 0 {
		// Both have empty processors, now compare the rest of pipeline
		pipeDesiredCopy := copyMap(normDesired.BenthosConfig.Pipeline)
		pipeObservedCopy := copyMap(normObserved.BenthosConfig.Pipeline)
		delete(pipeDesiredCopy, "processors")
		delete(pipeObservedCopy, "processors")
		if !reflect.DeepEqual(pipeDesiredCopy, pipeObservedCopy) {
			return false
		}
	} else if !reflect.DeepEqual(desiredProcs, observedProcs) {
		return false
	}

	// Special handling for buffer
	if len(normDesired.BenthosConfig.Buffer) == 1 && len(normObserved.BenthosConfig.Buffer) == 1 {
		if _, hasNoneDesired := normDesired.BenthosConfig.Buffer["none"]; hasNoneDesired {
			if _, hasNoneObserved := normObserved.BenthosConfig.Buffer["none"]; hasNoneObserved {
				// Both have "none" buffer, consider them equal
				return true
			}
		}
	}
	return reflect.DeepEqual(normDesired.BenthosConfig.Buffer, normObserved.BenthosConfig.Buffer)
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

// isResourcesEqual handles comparison of resource slices, properly handling nil and empty slices
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

// isNilOrEmpty checks if a value is nil or an empty slice
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
