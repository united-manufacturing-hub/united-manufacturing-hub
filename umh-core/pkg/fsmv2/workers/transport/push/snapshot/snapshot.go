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

// Package snapshot provides the dependencies interface for push actions.
// Action packages import this interface to avoid import cycles with the parent push package.
package snapshot

import (
	"time"

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
