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

package fsm

import (
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
)

// FindManager finds a manager in the system snapshot.
// Returns nil and false if the manager is not found.
func FindManager(
	snap SystemSnapshot,
	managerName string,
) (ManagerSnapshot, bool) {
	mgr, ok := snap.Managers[managerName]
	return mgr, ok
}

// FindInstance finds an instance in the system snapshot.
// This is useful if we want to fetch the data from a manager that always has one instance (e.g., core, agent, container, redpanda).
// Returns nil and false if the instance is not found.
func FindInstance(
	snap SystemSnapshot,
	managerName, instanceName string,
) (*FSMInstanceSnapshot, bool) {
	mgr, ok := FindManager(snap, managerName)
	if !ok {
		return nil, false
	}
	inst, ok := mgr.GetInstances()[instanceName]
	return inst, ok
}

// FindDfcInstanceByUUID finds a dataflow component instance with the given UUID.
// It returns the instance if found, otherwise an error is returned.
func FindDfcInstanceByUUID(systemSnapshot SystemSnapshot, dfcUUID string) (*FSMInstanceSnapshot, error) {
	dfcManager, ok := FindManager(systemSnapshot, constants.DataflowcomponentManagerName)
	if !ok {
		return nil, fmt.Errorf("dfc manager not found")
	}

	dfcInstances := dfcManager.GetInstances()

	for _, instance := range dfcInstances {
		if instance == nil {
			continue
		}

		currentUUID := dataflowcomponentserviceconfig.GenerateUUIDFromName(instance.ID).String()
		if currentUUID == dfcUUID {
			return instance, nil
		}
	}

	return nil, fmt.Errorf("the requested DFC with UUID %s was not found", dfcUUID)
}
