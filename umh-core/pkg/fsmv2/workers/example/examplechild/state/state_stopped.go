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
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	example_child "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild"
)

func init() {
	fsmv2.RegisterInitialState("examplechild", &StoppedState{})
}

// StoppedState represents the initial state where the child worker is not connected.
type StoppedState struct {
	helpers.StoppedBase
}

func (s *StoppedState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[example_child.ExamplechildConfig, example_child.ExamplechildStatus](snapAny)

	if snap.IsShutdownRequested {
		return fsmv2.Result[any, any](s, fsmv2.SignalNeedsRemoval, nil, fmt.Sprintf("shutdown requested, stopping: parentState=%s", snap.ParentMappedState))
	}

	if snap.ParentMappedState == config.DesiredStateRunning {
		return fsmv2.Result[any, any](&TryingToConnectState{}, fsmv2.SignalNone, nil, fmt.Sprintf("parent wants running, attempting to connect: parentState=%s", snap.ParentMappedState))
	}

	return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil, "Child is stopped, no connection")
}

func (s *StoppedState) String() string {
	return helpers.DeriveStateName(s)
}
