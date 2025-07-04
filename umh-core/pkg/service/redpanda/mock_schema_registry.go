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
	"net/http"
	"net/http/httptest"
	"sync"
)

// MockSchemaRegistry represents a mock schema registry server for testing
type MockSchemaRegistry struct {
	server   *httptest.Server
	subjects map[string][]MockSchema
	mu       sync.RWMutex
}

// MockSchema represents a schema in the mock registry
type MockSchema struct {
	ID      int    `json:"id"`
	Version int    `json:"version"`
	Schema  string `json:"schema"`
	Subject string `json:"subject"`
}

// NewMockSchemaRegistry creates a new mock schema registry server
func NewMockSchemaRegistry() *MockSchemaRegistry {
	mock := &MockSchemaRegistry{
		subjects: make(map[string][]MockSchema),
	}

	// Set up the HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/subjects", mock.handleGetSubjects)
	mux.HandleFunc("/subjects/", mock.handleSubjectOperations)
	mux.HandleFunc("/schemas/ids/", mock.handleGetSchemaByID)
	mux.HandleFunc("/config", mock.handleConfig)
	mux.HandleFunc("/config/", mock.handleSubjectConfig)

	mock.server = httptest.NewServer(mux)
	return mock
}

// URL returns the base URL of the mock schema registry
func (m *MockSchemaRegistry) URL() string {
	return m.server.URL
}

// Close shuts down the mock schema registry server
func (m *MockSchemaRegistry) Close() {
	m.server.Close()
}

// AddSchema adds a schema to the mock registry
func (m *MockSchemaRegistry) AddSchema(subject string, schema string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	schemas := m.subjects[subject]
	id := len(schemas) + 1
	version := len(schemas) + 1

	newSchema := MockSchema{
		ID:      id,
		Version: version,
		Schema:  schema,
		Subject: subject,
	}

	m.subjects[subject] = append(schemas, newSchema)
}

// AddTestSchemas adds some test schemas for common testing scenarios
func (m *MockSchemaRegistry) AddTestSchemas() {
	// Add some test schemas based on the expected naming convention
	testSchemas := map[string]string{
		"_pump_v1_timeseries-number": `{
			"type": "record",
			"name": "pump_v1_timeseries_number",
			"fields": [
				{"name": "value", "type": "double"},
				{"name": "timestamp", "type": "long"}
			]
		}`,
		"_pump_v1_timeseries-string": `{
			"type": "record",
			"name": "pump_v1_timeseries_string",
			"fields": [
				{"name": "value", "type": "string"},
				{"name": "timestamp", "type": "long"}
			]
		}`,
		"_pump_v1_timeseries-boolean": `{
			"type": "record",
			"name": "pump_v1_timeseries_boolean",
			"fields": [
				{"name": "value", "type": "boolean"},
				{"name": "timestamp", "type": "long"}
			]
		}`,
		"_motor_v2_timeseries-number": `{
			"type": "record",
			"name": "motor_v2_timeseries_number",
			"fields": [
				{"name": "value", "type": "double"},
				{"name": "timestamp", "type": "long"}
			]
		}`,
		"_orphaned_v1_timeseries-boolean": `{
			"type": "record",
			"name": "orphaned_v1_timeseries_boolean",
			"fields": [
				{"name": "value", "type": "boolean"},
				{"name": "timestamp", "type": "long"}
			]
		}`,
	}

	for subject, schema := range testSchemas {
		m.AddSchema(subject, schema)
	}
}

// HTTP handlers

func (m *MockSchemaRegistry) handleGetSubjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	var subjects []string
	for subject := range m.subjects {
		subjects = append(subjects, subject)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(subjects); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func (m *MockSchemaRegistry) handleSubjectOperations(w http.ResponseWriter, r *http.Request) {
	// Parse the subject from the URL path
	// URL format: /subjects/{subject}/versions/{version}
	path := r.URL.Path
	if len(path) < 10 { // "/subjects/" is 10 characters
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	// Simple path parsing - in a real implementation this would be more robust
	subjectPath := path[10:] // Remove "/subjects/"

	switch r.Method {
	case http.MethodGet:
		m.handleGetSubjectVersions(w, r, subjectPath)
	case http.MethodPost:
		m.handleRegisterSchema(w, r, subjectPath)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (m *MockSchemaRegistry) handleGetSubjectVersions(w http.ResponseWriter, r *http.Request, subjectPath string) {
	// For simplicity, just return the latest version info
	subject := subjectPath
	if idx := len(subjectPath) - 9; idx > 0 && subjectPath[idx:] == "/versions" {
		subject = subjectPath[:idx]
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	schemas, exists := m.subjects[subject]
	if !exists {
		http.Error(w, "Subject not found", http.StatusNotFound)
		return
	}

	// Return versions list
	var versions []int
	for _, schema := range schemas {
		versions = append(versions, schema.Version)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(versions); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func (m *MockSchemaRegistry) handleRegisterSchema(w http.ResponseWriter, r *http.Request, subjectPath string) {
	// Parse schema registration request
	var req struct {
		Schema string `json:"schema"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	subject := subjectPath
	if idx := len(subjectPath) - 9; idx > 0 && subjectPath[idx:] == "/versions" {
		subject = subjectPath[:idx]
	}

	m.AddSchema(subject, req.Schema)

	// Return the schema ID
	m.mu.RLock()
	schemas, exists := m.subjects[subject]
	if !exists || len(schemas) == 0 {
		m.mu.RUnlock()
		http.Error(w, "Subject not found", http.StatusNotFound)
		return
	}
	latestSchema := schemas[len(schemas)-1]
	m.mu.RUnlock()

	response := map[string]interface{}{
		"id": latestSchema.ID,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func (m *MockSchemaRegistry) handleGetSchemaByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// In a real implementation, we'd parse the ID from the URL
	// For now, just return a generic response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"schema": `{"type": "string"}`,
	}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func (m *MockSchemaRegistry) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Return default config
	config := map[string]interface{}{
		"compatibilityLevel": "BACKWARD",
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(config); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func (m *MockSchemaRegistry) handleSubjectConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Return subject-specific config
	config := map[string]interface{}{
		"compatibilityLevel": "BACKWARD",
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(config); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// TestWithMockSchemaRegistry is a helper function for running tests with a mock schema registry
func TestWithMockSchemaRegistry(ctx context.Context, testFunc func(registryURL string)) error {
	mock := NewMockSchemaRegistry()
	defer mock.Close()

	// Add test schemas
	mock.AddTestSchemas()

	// Run the test function with the mock registry URL
	testFunc(mock.URL())

	return nil
}

// GetMockSchemaRegistryURL returns the URL of a mock schema registry for testing
func GetMockSchemaRegistryURL(mockRegistry *MockSchemaRegistry) string {
	return mockRegistry.URL()
}
