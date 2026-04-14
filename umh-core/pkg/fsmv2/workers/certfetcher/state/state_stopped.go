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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/certfetcher/snapshot"
)

const workerType = "certfetcher"

func init() {
	fsmv2.RegisterInitialState(workerType, &StoppedState{})
}

// StoppedState is the initial state of the cert fetcher worker.
type StoppedState struct {
	helpers.StoppedBase
}

// Next transitions to Running when SubHandler is available.
func (s *StoppedState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[snapshot.CertFetcherConfig, snapshot.CertFetcherStatus](snapAny)

	if snap.IsShutdownRequested {
		return fsmv2.Result[any, any](s, fsmv2.SignalNeedsRemoval, nil, "shutdown requested")
	}

	if snap.Status.HasSubHandler {
		return fsmv2.Result[any, any](&RunningState{}, fsmv2.SignalNone, nil, "sub handler available, starting")
	}

	return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil, "waiting for sub handler")
}

func (s *StoppedState) String() string {
	return helpers.DeriveStateName(s)
}
