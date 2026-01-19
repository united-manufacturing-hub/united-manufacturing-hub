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

// MergeDependencies creates a new map combining parent and child dependencies.
// Child dependencies override parent dependencies with the same key.
//
// IMPORTANT: This performs a SHALLOW merge. Interface and channel values are
// shared between parent and child, which is intentional - dependencies represent
// shared resources like connection pools and channel providers.
//
// Setting a key to nil in child does NOT remove it - the merged result will
// have key=nil. To completely exclude a parent dependency, the worker must
// check for nil when accessing.
//
// Example:
//
//	parent = {"channelProvider": provider1, "timeout": 30}
//	child  = {"timeout": 60, "customDep": value}
//	result = {"channelProvider": provider1, "timeout": 60, "customDep": value}
func MergeDependencies(parent, child map[string]any) map[string]any {
	// Both nil - return nil (no allocation)
	if parent == nil && child == nil {
		return nil
	}

	// Only child - return shallow copy of child (not original reference)
	if parent == nil {
		result := make(map[string]any, len(child))
		for k, v := range child {
			result[k] = v
		}

		return result
	}

	// Only parent - return shallow copy of parent (not original reference)
	if child == nil {
		result := make(map[string]any, len(parent))
		for k, v := range parent {
			result[k] = v
		}

		return result
	}

	// Both present - merge (child overrides parent)
	result := make(map[string]any, len(parent)+len(child))
	for k, v := range parent {
		result[k] = v
	}

	for k, v := range child {
		result[k] = v
	}

	return result
}
