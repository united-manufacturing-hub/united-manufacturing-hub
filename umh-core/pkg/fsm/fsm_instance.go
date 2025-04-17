package fsm

import (
	"context"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/snapshot"
)

// FSMInstance defines the interface for a finite state machine instance.
// Each instance has a current state and a desired state, and can be reconciled
// to move toward the desired state.
type FSMInstance interface {
	// GetCurrentFSMState returns the current state of the instance
	GetCurrentFSMState() string
	// GetDesiredFSMState returns the desired state of the instance
	GetDesiredFSMState() string
	// SetDesiredFSMState sets the desired state of the instance
	SetDesiredFSMState(desiredState string) error
	// Reconcile moves the instance toward its desired state
	// Returns an error if reconciliation fails, and a boolean indicating
	// whether a change was made to the instance's state
	// The filesystemService parameter is used to read and write to the filesystem.
	// Specifically it is used so that we only need to read in the entire file system once, and then can pass it to all the managers and instances, who can then save on I/O operations.
	Reconcile(ctx context.Context, currentSnapshot snapshot.SystemSnapshot, filesystemService filesystem.Service) (error, bool)
	// Remove initiates the removal process for this instance
	Remove(ctx context.Context) error
	// GetLastObservedState returns the last known state of the instance
	// This is cached data from the last reconciliation cycle
	GetLastObservedState() ObservedState
	// GetExpectedMaxP95ExecutionTimePerInstance returns the expected max p95 execution time of the instance
	GetExpectedMaxP95ExecutionTimePerInstance() time.Duration
}
