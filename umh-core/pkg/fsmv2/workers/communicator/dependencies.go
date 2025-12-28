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

package communicator

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
	"go.uber.org/zap"
)

// CommunicatorDependencies provides access to tools needed by communicator worker actions.
// It extends BaseDependencies with communicator-specific tools (transport).
type CommunicatorDependencies struct {
	*fsmv2.BaseDependencies
	transport transport.Transport
}

// NewCommunicatorDependencies creates a new dependencies for the communicator worker.
func NewCommunicatorDependencies(transport transport.Transport, logger *zap.SugaredLogger, stateReader fsmv2.StateReader, identity fsmv2.Identity) *CommunicatorDependencies {
	return &CommunicatorDependencies{
		BaseDependencies: fsmv2.NewBaseDependencies(logger, stateReader, identity),
		transport:        transport,
	}
}

// GetTransport returns the transport for HTTP communication.
func (d *CommunicatorDependencies) GetTransport() transport.Transport {
	return d.transport
}
