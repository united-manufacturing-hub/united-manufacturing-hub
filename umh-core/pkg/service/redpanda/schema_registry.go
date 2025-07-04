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

package redpanda

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
)

// SchemaSubject represents a schema subject with its parsed details
type SchemaSubject struct {
	// Subject is the name of the subject
	Subject string `json:"subject"`

	// Parsed components (empty if parsing fails)
	Name          string `json:"name,omitempty"`          // e.g., "pump"
	Version       string `json:"version,omitempty"`       // e.g., "v1"
	DataModelType string `json:"dataModelType,omitempty"` // e.g., "timeseries"
	DataType      string `json:"dataType,omitempty"`      // e.g., "number", "string", "bool"

	// ParsedSuccessfully indicates if the schema name was successfully parsed
	ParsedSuccessfully bool `json:"parsedSuccessfully"`
}

// parseSchemaName parses a schema name into its components
// Expected format: _<Name>_<Version>_<DataModelType>-<DataType> (leading underscore from translation)
// Example: "_pump_v1_timeseries-number"
func parseSchemaName(schemaName string) (name, version, dataModelType, dataType string, success bool) {
	// Skip leading underscore if present (from translation code)
	schemaName = strings.TrimPrefix(schemaName, "_")

	// Find first underscore
	firstUnderscore := strings.IndexByte(schemaName, '_')
	if firstUnderscore == -1 || firstUnderscore == 0 {
		return "", "", "", "", false
	}

	name = schemaName[:firstUnderscore]

	// Find second underscore
	secondUnderscore := strings.IndexByte(schemaName[firstUnderscore+1:], '_')
	if secondUnderscore == -1 {
		return "", "", "", "", false
	}
	secondUnderscore += firstUnderscore + 1 // Adjust for offset

	version = schemaName[firstUnderscore+1 : secondUnderscore]

	// Find hyphen in the remaining part
	remaining := schemaName[secondUnderscore+1:]
	hyphenIndex := strings.IndexByte(remaining, '-')
	if hyphenIndex == -1 || hyphenIndex == 0 || hyphenIndex == len(remaining)-1 {
		return "", "", "", "", false
	}

	dataModelType = remaining[:hyphenIndex]
	dataType = remaining[hyphenIndex+1:]

	// Validate that we have reasonable values (length check is faster than string comparison)
	if len(name) == 0 || len(version) == 0 || len(dataModelType) == 0 || len(dataType) == 0 {
		return "", "", "", "", false
	}

	return name, version, dataModelType, dataType, true
}

// GetAllSchemas fetches all schema subject names from the Redpanda Schema Registry
func (s *RedpandaService) GetAllSchemas(ctx context.Context) ([]SchemaSubject, error) {
	if s.httpClient == nil {
		return nil, fmt.Errorf("http client not initialized")
	}

	// Get all subjects
	subjectsReq, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/subjects", constants.SchemaRegistryPort), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create subjects request: %w", err)
	}

	subjectsResp, err := s.httpClient.Do(subjectsReq)
	if err != nil {
		// If we get a connection refused error, it means that the Schema Registry is not yet ready, so we return an empty list
		if strings.Contains(err.Error(), "connection refused") {
			return []SchemaSubject{}, nil
		}
		return nil, fmt.Errorf("failed to fetch subjects: %w", err)
	}
	if subjectsResp == nil {
		return nil, fmt.Errorf("received nil response from subjects request")
	}
	defer func() {
		_ = subjectsResp.Body.Close()
	}()

	if subjectsResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", subjectsResp.StatusCode)
	}

	subjectsBody, err := io.ReadAll(subjectsResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read subjects response: %w", err)
	}

	var subjectNames []string
	if err := json.Unmarshal(subjectsBody, &subjectNames); err != nil {
		return nil, fmt.Errorf("failed to unmarshal subjects response: %w", err)
	}

	// Convert to SchemaSubject structs with parsed components
	schemas := make([]SchemaSubject, len(subjectNames))
	for i, subject := range subjectNames {
		name, version, dataModelType, dataType, success := parseSchemaName(subject)
		schemas[i] = SchemaSubject{
			Subject:            subject,
			Name:               name,
			Version:            version,
			DataModelType:      dataModelType,
			DataType:           dataType,
			ParsedSuccessfully: success,
		}
	}

	return schemas, nil
}

