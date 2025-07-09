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
```

### Core SchemaRegistry Struct
```go
type SchemaRegistry struct {
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
    
    // Operation tracking
    currentOperationSubject SubjectName                            // Which subject being processed
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

### Return Values
- **`reconciled = false`**: Made internal progress, no external changes
- **`reconciled = true`**: Made actual changes to schema registry (DELETE/POST succeeded)

### Error Handling
- **Transient errors**: Return error, retry in next reconciliation
- **Context timeout**: Return error early, preserve state for next attempt
- **Permanent errors**: Log and continue (consider marking as processed)

## Phase Implementation Details

### 1. Lookup Phase
**Purpose**: Fetch current registry state as raw bytes

**Implementation**:
```go
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
    // ... handle request
    
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
    
    if resp.StatusCode != http.StatusOK {
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
    
    if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
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
- Extend SchemaRegistry struct with new fields  
- Add new phase constants
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

### Context Timeouts
- Each phase checks remaining time before operations
- Return error immediately if insufficient time
- State preserved for next reconciliation attempt

### Network Failures
- HTTP timeouts, connection refused, etc.
- Return error, retry in next reconciliation
- No state corruption

### Malformed Responses
- JSON decode failures
- Unexpected HTTP status codes
- Log errors, return failure

### Partial Completion
- If remove/add operations partially complete, maps track remaining work
- Next reconciliation continues from where left off
- No duplicate operations

## Performance Considerations

### Memory Management
- Clear rawSubjectsData after decode
- Use maps for O(1) lookups in compare phase
- Limit expected subjects to reasonable size

### Network Efficiency
- Process one schema at a time (atomic operations)
- Context-aware timeouts prevent hanging
- HTTP client reuse

### Reconciliation Frequency
- Only return `reconciled=true` for actual changes
- Reduces unnecessary reconciliation calls
- Analysis phases are fast (return quickly)

## Future Enhancements

### Schema Versioning
- Track and handle schema version evolution
- Compatibility checking before updates
- Rollback capabilities

### Batch Operations
- Add support for batch schema operations
- Reduce HTTP request overhead
- Maintain atomicity per schema

### Configuration Management
- Dynamic schema configuration updates
- Hot-reload capabilities
- Validation of schema definitions

### Monitoring & Observability
- Metrics for phase transitions
- Schema operation counters
- Error rate tracking
- Performance metrics per phase 