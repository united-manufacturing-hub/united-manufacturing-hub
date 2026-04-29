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

package collection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/internal/panicutil"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/metrics"
)

type collectorState int

const (
	collectorStateCreated collectorState = iota
	collectorStateRunning
	collectorStateStopped
)

// String returns a human-readable name for the collector state.
func (s collectorState) String() string {
	switch s {
	case collectorStateCreated:
		return "created"
	case collectorStateRunning:
		return "running"
	case collectorStateStopped:
		return "stopped"
	default:
		return fmt.Sprintf("unknown(%d)", s)
	}
}

// CollectorConfig provides configuration for observation data collection.
// The type parameter TObserved represents the observed state type for this collector.
type CollectorConfig[TObserved any] struct {
	Worker                    fsmv2.Worker
	Store                     storage.TriangularStoreInterface
	Logger                    deps.FSMLogger
	StateProvider             func() string                       // Returns current FSM state name (injected by supervisor)
	ShutdownRequestedProvider func() bool                         // Returns current shutdown requested status (injected by supervisor)
	// ChildrenCountsProvider returns the per-tick (healthy, unhealthy) counts.
	// Retained for workers that satisfy SetChildrenCounts but not SetChildrenView;
	// new code should consume the richer ChildrenView and read view.HealthyCount /
	// view.UnhealthyCount instead. The two providers carry redundant data by
	// construction (NewChildrenView derives counts from the same per-child Phase
	// the supervisor reports here), so satisfying both setters is harmless.
	ChildrenCountsProvider func() (healthy int, unhealthy int)
	// ChildrenViewProvider returns the full ChildrenView snapshot for parent
	// workers that need per-child detail. Counts are also exposed on the view
	// (view.HealthyCount / view.UnhealthyCount) so workers consuming the view
	// can ignore ChildrenCountsProvider.
	ChildrenViewProvider func() config.ChildrenView
	// FrameworkMetricsProvider returns current framework metrics from supervisor.
	// Called BEFORE collection to inject into worker dependencies.
	// The provider captures workerCtx and acquires RLock when called (thread-safe).
	FrameworkMetricsProvider func() *deps.FrameworkMetrics
	// FrameworkMetricsSetter sets framework metrics on worker dependencies.
	// Called BEFORE CollectObservedState so workers can access via deps.GetFrameworkState().
	// Replaces the duck-typing injection pattern for explicit metrics copying.
	FrameworkMetricsSetter func(*deps.FrameworkMetrics)
	// ActionHistoryProvider returns buffered action results from supervisor's workerCtx.
	// Called BEFORE CollectObservedState to get action results for injection into deps.
	// The supervisor auto-records action results via ActionExecutor callback.
	ActionHistoryProvider func() []deps.ActionResult
	// ActionHistorySetter sets action history on worker dependencies BEFORE CollectObservedState.
	// Workers then access via deps.GetActionHistory() and assign to their ObservedState.
	// This follows the same pattern as FrameworkMetricsSetter.
	ActionHistorySetter func([]deps.ActionResult)
	// DesiredStateProvider returns the current desired state from the CSE store.
	// Called BEFORE CollectObservedState so workers can access configuration
	// (target IP, port, etc.) without workarounds. If nil is returned (desired
	// state not yet saved), collection is skipped entirely — workers are
	// guaranteed to always receive a non-nil desired state.
	DesiredStateProvider func() fsmv2.DesiredState
	Identity             deps.Identity
	ObservationInterval time.Duration
	ObservationTimeout  time.Duration
	EnableTraceLogging  bool // Whether to emit verbose per-collection logs
}

// Collector manages the observation loop lifecycle and data collection.
// The type parameter TObserved represents the observed state type for this collector.
type Collector[TObserved any] struct {
	ctx           context.Context
	parentCtx     context.Context
	cancel        context.CancelFunc
	goroutineDone chan struct{}
	restartChan   chan struct{}
	config        CollectorConfig[TObserved]
	state         collectorState
	mu            sync.RWMutex
	collectionMu  sync.Mutex // Serializes collectAndSaveObservedState between observation loop and CollectFinalObservation
	running       bool
}

