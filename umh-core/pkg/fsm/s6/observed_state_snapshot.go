package s6

import (
	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/service/s6"
)

// S6ObservedStateSnapshot is a deep-copyable snapshot of S6ObservedState
type S6ObservedStateSnapshot struct {
	Config                  config.S6FSMConfig
	LastStateChange         int64
	ServiceInfo             s6.ServiceInfo
	ObservedS6ServiceConfig config.S6ServiceConfig
}

// IsObservedStateSnapshot implements the fsm.ObservedStateSnapshot interface
func (s *S6ObservedStateSnapshot) IsObservedStateSnapshot() {
	// Marker method implementation
}

// S6InstanceConverter implements the fsm.ObservedStateConverter interface for S6Instance
func (s *S6Instance) CreateObservedStateSnapshot() fsm.ObservedStateSnapshot {
	// Create a deep copy of the observed state
	snapshot := &S6ObservedStateSnapshot{
		LastStateChange: s.ObservedState.LastStateChange,
	}

	// Deep copy config
	snapshot.Config = s.config

	// Deep copy service info
	snapshot.ServiceInfo = s.ObservedState.ServiceInfo

	// Deep copy observed config
	snapshot.ObservedS6ServiceConfig = s.ObservedState.ObservedS6ServiceConfig

	return snapshot
}
