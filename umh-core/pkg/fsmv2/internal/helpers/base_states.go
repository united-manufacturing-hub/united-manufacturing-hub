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

package helpers

import "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"

// StartingBase provides LifecyclePhase() for PhaseStarting states.
// Embed in your state to automatically get the correct phase.
//
// Usage:
//
//	type ConnectingState struct {
//	    helpers.StartingBase
//	}
type StartingBase struct{}

func (b StartingBase) LifecyclePhase() config.LifecyclePhase {
	return config.PhaseStarting
}

// RunningHealthyBase provides LifecyclePhase() for PhaseRunningHealthy states.
// Embed in your state for operational, stable states.
//
// Usage:
//
//	type SyncingState struct {
//	    helpers.RunningHealthyBase
//	}
type RunningHealthyBase struct{}

func (b RunningHealthyBase) LifecyclePhase() config.LifecyclePhase {
	return config.PhaseRunningHealthy
}

// RunningDegradedBase provides LifecyclePhase() for PhaseRunningDegraded states.
// Embed in your state for operational but impaired states.
//
// Usage:
//
//	type RecoveringState struct {
//	    helpers.RunningDegradedBase
//	}
type RunningDegradedBase struct{}

func (b RunningDegradedBase) LifecyclePhase() config.LifecyclePhase {
	return config.PhaseRunningDegraded
}

// StoppingBase provides LifecyclePhase() for PhaseStopping states.
// Embed in your state for graceful shutdown states.
//
// Usage:
//
//	type ShuttingDownState struct {
//	    helpers.StoppingBase
//	}
type StoppingBase struct{}

func (b StoppingBase) LifecyclePhase() config.LifecyclePhase {
	return config.PhaseStopping
}

// StoppedBase provides LifecyclePhase() for PhaseStopped states.
// Embed in your state for terminal, cleanly shut down states.
//
// Usage:
//
//	type StoppedState struct {
//	    helpers.StoppedBase
//	}
type StoppedBase struct{}

func (b StoppedBase) LifecyclePhase() config.LifecyclePhase {
	return config.PhaseStopped
}
