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
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
)

// PushDependencies interface to avoid import cycles between push and transport packages.
type PushDependencies interface {
	deps.Dependencies
	GetOutboundChan() <-chan *transport.UMHMessage
	GetTransport() transport.Transport
	GetJWTToken() string
	GetAuthenticatedUUID() string
	RecordTypedError(errType httpTransport.ErrorType, retryAfter time.Duration)
	RecordSuccess()
	RecordTransportSuccess()
	RecordError()
	GetConsecutiveErrors() int
	GetLastErrorType() httpTransport.ErrorType
	MetricsRecorder() *deps.MetricsRecorder

	// Pending buffer for retry on push failure
	StorePendingMessages(msgs []*transport.UMHMessage)
	DrainPendingMessages() []*transport.UMHMessage
	PendingMessageCount() int

	// Token pre-check (1-minute safety buffer)
	IsTokenValid() bool

	// Parent transport reset detection
	GetResetGeneration() uint64
	CheckAndClearOnReset() bool

	// Backoff timing from child's own error tracking
	GetLastRetryAfter() time.Duration
	GetDegradedEnteredAt() time.Time
	GetLastErrorAt() time.Time
}

// PushSnapshot represents a point-in-time view of the push worker state.
type PushSnapshot struct {
	Desired  *PushDesiredState
	Identity deps.Identity
	Observed PushObservedState
}

// PushDesiredState represents the target configuration for the push worker.
type PushDesiredState struct {
	ParentMappedState string `json:"parent_mapped_state"`
	config.BaseDesiredState
}

// ShouldBeRunning returns true if the push worker should be in a running state.
func (s *PushDesiredState) ShouldBeRunning() bool {
	if s.ShutdownRequested {
		return false
	}

	return s.ParentMappedState == config.DesiredStateRunning
}

// PushObservedState represents the current state of the push worker.
type PushObservedState struct {
	CollectedAt       time.Time `json:"collected_at"`
	DegradedEnteredAt time.Time `json:"degraded_entered_at,omitempty"`
	LastErrorAt       time.Time `json:"last_error_at,omitempty"`

	PushDesiredState `json:",inline"`

	State string `json:"state"`

	LastActionResults []deps.ActionResult `json:"last_action_results,omitempty"`

	deps.MetricsEmbedder `json:",inline"`

	LastRetryAfter time.Duration `json:"last_retry_after,omitempty"`

	LastErrorType       httpTransport.ErrorType `json:"last_error_type"`
	ConsecutiveErrors   int                     `json:"consecutive_errors"`
	PendingMessageCount int                     `json:"pending_message_count"`

	HasTransport  bool `json:"has_transport"`
	HasValidToken bool `json:"has_valid_token"`
}

func (o PushObservedState) GetTimestamp() time.Time {
	return o.CollectedAt
}

func (o PushObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &o.PushDesiredState
}

// SetState sets the FSM state name on this observed state.
func (o PushObservedState) SetState(s string) fsmv2.ObservedState {
	o.State = s

	return o
}

// SetShutdownRequested sets the shutdown requested status on this observed state.
func (o PushObservedState) SetShutdownRequested(v bool) fsmv2.ObservedState {
	o.ShutdownRequested = v

	return o
}

// SetParentMappedState sets the parent's mapped state on this observed state.
func (o PushObservedState) SetParentMappedState(state string) fsmv2.ObservedState {
	o.ParentMappedState = state

	return o
}

// IsStopRequired reports whether the push worker needs to stop.
func (o PushObservedState) IsStopRequired() bool {
	return o.IsShutdownRequested() || !o.ShouldBeRunning()
}
