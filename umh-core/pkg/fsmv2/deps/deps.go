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

package deps

import (
	"context"

	"go.uber.org/zap"
)

// Identity uniquely identifies a worker instance.
// This is immutable for the lifetime of the worker.
type Identity struct {
	ID            string `json:"id"`            // Unique identifier (e.g., UUID).
	Name          string `json:"name"`          // Human-readable name.
	WorkerType    string `json:"workerType"`    // Type of worker (e.g., "container", "pod").
	HierarchyPath string `json:"hierarchyPath"` // Full path from root: "scenario123(application)/parent-123(parent)/child001(child)".
}

// String implements fmt.Stringer for logging purposes.
// Returns HierarchyPath if available, falls back to "ID(Type)" for root workers.
func (i Identity) String() string {
	if i.HierarchyPath != "" {
		return i.HierarchyPath
	}

	if i.ID != "" && i.WorkerType != "" {
		return i.ID + "(" + i.WorkerType + ")"
	}

	return "unknown"
}

// StateReader provides read-only access to the TriangularStore for workers.
// Workers can query their own previous observed state (for state change detection)
// and query children's observed states (for richer parent-child coordination).
//
// IMPORTANT: This is READ-ONLY. Workers MUST NOT write to the store directly.
// All writes go through the supervisor's collector.
type StateReader interface {
	// LoadObservedTyped loads the observed state for a worker into the provided pointer.
	// Use this to:
	//   - Query your own previous observed state (for state change detection)
	//   - Query children's observed states (for parent-child coordination)
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeouts
	//   - workerType: Type of worker (e.g., "exampleparent", "examplechild")
	//   - id: Worker's unique identifier
	//   - result: Pointer to struct where result will be unmarshaled
	//
	// Returns error if the observed state doesn't exist or unmarshaling fails.
	LoadObservedTyped(ctx context.Context, workerType string, id string, result interface{}) error
}

// Dependencies provides access to worker-specific tools for actions.
// All worker dependencies embed BaseDependencies and extend with worker-specific tools.
type Dependencies interface {
	GetLogger() *zap.SugaredLogger
	// ActionLogger returns a logger enriched with action context.
	// Use this when logging from within an action for consistent structured logs.
	ActionLogger(actionType string) *zap.SugaredLogger
	// GetStateReader returns read-only access to the TriangularStore.
	// Returns nil if no StateReader was provided during construction.
	GetStateReader() StateReader
}

// BaseDependencies provides common tools for all workers.
// Worker-specific dependencies should embed this struct.
type BaseDependencies struct {
	stateReader     StateReader
	logger          *zap.SugaredLogger
	metricsRecorder *MetricsRecorder
	frameworkState  *FrameworkMetrics // Set by supervisor before collection, may be stale (~1 tick)
	workerType      string
	workerID        string
	actionHistory   []ActionResult // Set by supervisor before CollectObservedState, read-only for workers
}

// NewBaseDependencies creates a new base dependencies with common tools.
// The logger is automatically enriched with hierarchical worker context using identity.String().
// The stateReader can be nil if state access is not needed.
func NewBaseDependencies(logger *zap.SugaredLogger, stateReader StateReader, identity Identity) *BaseDependencies {
	if logger == nil {
		panic("NewBaseDependencies: logger cannot be nil")
	}

	return &BaseDependencies{
		logger:          logger.With("worker", identity.String()),
		stateReader:     stateReader,
		metricsRecorder: NewMetricsRecorder(),
		actionHistory:   nil, // Set by supervisor before CollectObservedState
		frameworkState:  nil, // Set by supervisor before collection
		workerType:      identity.WorkerType,
		workerID:        identity.ID,
	}
}

// GetLogger returns the logger for this dependencies.
func (d *BaseDependencies) GetLogger() *zap.SugaredLogger {
	return d.logger
}

// ActionLogger returns a logger enriched with action context.
// Use this when logging from within an action for consistent structured logs.
func (d *BaseDependencies) ActionLogger(actionType string) *zap.SugaredLogger {
	return d.logger.With("log_source", "action", "action_type", actionType)
}

// GetStateReader returns read-only access to the TriangularStore.
// Returns nil if no StateReader was provided during construction.
func (d *BaseDependencies) GetStateReader() StateReader {
	return d.stateReader
}

// MetricsRecorder returns the MetricsRecorder for actions to record metrics.
// Actions call IncrementCounter/SetGauge with typed constants from the metrics package.
// CollectObservedState implementations should call Drain() to merge buffered metrics
// into the observed state.
//
// This is a WRITE-ONLY buffer. Metrics written here become visible in ObservedState
// only AFTER CollectObservedState() drains the buffer.
//
// Example usage in an action:
//
//	deps.MetricsRecorder().IncrementCounter(deps.CounterPullOps, 1)
//	deps.MetricsRecorder().SetGauge(deps.GaugeLastPullLatencyMs, float64(latency.Milliseconds()))
func (d *BaseDependencies) MetricsRecorder() *MetricsRecorder {
	return d.metricsRecorder
}

// GetActionHistory returns the action history set by the supervisor.
// This is supervisor-managed data (like FrameworkMetrics) - workers can only read, not write.
// The supervisor auto-records action results via ActionExecutor callback and injects
// them into deps before CollectObservedState() is called.
//
// Workers should read this in CollectObservedState() and assign to their ObservedState:
//
//	func (w *MyWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
//	    deps := w.GetDependencies()
//	    return MyObservedState{
//	        LastActionResults: deps.GetActionHistory(),
//	        // ... other fields
//	    }, nil
//	}
//
// Returns nil if no action history has been set (e.g., no actions executed yet).
// Returns a shallow copy to enforce read-only contract - callers cannot modify
// supervisor-owned history.
func (d *BaseDependencies) GetActionHistory() []ActionResult {
	if d.actionHistory == nil {
		return nil
	}

	out := make([]ActionResult, len(d.actionHistory))
	copy(out, d.actionHistory)

	return out
}

// SetActionHistory sets the action history. Called by supervisor before CollectObservedState.
// Workers should NOT call this directly - it's for supervisor use only.
// This follows the same pattern as SetFrameworkState().
func (d *BaseDependencies) SetActionHistory(history []ActionResult) {
	d.actionHistory = history
}

// GetFrameworkState returns the framework metrics provided by the supervisor.
// This data is updated by the supervisor BEFORE CollectObservedState() runs,
// so it may be ~1 tick stale. Do NOT use for sub-second timing decisions.
//
// For timing info about the current state (TimeInCurrentStateMs, etc.), prefer
// reading from ObservedState.FrameworkMetrics in State.Next() where it's freshest.
//
// For action-level timing, use time.Now() and time.Since() directly.
func (d *BaseDependencies) GetFrameworkState() *FrameworkMetrics {
	return d.frameworkState
}

// SetFrameworkState sets the framework metrics. Called by supervisor before collection.
// Workers should NOT call this directly - it's for supervisor use only.
func (d *BaseDependencies) SetFrameworkState(fm *FrameworkMetrics) {
	d.frameworkState = fm
}

// GetWorkerType returns the worker type for this dependencies.
func (d *BaseDependencies) GetWorkerType() string {
	return d.workerType
}

// GetWorkerID returns the worker ID for this dependencies.
func (d *BaseDependencies) GetWorkerID() string {
	return d.workerID
}
