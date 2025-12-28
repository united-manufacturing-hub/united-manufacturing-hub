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
	ShouldFail bool `yaml:"should_fail" json:"should_fail"`
	// MaxFailures is the number of times the connect action will fail before succeeding.
	// Default is 3 if not specified.
	MaxFailures int `yaml:"max_failures" json:"max_failures"`
	// RestartAfterFailures triggers SignalNeedsRestart after this many consecutive failures.
	// Default is 0 (no restart). When set, the worker will request a full restart
	// instead of continuing to retry forever.
	RestartAfterFailures int `yaml:"restart_after_failures" json:"restart_after_failures"`
}

// GetMaxFailures returns the configured max failures, defaulting to 3.
func (s *FailingUserSpec) GetMaxFailures() int {
	if s.MaxFailures <= 0 {
		return 3 // Default
	}
	return s.MaxFailures
}

// GetRestartAfterFailures returns the configured restart threshold.
// Returns 0 (default) to indicate no restart.
func (s *FailingUserSpec) GetRestartAfterFailures() int {
	return s.RestartAfterFailures
}
