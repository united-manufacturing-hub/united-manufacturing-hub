# Schema Registry Multi-Phase Implementation Plan

## Overview

This document outlines the complete implementation plan for the schema registry reconciliation system that processes **JSON schemas only** with **single version per subject** through multiple phases: Lookup → Decode → Compare → RemoveUnknown → AddNew → Lookup (cycle).

## Simplifications
- **JSON schemas only**: No need to handle Avro/Protobuf formats
- **Single version per subject**: No version tracking or compatibility checks needed
- **Simple API calls**: Direct subject-level operations without version management

### Simplified API Endpoints
- `GET /subjects` - List all subjects (unchanged)
- `DELETE /subjects/{subject}` - Delete entire subject (no version needed)
- `POST /subjects/{subject}/versions` - Add schema (schemaType always "JSON")

## Phase Flow Architecture

```
┌─────────┐    ┌────────┐    ┌─────────┐    ┌──────────────┐    ┌────────┐
│ Lookup  │───▶│ Decode │───▶│ Compare │───▶│ RemoveUnknown│───▶│ AddNew │
└─────────┘    └────────┘    └─────────┘    └──────────────┘    └────────┘
     ▲                                             │                  │
     │                                             ▼                  ▼
     └─────────────────────────────────────────────┴──────────────────┘
```

## Data Structures

### Type Definitions
```go
// Type aliases for improved type safety and documentation
type SubjectName string
type JSONSchemaDefinition string
type SchemaRegistryPhase string

// Metrics for observability
type SchemaRegistryMetrics struct {
    CurrentPhase            SchemaRegistryPhase
    TotalReconciliations    int64
    SuccessfulOperations    int64
    FailedOperations        int64
    SubjectsToAdd          int
    SubjectsToRemove       int
    LastOperationTime      time.Time
    LastError              string
}
```

### Core SchemaRegistry Struct
```go
type SchemaRegistry struct {
    // Concurrency protection
    mu sync.RWMutex
    
    // Core state
    currentPhase SchemaRegistryPhase
    httpClient   http.Client
    
    // Phase-specific data
    rawSubjectsData []byte                                          // Raw HTTP response from lookup
    registrySubjects []SubjectName                                  // Decoded subjects from registry
    
    // Configuration/Expected state (JSON schemas only, single version)
    expectedSubjects map[SubjectName]JSONSchemaDefinition          // Subject name -> JSON schema definition
    
    // Comparison results  
    missingInRegistry map[SubjectName]JSONSchemaDefinition         // Subject -> schema (we have, registry doesn't)
    inRegistryButUnknownLocally map[SubjectName]bool               // Registry has, we don't expect
    
    // Operation tracking and metrics
    currentOperationSubject SubjectName                            // Which subject being processed
    totalReconciliations    int64
    successfulOperations    int64
    failedOperations        int64
    lastOperationTime       time.Time
    lastError               string
}
```

### Phase Constants
```go
const (
    SchemaRegistryPhaseLookup       SchemaRegistryPhase = "lookup"
    SchemaRegistryPhaseDecode       SchemaRegistryPhase = "decode"
    SchemaRegistryPhaseCompare      SchemaRegistryPhase = "compare"
    SchemaRegistryPhaseRemoveUnknown SchemaRegistryPhase = "remove_unknown"
    SchemaRegistryPhaseAddNew       SchemaRegistryPhase = "add_new"
)

// Context timeout requirements per phase
const (
    MinimumLookupTime      = 10 * time.Millisecond  // HTTP GET /subjects (local)
    MinimumDecodeTime      = 1 * time.Millisecond   // JSON parsing (fast)
    MinimumCompareTime     = 1 * time.Millisecond   // Map operations (instant)
    MinimumRemoveTime      = 10 * time.Millisecond  // HTTP DELETE (local)
    MinimumAddTime         = 15 * time.Millisecond  // HTTP POST with schema (local, slightly larger payload)
)
```

## Reconciliation Semantics

