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
	"strings"
	"sync"
)

// MockSchemaRegistry simulates a Redpanda Schema Registry for testing
type MockSchemaRegistry struct {
	// subjects stores the schema subjects by subject name
	subjects map[string]MockSubject
	// mutex protects concurrent access to subjects
	mutex sync.RWMutex
	// basePort is the port the mock registry is running on
	basePort int
	// server is the HTTP server
	server *httptest.Server
	// deleteHistory tracks deleted subjects for testing
	deleteHistory []string
	// registrationHistory tracks registered subjects for testing
	registrationHistory []MockSubject
}

// MockSubject represents a schema subject in the mock registry
type MockSubject struct {
	// Name is the subject name
	Name string
	// Schema is the schema content
	Schema string
	// SchemaType is the type of schema (JSON, AVRO, etc.)
	SchemaType string
	// ID is the schema ID
	ID int
	// Version is the schema version
	Version int
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
		subjects: make(map[string]MockSubject),
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
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Get current schema if it exists to determine new ID
	var newID int
	if existingSchema, exists := m.subjects[subject]; exists {
		newID = existingSchema.ID + 1
	} else {
		newID = len(m.subjects) + 1
	}

	newSubject := MockSubject{
		Name:       subject,
		Schema:     schema,
		SchemaType: "JSON", // Default to JSON
		ID:         newID,
		Version:    1, // Always version 1 for simplicity
	}

	m.subjects[subject] = newSubject
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

	m.mutex.RLock()
	defer m.mutex.RUnlock()

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

	m.mutex.RLock()
	defer m.mutex.RUnlock()

	schema, exists := m.subjects[subject]
	if !exists {
		http.Error(w, "Subject not found", http.StatusNotFound)
		return
	}

	// Return versions list (always just version 1 for simplicity)
	versions := []int{schema.Version}

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
	m.mutex.RLock()
	schema, exists := m.subjects[subject]
	if !exists {
		m.mutex.RUnlock()
		http.Error(w, "Subject not found", http.StatusNotFound)
		return
	}
	m.mutex.RUnlock()

	response := map[string]interface{}{
		"id": schema.ID,
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

func (m *MockSchemaRegistry) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	// Handle preflight requests
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch r.Method {
	case "GET":
		m.handleGet(w, r)
	case "POST":
		m.handlePost(w, r)
	case "DELETE":
		m.handleDelete(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		if err := json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"}); err != nil {
			fmt.Printf("Failed to encode error response: %v\n", err)
		}
	}
}

// handleGet handles GET requests (existing functionality)
func (m *MockSchemaRegistry) handleGet(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/subjects" {
		// Return list of subjects
		subjects := make([]string, 0, len(m.subjects))
		for name := range m.subjects {
			subjects = append(subjects, name)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(subjects); err != nil {
			fmt.Printf("Failed to encode subjects response: %v\n", err)
		}
		return
	}

	// Handle other GET requests (not implemented for this testing)
	w.WriteHeader(http.StatusNotFound)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": "not found"}); err != nil {
		fmt.Printf("Failed to encode error response: %v\n", err)
	}
}

// handlePost handles POST requests for registering schemas
func (m *MockSchemaRegistry) handlePost(w http.ResponseWriter, r *http.Request) {
	// Parse URL path: /subjects/{subject}/versions
	path := strings.TrimPrefix(r.URL.Path, "/subjects/")
	if !strings.HasSuffix(path, "/versions") {
		w.WriteHeader(http.StatusNotFound)
		if err := json.NewEncoder(w).Encode(map[string]string{"error": "invalid path"}); err != nil {
			fmt.Printf("Failed to encode error response: %v\n", err)
		}
		return
	}

	subjectName := strings.TrimSuffix(path, "/versions")
	if subjectName == "" {
		w.WriteHeader(http.StatusBadRequest)
		if err := json.NewEncoder(w).Encode(map[string]string{"error": "subject name required"}); err != nil {
			fmt.Printf("Failed to encode error response: %v\n", err)
		}
		return
	}

	// Parse request body
	var req SchemaRegistrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		if err := json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"}); err != nil {
			fmt.Printf("Failed to encode error response: %v\n", err)
		}
		return
	}

	// Generate new ID
	newID := len(m.subjects) + 1
	for _, subject := range m.subjects {
		if subject.ID >= newID {
			newID = subject.ID + 1
		}
	}

	// Create new subject
	newSubject := MockSubject{
		Name:       subjectName,
		Schema:     req.Schema,
		SchemaType: req.SchemaType,
		ID:         newID,
		Version:    1, // Always version 1 for simplicity
	}

	// Store the subject
	m.subjects[subjectName] = newSubject

	// Track registration history
	m.registrationHistory = append(m.registrationHistory, newSubject)

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(SchemaRegistrationResponse{ID: newID}); err != nil {
		fmt.Printf("Failed to encode registration response: %v\n", err)
	}
}

// handleDelete handles DELETE requests for removing schemas
func (m *MockSchemaRegistry) handleDelete(w http.ResponseWriter, r *http.Request) {
	// Parse URL path: /subjects/{subject}
	path := strings.TrimPrefix(r.URL.Path, "/subjects/")
	if path == "" {
		w.WriteHeader(http.StatusBadRequest)
		if err := json.NewEncoder(w).Encode(map[string]string{"error": "subject name required"}); err != nil {
			fmt.Printf("Failed to encode error response: %v\n", err)
		}
		return
	}

	subjectName := path

	// Check if subject exists
	subject, exists := m.subjects[subjectName]
	if !exists {
		w.WriteHeader(http.StatusNotFound)
		if err := json.NewEncoder(w).Encode(map[string]string{"error": "subject not found"}); err != nil {
			fmt.Printf("Failed to encode error response: %v\n", err)
		}
		return
	}

	// Delete the subject
	delete(m.subjects, subjectName)

	// Track deletion history
	m.deleteHistory = append(m.deleteHistory, subjectName)

	// Return success response with deleted versions
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode([]int{subject.Version}); err != nil {
		fmt.Printf("Failed to encode deletion response: %v\n", err)
	}
}

// GetDeleteHistory returns the history of deleted subjects
func (m *MockSchemaRegistry) GetDeleteHistory() []string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Return a copy to prevent data races
	history := make([]string, len(m.deleteHistory))
	copy(history, m.deleteHistory)
	return history
}

// GetRegistrationHistory returns the history of registered subjects
func (m *MockSchemaRegistry) GetRegistrationHistory() []MockSubject {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Return a copy to prevent data races
	history := make([]MockSubject, len(m.registrationHistory))
	copy(history, m.registrationHistory)
	return history
}

// ClearHistory clears the deletion and registration history
func (m *MockSchemaRegistry) ClearHistory() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.deleteHistory = nil
	m.registrationHistory = nil
}

// GetSubjects returns a copy of all subjects
func (m *MockSchemaRegistry) GetSubjects() map[string]MockSubject {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Return a copy to prevent data races
	subjects := make(map[string]MockSubject)
	for name, subject := range m.subjects {
		subjects[name] = subject
	}
	return subjects
}
