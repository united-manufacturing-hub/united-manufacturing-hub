package supervisor

import (
	"context"
	"fmt"
	"sync"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
)

type ActionExecutor struct {
	workerCount int
	actionQueue chan actionWork
	inProgress  map[string]bool
	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

type actionWork struct {
	actionID string
	action   fsmv2.Action
}

func NewActionExecutor(workerCount int) *ActionExecutor {
	if workerCount <= 0 {
		workerCount = 10
	}

	return &ActionExecutor{
		workerCount: workerCount,
		actionQueue: make(chan actionWork, workerCount*2),
		inProgress:  make(map[string]bool),
	}
}

func (ae *ActionExecutor) Start(ctx context.Context) {
	ae.ctx, ae.cancel = context.WithCancel(ctx)

	for i := 0; i < ae.workerCount; i++ {
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
			_ = work.action.Execute(ae.ctx)

			ae.mu.Lock()
			delete(ae.inProgress, work.actionID)
			ae.mu.Unlock()
		}
	}
}

func (ae *ActionExecutor) EnqueueAction(actionID string, action fsmv2.Action) error {
	ae.mu.Lock()
	if ae.inProgress[actionID] {
		ae.mu.Unlock()
		return fmt.Errorf("action %s already in progress", actionID)
	}
	ae.inProgress[actionID] = true
	ae.mu.Unlock()

	work := actionWork{
		actionID: actionID,
		action:   action,
	}

	select {
	case ae.actionQueue <- work:
		return nil
	default:
		ae.mu.Lock()
		delete(ae.inProgress, actionID)
		ae.mu.Unlock()
		return fmt.Errorf("action queue full")
	}
}

func (ae *ActionExecutor) HasActionInProgress(actionID string) bool {
	ae.mu.RLock()
	defer ae.mu.RUnlock()
	return ae.inProgress[actionID]
}

func (ae *ActionExecutor) Shutdown() {
	ae.cancel()
	ae.wg.Wait()
}
