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
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
)

type ActionExecutor struct {
	workerCount    int
	actionQueue    chan actionWork
	inProgress     map[string]bool
	mu             sync.RWMutex
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	timeouts       map[string]time.Duration
	defaultTimeout time.Duration
}

type actionWork struct {
	actionID string
	action   fsmv2.Action
	timeout  time.Duration
}

func NewActionExecutor(workerCount int) *ActionExecutor {
	if workerCount <= 0 {
		workerCount = 10
	}

	return &ActionExecutor{
		workerCount:    workerCount,
		actionQueue:    make(chan actionWork, workerCount*2),
		inProgress:     make(map[string]bool),
		timeouts:       make(map[string]time.Duration),
		defaultTimeout: 30 * time.Second,
	}
}

func NewActionExecutorWithTimeout(workerCount int, timeouts map[string]time.Duration) *ActionExecutor {
	if workerCount <= 0 {
		workerCount = 10
	}

	return &ActionExecutor{
		workerCount:    workerCount,
		actionQueue:    make(chan actionWork, workerCount*2),
		inProgress:     make(map[string]bool),
		timeouts:       timeouts,
		defaultTimeout: 30 * time.Second,
	}
}

func (ae *ActionExecutor) Start(ctx context.Context) {
	ae.ctx, ae.cancel = context.WithCancel(ctx)

	for range ae.workerCount {
		ae.wg.Add(1)

		go ae.worker()
	}
}

func (ae *ActionExecutor) worker() {
	defer ae.wg.Done()

	for {
		select {
		case <-ae.ctx.Done():
			return

		case work := <-ae.actionQueue:
			actionCtx, cancel := context.WithTimeout(ae.ctx, work.timeout)
			_ = work.action.Execute(actionCtx)

			cancel()

			ae.mu.Lock()
			delete(ae.inProgress, work.actionID)
			ae.mu.Unlock()
		}
	}
}

// EnqueueAction adds an action to the execution queue without blocking.
// It uses a buffered channel with select/default to ensure non-blocking behavior.
// If the queue is full, it returns an error immediately without waiting.
//
// Performance: <1ms latency, even under high load (100+ concurrent actions).
//
// Thread-safe: Multiple goroutines can call EnqueueAction concurrently.
func (ae *ActionExecutor) EnqueueAction(actionID string, action fsmv2.Action) error {
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
	}

	select {
	case ae.actionQueue <- work:
		return nil
	default:
		ae.mu.Lock()
		delete(ae.inProgress, actionID)
		ae.mu.Unlock()

		return errors.New("action queue full")
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

func (ae *ActionExecutor) Shutdown() {
	if ae.cancel != nil {
		ae.cancel()
	}

	ae.wg.Wait()
}
