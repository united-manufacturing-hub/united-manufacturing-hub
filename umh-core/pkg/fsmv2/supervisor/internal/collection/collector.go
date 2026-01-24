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
	"fmt"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/metrics"
	"go.uber.org/zap"
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
	Logger                    *zap.SugaredLogger
	StateProvider             func() string                       // Returns current FSM state name (injected by supervisor)
	ShutdownRequestedProvider func() bool                         // Returns current shutdown requested status (injected by supervisor)
	ChildrenCountsProvider    func() (healthy int, unhealthy int) // Returns children health counts (injected by supervisor for parent workers)
	MappedParentStateProvider func() string                       // Returns mapped state from parent's StateMapping (injected by supervisor for child workers)
	ChildrenViewProvider      func() any                          // Returns config.ChildrenView for parent workers to inspect children (injected by supervisor)
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
	Identity            deps.Identity
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
func (c *Collector[TObserved]) logTrace(msg string, keysAndValues ...any) {
	if c.config.EnableTraceLogging {
		c.config.Logger.Debugw(msg, keysAndValues...)
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

	c.config.Logger.Infow("collector_starting",
		"from_state", c.state.String(),
		"to_state", "running")

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

// Restart signals the observation loop to collect immediately.
func (c *Collector[TObserved]) Restart() {
	c.mu.RLock()
	running := c.state == collectorStateRunning
	c.mu.RUnlock()

	if !running {
		c.config.Logger.Errorw("collector_restart_failed",
			"reason", "not_running",
			"current_state", c.state.String())

		return
	}

	c.config.Logger.Infow("collector_restart_requested")

	select {
	case c.restartChan <- struct{}{}:
		c.config.Logger.Debugw("collector_restart_signal_sent")
	default:
		c.config.Logger.Debugw("collector_restart_already_pending")
	}
}

func (c *Collector[TObserved]) Stop(ctx context.Context) {
	c.mu.Lock()

	if c.state != collectorStateRunning {
		c.config.Logger.Warnw("collector_stop_skipped",
			"reason", "not_running",
			"current_state", c.state.String())
		c.mu.Unlock()

		return
	}

	c.config.Logger.Debugw("collector_stopping")
	c.cancel()
	doneChan := c.goroutineDone
	c.mu.Unlock()

	select {
	case <-doneChan:
		c.config.Logger.Debugw("collector_stopped",
			"result", "success")
	case <-ctx.Done():
		c.config.Logger.Warnw("collector_stopped",
			"result", "context_cancelled")
	case <-time.After(5 * time.Second):
		c.config.Logger.Errorw("collector_stopped",
			"result", "timeout")
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
		c.mu.RUnlock()

		return fmt.Errorf("collector not running (state: %s)", c.state.String())
	}

	timeout := c.config.ObservationTimeout
	c.mu.RUnlock()

	collectCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	c.config.Logger.Debugw("collector_final_observation_starting")

	err := c.collectAndSaveObservedState(collectCtx)
	if err != nil {
		c.config.Logger.Warnw("collector_final_observation_failed", "error", err)

		return err
	}

	c.config.Logger.Debugw("collector_final_observation_completed")

	return nil
}

func (c *Collector[TObserved]) observationLoop() {
	defer func() {
		c.mu.Lock()
		c.state = collectorStateStopped
		c.running = false
		close(c.goroutineDone)
		c.mu.Unlock()
		c.config.Logger.Debugw("collector_loop_stopped",
			"final_state", c.state.String())
	}()

	c.mu.RLock()
	ctx := c.ctx
	interval := c.config.ObservationInterval
	timeout := c.config.ObservationTimeout
	c.mu.RUnlock()

	c.config.Logger.Infow("collector_loop_starting",
		"interval", interval.String(),
		"timeout", timeout.String())

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.config.Logger.Debugw("collector_loop_context_done")

			return

		case <-c.restartChan:
			c.config.Logger.Debugw("collector_restart_triggered")

			collectCtx, cancel := context.WithTimeout(ctx, timeout)
			if err := c.collectAndSaveObservedState(collectCtx); err != nil {
				c.config.Logger.Errorw("collector_observation_failed",
					"trigger", "restart",
					"error", err)
			}

			cancel()

		case <-ticker.C:
			collectCtx, cancel := context.WithTimeout(ctx, timeout)
			if err := c.collectAndSaveObservedState(collectCtx); err != nil {
				c.config.Logger.Errorw("collector_observation_failed",
					"trigger", "ticker",
					"error", err)
			}

			cancel()
		}
	}
}

