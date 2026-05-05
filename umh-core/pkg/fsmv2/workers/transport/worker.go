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

// Package transport authenticates with the relay server and orchestrates
// PushWorker/PullWorker children for bidirectional message exchange.
package transport

import (
	"context"
	"errors"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
)

// Compile-time interface check: TransportWorker implements fsmv2.Worker.
var _ fsmv2.Worker = (*TransportWorker)(nil)

// TransportWorker implements the FSM Worker interface for HTTP transport.
// It manages authentication and coordinates PushWorker/PullWorker children
// for bidirectional message exchange with the backend relay server.
type TransportWorker struct {
	deps *TransportDependencies
	fsmv2.WorkerBase[TransportConfig, TransportStatus, *TransportDependencies]
}

// NewTransportWorker creates a new Transport worker.
// Returns an error if required dependencies are missing.
//
// Publishes its TransportDependencies via register.SetDeps[*TransportDependencies]
// keyed by workerType so push and pull child workers can build their per-instance
// PushDependencies/PullDependencies via register.SetDepsBuilder callbacks defined
// in their own init() functions. This avoids the import cycle that would arise
// from transport importing push/pull (push/pull already import transport for the
// TransportDependencies type and ChildFailureRateConfig).
func NewTransportWorker(
	identity deps.Identity,
	logger deps.FSMLogger,
	stateReader deps.StateReader,
) (fsmv2.Worker, error) {
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	if identity.WorkerType == "" {
		identity.WorkerType = workerType
	}

	w := &TransportWorker{}
	w.InitBase(identity, logger, stateReader)

	w.deps = NewTransportDependencies(nil, logger, stateReader, identity)
	if w.deps == nil {
		return nil, errors.New("NewTransportDependencies returned nil")
	}

	w.BindDeps(w.deps)

	register.SetDeps[*TransportDependencies](workerType, w.deps)

	return w, nil
}

// CollectObservedState returns the current observed state of the transport worker.
// Returns NewObservation — the collector handles CollectedAt, framework metrics,
// action history, and metric accumulation automatically after COS returns.
func (w *TransportWorker) CollectObservedState(ctx context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	failedToken, failedRelay, failedUUID := w.deps.GetFailedAuthConfig()

	return fsmv2.NewObservation(TransportStatus{
		JWTToken:          w.deps.GetJWTToken(),
		JWTExpiry:         w.deps.GetJWTExpiry(),
		AuthenticatedUUID: w.deps.GetAuthenticatedUUID(),
		ConsecutiveErrors: w.deps.GetConsecutiveErrors(),
		LastErrorType:     w.deps.GetLastErrorType(),
		LastAuthAttemptAt: w.deps.GetLastAuthAttemptAt(),
		LastRetryAfter:    w.deps.GetLastRetryAfter(),
		FailedAuthConfig: FailedAuthConfig{
			AuthToken:    failedToken,
			RelayURL:     failedRelay,
			InstanceUUID: failedUUID,
		},
	}), nil
}

const workerType = "transport"

// init registers the transport worker via the generic register.Worker helper
// with typed TDeps = *TransportDependencies. Push/pull children read the
// published deps via register.GetDeps in their SetDepsBuilder callbacks.
func init() {
	register.Worker[TransportConfig, TransportStatus, *TransportDependencies](workerType, NewTransportWorker)
}