### Concurrency Safety
```go
func (s *SchemaRegistry) Reconcile(ctx context.Context) (err error, reconciled bool) {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    s.totalReconciliations++
    defer func() {
        s.lastOperationTime = time.Now()
        if err != nil {
            s.failedOperations++
            s.lastError = err.Error()
        } else if reconciled {
            s.successfulOperations++
            s.lastError = ""
        }
    }()
    
    return s.reconcileInternal(ctx)
}

func (s *SchemaRegistry) GetMetrics() SchemaRegistryMetrics {
    s.mu.RLock()
    defer s.mu.RUnlock()
    
    return SchemaRegistryMetrics{
        CurrentPhase:         s.currentPhase,
        TotalReconciliations: s.totalReconciliations,
        SuccessfulOperations: s.successfulOperations,
        FailedOperations:     s.failedOperations,
        SubjectsToAdd:       len(s.missingInRegistry),
        SubjectsToRemove:    len(s.inRegistryButUnknownLocally),
        LastOperationTime:   s.lastOperationTime,
        LastError:           s.lastError,
    }
}
```

### Return Values
- **`reconciled = false`**: Made internal progress, no external changes
- **`reconciled = true`**: Made actual changes to schema registry (DELETE/POST succeeded)

### Error Handling
Based on comprehensive Redpanda Schema Registry error code analysis:

- **Lookup errors**: Only HTTP 200 is success, all others are transient failures
- **Remove errors**: Complex handling required:
  - **Success**: HTTP 200, 204 (successful deletion)
  - **Already gone**: HTTP 404, custom error 40401 (subject not found), 40406 (already soft-deleted)
  - **Blocked**: Custom error 42206 (schema has references) - treat as transient failure
  - **Auth/Perms**: HTTP 401, 403 - treat as transient failure
  - **Other**: All other codes are transient failures
- **Add errors**: Success cases:
  - **Created**: HTTP 201 (new schema registered)
  - **Exists**: HTTP 409 (schema already exists with same definition)
  - **Updated**: HTTP 200 (version updated)
- **Add failures**: Treat as transient:
  - **Client errors**: HTTP 400 (bad request), 422 (validation failed)
  - **Auth/Perms**: HTTP 401, 403
  - **Other**: All other codes
- **Context timeout**: Return error early, preserve state for next attempt
- **Custom error parsing**: Parse JSON response for `error_code` field when HTTP status indicates error

## Phase Implementation Details

### 1. Lookup Phase
**Purpose**: Fetch current registry state as raw bytes

**Implementation**:
```go
func (s *SchemaRegistry) reconcileInternal(ctx context.Context) (err error, reconciled bool) {
    switch s.currentPhase {
    case SchemaRegistryPhaseLookup:
        return s.lookup(ctx)
    case SchemaRegistryPhaseDecode:
        return s.decode(ctx)
    case SchemaRegistryPhaseCompare:
        return s.compare(ctx)
    case SchemaRegistryPhaseRemoveUnknown:
        return s.removeUnknown(ctx)
    case SchemaRegistryPhaseAddNew:
        return s.addNew(ctx)
    default:
        return fmt.Errorf("unknown phase: %s", s.currentPhase), false
    }
}

func (s *SchemaRegistry) lookup(ctx context.Context) (err error, reconciled bool) {
    // Check context has >= MinimumLookupTime
    if deadline, ok := ctx.Deadline(); ok {
        if time.Until(deadline) < MinimumLookupTime {
            return fmt.Errorf("insufficient time for lookup"), false
        }
    }
    
    // HTTP GET /subjects
    url := fmt.Sprintf("http://%s/subjects", SchemaRegistryAddress)
    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return err, false
    }
    
    resp, err := s.httpClient.Do(req)
    if err != nil {
        return err, false
    }
    defer func() {
        if closeErr := resp.Body.Close(); closeErr != nil {
            // Log warning but don't fail the operation
        }
    }()
    
    // Only HTTP 200 is considered success - all others are transient failures
    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("schema registry lookup failed with status %d", resp.StatusCode), false
    }
    
    // Store raw response bytes
    s.rawSubjectsData, err = io.ReadAll(resp.Body)
    if err != nil {
        return err, false
    }
    
    s.currentPhase = SchemaRegistryPhaseDecode
    return nil, false  // Downloaded data, no changes made
}
```

**Transitions**: Always → `decode`
**Returns**: `nil, false` (no external changes)

### 2. Decode Phase
**Purpose**: Parse raw JSON into structured data

**Implementation**:
```go
func (s *SchemaRegistry) decode(ctx context.Context) (err error, reconciled bool) {
    // Check context time
    if deadline, ok := ctx.Deadline(); ok {
        if time.Until(deadline) < MinimumDecodeTime {
            return fmt.Errorf("insufficient time for decode"), false
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
    
    s.currentPhase = SchemaRegistryPhaseCompare
    return nil, false  // Parsed data, no changes made
}
```

