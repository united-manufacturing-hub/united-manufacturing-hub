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

// Package snapshot provides the dependencies interface for transport actions.
// Action packages import this interface to avoid import cycles with the parent transport package.
package snapshot

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps/retry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
)

// TransportDependencies is the dependencies interface for transport actions (avoids import cycles).
type TransportDependencies interface {
	deps.Dependencies
	MetricsRecorder() *deps.MetricsRecorder

	// Transport management
	GetTransport() transport.Transport
	SetTransport(t transport.Transport)

	// JWT token management
	SetJWT(token string, expiry time.Time)
	GetJWTToken() string
	GetJWTExpiry() time.Time

	// Error tracking for intelligent backoff
	RecordError()
	RecordSuccess()
	RecordTypedError(errType httpTransport.ErrorType, retryAfter time.Duration)
	GetConsecutiveErrors() int
	GetLastErrorType() httpTransport.ErrorType
	GetLastRetryAfter() time.Duration

	// Auth attempt tracking
	SetLastAuthAttemptAt(t time.Time)
	GetLastAuthAttemptAt() time.Time

	// Instance identity from backend
	SetAuthenticatedUUID(uuid string)
	GetAuthenticatedUUID() string

	// Reset generation for parent-level transport reset signaling to children
	GetResetGeneration() uint64
	IncrementResetGeneration()

	// RetryTracker for backoff and modulo-trigger breaking after transport reset
	RetryTracker() retry.Tracker
}
