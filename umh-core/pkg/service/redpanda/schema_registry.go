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

// Package redpanda provides schema registry reconciliation for Redpanda/Kafka environments.
//
// This package implements a multi-phase reconciliation system that ensures JSON schemas
// in a Redpanda Schema Registry match expected configurations. It handles automatic
// cleanup of unexpected schemas and registration of missing schemas through a robust
// state machine with proper error handling and recovery capabilities.
//
// # Architecture Overview
//
// The reconciliation process follows a 5-phase cycle:
//  1. Lookup: Fetch current registry state via HTTP GET /subjects
//  2. Decode: Parse JSON response into typed Go structures
//  3. Compare: Analyze differences and build work queues for actions
//  4. RemoveUnknown: Delete unexpected schemas (one at a time)
//  5. AddNew: Add missing schemas (one at a time)
//
// Each phase is designed for fault tolerance with proper timeout handling,
// error classification, and incremental progress to enable recovery after failures.

package redpanda

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// SubjectName represents a schema subject identifier in the registry.
// Subject names follow Kafka/Redpanda naming conventions and are used
// as unique keys for schema storage and retrieval operations.
type SubjectName string

// JSONSchemaDefinition contains a complete JSON schema definition.
// This includes all schema validation rules, constraints, and metadata
// required for message validation in the registry.
type JSONSchemaDefinition string

// SchemaRegistryPhase represents the current state in the reconciliation process.
// The reconciliation follows a 5-phase state machine: lookup → decode → compare → remove_unknown → add_new.
// Each phase has specific responsibilities and error conditions.
type SchemaRegistryPhase string

// SchemaRegistryMetrics provides observability data for monitoring and alerting.
// All metrics are thread-safe and updated atomically during reconciliation operations.
// Use these metrics to track system health, performance, and operational status.
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

// urlBuilder provides optimized URL construction to avoid repeated allocations
type urlBuilder struct {
	baseURL string
	buf     strings.Builder
}

// subjectsURL builds the /subjects endpoint URL
func (u *urlBuilder) subjectsURL() string {
	u.buf.Reset()
	u.buf.WriteString(u.baseURL)
	u.buf.WriteString("/subjects")
	return u.buf.String()
}

// subjectURL builds the /subjects/{subject} endpoint URL
func (u *urlBuilder) subjectURL(subject SubjectName) string {
	u.buf.Reset()
	u.buf.WriteString(u.baseURL)
	u.buf.WriteString("/subjects/")
	u.buf.WriteString(string(subject))
	return u.buf.String()
}

// subjectVersionsURL builds the /subjects/{subject}/versions endpoint URL
func (u *urlBuilder) subjectVersionsURL(subject SubjectName) string {
	u.buf.Reset()
	u.buf.WriteString(u.baseURL)
	u.buf.WriteString("/subjects/")
	u.buf.WriteString(string(subject))
	u.buf.WriteString("/versions")
	return u.buf.String()
}

// ISchemaRegistry defines the interface for schema registry operations.
// Implementations must provide thread-safe reconciliation and metrics collection.
// All operations respect context cancellation and timeout requirements.
//
// Usage example:
//
//	registry := NewSchemaRegistry()
//	expectedSchemas := map[SubjectName]JSONSchemaDefinition{
//		"user-events": `{"type": "object", "properties": {"id": {"type": "string"}}}`,
//	}
//	if err := registry.Reconcile(ctx, expectedSchemas); err != nil {
//		// Handle reconciliation error
//	}
type ISchemaRegistry interface {
	// Reconcile ensures the registry contains exactly the expected schemas.
	// It adds missing schemas and removes unexpected ones through a multi-phase process.
	// Returns error if reconciliation fails; use backoff for transient failures.
	Reconcile(ctx context.Context, expectedSubjects map[SubjectName]JSONSchemaDefinition) error

	// GetMetrics returns current operational metrics for monitoring and alerting.
	// All metrics are atomic and thread-safe. Use for health checks and dashboards.
	GetMetrics() SchemaRegistryMetrics
}

