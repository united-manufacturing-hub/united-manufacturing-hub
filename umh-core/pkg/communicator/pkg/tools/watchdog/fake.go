// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
