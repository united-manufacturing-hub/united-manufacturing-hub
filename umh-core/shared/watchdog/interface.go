package watchdog

import (
	"github.com/google/uuid"
)

type Iface interface {
	Start()
	RegisterHeartbeat(name string, warningsUntilFailure uint64, timeout uint64, onlyIfSubscribers bool) uuid.UUID
	UnregisterHeartbeat(uniqueIdentifier uuid.UUID)
	ReportHeartbeatStatus(uniqueIdentifier uuid.UUID, status HeartbeatStatus)
	SetHasSubscribers(has bool)
}
