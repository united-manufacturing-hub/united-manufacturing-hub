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

package container

import (
	"github.com/tiendc/go-deepcopy"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/container_monitor"
)

// ContainerObservedStateSnapshot is a copy of the current metrics so the manager can store snapshots.
type ContainerObservedStateSnapshot struct {
	// For example, copy the entire *models.Container if that's your data
	ContainerStatusSnapshot container_monitor.ContainerStatus
}

// Ensure it satisfies fsm.ObservedStateSnapshot
func (c *ContainerObservedStateSnapshot) IsObservedStateSnapshot() {}

// CreateObservedStateSnapshot is called by the manager to record the state
func (c *ContainerInstance) CreateObservedStateSnapshot() fsm.ObservedStateSnapshot {
	snapshot := &ContainerObservedStateSnapshot{}
	if c.ObservedState.ContainerStatus != nil {
		deepcopy.Copy(&snapshot.ContainerStatusSnapshot, c.ObservedState.ContainerStatus)
	}
	return snapshot
}
