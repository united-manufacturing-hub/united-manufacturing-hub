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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Type aliases for improved type safety and documentation
type SubjectName string
type JSONSchemaDefinition string
type SchemaRegistryPhase string

// Metrics for observability
type SchemaRegistryMetrics struct {
	CurrentPhase         SchemaRegistryPhase
	TotalReconciliations int64
	SuccessfulOperations int64
	FailedOperations     int64
	SubjectsToAdd        int
	SubjectsToRemove     int
	LastOperationTime    time.Time
	LastError            string
}

// ISchemaRegistry defines the interface for schema registry operations
type ISchemaRegistry interface {
	Reconcile(ctx context.Context, expectedSubjects map[SubjectName]JSONSchemaDefinition) error
	GetMetrics() SchemaRegistryMetrics
}

type SchemaRegistry struct {
	// Concurrency protection
	mu sync.RWMutex

	// Core state
	currentPhase SchemaRegistryPhase
	httpClient   http.Client

	// Phase-specific data
	rawSubjectsData  []byte        // Raw HTTP response from lookup
	registrySubjects []SubjectName // Decoded subjects from registry

	// Comparison results (populated during reconcile with current expectedSubjects)
	missingInRegistry           map[SubjectName]JSONSchemaDefinition // Subject -> schema (we have, registry doesn't)
	inRegistryButUnknownLocally map[SubjectName]bool                 // Registry has, we don't expect

	// Operation tracking and metrics
	currentOperationSubject SubjectName // Which subject being processed
	totalReconciliations    int64
	successfulOperations    int64
	failedOperations        int64
	lastOperationTime       time.Time
	lastError               string
}

const (
	SchemaRegistryPhaseLookup        SchemaRegistryPhase = "lookup"
	SchemaRegistryPhaseDecode        SchemaRegistryPhase = "decode"
	SchemaRegistryPhaseCompare       SchemaRegistryPhase = "compare"
	SchemaRegistryPhaseRemoveUnknown SchemaRegistryPhase = "remove_unknown"
	SchemaRegistryPhaseAddNew        SchemaRegistryPhase = "add_new"
)

// Context timeout requirements per phase
const (
	MinimumLookupTime  = 10 * time.Millisecond // HTTP GET /subjects (local)
	MinimumDecodeTime  = 1 * time.Millisecond  // JSON parsing (fast)
	MinimumCompareTime = 1 * time.Millisecond  // Map operations (instant)
	MinimumRemoveTime  = 10 * time.Millisecond // HTTP DELETE (local)
	MinimumAddTime     = 15 * time.Millisecond // HTTP POST with schema (local, slightly larger payload)
)

const SchemaRegistryAddress = "localhost:8081"

func NewSchemaRegistry() *SchemaRegistry {
	return &SchemaRegistry{
		currentPhase:                SchemaRegistryPhaseLookup,
		httpClient:                  http.Client{},
		missingInRegistry:           make(map[SubjectName]JSONSchemaDefinition),
		inRegistryButUnknownLocally: make(map[SubjectName]bool),
		totalReconciliations:        0,
		successfulOperations:        0,
		failedOperations:            0,
		lastOperationTime:           time.Time{},
		lastError:                   "",
	}
}

func (s *SchemaRegistry) Reconcile(ctx context.Context, expectedSubjects map[SubjectName]JSONSchemaDefinition) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.totalReconciliations++

	var err error
	defer func() {
		s.lastOperationTime = time.Now()
		if err != nil {
			s.failedOperations++
			s.lastError = err.Error()
		} else {
			s.successfulOperations++
			s.lastError = ""
		}
	}()

	err, _ = s.reconcileInternal(ctx, expectedSubjects)
	return err
}

