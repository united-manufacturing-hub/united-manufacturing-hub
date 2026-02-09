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

package execution

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/metrics"
)

const (
	// stuckActionDetectionMultiplier defines when an in-progress action is considered stuck (elapsed > multiplier * timeout).
	stuckActionDetectionMultiplier = 2
	// stuckActionForceRemoveMultiplier defines when a stuck action is force-removed, allowing re-enqueue.
	stuckActionForceRemoveMultiplier = 3
	// defaultWorkerCount is the number of concurrent action workers when none is specified.
	defaultWorkerCount = 10
	// queueSizeMultiplier determines queue capacity as workerCount * multiplier.
	queueSizeMultiplier = 2
	// defaultActionTimeout is the per-action execution timeout when no override is configured.
	defaultActionTimeout = 30 * time.Second
	// defaultMetricsInterval is how often the metrics reporter checks for stuck actions.
	defaultMetricsInterval = 5 * time.Second
)

type inProgressEntry struct {
	startTime     time.Time
	actionName    string
	timeout       time.Duration
	generation    uint64
	stuckReported bool
}

type ActionExecutor struct {
	ctx              context.Context
	actionQueue      chan actionWork
	inProgress       map[string]inProgressEntry
	cancel           context.CancelFunc
	timeouts         map[string]time.Duration
	metricsCancel    context.CancelFunc
	logger           deps.FSMLogger
	onActionComplete func(result deps.ActionResult)
	identity         deps.Identity
	supervisorID     string
	wg               sync.WaitGroup
	metricsWg        sync.WaitGroup
	workerCount      int
	defaultTimeout   time.Duration
	metricsInterval  time.Duration
	nextGeneration   uint64
	mu               sync.RWMutex
	closeOnce        sync.Once
	stopped          bool
}

type actionWork struct {
	action     fsmv2.Action[any]
	deps       any
	actionID   string
	timeout    time.Duration
	generation uint64
}

type stuckEntry struct {
	actionID    string
	actionName  string
	elapsedMs   int64
	timeoutMs   int64
	forceRemove bool
}

func NewActionExecutor(workerCount int, supervisorID string, identity deps.Identity, logger deps.FSMLogger) *ActionExecutor {
	return NewActionExecutorWithTimeout(workerCount, nil, supervisorID, identity, logger)
}

func NewActionExecutorWithTimeout(workerCount int, timeouts map[string]time.Duration, supervisorID string, identity deps.Identity, logger deps.FSMLogger) *ActionExecutor {
	if workerCount <= 0 {
		workerCount = defaultWorkerCount
	}

	if timeouts == nil {
		timeouts = make(map[string]time.Duration)
	}

	return &ActionExecutor{
		supervisorID:    supervisorID,
		identity:        identity,
		workerCount:     workerCount,
		actionQueue:     make(chan actionWork, workerCount*queueSizeMultiplier),
		inProgress:      make(map[string]inProgressEntry),
		timeouts:        timeouts,
		defaultTimeout:  defaultActionTimeout,
		metricsInterval: defaultMetricsInterval,
		logger:          logger,
	}
}

func (ae *ActionExecutor) Start(ctx context.Context) {
	ae.mu.Lock()
	ae.ctx, ae.cancel = context.WithCancel(ctx)
	// Pre-add all workers to WaitGroup under lock to prevent race with Shutdown()
	for range ae.workerCount {
		ae.wg.Add(1)
	}

	ae.mu.Unlock()

	// Start workers - they're already counted in the WaitGroup
	for range ae.workerCount {
		go ae.worker()
	}

	metricsCtx, metricsCancel := context.WithCancel(ctx)

	ae.mu.Lock()
	ae.metricsCancel = metricsCancel
	ae.metricsWg.Add(1) // Add inside lock to prevent race with Shutdown()
	ae.mu.Unlock()

	go ae.metricsReporter(metricsCtx)
}

func (ae *ActionExecutor) worker() {
	defer ae.wg.Done()

	// Capture context under lock to prevent race with Start()/Shutdown()
	ae.mu.RLock()
	ctx := ae.ctx
	ae.mu.RUnlock()

	for {
		select {
		case <-ctx.Done():
			return

		case work, ok := <-ae.actionQueue:
			if !ok {
				return
			}

			ae.executeWorkWithRecovery(ctx, work)
		}
	}
}