// SchemaRegistry implements ISchemaRegistry with a multi-phase reconciliation system.
// It manages schema synchronization between expected state and Redpanda/Kafka registry.
// All operations are thread-safe and support context cancellation.
//
// The reconciliation process follows these phases:
//  1. lookup: Fetch current registry state via HTTP GET /subjects
//  2. decode: Parse JSON response into typed structures
//  3. compare: Analyze differences and build work queues
//  4. remove_unknown: Delete unexpected schemas (one at a time)
//  5. add_new: Add missing schemas (one at a time)
//
// State management:
//   - Each phase maintains specific data (rawSubjectsData, work queues, etc.)
//   - Phase transitions occur based on completion and error conditions
//   - Internal state is protected by RWMutex for concurrent access
//
// Error handling:
//   - Network errors: Exponential backoff recommended
//   - Schema validation errors: Check schema format and registry compatibility
//   - Context cancellation: Immediate abort with proper cleanup
//
// Performance characteristics:
//   - Processes one schema per reconciliation cycle in action phases
//   - Typical operation time: <100ms for small schema sets
//   - Memory usage: O(n) where n is number of schemas
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
	schemaRegistryAddress   string

	// Performance optimizations
	urlBuilder urlBuilder // Reused for URL construction to avoid allocations
}

// Schema registry reconciliation phases
const (
	// SchemaRegistryPhaseLookup fetches current registry state via HTTP GET /subjects
	SchemaRegistryPhaseLookup SchemaRegistryPhase = "lookup"

	// SchemaRegistryPhaseDecode parses JSON response into typed Go structures
	SchemaRegistryPhaseDecode SchemaRegistryPhase = "decode"

	// SchemaRegistryPhaseCompare analyzes differences and builds work queues for actions
	SchemaRegistryPhaseCompare SchemaRegistryPhase = "compare"

	// SchemaRegistryPhaseRemoveUnknown deletes unexpected schemas (one at a time)
	SchemaRegistryPhaseRemoveUnknown SchemaRegistryPhase = "remove_unknown"

	// SchemaRegistryPhaseAddNew adds missing schemas (one at a time)
	SchemaRegistryPhaseAddNew SchemaRegistryPhase = "add_new"
)

// Context timeout requirements per phase for performance monitoring and SLA compliance
const (
	// MinimumLookupTime is the minimum context timeout for HTTP GET /subjects operations
	MinimumLookupTime = 25 * time.Millisecond // HTTP GET /subjects (accounts for container networking variations)

	// MinimumDecodeTime is the minimum context timeout for JSON parsing operations
	MinimumDecodeTime = 5 * time.Millisecond // JSON parsing (accounts for slower CPUs and GC pauses)

	// MinimumCompareTime is the minimum context timeout for map comparison operations
	MinimumCompareTime = 5 * time.Millisecond // Map operations (accounts for slower CPUs and large schema sets)

	// MinimumRemoveTime is the minimum context timeout for HTTP DELETE operations
	MinimumRemoveTime = 30 * time.Millisecond // HTTP DELETE (accounts for registry processing time)

	// MinimumAddTime is the minimum context timeout for HTTP POST operations with schema payload
	MinimumAddTime = 35 * time.Millisecond // HTTP POST with schema (accounts for validation and larger payloads)
)

// DefaultSchemaRegistryAddress is the default address for the Redpanda Schema Registry HTTP API.
// This follows the standard Kafka/Redpanda schema registry port convention.
// In production, this should be configurable via environment variables or configuration files.
const DefaultSchemaRegistryAddress = "http://localhost:8081"

