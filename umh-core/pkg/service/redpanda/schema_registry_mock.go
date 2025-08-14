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
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
)

// MockSchemaRegistry simulates Redpanda's Schema Registry for testing.
type MockSchemaRegistry struct {
	server  *httptest.Server
	schemas map[string]map[int]*MockSchemaVersion // subject -> version -> schema
}

// MockSchemaVersion represents a schema version in the mock registry.
type MockSchemaVersion struct {
	Schema  string `json:"schema"`
	Subject string `json:"subject"`
	ID      int    `json:"id"`
	Version int    `json:"version"`
}

// NoOpSchemaRegistry is a mock implementation that does nothing.
type NoOpSchemaRegistry struct{}

// NewMockSchemaRegistry creates a new mock schema registry server.
func NewMockSchemaRegistry() *MockSchemaRegistry {
	mock := &MockSchemaRegistry{
		schemas: make(map[string]map[int]*MockSchemaVersion),
	}

	// Create HTTP server with Redpanda-compatible API
	mux := http.NewServeMux()
	mux.HandleFunc("/subjects", mock.handleSubjects)
	mux.HandleFunc("/subjects/", mock.handleSubjectOperations)

	mock.server = httptest.NewServer(mux)

	return mock
}

// NewNoOpSchemaRegistry creates a new no-op schema registry for testing.
func NewNoOpSchemaRegistry() *NoOpSchemaRegistry {
	return &NoOpSchemaRegistry{}
}

// URL returns the base URL of the mock server.
func (m *MockSchemaRegistry) URL() string {
	return m.server.URL
}

// Close shuts down the mock server.
func (m *MockSchemaRegistry) Close() {
	m.server.Close()
}

// AddSchema adds a schema to the mock registry.
func (m *MockSchemaRegistry) AddSchema(subject string, version int, schema string) {
	if m.schemas[subject] == nil {
		m.schemas[subject] = make(map[int]*MockSchemaVersion)
	}

	// Generate a unique ID (simple incrementing for mock)
	id := len(m.schemas) + version*1000

	m.schemas[subject][version] = &MockSchemaVersion{
		ID:      id,
		Version: version,
		Schema:  schema,
		Subject: subject,
	}
}

// RemoveSchema removes a schema from the mock registry.
func (m *MockSchemaRegistry) RemoveSchema(subject string, version int) {
	if versions, exists := m.schemas[subject]; exists {
		delete(versions, version)

		if len(versions) == 0 {
			delete(m.schemas, subject)
		}
	}
}

// GetSchema gets a schema from the mock registry.
func (m *MockSchemaRegistry) GetSchema(subject string, version int) *MockSchemaVersion {
	if versions, exists := m.schemas[subject]; exists {
		return versions[version]
	}

	return nil
}

// ClearSchemas removes all schemas from the mock registry.
func (m *MockSchemaRegistry) ClearSchemas() {
	m.schemas = make(map[string]map[int]*MockSchemaVersion)
}

// handleSubjects handles GET /subjects - returns all available subjects.
func (m *MockSchemaRegistry) handleSubjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)

		return
	}

	// Collect all subjects
	subjects := make([]string, 0, len(m.schemas))
	for subject := range m.schemas {
		subjects = append(subjects, subject)
	}

	w.Header().Set("Content-Type", "application/json")

	err := json.NewEncoder(w).Encode(subjects)
	if err != nil {
		http.Error(w, "Failed to encode subjects", http.StatusInternalServerError)

		return
	}
}

