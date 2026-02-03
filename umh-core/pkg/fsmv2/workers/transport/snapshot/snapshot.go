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
)

// TransportDependencies is the dependencies interface for transport actions (avoids import cycles).
type TransportDependencies interface {
	deps.Dependencies
	MetricsRecorder() *deps.MetricsRecorder
}

// TransportSnapshot represents a point-in-time view of the transport worker state.
type TransportSnapshot struct {
	Desired  *TransportDesiredState
	Identity deps.Identity
	Observed TransportObservedState
}

// Compile-time check that TransportDesiredState implements fsmv2.DesiredState.
var _ fsmv2.DesiredState = (*TransportDesiredState)(nil)

// TransportDesiredState represents the target configuration for the transport worker.
type TransportDesiredState struct {
	config.BaseDesiredState // Provides State, ShutdownRequested + IsShutdownRequested() + SetShutdownRequested()

	InstanceUUID string        `json:"instanceUUID"` // Used by AuthenticateAction for backend authentication
	AuthToken    string        `json:"authToken"`
	RelayURL     string        `json:"relayURL"`
	Timeout      time.Duration `json:"timeout"`
}

// GetState returns the desired lifecycle state ("running" or "stopped").
func (d *TransportDesiredState) GetState() string {
	return d.BaseDesiredState.GetState()
}

// TransportObservedState represents the current state of the transport worker.
type TransportObservedState struct {
	CollectedAt time.Time `json:"collected_at"`

	JWTExpiry time.Time `json:"jwt_expiry,omitempty"`

	State string `json:"state"` // Observed lifecycle state (e.g., "running_healthy")

	JWTToken string `json:"jwt_token,omitempty"`

	// Children contains the observed state of child workers (PushWorker, PullWorker).
	Children map[string]fsmv2.ObservedState `json:"children,omitempty"`

	// DesiredState embedded for state consistency
	TransportDesiredState `json:",inline"`

	deps.MetricsEmbedder `json:",inline"`

	// LastActionResults contains the action history from the last collection cycle (supervisor-managed).
	LastActionResults []deps.ActionResult `json:"last_action_results,omitempty"`

	// TotalMessagesPushed tracks cumulative messages pushed to backend.
	TotalMessagesPushed int64 `json:"total_messages_pushed"`

	// TotalMessagesPulled tracks cumulative messages pulled from backend.
	TotalMessagesPulled int64 `json:"total_messages_pulled"`
}

// GetTimestamp returns when this observed state was collected.
func (o TransportObservedState) GetTimestamp() time.Time {
	return o.CollectedAt
}

// GetObservedDesiredState returns the desired state that is actually deployed.
func (o TransportObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &o.TransportDesiredState
}

// SetState sets the FSM state name on this observed state.
func (o TransportObservedState) SetState(s string) fsmv2.ObservedState {
	o.State = s

	return o
}

// SetShutdownRequested sets the shutdown requested status on this observed state.
func (o TransportObservedState) SetShutdownRequested(v bool) fsmv2.ObservedState {
	o.ShutdownRequested = v

	return o
}

// IsTokenExpired returns true if the JWT token is expired or will expire within 10 minutes.
func (o TransportObservedState) IsTokenExpired() bool {
	if o.JWTExpiry.IsZero() {
		return false
	}

	const refreshBuffer = 10 * time.Minute

	return time.Now().Add(refreshBuffer).After(o.JWTExpiry)
}