// NewSchemaRegistry creates a new SchemaRegistry instance with default configuration.
// The registry starts in lookup phase and initializes all internal state for reconciliation.
// Use this constructor for standard schema registry operations.
//
// Default configuration:
//   - Registry address: localhost:8081
//   - HTTP client: Optimized with connection pooling and timeouts
//   - Phase: lookup (ready for first reconciliation)
//   - Metrics: Zero-initialized counters
//
// Example:
//
//	registry := NewSchemaRegistry()
//	defer registry.Close() // If cleanup needed
//
//	schemas := map[SubjectName]JSONSchemaDefinition{
//		"events": `{"type": "object"}`,
//	}
//	if err := registry.Reconcile(ctx, schemas); err != nil {
//		log.Printf("Reconciliation failed: %v", err)
//	}
func NewSchemaRegistry(opts ...func(*SchemaRegistry)) *SchemaRegistry {
	registry := &SchemaRegistry{
		currentPhase: SchemaRegistryPhaseLookup,
		httpClient: http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        10,               // Connection pool size
				MaxIdleConnsPerHost: 5,                // Connections per registry host
				IdleConnTimeout:     90 * time.Second, // Keep connections alive
				DisableCompression:  false,            // Enable gzip compression
			},
		},
		missingInRegistry:           make(map[SubjectName]JSONSchemaDefinition),
		inRegistryButUnknownLocally: make(map[SubjectName]bool),
		totalReconciliations:        0,
		successfulOperations:        0,
		failedOperations:            0,
		lastOperationTime:           time.Time{},
		lastError:                   "",
		schemaRegistryAddress:       DefaultSchemaRegistryAddress,
		urlBuilder:                  urlBuilder{baseURL: DefaultSchemaRegistryAddress},
	}
	for _, opt := range opts {
		opt(registry)
	}
	// Update urlBuilder baseURL in case it was changed by options
	registry.urlBuilder.baseURL = registry.schemaRegistryAddress
	return registry
}

// WithSchemaRegistryAddress sets the schema registry address for the SchemaRegistry.
// This is useful for testing and integration with Redpanda.
//
// Example:
//
//	registry := NewSchemaRegistry(WithSchemaRegistryAddress("localhost:8081"))
func WithSchemaRegistryAddress(address string) func(*SchemaRegistry) {
	return func(s *SchemaRegistry) {
		s.schemaRegistryAddress = address
	}
}

// Reconcile ensures the schema registry contains exactly the expected schemas.
// It synchronizes the registry state by adding missing schemas and removing unexpected ones.
// The operation follows a multi-phase process with proper error handling and context support.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control (required)
//   - expectedSubjects: Map of subject names to their JSON schema definitions
//
// Behavior:
//   - Adds schemas that exist in expectedSubjects but not in registry
//   - Removes schemas that exist in registry but not in expectedSubjects
//   - Processes operations atomically (one schema per call)
//   - Updates internal metrics for monitoring
//
// Error conditions:
//   - Network failures: HTTP errors, connection timeouts
//   - Schema validation: Invalid JSON schema format
//   - Registry conflicts: Concurrent modifications
//   - Context cancellation: Immediate abort with context.Canceled
//
// Thread safety:
//   - Method is thread-safe with internal synchronization
//   - Concurrent calls will serialize through internal mutex
//   - Metrics are updated atomically
//
// Usage example:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
//	defer cancel()
//
//	schemas := map[SubjectName]JSONSchemaDefinition{
//		"user-events": `{"type": "object", "properties": {"id": {"type": "string"}}}`,
//		"system-logs": `{"type": "object", "properties": {"level": {"type": "string"}}}`,
//	}
//
//	if err := registry.Reconcile(ctx, schemas); err != nil {
//		if errors.Is(err, context.DeadlineExceeded) {
//			// Handle timeout
//		}
//		// Handle other errors
//	}
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

// getNextPhase calculates the next phase based on current phase and whether to change phase.
// This centralizes all phase transition logic and ensures consistent state machine behavior.
//
// Phase flow logic:
// 1. Lookup → Decode (always advance after successful HTTP GET)
// 2. Decode → Compare (always advance after successful JSON parsing)
// 3. Compare → RemoveUnknown | AddNew | Lookup (based on what work needs to be done)
// 4. RemoveUnknown → RemoveUnknown | AddNew (stay if more deletes, advance when done)
// 5. AddNew → AddNew | Lookup (stay if more adds, cycle when done)
//
// The key insight: Compare phase acts as a "traffic controller" that routes to the right
// action phase based on the work that needs to be done. Action phases stay in themselves
// until their work queue is empty, then advance to the next logical phase.
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

