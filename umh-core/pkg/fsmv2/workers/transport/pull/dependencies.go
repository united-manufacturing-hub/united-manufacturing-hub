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

package pull

import (
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	communicator_transport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
	transport_pkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/pull/snapshot"
)

const maxPendingMessages = 1000

var _ snapshot.PullDependencies = (*PullDependencies)(nil)

type PullDependencies struct {
	*deps.BaseDependencies
	parentDeps              *transport_pkg.TransportDependencies
	pendingMessages         []*communicator_transport.UMHMessage
	pendingMu               sync.Mutex
	lastSeenResetGeneration uint64
	backpressured           bool
	backpressureMu          sync.RWMutex
}

func NewPullDependencies(parentDeps *transport_pkg.TransportDependencies, identity deps.Identity, logger *zap.SugaredLogger, stateReader deps.StateReader) (*PullDependencies, error) {
	if parentDeps == nil {
		return nil, errors.New("parentDeps must not be nil")
	}

	return &PullDependencies{
		BaseDependencies: deps.NewBaseDependencies(logger, stateReader, identity),
		parentDeps:       parentDeps,
	}, nil
}

func (d *PullDependencies) GetInboundChan() chan<- *communicator_transport.UMHMessage {
	return d.parentDeps.GetInboundChan()
}

func (d *PullDependencies) GetInboundChanStats() (capacity int, length int) {
	return d.parentDeps.GetInboundChanStats()
}

func (d *PullDependencies) GetTransport() communicator_transport.Transport {
	return d.parentDeps.GetTransport()
}

func (d *PullDependencies) GetJWTToken() string {
	return d.parentDeps.GetJWTToken()
}

func (d *PullDependencies) RecordTypedError(errType httpTransport.ErrorType, retryAfter time.Duration) {
	d.parentDeps.RecordTypedError(errType, retryAfter)
}

func (d *PullDependencies) RecordSuccess() {
	d.parentDeps.RecordSuccess()
}

func (d *PullDependencies) RecordError() {
	d.parentDeps.RecordError()
}

func (d *PullDependencies) GetConsecutiveErrors() int {
	return d.parentDeps.GetConsecutiveErrors()
}

func (d *PullDependencies) GetLastErrorType() httpTransport.ErrorType {
	return d.parentDeps.GetLastErrorType()
}

func (d *PullDependencies) StorePendingMessages(msgs []*communicator_transport.UMHMessage) {
	d.pendingMu.Lock()
	defer d.pendingMu.Unlock()

	d.pendingMessages = append(d.pendingMessages, msgs...)
	if len(d.pendingMessages) > maxPendingMessages {
		dropped := len(d.pendingMessages) - maxPendingMessages
		d.pendingMessages = d.pendingMessages[len(d.pendingMessages)-maxPendingMessages:]
		d.BaseDependencies.GetLogger().Warnw("pending_buffer_overflow",
			"dropped", dropped, "cap", maxPendingMessages)
	}
}

func (d *PullDependencies) DrainPendingMessages() []*communicator_transport.UMHMessage {
	d.pendingMu.Lock()
	defer d.pendingMu.Unlock()

	msgs := d.pendingMessages
	d.pendingMessages = nil

	return msgs
}

func (d *PullDependencies) PendingMessageCount() int {
	d.pendingMu.Lock()
	defer d.pendingMu.Unlock()

	return len(d.pendingMessages)
}

func (d *PullDependencies) IsBackpressured() bool {
	d.backpressureMu.RLock()
	defer d.backpressureMu.RUnlock()

	return d.backpressured
}

func (d *PullDependencies) SetBackpressured(v bool) {
	d.backpressureMu.Lock()
	defer d.backpressureMu.Unlock()

	d.backpressured = v
}

func (d *PullDependencies) IsTokenValid() bool {
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

func (d *PullDependencies) GetLastRetryAfter() time.Duration {
	return d.parentDeps.GetLastRetryAfter()
}

func (d *PullDependencies) GetDegradedEnteredAt() time.Time {
	return d.parentDeps.GetDegradedEnteredAt()
}

func (d *PullDependencies) GetLastErrorAt() time.Time {
	return d.parentDeps.GetLastErrorAt()
}

func (d *PullDependencies) GetResetGeneration() uint64 {
	return d.parentDeps.GetResetGeneration()
}

func (d *PullDependencies) CheckAndClearOnReset() bool {
	currentGen := d.parentDeps.GetResetGeneration()

	d.pendingMu.Lock()
	defer d.pendingMu.Unlock()

	if currentGen != d.lastSeenResetGeneration {
		d.pendingMessages = nil
		d.lastSeenResetGeneration = currentGen

		d.backpressureMu.Lock()
		d.backpressured = false
		d.backpressureMu.Unlock()

		return true
	}

	return false
}
