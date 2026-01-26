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
	"sync"

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
// Workers MUST NOT write to the store directly; all writes go through the supervisor's collector.
type StateReader interface {
	// LoadObservedTyped loads observed state into result pointer.
	// Returns error if state doesn't exist or unmarshaling fails.
	LoadObservedTyped(ctx context.Context, workerType string, id string, result interface{}) error
}

// Dependencies provides access to worker-specific tools for actions.
// Worker-specific dependencies embed BaseDependencies and extend with additional tools.
type Dependencies interface {
	GetLogger() *zap.SugaredLogger
	// ActionLogger returns a logger enriched with action context for structured logs.
	ActionLogger(actionType string) *zap.SugaredLogger
	// GetStateReader returns read-only access to the TriangularStore, or nil if not provided.
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
	mu              sync.RWMutex   // Protects frameworkState and actionHistory
}

// NewBaseDependencies creates base dependencies with common tools.
// Logger is enriched with worker context; stateReader can be nil.
func NewBaseDependencies(logger *zap.SugaredLogger, stateReader StateReader, identity Identity) *BaseDependencies {
	if logger == nil {
		panic("NewBaseDependencies: logger cannot be nil")
	}

	return &BaseDependencies{
		logger:          logger.With("worker", identity.String()),
		stateReader:     stateReader,
		metricsRecorder: NewMetricsRecorder(),
		workerType:      identity.WorkerType,
		workerID:        identity.ID,
	}
}

func (d *BaseDependencies) GetLogger() *zap.SugaredLogger {
	return d.logger
}

func (d *BaseDependencies) ActionLogger(actionType string) *zap.SugaredLogger {
	return d.logger.With("log_source", "action", "action_type", actionType)
}

func (d *BaseDependencies) GetStateReader() StateReader {
	return d.stateReader
}

// MetricsRecorder returns the write-only metrics buffer.
// Metrics become visible in ObservedState only after CollectObservedState() drains the buffer.
func (d *BaseDependencies) MetricsRecorder() *MetricsRecorder {
	return d.metricsRecorder
}

// GetActionHistory returns supervisor-managed action history (read-only).
// Returns a shallow copy; nil if no actions have been executed.
func (d *BaseDependencies) GetActionHistory() []ActionResult {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.actionHistory == nil {
		return nil
	}

	out := make([]ActionResult, len(d.actionHistory))
	copy(out, d.actionHistory)

	return out
}

// SetActionHistory is for supervisor use only; called before CollectObservedState.
func (d *BaseDependencies) SetActionHistory(history []ActionResult) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.actionHistory = history
}

// GetFrameworkState returns framework metrics (~1 tick stale).
// For sub-second timing, use time.Now() directly in actions.
func (d *BaseDependencies) GetFrameworkState() *FrameworkMetrics {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.frameworkState
}

// SetFrameworkState is for supervisor use only; called before collection.
func (d *BaseDependencies) SetFrameworkState(fm *FrameworkMetrics) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.frameworkState = fm
}

func (d *BaseDependencies) GetWorkerType() string {
	return d.workerType
}

func (d *BaseDependencies) GetWorkerID() string {
	return d.workerID
}
