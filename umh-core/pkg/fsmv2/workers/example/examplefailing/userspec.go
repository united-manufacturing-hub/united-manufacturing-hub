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

// FailingUserSpec defines the typed configuration for the failing worker.
// This is parsed from the UserSpec.Config YAML/JSON string.
//
// Example YAML:
//
//	should_fail: true
//	max_failures: 3
//	restart_after_failures: 5
//
// This configures the worker to fail 3 times before succeeding,
// demonstrating exponential backoff and eventual recovery.
// If restart_after_failures is set, the worker will emit SignalNeedsRestart
// after that many consecutive failures, triggering a full worker restart.
type FailingUserSpec struct {
	config.BaseUserSpec // Provides State field with GetState() defaulting to "running"
	// ShouldFail controls whether the connect action should fail.
	// When true, the action will fail MaxFailures times before succeeding.
	ShouldFail bool `json:"should_fail" yaml:"should_fail"`
	// MaxFailures is the number of times the connect action will fail before succeeding.
	// Default is 3 if not specified.
	MaxFailures int `json:"max_failures" yaml:"max_failures"`
	// FailureCycles is the number of complete failure cycles to perform.
	// Each cycle consists of MaxFailures failures followed by a successful connection.
	// After each successful connection (except the last cycle), the worker will
	// simulate a disconnection and start failing again.
	// Default is 1 (single failure cycle, backward compatible).
	FailureCycles int `json:"failure_cycles" yaml:"failure_cycles"`
	// RestartAfterFailures triggers SignalNeedsRestart after this many consecutive failures.
	// Default is 0 (no restart). When set, the worker will request a full restart
	// instead of continuing to retry forever.
	RestartAfterFailures int `json:"restart_after_failures" yaml:"restart_after_failures"`
}

// GetMaxFailures returns the configured max failures, defaulting to 3.
func (s *FailingUserSpec) GetMaxFailures() int {
	if s.MaxFailures <= 0 {
		return 3 // Default
	}

	return s.MaxFailures
}

// GetFailureCycles returns the configured number of failure cycles, defaulting to 1.
func (s *FailingUserSpec) GetFailureCycles() int {
	if s.FailureCycles <= 0 {
		return 1 // Default: single cycle (backward compatible)
	}

	return s.FailureCycles
}

// GetRestartAfterFailures returns the configured restart threshold.
// Returns 0 (default) to indicate no restart.
func (s *FailingUserSpec) GetRestartAfterFailures() int {
	return s.RestartAfterFailures
}
