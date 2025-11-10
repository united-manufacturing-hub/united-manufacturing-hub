# FSM v2 Worker System: Complete Capability Analysis

**Purpose**: Comprehensive documentation of ALL capabilities that example workers must demonstrate, based on `worker.go` and `communicator/worker.go` implementations.

**Source Files**:
- `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/worker.go` (lines 1-318)
- `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/workers/communicator/worker.go` (lines 1-270)
- Related: `communicator/state/*.go`, `communicator/action/*.go`, `communicator/snapshot/snapshot.go`

---

## CAPABILITY MATRIX

Example workers MUST demonstrate all 13 core capabilities below. This matrix shows which capabilities are shown in communicator worker (✓) and required for new workers (MUST).

| # | Capability | Communicator | Must Show | Complexity | Lines (worker.go) |
|---|-----------|-----------|----------|-----------|---------|
| 1 | CollectObservedState async/timeout pattern | ✓ | MUST | Medium | 276-297 |
| 2 | CollectObservedState error handling | ✓ | MUST | Medium | 289-292 |
| 3 | DeriveDesiredState pure function | ✓ | MUST | High | 299-316 |
| 4 | DeriveDesiredState templating/composition | ✓ | MUST | High | 299-316 |
| 5 | GetInitialState pattern | ✓ | MUST | Low | 312-316 |
| 6 | Signal types (SignalNone, SignalNeedsRemoval, SignalNeedsRestart) | ✓ | MUST | Low | 38-49 |
| 7 | Active state pattern (TryingTo*) | ✓ | MUST | High | 180-194 |
| 8 | Passive state pattern (descriptive noun) | ✓ | MUST | High | 180-194 |
| 9 | State transition patterns (explicit conditions) | ✓ | MUST | High | 210-250 |
| 10 | Snapshot immutability (pass-by-value) | ✓ (implicit) | MUST (test) | Medium | 82-114 |
| 11 | Action idempotency requirement | ✓ | MUST | Critical | 116-167 |
| 12 | Action context cancellation handling | ✓ | MUST | High | 162-164 |
| 13 | ObservedState interface implementation | ✓ | MUST | High | 59-71 |

---

## 1. COLLECTOBSERVEDSTATE: ASYNC MONITORING PATTERN

