package watchdog

import (
	"github.com/google/uuid"
)

type FakeWatchdog struct {
}

func NewFakeWatchdog() *FakeWatchdog {
	return &FakeWatchdog{}
}

func (f *FakeWatchdog) Start() {

}

func (f *FakeWatchdog) RegisterHeartbeat(name string, warningsUntilFailure uint64, timeout uint64, onlyIfSubscribers bool) uuid.UUID {
	return uuid.New()
}

func (f *FakeWatchdog) UnregisterHeartbeat(uniqueIdentifier uuid.UUID) {

}

func (f *FakeWatchdog) ReportHeartbeatStatus(uniqueIdentifier uuid.UUID, status HeartbeatStatus) {

}

func (f *FakeWatchdog) SetHasSubscribers(has bool) {

}