// getNextPhase calculates the next phase based on current phase and whether to change phase
func (s *SchemaRegistry) getNextPhase(currentPhase SchemaRegistryPhase, changePhase bool) SchemaRegistryPhase {
	if !changePhase {
		return currentPhase // Stay in same phase
	}

	switch currentPhase {
	case SchemaRegistryPhaseLookup:
		return SchemaRegistryPhaseDecode
	case SchemaRegistryPhaseDecode:
		return SchemaRegistryPhaseCompare
	case SchemaRegistryPhaseCompare:
		// Decision logic based on what needs to be done
		if len(s.inRegistryButUnknownLocally) > 0 {
			return SchemaRegistryPhaseRemoveUnknown
		} else if len(s.missingInRegistry) > 0 {
			return SchemaRegistryPhaseAddNew
		} else {
			return SchemaRegistryPhaseLookup // Fully in sync, restart cycle
		}
	case SchemaRegistryPhaseRemoveUnknown:
		// Check if more work to do in this phase
		if len(s.inRegistryButUnknownLocally) > 0 {
			return SchemaRegistryPhaseRemoveUnknown // Stay in same phase
		} else {
			return SchemaRegistryPhaseAddNew
		}
	case SchemaRegistryPhaseAddNew:
		// Check if more work to do in this phase
		if len(s.missingInRegistry) > 0 {
			return SchemaRegistryPhaseAddNew // Stay in same phase
		} else {
			return SchemaRegistryPhaseLookup // Start new cycle
		}
	default:
		return SchemaRegistryPhaseLookup // Default fallback
	}
}

func (s *SchemaRegistry) GetMetrics() SchemaRegistryMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return SchemaRegistryMetrics{
		CurrentPhase:         s.currentPhase,
		TotalReconciliations: s.totalReconciliations,
		SuccessfulOperations: s.successfulOperations,
		FailedOperations:     s.failedOperations,
		SubjectsToAdd:        len(s.missingInRegistry),
		SubjectsToRemove:     len(s.inRegistryButUnknownLocally),
		LastOperationTime:    s.lastOperationTime,
		LastError:            s.lastError,
	}
}

func (s *SchemaRegistry) reconcileInternal(ctx context.Context, expectedSubjects map[SubjectName]JSONSchemaDefinition) (err error, reconciled bool) {
	// Run through phases until we complete the reconciliation cycle or hit an error
	for {
		// Early abort if context is done
		if ctx.Err() != nil {
			return ctx.Err(), false
		}

		var changePhase bool
		switch s.currentPhase {
		case SchemaRegistryPhaseLookup:
			err, changePhase = s.lookup(ctx)
		case SchemaRegistryPhaseDecode:
			err, changePhase = s.decode(ctx)
		case SchemaRegistryPhaseCompare:
			err, changePhase = s.compare(ctx, expectedSubjects)
		case SchemaRegistryPhaseRemoveUnknown:
			err, changePhase = s.removeUnknown(ctx)
		case SchemaRegistryPhaseAddNew:
			err, changePhase = s.addNew(ctx)
		default:
			return fmt.Errorf("unknown phase: %s", s.currentPhase), false
		}

		// Handle error
		if err != nil {
			// Update phase if needed before returning
			if changePhase {
				s.currentPhase = s.getNextPhase(s.currentPhase, changePhase)
			}
			return err, false
		}

		// Update phase if requested
		if changePhase {
			s.currentPhase = s.getNextPhase(s.currentPhase, changePhase)

			// Special case: if we're fully in sync after compare, return early
			if s.currentPhase == SchemaRegistryPhaseLookup &&
				len(s.missingInRegistry) == 0 && len(s.inRegistryButUnknownLocally) == 0 {
				return nil, true // Fully in sync, reconciliation complete
			}

			continue // Continue to next phase
		} else {
			// No phase change requested, reconciliation complete for this cycle
			return nil, true
		}
	}
}

func (s *SchemaRegistry) lookup(ctx context.Context) (err error, changePhase bool) {
	// Check if context has enough time remaining
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("context deadline already passed"), false
		}
		if remaining < MinimumLookupTime {
			return fmt.Errorf("insufficient time remaining in context (< %v)", MinimumLookupTime), false
		}
	}

	url := fmt.Sprintf("http://%s/subjects", SchemaRegistryAddress)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err, false
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err, false
	}
	if resp == nil {
		return fmt.Errorf("received nil response from schema registry"), false
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			fmt.Printf("Warning: Failed to close schema registry response body: %v\n", closeErr)
		}
	}()

	// Only HTTP 200 is considered success - all others are transient failures
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("schema registry lookup failed with status %d", resp.StatusCode), false
	}

	// Store raw response bytes for decode phase
	decoder := json.NewDecoder(resp.Body)
	var subjects []string
	if err := decoder.Decode(&subjects); err != nil {
		return err, false
	}

	// We need to re-encode to store as raw bytes for the decode phase
	s.rawSubjectsData, err = json.Marshal(subjects)
	if err != nil {
		return err, false
	}

	return nil, true // Downloaded data, advance to next phase
}

