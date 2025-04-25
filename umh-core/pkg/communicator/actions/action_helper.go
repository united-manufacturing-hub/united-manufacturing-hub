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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
)

// CreateMockSystemSnapshot is a helper function to create a mock system snapshot with dataflow components
func CreateMockSystemSnapshot() *fsm.SystemSnapshot {
	// Create a dataflowcomponent manager with test instances
	managerSnapshot := CreateManagerSnapshot()

	// Create and return system snapshot
	return &fsm.SystemSnapshot{
		Managers: map[string]fsm.ManagerSnapshot{
			constants.DataflowcomponentManagerName: managerSnapshot,
		},
	}
}

// CreateManagerSnapshot is a helper function to create a manager snapshot with instances
func CreateManagerSnapshot(additionalInstances ...fsm.FSMInstanceSnapshot) fsm.ManagerSnapshot {
	// Create instance snapshots
	instancesSlice := []fsm.FSMInstanceSnapshot{
		{
			ID:           "test-component-1",
			DesiredState: "active",
			CurrentState: "active",
			LastObservedState: &dataflowcomponent.DataflowComponentObservedStateSnapshot{
				Config: dataflowcomponentconfig.DataFlowComponentConfig{
					BenthosConfig: dataflowcomponentconfig.BenthosConfig{
						Input: map[string]interface{}{
							"kafka": map[string]interface{}{
								"addresses": []string{"localhost:9092"},
								"topics":    []string{"test-topic"},
							},
						},
						Output: map[string]interface{}{
							"kafka": map[string]interface{}{
								"addresses": []string{"localhost:9092"},
								"topic":     "output-topic",
							},
						},
						Pipeline: map[string]interface{}{
							"processors": []interface{}{
								map[string]interface{}{
									"mapping": "root = this",
								},
							},
						},
						CacheResources: []map[string]interface{}{
							{
								"label":  "test-cache",
								"memory": map[string]interface{}{},
							},
						},
						RateLimitResources: []map[string]interface{}{
							{
								"label": "test-rate-limit",
								"local": map[string]interface{}{},
							},
						},
						Buffer: map[string]interface{}{
							"memory": map[string]interface{}{},
						},
					},
				},
			},
		},
		{
			ID:           "test-component-2",
			DesiredState: "active",
			CurrentState: "active",
			LastObservedState: &dataflowcomponent.DataflowComponentObservedStateSnapshot{
				Config: dataflowcomponentconfig.DataFlowComponentConfig{
					BenthosConfig: dataflowcomponentconfig.BenthosConfig{
						Input: map[string]interface{}{
							"file": map[string]interface{}{
								"paths": []string{"/tmp/input.txt"},
							},
						},
						Output: map[string]interface{}{
							"file": map[string]interface{}{
								"path": "/tmp/output.txt",
							},
						},
						Pipeline: map[string]interface{}{
							"processors": []interface{}{
								map[string]interface{}{
									"text": map[string]interface{}{
										"operator": "to_upper",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Convert slice to map required by ManagerSnapshot interface
	instances := make(map[string]*fsm.FSMInstanceSnapshot)
	for i := range instancesSlice {
		instance := instancesSlice[i]
		instances[instance.ID] = &instance
	}

	// Add additional instances if provided
	for _, instance := range additionalInstances {
		instances[instance.ID] = &instance
	}

	return &MockManagerSnapshot{
		Instances: instances,
	}
}

// CreateMockSystemSnapshotWithMissingState is a helper function to create a system snapshot with a component that has no observed state
func CreateMockSystemSnapshotWithMissingState() *fsm.SystemSnapshot {
	// Create a dataflowcomponent manager with an instance that has no observed state
	instanceSlice := []fsm.FSMInstanceSnapshot{
		{
			ID:                "test-component-missing-state",
			DesiredState:      "active",
			CurrentState:      "active",
			LastObservedState: nil, // No observed state
		},
	}

	// Convert slice to map
	instances := make(map[string]*fsm.FSMInstanceSnapshot)
	for i := range instanceSlice {
		instance := instanceSlice[i]
		instances[instance.ID] = &instance
	}

	managerSnapshot := &MockManagerSnapshot{
		Instances: instances,
	}

	// Create and return system snapshot
	return &fsm.SystemSnapshot{
		Managers: map[string]fsm.ManagerSnapshot{
			constants.DataflowcomponentManagerName: managerSnapshot,
		},
	}
}

// MockManagerSnapshot is a simple implementation of ManagerSnapshot interface for testing
type MockManagerSnapshot struct {
	Instances map[string]*fsm.FSMInstanceSnapshot
}

func (m *MockManagerSnapshot) GetName() string {
	return constants.DataflowcomponentManagerName
}

func (m *MockManagerSnapshot) GetInstances() map[string]*fsm.FSMInstanceSnapshot {
	return m.Instances
}

func (m *MockManagerSnapshot) GetSnapshotTime() time.Time {
	return time.Now()
}

func (m *MockManagerSnapshot) GetManagerTick() uint64 {
	return 0
}

// MockObservedState is a fake implementation of ObservedStateSnapshot for testing
type MockObservedState struct{}

func (m *MockObservedState) IsObservedStateSnapshot() {}
