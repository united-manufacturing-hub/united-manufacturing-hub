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

// Package snapshot holds the examplefailing worker's Config and Status value
// types plus the ExamplefailingDependencies interface consumed by actions. It
// exists as a separate leaf package so the state sub-package can depend on
// these types without introducing an import cycle with the worker package.
//
// Post-PR3-C4 the examplefailing worker uses fsmv2.Observation[ExamplefailingStatus]
// and *fsmv2.WrappedDesiredState[ExamplefailingConfig]; the underlying value
// types are defined here and re-exported from the worker package as type
// aliases for caller convenience.
package snapshot

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// ExamplefailingConfig holds the user-provided configuration for the failing
// worker. Embeds BaseUserSpec to expose GetState() for WorkerBase.DeriveDesiredState, allowing
// WorkerBase.DeriveDesiredState to extract the desired state from the "state"
// YAML field. Configured values are synced into the runtime dependencies via
// the worker's SetPostParseHook closure so actions can observe them.
type ExamplefailingConfig struct {
	config.BaseUserSpec `yaml:",inline"`
	// ShouldFail: when true, fails MaxFailures times before succeeding.
	ShouldFail bool `json:"should_fail" yaml:"should_fail"`
	// MaxFailures: times to fail before succeeding (default 3).
	MaxFailures int `json:"max_failures" yaml:"max_failures"`
	// FailureCycles: number of failure/success cycles (default 1).
	FailureCycles int `json:"failure_cycles" yaml:"failure_cycles"`
	// RestartAfterFailures: triggers SignalNeedsRestart after this many failures (0 = disabled).
	RestartAfterFailures int `json:"restart_after_failures" yaml:"restart_after_failures"`
	// RecoveryDelayMs: time in milliseconds to wait after a failure before retrying.
	// Kept for backward compatibility; prefer RecoveryDelayObservations for deterministic tests.
	RecoveryDelayMs int `json:"recovery_delay_ms" yaml:"recovery_delay_ms"`
	// RecoveryDelayObservations: number of observation cycles to wait after failure.
	// This is preferred over RecoveryDelayMs for deterministic testing.
	RecoveryDelayObservations int `json:"recovery_delay_observations" yaml:"recovery_delay_observations"`
}

// GetMaxFailures returns the configured max failures, defaulting to 3.
func (c *ExamplefailingConfig) GetMaxFailures() int {
	if c.MaxFailures <= 0 {
		return 3
	}

	return c.MaxFailures
}

// GetFailureCycles returns the configured number of failure cycles, defaulting to 1.
func (c *ExamplefailingConfig) GetFailureCycles() int {
	if c.FailureCycles <= 0 {
		return 1
	}

	return c.FailureCycles
}

// GetRestartAfterFailures returns the configured restart threshold (0 = disabled).
func (c *ExamplefailingConfig) GetRestartAfterFailures() int {
	return c.RestartAfterFailures
}

// GetRecoveryDelayObservations returns the configured observation delay threshold.
func (c *ExamplefailingConfig) GetRecoveryDelayObservations() int {
	return c.RecoveryDelayObservations
}

// ExamplefailingStatus holds the runtime observation data for the failing worker.
// Framework fields (CollectedAt, State, LastActionResults, MetricsEmbedder,
// ShutdownRequested, children counts) are carried by fsmv2.Observation[ExamplefailingStatus]
// and are not duplicated here.
type ExamplefailingStatus struct {
	ConnectionHealth         string `json:"connection_health"`
	ConnectAttempts          int    `json:"connect_attempts"`
	RestartAfterFailures     int    `json:"restart_after_failures"`
	TicksInConnectedState    int    `json:"ticks_in_connected"`
	CurrentCycle             int    `json:"current_cycle"`
	TotalCycles              int    `json:"total_cycles"`
	ObservationsSinceFailure int    `json:"observations_since_failure"`
	ShouldFail               bool   `json:"ShouldFail"`
	AllCyclesComplete        bool   `json:"all_cycles_complete"`
	// RecoveryDelayActive is true when waiting after a failure before retrying.
	// Keeps the worker in the unhealthy state long enough for parents to observe.
	RecoveryDelayActive bool `json:"recovery_delay_active"`
}

// ExamplefailingDependencies is the dependencies interface for examplefailing
// actions (avoids import cycles between action/ and the worker package).
type ExamplefailingDependencies interface {
	deps.Dependencies
	GetShouldFail() bool
	IncrementAttempts() int
	GetAttempts() int
	ResetAttempts()
	GetMaxFailures() int
	SetConnected(connected bool)
	IsConnected() bool
	GetRestartAfterFailures() int
	GetFailureCycles() int
	GetCurrentCycle() int
	AllCyclesComplete() bool
	AdvanceCycle() int
	IncrementTicksInConnected() int
	GetTicksInConnected() int
	ResetTicksInConnected()
	// Recovery delay support (time-based - kept for backward compatibility).
	SetLastFailureTime(t time.Time)
	GetLastFailureTime() time.Time
	ShouldDelayRecovery() bool
	GetRecoveryDelayMs() int
	// Recovery delay support (observation-based - preferred).
	SetRecoveryDelayObservations(n int)
	GetRecoveryDelayObservations() int
	IncrementObservationsSinceFailure() int
	GetObservationsSinceFailure() int
	ResetObservationsSinceFailure()
}
