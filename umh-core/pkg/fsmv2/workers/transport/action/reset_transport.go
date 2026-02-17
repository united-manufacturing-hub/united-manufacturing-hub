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
	"errors"

	depspkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/snapshot"
)

const ResetTransportActionName = "reset_transport"

// ResetTransportAction resets the HTTP transport to establish fresh connections.
// Triggered by DegradedState when ShouldResetTransport threshold is reached.
// Resolves stale TCP connections, DNS caching, corrupted connection pool, or TLS issues.
// Idempotent: creates fresh HTTP client while preserving JWT tokens.
// After reset, increments resetGeneration to signal children to clear pending buffers.
type ResetTransportAction struct{}

func NewResetTransportAction() *ResetTransportAction {
	return &ResetTransportAction{}
}

func (a *ResetTransportAction) Name() string {
	return ResetTransportActionName
}

func (a *ResetTransportAction) Execute(ctx context.Context, depsAny any) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	deps, ok := depsAny.(snapshot.TransportDependencies)
	if !ok {
		return errors.New("invalid dependencies type: expected TransportDependencies")
	}

	transport := deps.GetTransport()
	transport.Reset()
	deps.GetLogger().Info("transport_reset_completed", depspkg.String("reason", "degraded_state_threshold"))

	deps.IncrementResetGeneration()

	// Advance retry counter to break the modulo-N trigger condition.
	// Without this, ShouldResetTransport keeps returning true (e.g. 5%5==0),
	// causing an infinite reset loop. After Attempt(), counter becomes 6,
	// and ShouldResetTransport returns false (6%5!=0).
	deps.RetryTracker().Attempt()

	return nil
}