// NewCollector creates a new collector with the given configuration.
// The type parameter TObserved is inferred from the config parameter.
func NewCollector[TObserved any](config CollectorConfig[TObserved]) *Collector[TObserved] {
	return &Collector[TObserved]{
		config:      config,
		state:       collectorStateCreated,
		restartChan: make(chan struct{}, 1),
	}
}

// logTrace logs a structured message only when trace logging is enabled.
// Used for per-collection verbose logs to reduce noise at scale.
func (c *Collector[TObserved]) logTrace(msg string, fields ...deps.Field) {
	if c.config.EnableTraceLogging {
		c.config.Logger.Debug(msg, fields...)
	}
}

// Start launches the observation loop in a goroutine.
// The loop runs until the context is cancelled.
func (c *Collector[TObserved]) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state == collectorStateRunning {
		panic("Invariant I8 violated: collector already started. Collector.Start() must not be called twice. Check lifecycle management in supervisor code.")
	}

	c.config.Logger.Info("collector_starting",
		deps.String("from_state", c.state.String()),
		deps.String("to_state", "running"))

	c.state = collectorStateRunning
	c.parentCtx = ctx
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.goroutineDone = make(chan struct{})
	c.running = true

	go c.observationLoop()

	return nil
}

// IsRunning returns true if the observation loop is currently active.
func (c *Collector[TObserved]) IsRunning() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.running
}

// TriggerNow signals the observation loop to collect immediately.
// This does NOT restart the collector goroutine - it just triggers an early collection.
// Use Restart() for emergency recovery when the collector goroutine is hung.
func (c *Collector[TObserved]) TriggerNow() {
	c.mu.RLock()
	running := c.state == collectorStateRunning
	c.mu.RUnlock()

	if !running {
		c.config.Logger.SentryWarn(deps.FeatureForWorker(c.config.Identity.WorkerType), c.config.Identity.HierarchyPath, "collector_trigger_now_failed",
			deps.Reason("not_running"),
			deps.String("current_state", c.state.String()))

		return
	}

	c.config.Logger.Info("collector_trigger_now_requested")

	select {
	case c.restartChan <- struct{}{}:
		c.config.Logger.Debug("collector_trigger_now_signal_sent")
	default:
		c.config.Logger.Debug("collector_trigger_now_already_pending")
	}
}

// Restart stops and restarts the collector goroutine.
// This is an emergency recovery mechanism for hung collectors.
// Use TriggerNow() for normal on-demand collection triggering.
func (c *Collector[TObserved]) Restart() {
	c.mu.RLock()
	running := c.state == collectorStateRunning
	parentCtx := c.parentCtx
	c.mu.RUnlock()

	if !running {
		c.config.Logger.SentryWarn(deps.FeatureForWorker(c.config.Identity.WorkerType), c.config.Identity.HierarchyPath, "collector_restart_failed",
			deps.Reason("not_running"),
			deps.String("current_state", c.state.String()))

		return
	}

	c.config.Logger.Info("collector_restart_starting")

	// Stop the current collector goroutine
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c.Stop(stopCtx)

	// Reset state so we can start again (Stop sets state to stopped)
	c.mu.Lock()
	c.state = collectorStateCreated
	c.restartChan = make(chan struct{}, 1)
	c.mu.Unlock()

	// Start again with the original parent context.
	// NOTE: Start() currently always returns nil (panics on double-start instead).
	// Error handling preserved for future extensibility (e.g., context validation, resource allocation).
	if parentCtx != nil {
		if err := c.Start(parentCtx); err != nil {
			c.config.Logger.SentryError(deps.FeatureForWorker(c.config.Identity.WorkerType), c.config.Identity.HierarchyPath, err, "collector_restart_start_failed")
		} else {
			c.config.Logger.Info("collector_restart_complete")
		}
	} else {
		c.config.Logger.SentryError(deps.FeatureForWorker(c.config.Identity.WorkerType), c.config.Identity.HierarchyPath, errors.New("no_parent_context"), "collector_restart_failed")
	}
}