// handleSubjectOperations handles all operations under /subjects/{subject}/...
func (m *MockSchemaRegistry) handleSubjectOperations(w http.ResponseWriter, r *http.Request) {
	// Parse the URL path: /subjects/{subject} or /subjects/{subject}/versions
	path := strings.TrimPrefix(r.URL.Path, "/subjects/")
	parts := strings.Split(path, "/")

	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Invalid path", http.StatusBadRequest)

		return
	}

	subject := parts[0]

	switch r.Method {
	case http.MethodDelete:
		// DELETE /subjects/{subject}
		if len(parts) == 1 {
			m.handleDeleteSubject(w, r, subject)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	case http.MethodPost:
		// POST /subjects/{subject}/versions
		if len(parts) == 2 && parts[1] == "versions" {
			m.handlePostSubjectVersion(w, r, subject)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	case http.MethodGet:
		// GET /subjects/{subject}/versions/{version}
		m.handleGetSubjectVersions(w, r, subject, parts)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleDeleteSubject handles DELETE /subjects/{subject}.
func (m *MockSchemaRegistry) handleDeleteSubject(w http.ResponseWriter, r *http.Request, subject string) {
	_ = r // HTTP request parameter required by handler interface

	// Check if subject exists
	if _, exists := m.schemas[subject]; !exists {
		// Subject not found
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprintf(w, `{"error_code":40401,"message":"Subject '%s' not found."}`, subject)

		return
	}

	// Delete the entire subject
	delete(m.schemas, subject)

	// Return success with version list (what Redpanda returns)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`[1]`)) // Mock version list
}

// handlePostSubjectVersion handles POST /subjects/{subject}/versions.
func (m *MockSchemaRegistry) handlePostSubjectVersion(w http.ResponseWriter, r *http.Request, subject string) {
	// Parse JSON body
	var req struct {
		Schema     string `json:"schema"`
		SchemaType string `json:"schemaType"`
	}

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error_code":42201,"message":"Invalid JSON"}`))

		return
	}

	// Check if schema already exists (simulate conflict detection)
	if versions, exists := m.schemas[subject]; exists {
		for _, schema := range versions {
			if schema.Schema == req.Schema {
				// Schema already exists, return 409 Conflict
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusConflict)
				_, _ = fmt.Fprintf(w, `{"id":%d}`, schema.ID)

				return
			}
		}
	}

	// Add new schema version
	version := 1

	if versions, exists := m.schemas[subject]; exists {
		// Find next version number
		for v := range versions {
			if v >= version {
				version = v + 1
			}
		}
	} else {
		m.schemas[subject] = make(map[int]*MockSchemaVersion)
	}

	// Generate unique ID
	id := len(m.schemas)*1000 + version

	m.schemas[subject][version] = &MockSchemaVersion{
		ID:      id,
		Version: version,
		Schema:  req.Schema,
		Subject: subject,
	}

	// Return success with schema ID
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_, _ = fmt.Fprintf(w, `{"id":%d}`, id)
}

// handleGetSubjectVersions handles GET /subjects/{subject}/versions/{version}.
func (m *MockSchemaRegistry) handleGetSubjectVersions(w http.ResponseWriter, r *http.Request, subject string, parts []string) {
	_ = r // HTTP request parameter required by handler interface

	if len(parts) < 3 || parts[1] != "versions" {
		http.Error(w, "Invalid path", http.StatusBadRequest)

		return
	}

	versionStr := parts[2]

	// Handle "latest" version request
	if versionStr == "latest" {
		m.handleLatestVersion(w, subject)

		return
	}

	// Parse version number
	version, err := strconv.Atoi(versionStr)
	if err != nil {
		http.Error(w, "Invalid version format", http.StatusBadRequest)

		return
	}

	// Check if subject exists
	versions, subjectExists := m.schemas[subject]
	if !subjectExists {
		http.Error(w, fmt.Sprintf(`{"error_code":40401,"message":"Subject '%s' not found."}`, subject), http.StatusNotFound)

		return
	}

	// Check if version exists
	schema, versionExists := versions[version]
	if !versionExists {
		http.Error(w, fmt.Sprintf(`{"error_code":40402,"message":"Version %d not found for subject '%s'."}`, version, subject), http.StatusNotFound)

		return
	}

	// Return the schema version (Redpanda format)
	w.Header().Set("Content-Type", "application/json")

	err = json.NewEncoder(w).Encode(schema)
	if err != nil {
		http.Error(w, "Failed to encode schema", http.StatusInternalServerError)

		return
	}
}

// handleLatestVersion handles requests for the latest version of a subject.
func (m *MockSchemaRegistry) handleLatestVersion(w http.ResponseWriter, subject string) {
	versions, subjectExists := m.schemas[subject]
	if !subjectExists {
		http.Error(w, fmt.Sprintf(`{"error_code":40401,"message":"Subject '%s' not found."}`, subject), http.StatusNotFound)

		return
	}

	// Find the latest version
	var (
		latestVersion int
		latestSchema  *MockSchemaVersion
	)

	for version, schema := range versions {
		if version > latestVersion {
			latestVersion = version
			latestSchema = schema
		}
	}

	if latestSchema == nil {
		http.Error(w, fmt.Sprintf(`{"error_code":40402,"message":"No versions found for subject '%s'."}`, subject), http.StatusNotFound)

		return
	}

	w.Header().Set("Content-Type", "application/json")

	err := json.NewEncoder(w).Encode(latestSchema)
	if err != nil {
		http.Error(w, "Failed to encode latest schema", http.StatusInternalServerError)

		return
	}
}

// SetupTestSchemas adds common test schemas to the mock registry.
func (m *MockSchemaRegistry) SetupTestSchemas() {
	// Add sensor data schemas v1 with different data types
	sensorDataV1NumberSchema := `{
		"type": "object",
		"properties": {
			"virtual_path": {
				"type": "string",
				"enum": ["temperature"]
			},
			"fields": {
				"type": "object",
				"properties": {
					"timestamp_ms": {"type": "number"},
					"value": {"type": "number"}
				},
				"required": ["timestamp_ms", "value"],
				"additionalProperties": false
			}
		},
		"required": ["virtual_path", "fields"],
		"additionalProperties": false
	}`
	m.AddSchema("_sensor_data_v1_timeseries-number", 1, sensorDataV1NumberSchema)

	// Add sensor data schemas v2 with expanded virtual paths and number type
	sensorDataV2NumberSchema := `{
		"type": "object",
		"properties": {
			"virtual_path": {
				"type": "string",
				"enum": ["temperature", "humidity", "pressure"]
			},
			"fields": {
				"type": "object",
				"properties": {
					"timestamp_ms": {"type": "number"},
					"value": {"type": "number"}
				},
				"required": ["timestamp_ms", "value"],
				"additionalProperties": false
			}
		},
		"required": ["virtual_path", "fields"],
		"additionalProperties": false
	}`
	m.AddSchema("_sensor_data_v2_timeseries-number", 2, sensorDataV2NumberSchema)

	// Add pump data schemas v1 with different data types
	pumpDataV1NumberSchema := `{
		"type": "object",
		"properties": {
			"virtual_path": {
				"type": "string",
				"enum": ["vibration.x-axis", "vibration.y-axis", "count"]
			},
			"fields": {
				"type": "object",
				"properties": {
					"timestamp_ms": {"type": "number"},
					"value": {"type": "number"}
				},
				"required": ["timestamp_ms", "value"],
				"additionalProperties": false
			}
		},
		"required": ["virtual_path", "fields"],
		"additionalProperties": false
	}`
	m.AddSchema("_pump_data_v1_timeseries-number", 1, pumpDataV1NumberSchema)

	// Add pump data string schema for serial numbers
	pumpDataV1StringSchema := `{
		"type": "object",
		"properties": {
			"virtual_path": {
				"type": "string",
				"enum": ["serialNumber", "status"]
			},
			"fields": {
				"type": "object",
				"properties": {
					"timestamp_ms": {"type": "number"},
					"value": {"type": "string"}
				},
				"required": ["timestamp_ms", "value"],
				"additionalProperties": false
			}
		},
		"required": ["virtual_path", "fields"],
		"additionalProperties": false
	}`
	m.AddSchema("_pump_data_v1_timeseries-string", 1, pumpDataV1StringSchema)

	// Add motor controller schemas v3 (skipping v1 and v2 to test version gaps)
	motorDataV3NumberSchema := `{
		"type": "object",
		"properties": {
			"virtual_path": {
				"type": "string",
				"enum": ["rpm", "temperature", "status"]
			},
			"fields": {
				"type": "object",
				"properties": {
					"timestamp_ms": {"type": "number"},
					"value": {"type": "number"}
				},
				"required": ["timestamp_ms", "value"],
				"additionalProperties": false
			}
		},
		"required": ["virtual_path", "fields"],
		"additionalProperties": false
	}`
	m.AddSchema("_motor_controller_v3_timeseries-number", 3, motorDataV3NumberSchema)

	// Add motor controller string schema for status
	motorDataV3StringSchema := `{
		"type": "object",
		"properties": {
			"virtual_path": {
				"type": "string",
				"enum": ["status", "mode"]
			},
			"fields": {
				"type": "object",
				"properties": {
					"timestamp_ms": {"type": "number"},
					"value": {"type": "string"}
				},
				"required": ["timestamp_ms", "value"],
				"additionalProperties": false
			}
		},
		"required": ["virtual_path", "fields"],
		"additionalProperties": false
	}`
	m.AddSchema("_motor_controller_v3_timeseries-string", 3, motorDataV3StringSchema)

	// Add string data schemas v1 for testing different data types
	stringDataV1Schema := `{
		"type": "object",
		"properties": {
			"virtual_path": {
				"type": "string",
				"enum": ["serialNumber", "status"]
			},
			"fields": {
				"type": "object",
				"properties": {
					"value": {
						"type": "object",
						"properties": {
							"timestamp_ms": {"type": "number"},
							"value": {"type": "string"}
						},
						"required": ["timestamp_ms", "value"],
						"additionalProperties": false
					}
				},
				"additionalProperties": false
			}
		},
		"required": ["virtual_path", "fields"],
		"additionalProperties": false
	}`
	m.AddSchema("_string_data_v1_timeseries-string", 1, stringDataV1Schema)
}

// SimulateNetworkError makes the mock server return 500 errors for testing.
func (m *MockSchemaRegistry) SimulateNetworkError(enable bool) {
	if enable {
		// Replace handlers with error handlers
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		})
		m.server.Config.Handler = mux
	} else {
		// Restore normal handlers
		mux := http.NewServeMux()
		mux.HandleFunc("/subjects", m.handleSubjects)
		mux.HandleFunc("/subjects/", m.handleSubjectOperations)
		m.server.Config.Handler = mux
	}
}

// GetRegisteredSubjects returns all subjects currently in the mock registry.
func (m *MockSchemaRegistry) GetRegisteredSubjects() []string {
	subjects := make([]string, 0, len(m.schemas))
	for subject := range m.schemas {
		subjects = append(subjects, subject)
	}

	return subjects
}

// GetVersionsForSubject returns all versions for a given subject.
func (m *MockSchemaRegistry) GetVersionsForSubject(subject string) []int {
	var versions []int

	if subjectVersions, exists := m.schemas[subject]; exists {
		for version := range subjectVersions {
			versions = append(versions, version)
		}
	}

	return versions
}

func (n *NoOpSchemaRegistry) Reconcile(ctx context.Context, dataModels []config.DataModelsConfig, dataContracts []config.DataContractsConfig, payloadShapes map[string]config.PayloadShape) error {
	return nil
}

func (n *NoOpSchemaRegistry) GetMetrics() SchemaRegistryMetrics {
	return SchemaRegistryMetrics{}
}
