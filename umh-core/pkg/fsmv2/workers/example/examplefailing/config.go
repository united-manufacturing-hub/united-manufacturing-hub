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

package examplefailing

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// ExamplefailingConfig is the typed configuration for the failing worker.
type ExamplefailingConfig struct {
	config.BaseUserSpec
	ShouldFail                bool `json:"should_fail"                 yaml:"should_fail"`
	MaxFailures               int  `json:"max_failures"                yaml:"max_failures"`
	FailureCycles             int  `json:"failure_cycles"              yaml:"failure_cycles"`
	RestartAfterFailures      int  `json:"restart_after_failures"      yaml:"restart_after_failures"`
	RecoveryDelayMs           int  `json:"recovery_delay_ms"           yaml:"recovery_delay_ms"`
	RecoveryDelayObservations int  `json:"recovery_delay_observations" yaml:"recovery_delay_observations"`
}

// GetMaxFailures returns the configured max failures, defaulting to 3.
func (s *ExamplefailingConfig) GetMaxFailures() int {
	if s.MaxFailures <= 0 {
		return 3
	}

	return s.MaxFailures
}

// GetFailureCycles returns the configured number of failure cycles, defaulting to 1.
func (s *ExamplefailingConfig) GetFailureCycles() int {
	if s.FailureCycles <= 0 {
		return 1
	}

	return s.FailureCycles
}

// GetRestartAfterFailures returns the configured restart threshold (0 = disabled).
func (s *ExamplefailingConfig) GetRestartAfterFailures() int {
	return s.RestartAfterFailures
}

// GetRecoveryDelayObservations returns the configured number of observation cycles to wait after failure.
func (s *ExamplefailingConfig) GetRecoveryDelayObservations() int {
	return s.RecoveryDelayObservations
}

// ExamplefailingStatus is the observed status for the failing worker.
type ExamplefailingStatus struct {
	ConnectionHealth         string `json:"connection_health"`
	ConnectAttempts          int    `json:"connect_attempts"`
	RestartAfterFailures     int    `json:"restart_after_failures"`
	TicksInConnectedState    int    `json:"ticks_in_connected"`
	CurrentCycle             int    `json:"current_cycle"`
	TotalCycles              int    `json:"total_cycles"`
	ObservationsSinceFailure int    `json:"observations_since_failure"`
	AllCyclesComplete        bool   `json:"all_cycles_complete"`
	RecoveryDelayActive      bool   `json:"recovery_delay_active"`
}

// ExamplefailingDepsIface is the dependency interface used by examplefailing action files
// and CollectObservedState for applying config mutations.
type ExamplefailingDepsIface interface {
	deps.Dependencies
	SetShouldFail(shouldFail bool)
	GetShouldFail() bool
	SetMaxFailures(maxFailures int)
	GetMaxFailures() int
	SetRestartAfterFailures(n int)
	GetRestartAfterFailures() int
	SetFailureCycles(cycles int)
	GetFailureCycles() int
	SetRecoveryDelayMs(delayMs int)
	GetRecoveryDelayMs() int
	SetRecoveryDelayObservations(n int)
	GetRecoveryDelayObservations() int
	IncrementAttempts() int
	GetAttempts() int
	ResetAttempts()
	SetConnected(connected bool)
	IsConnected() bool
	GetCurrentCycle() int
	AllCyclesComplete() bool
	AdvanceCycle() int
	IncrementTicksInConnected() int
	GetTicksInConnected() int
	ResetTicksInConnected()
	SetLastFailureTime(t time.Time)
	GetLastFailureTime() time.Time
	ShouldDelayRecovery() bool
	IncrementObservationsSinceFailure() int
	GetObservationsSinceFailure() int
	ResetObservationsSinceFailure()
}

// Compile-time check: FailingDependencies must satisfy ExamplefailingDepsIface.
var _ ExamplefailingDepsIface = (*FailingDependencies)(nil)
