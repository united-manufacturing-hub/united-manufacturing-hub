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
type FailingUserSpec struct {
	config.BaseUserSpec
	// ShouldFail: when true, fails MaxFailures times before succeeding.
	ShouldFail bool `json:"should_fail" yaml:"should_fail"`
	// MaxFailures: times to fail before succeeding (default 3).
	MaxFailures int `json:"max_failures" yaml:"max_failures"`
	// FailureCycles: number of failure/success cycles (default 1).
	FailureCycles int `json:"failure_cycles" yaml:"failure_cycles"`
	// RestartAfterFailures: triggers SignalNeedsRestart after this many failures (0 = disabled).
	RestartAfterFailures int `json:"restart_after_failures" yaml:"restart_after_failures"`
}

// GetMaxFailures returns the configured max failures, defaulting to 3.
func (s *FailingUserSpec) GetMaxFailures() int {
	if s.MaxFailures <= 0 {
		return 3
	}

	return s.MaxFailures
}

// GetFailureCycles returns the configured number of failure cycles, defaulting to 1.
func (s *FailingUserSpec) GetFailureCycles() int {
	if s.FailureCycles <= 0 {
		return 1
	}

	return s.FailureCycles
}

// GetRestartAfterFailures returns the configured restart threshold (0 = disabled).
func (s *FailingUserSpec) GetRestartAfterFailures() int {
	return s.RestartAfterFailures
}
