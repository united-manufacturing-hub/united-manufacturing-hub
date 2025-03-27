package actions

/*
 ActionTracker is used to track the execution time of actions.
 We already monitor it in the frontend, but for ginkgo e2e tests we need to track it here as well.
 This allows us to see the execution time of actions in the logs and find stuck actions.
*/

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/shared/models"
	"go.uber.org/zap"
)

var (
	actionTracker *ActionTracker
	trackerOnce   sync.Once
)

func InitializeActionTracker(dog watchdog.Iface) *ActionTracker {
	trackerOnce.Do(func() {
		actionTracker = newActionTracker(dog)
	})
	return actionTracker
}

// GetActionTracker returns the singleton instance of ActionTracker
func GetActionTracker() *ActionTracker {
	return actionTracker
}

type TrackedAction struct {
	ActionType models.ActionType
	StartTime  time.Time
}

// ActionTracker keeps track of ongoing actions and their execution times
type ActionTracker struct {
	actions sync.Map // map[uuid.UUID]TrackedAction
	done    chan struct{}
	dog     watchdog.Iface
}

// NewActionTracker creates a new ActionTracker and starts the monitoring goroutine
func newActionTracker(dog watchdog.Iface) *ActionTracker {
	tracker := &ActionTracker{
		done: make(chan struct{}),
		dog:  dog,
	}
	go tracker.monitor()
	return tracker
}

// StartTracking adds an action to the tracker
func (t *ActionTracker) StartTracking(actionUUID uuid.UUID, actionType models.ActionType) {
	t.actions.Store(actionUUID, TrackedAction{ActionType: actionType, StartTime: time.Now()})
	zap.S().Debugf("[ActionTracker] Started tracking action %s [%s]", actionUUID, actionType)
}

// StopTracking removes an action from the tracker and reports its execution time
func (t *ActionTracker) StopTracking(actionUUID uuid.UUID) {
	if value, ok := t.actions.LoadAndDelete(actionUUID); ok {
		trackedAction, ok := value.(TrackedAction)
		if !ok {
			zap.S().Errorf("[ActionTracker] Invalid action type for %s: expected TrackedAction, got %T", actionUUID, value)
			return
		}
		elapsed := time.Since(trackedAction.StartTime)
		zap.S().Debugf("[ActionTracker] Completed action %s [%s] in %v", actionUUID, trackedAction.ActionType, elapsed)
	} else {
		zap.S().Debugf("[ActionTracker] Action %s not found", actionUUID)
	}
}

// monitor periodically checks for long-running actions
func (t *ActionTracker) monitor() {
	// Skip watchdog registration if no dog provided
	var heartbeatID uuid.UUID
	if t.dog != nil {
		// Register this goroutine with the watchdog
		// 3 warnings until failure, 10 second timeout, don't require subscribers
		heartbeatID = t.dog.RegisterHeartbeat("action-tracker-monitor", 3, 10, false)
		defer t.dog.UnregisterHeartbeat(heartbeatID)
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			t.reportRunningActions()
			if t.dog != nil {
				// Report healthy heartbeat
				t.dog.ReportHeartbeatStatus(heartbeatID, watchdog.HEARTBEAT_STATUS_OK)
			}
		case <-t.done:
			if t.dog != nil {
				// Report final heartbeat before exiting
				t.dog.ReportHeartbeatStatus(heartbeatID, watchdog.HEARTBEAT_STATUS_OK)
			}
			return
		}
	}
}

// reportRunningActions logs information about all ongoing actions
func (t *ActionTracker) reportRunningActions() {
	var runningActions []string

	t.actions.Range(func(key, value interface{}) bool {
		actionUUID, ok := key.(uuid.UUID)
		if !ok {
			zap.S().Errorf("[ActionTracker] Invalid key type: expected uuid.UUID, got %T", key)
			return true
		}

		action, ok := value.(TrackedAction)
		if !ok {
			zap.S().Errorf("[ActionTracker] Invalid value type for %s: expected TrackedAction, got %T", actionUUID, value)
			return true
		}

		elapsed := time.Since(action.StartTime)
		runningActions = append(runningActions, fmt.Sprintf("%s [%s] (running for %v)", actionUUID, action.ActionType, elapsed))
		return true
	})

	if len(runningActions) > 0 {
		zap.S().Debugf("[ActionTracker] === Currently running actions ===\n%v\n", strings.Join(runningActions, "\n"))
	}
}

// Stop stops the action tracker
func (t *ActionTracker) Stop() {
	close(t.done)
}