func (s *SchemaRegistry) decode(ctx context.Context) (err error, changePhase bool) {
	// Check if context has enough time remaining
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("context deadline already passed"), false
		}
		if remaining < MinimumDecodeTime {
			return fmt.Errorf("insufficient time remaining in context (< %v)", MinimumDecodeTime), false
		}
	}

	// Parse JSON into temporary string slice, then convert to typed slice
	var subjects []string
	if err := json.Unmarshal(s.rawSubjectsData, &subjects); err != nil {
		return fmt.Errorf("failed to decode subjects: %w", err), false
	}

	// Convert to typed slice
	s.registrySubjects = make([]SubjectName, len(subjects))
	for i, subject := range subjects {
		s.registrySubjects[i] = SubjectName(subject)
	}

	// Clear raw data to free memory
	s.rawSubjectsData = nil

	return nil, true // Parsed data, advance to next phase
}

func (s *SchemaRegistry) compare(ctx context.Context, expectedSubjects map[SubjectName]JSONSchemaDefinition) (err error, changePhase bool) {
	// Check if context has enough time remaining
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("context deadline already passed"), false
		}
		if remaining < MinimumCompareTime {
			return fmt.Errorf("insufficient time remaining in context (< %v)", MinimumCompareTime), false
		}
	}

	// Reset comparison maps
	s.missingInRegistry = make(map[SubjectName]JSONSchemaDefinition)
	s.inRegistryButUnknownLocally = make(map[SubjectName]bool)

	// Convert registry subjects to map for O(1) lookup
	registryMap := make(map[SubjectName]bool)
	for _, subject := range s.registrySubjects {
		registryMap[subject] = true
	}

	// Find missing in registry (we expect, registry doesn't have)
	for subject, schema := range expectedSubjects {
		if !registryMap[subject] {
			s.missingInRegistry[subject] = schema
		}
	}

	// Find unknown in registry (registry has, we don't expect)
	for _, subject := range s.registrySubjects {
		if _, expected := expectedSubjects[subject]; !expected {
			s.inRegistryButUnknownLocally[subject] = true
		}
	}

	return nil, true // Analyzed differences, advance to next phase
}

func (s *SchemaRegistry) removeUnknown(ctx context.Context) (err error, changePhase bool) {
	// Check if context has enough time remaining
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("context deadline already passed"), false
		}
		if remaining < MinimumRemoveTime {
			return fmt.Errorf("insufficient time remaining in context (< %v)", MinimumRemoveTime), false
		}
	}

	// Check if work to do
	if len(s.inRegistryButUnknownLocally) == 0 {
		return nil, true // Nothing to remove, advance to next phase
	}

	// Get first subject to remove
	var subjectToRemove SubjectName
	for subject := range s.inRegistryButUnknownLocally {
		subjectToRemove = subject
		break
	}
	s.currentOperationSubject = subjectToRemove

	// HTTP DELETE /subjects/{subject}
	url := fmt.Sprintf("http://%s/subjects/%s", SchemaRegistryAddress, string(subjectToRemove))
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return err, false
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete subject %s: %w", string(subjectToRemove), err), false
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			fmt.Printf("Warning: Failed to close response body: %v\n", closeErr)
		}
	}()

	// Handle success cases: HTTP 200, 204 (successful deletion)
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
		// Successful deletion - continue to cleanup
	} else if resp.StatusCode == http.StatusNotFound {
		// HTTP 404 - subject already gone, treat as success
	} else if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		// Parse custom error codes for client errors
		respBody, readErr := io.ReadAll(resp.Body)
		if readErr == nil {
			var errorResp map[string]interface{}
			if json.Unmarshal(respBody, &errorResp) == nil {
				if errorCode, ok := errorResp["error_code"].(float64); ok {
					switch int(errorCode) {
					case 40401: // Subject not found
						// Already gone, treat as success
					case 40406: // Already soft-deleted
						// Already deleted, treat as success
					case 42206: // Schema has references
						return fmt.Errorf("cannot delete subject %s: schema has references (error 42206)", string(subjectToRemove)), false
					default:
						return fmt.Errorf("delete subject %s failed with custom error %d", string(subjectToRemove), int(errorCode)), false
					}
				} else {
					return fmt.Errorf("delete subject %s returned client error status %d", string(subjectToRemove), resp.StatusCode), false
				}
			} else {
				return fmt.Errorf("delete subject %s returned client error status %d", string(subjectToRemove), resp.StatusCode), false
			}
		} else {
			return fmt.Errorf("delete subject %s returned client error status %d", string(subjectToRemove), resp.StatusCode), false
		}
	} else {
		// Server errors and other cases - transient failure
		return fmt.Errorf("delete subject %s returned status %d", string(subjectToRemove), resp.StatusCode), false
	}

	// Remove from tracking map
	delete(s.inRegistryButUnknownLocally, subjectToRemove)

	// Determine if we should advance to next phase
	if len(s.inRegistryButUnknownLocally) == 0 {
		return nil, true // Deleted schema, advance to next phase
	}

	return nil, false // Deleted schema, stay in same phase (more to remove)
}

