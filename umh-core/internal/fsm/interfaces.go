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
}
