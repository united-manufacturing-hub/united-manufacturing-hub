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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

// PullDependencies abstracts the pull worker's runtime dependencies to avoid import
// cycles between the pull and transport packages.
type PullDependencies interface {
	deps.Dependencies
	GetInboundChan() chan<- *types.UMHMessage
	GetInboundChanStats() (capacity int, length int)
	GetTransport() types.Transport
	RecordTypedError(errType types.ErrorType, retryAfter time.Duration)
	RecordSuccess()
	RecordError()
	GetConsecutiveErrors() int
	GetLastErrorType() types.ErrorType
	MetricsRecorder() *deps.MetricsRecorder

	StorePendingMessages(msgs []*types.UMHMessage)
	DrainPendingMessages() []*types.UMHMessage
	PendingMessageCount() int

	IsBackpressured() bool
	SetBackpressured(v bool)

	GetResetGeneration() uint64
	CheckAndClearOnReset() bool

	GetLastRetryAfter() time.Duration
	GetDegradedEnteredAt() time.Time
	GetLastErrorAt() time.Time
}

// PullDesiredState represents the target configuration for the pull worker.
type PullDesiredState struct {
	AuthSession types.AuthSession `json:"auth_session,omitempty" yaml:"auth_session,omitempty"`
	State       string            `json:"state"                  yaml:"state"`
	config.BaseDesiredState
}

// GetState returns the desired lifecycle state, defaulting to "running" if empty.
// Pull is a leaf child whose lifecycle is gated by the parent transport worker via
// the Disabled bit, not by its own State. The transport worker therefore leaves State
// unset when constructing PullDesiredState, and GetState defaults to running.
func (s *PullDesiredState) GetState() string {
	if s.State == "" {
		return config.DesiredStateRunning
	}

	return s.State
}

// PullStatus holds the runtime observation data for the pull worker.
type PullStatus struct {
	DegradedEnteredAt   time.Time       `json:"degraded_entered_at,omitempty"`
	LastErrorAt         time.Time       `json:"last_error_at,omitempty"`
	LastRetryAfter      time.Duration   `json:"last_retry_after,omitempty"`
	LastErrorType       types.ErrorType `json:"last_error_type"`
	ConsecutiveErrors   int             `json:"consecutive_errors"`
	PendingMessageCount int             `json:"pending_message_count"`
	HasTransport        bool            `json:"has_transport"`
	HasValidToken       bool            `json:"has_valid_token"`
	IsBackpressured     bool            `json:"is_backpressured"`
}
