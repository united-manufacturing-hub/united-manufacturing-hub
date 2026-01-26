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

package action

import (
	"context"
)

const ResetTransportActionName = "reset_transport"

// ResetTransportAction resets the HTTP transport to establish fresh connections.
// Triggered by DegradedState at multiples of TransportResetThreshold (5, 10, 15...).
// Resolves stale TCP connections, DNS caching, corrupted connection pool, or TLS issues.
// Idempotent: creates fresh HTTP client while preserving JWT tokens.
type ResetTransportAction struct{}

func NewResetTransportAction() *ResetTransportAction {
	return &ResetTransportAction{}
}

func (a *ResetTransportAction) Name() string {
	return ResetTransportActionName
}

// Execute resets the transport. Transport guaranteed non-nil per worker.go C3.
func (a *ResetTransportAction) Execute(ctx context.Context, depsAny any) error {
	// Check for context cancellation before proceeding
	if err := ctx.Err(); err != nil {
		return err
	}

	deps := depsAny.(CommunicatorDependencies)

	transport := deps.GetTransport()
	transport.Reset()
	deps.GetLogger().Infow("transport_reset_completed", "reason", "degraded_state_threshold")

	// FIX: Advance the retry counter to break the modulo-N trigger condition.
	// Without this, ShouldResetTransport(5) keeps returning true (5 % 5 == 0),
	// causing an infinite reset loop. After Attempt(), counter becomes 6,
	// and ShouldResetTransport(6) returns false (6 % 5 != 0).
	// The counter resets to 0 only when SyncAction.Execute() succeeds.
	deps.RetryTracker().Attempt()

	return nil
}
