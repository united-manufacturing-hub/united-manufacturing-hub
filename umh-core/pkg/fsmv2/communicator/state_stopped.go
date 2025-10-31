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

type StoppedState struct {
	Worker *CommunicatorWorker
}

func (s *StoppedState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
	desired := snapshot.Desired.(*CommunicatorDesiredState)

	if desired.ShutdownRequested() {
		return s, fsmv2.SignalNeedsRemoval, nil
	}

	return &TryingToAuthenticateState{Worker: s.Worker}, fsmv2.SignalNone, nil
}

func (s *StoppedState) String() string {
	return "Stopped"
}

func (s *StoppedState) Reason() string {
	return "Communicator is stopped"
}
