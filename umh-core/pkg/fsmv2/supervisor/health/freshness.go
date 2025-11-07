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
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"go.uber.org/zap"
)

// FreshnessChecker validates observation data age against thresholds.
type FreshnessChecker struct {
	staleThreshold time.Duration
	timeout        time.Duration
	logger         *zap.SugaredLogger
}

// NewFreshnessChecker creates a checker with the given thresholds.
func NewFreshnessChecker(staleThreshold, timeout time.Duration, logger *zap.SugaredLogger) *FreshnessChecker {
	return &FreshnessChecker{
		staleThreshold: staleThreshold,
		timeout:        timeout,
		logger:         logger,
	}
}

// extractTimestamp extracts the collection timestamp from snapshot.Observed.
// Returns (timestamp, true) if extraction succeeds, (zero, false) otherwise.
// Handles both GetTimestamp() interface and persistence.Document formats.
func (f *FreshnessChecker) extractTimestamp(snapshot *fsmv2.Snapshot) (time.Time, bool) {
	if snapshot.Observed == nil {
		return time.Time{}, false
	}

	if timestampProvider, ok := snapshot.Observed.(interface{ GetTimestamp() time.Time }); ok {
		return timestampProvider.GetTimestamp(), true
	}

	doc, ok := snapshot.Observed.(persistence.Document)
	if !ok {
		f.logger.Warnw("Observed state is neither GetTimestamp() nor Document, assuming fresh data",
			"identity", snapshot.Identity,
			"type", fmt.Sprintf("%T", snapshot.Observed))
		return time.Time{}, false
	}

	ts, exists := doc["collectedAt"]
	if !exists {
		f.logger.Warnw("Document does not have collectedAt field, assuming fresh data",
			"identity", snapshot.Identity)
		return time.Time{}, false
	}

	if timestamp, ok := ts.(time.Time); ok {
		return timestamp, true
	}

	if timeStr, ok := ts.(string); ok {
		collectedAt, err := time.Parse(time.RFC3339Nano, timeStr)
		if err != nil {
			f.logger.Warnw("collectedAt field is string but cannot parse as RFC3339",
				"identity", snapshot.Identity,
				"value", timeStr,
				"error", err)
			return time.Time{}, false
		}
		return collectedAt, true
	}

	f.logger.Warnw("collectedAt field exists but is not time.Time or string",
		"identity", snapshot.Identity,
		"type", fmt.Sprintf("%T", ts))
	return time.Time{}, false
}

// Check validates observation freshness.
// Returns true if data is fresh.
func (f *FreshnessChecker) Check(snapshot *fsmv2.Snapshot) bool {
	if snapshot.Observed == nil {
		return false
	}

	collectedAt, ok := f.extractTimestamp(snapshot)
	if !ok {
		return true
	}

	age := time.Since(collectedAt)
	isFresh := age < f.staleThreshold

	if !isFresh {
		f.logger.Debugw("Observed state is stale",
			"identity", snapshot.Identity,
			"age", age,
			"threshold", f.staleThreshold)
	}

	return isFresh
}

// IsTimeout checks if observation data has exceeded the timeout threshold.
// Returns true if data is critically old and requires collector restart.
func (f *FreshnessChecker) IsTimeout(snapshot *fsmv2.Snapshot) bool {
	collectedAt, ok := f.extractTimestamp(snapshot)
	if !ok {
		return false
	}

	age := time.Since(collectedAt)
	isTimedOut := age >= f.timeout

	if isTimedOut {
		f.logger.Warnw("Observed state has timed out",
			"identity", snapshot.Identity,
			"age", age,
			"threshold", f.timeout)
	}

	return isTimedOut
}