func (c *Collector[TObserved]) collectAndSaveObservedState(ctx context.Context) error {
	collectionStartTime := time.Now()
	c.logTrace("observation_collection_starting",
		"collection_start_time", collectionStartTime.Format(time.RFC3339Nano))

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

	observed, err := c.config.Worker.CollectObservedState(ctx)
	if err != nil {
		c.config.Logger.Debugw("collector_collect_failed",
			"error", err)

		return err
	}

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

	// Inject mapped parent state so child workers know when to start/stop via StateMapping
	if c.config.MappedParentStateProvider != nil {
		mappedState := c.config.MappedParentStateProvider()
		if setter, ok := observed.(interface {
			SetParentMappedState(string) fsmv2.ObservedState
		}); ok {
			observed = setter.SetParentMappedState(mappedState)
		}
	}

	// Inject children view so parent workers can inspect individual child states
	if c.config.ChildrenViewProvider != nil {
		childrenView := c.config.ChildrenViewProvider()
		if setter, ok := observed.(interface{ SetChildrenView(any) fsmv2.ObservedState }); ok {
			observed = setter.SetChildrenView(childrenView)
		}
	}

	// NOTE: ActionHistory injection was removed - collector must not modify ObservedState
	// after CollectObservedState returns. Workers read deps.GetActionHistory() directly.
	var observationTimestamp time.Time
	if timestampProvider, ok := observed.(fsmv2.TimestampProvider); ok {
		observationTimestamp = timestampProvider.GetTimestamp()
		c.logTrace("observation_collected",
			"observation_timestamp", observationTimestamp.Format(time.RFC3339Nano))
	} else {
		c.logTrace("observation_collected_no_timestamp",
			"actual_type", fmt.Sprintf("%T", observed))
	}

	saveStartTime := time.Now()

	observedTyped, ok := observed.(TObserved)
	if !ok {
		c.config.Logger.Errorw("collector_type_mismatch",
			"expected_type", fmt.Sprintf("%T", *new(TObserved)),
			"actual_type", fmt.Sprintf("%T", observed))

		return fmt.Errorf("observed state type mismatch: expected %T, got %T", *new(TObserved), observed)
	}

	ts, ok := c.config.Store.(*storage.TriangularStore)
	if !ok {
		c.config.Logger.Errorw("collector_store_type_mismatch",
			"expected_type", "*storage.TriangularStore",
			"actual_type", fmt.Sprintf("%T", c.config.Store))

		return fmt.Errorf("store is not *TriangularStore, got %T", c.config.Store)
	}

	changed, err := storage.SaveObservedTyped[TObserved](ts, ctx, c.config.Identity.ID, observedTyped)
	if err != nil {
		c.config.Logger.Debugw("collector_save_failed",
			"error", err)

		return err
	}

	saveDuration := time.Since(saveStartTime)

	workerType, err := storage.DeriveWorkerType[TObserved]()
	if err != nil {
		return fmt.Errorf("failed to derive worker type for metrics: %w", err)
	}

	metrics.RecordObservationSave(workerType, changed, saveDuration)
	metrics.ExportWorkerMetrics(workerType, c.config.Identity.ID, observed)

	if changed {
		c.logTrace("observation_saved",
			"save_duration", saveDuration,
			"changed", true)
	} else {
		c.logTrace("observation_unchanged",
			"save_duration", saveDuration,
			"changed", false)
	}

	return nil
}
