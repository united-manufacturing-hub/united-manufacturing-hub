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
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// CommunicatorConfig holds the user-provided configuration for the communicator worker.
// Embeds BaseUserSpec to support the StateGetter interface, allowing WorkerBase.DeriveDesiredState
// to extract the desired state from the "state" YAML field.
type CommunicatorConfig struct {
	config.BaseUserSpec `yaml:",inline"`

	RelayURL     string        `json:"relayURL"     yaml:"relayURL"`
	InstanceUUID string        `json:"instanceUUID" yaml:"instanceUUID"`
	AuthToken    string        `json:"authToken"    yaml:"authToken"`
	Timeout      time.Duration `json:"timeout"      yaml:"timeout"`
}

// CommunicatorStatus holds the runtime observation data for the communicator worker.
// Deprecated fields (JWTToken, JWTExpiry, Authenticated, AuthenticatedUUID,
// MessagesReceived, ConsecutiveErrors, IsBackpressured, LastErrorType,
// LastRetryAfter, LastErrorAt, LastAuthAttemptAt) were removed in ENG-4265.
// These are now tracked by TransportWorker.
// AuthenticatedUUID is retained because cmd/main.go reads it from CSE to update LoginResponse.
type CommunicatorStatus struct {
	DegradedEnteredAt time.Time `json:"degradedEnteredAt,omitempty"`
}
