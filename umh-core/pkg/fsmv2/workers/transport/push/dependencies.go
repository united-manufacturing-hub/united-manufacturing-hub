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

package push

import (
	"errors"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	communicator_transport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
	transport_pkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push/snapshot"
)

const maxPendingMessages = 1000

var _ snapshot.PushDependencies = (*PushDependencies)(nil)

type PushDependencies struct {
	*deps.BaseDependencies
	parentDeps              *transport_pkg.TransportDependencies
	pendingMessages         []*communicator_transport.UMHMessage
	pendingMu               sync.Mutex
	lastSeenResetGeneration uint64
}

func NewPushDependencies(parentDeps *transport_pkg.TransportDependencies, identity deps.Identity, logger deps.FSMLogger, stateReader deps.StateReader) (*PushDependencies, error) {
	if parentDeps == nil {
		return nil, errors.New("parentDeps must not be nil")
	}

	return &PushDependencies{
		BaseDependencies: deps.NewBaseDependencies(logger, stateReader, identity),
		parentDeps:       parentDeps,
	}, nil
}

func (d *PushDependencies) GetOutboundChan() <-chan *communicator_transport.UMHMessage {
	return d.parentDeps.GetOutboundChan()
}

func (d *PushDependencies) GetTransport() communicator_transport.Transport {
	return d.parentDeps.GetTransport()
}

func (d *PushDependencies) GetJWTToken() string {
	return d.parentDeps.GetJWTToken()
}

func (d *PushDependencies) RecordTypedError(errType httpTransport.ErrorType, retryAfter time.Duration) {
	d.parentDeps.RecordTypedError(errType, retryAfter)
}

func (d *PushDependencies) RecordSuccess() {
	d.parentDeps.RecordSuccess()
}

func (d *PushDependencies) RecordError() {
	d.parentDeps.RecordError()
}

func (d *PushDependencies) GetConsecutiveErrors() int {
	return d.parentDeps.GetConsecutiveErrors()
}

func (d *PushDependencies) GetLastErrorType() httpTransport.ErrorType {
	return d.parentDeps.GetLastErrorType()
}

func (d *PushDependencies) StorePendingMessages(msgs []*communicator_transport.UMHMessage) {
	d.pendingMu.Lock()
	defer d.pendingMu.Unlock()

	d.pendingMessages = append(d.pendingMessages, msgs...)
	if len(d.pendingMessages) > maxPendingMessages {
		dropped := len(d.pendingMessages) - maxPendingMessages
		d.pendingMessages = d.pendingMessages[len(d.pendingMessages)-maxPendingMessages:]
		d.BaseDependencies.GetLogger().SentryWarn(deps.FeatureCommunicator, d.GetHierarchyPath(), "pending_buffer_overflow",
			deps.Int("dropped", dropped), deps.Int("cap", maxPendingMessages))
		d.MetricsRecorder().IncrementCounter(deps.CounterMessagesDropped, int64(dropped))
	}
}

func (d *PushDependencies) DrainPendingMessages() []*communicator_transport.UMHMessage {
	d.pendingMu.Lock()
	defer d.pendingMu.Unlock()

	msgs := d.pendingMessages
	d.pendingMessages = nil

	return msgs
}

func (d *PushDependencies) PendingMessageCount() int {
	d.pendingMu.Lock()
	defer d.pendingMu.Unlock()

	return len(d.pendingMessages)
}

func (d *PushDependencies) IsTokenValid() bool {
	token := d.parentDeps.GetJWTToken()
	if token == "" {
		return false
	}

	expiry := d.parentDeps.GetJWTExpiry()
	if expiry.IsZero() {
		return false
	}

	const safetyBuffer = 1 * time.Minute

	return !time.Now().Add(safetyBuffer).After(expiry)
}

func (d *PushDependencies) GetLastRetryAfter() time.Duration {
	return d.parentDeps.GetLastRetryAfter()
}

func (d *PushDependencies) GetDegradedEnteredAt() time.Time {
	return d.parentDeps.GetDegradedEnteredAt()
}

func (d *PushDependencies) GetLastErrorAt() time.Time {
	return d.parentDeps.GetLastErrorAt()
}

func (d *PushDependencies) GetResetGeneration() uint64 {
	return d.parentDeps.GetResetGeneration()
}

// CheckAndClearOnReset checks if parent has done a transport reset.
// If resetGeneration changed, clears all pending messages and returns true.
func (d *PushDependencies) CheckAndClearOnReset() bool {
	currentGen := d.parentDeps.GetResetGeneration()

	d.pendingMu.Lock()
	defer d.pendingMu.Unlock()

	if currentGen != d.lastSeenResetGeneration {
		d.pendingMessages = nil
		d.lastSeenResetGeneration = currentGen

		return true
	}

	return false
}
