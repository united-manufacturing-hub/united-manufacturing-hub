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
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	transport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
)

// RenderChildren returns the ChildSpec set for the communicator worker.
//
// Communicator manages a single transport child whose configuration is derived
// directly from the communicator's own config fields. All auth parameters
// (RelayURL, InstanceUUID, AuthToken, Timeout) flow through unchanged.
//
// The enabled parameter controls whether the transport child should be active
// (true = alive trajectory) or resident-disabled (false = stop trajectory,
// pause-not-delete). The transport child is never despawned — it holds
// connection buffers and retry state.
func RenderChildren(cfg CommunicatorConfig, enabled bool) ([]config.ChildSpec, error) {
	childCfg := transport.TransportUserSpec{
		BaseUserSpec: cfg.BaseUserSpec,
		RelayURL:     cfg.RelayURL,
		InstanceUUID: cfg.InstanceUUID,
		AuthToken:    cfg.AuthToken,
		Timeout:      cfg.Timeout,
	}

	spec, err := config.NewChildSpec("transport", "transport", childCfg, enabled)
	if err != nil {
		return nil, fmt.Errorf("communicator RenderChildren: transport: %w", err)
	}

	return []config.ChildSpec{spec}, nil
}