// DataModelSchemaMapping represents the mapping between datamodels and expected schemas
type DataModelSchemaMapping struct {
	// MissingInRegistry contains schema names that should exist in registry but don't
	MissingInRegistry []string
	// OrphanedInRegistry contains schema names that exist in registry but have no corresponding datamodel
	OrphanedInRegistry []string
	// ExpectedSchemas contains all schema names we expect based on our datamodels
	ExpectedSchemas []string
	// RegistrySchemas contains all schema names found in the registry
	RegistrySchemas []string
}

// GenerateExpectedSchemaNames creates expected schema names from datamodels
// Schema name format: <DataModelsConfig.Name>_<Version>_<datamodeltype>-<data_type>
func GenerateExpectedSchemaNames(dataModels map[string]config.DataModelsConfig) []string {
	// Pre-allocate slice with estimated capacity (models * versions * 3 avg data types)
	estimatedCapacity := len(dataModels) * 3 * 3
	expectedSchemas := make([]string, 0, estimatedCapacity)

	// Reuse a single map to reduce allocations
	dataTypes := make(map[string]bool, 3) // Pre-allocate for common case of 3 data types

	// Reuse string builder for schema name construction
	var schemaNameBuilder strings.Builder
	schemaNameBuilder.Grow(64) // Pre-allocate buffer for typical schema name length

	for _, dataModel := range dataModels {
		for versionKey, version := range dataModel.Versions {
			// Clear map more efficiently - recreate if it gets too large
			if len(dataTypes) > 10 {
				dataTypes = make(map[string]bool, 3)
			} else {
				// Clear existing map
				for k := range dataTypes {
					delete(dataTypes, k)
				}
			}

			// Collect unique data types for this model version
			collectTimeseriesDataTypes(version.Structure, dataTypes)

			// Generate schema names for each unique data type
			for dataType := range dataTypes {
				// Reset and build schema name efficiently
				schemaNameBuilder.Reset()
				schemaNameBuilder.WriteString(dataModel.Name)
				schemaNameBuilder.WriteByte('_')
				schemaNameBuilder.WriteString(versionKey)
				schemaNameBuilder.WriteString("_timeseries-")
				schemaNameBuilder.WriteString(dataType)

				expectedSchemas = append(expectedSchemas, schemaNameBuilder.String())
			}
		}
	}

	return expectedSchemas
}

// collectTimeseriesDataTypes recursively collects unique timeseries data types from field structure
func collectTimeseriesDataTypes(structure map[string]config.Field, dataTypes map[string]bool) {
	for _, field := range structure {
		// Check for specific timeseries types more efficiently
		switch field.Type {
		case "timeseries-number":
			dataTypes["number"] = true
		case "timeseries-string":
			dataTypes["string"] = true
		case "timeseries-boolean":
			dataTypes["boolean"] = true
		}

		// Recursively process subfields
		if len(field.Subfields) > 0 {
			collectTimeseriesDataTypes(field.Subfields, dataTypes)
		}
	}
}

// CompareDataModelsWithRegistry compares our datamodels against registry schemas
func (s *RedpandaService) CompareDataModelsWithRegistry(ctx context.Context, dataModels map[string]config.DataModelsConfig) (*DataModelSchemaMapping, error) {
	// Get all schemas from registry
	registrySchemas, err := s.GetAllSchemas(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch registry schemas: %w", err)
	}

	// Generate expected schema names from our datamodels
	expectedSchemas := GenerateExpectedSchemaNames(dataModels)

	// Convert registry schemas to string slice for easier comparison
	registrySchemaNames := make([]string, len(registrySchemas))
	for i, schema := range registrySchemas {
		registrySchemaNames[i] = schema.Subject
	}

	// Find missing schemas (expected but not in registry)
	var missingInRegistry []string
	registrySchemaSet := make(map[string]bool)
	for _, schema := range registrySchemaNames {
		registrySchemaSet[schema] = true
	}

	for _, expected := range expectedSchemas {
		if !registrySchemaSet[expected] {
			missingInRegistry = append(missingInRegistry, expected)
		}
	}

	// Find orphaned schemas (in registry but not expected)
	var orphanedInRegistry []string
	expectedSchemaSet := make(map[string]bool)
	for _, expected := range expectedSchemas {
		expectedSchemaSet[expected] = true
	}

	for _, registry := range registrySchemaNames {
		if !expectedSchemaSet[registry] {
			orphanedInRegistry = append(orphanedInRegistry, registry)
		}
	}

	return &DataModelSchemaMapping{
		MissingInRegistry:  missingInRegistry,
		OrphanedInRegistry: orphanedInRegistry,
		ExpectedSchemas:    expectedSchemas,
		RegistrySchemas:    registrySchemaNames,
	}, nil
}
