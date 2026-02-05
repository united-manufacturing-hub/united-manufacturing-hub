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
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	communicator_transport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
	transport_pkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push/snapshot"
	"go.uber.org/zap"
)

var _ snapshot.PushDependencies = (*PushDependencies)(nil)

type PushDependencies struct {
	*deps.BaseDependencies
	parentDeps *transport_pkg.TransportDependencies
}

func NewPushDependencies(parentDeps *transport_pkg.TransportDependencies, identity deps.Identity, logger *zap.SugaredLogger, stateReader deps.StateReader) (*PushDependencies, error) {
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
