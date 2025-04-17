package fsm

import (
	"context"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// FSMActions defines the standard lifecycle actions that all FSM instances should implement
type FSMInstanceActions interface {
	// CreateInstance initiates the creation of a managed instance
	CreateInstance(ctx context.Context, filesystemService filesystem.Service) error

	// RemoveInstance initiates the removal of a managed instance
	RemoveInstance(ctx context.Context, filesystemService filesystem.Service) error

	// StartInstance initiates the starting of a managed instance
	StartInstance(ctx context.Context, filesystemService filesystem.Service) error

	// StopInstance initiates the stopping of a managed instance
	StopInstance(ctx context.Context, filesystemService filesystem.Service) error

	// UpdateObservedStateOfInstance updates the observed state of the instance
	UpdateObservedStateOfInstance(ctx context.Context, filesystemService filesystem.Service, tick uint64, loopStartTime time.Time) error

	// ForceRemoveInstance forces the removal of a managed instance
	ForceRemoveInstance(ctx context.Context, filesystemService filesystem.Service) error
}

// FSMInstanceChecks defines the standard checks that all FSM instances should implement
type FSMInstanceChecks interface {
	// IsRemoving returns true if the instance is in the removing state
	// A default implementation is provided by the baseFSMInstance
	IsRemoving() bool

	// IsRemoved returns true if the instance is in the removed state
	// A default implementation is provided by the baseFSMInstance
	IsRemoved() bool

	// IsStopping returns true if the instance is in the stopping state
	// No default implementation is provided by the baseFSMInstance
	IsStopping() bool

	// IsStopped returns true if the instance is in the stopped state
	// No default implementation is provided by the baseFSMInstance
	IsStopped() bool
}

// FSMInstanceReconcile defines the standard reconcile logics that all FSM instances should implement
type FSMInstanceReconcile interface {
	ReconcileOperationalStates(ctx context.Context, currentState string, desiredState string, filesystemService filesystem.Service, currentTime time.Time) (err error, reconciled bool)
}
