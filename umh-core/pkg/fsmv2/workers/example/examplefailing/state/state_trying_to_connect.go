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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	examplefailing "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplefailing"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplefailing/action"
)

// TryingToConnectState represents the state where the worker is attempting to establish a connection.
type TryingToConnectState struct {
	helpers.StartingBase
}

func (s *TryingToConnectState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[examplefailing.ExamplefailingConfig, examplefailing.ExamplefailingStatus](snapAny)

	if snap.ShouldStop() {
		return fsmv2.Transition(&TryingToStopState{}, fsmv2.SignalNone, nil,
			"stop required: "+snap.StopReason(), nil)
	}

	if snap.Status.RestartAfterFailures > 0 &&
		snap.Status.ConnectAttempts >= snap.Status.RestartAfterFailures {
		return fsmv2.Transition(s, fsmv2.SignalNeedsRestart, nil, "max connection attempts reached, signaling restart", nil)
	}

	if snap.Status.ConnectionHealth == "healthy" {
		return fsmv2.Transition(&ConnectedState{}, fsmv2.SignalNone, nil, "connection established successfully", nil)
	}

	if snap.Status.RecoveryDelayActive {
		return fsmv2.Transition(s, fsmv2.SignalNone, &action.TriggerObservationAction{},
			"waiting for recovery delay (triggering observation)", nil)
	}

	return fsmv2.Transition(s, fsmv2.SignalNone, &action.ConnectAction{}, "attempting to establish connection", nil)
}

func (s *TryingToConnectState) String() string {
	return helpers.DeriveStateName(s)
}