func (c *Collector[TObserved]) Stop(ctx context.Context) {
	c.mu.Lock()

	if c.state != collectorStateRunning {
		c.config.Logger.SentryWarn(deps.FeatureForWorker(c.config.Identity.WorkerType), c.config.Identity.HierarchyPath, "collector_stop_skipped",
			deps.Reason("not_running"),
			deps.String("current_state", c.state.String()))
		c.mu.Unlock()

		return
	}

	c.config.Logger.Debug("collector_stopping")
	c.cancel()
	doneChan := c.goroutineDone
	c.mu.Unlock()

	stopTimer := time.NewTimer(5 * time.Second)
	defer stopTimer.Stop()

	select {
	case <-doneChan:
		c.config.Logger.Debug("collector_stopped",
			deps.String("result", "success"))
	case <-ctx.Done():
		c.config.Logger.SentryWarn(deps.FeatureForWorker(c.config.Identity.WorkerType), c.config.Identity.HierarchyPath, "collector_stopped",
			deps.String("result", "context_cancelled"))
	case <-stopTimer.C:
		c.config.Logger.SentryWarn(deps.FeatureForWorker(c.config.Identity.WorkerType), c.config.Identity.HierarchyPath, "collector_stopped",
			deps.String("result", "timeout"))
	}
}

// CollectFinalObservation triggers one final observation synchronously before shutdown.
// The terminal FSM state (e.g., "Stopped") is captured in the database
// before the collector is stopped. Without this, the last observation may show
// an intermediate state like "TryingToStop" instead of the final state.
//
// This method is safe to call even if the collector is not running (returns error).
// The caller should handle errors gracefully - a failed final observation is not fatal.
func (c *Collector[TObserved]) CollectFinalObservation(ctx context.Context) error {
	c.mu.RLock()

	if c.state != collectorStateRunning {
		currentState := c.state.String()
		c.mu.RUnlock()

		c.config.Logger.SentryWarn(deps.FeatureForWorker(c.config.Identity.WorkerType), c.config.Identity.HierarchyPath, "collector_final_observation_skipped",
			deps.Reason("not_running"),
			deps.String("current_state", currentState))

		return errors.New("collector not running")
	}

	timeout := c.config.ObservationTimeout
	c.mu.RUnlock()

	collectCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	c.config.Logger.Debug("collector_final_observation_starting")

	c.collectionMu.Lock()
	err := c.collectAndSaveObservedState(collectCtx)
	c.collectionMu.Unlock()

	if err != nil {
		c.config.Logger.SentryWarn(deps.FeatureForWorker(c.config.Identity.WorkerType), c.config.Identity.HierarchyPath, "collector_final_observation_failed",
			deps.Err(err))

		return err
	}

	c.config.Logger.Debug("collector_final_observation_completed")

	return nil
}

func (c *Collector[TObserved]) observationLoop() {
	defer func() {
		c.mu.Lock()
		c.state = collectorStateStopped
		c.running = false
		close(c.goroutineDone)
		// Read state string while holding lock to avoid race with Restart()
		finalState := c.state.String()
		c.mu.Unlock()
		c.config.Logger.Debug("collector_loop_stopped",
			deps.String("final_state", finalState))
	}()

	c.mu.RLock()
	ctx := c.ctx
	interval := c.config.ObservationInterval
	timeout := c.config.ObservationTimeout
	c.mu.RUnlock()

	c.config.Logger.Info("collector_loop_starting",
		deps.String("interval", interval.String()),
		deps.String("timeout", timeout.String()))

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.config.Logger.Debug("collector_loop_context_done")

			return

		case <-c.restartChan:
			c.config.Logger.Debug("collector_restart_triggered")

			collectCtx, cancel := context.WithTimeout(ctx, timeout)

			c.collectionMu.Lock()
			err := c.collectAndSaveObservedState(collectCtx)
			c.collectionMu.Unlock()

			if err != nil {
				c.config.Logger.SentryError(deps.FeatureForWorker(c.config.Identity.WorkerType), c.config.Identity.HierarchyPath, err, "collector_observation_failed",
					deps.String("trigger", "restart"))
			}

			cancel()

		case <-ticker.C:
			collectCtx, cancel := context.WithTimeout(ctx, timeout)

			c.collectionMu.Lock()
			err := c.collectAndSaveObservedState(collectCtx)
			c.collectionMu.Unlock()

			if err != nil {
				c.config.Logger.SentryError(deps.FeatureForWorker(c.config.Identity.WorkerType), c.config.Identity.HierarchyPath, err, "collector_observation_failed",
					deps.String("trigger", "ticker"))
			}

			cancel()
		}
	}
}

