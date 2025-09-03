package s6

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

// Desired State

type S6DesiredState struct {
	S6ServiceConfig   s6serviceconfig.S6ServiceConfig
	shutdownRequested bool
}

func (s *S6DesiredState) ShutdownRequested() bool {
	return s.shutdownRequested
}

// Observed State

type S6ObservedState struct {
	ObservedDesiredState S6DesiredState
	ServiceInfo          s6.ServiceInfo
	CollectedAt          time.Time
}

func (s *S6ObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &s.ObservedDesiredState
}

func (s *S6ObservedState) GetTimestamp() time.Time {
	return s.CollectedAt
}
