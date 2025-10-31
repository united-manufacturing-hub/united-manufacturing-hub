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

import "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"

type TryingToAuthenticateState struct {
	Worker *CommunicatorWorker
}

func (s *TryingToAuthenticateState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
	desired := snapshot.Desired.(*CommunicatorDesiredState)
	observed := snapshot.Observed.(*CommunicatorObservedState)

	if desired.ShutdownRequested() {
		return &StoppedState{Worker: s.Worker}, fsmv2.SignalNone, nil
	}

	if observed.IsAuthenticated() && !observed.IsTokenExpired() {
		return &SyncingState{Worker: s.Worker}, fsmv2.SignalNone, nil
	}

	// Create AuthenticateAction with proper dependencies
	authenticateAction := NewAuthenticateAction(
		s.Worker.relayURL,
		s.Worker.instanceUUID,
		s.Worker.authToken,
		s.Worker.observedState,
	)
	return s, fsmv2.SignalNone, authenticateAction
}

func (s *TryingToAuthenticateState) String() string {
	return "TryingToAuthenticate"
}

func (s *TryingToAuthenticateState) Reason() string {
	return "Attempting to authenticate with relay server"
}
