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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/metrics"
)

type ActionExecutor struct {
	supervisorID     string
	identity         fsmv2.Identity // Worker identity for logging
	workerCount      int
	actionQueue      chan actionWork
	inProgress       map[string]bool
	mu               sync.RWMutex
	ctx              context.Context
	cancel           context.CancelFunc
	wg               sync.WaitGroup
	timeouts         map[string]time.Duration
	defaultTimeout   time.Duration
	metricsCancel    context.CancelFunc
	metricsWg        sync.WaitGroup
	logger           *zap.SugaredLogger
	closeOnce        sync.Once                       // Ensures actionQueue is closed only once
	onActionComplete func(result fsmv2.ActionResult) // Called after each action execution (for auto-recording to ActionHistory)
}

type actionWork struct {
	actionID string
	action   fsmv2.Action[any]
	timeout  time.Duration
	deps     any // Dependencies to pass to the action
}

func NewActionExecutor(workerCount int, supervisorID string, identity fsmv2.Identity, logger *zap.SugaredLogger) *ActionExecutor {
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

func NewActionExecutorWithTimeout(workerCount int, timeouts map[string]time.Duration, supervisorID string, identity fsmv2.Identity, logger *zap.SugaredLogger) *ActionExecutor {
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
	ae.ctx, ae.cancel = context.WithCancel(ctx)

	for range ae.workerCount {
		ae.wg.Add(1)

		go ae.worker()
	}

	metricsCtx, metricsCancel := context.WithCancel(ctx)
	ae.metricsCancel = metricsCancel
	ae.metricsWg.Add(1)

	go ae.metricsReporter(metricsCtx)
}

func (ae *ActionExecutor) worker() {
	defer ae.wg.Done()

	for {
		select {
		case <-ae.ctx.Done():
			return

		case work, ok := <-ae.actionQueue:
			if !ok {
				// Channel closed, exit gracefully
				return
			}

			ae.executeWorkWithRecovery(work)
		}
	}
}

// executeWorkWithRecovery handles action execution with panic recovery.
// Panic recovery prevents a panicking action from crashing the worker goroutine,
// allowing the executor to remain functional.
func (ae *ActionExecutor) executeWorkWithRecovery(work actionWork) {
	startTime := time.Now()

	actionCtx, cancel := context.WithTimeout(ae.ctx, work.timeout)
	defer cancel()

	var err error

	var status string

	// Panic recovery - ensures worker survives and in-progress is cleared
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("action panicked: %v", r)
			status = "panic"

			ae.logger.Errorw("action_panic",
				"worker", ae.identity.String(),
				"action", work.actionID,
				"action_name", work.action.Name(),
				"panic", fmt.Sprintf("%v", r),
				"stack", string(debug.Stack()))
		}

		// Always clear in-progress status (even after panic)
		ae.mu.Lock()
		delete(ae.inProgress, work.actionID)
		ae.mu.Unlock()

		// Record metrics
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

		metrics.RecordActionExecutionDuration(ae.supervisorID, work.action.Name(), status, duration)

		// Auto-record to ActionHistory if callback is set
		if ae.onActionComplete != nil {
			errorMsg := ""
			if err != nil {
				errorMsg = err.Error()
			}

			ae.onActionComplete(fsmv2.ActionResult{
				Timestamp:  time.Now(),
				ActionType: work.action.Name(),
				Success:    status == "success",
				ErrorMsg:   errorMsg,
				Latency:    duration,
			})
		}
	}()

	// Execute action
	err = work.action.Execute(actionCtx, work.deps)

	// Handle normal completion (non-panic)
	duration := time.Since(startTime)

	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			metrics.RecordActionTimeout(ae.supervisorID, work.action.Name())

			ae.logger.Errorw("action_failed",
				"worker", ae.identity.String(),
				"action", work.action.Name(),
				"error", "timeout",
				"duration_ms", duration.Milliseconds(),
				"timeout_ms", work.timeout.Milliseconds())
		} else {
			ae.logger.Errorw("action_failed",
				"worker", ae.identity.String(),
				"action", work.action.Name(),
				"error", err.Error(),
				"duration_ms", duration.Milliseconds())
		}
	} else {
		// Success logs at DEBUG - operators only need failures, not routine success
		ae.logger.Debugw("action_completed",
			"worker", ae.identity.String(),
			"action", work.action.Name(),
			"duration_ms", duration.Milliseconds(),
			"success", true)
	}
}