// executeWorkWithRecovery handles action execution with panic recovery.
// ctx is passed from worker() which captures it under lock to prevent races.
func (ae *ActionExecutor) executeWorkWithRecovery(ctx context.Context, work actionWork) {
	startTime := time.Now()

	actionCtx, cancel := context.WithTimeout(ctx, work.timeout)
	defer cancel()

	var err error

	var status string

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("action panicked: %v", r)
			status = "panic"

			ae.logger.SentryError(deps.FeatureFSMv2, ae.identity.HierarchyPath, err, "action_panic",
				deps.CorrelationID(work.actionID),
				deps.ActionName(work.action.Name()),
				deps.Int64("timeout_ms", work.timeout.Milliseconds()),
				deps.String("deps_type", fmt.Sprintf("%T", work.deps)),
				deps.String("panic", fmt.Sprintf("%v", r)),
				deps.String("stack", string(debug.Stack())))
		}

		ae.mu.Lock()
		entry, exists := ae.inProgress[work.actionID]
		generationMatch := exists && entry.generation == work.generation
		if generationMatch {
			delete(ae.inProgress, work.actionID)
		}
		callback := ae.onActionComplete
		ae.mu.Unlock()

		if !exists {
			ae.logger.Info("action_completion_discarded_entry_missing",
				deps.CorrelationID(work.actionID),
				deps.ActionName(work.action.Name()),
				deps.Field{Key: "work_generation", Value: work.generation})
			return
		}
		if !generationMatch {
			ae.logger.Info("action_completion_discarded_stale_generation",
				deps.CorrelationID(work.actionID),
				deps.ActionName(work.action.Name()),
				deps.Field{Key: "work_generation", Value: work.generation},
				deps.Field{Key: "current_generation", Value: entry.generation})
			return
		}

		duration := time.Since(startTime)

		if status == "" {
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					status = "timeout"
				} else {
					status = "error"
				}
			} else {
				status = "success"
			}
		}

		metrics.RecordActionExecutionDuration(ae.identity.HierarchyPath, work.action.Name(), status, duration)

		if callback != nil {
			errorMsg := ""
			if err != nil {
				errorMsg = err.Error()
			}

			callback(deps.ActionResult{
				Timestamp:  time.Now(),
				ActionType: work.action.Name(),
				Success:    status == "success",
				ErrorMsg:   errorMsg,
				Latency:    duration,
			})
		}
	}()

	err = work.action.Execute(actionCtx, work.deps)

	duration := time.Since(startTime)

	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			metrics.RecordActionTimeout(ae.identity.HierarchyPath, work.action.Name())

			ae.logger.SentryError(deps.FeatureFSMv2, ae.identity.HierarchyPath, err, "action_failed",
				deps.CorrelationID(work.actionID),
				deps.ActionName(work.action.Name()),
				deps.DurationMs(duration.Milliseconds()),
				deps.Int64("timeout_ms", work.timeout.Milliseconds()))
		} else {
			ae.logger.SentryError(deps.FeatureFSMv2, ae.identity.HierarchyPath, err, "action_failed",
				deps.CorrelationID(work.actionID),
				deps.ActionName(work.action.Name()),
				deps.DurationMs(duration.Milliseconds()))
		}
	} else {
		ae.logger.Debug("action_completed",
			deps.HierarchyPath(ae.identity.HierarchyPath),
			deps.CorrelationID(work.actionID),
			deps.ActionName(work.action.Name()),
			deps.DurationMs(duration.Milliseconds()))
	}
}

// EnqueueAction adds an action to the execution queue without blocking.
// Returns error if action is already in progress, queue is full, or executor is stopped.
func (ae *ActionExecutor) EnqueueAction(actionID string, action fsmv2.Action[any], workerDeps any) error {
	ae.mu.Lock()

	// Check if executor is stopped to prevent sending on closed channel
	if ae.stopped {
		inProgressCount := len(ae.inProgress)
		ae.mu.Unlock()

		ae.logger.SentryWarn(deps.FeatureFSMv2, ae.identity.HierarchyPath, "action_enqueue_rejected",
			deps.CorrelationID(actionID),
			deps.ActionName(action.Name()),
			deps.Reason("executor_stopped"),
			deps.Capacity(cap(ae.actionQueue)),
			deps.Int("in_progress_count", inProgressCount))

		return errors.New("executor stopped")
	}

	if _, exists := ae.inProgress[actionID]; exists {
		ae.mu.Unlock()

		ae.logger.SentryWarn(deps.FeatureFSMv2, ae.identity.HierarchyPath, "action_enqueue_rejected",
			deps.CorrelationID(actionID),
			deps.ActionName(action.Name()),
			deps.Reason("already_in_progress"))

		return errors.New("action already in progress")
	}

	// Read timeout while still holding lock to prevent race with concurrent access
	timeout, timeoutExists := ae.timeouts[actionID]
	if !timeoutExists {
		timeout = ae.defaultTimeout
	}

	ae.nextGeneration++
	gen := ae.nextGeneration

	ae.inProgress[actionID] = inProgressEntry{
		startTime:  time.Now(),
		actionName: action.Name(),
		timeout:    timeout,
		generation: gen,
	}
	ae.mu.Unlock()

	work := actionWork{
		actionID:   actionID,
		action:     action,
		timeout:    timeout,
		deps:       workerDeps,
		generation: gen,
	}

	select {
	case ae.actionQueue <- work:
		metrics.RecordActionQueued(ae.identity.HierarchyPath, action.Name())

		return nil
	default:
		ae.mu.Lock()
		delete(ae.inProgress, actionID)
		inProgressCount := len(ae.inProgress)
		ae.mu.Unlock()

		queueErr := errors.New("action queue full")
		ae.logger.SentryError(deps.FeatureFSMv2, ae.identity.HierarchyPath, queueErr, "action_queue_full",
			deps.CorrelationID(actionID),
			deps.ActionName(action.Name()),
			deps.Capacity(cap(ae.actionQueue)),
			deps.Length(len(ae.actionQueue)),
			deps.Int("in_progress_count", inProgressCount),
			deps.Int("worker_count", ae.workerCount))

		return queueErr
	}
}

