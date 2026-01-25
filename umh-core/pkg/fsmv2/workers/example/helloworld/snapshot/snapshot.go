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

// Package snapshot defines the state types for the helloworld worker.
//
// NAMING CONVENTION: Types must be named {FolderName}ObservedState and
// {FolderName}DesiredState where FolderName is the parent directory name
// with only the first letter capitalized.
//
// Folder: helloworld -> Type prefix: Helloworld
// So we get: HelloworldObservedState, HelloworldDesiredState
package snapshot

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// HelloworldDependencies defines the interface for accessing worker dependencies.
// Defined here to avoid import cycles between snapshot and the main package.
type HelloworldDependencies interface {
	deps.Dependencies
	SetHelloSaid(said bool)
	HasSaidHello() bool
}

// HelloworldDesiredState represents the target configuration for the worker.
// Embed BaseDesiredState to get standard fields like ShutdownRequested.
type HelloworldDesiredState struct {
	config.BaseDesiredState
}

// HelloworldObservedState represents the current observed state of the worker.
// This is collected by CollectObservedState() and persisted to the triangular store.
//
// REQUIRED: Embed deps.MetricsEmbedder to enable metrics collection.
// REQUIRED: Embed HelloworldDesiredState with json:",inline" for architecture tests.
type HelloworldObservedState struct {
	// CollectedAt is when this observation was taken
	CollectedAt time.Time `json:"collected_at"`

	// State is the current FSM state name (set by supervisor)
	State string `json:"state"`

	// HelloSaid tracks whether the SayHelloAction has been executed
	HelloSaid bool `json:"hello_said"`

	// LastActionResults contains action history (managed by supervisor)
	LastActionResults []deps.ActionResult `json:"last_action_results,omitempty"`

	// HelloworldDesiredState embedded for state consistency
	// REQUIRED: Architecture tests verify this pattern
	HelloworldDesiredState `json:",inline"`

	// MetricsEmbedder provides framework and worker metrics
	// REQUIRED: Without this, metrics won't be collected
	deps.MetricsEmbedder `json:",inline"`
}

// GetTimestamp returns when this observation was collected.
// Required by fsmv2.ObservedState interface.
func (o HelloworldObservedState) GetTimestamp() time.Time {
	return o.CollectedAt
}

// GetObservedDesiredState returns the desired state embedded in this observation.
// Required by fsmv2.ObservedState interface for shutdown handling.
func (o HelloworldObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &o.HelloworldDesiredState
}

// SetState sets the FSM state name on this observed state.
// Required by fsmv2.ObservedState interface.
func (o HelloworldObservedState) SetState(s string) fsmv2.ObservedState {
	o.State = s

	return o
}

// SetShutdownRequested sets the shutdown requested flag.
// Required by fsmv2.ObservedState interface.
func (o HelloworldObservedState) SetShutdownRequested(v bool) fsmv2.ObservedState {
	o.ShutdownRequested = v

	return o
}

// SetParentMappedState is a no-op for root workers (no parent).
// Required by fsmv2.ObservedState interface.
func (o HelloworldObservedState) SetParentMappedState(_ string) fsmv2.ObservedState {
	return o
}
