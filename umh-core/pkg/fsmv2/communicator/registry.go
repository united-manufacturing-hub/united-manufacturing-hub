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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/communicator/transport"
	"go.uber.org/zap"
)

// CommunicatorRegistry provides access to tools needed by communicator worker actions.
// It extends BaseRegistry with communicator-specific tools (transport).
type CommunicatorRegistry struct {
	*fsmv2.BaseRegistry
	transport transport.Transport
}

// NewCommunicatorRegistry creates a new registry for the communicator worker.
func NewCommunicatorRegistry(transport transport.Transport, logger *zap.SugaredLogger) *CommunicatorRegistry {
	return &CommunicatorRegistry{
		BaseRegistry: fsmv2.NewBaseRegistry(logger),
		transport:    transport,
	}
}

// GetTransport returns the transport for HTTP communication.
func (r *CommunicatorRegistry) GetTransport() transport.Transport {
	return r.transport
}
