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
				return
			}

			ae.executeWorkWithRecovery(work)
		}
	}
}

// executeWorkWithRecovery handles action execution with panic recovery.
func (ae *ActionExecutor) executeWorkWithRecovery(work actionWork) {
	startTime := time.Now()

	actionCtx, cancel := context.WithTimeout(ae.ctx, work.timeout)
	defer cancel()

	var err error

	var status string

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("action panicked: %v", r)
			status = "panic"

			ae.logger.Errorw("action_panic",
				"correlation_id", work.actionID,
				"worker", ae.identity.String(),
				"action_name", work.action.Name(),
				"panic", fmt.Sprintf("%v", r),
				"stack", string(debug.Stack()))
		}

		ae.mu.Lock()
		delete(ae.inProgress, work.actionID)
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

		metrics.RecordActionExecutionDuration(ae.supervisorID, work.action.Name(), status, duration)

		if ae.onActionComplete != nil {
			errorMsg := ""
			if err != nil {
				errorMsg = err.Error()
			}

			ae.onActionComplete(deps.ActionResult{
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
			metrics.RecordActionTimeout(ae.supervisorID, work.action.Name())

			ae.logger.Errorw("action_failed",
				"correlation_id", work.actionID,
				"worker", ae.identity.String(),
				"action_name", work.action.Name(),
				"error", "timeout",
				"duration_ms", duration.Milliseconds(),
				"timeout_ms", work.timeout.Milliseconds())
		} else {
			ae.logger.Errorw("action_failed",
				"correlation_id", work.actionID,
				"worker", ae.identity.String(),
				"action_name", work.action.Name(),
				"error", err.Error(),
				"duration_ms", duration.Milliseconds())
		}
	} else {
		// Success logs at DEBUG - operators only need failures, not routine success
		ae.logger.Debugw("action_completed",
			"correlation_id", work.actionID,
			"worker", ae.identity.String(),
			"action_name", work.action.Name(),
			"duration_ms", duration.Milliseconds())
	}
}

// EnqueueAction adds an action to the execution queue without blocking.
// Returns error if action is already in progress or queue is full.
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

// SetOnActionComplete sets a callback invoked after each action execution.
// Should be called during executor setup, before Start().
func (ae *ActionExecutor) SetOnActionComplete(fn func(deps.ActionResult)) {
	ae.onActionComplete = fn
}

func (ae *ActionExecutor) Shutdown() {
	if ae.metricsCancel != nil {
		ae.metricsCancel()
	}

	ae.metricsWg.Wait()

	ae.closeOnce.Do(func() {
		close(ae.actionQueue)
	})

	if ae.cancel != nil {
		ae.cancel()
	}

	ae.wg.Wait()
}
