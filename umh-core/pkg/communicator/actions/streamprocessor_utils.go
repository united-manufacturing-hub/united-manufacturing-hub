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

// Package actions contains shared utility functions for stream processor actions.
package actions

import (
	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// generateUUIDFromName generates a UUID from a name (similar to protocol converter pattern)
func generateUUIDFromName(name string) uuid.UUID {
	// Use a deterministic UUID generation based on name
	// This ensures the same name always produces the same UUID
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(name))
}

// convertStreamProcessorMappingToInterface converts recursive StreamProcessorMapping to map[string]interface{}
func convertStreamProcessorMappingToInterface(mapping models.StreamProcessorMapping) map[string]interface{} {
	result := make(map[string]interface{})

	// Add source and transform fields if they exist
	if mapping.Source != "" {
		result["source"] = mapping.Source
	}
	if mapping.Transform != "" {
		result["transform"] = mapping.Transform
	}

	// Recursively convert subfields
	for key, subMapping := range mapping.Subfields {
		result[key] = convertStreamProcessorMappingToInterface(subMapping)
	}

	return result
}

// convertInterfaceToStreamProcessorMapping converts map[string]interface{} to recursive StreamProcessorMapping
func convertInterfaceToStreamProcessorMapping(mapping map[string]interface{}) models.StreamProcessorMapping {
	result := models.StreamProcessorMapping{
		Subfields: make(map[string]models.StreamProcessorMapping),
	}

	for key, value := range mapping {
		switch key {
		case "source":
			if str, ok := value.(string); ok {
				result.Source = str
			}
		case "transform":
			if str, ok := value.(string); ok {
				result.Transform = str
			}
		default:
			// Handle nested mappings
			if valueMap, ok := value.(map[string]interface{}); ok {
				result.Subfields[key] = convertInterfaceToStreamProcessorMapping(valueMap)
			}
		}
	}

	return result
}