func (c *Collector[TObserved]) collectAndSaveObservedState(ctx context.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = c.handleCollectorPanic(r)
		}
	}()

	collectionStartTime := time.Now()
	c.logTrace("observation_collection_starting",
		deps.String("collection_start_time", collectionStartTime.Format(time.RFC3339Nano)))

	// Inject framework metrics before CollectObservedState so workers can access via deps.GetFrameworkState()
	if c.config.FrameworkMetricsProvider != nil && c.config.FrameworkMetricsSetter != nil {
		fm := c.config.FrameworkMetricsProvider()
		c.config.FrameworkMetricsSetter(fm)
	}

	// Inject action history before CollectObservedState so workers can access via deps.GetActionHistory()
	if c.config.ActionHistoryProvider != nil && c.config.ActionHistorySetter != nil {
		actionHistory := c.config.ActionHistoryProvider()
		c.config.ActionHistorySetter(actionHistory)
	}

	// Load desired state to pass to CollectObservedState.
	// Skip collection entirely if no desired state exists yet — the supervisor
	// guarantees workers always receive a non-nil desired state.
	var desired fsmv2.DesiredState
	if c.config.DesiredStateProvider != nil {
		desired = c.config.DesiredStateProvider()
	}

	if desired == nil {
		c.logTrace("collector_skipped_no_desired_state")

		return nil
	}

	// Framework-level ctx check before calling into worker code.
	// Guarantees responsive shutdown even if the worker omits its own check.
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	observed, err := c.config.Worker.CollectObservedState(ctx, desired)
	if err != nil {
		c.config.Logger.Debug("collector_collect_failed",
			deps.Err(err))

		return err
	}

	// Post-COS framework wrapping: fill CollectedAt, framework metrics,
	// action history, and accumulated worker metrics.
	observed = c.wrapNewObservation(ctx, observed)

	// Inject FSM state via callback to preserve "Collector-only writes ObservedState" boundary
	if c.config.StateProvider != nil {
		stateName := c.config.StateProvider()
		if setter, ok := observed.(interface {
			SetState(string) fsmv2.ObservedState
		}); ok {
			observed = setter.SetState(stateName)
		}
	}

	// Inject shutdown requested status from desired state into observed snapshot
	if c.config.ShutdownRequestedProvider != nil {
		shutdownRequested := c.config.ShutdownRequestedProvider()
		if setter, ok := observed.(interface {
			SetShutdownRequested(bool) fsmv2.ObservedState
		}); ok {
			observed = setter.SetShutdownRequested(shutdownRequested)
		}
	}

	// Inject children health counts for parent workers to track state transitions
	if c.config.ChildrenCountsProvider != nil {
		healthy, unhealthy := c.config.ChildrenCountsProvider()
		if setter, ok := observed.(interface {
			SetChildrenCounts(int, int) fsmv2.ObservedState
		}); ok {
			observed = setter.SetChildrenCounts(healthy, unhealthy)
		}
	}

	// Inject children view so parent workers can inspect individual child states
	if c.config.ChildrenViewProvider != nil {
		childrenView := c.config.ChildrenViewProvider()
		if setter, ok := observed.(interface {
			SetChildrenView(config.ChildrenView) fsmv2.ObservedState
		}); ok {
			observed = setter.SetChildrenView(childrenView)
		}
	}

	var observationTimestamp time.Time
	if timestampProvider, ok := observed.(fsmv2.TimestampProvider); ok {
		observationTimestamp = timestampProvider.GetTimestamp()
		c.logTrace("observation_collected",
			deps.String("observation_timestamp", observationTimestamp.Format(time.RFC3339Nano)))
	} else {
		c.logTrace("observation_collected_no_timestamp",
			deps.String("actual_type", fmt.Sprintf("%T", observed)))
	}

	saveStartTime := time.Now()

	observedTyped, ok := observed.(TObserved)
	if !ok {
		err := fmt.Errorf("observed state type mismatch: expected %T, got %T", *new(TObserved), observed)
		c.config.Logger.SentryError(deps.FeatureForWorker(c.config.Identity.WorkerType), c.config.Identity.HierarchyPath, err, "collector_type_mismatch",
			deps.String("expected_type", fmt.Sprintf("%T", *new(TObserved))),
			deps.String("actual_type", fmt.Sprintf("%T", observed)))

		return err
	}

	// Use non-generic save path: marshal to JSON → unmarshal to Document → SaveObserved.
	// SaveObservedTyped[TObserved] would call DeriveWorkerType which fails for generic
	// type names like Observation[SomeStatus] (name ends in "]" not "ObservedState").
	observedJSON, err := json.Marshal(observedTyped)
	if err != nil {
		return fmt.Errorf("collector marshal observed: %w", err)
	}

	observedDoc := make(persistence.Document)
	if err := json.Unmarshal(observedJSON, &observedDoc); err != nil {
		return fmt.Errorf("collector unmarshal observed to document: %w", err)
	}

	observedDoc["id"] = c.config.Identity.ID

	changed, err := c.config.Store.SaveObserved(ctx, c.config.Identity.WorkerType, c.config.Identity.ID, observedDoc)
	if err != nil {
		c.config.Logger.Debug("collector_save_failed",
			deps.Err(err))

		return err
	}

	saveDuration := time.Since(saveStartTime)

	metrics.RecordObservationSave(c.config.Identity.HierarchyPath, changed, saveDuration)
	metrics.ExportWorkerMetrics(c.config.Identity.HierarchyPath, observed)

	if changed {
		c.logTrace("observation_saved",
			deps.Duration("save_duration", saveDuration),
			deps.Bool("changed", true))
	} else {
		c.logTrace("observation_unchanged",
			deps.Duration("save_duration", saveDuration),
			deps.Bool("changed", false))
	}

	return nil
}

