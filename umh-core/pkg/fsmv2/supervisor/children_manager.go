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

package supervisor

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// ChildrenManager provides a read-only view of children for parent workers.
// It implements the config.ChildrenView interface.
type ChildrenManager struct {
	children map[string]SupervisorInterface
}

// NewChildrenManager creates a new ChildrenManager wrapping the given children map.
func NewChildrenManager(children map[string]SupervisorInterface) *ChildrenManager {
	return &ChildrenManager{
		children: children,
	}
}

// List returns info about all children.
// Implements config.ChildrenView.
func (m *ChildrenManager) List() []config.ChildInfo {
	infos := make([]config.ChildInfo, 0, len(m.children))
	for name, child := range m.children {
		infos = append(infos, m.buildChildInfo(name, child))
	}

	return infos
}

// Get returns info about a specific child by name, or nil if not found.
// Implements config.ChildrenView.
func (m *ChildrenManager) Get(name string) *config.ChildInfo {
	child, exists := m.children[name]
	if !exists {
		return nil
	}

	info := m.buildChildInfo(name, child)

	return &info
}

// Counts returns the number of healthy and unhealthy children.
// healthy = PhaseRunningHealthy only
// unhealthy = everything except PhaseRunningHealthy and PhaseStopped
// Implements config.ChildrenView.
func (m *ChildrenManager) Counts() (healthy, unhealthy int) {
	for _, child := range m.children {
		phase := child.GetLifecyclePhase()

		if phase.IsHealthy() {
			healthy++
		} else if !phase.IsStopped() {
			// Everything except healthy and stopped is unhealthy
			// This includes: PhaseUnknown, PhaseStarting, PhaseRunningDegraded, PhaseStopping
			unhealthy++
		}
		// Stopped states are neither healthy nor unhealthy
	}

	return healthy, unhealthy
}

// AllHealthy returns true if all children are PhaseRunningHealthy (or there are no children).
// Note: PhaseRunningDegraded is NOT healthy (even though it IS operational).
// Implements config.ChildrenView.
func (m *ChildrenManager) AllHealthy() bool {
	if len(m.children) == 0 {
		return true
	}

	for _, child := range m.children {
		if !child.GetLifecyclePhase().IsHealthy() {
			return false
		}
	}

	return true
}

// AllOperational returns true if all children are operational (Healthy OR Degraded).
// Use this for "can system serve requests?" checks.
// Implements config.ChildrenView.
func (m *ChildrenManager) AllOperational() bool {
	if len(m.children) == 0 {
		return true
	}

	for _, child := range m.children {
		if !child.GetLifecyclePhase().IsOperational() {
			return false
		}
	}

	return true
}

// AllStopped returns true if all children are in the Stopped state (or there are no children).
// Uses lifecycle phase check via phase.IsStopped().
// Implements config.ChildrenView.
func (m *ChildrenManager) AllStopped() bool {
	if len(m.children) == 0 {
		return true
	}

	for _, child := range m.children {
		if !child.GetLifecyclePhase().IsStopped() {
			return false
		}
	}

	return true
}

// buildChildInfo creates a ChildInfo struct from a child supervisor.
func (m *ChildrenManager) buildChildInfo(name string, child SupervisorInterface) config.ChildInfo {
	stateName, stateReason := child.GetCurrentStateNameAndReason()
	phase := child.GetLifecyclePhase()

	return config.ChildInfo{
		Name:          name,
		WorkerType:    child.GetWorkerType(),
		StateName:     stateName,
		StateReason:   stateReason,
		IsHealthy:     phase.IsHealthy(), // Only PhaseRunningHealthy is healthy
		ErrorMsg:      "",
		HierarchyPath: child.GetHierarchyPath(),
		IsStale:       child.IsObservationStale(),
		IsCircuitOpen: child.IsCircuitOpen(),
	}
}

// Compile-time check that ChildrenManager implements ChildrenView.
var _ config.ChildrenView = (*ChildrenManager)(nil)
