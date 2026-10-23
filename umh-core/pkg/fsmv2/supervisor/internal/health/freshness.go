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

package health

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// FreshnessChecker validates observation data age against thresholds.
type FreshnessChecker struct {
	logger         deps.FSMLogger
	workerType     string
	staleThreshold time.Duration
	timeout        time.Duration
}

// NewFreshnessChecker creates a checker with the given thresholds.
func NewFreshnessChecker(staleThreshold, timeout time.Duration, workerType string, logger deps.FSMLogger) *FreshnessChecker {
	return &FreshnessChecker{
		staleThreshold: staleThreshold,
		timeout:        timeout,
		workerType:     workerType,
		logger:         logger,
	}
}

// Check validates observation freshness.
// Returns true if data is fresh.
func (f *FreshnessChecker) Check(snapshot *fsmv2.Snapshot) bool {
	age := time.Since(snapshot.Observed.GetTimestamp())
	isFresh := age < f.staleThreshold

	if !isFresh {
		f.logger.Debug("observed_state_stale",
			deps.HierarchyPath(snapshot.Identity.HierarchyPath),
			deps.Duration("age", age),
			deps.Duration("threshold", f.staleThreshold))
	}

	return isFresh
}

// IsTimeout checks if observation data has exceeded the timeout threshold.
// Returns true if data is stale and requires collector restart.
func (f *FreshnessChecker) IsTimeout(snapshot *fsmv2.Snapshot) bool {
	collectedAt := snapshot.Observed.GetTimestamp()
	age := time.Since(collectedAt)
	isTimedOut := age >= f.timeout

	if isTimedOut {
		f.logger.SentryWarn(deps.FeatureForWorker(f.workerType), snapshot.Identity.HierarchyPath, "observed_state_timeout",
			deps.Duration("age", age),
			deps.Int64("age_ms", age.Milliseconds()),
			deps.Duration("threshold", f.timeout),
			deps.Int64("threshold_ms", f.timeout.Milliseconds()),
			deps.String("collected_at", collectedAt.Format(time.RFC3339Nano)))
	}

	return isTimedOut
}
