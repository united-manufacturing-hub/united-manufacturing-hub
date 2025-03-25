package benthos

import (
	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/service/benthos"
)

// BenthosObservedStateSnapshot is a deep-copyable snapshot of BenthosObservedState
type BenthosObservedStateSnapshot struct {
	Config      config.BenthosServiceConfig
	ServiceInfo benthos.ServiceInfo
}

// IsObservedStateSnapshot implements the fsm.ObservedStateSnapshot interface
func (s *BenthosObservedStateSnapshot) IsObservedStateSnapshot() {
	// Marker method implementation
}

// CreateObservedStateSnapshot implements the fsm.ObservedStateConverter interface for BenthosInstance
func (b *BenthosInstance) CreateObservedStateSnapshot() fsm.ObservedStateSnapshot {
	// Create a deep copy of the observed state
	snapshot := &BenthosObservedStateSnapshot{}

	// Deep copy config
	snapshot.Config = b.config

	// Deep copy service info
	snapshot.ServiceInfo = b.ObservedState.ServiceInfo

	return snapshot
}