**Transitions**: Always → `compare`
**Returns**: `nil, false` (no external changes)

### 3. Compare Phase
**Purpose**: Analyze differences between expected and actual state

**Implementation**:
```go
func (s *SchemaRegistry) compare(ctx context.Context) (err error, reconciled bool) {
    // Check context time
    if deadline, ok := ctx.Deadline(); ok {
        if time.Until(deadline) < MinimumCompareTime {
            return fmt.Errorf("insufficient time for compare"), false
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
    for subject, schema := range s.expectedSubjects {
        if !registryMap[subject] {
            s.missingInRegistry[subject] = schema
        }
    }
    
    // Find unknown in registry (registry has, we don't expect)
    for _, subject := range s.registrySubjects {
        if _, expected := s.expectedSubjects[subject]; !expected {
            s.inRegistryButUnknownLocally[subject] = true
        }
    }
    
    // Determine next phase
    if len(s.inRegistryButUnknownLocally) > 0 {
        s.currentPhase = SchemaRegistryPhaseRemoveUnknown
    } else if len(s.missingInRegistry) > 0 {
        s.currentPhase = SchemaRegistryPhaseAddNew
    } else {
        s.currentPhase = SchemaRegistryPhaseLookup  // Fully in sync
    }
    
    return nil, false  // Analyzed differences, no changes made
}
```

**Transitions**: 
- If unknowns exist → `remove_unknown`
- Else if missing exist → `add_new`  
- Else → `lookup` (fully in sync)
**Returns**: `nil, false` (no external changes)

### 4. RemoveUnknown Phase
**Purpose**: Delete unexpected schemas from registry (one at a time)

**Implementation**:
```go
func (s *SchemaRegistry) removeUnknown(ctx context.Context) (err error, reconciled bool) {
    // Check if work to do
    if len(s.inRegistryButUnknownLocally) == 0 {
        s.currentPhase = SchemaRegistryPhaseAddNew
        return nil, false  // Nothing to remove
    }
    
    // Check context time
    if deadline, ok := ctx.Deadline(); ok {
        if time.Until(deadline) < MinimumRemoveTime {
            return fmt.Errorf("insufficient time for remove operation"), false
        }
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
    defer resp.Body.Close()
    
    // Handle success cases: HTTP 200, 204 (successful deletion)
    if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
        // Successful deletion
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
    
    // Determine next phase/state
    if len(s.inRegistryButUnknownLocally) == 0 {
        s.currentPhase = SchemaRegistryPhaseAddNew
    }
    // Stay in same phase if more to remove
    
    return nil, true  // Successfully deleted schema - actual change made!
}
```

**Transitions**:
- If more to remove → stay in `remove_unknown`
- If done → `add_new`
**Returns**: `nil, true` (external change made)

### 5. AddNew Phase
**Purpose**: Add missing schemas to registry (one at a time)

**Implementation**:
```go
func (s *SchemaRegistry) addNew(ctx context.Context) (err error, reconciled bool) {
    // Check if work to do
    if len(s.missingInRegistry) == 0 {
        s.currentPhase = SchemaRegistryPhaseLookup  // Start new cycle
        return nil, false  // Nothing to add
    }
    
    // Check context time
    if deadline, ok := ctx.Deadline(); ok {
        if time.Until(deadline) < MinimumAddTime {
            return fmt.Errorf("insufficient time for add operation"), false
        }
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
    defer resp.Body.Close()
    
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
    
    // Determine next phase/state
    if len(s.missingInRegistry) == 0 {
        s.currentPhase = SchemaRegistryPhaseLookup  // Start new cycle
    }
    // Stay in same phase if more to add
    
    return nil, true  // Successfully added schema - actual change made!
}
```

**Transitions**:
- If more to add → stay in `add_new`
- If done → `lookup` (start new cycle)
**Returns**: `nil, true` (external change made)

## Constructor and Configuration

### Constructor Design
The constructor creates a purely generic schema registry without any hardcoded schemas:

```go
// Single constructor - purely generic infrastructure
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
```

### Schema Configuration
Schemas are provided dynamically at reconcile time by the caller:

```go
// Caller provides schemas when reconciling
expectedSchemas := map[SubjectName]JSONSchemaDefinition{
    SubjectName("sensor-value"): JSONSchemaDefinition(`{"type": "object", ...}`),
    SubjectName("machine-state"): JSONSchemaDefinition(`{"type": "object", ...}`),
}
err := registry.Reconcile(ctx, expectedSchemas)
```