// GetMetrics returns current operational metrics for monitoring and alerting.
// All metrics are atomic and thread-safe, providing real-time visibility into
// reconciliation status, performance, and health.
//
// Returned metrics:
//   - CurrentPhase: Current reconciliation state (lookup, decode, compare, remove_unknown, add_new)
//   - TotalReconciliations: Total number of Reconcile() calls
//   - SuccessfulOperations: Count of successful reconciliation operations
//   - FailedOperations: Count of failed reconciliation operations
//   - SubjectsToAdd: Number of schemas pending addition to registry
//   - SubjectsToRemove: Number of schemas pending removal from registry
//   - LastOperationTime: Timestamp of last reconciliation attempt
//   - LastError: Error message from most recent failure (empty if last operation succeeded)
//
// Thread safety:
//   - Method is thread-safe with read-only access
//   - Metrics are updated atomically during reconciliation
//   - Concurrent calls return consistent snapshots
//
// Usage for monitoring:
//
//	metrics := registry.GetMetrics()
//
//	// Health check
//	if metrics.FailedOperations > 0 && metrics.LastError != "" {
//		log.Printf("Schema registry unhealthy: %s", metrics.LastError)
//	}
//
//	// Performance monitoring
//	if time.Since(metrics.LastOperationTime) > 5*time.Minute {
//		log.Printf("Schema registry stale - last operation: %v", metrics.LastOperationTime)
//	}
//
//	// Work queue monitoring
//	if metrics.SubjectsToAdd > 0 || metrics.SubjectsToRemove > 0 {
//		log.Printf("Schema registry pending work: +%d -%d", metrics.SubjectsToAdd, metrics.SubjectsToRemove)
//	}
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

// reconcileInternal orchestrates the multi-phase reconciliation process.
// This is the main control loop that executes phases in sequence until either:
// 1. An error occurs (return immediately)
// 2. A phase requests to stay (return for backoff/retry)
// 3. We complete a full cycle and reach synchronization
//
// Control flow strategy:
// - Each phase returns (error, changePhase) to indicate success and whether to advance
// - Loop continues while phases request advancement (changePhase = true)
// - Early abort on context cancellation to respect timeouts
// - Special case: if we reach Lookup phase with empty work queues, we're fully synchronized
//
// Error handling:
// - Any phase error stops the loop and bubbles up to the caller
// - Phase errors typically indicate transient issues (network, parsing, registry conflicts)
// - Caller (external control loop) handles backoff and retry logic
//
// Memory management:
// - Each phase cleans up its data when transitioning (e.g., decode clears rawSubjectsData)
// - Work queues are reset at compare phase start to ensure clean state
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
			// Phase 1: Fetch current registry state via HTTP GET /subjects
			err, changePhase = s.lookup(ctx)
		case SchemaRegistryPhaseDecode:
			// Phase 2: Parse JSON response into typed Go structures
			err, changePhase = s.decode(ctx)
		case SchemaRegistryPhaseCompare:
			// Phase 3: Analyze differences and build work queues for actions
			err, changePhase = s.compare(ctx, expectedSubjects)
		case SchemaRegistryPhaseRemoveUnknown:
			// Phase 4: Delete unexpected schemas (one at a time)
			err, changePhase = s.removeUnknown(ctx)
		case SchemaRegistryPhaseAddNew:
			// Phase 5: Add missing schemas (one at a time)
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
		}
		// No phase change requested, continue while we have more time
	}
}

// lookup fetches the current state of subjects from the schema registry via HTTP GET /subjects.
// This is the first phase of reconciliation and provides the "ground truth" of what schemas
// currently exist in the registry. We store the raw JSON response for the decode phase to ensure
// we're working with exactly what the registry returned, avoiding any parsing inconsistencies.
//
// Why this phase exists:
// - We need to know the current registry state before making any changes
// - HTTP calls can fail, so we isolate network operations from parsing operations
// - Raw storage allows us to retry parsing without re-fetching if decode fails
//
// Returns: (error, changePhase)
// - error: nil on success, non-nil on network/HTTP errors
// - changePhase: true to advance to decode phase, false to retry this phase
func (s *SchemaRegistry) lookup(ctx context.Context) (err error, changePhase bool) {
	// Check if context has enough time remaining for this operation
	// Each phase has a minimum time requirement to prevent partial operations that might leave
	// the registry in an inconsistent state. Better to fail fast than start an operation
	// that will timeout mid-execution.
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("context deadline already passed"), false
		}
		if remaining < MinimumLookupTime {
			return fmt.Errorf("insufficient time remaining in context (< %v)", MinimumLookupTime), false
		}
	}

	url := s.urlBuilder.subjectsURL()

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

