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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/container_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/snapshot"
)

// ContainerObservedStateSnapshot is a copy of the current metrics so the manager can store snapshots.
type ContainerObservedStateSnapshot struct {
	ServiceInfoSnapshot container_monitor.ServiceInfo
	// No need for the config, as it is basically empty
}

// Ensure it satisfies fsm.ObservedStateSnapshot
func (c *ContainerObservedStateSnapshot) IsObservedStateSnapshot() {}

// CreateObservedStateSnapshot is called by the manager to record the state
func (c *ContainerInstance) CreateObservedStateSnapshot() snapshot.ObservedStateSnapshot {
	snapshot := &ContainerObservedStateSnapshot{}
	if c.ObservedState.ServiceInfo != nil {
		deepcopy.Copy(&snapshot.ServiceInfoSnapshot, c.ObservedState.ServiceInfo)
	}
	return snapshot
}
