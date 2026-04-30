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

package snapshot

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
)

// CommunicatorDependencies is the dependencies interface for communicator actions (avoids import cycles).
type CommunicatorDependencies interface {
	deps.Dependencies
	GetTransport() transport.Transport
	MetricsRecorder() *deps.MetricsRecorder
}

type CommunicatorSnapshot struct {
	Identity deps.Identity
	Desired  CommunicatorDesiredState
	Observed CommunicatorObservedState
}

var _ fsmv2.DesiredState = (*CommunicatorDesiredState)(nil)
var _ config.ChildSpecProvider = (*CommunicatorDesiredState)(nil)

// CommunicatorDesiredState represents the target configuration for the communicator.
type CommunicatorDesiredState struct {
	config.BaseDesiredState // Provides ShutdownRequested + IsShutdownRequested() + SetShutdownRequested()

	// Authentication fields passed through to TransportWorker child via UserSpec.
	InstanceUUID string `json:"instanceUUID"`
	AuthToken    string `json:"authToken"`
	RelayURL     string `json:"relayURL"`

	ChildrenSpecs []config.ChildSpec `json:"childrenSpecs,omitempty"`

	// Messages
	MessagesToBeSent []transport.UMHMessage `json:"messagesToBeSent,omitempty"`
	Timeout          time.Duration          `json:"timeout"`
}

// GetChildrenSpecs returns the children specifications for the communicator worker.
func (d *CommunicatorDesiredState) GetChildrenSpecs() []config.ChildSpec {
	return d.ChildrenSpecs
}

// GetState returns the desired lifecycle state ("running" or "stopped").
func (d *CommunicatorDesiredState) GetState() string {
	return d.State
}

// CommunicatorObservedState represents the current state of the communicator.
type CommunicatorObservedState struct {
	CollectedAt       time.Time `json:"collectedAt"`
	DegradedEnteredAt time.Time `json:"degradedEnteredAt,omitempty"` // When errors started (zero = not degraded)

	State string `json:"state"` // Observed lifecycle state (e.g., "running_connected")

	// DesiredState
	CommunicatorDesiredState `json:",inline"`

	deps.MetricsEmbedder `json:",inline"`

	// Error tracking for health monitoring
	ConsecutiveErrors int `json:"consecutiveErrors"`

	ChildrenHealthy   int `json:"children_healthy"`
	ChildrenUnhealthy int `json:"children_unhealthy"`
}

// IsSyncHealthy returns true when at least one child is healthy and no children
// are unhealthy. Returns false when no children exist, which is a transient
// state during startup that resolves within 1-2 supervisor ticks.
func (o CommunicatorObservedState) IsSyncHealthy() bool {
	return o.ChildrenHealthy > 0 && o.ChildrenUnhealthy == 0
}

func (o CommunicatorObservedState) GetConsecutiveErrors() int {
	return o.ConsecutiveErrors
}

func (o CommunicatorObservedState) GetTimestamp() time.Time {
	return o.CollectedAt
}

// SetState sets the FSM state name on this observed state.
func (o CommunicatorObservedState) SetState(s string) fsmv2.ObservedState {
	o.State = s

	return o
}

// SetShutdownRequested sets the shutdown requested status on this observed state.
func (o CommunicatorObservedState) SetShutdownRequested(v bool) fsmv2.ObservedState {
	o.ShutdownRequested = v

	return o
}

// SetChildrenCounts sets the children health counts on this observed state.
func (o CommunicatorObservedState) SetChildrenCounts(healthy, unhealthy int) fsmv2.ObservedState {
	o.ChildrenHealthy = healthy
	o.ChildrenUnhealthy = unhealthy

	return o
}