// decode parses the raw JSON response from the lookup phase into structured Go data.
// This phase converts the raw bytes into a typed slice of SubjectName for easier manipulation
// in subsequent phases. We separate this from lookup to isolate parsing errors from network errors.
//
// Why this phase exists:
// - JSON parsing can fail independently of network operations
// - Type conversion ensures we work with strongly-typed data throughout the rest of reconciliation
// - Memory optimization: we clear the raw data after parsing to free memory
// - Error isolation: parsing failures don't require re-fetching data from the network
//
// Returns: (error, changePhase)
// - error: nil on success, non-nil on JSON parsing errors
// - changePhase: true to advance to compare phase, false to retry this phase
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

// compare analyzes the differences between what we expect (expectedSubjects) and what exists
// in the registry (registrySubjects from decode phase). This is the "brain" of reconciliation
// that determines what actions need to be taken to bring the registry into the desired state.
//
// Why this phase exists:
// - Separates analysis from action, making the logic easier to understand and test
// - Builds work queues (missingInRegistry, inRegistryButUnknownLocally) for execution phases
// - Enables intelligent routing: removes unexpected schemas first, then adds missing ones
// - Fast O(1) lookups using maps instead of O(n²) nested loops
//
// Decision logic:
// - missingInRegistry: schemas we expect but registry doesn't have → need to ADD
// - inRegistryButUnknownLocally: schemas registry has but we don't expect → need to REMOVE
// - Next phase routing: RemoveUnknown (if any) → AddNew (if any) → Lookup (if fully in sync)
//
// Returns: (error, changePhase)
// - error: nil on success (analysis operations don't typically fail)
// - changePhase: always true (analysis complete, time to act or start new cycle)
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

// removeUnknown deletes schemas from the registry that exist but are not in our expected set.
// This phase processes one subject at a time to avoid overwhelming the registry and to allow
// for proper error handling per operation. We delete BEFORE adding to prevent conflicts and
// ensure clean state transitions.
//
// Why this phase exists:
// - Clean slate approach: remove unexpected schemas before adding new ones
// - Conflict prevention: avoid naming conflicts between old and new schemas
// - Incremental progress: process one subject per call to enable resumption after failures
// - Error isolation: individual delete failures don't affect other operations
//
// Why delete first:
// - Schema registries may have constraints on subject names or counts
// - Removing unused schemas frees up resources for new ones
// - Cleaner error messages: "already exists" vs "constraint violation"
//
// Error handling strategy:
// - Success: HTTP 200, 204 (deleted), 404 (already gone), custom 40401/40406 (not found/soft-deleted)
// - Transient failures: HTTP 42206 (has references - retry later), network errors, server errors
// - Permanent failures: authentication/authorization errors (but we still retry)
//
// Returns: (error, changePhase)
// - error: nil on success, non-nil on network/HTTP/registry errors
// - changePhase: true if work queue empty (advance), false if more subjects to delete (stay)
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
	url := s.urlBuilder.subjectURL(subjectToRemove)
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return err, false
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete subject %s: %w", string(subjectToRemove), err), false
	}
	if resp == nil {
		return fmt.Errorf("received nil response from schema registry for DELETE %s", string(subjectToRemove)), false
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			fmt.Printf("Warning: Failed to close response body: %v\n", closeErr)
		}
	}()

	// Handle HTTP response status codes
	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent:
		// HTTP 200, 204 - successful deletion, no need to read body

	case http.StatusNotFound:
		// HTTP 404 - subject already gone, treat as success

	default:
		// Handle client errors with custom error code parsing
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
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
	}

	// Remove from tracking map
	delete(s.inRegistryButUnknownLocally, subjectToRemove)

	// Determine if we should advance to next phase
	if len(s.inRegistryButUnknownLocally) == 0 {
		return nil, true // Deleted schema, advance to next phase
	}

	return nil, false // Deleted schema, stay in same phase (more to remove)
}