### Design Benefits
1. **Pure Infrastructure**: No business logic embedded in infrastructure
2. **Dynamic Configuration**: Schemas come from configuration files, services, or runtime decisions
3. **Reusable Component**: Can be used in any context with any schemas
4. **Proper Separation**: Business schemas belong in business layer, not infrastructure layer
5. **Testing Flexibility**: Easy to test with different schema sets

## Test Strategy

### Mock Registry Extensions
Extend existing mock to handle new endpoints:
- `DELETE /subjects/{subject}` 
- `POST /subjects/{subject}/versions`
- Track state changes for verification

### Test Categories
1. **Phase Transition Tests**: Verify correct phase flow
2. **Context Timeout Tests**: Each phase handles insufficient time
3. **Error Handling Tests**: 
   - Network failures, malformed responses
   - **Standard HTTP errors**: 400, 401, 403, 404, 422, 5xx
   - **Custom error codes**: 40401, 40403, 40406, 42206
   - **Success scenarios**: 200, 201, 204, 409
   - **Error response parsing**: JSON error format handling
4. **Reconciliation Logic Tests**: Proper `reconciled` return values
5. **End-to-End Tests**: Complete cycles with add/remove operations
6. **Schema Reference Tests**: Handling of 42206 (schema has references) errors
7. **Soft Deletion Tests**: Handling of 40406 (already soft-deleted) scenarios

### Test Data Structure
```go
type TestScenario struct {
    Name                string
    InitialRegistry     []SubjectName                          // What's in mock registry
    ExpectedSubjects    map[SubjectName]JSONSchemaDefinition   // Subject -> JSON schema
    ExpectedPhases      []SchemaRegistryPhase
    ExpectedChanges     []SubjectName                          // Which subjects added/removed
    ExpectedReconciled  []bool                                 // reconciled value per phase
}

type ErrorTestScenario struct {
    Name                string
    Phase              SchemaRegistryPhase
    HTTPStatusCode     int
    CustomErrorCode    int                                     // For custom Redpanda errors
    ErrorMessage       string
    ExpectedHandling   string                                  // "success", "transient_failure", "permanent_failure"
    ExpectedReconciled bool
}
```

### Example Error Test Scenarios
```go
var errorTestScenarios = []ErrorTestScenario{
    {
        Name:               "DELETE: Subject already gone (404)",
        Phase:              SchemaRegistryPhaseRemoveUnknown,
        HTTPStatusCode:     404,
        ExpectedHandling:   "success",
        ExpectedReconciled: true,
    },
    {
        Name:               "DELETE: Subject not found (40401)",
        Phase:              SchemaRegistryPhaseRemoveUnknown,
        HTTPStatusCode:     404,
        CustomErrorCode:    40401,
        ErrorMessage:       "Subject 'test-subject' not found.",
        ExpectedHandling:   "success",
        ExpectedReconciled: true,
    },
    {
        Name:               "DELETE: Already soft-deleted (40406)",
        Phase:              SchemaRegistryPhaseRemoveUnknown,
        HTTPStatusCode:     422,
        CustomErrorCode:    40406,
        ErrorMessage:       "Subject 'test-subject' Version 1 was soft deleted.",
        ExpectedHandling:   "success",
        ExpectedReconciled: true,
    },
    {
        Name:               "DELETE: Schema has references (42206)",
        Phase:              SchemaRegistryPhaseRemoveUnknown,
        HTTPStatusCode:     422,
        CustomErrorCode:    42206,
        ErrorMessage:       "One or more references exist to the schema",
        ExpectedHandling:   "transient_failure",
        ExpectedReconciled: false,
    },
    {
        Name:               "POST: Schema created (201)",
        Phase:              SchemaRegistryPhaseAddNew,
        HTTPStatusCode:     201,
        ExpectedHandling:   "success",
        ExpectedReconciled: true,
    },
    {
        Name:               "POST: Schema already exists (409)",
        Phase:              SchemaRegistryPhaseAddNew,
        HTTPStatusCode:     409,
        ExpectedHandling:   "success",
        ExpectedReconciled: true,
    },
    {
        Name:               "POST: Validation failed (422)",
        Phase:              SchemaRegistryPhaseAddNew,
        HTTPStatusCode:     422,
        ExpectedHandling:   "transient_failure",
        ExpectedReconciled: false,
    },
}
```