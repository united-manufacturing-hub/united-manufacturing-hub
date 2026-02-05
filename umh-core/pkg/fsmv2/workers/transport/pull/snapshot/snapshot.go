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

type PullDependencies interface {
	deps.Dependencies
	GetInboundChan() chan<- *transport.UMHMessage
	GetInboundChanStats() (capacity int, length int)
	GetTransport() transport.Transport
	GetJWTToken() string
	RecordTypedError(errType httpTransport.ErrorType, retryAfter time.Duration)
	RecordSuccess()
	RecordError()
	GetConsecutiveErrors() int
	GetLastErrorType() httpTransport.ErrorType
	MetricsRecorder() *deps.MetricsRecorder

	StorePendingMessages(msgs []*transport.UMHMessage)
	DrainPendingMessages() []*transport.UMHMessage
	PendingMessageCount() int

	IsBackpressured() bool
	SetBackpressured(v bool)

	IsTokenValid() bool

	GetResetGeneration() uint64
	CheckAndClearOnReset() bool

	GetLastRetryAfter() time.Duration
	GetDegradedEnteredAt() time.Time
	GetLastErrorAt() time.Time
}

type PullDesiredState struct {
	ParentMappedState string `json:"parent_mapped_state"`
	config.BaseDesiredState
}

func (s *PullDesiredState) ShouldBeRunning() bool {
	if s.ShutdownRequested {
		return false
	}

	return s.ParentMappedState == config.DesiredStateRunning
}

type PullObservedState struct {
	CollectedAt       time.Time `json:"collected_at"`
	DegradedEnteredAt time.Time `json:"degraded_entered_at,omitempty"`
	LastErrorAt       time.Time `json:"last_error_at,omitempty"`

	PullDesiredState `json:",inline"`

	State string `json:"state"`

	LastActionResults []deps.ActionResult `json:"last_action_results,omitempty"`

	deps.MetricsEmbedder `json:",inline"`

	LastRetryAfter time.Duration `json:"last_retry_after,omitempty"`

	LastErrorType       httpTransport.ErrorType `json:"last_error_type"`
	ConsecutiveErrors   int                     `json:"consecutive_errors"`
	PendingMessageCount int                     `json:"pending_message_count"`

	HasTransport    bool `json:"has_transport"`
	HasValidToken   bool `json:"has_valid_token"`
	IsBackpressured bool `json:"is_backpressured"`
}

func (o PullObservedState) GetTimestamp() time.Time {
	return o.CollectedAt
}

func (o PullObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &o.PullDesiredState
}

func (o PullObservedState) SetState(s string) fsmv2.ObservedState {
	o.State = s

	return o
}

func (o PullObservedState) SetShutdownRequested(v bool) fsmv2.ObservedState {
	o.ShutdownRequested = v

	return o
}

func (o PullObservedState) SetParentMappedState(state string) fsmv2.ObservedState {
	o.ParentMappedState = state

	return o
}

func (o PullObservedState) IsStopRequired() bool {
	return o.IsShutdownRequested() || !o.ShouldBeRunning()
}
