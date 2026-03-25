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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
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
// JSON tags must match the old CommunicatorObservedState serialization exactly
// for CSE backward compatibility. Fields without explicit tags in the old code
// used Go-default serialization (capitalized field name).
type CommunicatorStatus struct {
	JWTExpiry         time.Time
	DegradedEnteredAt time.Time `json:"degradedEnteredAt,omitempty"`
	LastAuthAttemptAt time.Time `json:"lastAuthAttemptAt,omitempty"`
	LastErrorAt       time.Time `json:"lastErrorAt,omitempty"`

	JWTToken          string
	AuthenticatedUUID string `json:"authenticatedUUID,omitempty"`

	MessagesReceived []transport.UMHMessage

	ConsecutiveErrors int

	LastErrorType  httpTransport.ErrorType `json:"lastErrorType,omitempty"`
	LastRetryAfter time.Duration           `json:"lastRetryAfter,omitempty"`

	IsBackpressured bool `json:"isBackpressured,omitempty"`

	// Deprecated: Authentication is now handled by TransportWorker (ENG-4264).
	Authenticated bool
}

// IsTokenExpired returns true if the JWT token is expired or will expire within 10 minutes.
func (s CommunicatorStatus) IsTokenExpired() bool {
	if s.JWTExpiry.IsZero() {
		return false
	}

	const refreshBuffer = 10 * time.Minute

	return time.Now().Add(refreshBuffer).After(s.JWTExpiry)
}

// GetConsecutiveErrors returns the number of consecutive errors.
func (s CommunicatorStatus) GetConsecutiveErrors() int {
	return s.ConsecutiveErrors
}
