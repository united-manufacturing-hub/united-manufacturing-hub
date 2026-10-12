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

package config

// DeepCloneMap creates a deep copy of a map preserving original types.
// Unlike a JSON round-trip, this preserves numeric types (int, int64,
// float32, etc.) instead of coercing all numbers to float64.
//
// The values it copies are map[string]any, map[any]any and []any, cloned
// recursively; every other value is returned as is.
//
// UserSpec's three map fields (Variables, ConfigValues, Names) pass through
// here on every Clone: a spec handed to another goroutine must not share
// map storage with the spec the supervisor keeps.
func DeepCloneMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}

	result := make(map[string]any, len(m))

	for k, v := range m {
		result[k] = deepCloneValue(v)
	}

	return result
}

// deepCloneValue recursively clones a value, preserving its original type.
func deepCloneValue(v any) any {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case map[string]any:
		return DeepCloneMap(val)

	case map[any]any:
		result := make(map[any]any, len(val))
		for k, item := range val {
			result[k] = deepCloneValue(item)
		}

		return result

	case []any:
		result := make([]any, len(val))
		for i, item := range val {
			result[i] = deepCloneValue(item)
		}

		return result

	// Every other value is immutable or a typed container the caller owns,
	// so it is returned as is. This preserves int, int64, float32, float64,
	// bool, string, []string, map[string]string, etc.
	default:
		return v
	}
}