### What It Enables
CollectObservedState runs in a separate supervisor goroutine to continuously monitor actual system state WITHOUT blocking the FSM's main state machine loop. This enables:
- Non-blocking state collection (FSM never waits for monitoring to complete)
- Timeout protection (operations that hang are automatically cancelled)
- Error resilience (collection failures don't stop the FSM)
- Fresh data on every tick (supervisor always has latest observation)

### When to Use
Implement when your worker needs to monitor:
- External process status (is it running? what resources does it use?)
- Network connectivity (can we reach the service?)
- API health (does the endpoint respond?)
- File/directory existence and state
- Hardware resource availability

### How Communicator Demonstrates It

**File**: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/workers/communicator/worker.go`
**Lines**: 220-243 (CollectObservedState method)

```go
// CollectObservedState returns the current observed state of the communicator.
//
// This method is called periodically by the FSM v2 Supervisor to determine
// what state the system is in. The returned state is used to decide which
// state transitions are possible.
//
// The observed state includes:
//   - CollectedAt: Timestamp of this observation
//
// Actions (AuthenticateAction, SyncAction) update the shared observedState,
// and this method returns it to the supervisor.
//
// This method never returns an error for the communicator worker.
func (w *CommunicatorWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	observed := snapshot.CommunicatorObservedState{
		CollectedAt: time.Now(),  // Line 235: Timestamp for staleness checks
	}

	// TODO: Implement proper state collection based on transport state
	// This will need to query the transport for current authentication status
	// and sync health, rather than relying on previousAction pattern

	return observed, nil
}
```

### Key Requirements (from worker.go lines 275-297)

1. **Context Handling (Invariant I6)** - MUST respect context cancellation:
   - Failure to exit after context cancellation will cause panic (enforces lifecycle)
   - Grace period: 5 seconds
   - This is a hard requirement for proper async operation

2. **Timeout Protection**:
   - Wrapped with per-operation timeout (observation interval + cgroup buffer + margin)
   - Default: 1s interval + 200ms cgroup throttle + 1s margin = 2.2s timeout
   - Accounts for Docker/Kubernetes CPU throttling (100ms cgroup period)
   - Operations exceeding timeout are cancelled automatically

3. **Error Handling**:
   - Errors are logged but don't stop the FSM
   - Supervisor handles staleness via FreshnessChecker
   - Repeated timeouts trigger collector restart with backoff

### What Example Workers Should Show

```go
func (w *ExampleWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	// Example 1: Poll external process status
	status, err := checkProcessStatus(ctx)  // MUST respect ctx cancellation
	if err != nil {
		// Errors are logged but don't fail the FSM
		w.logger.Warnf("Failed to collect observed state: %v", err)
		// Return last known state or default observation
		return w.lastKnownState, nil
	}

	// Example 2: Check file existence (fast operation, respects timeout)
	exists, modTime := fileExists(ctx, "/path/to/config")

	return &ExampleObservedState{
		CollectedAt: time.Now(),  // MUST include timestamp for staleness detection
		ProcessRunning: status.Running,
		ProcessPID: status.PID,
		ConfigExists: exists,
		LastModified: modTime,
	}, nil
}
```

---

## 2. COLLECTOBSERVEDSTATE: ERROR HANDLING PATTERN

### What It Enables
Proper error handling in CollectObservedState prevents monitoring failures from cascading into FSM failures. This enables:
- Graceful degradation (FSM continues even if monitoring fails)
- Error logging for debugging without stopping operations
- Automatic recovery (supervisor retries with backoff)

### When to Use
Always implement robust error handling in CollectObservedState because:
- Network calls can timeout
- File I/O can fail
- APIs can return errors
- Processes can disappear unexpectedly
- Partial data collection is acceptable if you return what you can

### How Communicator Demonstrates It

**Lines**: 220-243 (CollectObservedState method, specifically line 232)

```go
// This method never returns an error for the communicator worker.
func (w *CommunicatorWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	observed := snapshot.CommunicatorObservedState{
		CollectedAt: time.Now(),
	}
	
	// Note: Communicator returns nil error here, delegating auth/sync error
	// handling to the state machine via observed state flags
	// (Authenticated, IsTokenExpired, IsSyncHealthy)
	return observed, nil
}
```

### Key Pattern (from worker.go lines 289-292)

```
// Error Handling:
//   - Errors are logged but don't stop the FSM
//   - Supervisor handles staleness via FreshnessChecker
//   - Repeated timeouts trigger collector restart with backoff
```

### What Example Workers Should Show

**Pattern 1: Graceful Degradation with Last Known State**

```go
func (w *ExampleWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	status, err := w.queryAPI(ctx)
	if err != nil {
		w.logger.Warnf("API query failed: %v, using cached state", err)
		// Return last known good state - FSM continues with stale data
		// until next successful collection
		return w.lastObservedState, nil  // ← Return nil error, not error
	}
	
	// Store for next time
	w.lastObservedState = &ExampleObservedState{
		CollectedAt: time.Now(),
		APIHealthy: status.Healthy,
		Latency: status.Latency,
	}
	return w.lastObservedState, nil
}
```

**Pattern 2: Partial Data Collection**

```go
func (w *ExampleWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	// Collect what you can, skip failures gracefully
	var processes []ProcessInfo
	if procData, err := readProcessList(ctx); err != nil {
		w.logger.Warnf("Process list failed: %v", err)
		// Continue without process data
	} else {
		processes = procData
	}
	
	var fileState bool
	if exists, err := checkFile(ctx); err != nil {
		w.logger.Warnf("File check failed: %v", err)
		fileState = false
	} else {
		fileState = exists
	}
	
	return &ExampleObservedState{
		CollectedAt: time.Now(),
		Processes: processes,      // May be empty if collection failed
		FileExists: fileState,
	}, nil  // Always return nil error - state has partial data
}
```

---

## 3. DERIVEDESIREDSTATE: PURE FUNCTION PATTERN

### What It Enables
DeriveDesiredState as a pure function (no side effects, deterministic output) enables:
- Reproducible state computation (same input → same output)
- Easy testing (no mocks, no setup)
- Stateless transformation (can be called repeatedly safely)
- Deterministic FSM behavior (no hidden state affecting decisions)

### When to Use
Implement as pure function when:
- Transforming user configuration to internal representation
- Computing derived values (e.g., location paths, flattened variables)
- Validating configuration against constraints
- Applying templates
- Building hierarchical state structures

### How Communicator Demonstrates It

**File**: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/workers/communicator/worker.go`
**Lines**: 245-261 (DeriveDesiredState method)

```go
// DeriveDesiredState determines what state the communicator should be in.
//
// For MVP, this always returns "not shutdown" - the communicator should
// always be running and syncing. Future versions may derive desired state
// from configuration (e.g., enable/disable sync).
//
// The spec parameter is reserved for future use and currently ignored.
//
// This method never returns an error for the communicator worker.
func (w *CommunicatorWorker) DeriveDesiredState(spec interface{}) (fsmv2types.DesiredState, error) {
	// For now, communicator uses simple desired state without children
	// Future: may populate ChildrenSpecs for sub-components
	return fsmv2types.DesiredState{
		State:         "running",
		ChildrenSpecs: nil,
	}, nil
}
```

**Note**: This is a minimal MVP implementation. Real workers typically do significant computation.

### More Complex Example: TemplateWorker

**File**: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/examples/template_worker.go`
**Lines**: 51-64

```go
func (w *TemplateWorker) DeriveDesiredState(userSpec types.UserSpec) (types.DesiredState, error) {
	// Step 1: Pure transformation - flatten variables (no side effects)
	flattened := userSpec.Variables.Flatten()  // Pure function call

	// Step 2: Pure computation - merge and fill locations (deterministic)
	mergedLocation := location.MergeLocations(w.ParentLocation, w.ChildLocation)
	filledLocation := location.FillISA95Gaps(mergedLocation)
	locationPath := location.ComputeLocationPath(filledLocation)
	flattened["location_path"] = locationPath

	// Step 3: Dispatch to specific implementation based on config
	// (Still pure - no state mutation, no external calls)
	if w.MultiChild {
		return w.deriveMultiChild(flattened)
	}
	return w.deriveSingleChild(flattened)
}

func (w *TemplateWorker) deriveSingleChild(flattened map[string]any) (types.DesiredState, error) {
	// Pure function: render template with flattened variables
	childConfig, err := templating.RenderTemplate(w.Template, flattened)
	if err != nil {
		return types.DesiredState{}, fmt.Errorf("render template: %w", err)
	}

	// Return new DesiredState with rendered config
	return types.DesiredState{
		State: "running",
		ChildrenSpecs: []types.ChildSpec{
			{
				Name:       "mqtt_source",
				WorkerType: "benthos_dataflow",
				UserSpec: types.UserSpec{
					Config: childConfig,
					Variables: types.VariableBundle{
						User: map[string]any{
							"name": "mqtt_source",
						},
					},
				},
			},
		},
	}, nil
}
```

### Key Principles (from worker.go lines 299-310)

```
// DeriveDesiredState transforms user configuration into desired state.
// Pure function - no side effects. Called on each tick.
// The spec parameter comes from user configuration.
//
// This is used for templating, for example to convert user configuration 
// to the actual "technical" template.
//
// Returns concrete types.DesiredState to enable hierarchical composition 
// via ChildrenSpecs field.
```

### What Example Workers Should Show

**Pattern: Pure Transformation with Validation**

```go
func (w *ExampleWorker) DeriveDesiredState(spec interface{}) (types.DesiredState, error) {
	// Step 1: Type assertion (no side effects)
	config, ok := spec.(ExampleConfig)
	if !ok {
		return types.DesiredState{}, fmt.Errorf("invalid spec type")
	}

	// Step 2: Validate configuration (pure check)
	if err := w.validateConfig(config); err != nil {
		return types.DesiredState{}, fmt.Errorf("config validation failed: %w", err)
	}

	// Step 3: Pure computation - derive values from config
	computedAddress := fmt.Sprintf("%s:%d", config.Host, config.Port)
	computedTimeout := time.Duration(config.TimeoutSeconds) * time.Second

	// Step 4: Return immutable state (never mutates input)
	return types.DesiredState{
		State: "running",
		ChildrenSpecs: []types.ChildSpec{
			{
				Name:       "processor_1",
				WorkerType: "data_processor",
				UserSpec: types.UserSpec{
					Config: fmt.Sprintf(`
address: %s
timeout: %s
enable_logging: %v
`, computedAddress, computedTimeout, config.EnableLogging),
				},
			},
		},
	}, nil
}

// Helper: Pure function (no side effects, same input → same output)
func (w *ExampleWorker) validateConfig(config ExampleConfig) error {
	if config.Host == "" {
		return errors.New("host required")
	}
	if config.Port < 1 || config.Port > 65535 {
		return errors.New("invalid port")
	}
	return nil
}
```

---

## 4. DERIVEDESIREDSTATE: TEMPLATING & COMPOSITION PATTERN

### What It Enables
DeriveDesiredState can use ChildrenSpecs to declare child workers, enabling:
- Hierarchical FSM composition (parent declares, supervisor manages children)
- Kubernetes-style declarative management (parent specifies desired children)
- Dynamic child creation (parent creates/removes children based on config)
- Clean separation (parent focuses on orchestration, children on implementation)

### When to Use
Use ChildrenSpecs when:
- Your worker is a container/orchestrator for other workers
- You need to manage multiple sub-components
- Configuration drives the number of children (e.g., 3 bridges → 3 child workers)
- You want supervisor to reconcile actual children with desired specs

### How TemplateWorker Demonstrates It

**File**: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/examples/template_worker.go`
**Lines**: 66-89 (single child) and 91-142 (multiple children)

**Single Child Pattern**:

```go
func (w *TemplateWorker) deriveSingleChild(flattened map[string]any) (types.DesiredState, error) {
	childConfig, err := templating.RenderTemplate(w.Template, flattened)
	if err != nil {
		return types.DesiredState{}, fmt.Errorf("render template: %w", err)
	}

	return types.DesiredState{
		State: "running",
		ChildrenSpecs: []types.ChildSpec{  // ← Declare desired child
			{
				Name:       "mqtt_source",
				WorkerType: "benthos_dataflow",
				UserSpec: types.UserSpec{
					Config: childConfig,  // ← Templated configuration
					Variables: types.VariableBundle{
						User: map[string]any{
							"name": "mqtt_source",
						},
					},
				},
			},
		],
	}, nil
}
```

**Multiple Children Pattern**:

```go
func (w *TemplateWorker) deriveMultiChild(flattened map[string]any) (types.DesiredState, error) {
	mqttTemplate := `
input:
  mqtt:
    urls: ["tcp://{{ .IP }}:{{ .PORT }}"]
`
	kafkaTemplate := `
output:
  kafka:
    topic: "umh.v1.{{ .location_path }}.{{ .name }}"
`

	mqttConfig, err := templating.RenderTemplate(mqttTemplate, flattened)
	if err != nil {
		return types.DesiredState{}, fmt.Errorf("render mqtt template: %w", err)
	}

	kafkaConfig, err := templating.RenderTemplate(kafkaTemplate, flattened)
	if err != nil {
		return types.DesiredState{}, fmt.Errorf("render kafka template: %w", err)
	}

	return types.DesiredState{
		State: "running",
		ChildrenSpecs: []types.ChildSpec{
			// Child 1: MQTT source
			{
				Name:       "mqtt_source",
				WorkerType: "benthos_dataflow",
				UserSpec: types.UserSpec{
					Config: mqttConfig,
					Variables: types.VariableBundle{
						User: map[string]any{"name": "mqtt_source"},
					},
				},
			},
			// Child 2: Kafka sink
			{
				Name:       "kafka_sink",
				WorkerType: "benthos_dataflow",
				UserSpec: types.UserSpec{
					Config: kafkaConfig,
					Variables: types.VariableBundle{
						User: map[string]any{"name": "kafka_sink"},
					},
				},
			},
		},
	}, nil
}
```

### Key Pattern (from worker.go lines 305-310)

```
// Returns concrete types.DesiredState to enable hierarchical composition 
// via ChildrenSpecs field.
// Parent workers can declare child FSM workers by populating ChildrenSpecs, 
// allowing supervisor to reconcile actual children to match desired specs 
// (Kubernetes-style declarative management).
```

### What Example Workers Should Show

**Pattern: Dynamic Child Creation Based on Config**

```go
func (w *OrchestrationWorker) DeriveDesiredState(spec interface{}) (types.DesiredState, error) {
	config, ok := spec.(BridgeConfig)
	if !ok {
		return types.DesiredState{}, fmt.Errorf("invalid spec")
	}

	// Build children dynamically from config
	var children []types.ChildSpec
	for i, endpoint := range config.Endpoints {
		childName := fmt.Sprintf("bridge_%d", i)
		
		// Render configuration for this child
		childConfig, err := templating.RenderTemplate(config.Template, map[string]any{
			"IP":   endpoint.IP,
			"PORT": endpoint.Port,
			"name": childName,
		})
		if err != nil {
			return types.DesiredState{}, fmt.Errorf("render child config: %w", err)
		}

		children = append(children, types.ChildSpec{
			Name:       childName,
			WorkerType: "protocol_converter",
			UserSpec: types.UserSpec{
				Config: childConfig,
				Variables: types.VariableBundle{
					User: map[string]any{
						"endpoint": fmt.Sprintf("%s:%d", endpoint.IP, endpoint.Port),
					},
				},
			},
		})
	}

	return types.DesiredState{
		State:         "running",
		ChildrenSpecs: children,  // ← Supervisor reconciles these
	}, nil
}
```

---

## 5. GETINITIALSTATE: ENTRY POINT PATTERN

### What It Enables
GetInitialState defines the FSM's starting state, enabling:
- Clear initialization (FSM always starts in known, explicit state)
- Flexible startup (can start in any valid state: Stopped, Ready, Error, etc.)
- Deterministic behavior (same initialization every time)

### When to Use
Implement GetInitialState to return:
- **Stopped/Dormant state**: Most common. Waits for first tick before activating.
- **Ready state**: If initialization is already complete (rare).
- **Error state**: If initialization detects problems (e.g., required file missing).

### How Communicator Demonstrates It

**File**: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/workers/communicator/worker.go`
**Lines**: 263-269

```go
// GetInitialState returns the state the FSM should start in.
//
// The communicator always starts in StoppedState. The FSM will transition
// through Authenticating → Authenticated → Syncing based on observed state.
func (w *CommunicatorWorker) GetInitialState() state.BaseCommunicatorState {
	return &state.StoppedState{}
}
```

### Key Pattern (from worker.go lines 312-316)

```
// GetInitialState returns the starting state for this worker.
// Called once during worker creation.
//
// Example: return &InitializingState{} or &StoppedState{}
```

### What Example Workers Should Show

**Pattern 1: Start from Dormant State**

```go
func (w *ExampleWorker) GetInitialState() State {
	// Most common pattern: start stopped, FSM transitions on first tick
	return &StoppedState{}
}
```

**Pattern 2: Start with Conditional Logic**

```go
func (w *ExampleWorker) GetInitialState() State {
	// Can check environment at initialization time
	if w.configFile != "" {
		return &InitializingState{}  // Load config on first tick
	}
	return &StoppedState{}  // No config, stay dormant
}
```

**Pattern 3: Start Ready (rare)**

```go
func (w *ExampleWorker) GetInitialState() State {
	// Only if initialization is synchronous and complete
	// (Most workers should not do this - keep init work in states/actions)
	return &RunningState{}
}
```

---

## 6. SIGNAL TYPES: SUPERVISOR-LEVEL COMMUNICATION

### What It Enables
Signals communicate special FSM-level conditions to the supervisor, enabling:
- Worker removal (graceful cleanup and termination)
- Worker restart (controlled bounce for recovery)
- Normal operation (no special action needed)

### When to Use
- **SignalNone** (most common): Normal state transitions, no special supervisor action
- **SignalNeedsRemoval**: Worker cleanup complete, safe to remove from system
- **SignalNeedsRestart**: Worker needs controlled restart cycle

### How Communicator Demonstrates It

**File**: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/worker.go`
**Lines**: 38-49

```go
// Signal is used by states to communicate special conditions to the supervisor.
// These signals trigger supervisor-level actions beyond normal state transitions.
type Signal int

const (
	// SignalNone indicates normal operation, no special action needed.
	SignalNone Signal = iota
	// SignalNeedsRemoval tells supervisor this worker has completed cleanup and can be removed.
	SignalNeedsRemoval
	// SignalNeedsRestart tells supervisor to initiate shutdown for a restart cycle.
	SignalNeedsRestart
)
```

**Usage in Communicator's StoppedState**:

```go
func (s *StoppedState) Next(snapshot snapshot.CommunicatorSnapshot) (BaseCommunicatorState, fsmv2.Signal, fsmv2.Action) {
	desired := snapshot.Desired

	if desired.ShutdownRequested() {
		return s, fsmv2.SignalNeedsRemoval, nil  // ← Tell supervisor to remove worker
	}

	return &TryingToAuthenticateState{}, fsmv2.SignalNone, nil  // ← Normal transition
}
```

### What Example Workers Should Show

**Pattern: Graceful Shutdown with Cleanup**

```go
// In your shutdown/cleanup state
func (s *CleaningUpState) Next(snapshot Snapshot) (State, Signal, Action) {
	desired := snapshot.Desired
	observed := snapshot.Observed

	// Check if cleanup is still in progress
	if !observed.CleanupComplete {
		// Emit action to continue cleanup
		return s, fsmv2.SignalNone, &CleanupAction{}
	}

	// Cleanup complete, tell supervisor we're done
	return s, fsmv2.SignalNeedsRemoval, nil  // ← Worker will be removed
}
```

**Pattern: Signal Restart Cycle**

```go
func (s *ErrorRecoveryState) Next(snapshot Snapshot) (State, Signal, Action) {
	// After too many retries, restart the worker
	if snapshot.Observed.ConsecutiveErrors > 10 {
		return s, fsmv2.SignalNeedsRestart, nil
	}
	// Otherwise, emit retry action
	return s, fsmv2.SignalNone, &RetryAction{}
}
```

---

## 7. ACTIVE STATE PATTERN: "TryingTo*" NAMING & BEHAVIOR

### What It Enables
Active states (prefix: "TryingTo") emit actions on every tick, enabling:
- Repeated retry loops (action keeps retrying until success)
- Progress monitoring (each tick attempts work)
- Eventual success (loop exits when preconditions are met)
- Clear code semantics (name says "we're trying to X")

### When to Use
Use "TryingTo*" states when:
- Performing operations that may take multiple attempts
- Network connections, authentication, waiting for resources
- Each tick tries again without accumulating side effects
- Success condition is observable in the snapshot

### How Communicator Demonstrates It

**File**: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/workers/communicator/state/state_trying_to_authenticate.go`
**Lines**: 23-114

```go
// TryingToAuthenticateState represents the authentication phase 
// where the worker obtains a JWT token.
//
// # Purpose
//
// This state handles the initial handshake with the relay server 
// to establish authenticated communication. It executes AuthenticateAction 
// to obtain a JWT token required for all subsequent sync operations.
type TryingToAuthenticateState struct {
	BaseCommunicatorState
}

func (s *TryingToAuthenticateState) Next(snapshot snapshot.CommunicatorSnapshot) (BaseCommunicatorState, fsmv2.Signal, fsmv2.Action) {
	desired := snapshot.Desired
	observed := snapshot.Observed

	if desired.ShutdownRequested() {
		return &StoppedState{}, fsmv2.SignalNone, nil
	}

	// Check success condition
	if observed.Authenticated && !observed.IsTokenExpired() {
		return &SyncingState{}, fsmv2.SignalNone, nil  // ← Transition to next state
	}

	// Still trying - emit action to retry
	authenticateAction := action.NewAuthenticateAction(
		desired.Dependencies,
		desired.RelayURL,
		desired.InstanceUUID,
		desired.AuthToken,
	)

	return s, fsmv2.SignalNone, authenticateAction  // ← Loop: returns self + action
}
```

### Key Pattern (from worker.go lines 180-194)

```
// ACTIVE STATES (prefix: "TryingTo")
//   - Emit actions on every tick until success condition met
//   - Examples: TryingToStartState, TryingToStopState
//   - Represent ongoing operations that need retrying
```

### What Example Workers Should Show

**Pattern: Retry Loop Until Success**

```go
// TryingToStartState - attempts to start external process
func (s *TryingToStartState) Next(snapshot Snapshot) (State, Signal, Action) {
	desired := snapshot.Desired
	observed := snapshot.Observed

	// Check shutdown first
	if desired.ShutdownRequested() {
		return &StoppedState{}, fsmv2.SignalNone, nil
	}

	// Check success condition
	if observed.ProcessRunning && observed.ProcessPID > 0 {
		return &RunningState{}, fsmv2.SignalNone, nil  // ← Success, move to next state
	}

	// Still trying - emit action to retry startup
	return s, fsmv2.SignalNone, &StartProcessAction{}  // ← Loop on self
}
```

**Pattern: Timeout-Based Transitions**

```go
func (s *TryingToConnectState) Next(snapshot Snapshot) (State, Signal, Action) {
	desired := snapshot.Desired
	observed := snapshot.Observed

	// Check timeout
	elapsedTime := time.Since(observed.ConnectionStartedAt)
	if elapsedTime > 30*time.Second {
		return &ConnectionFailedState{}, fsmv2.SignalNone, nil  // ← Timeout reached
	}

	// Still trying
	if observed.Connected {
		return &ConnectedState{}, fsmv2.SignalNone, nil  // ← Success
	}

	return s, fsmv2.SignalNone, &ReconnectAction{}  // ← Loop: retry
}
```

---

## 8. PASSIVE STATE PATTERN: DESCRIPTIVE NOUN NAMING & BEHAVIOR

### What It Enables
Passive states (descriptive nouns) only observe and transition, enabling:
- Stable conditions (state represents steady-state operation)
- No action emission (FSM just monitors, doesn't push)
- Clean semantics (name describes what we ARE, not what we're TRYING)
- Guard conditions (transitions only occur when conditions change)

### When to Use
Use descriptive noun states when:
- Representing stable conditions (Running, Stopped, Connected)
- Monitoring for condition changes (DegradedState watches for recovery)
- No action needed to maintain state
- Transitions happen when observed state changes

### How Communicator Demonstrates It

**File**: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/workers/communicator/state/state_stopped.go`
**Lines**: 22-90

```go
// StoppedState represents the initial dormant state before any 
// communication with the relay server.
//
// # Purpose
//
// This is the entry point for all communicator workers, returned by 
// worker.GetInitialState(). In this state, no authentication attempts 
// occur, no sync operations run, and no network communication happens.
type StoppedState struct {
	BaseCommunicatorState
}

func (s *StoppedState) Next(snapshot snapshot.CommunicatorSnapshot) (BaseCommunicatorState, fsmv2.Signal, fsmv2.Action) {
	desired := snapshot.Desired

	if desired.ShutdownRequested() {
		return s, fsmv2.SignalNeedsRemoval, nil  // ← No action, just signal
	}

	return &TryingToAuthenticateState{}, fsmv2.SignalNone, nil  // ← No action, just transition
}
```

**File**: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/workers/communicator/state/state_syncing.go`
**Lines**: 23-135

```go
// SyncingState represents the primary operational state for bidirectional 
// message synchronization.
//
// # Purpose
//
// This is the steady-state mode of the communicator worker, where continuous 
// bidirectional message exchange occurs between edge and backend tiers.
//
// This state is designed to run indefinitely once authentication succeeds, 
// only exiting on token expiry, authentication loss, sync health degradation, 
// or shutdown requests.
type SyncingState struct {
}

func (s *SyncingState) Next(snapshot snapshot.CommunicatorSnapshot) (BaseCommunicatorState, fsmv2.Signal, fsmv2.Action) {
	desired := snapshot.Desired
	observed := snapshot.Observed

	if desired.ShutdownRequested() {
		return &StoppedState{}, fsmv2.SignalNone, nil  // ← Check conditions first
	}

	if observed.IsTokenExpired() {
		return &TryingToAuthenticateState{}, fsmv2.SignalNone, nil  // ← On condition change
	}

	if !observed.Authenticated {
		return &TryingToAuthenticateState{}, fsmv2.SignalNone, nil  // ← On condition change
	}

	if !observed.IsSyncHealthy() {
		return &DegradedState{}, fsmv2.SignalNone, nil  // ← Monitor health
	}

	// Still healthy - emit action to continue syncing
	syncAction := action.NewSyncAction(
		desired.Dependencies, observed.JWTToken)

	return s, fsmv2.SignalNone, syncAction  // ← Loop while conditions hold
}
```

### Key Pattern (from worker.go lines 180-194)

```
// PASSIVE STATES (descriptive nouns)
//   - Only observe and transition based on conditions
//   - Examples: RunningState, StoppedState, DegradedState
//   - Represent stable conditions where no action needed
```

### What Example Workers Should Show

**Pattern: Condition Monitoring**

```go
// RunningState - monitors if process is still running
func (s *RunningState) Next(snapshot Snapshot) (State, Signal, Action) {
	desired := snapshot.Desired
	observed := snapshot.Observed

	// Check shutdown first
	if desired.ShutdownRequested() {
		return &StoppingState{}, fsmv2.SignalNone, nil
	}

	// Monitor for condition changes
	if !observed.ProcessRunning {
		return &CrashedState{}, fsmv2.SignalNone, nil  // ← Process died
	}

	if observed.ResourceUsageExceeded {
		return &OverloadedState{}, fsmv2.SignalNone, nil  // ← Resource limit exceeded
	}

	// Everything still running - no action, just loop
	return s, fsmv2.SignalNone, nil  // ← Passive: observe only
}

// DegradedState - monitors for recovery
func (s *DegradedState) Next(snapshot Snapshot) (State, Signal, Action) {
	desired := snapshot.Desired
	observed := snapshot.Observed

	if desired.ShutdownRequested() {
		return &StoppedState{}, fsmv2.SignalNone, nil
	}

	// Wait for recovery
	if observed.IsSyncHealthy() && observed.ConsecutiveErrors == 0 {
		return &SyncingState{}, fsmv2.SignalNone, nil  // ← Recovered
	}

	// Still degraded - emit recovery action
	return s, fsmv2.SignalNone, &RecoveryAction{}
}
```

---

## 9. STATE TRANSITIONS: EXPLICIT PATTERNS & GUARD CONDITIONS

### What It Enables
Explicit, condition-based state transitions enable:
- Deterministic FSM behavior (transitions are predicable from code)
- Visible logic (all transitions explicit in Next() method)
- Guard conditions (explicit checks prevent invalid transitions)
- Priority ordering (conditions checked in priority order)

### When to Use
Implement state transitions with:
- **First check**: Shutdown signal (Invariant I4 from worker.go)
- **Then check**: Success conditions (move to next operational state)
- **Then check**: Failure conditions (move to error/degraded state)
- **Then emit action** or loop on self

### How Communicator Demonstrates It

**File**: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/workers/communicator/state/state_trying_to_authenticate.go`
**Lines**: 85-106

```go
func (s *TryingToAuthenticateState) Next(snapshot snapshot.CommunicatorSnapshot) (BaseCommunicatorState, fsmv2.Signal, fsmv2.Action) {
	desired := snapshot.Desired
	observed := snapshot.Observed

	// Priority 1: Check shutdown first (Invariant C4/I4)
	if desired.ShutdownRequested() {
		return &StoppedState{}, fsmv2.SignalNone, nil
	}

	// Priority 2: Check success condition
	if observed.Authenticated && !observed.IsTokenExpired() {
		return &SyncingState{}, fsmv2.SignalNone, nil
	}

	// Priority 3: Default - emit action to retry
	authenticateAction := action.NewAuthenticateAction(
		desired.Dependencies,
		desired.RelayURL,
		desired.InstanceUUID,
		desired.AuthToken,
	)

	return s, fsmv2.SignalNone, authenticateAction
}
```

### Key Pattern (from worker.go lines 170-250)

```
// States are stateless - they examine the snapshot and decide what happens next.
//
// Key principles:
//   - States MUST handle ShutdownRequested first
//   - State transitions are explicit and visible in code
//   - States return actions for side effects, not perform them
```

### What Example Workers Should Show

**Pattern: Ordered Condition Checks**

```go
func (s *ProcessState) Next(snapshot Snapshot) (State, Signal, Action) {
	desired := snapshot.Desired
	observed := snapshot.Observed

	// 1. PRIORITY: Always check shutdown first (Invariant I4)
	if desired.ShutdownRequested() {
		return &StoppingState{}, fsmv2.SignalNone, nil
	}

	// 2. CRITICAL: Check for fatal errors
	if observed.HasFatalError {
		return &FailedState{}, fsmv2.SignalNone, nil
	}

	// 3. OPERATIONAL: Check success conditions
	if observed.ProcessRunning && observed.ConfigurationValid {
		return &RunningState{}, fsmv2.SignalNone, nil
	}

	// 4. RECOVERY: Check for recoverable conditions
	if observed.Retries < 3 {
		return s, fsmv2.SignalNone, &RetryAction{}
	}

	// 5. DEFAULT: Give up, move to error state
	return &FailedState{}, fsmv2.SignalNone, nil
}
```

**Pattern: Guard Conditions Prevent Invalid Transitions**

```go
// DegradedState enforces that we can't go back to Syncing without recovery
func (s *DegradedState) Next(snapshot Snapshot) (State, Signal, Action) {
	desired := snapshot.Desired
	observed := snapshot.Observed

	if desired.ShutdownRequested() {
		return &StoppedState{}, fsmv2.SignalNone, nil
	}

	// Guard: Can only exit degraded if healthy AND no errors
	// (prevents thrashing between states)
	if !observed.IsSyncHealthy() {
		return s, fsmv2.SignalNone, &RecoveryAction{}  // Still degraded
	}

	// Now safe to return to healthy state
	return &SyncingState{}, fsmv2.SignalNone, nil
}
```

---

## 10. SNAPSHOT IMMUTABILITY: PASS-BY-VALUE GUARANTEES

### What It Enables
Snapshot passed by value ensures:
- Pure functional transitions (states can't affect supervisor's snapshot)
- Safe goroutine operations (multiple states can examine same snapshot safely)
- No defensive copying needed (Go's language design enforces it)
- Clear contract (transitions cannot have side effects on input)

### When to Use
Always rely on immutability when:
- Examining snapshot in Next() method
- Making decisions based on snapshot fields
- Returning new state based on snapshot comparison
- Never mutate the snapshot you receive

### How FSM v2 Enforces It

**File**: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/worker.go`
**Lines**: 82-114

```go
// Snapshot is the complete view of the worker at a point in time.
// The supervisor assembles this from the database and passes it to State.Next().
// This enables pure functional state transitions based on complete information.
//
// IMMUTABILITY (Invariant I9):
// Snapshot is passed by value to State.Next(), making it inherently immutable.
// States receive a COPY of the snapshot, so mutations don't affect the original.
// This guarantees that state transitions are pure functions without side effects.
//
// Go's pass-by-value semantics enforce this at the language level:
//   - When State.Next(snapshot Snapshot) is called, Go copies the struct
//   - Fields (Identity, Observed, Desired) are copied as interface pointers
//   - States can mutate their local copy without affecting supervisor's snapshot
//   - No runtime validation needed - the compiler enforces this
//
// Example showing immutability in practice:
//
//	func (s MyState) Next(snapshot Snapshot) (State, Signal, Action) {
//	    // snapshot is a copy - mutations here don't affect supervisor's snapshot
//	    snapshot.Observed = nil  // This only affects the local copy
//	    snapshot.Identity.Name = "modified"  // Local copy only
//	    return s, SignalNone, nil
//	}
//
// Defense-in-depth layers:
//   - Layer 1: Pass-by-value (Go language design)
//   - Layer 2: Documentation (this godoc)
//   - Layer 3: Tests demonstrating immutability (supervisor/immutability_test.go)
type Snapshot struct {
	Identity Identity    // Who am I?
	Observed interface{} // What is the actual state? (ObservedState or basic.Document)
	Desired  interface{} // What should the state be? (DesiredState or basic.Document)
}
```

**State.Next() Signature** (lines 210-241):

```go
type State interface {
	// Next evaluates the snapshot and returns the next transition.
	// This is a pure function - no side effects, no external calls.
	// The supervisor calls this on each tick (e.g., every second).
	//
	// IMMUTABILITY (Invariant I9):
	// The snapshot parameter is passed by value (copied), so any modifications
	// to it within Next() do not affect the supervisor's original snapshot.
	// This enforces immutability and enables pure functional transitions.
	//
	// Go's pass-by-value semantics guarantee:
	//   - snapshot is a COPY of the supervisor's snapshot
	//   - Mutations to snapshot only affect this local copy
	//   - The supervisor's snapshot remains unchanged
	//   - No defensive copying or validation needed
	//
	// Returns:
	//   - nextState: State to transition to (can return self to stay)
	//   - signal: Optional signal to supervisor (usually SignalNone)
	//   - action: Optional action to execute before next tick (can be nil)
	//
	// Only returns new state when all conditions are met.
	// Should not switch the state and emit an action at the same time 
	// (supervisor should check for this and panic if this happens as this is an 
	// application logic issue).
	//
	// Supervisor will only call Next() if there is no ongoing action 
	// (to prevent multiple actions).
	Next(snapshot Snapshot) (State, Signal, Action)
}
```

### Key Principle: Language-Level Enforcement

The immutability guarantee is enforced at the Go language level:
1. Go passes struct values by copying them
2. The supervisor's snapshot is not affected by state mutations
3. No additional runtime checks needed
4. Compiler guarantees this, not runtime

### What Example Workers Should Show

**Pattern: Don't Mutate Snapshot**

```go
// GOOD: Use snapshot as read-only reference
func (s *MyState) Next(snapshot Snapshot) (State, Signal, Action) {
	// Snapshot is immutable - read from it safely
	if snapshot.Observed.IsHealthy {
		return &HealthyState{}, fsmv2.SignalNone, nil
	}
	return s, fsmv2.SignalNone, &HealthCheckAction{}
}

// BAD: Don't try to mutate snapshot (it won't affect supervisor's copy)
func (s *MyState) Next(snapshot Snapshot) (State, Signal, Action) {
	snapshot.Observed = nil  // WRONG - mutates only local copy, useless
	// This doesn't break anything, but it's unnecessary and confusing
	return s, fsmv2.SignalNone, nil
}
```

**Pattern: Snapshot Comparison is Safe**

```go
func (s *ConfiguredState) Next(snapshot Snapshot) (State, Signal, Action) {
	observed := snapshot.Observed.(*MyObservedState)
	desired := snapshot.Desired.(*MyDesiredState)

	// Safe to compare - snapshot is immutable
	if observed.ConfigHash != desired.ConfigHash {
		return &ReconfigState{}, fsmv2.SignalNone, &ReconfigAction{}
	}

	return s, fsmv2.SignalNone, nil
}
```

---

## 11. ACTION IDEMPOTENCY: THE CRITICAL INVARIANT I10

### What It Enables
Idempotent actions enable:
- Safe retries on failure (action can be called multiple times)
- Network resilience (transient failures don't corrupt state)
- Partial failure recovery (action can resume from checkpoints)
- Guaranteed eventual consistency (eventual success despite failures)

### When to Use
ALL actions must be idempotent because:
- Supervisor retries failed actions with exponential backoff
- Network issues cause transient failures
- Process crashes may interrupt execution
- Same action may be called many times before FSM sees result

### How FSM v2 Requires It

**File**: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/worker.go`
**Lines**: 116-167

```go
// Action represents a side effect that transitions the system between states.
// Actions are executed by the supervisor after State.Next() returns them.
// They can be long-running and will be retried with backoff on failure.
// Actions MUST be idempotent - safe to retry after partial completion.
//
// IDEMPOTENCY REQUIREMENT (Invariant I10):
// Actions MUST be safe to call multiple times. Each action implementation should:
//   1. Check if work is already done before performing it
//   2. Produce the same final state whether called once or multiple times
//   3. Handle partial completion gracefully (retry from checkpoint)
//
// Example idempotent action:
//
//	func (a *CreateFileAction) Execute(ctx context.Context) error {
//	    // Check if already done
//	    if fileExists(a.path) {
//	        return nil  // Already created, idempotent
//	    }
//	    return createFile(a.path, a.content)
//	}
//
// Example NON-idempotent action (DO NOT DO THIS):
//
//	func (a *IncrementCounterAction) Execute(ctx context.Context) error {
//	    counter++  // WRONG! Multiple calls increment multiple times
//	    return nil
//	}
//
// Testing idempotency:
// Use the idempotency test helper in supervisor/action_helpers_test.go:
//
//	VerifyActionIdempotency(action, 3, func() {
//	    Expect(fileExists("test.txt")).To(BeTrue())
//	})
//
// REQUIREMENT (FSM v2): Every Action implementation MUST have an idempotency test.
// Code reviewers: Check that action_*_test.go files use VerifyActionIdempotency.
//
// Defense-in-depth layers:
//   - Layer 1: Document requirement in Action interface
//   - Layer 2: Provide test helpers for verification
//   - Layer 3: Examples showing idempotent patterns
//   - Layer 4: Retry logic in executeActionWithRetry validates this
//
// Example: StartProcess, StopProcess, CreateConfigFiles, CallAPI.
type Action interface {
	// Execute performs the action. Can be blocking and long-running.
	// Must handle context cancellation. Must be idempotent.
	Execute(ctx context.Context) error
	// Name returns a descriptive name for logging/debugging
	Name() string
}
```

### How Communicator Demonstrates It

**File**: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/workers/communicator/action/sync.go`
**Lines**: 26-114

```go
// SyncAction performs bidirectional message synchronization via HTTP transport.
//
// # Idempotency guarantee:
//   - Calling Execute() multiple times is safe
//   - Duplicate message detection handled by backend (not by FSM)
//   - Messages are not reprocessed within same tick
//
// Returns an error if:
//   - Context is cancelled
//   - HTTP transport fails critically (e.g., network unreachable)
//   - Authentication token expired (triggers re-authentication)
//
// Non-critical failures (e.g., channel full) are logged but not returned.
// This allows the sync loop to continue even if some operations fail.
func (a *SyncAction) Execute(ctx context.Context) error {
	// 1. Pull messages from backend (idempotent - just GET)
	messages, err := a.dependencies.GetTransport().Pull(ctx, a.JWTToken)
	if err != nil {
		return fmt.Errorf("pull failed: %w", err)
	}

	// 2. Store pulled messages (they will be available in next observed state)
	_ = messages

	// 3. Push batch to backend if we have messages (idempotent - message dedup on backend)
	if len(a.MessagesToBePushed) > 0 {
		if err := a.dependencies.GetTransport().Push(ctx, a.JWTToken, a.MessagesToBePushed); err != nil {
			return fmt.Errorf("push failed: %w", err)
		}
	}

	return nil
}

func (a *SyncAction) Name() string {
	return SyncActionName
}
```

### Testing Idempotency Pattern

**File**: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/supervisor/execution/action_idempotency_test.go`
**Lines**: 24-46

```go
var _ = Describe("Action Idempotency Pattern (I10)", func() {
	Describe("Example idempotent action", func() {
		It("should demonstrate idempotency pattern", func() {
			action := &exampleIdempotentAction{value: 42}

			// Helper: Call action multiple times, verify final state is identical
			VerifyActionIdempotency(action, 5, func() {
				Expect(action.executionCount).To(BeNumerically(">=", 5))
				Expect(action.finalValue).To(Equal(42))
			})
		})
	})

	Describe("Counter-example: non-idempotent action", func() {
		It("should show why non-idempotent actions fail", func() {
			action := &exampleNonIdempotentAction{}

			for i := 0; i < 3; i++ {
				_ = action.Execute(context.Background())
			}

			Expect(action.counter).To(Equal(3), 
				"Non-idempotent action increments on each call - WRONG!")
		})
	})
})
```

### What Example Workers Should Show

**Pattern 1: Check-then-act for idempotency**

```go
// CreateFileAction is idempotent: safe to call multiple times
type CreateFileAction struct {
	Path    string
	Content string
}

func (a *CreateFileAction) Execute(ctx context.Context) error {
	// Step 1: Check if already done
	if _, err := os.Stat(a.Path); err == nil {
		// File already exists - idempotent return
		return nil
	}

	// Step 2: Create if not done
	return os.WriteFile(a.Path, []byte(a.Content), 0644)
}

func (a *CreateFileAction) Name() string {
	return "CreateFileAction"
}
```

**Pattern 2: Idempotent GET/POST operations**

```go
// RegisterWithServerAction is idempotent: backend deduplicates
type RegisterWithServerAction struct {
	ServerURL string
	ClientID  string
}

func (a *RegisterWithServerAction) Execute(ctx context.Context) error {
	// GET /is-registered?client_id=X to check if already done
	resp, err := http.Get(a.ServerURL + "/is-registered?client_id=" + a.ClientID)
	if err == nil && resp.StatusCode == 200 {
		// Already registered - idempotent return
		return nil
	}

	// POST /register (idempotent on server - deduplicates by ClientID)
	body := fmt.Sprintf(`{"client_id": "%s"}`, a.ClientID)
	resp, err = http.Post(a.ServerURL+"/register", "application/json", 
		strings.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (a *RegisterWithServerAction) Name() string {
	return "RegisterWithServerAction"
}
```

**Pattern 3: Checkpoint-based recovery for long operations**

```go
// BackupDataAction is idempotent: can resume from checkpoint
type BackupDataAction struct {
	DataDir  string
	BackupDir string
	
	// Checkpoint tracking (safe to retry)
	completedFiles map[string]bool
}

func (a *BackupDataAction) Execute(ctx context.Context) error {
	files, err := ioutil.ReadDir(a.DataDir)
	if err != nil {
		return err
	}

	for _, file := range files {
		// Skip already backed up files
		if a.completedFiles[file.Name()] {
			continue
		}

		// Backup this file
		if err := a.backupFile(ctx, file); err != nil {
			// Partial failure - next retry will skip completed files
			return fmt.Errorf("backup %s failed: %w", file.Name(), err)
		}

		// Mark as done
		a.completedFiles[file.Name()] = true
	}

	return nil
}

func (a *BackupDataAction) backupFile(ctx context.Context, file os.FileInfo) error {
	// Implementation
	return nil
}

func (a *BackupDataAction) Name() string {
	return "BackupDataAction"
}
```

---

## 12. ACTION CONTEXT CANCELLATION: GRACEFUL ASYNC SHUTDOWN

### What It Enables
Context cancellation in Execute() enables:
- Graceful shutdown (supervisor can cancel long-running actions)
- Timeout handling (context deadlines enforce operation limits)
- Resource cleanup (cancelled operations release locks/connections)
- Clean goroutine termination (no zombie goroutines)

### When to Use
ALWAYS handle context cancellation when:
- Making blocking I/O calls (network, file, database)
- Spawning goroutines
- Using time.Sleep or time.After
- Any operation that could block indefinitely

### How Communicator Demonstrates It

**File**: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/workers/communicator/action/sync.go`
**Lines**: 96-114

```go
// Execute performs a sync tick using HTTP push/pull operations.
//
// Context Handling:
//   - ctx parameter allows supervisor to cancel operation on shutdown
//   - Must respect context cancellation
//   - HTTPTransport.Pull() and HTTPTransport.Push() handle ctx internally
//
func (a *SyncAction) Execute(ctx context.Context) error {
	// All operations must respect context cancellation
	messages, err := a.dependencies.GetTransport().Pull(ctx, a.JWTToken)  // ← ctx passed
	if err != nil {
		return fmt.Errorf("pull failed: %w", err)
	}

	if len(a.MessagesToBePushed) > 0 {
		if err := a.dependencies.GetTransport().Push(ctx, a.JWTToken, a.MessagesToBePushed); err != nil {  // ← ctx passed
			return fmt.Errorf("push failed: %w", err)
		}
	}

	return nil
}
```

### Key Requirement (from worker.go lines 162-164)

```
// Execute performs the action. Can be blocking and long-running.
// Must handle context cancellation. Must be idempotent.
Execute(ctx context.Context) error
```

### What Example Workers Should Show

**Pattern 1: Simple blocking operation with context**

```go
type ConnectToServiceAction struct {
	ServiceURL string
}

func (a *ConnectToServiceAction) Execute(ctx context.Context) error {
	// Create request with context (respects cancellation)
	req, err := http.NewRequestWithContext(ctx, "POST", a.ServiceURL, nil)
	if err != nil {
		return err
	}

	// Client respects context deadlines
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		// Context cancellation returns context.Canceled
		if errors.Is(err, context.Canceled) {
			return fmt.Errorf("operation cancelled: %w", err)
		}
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (a *ConnectToServiceAction) Name() string {
	return "ConnectToServiceAction"
}
```

**Pattern 2: Goroutine with context cancellation**

```go
type StreamDataAction struct {
	Sources []string
}

func (a *StreamDataAction) Execute(ctx context.Context) error {
	results := make(chan error, len(a.Sources))
	
	for _, source := range a.Sources {
		go func(src string) {
			// Goroutine respects context cancellation
			select {
			case <-ctx.Done():
				results <- ctx.Err()
				return
			default:
			}
			
			// Do work
			results <- a.streamFromSource(ctx, src)
		}(source)
	}

	// Wait for all goroutines to complete or context to cancel
	for i := 0; i < len(a.Sources); i++ {
		select {
		case err := <-results:
			if err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

func (a *StreamDataAction) streamFromSource(ctx context.Context, source string) error {
	// Implementation respects ctx
	return nil
}

func (a *StreamDataAction) Name() string {
	return "StreamDataAction"
}
```

**Pattern 3: Timeout-aware operation**

```go
type LongRunningTaskAction struct {
	TaskID string
}

func (a *LongRunningTaskAction) Execute(ctx context.Context) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Supervisor cancelled - clean shutdown
			return ctx.Err()
		case <-ticker.C:
			// Perform periodic work
			status, err := a.checkTaskProgress(ctx, a.TaskID)
			if err != nil {
				return err
			}
			if status.Complete {
				return nil  // Success
			}
		}
	}
}

func (a *LongRunningTaskAction) checkTaskProgress(ctx context.Context, taskID string) (*TaskStatus, error) {
	// Implementation respects ctx
	return nil, nil
}

func (a *LongRunningTaskAction) Name() string {
	return "LongRunningTaskAction"
}
```

---

## 13. OBSERVEDSTATE INTERFACE IMPLEMENTATION

### What It Enables
ObservedState interface implementation enables:
- State inspection by supervisor
- Staleness detection (timestamp checking)
- State-to-state comparison (observed vs desired)
- Type-safe state access

### When to Use
Implement ObservedState interface when creating custom observed state structs that:
- Are returned from CollectObservedState()
- Contain observation timestamps
- Embed or reference DesiredState for comparison
- Need to be passed to State.Next()

### How FSM v2 Defines It

**File**: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/worker.go`
**Lines**: 59-71

```go
// ObservedState represents the actual state gathered from monitoring the system.
// Implementations should include timestamps to detect staleness.
// The supervisor collects this via CollectObservedState() in a separate goroutine.
type ObservedState interface {
	// GetObservedDesiredState returns the desired state that is actually deployed.
	// This allows comparing what's deployed vs what we want to deploy.
	// It is required to enforce that everything we configure should also be read back 
	// to double-check it.
	GetObservedDesiredState() DesiredState

	// GetTimestamp returns the time when this observed state was collected,
	// used for staleness checks.
	GetTimestamp() time.Time
}
```

### How Communicator Demonstrates It

**File**: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/workers/communicator/snapshot/snapshot.go`
**Lines**: 99-167

```go
// CommunicatorObservedState represents the current state of the communicator.
//
// This structure is created by Worker.CollectObservedState() on every supervisor
// tick and passed to states for decision-making. It is immutable after creation.
type CommunicatorObservedState struct {
	CollectedAt time.Time

	// DesiredState
	CommunicatorDesiredState

	// Authentication
	Authenticated bool
	JWTToken      string
	JWTExpiry     time.Time

	// Inbound Messages
	MessagesReceived []transport.UMHMessage
}

// Implement ObservedState interface
func (o CommunicatorObservedState) IsTokenExpired() bool {
	return time.Now().After(o.JWTExpiry)
}

func (o CommunicatorObservedState) IsSyncHealthy() bool {
	return o.Authenticated && !o.IsTokenExpired()
}

func (o CommunicatorObservedState) GetConsecutiveErrors() int {
	return 0
}

func (o CommunicatorObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &o.CommunicatorDesiredState
}

func (o CommunicatorObservedState) GetTimestamp() time.Time {
	return o.CollectedAt
}
```

### What Example Workers Should Show

**Pattern: Complete ObservedState Implementation**

```go
// ExampleObservedState implements fsmv2.ObservedState interface
type ExampleObservedState struct {
	CollectedAt time.Time  // REQUIRED: Timestamp for staleness detection

	// Embedded or included desired state for comparison
	CurrentDesiredState *ExampleDesiredState

	// Actual observations
	ProcessRunning    bool
	ProcessPID        int32
	ProcessMemoryMB   int64
	ProcessCPUPercent float32
	
	ConfigHash        string
	LastConfigUpdate  time.Time
	
	ErrorCount        int
	LastError         string
	LastErrorTime     time.Time
}

// REQUIRED: GetTimestamp implements ObservedState interface
func (o *ExampleObservedState) GetTimestamp() time.Time {
	return o.CollectedAt
}

// REQUIRED: GetObservedDesiredState implements ObservedState interface
func (o *ExampleObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return o.CurrentDesiredState
}

// OPTIONAL: Helper methods for state queries
func (o *ExampleObservedState) IsProcessHealthy() bool {
	return o.ProcessRunning && o.ErrorCount < 3
}

func (o *ExampleObservedState) IsStale() bool {
	return time.Since(o.CollectedAt) > 5*time.Second
}

func (o *ExampleObservedState) HasRecentErrors() bool {
	return time.Since(o.LastErrorTime) < 1*time.Minute
}
```

---

## SUMMARY: MINIMUM VIABLE EXAMPLE WORKER

To create an example worker that demonstrates ALL 13 capabilities, implement:

### 1. Worker Interface (3 methods)

```go
// GetInitialState (Capability 5)
func (w *ExampleWorker) GetInitialState() State {
	return &InitialState{}
}

// CollectObservedState (Capabilities 1, 2)
func (w *ExampleWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	// Demonstrate: async timeout (ctx), error handling, timestamp
	return &ExampleObservedState{CollectedAt: time.Now()}, nil
}

// DeriveDesiredState (Capabilities 3, 4)
func (w *ExampleWorker) DeriveDesiredState(spec interface{}) (types.DesiredState, error) {
	// Demonstrate: pure function, templating, composition
	return types.DesiredState{State: "running"}, nil
}
```

### 2. States (Capabilities 6, 7, 8, 9)

```go
// Active state: TryingToDoX (Capability 7)
func (s *TryingToStartState) Next(snapshot Snapshot) (State, Signal, Action) {
	if snapshot.Desired.ShutdownRequested() {
		return &StoppedState{}, fsmv2.SignalNeedsRemoval, nil
	}
	if snapshot.Observed.Running {
		return &RunningState{}, fsmv2.SignalNone, nil
	}
	return s, fsmv2.SignalNone, &StartAction{}
}

// Passive state: DescriptiveNoun (Capability 8)
func (s *RunningState) Next(snapshot Snapshot) (State, Signal, Action) {
	if snapshot.Desired.ShutdownRequested() {
		return &StoppingState{}, fsmv2.SignalNone, nil
	}
	if !snapshot.Observed.Running {
		return &CrashedState{}, fsmv2.SignalNone, nil
	}
	return s, fsmv2.SignalNone, nil  // Passive: no action
}
```

### 3. Actions (Capabilities 11, 12)

```go
// Idempotent action with context handling
func (a *StartAction) Execute(ctx context.Context) error {
	// Check if already done (Capability 11: Idempotency)
	if processAlreadyRunning(ctx) {
		return nil
	}
	
	// Respect context cancellation (Capability 12)
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	
	return startProcess(ctx)
}
```

### 4. ObservedState (Capability 13)

```go
type ExampleObservedState struct {
	CollectedAt time.Time
	Running     bool
	// ... other fields
}

func (o *ExampleObservedState) GetTimestamp() time.Time {
	return o.CollectedAt
}

func (o *ExampleObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &ExampleDesiredState{}
}
```

### 5. Snapshot Immutability Test (Capability 10)

```go
func (s *MyState) Next(snapshot Snapshot) (State, Signal, Action) {
	// Demonstrate that snapshot is read-only
	// Any mutations only affect local copy
	return nextState, fsmv2.SignalNone, nil
}
```

---

## CHECKLIST FOR EXAMPLE WORKER IMPLEMENTATION

- [ ] Capability 1: CollectObservedState respects context, handles timeouts
- [ ] Capability 2: CollectObservedState handles errors gracefully
- [ ] Capability 3: DeriveDesiredState is pure function (no side effects)
- [ ] Capability 4: DeriveDesiredState demonstrates templating or composition
- [ ] Capability 5: GetInitialState returns clear starting state
- [ ] Capability 6: States use Signal types correctly (SignalNone, SignalNeedsRemoval, etc.)
- [ ] Capability 7: At least one "TryingTo*" state that emits actions on every tick
- [ ] Capability 8: At least one passive state that observes without action
- [ ] Capability 9: State transitions with explicit guard conditions, shutdown check first
- [ ] Capability 10: Snapshot handling is read-only (demonstrated in code)
- [ ] Capability 11: All actions are idempotent (check-then-act pattern)
- [ ] Capability 12: All actions handle context cancellation
- [ ] Capability 13: ObservedState implements interface (GetTimestamp, GetObservedDesiredState)
- [ ] All actions have idempotency tests using VerifyActionIdempotency helper

---

END OF ANALYSIS
