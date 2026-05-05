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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// ExamplefailingConfig defines the typed configuration for the failing worker.
type ExamplefailingConfig struct {
	config.BaseUserSpec
	// ShouldFail: when true, fails MaxFailures times before succeeding.
	ShouldFail bool `json:"should_fail" yaml:"should_fail"`
	// MaxFailures: times to fail before succeeding (default 3).
	MaxFailures int `json:"max_failures" yaml:"max_failures"`
	// FailureCycles: number of failure/success cycles (default 1).
	FailureCycles int `json:"failure_cycles" yaml:"failure_cycles"`
	// RestartAfterFailures: triggers SignalNeedsRestart after this many failures (0 = disabled).
	RestartAfterFailures int `json:"restart_after_failures" yaml:"restart_after_failures"`
	// RecoveryDelayObservations: number of observation cycles to wait after failure.
	RecoveryDelayObservations int `json:"recovery_delay_observations" yaml:"recovery_delay_observations"`
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

// ExamplefailingStatus holds the runtime observation data for the failing worker.
// Only worker-specific business fields belong here; the framework supplies
// CollectedAt, State, LastActionResults, MetricsEmbedder, and ChildrenHealthy/Unhealthy
// via Observation[ExamplefailingStatus].
type ExamplefailingStatus struct {
	// ConnectionHealth reports the current connection state ("healthy" or "no connection").
	// Populated by CollectObservedState from dependencies.IsConnected().
	ConnectionHealth string `json:"connection_health"`

	// ConnectAttempts is the number of connect attempts since the last cycle reset.
	ConnectAttempts int `json:"connect_attempts"`
	// TicksInConnectedState counts wall-clock ticks the worker has spent in Connected.
	TicksInConnectedState int `json:"ticks_in_connected"`
	// CurrentCycle is the index of the current failure/success cycle.
	CurrentCycle int `json:"current_cycle"`
	// AllCyclesComplete is true when CurrentCycle has reached the configured FailureCycles.
	AllCyclesComplete bool `json:"all_cycles_complete"`
	// RecoveryDelayActive is true when waiting after a failure before retrying.
	// Keeps the worker in the unhealthy state long enough for parents to observe.
	RecoveryDelayActive bool `json:"recovery_delay_active"`
	// ObservationsSinceFailure counts CollectObservedState calls since the last failure.
	ObservationsSinceFailure int `json:"observations_since_failure"`
}