// addNew registers new schemas in the registry that are in our expected set but don't exist yet.
// This is the final action phase that brings the registry to the desired state by adding missing
// schemas. Like removeUnknown, we process one subject at a time for proper error handling and
// recovery capabilities.
//
// Why this phase exists:
// - Goal completion: adds the schemas we actually want after cleanup is done
// - Incremental progress: one subject per call enables resumption after failures
// - Clean state: runs after removeUnknown to avoid conflicts with old schemas
// - Error isolation: individual add failures don't affect other operations
//
// Why add after delete:
// - Ensures clean namespace: no conflicts with old schema definitions
// - Better error messages: failures are clearly about the new schema, not conflicts
// - Resource optimization: registry has maximum space available for new schemas
//
// JSON Schema specifics:
// - Always uses schemaType: "JSON" (we only support JSON schemas)
// - Single version per subject (simplified model vs. full versioning)
// - Schema definition comes from caller's expected configuration
//
// Error handling strategy:
// - Success: HTTP 201 (created), 200 (updated), 409 (already exists with same definition)
// - Transient failures: HTTP 400/422 (validation errors), 401/403 (auth), server errors
// - Permanent failures: malformed schema JSON (but we still retry in case it's transient)
//
// Returns: (error, changePhase)
// - error: nil on success, non-nil on network/HTTP/registry errors
// - changePhase: true if work queue empty (start new cycle), false if more subjects to add (stay)
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
	url := s.urlBuilder.subjectVersionsURL(subjectToAdd)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payloadBytes))
	if err != nil {
		return err, false
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to add subject %s: %w", string(subjectToAdd), err), false
	}
	if resp == nil {
		return fmt.Errorf("received nil response from schema registry for POST %s", string(subjectToAdd)), false
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			fmt.Printf("Warning: Failed to close response body: %v\n", closeErr)
		}
	}()

	// Handle HTTP response status codes
	switch resp.StatusCode {
	case http.StatusCreated:
		// HTTP 201 - new schema registered successfully, no need to read body

	case http.StatusOK:
		// HTTP 200 - schema updated or already exists, no need to read body

	case http.StatusConflict:
		// HTTP 409 - schema already exists with same definition, treat as success

	default:
		// Handle client errors with custom error code parsing
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			respBody, readErr := io.ReadAll(resp.Body)
			if readErr == nil {
				var errorResp map[string]interface{}
				if json.Unmarshal(respBody, &errorResp) == nil {
					if errorCode, ok := errorResp["error_code"].(float64); ok {
						return fmt.Errorf("add subject %s failed with custom error %d: %s", string(subjectToAdd), int(errorCode), errorResp["message"]), false
					}
				}
			}

			// Generic client error handling by status code
			switch resp.StatusCode {
			case http.StatusBadRequest:
				return fmt.Errorf("add subject %s failed: bad request (400)", string(subjectToAdd)), false
			case http.StatusUnauthorized:
				return fmt.Errorf("add subject %s failed: unauthorized (401)", string(subjectToAdd)), false
			case http.StatusForbidden:
				return fmt.Errorf("add subject %s failed: forbidden (403)", string(subjectToAdd)), false
			case http.StatusUnprocessableEntity:
				return fmt.Errorf("add subject %s failed: schema validation error (422)", string(subjectToAdd)), false
			default:
				return fmt.Errorf("add subject %s failed with client error status %d", string(subjectToAdd), resp.StatusCode), false
			}
		} else {
			// Server errors and other cases - transient failure
			return fmt.Errorf("add subject %s returned status %d", string(subjectToAdd), resp.StatusCode), false
		}
	}

	// Remove from tracking map
	delete(s.missingInRegistry, subjectToAdd)

	// Determine if we should advance to next phase
	if len(s.missingInRegistry) == 0 {
		return nil, true // Added schema, advance to next phase (start new cycle)
	}

	return nil, false // Added schema, stay in same phase (more to add)
}
