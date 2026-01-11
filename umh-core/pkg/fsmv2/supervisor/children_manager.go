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
	"strings"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// ChildrenManager provides a read-only view of children for parent workers.
// It implements the config.ChildrenView interface.
//
// This type wraps the supervisor's internal children map and provides
// a snapshot-based view that doesn't allow modifications.
type ChildrenManager struct {
	children map[string]SupervisorInterface
}

// NewChildrenManager creates a new ChildrenManager wrapping the given children map.
// The children map should be obtained from the supervisor while holding the appropriate lock.
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
// Implements config.ChildrenView.
//
// Health determination:
//   - Healthy: state contains "Running" or "Connected"
//   - Unhealthy: non-empty state that isn't healthy and isn't "Stopped"
//   - Ignored: empty state, "unknown", or "Stopped" states
func (m *ChildrenManager) Counts() (healthy, unhealthy int) {
	for _, child := range m.children {
		stateName := child.GetCurrentStateName()

		if isHealthyState(stateName) {
			healthy++
		} else if isUnhealthyState(stateName) {
			unhealthy++
		}
		// Stopped and unknown states don't count as either
	}
	return healthy, unhealthy
}

// AllHealthy returns true if all children are healthy (or there are no children).
// Implements config.ChildrenView.
func (m *ChildrenManager) AllHealthy() bool {
	if len(m.children) == 0 {
		return true
	}

	for _, child := range m.children {
		stateName := child.GetCurrentStateName()
		if !isHealthyState(stateName) {
			return false
		}
	}
	return true
}

// AllStopped returns true if all children are in the Stopped state (or there are no children).
// Implements config.ChildrenView.
func (m *ChildrenManager) AllStopped() bool {
	if len(m.children) == 0 {
		return true
	}

	for _, child := range m.children {
		stateName := child.GetCurrentStateName()
		if !strings.Contains(stateName, "Stopped") {
			return false
		}
	}
	return true
}

// buildChildInfo creates a ChildInfo struct from a child supervisor.
func (m *ChildrenManager) buildChildInfo(name string, child SupervisorInterface) config.ChildInfo {
	stateName := child.GetCurrentStateName()

	return config.ChildInfo{
		Name:          name,
		WorkerType:    child.GetWorkerType(),
		StateName:     stateName,
		StateReason:   "", // TODO: expose state reason through SupervisorInterface if needed
		IsHealthy:     isHealthyState(stateName),
		ErrorMsg:      "", // TODO: expose error info through SupervisorInterface if needed
		HierarchyPath: child.GetHierarchyPath(),
	}
}

// isHealthyState returns true if the state indicates a healthy child.
func isHealthyState(stateName string) bool {
	return strings.Contains(stateName, "Running") || strings.Contains(stateName, "Connected")
}

// isUnhealthyState returns true if the state indicates an unhealthy child.
// Unhealthy means: has a known state that's not healthy and not stopped.
func isUnhealthyState(stateName string) bool {
	if stateName == "" || stateName == "unknown" {
		return false
	}
	if strings.Contains(stateName, "Stopped") {
		return false
	}
	if isHealthyState(stateName) {
		return false
	}
	return true
}

// Compile-time check that ChildrenManager implements ChildrenView.
var _ config.ChildrenView = (*ChildrenManager)(nil)
