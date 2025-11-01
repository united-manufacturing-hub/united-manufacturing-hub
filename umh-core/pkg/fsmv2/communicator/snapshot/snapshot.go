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

package snapshot

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/communicator/transport"
)

type CommunicatorSnapshot struct {
	Identity fsmv2.Identity
	Observed CommunicatorObservedState
	Desired  CommunicatorDesiredState
}

type CommunicatorDesiredState struct {
	// Boilerplate
	shutdownRequested bool

	// Authentication
	InstanceUUID string
	AuthToken    string
	RelayURL     string

	// Messages
	MessagesToBeSent []transport.UMHMessage
}

func (s *CommunicatorDesiredState) ShutdownRequested() bool {
	return s.shutdownRequested
}

type CommunicatorObservedState struct {
	CollectedAt time.Time

	// DesiredState
	CommunicatorDesiredState

	// Authentication
	Authenticated bool
	JWTToken      string
	JWTExpiry     time.Time

	// Inbound Messages
	MessagesReceived []transport.UMHMessage
}

func (o CommunicatorObservedState) IsTokenExpired() bool {
	return false // TODO
}