// EnqueueAction adds an action to the execution queue without blocking.
// It uses a buffered channel with select/default to ensure non-blocking behavior.
// If the queue is full, it returns an error immediately without waiting.
//
// The deps parameter provides worker dependencies to the action during execution.
// If deps is nil, the action's Execute method will receive nil (useful for stub actions).
//
// Performance: <1ms latency, even under high load (100+ concurrent actions).
//
// Thread-safe: Multiple goroutines can call EnqueueAction concurrently.
func (ae *ActionExecutor) EnqueueAction(actionID string, action fsmv2.Action[any], deps any) error {
	ae.mu.Lock()

	if ae.inProgress[actionID] {
		ae.mu.Unlock()

		return fmt.Errorf("action %s already in progress", actionID)
	}

	ae.inProgress[actionID] = true
	ae.mu.Unlock()

	timeout, exists := ae.timeouts[actionID]
	if !exists {
		timeout = ae.defaultTimeout
	}

	work := actionWork{
		actionID: actionID,
		action:   action,
		timeout:  timeout,
		deps:     deps,
	}

	select {
	case ae.actionQueue <- work:
		metrics.RecordActionQueued(ae.supervisorID, action.Name())

		return nil
	default:
		ae.mu.Lock()
		delete(ae.inProgress, actionID)
		ae.mu.Unlock()

		return errors.New("action queue full")
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

			metrics.RecordWorkerPoolQueueSize(ae.supervisorID, queueSize)

			utilization := float64(inProgressCount) / float64(ae.workerCount)
			metrics.RecordWorkerPoolUtilization(ae.supervisorID, utilization)
		}
	}
}

// HasActionInProgress checks if an action is currently executing.
// This method is non-blocking and uses a read lock for concurrent access.
//
// Performance: <1ms latency, safe to call from tick loop.
//
// Thread-safe: Multiple goroutines can call HasActionInProgress concurrently.
func (ae *ActionExecutor) HasActionInProgress(actionID string) bool {
	ae.mu.RLock()
	defer ae.mu.RUnlock()

	return ae.inProgress[actionID]
}

// GetActiveActionCount returns the number of actions currently in progress.
// This is used for heartbeat logging to show system activity.
//
// Thread-safe: Uses read lock for concurrent access.
func (ae *ActionExecutor) GetActiveActionCount() int {
	ae.mu.RLock()
	defer ae.mu.RUnlock()

	return len(ae.inProgress)
}

// SetOnActionComplete sets a callback to be called after each action execution.
// This is used by the supervisor to auto-record action results to ActionHistory.
// The callback receives an ActionResult with action name, success/failure, error message, and latency.
//
// Thread-safe: This method should be called during executor setup, before Start().
func (ae *ActionExecutor) SetOnActionComplete(fn func(fsmv2.ActionResult)) {
	ae.onActionComplete = fn
}

func (ae *ActionExecutor) Shutdown() {
	if ae.metricsCancel != nil {
		ae.metricsCancel()
	}

	ae.metricsWg.Wait()

	// Close the action queue to wake blocked workers.
	// Workers receiving on a closed channel get the zero value and ok=false.
	// Use sync.Once to ensure idempotency - Shutdown may be called multiple times.
	ae.closeOnce.Do(func() {
		close(ae.actionQueue)
	})

	if ae.cancel != nil {
		ae.cancel()
	}

	ae.wg.Wait()
}
