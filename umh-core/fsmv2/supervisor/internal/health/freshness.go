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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2"
)

// FreshnessChecker validates observation data age against thresholds.
type FreshnessChecker struct {
	staleThreshold time.Duration
	timeout        time.Duration
}

// NewFreshnessChecker creates a checker with the given thresholds.
func NewFreshnessChecker(staleThreshold, timeout time.Duration) *FreshnessChecker {
	return &FreshnessChecker{
		staleThreshold: staleThreshold,
		timeout:        timeout,
	}
}

// Check validates observation freshness.
// Returns true if data is fresh.
func (f *FreshnessChecker) Check(snapshot *fsmv2.Snapshot) bool {
	return time.Since(snapshot.Observed.GetTimestamp()) < f.staleThreshold
}

// IsTimeout checks if observation data has exceeded the timeout threshold.
// Returns true if data is stale and requires collector restart.
func (f *FreshnessChecker) IsTimeout(snapshot *fsmv2.Snapshot) bool {
	return time.Since(snapshot.Observed.GetTimestamp()) >= f.timeout
}
