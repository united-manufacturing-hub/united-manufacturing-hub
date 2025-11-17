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

package state

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-child/action"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-child/snapshot"
)

// TryingToConnectState represents the state where the worker is attempting to establish a connection
type TryingToConnectState struct {
	BaseChildState
}

func NewTryingToConnectState() *TryingToConnectState {
	return &TryingToConnectState{}
}

func (s *TryingToConnectState) Next(snapAny any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
	// Type-assert once at entry point for type safety
	snap := snapAny.(snapshot.ChildSnapshot)
	if snap.Desired.IsShutdownRequested() {
		return NewTryingToStopState(), fsmv2.SignalNone, nil
	}

	return s, fsmv2.SignalNone, action.NewConnectAction()
}

func (s *TryingToConnectState) String() string {
	return "TryingToConnect"
}

func (s *TryingToConnectState) Reason() string {
	return "Attempting to establish connection"
}
