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

// GenerateUUIDFromName generates a UUID from a name (similar to protocol converter pattern)
func GenerateUUIDFromName(name string) uuid.UUID {
	// Use a deterministic UUID generation based on name
	// This ensures the same name always produces the same UUID
	return uuid.NewSHA1(uuid.NameSpaceDNS, []byte(name))
}

// convertStreamProcessorMappingToInterface converts StreamProcessorMapping to map[string]interface{}
func convertStreamProcessorMappingToInterface(mapping models.StreamProcessorMapping) map[string]interface{} {
	// Since StreamProcessorMapping is already map[string]interface{}, we can return it directly
	// but we make a copy to avoid any potential mutations
	result := make(map[string]interface{}, len(mapping))
	for key, value := range mapping {
		result[key] = value
	}
	return result
}

// convertInterfaceToStreamProcessorMapping converts map[string]interface{} to StreamProcessorMapping
func convertInterfaceToStreamProcessorMapping(mapping map[string]interface{}) models.StreamProcessorMapping {
	// Since StreamProcessorMapping is already map[string]interface{}, we can assign directly
	// but we make a copy to avoid any potential mutations
	result := make(models.StreamProcessorMapping, len(mapping))
	for key, value := range mapping {
		result[key] = value
	}
	return result
}