// baseDepsAccessor is a duck-type interface for extracting framework data from
// any worker's dependencies (BaseDependencies or a struct embedding it).
type baseDepsAccessor interface {
	GetFrameworkState() *deps.FrameworkMetrics
	GetActionHistory() []deps.ActionResult
	MetricsRecorder() *deps.MetricsRecorder
}

// wrapNewObservation fills framework fields on the ObservedState returned by
// CollectObservedState. Steps: set CollectedAt, inject framework metrics +
// action history, accumulate worker metrics (load previous from CSE, drain
// recorder, merge).
func (c *Collector[TObserved]) wrapNewObservation(ctx context.Context, observed fsmv2.ObservedState) fsmv2.ObservedState {
	// Step 1: Set CollectedAt to now.
	if setter, ok := observed.(interface {
		SetCollectedAt(time.Time) fsmv2.ObservedState
	}); ok {
		observed = setter.SetCollectedAt(time.Now())
	}

	// Step 2: Get BaseDependencies from worker via DependencyProvider → baseDepsAccessor.
	depProvider, ok := c.config.Worker.(fsmv2.DependencyProvider)
	if !ok {
		c.config.Logger.Debug("wrap_new_observation_no_dependency_provider",
			deps.String("worker_type", fmt.Sprintf("%T", c.config.Worker)))

		return observed
	}

	depsAny := depProvider.GetDependenciesAny()

	bd, ok := depsAny.(baseDepsAccessor)
	if !ok {
		c.config.Logger.Debug("wrap_new_observation_no_base_deps_accessor",
			deps.String("deps_type", fmt.Sprintf("%T", depsAny)))

		return observed
	}

	// Step 3: Inject framework metrics.
	if fm := bd.GetFrameworkState(); fm != nil {
		if setter, ok := observed.(interface {
			SetFrameworkMetrics(deps.FrameworkMetrics) fsmv2.ObservedState
		}); ok {
			observed = setter.SetFrameworkMetrics(*fm)
		}
	}

	// Step 4: Inject action history.
	if ah := bd.GetActionHistory(); ah != nil {
		if setter, ok := observed.(interface {
			SetActionHistory([]deps.ActionResult) fsmv2.ObservedState
		}); ok {
			observed = setter.SetActionHistory(ah)
		}
	}

	// Step 5: Accumulate worker metrics (load previous → drain → merge).
	var prevWorkerMetrics deps.Metrics

	// Step 5a: Load previous state from CSE BEFORE drain (drain is destructive).
	if ctx.Err() == nil {
		var prev struct {
			Metrics deps.MetricsContainer `json:"metrics"`
		}

		if err := c.config.Store.LoadObservedTyped(ctx, c.config.Identity.WorkerType, c.config.Identity.ID, &prev); err == nil {
			prevWorkerMetrics = prev.Metrics.Worker
		} else {
			c.config.Logger.Debug("wrap_new_observation_cse_load_failed",
				deps.Err(err))
		}
	}

	// Step 5b: Init empty maps if CSE load failed or returned nil maps.
	if prevWorkerMetrics.Counters == nil {
		prevWorkerMetrics.Counters = make(map[string]int64)
	}

	if prevWorkerMetrics.Gauges == nil {
		prevWorkerMetrics.Gauges = make(map[string]float64)
	}

	// Step 5c: Drain MetricsRecorder AFTER CSE read.
	if recorder := bd.MetricsRecorder(); recorder != nil {
		drained := recorder.Drain()

		// Step 5d: Merge — counters additive, gauges replace.
		for name, delta := range drained.Counters {
			prevWorkerMetrics.Counters[name] += delta
		}

		for name, value := range drained.Gauges {
			prevWorkerMetrics.Gauges[name] = value
		}
	}

	// Step 5e: Set accumulated worker metrics.
	if setter, ok := observed.(interface {
		SetWorkerMetrics(deps.Metrics) fsmv2.ObservedState
	}); ok {
		observed = setter.SetWorkerMetrics(prevWorkerMetrics)
	}

	return observed
}

