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

package actions

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
)

// the state mocker is used for unit testing to mock the state of the system
// it runs in a separate goroutine and regularly updates the state of the system
// it is passed a pointer to the config that is being used by the action unit tests
// it then updates the state of the system according to the config just like the real system would do
type StateMocker struct {
	State  *fsm.SystemSnapshot
	Config *config.FullConfig
}

// NewStateMocker creates a new StateMocker
func NewStateMocker(config *config.FullConfig) *StateMocker {
	return &StateMocker{
		Config: config,
	}
}

// GetState returns the current state of the system
// it is used by the action unit tests to check the state of the system
// it is called in a separate goroutine and regularly updates the state of the system
// it is passed a pointer to the config that is being used by the action unit tests
// it then updates the state of the system according to the config just like the real system would do

func (s *StateMocker) GetState() *fsm.SystemSnapshot {
	return s.State
}

// here, the actual state update logic is implemented
func (s *StateMocker) UpdateState() {
	// only take the dataflowcomponent configs and add them to the state of the dataflowcomponent manager
	// the other managers are not updated

	//start with a basic snapshot

	dfcManagerInstaces := map[string]*fsm.FSMInstanceSnapshot{}

	for _, curDataflowcomponent := range s.Config.DataFlow {
		dfcManagerInstaces[curDataflowcomponent.Name] = &fsm.FSMInstanceSnapshot{
			ID:           curDataflowcomponent.Name,
			DesiredState: curDataflowcomponent.DesiredFSMState,
			CurrentState: curDataflowcomponent.DesiredFSMState,
			LastObservedState: &dataflowcomponent.DataflowComponentObservedStateSnapshot{
				Config: curDataflowcomponent.DataFlowComponentServiceConfig,
			},
		}
	}

	managerSnapshot := &MockManagerSnapshot{
		Instances: dfcManagerInstaces,
	}

	snapshot := &fsm.SystemSnapshot{
		Managers: map[string]fsm.ManagerSnapshot{
			constants.DataflowcomponentManagerName: managerSnapshot,
		},
	}

	s.State = snapshot
}

// UpdateState is spawned as a separate goroutine and updates the state of the system
func (s *StateMocker) Start() {

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		s.UpdateState()
	}
}