func (s *SchemaRegistry) addNew(ctx context.Context) (err error, changePhase bool) {
	// Check if context has enough time remaining
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("context deadline already passed"), false
		}
		if remaining < MinimumAddTime {
			return fmt.Errorf("insufficient time remaining in context (< %v)", MinimumAddTime), false
		}
	}

	// Check if work to do
	if len(s.missingInRegistry) == 0 {
		return nil, true // Nothing to add, advance to next phase (start new cycle)
	}

	// Get first subject to add
	var subjectToAdd SubjectName
	var schemaDefinition JSONSchemaDefinition
	for subject, schema := range s.missingInRegistry {
		subjectToAdd = subject
		schemaDefinition = schema
		break
	}
	s.currentOperationSubject = subjectToAdd

	// Prepare JSON schema payload (schemaType defaults to JSON)
	payload := map[string]interface{}{
		"schema":     string(schemaDefinition),
		"schemaType": "JSON",
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal schema for %s: %w", string(subjectToAdd), err), false
	}

	// HTTP POST /subjects/{subject}/versions
	url := fmt.Sprintf("http://%s/subjects/%s/versions", SchemaRegistryAddress, string(subjectToAdd))
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payloadBytes))
	if err != nil {
		return err, false
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to add subject %s: %w", string(subjectToAdd), err), false
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			fmt.Printf("Warning: Failed to close response body: %v\n", closeErr)
		}
	}()

	// Handle success cases
	if resp.StatusCode == http.StatusCreated {
		// HTTP 201 - new schema registered successfully
	} else if resp.StatusCode == http.StatusOK {
		// HTTP 200 - schema updated or already exists
	} else if resp.StatusCode == http.StatusConflict {
		// HTTP 409 - schema already exists with same definition, treat as success
	} else if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		// Client errors - parse custom error codes if available
		respBody, readErr := io.ReadAll(resp.Body)
		if readErr == nil {
			var errorResp map[string]interface{}
			if json.Unmarshal(respBody, &errorResp) == nil {
				if errorCode, ok := errorResp["error_code"].(float64); ok {
					return fmt.Errorf("add subject %s failed with custom error %d: %s", string(subjectToAdd), int(errorCode), errorResp["message"]), false
				}
			}
		}
		// Generic client error handling
		if resp.StatusCode == http.StatusBadRequest {
			return fmt.Errorf("add subject %s failed: bad request (400)", string(subjectToAdd)), false
		} else if resp.StatusCode == http.StatusUnauthorized {
			return fmt.Errorf("add subject %s failed: unauthorized (401)", string(subjectToAdd)), false
		} else if resp.StatusCode == http.StatusForbidden {
			return fmt.Errorf("add subject %s failed: forbidden (403)", string(subjectToAdd)), false
		} else if resp.StatusCode == http.StatusUnprocessableEntity {
			return fmt.Errorf("add subject %s failed: schema validation error (422)", string(subjectToAdd)), false
		} else {
			return fmt.Errorf("add subject %s failed with client error status %d", string(subjectToAdd), resp.StatusCode), false
		}
	} else {
		// Server errors and other cases - transient failure
		return fmt.Errorf("add subject %s returned status %d", string(subjectToAdd), resp.StatusCode), false
	}

	// Remove from tracking map
	delete(s.missingInRegistry, subjectToAdd)

	// Determine if we should advance to next phase
	if len(s.missingInRegistry) == 0 {
		return nil, true // Added schema, advance to next phase (start new cycle)
	}

	return nil, false // Added schema, stay in same phase (more to add)
}
