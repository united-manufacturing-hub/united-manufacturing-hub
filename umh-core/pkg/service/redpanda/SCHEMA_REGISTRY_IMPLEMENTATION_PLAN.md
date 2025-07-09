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
    MinimumLookupTime      = 20 * time.Millisecond   // HTTP GET /subjects
    MinimumDecodeTime      = 5 * time.Millisecond    // JSON parsing
    MinimumCompareTime     = 10 * time.Millisecond   // Map operations
    MinimumRemoveTime      = 50 * time.Millisecond   // HTTP DELETE
    MinimumAddTime         = 100 * time.Millisecond  // HTTP POST with schema
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
- **Lookup errors**: Only HTTP 200 is success, all others are transient failures
- **Remove errors**: HTTP 200 and 404 are success (404 = already gone)
- **Add errors**: HTTP 200, 201, and 409 are success (409 = already exists)
- **Context timeout**: Return error early, preserve state for next attempt

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
    
    // HTTP 200 and 404 are both considered success (404 = already gone)
    if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
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
    
    // HTTP 200, 201, and 409 are considered success (409 = already exists with same schema)
    if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusConflict {
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

### Constructor Options
```go
func NewSchemaRegistry(expectedSubjects map[SubjectName]JSONSchemaDefinition) *SchemaRegistry {
    return &SchemaRegistry{
        currentPhase:                SchemaRegistryPhaseLookup,
        httpClient:                  http.Client{},
        expectedSubjects:            expectedSubjects,
        missingInRegistry:           make(map[SubjectName]JSONSchemaDefinition),
        inRegistryButUnknownLocally: make(map[SubjectName]bool),
        totalReconciliations:        0,
        successfulOperations:        0,
        failedOperations:            0,
        lastOperationTime:           time.Time{},
        lastError:                   "",
    }
}

// Alternative: Configuration-driven constructor
func NewSchemaRegistryFromConfig(config SchemaRegistryConfig) *SchemaRegistry {
    expectedSubjects := make(map[SubjectName]JSONSchemaDefinition)
    for subject, schema := range config.ExpectedSchemas {
        expectedSubjects[SubjectName(subject)] = JSONSchemaDefinition(schema)
    }
    return NewSchemaRegistry(expectedSubjects)
}
```

### Expected Subjects Source
For initial implementation, use embedded defaults:
```go
var DefaultExpectedSchemas = map[SubjectName]JSONSchemaDefinition{
    SubjectName("sensor-value"): JSONSchemaDefinition(`{
        "type": "object",
        "properties": {
            "timestamp": {"type": "string", "format": "date-time"},
            "value": {"type": "number"},
            "unit": {"type": "string"}
        },
        "required": ["timestamp", "value"]
    }`),
    SubjectName("machine-state"): JSONSchemaDefinition(`{
        "type": "object", 
        "properties": {
            "machineId": {"type": "string"},
            "state": {"type": "string", "enum": ["running", "stopped", "maintenance"]},
            "timestamp": {"type": "string", "format": "date-time"}
        },
        "required": ["machineId", "state", "timestamp"]
    }`),
    // ... more JSON schemas
}
```

## Test Strategy

### Mock Registry Extensions
Extend existing mock to handle new endpoints:
- `DELETE /subjects/{subject}` 
- `POST /subjects/{subject}/versions`
- Track state changes for verification

### Test Categories
1. **Phase Transition Tests**: Verify correct phase flow
2. **Context Timeout Tests**: Each phase handles insufficient time
3. **Error Handling Tests**: Network failures, malformed responses
4. **Reconciliation Logic Tests**: Proper `reconciled` return values
5. **End-to-End Tests**: Complete cycles with add/remove operations

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
```

## Implementation Steps

### Step 1: Update Data Structures
- Add type aliases for improved type safety (`SubjectName`, `JSONSchemaDefinition`)
- Extend SchemaRegistry struct with concurrency protection and metrics
- Add new phase constants
- Update ISchemaRegistry interface to include GetMetrics() method
- Use typed maps for better compile-time safety

### Step 2: Write Comprehensive Tests
- Phase transition tests
- Context timeout handling
- Add/remove operation tests
- Mock registry extensions

### Step 3: Implement Phases
- Update existing lookup to store raw bytes
- Implement decode phase
- Implement compare phase
- Implement removeUnknown phase
- Implement addNew phase

### Step 4: Integration
- Update Reconcile method routing
- Test end-to-end scenarios
- Performance validation

## Error Scenarios & Recovery

### HTTP Response Handling
- **Lookup**: Only HTTP 200 is success - all other codes are transient failures
- **Remove**: HTTP 200 and 404 are success (404 means subject already deleted)
- **Add**: HTTP 200, 201, and 409 are success (409 means schema already exists)
- All other HTTP codes are treated as transient failures and retried

### Context Timeouts
- Each phase checks remaining time before operations
- Return error immediately if insufficient time
- State preserved for next reconciliation attempt

### Network Failures
- HTTP timeouts, connection refused, etc.
- Return error, retry in next reconciliation
- No state corruption

### Schema Validation
- Incoming schemas from configuration are already validated - no additional checks needed
- Registry schemas that don't match expected schemas are marked for removal
- No circuit breaker needed - worst case is wasting a few reconciliation cycles

### Partial Completion
- If remove/add operations partially complete, maps track remaining work
- Next reconciliation continues from where left off
- No duplicate operations

### Concurrency Safety
- All public methods (Reconcile, GetMetrics) are protected by mutex
- Internal methods are private to prevent external concurrent access
- Single-threaded execution within reconciliation phases

## Performance Considerations

### Memory Management
- Clear rawSubjectsData after decode to free memory
- Use maps for O(1) lookups in compare phase
- HTTP client reuse for efficiency

### Network Efficiency
- Process one schema at a time (atomic operations)
- Context-aware timeouts prevent hanging
- Simple retry logic without circuit breakers

### Reconciliation Frequency
- Only return `reconciled=true` for actual changes
- Reduces unnecessary reconciliation calls
- Analysis phases are fast (return quickly)

### Observability
- **Metrics**: GetMetrics() function provides current state and counters
- **Logging**: Use standard logger for errors and debug information
- **Concurrency**: Mutex protection ensures thread-safe access to metrics

## Future Enhancements

### Configuration Management
- Dynamic schema configuration updates
- Hot-reload capabilities from external sources
- Schema definition validation improvements

### Enhanced Observability
- Detailed phase transition metrics
- Performance timing per operation
- Enhanced error categorization and reporting
- Integration with monitoring systems

### Operational Improvements
- Graceful shutdown handling
- Health check endpoints
- Configuration validation at startup 