// handleCollectorPanic processes a recovered panic from collectAndSaveObservedState.
// It classifies the panic, records metrics, and logs to Sentry. If the recovery handler
// itself panics, that secondary panic is caught and logged separately.
func (c *Collector[TObserved]) handleCollectorPanic(r interface{}) (err error) {
	logger := c.config.Logger
	hierarchyPath := c.config.Identity.HierarchyPath

	defer func() {
		if r2 := recover(); r2 != nil {
			err = fmt.Errorf("collector panic (recovery handler also panicked: %v): %v", r2, r)

			func() {
				defer func() { recover() }() //nolint:errcheck // recover() return value is intentionally unused in safety net

				logger.SentryError(deps.FeatureForWorker(c.config.Identity.WorkerType), hierarchyPath, err, "collector_double_panic",
					deps.String("stack", string(debug.Stack())))
			}()
		}
	}()

	panicType, panicErr := panicutil.ClassifyPanic(r)
	err = fmt.Errorf("collector panic: %w", panicErr)

	metrics.RecordPanicRecovery(hierarchyPath, panicType)

	logger.SentryError(deps.FeatureForWorker(c.config.Identity.WorkerType), hierarchyPath, err, "collector_panic",
		deps.Field{Key: "panic_value", Value: fmt.Sprintf("%v", r)},
		deps.Field{Key: "panic_type", Value: panicType},
		deps.Field{Key: "stack_trace", Value: string(debug.Stack())})

	return err
}
