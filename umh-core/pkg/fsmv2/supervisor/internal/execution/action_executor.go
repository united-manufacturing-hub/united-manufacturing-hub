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

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	fsmv2sentry "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/sentry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/metrics"
)

type ActionExecutor struct {
	ctx              context.Context
	actionQueue      chan actionWork
	inProgress       map[string]bool
	cancel           context.CancelFunc
	timeouts         map[string]time.Duration
	metricsCancel    context.CancelFunc
	logger           *zap.SugaredLogger
	onActionComplete func(result deps.ActionResult)
	identity         deps.Identity
	supervisorID     string
	wg               sync.WaitGroup
	metricsWg        sync.WaitGroup
	workerCount      int
	defaultTimeout   time.Duration
	mu               sync.RWMutex
	closeOnce        sync.Once
	stopped          bool
}

type actionWork struct {
	action   fsmv2.Action[any]
	deps     any
	actionID string
	timeout  time.Duration
}

func NewActionExecutor(workerCount int, supervisorID string, identity deps.Identity, logger *zap.SugaredLogger) *ActionExecutor {
	if workerCount <= 0 {
		workerCount = 10
	}

	return &ActionExecutor{
		supervisorID:   supervisorID,
		identity:       identity,
		workerCount:    workerCount,
		actionQueue:    make(chan actionWork, workerCount*2),
		inProgress:     make(map[string]bool),
		timeouts:       make(map[string]time.Duration),
		defaultTimeout: 30 * time.Second,
		logger:         logger,
	}
}

func NewActionExecutorWithTimeout(workerCount int, timeouts map[string]time.Duration, supervisorID string, identity deps.Identity, logger *zap.SugaredLogger) *ActionExecutor {
	if workerCount <= 0 {
		workerCount = 10
	}

	return &ActionExecutor{
		supervisorID:   supervisorID,
		identity:       identity,
		workerCount:    workerCount,
		actionQueue:    make(chan actionWork, workerCount*2),
		inProgress:     make(map[string]bool),
		timeouts:       timeouts,
		defaultTimeout: 30 * time.Second,
		logger:         logger,
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
			err = errors.New("action panicked")
			status = "panic"

			// Extract stack manually because Sentry SDK's ExtractStacktrace() only works on
			// error types with stacktrace interfaces. Panic recovery values are plain
			// interface{}, so capture debug.Stack() explicitly for Sentry visibility.
			ae.logger.Errorw("action_panic", append(fsmv2sentry.ErrorFields{
				Feature:       "fsmv2",
				Err:           err,
				HierarchyPath: ae.identity.HierarchyPath,
			}.ZapFields(),
				"correlation_id", work.actionID,
				"action_name", work.action.Name(),
				"panic", fmt.Sprintf("%v", r),
				"stack", string(debug.Stack()))...)
		}

		ae.mu.Lock()
		delete(ae.inProgress, work.actionID)
		callback := ae.onActionComplete
		ae.mu.Unlock()

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

			ae.logger.Errorw("action_failed", append(fsmv2sentry.ErrorFields{
				Feature:       "fsmv2",
				Err:           err,
				HierarchyPath: ae.identity.HierarchyPath,
			}.ZapFields(),
				"correlation_id", work.actionID,
				"action_name", work.action.Name(),
				"duration_ms", duration.Milliseconds(),
				"timeout_ms", work.timeout.Milliseconds())...)
		} else {
			ae.logger.Errorw("action_failed", append(fsmv2sentry.ErrorFields{
				Feature:       "fsmv2",
				Err:           err,
				HierarchyPath: ae.identity.HierarchyPath,
			}.ZapFields(),
				"correlation_id", work.actionID,
				"action_name", work.action.Name(),
				"duration_ms", duration.Milliseconds())...)
		}
	} else {
		// Success logs at DEBUG - operators only need failures, not routine success
		ae.logger.Debugw("action_completed",
			"hierarchy_path", ae.identity.HierarchyPath,
			"correlation_id", work.actionID,
			"action_name", work.action.Name(),
			"duration_ms", duration.Milliseconds())
	}
}

// EnqueueAction adds an action to the execution queue without blocking.
// Returns error if action is already in progress, queue is full, or executor is stopped.
func (ae *ActionExecutor) EnqueueAction(actionID string, action fsmv2.Action[any], deps any) error {
	ae.mu.Lock()

	// Check if executor is stopped to prevent sending on closed channel
	if ae.stopped {
		ae.mu.Unlock()

		ae.logger.Warnw("action_enqueue_rejected",
			"hierarchy_path", ae.identity.HierarchyPath,
			"correlation_id", actionID,
			"action_name", action.Name(),
			"reason", "executor_stopped")

		return errors.New("executor stopped")
	}

	if ae.inProgress[actionID] {
		ae.mu.Unlock()

		ae.logger.Warnw("action_enqueue_rejected",
			"hierarchy_path", ae.identity.HierarchyPath,
			"correlation_id", actionID,
			"action_name", action.Name(),
			"reason", "already_in_progress")

		return errors.New("action already in progress")
	}

	ae.inProgress[actionID] = true

	// Read timeout while still holding lock to prevent race with concurrent access
	timeout, exists := ae.timeouts[actionID]
	if !exists {
		timeout = ae.defaultTimeout
	}
	ae.mu.Unlock()

	work := actionWork{
		actionID: actionID,
		action:   action,
		timeout:  timeout,
		deps:     deps,
	}

	select {
	case ae.actionQueue <- work:
		metrics.RecordActionQueued(ae.identity.HierarchyPath, action.Name())

		return nil
	default:
		ae.mu.Lock()
		delete(ae.inProgress, actionID)
		ae.mu.Unlock()

		queueErr := errors.New("action queue full")
		ae.logger.Errorw("action_queue_full", append(fsmv2sentry.ErrorFields{
			Feature:       "fsmv2",
			Err:           queueErr,
			HierarchyPath: ae.identity.HierarchyPath,
		}.ZapFields(),
			"correlation_id", actionID,
			"action_name", action.Name(),
			"queue_capacity", cap(ae.actionQueue),
			"worker_count", ae.workerCount)...)

		return queueErr
	}
}

func (ae *ActionExecutor) metricsReporter(ctx context.Context) {
	defer ae.metricsWg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ae.mu.RLock()
			queueSize := len(ae.actionQueue)
			inProgressCount := len(ae.inProgress)
			ae.mu.RUnlock()

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

	return ae.inProgress[actionID]
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