func (ae *ActionExecutor) metricsReporter(ctx context.Context) {
	defer ae.metricsWg.Done()

	ticker := time.NewTicker(ae.metricsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ae.mu.Lock()
			queueSize := len(ae.actionQueue)
			inProgressCount := len(ae.inProgress)
			callback := ae.onActionComplete

			now := time.Now()

			var stuckActions []stuckEntry

			for actionID, entry := range ae.inProgress {
				elapsed := now.Sub(entry.startTime)

				if elapsed > stuckActionForceRemoveMultiplier*entry.timeout {
					delete(ae.inProgress, actionID)
					stuckActions = append(stuckActions, stuckEntry{
						actionID:    actionID,
						actionName:  entry.actionName,
						elapsedMs:   elapsed.Milliseconds(),
						timeoutMs:   entry.timeout.Milliseconds(),
						forceRemove: true,
					})
				} else if elapsed > stuckActionDetectionMultiplier*entry.timeout && !entry.stuckReported {
					entry.stuckReported = true
					ae.inProgress[actionID] = entry
					stuckActions = append(stuckActions, stuckEntry{
						actionID:   actionID,
						actionName: entry.actionName,
						elapsedMs:  elapsed.Milliseconds(),
						timeoutMs:  entry.timeout.Milliseconds(),
					})
				}
			}

			ae.mu.Unlock()

			for _, stuck := range stuckActions {
				if stuck.forceRemove {
					metrics.RecordStuckActionForceRemoved(ae.identity.HierarchyPath, stuck.actionName)
					ae.logger.SentryError(deps.FeatureFSMv2, ae.identity.HierarchyPath, fmt.Errorf("action %s stuck for %dms (timeout %dms), force-removed", stuck.actionName, stuck.elapsedMs, stuck.timeoutMs), "stuck_action_force_removed",
						deps.Field{Key: "action_id", Value: stuck.actionID},
						deps.Field{Key: "action_name", Value: stuck.actionName},
						deps.Field{Key: "elapsed_ms", Value: stuck.elapsedMs},
						deps.Field{Key: "timeout_ms", Value: stuck.timeoutMs})

					if callback != nil {
						callback(deps.ActionResult{
							Timestamp:  now,
							ActionType: stuck.actionName,
							ErrorMsg:   fmt.Sprintf("force-removed: stuck for %dms (timeout %dms)", stuck.elapsedMs, stuck.timeoutMs),
							Latency:    time.Duration(stuck.elapsedMs) * time.Millisecond,
						})
					}
				} else {
					metrics.RecordStuckActionDetected(ae.identity.HierarchyPath, stuck.actionName)
					ae.logger.SentryWarn(deps.FeatureFSMv2, ae.identity.HierarchyPath, "stuck_action_detected",
						deps.Field{Key: "action_id", Value: stuck.actionID},
						deps.Field{Key: "action_name", Value: stuck.actionName},
						deps.Field{Key: "elapsed_ms", Value: stuck.elapsedMs},
						deps.Field{Key: "timeout_ms", Value: stuck.timeoutMs})
				}
			}

			metrics.RecordWorkerPoolQueueSize(ae.identity.HierarchyPath, queueSize)

			utilization := float64(inProgressCount) / float64(ae.workerCount)
			metrics.RecordWorkerPoolUtilization(ae.identity.HierarchyPath, utilization)
		}
	}
}

// HasActionInProgress checks if an action is currently executing.
func (ae *ActionExecutor) HasActionInProgress(actionID string) bool {
	ae.mu.RLock()
	defer ae.mu.RUnlock()

	_, exists := ae.inProgress[actionID]

	return exists
}

// GetActiveActionCount returns the number of actions currently in progress.
func (ae *ActionExecutor) GetActiveActionCount() int {
	ae.mu.RLock()
	defer ae.mu.RUnlock()

	return len(ae.inProgress)
}

// SetOnActionComplete sets a callback that runs after each action execution.
// Call this during executor setup, before Start().
// Thread-safe: runs concurrently with action execution.
func (ae *ActionExecutor) SetOnActionComplete(fn func(deps.ActionResult)) {
	ae.mu.Lock()
	ae.onActionComplete = fn
	ae.mu.Unlock()
}

func (ae *ActionExecutor) Shutdown() {
	// Mark as stopped and capture cancel functions under lock to prevent race with Start()
	ae.mu.Lock()
	ae.stopped = true
	metricsCancel := ae.metricsCancel
	cancel := ae.cancel
	ae.mu.Unlock()

	if metricsCancel != nil {
		metricsCancel()
	}

	ae.metricsWg.Wait()

	ae.closeOnce.Do(func() {
		close(ae.actionQueue)
	})

	if cancel != nil {
		cancel()
	}

	ae.wg.Wait()
}
