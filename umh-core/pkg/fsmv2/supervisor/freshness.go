// Copyright 2025 UMH Systems GmbH
package supervisor

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

// Check validates observation freshness.
// Returns true if data is fresh.
func (f *FreshnessChecker) Check(snapshot *fsmv2.Snapshot) bool {
	if snapshot.Observed == nil {
		return false
	}

	var collectedAt time.Time

	if timestampProvider, ok := snapshot.Observed.(interface{ GetTimestamp() time.Time }); ok {
		collectedAt = timestampProvider.GetTimestamp()
	} else if doc, ok := snapshot.Observed.(persistence.Document); ok {
		if ts, exists := doc["collectedAt"]; exists {
			if timestamp, ok := ts.(time.Time); ok {
				collectedAt = timestamp
			} else if timeStr, ok := ts.(string); ok {
				var err error
				collectedAt, err = time.Parse(time.RFC3339Nano, timeStr)
				if err != nil {
					f.logger.Warnw("collectedAt field is string but cannot parse as RFC3339",
						"identity", snapshot.Identity,
						"value", timeStr,
						"error", err)
					return true
				}
			} else {
				f.logger.Warnw("collectedAt field exists but is not time.Time or string",
					"identity", snapshot.Identity,
					"type", fmt.Sprintf("%T", ts))
				return true
			}
		} else {
			f.logger.Warnw("Document does not have collectedAt field, assuming fresh data",
				"identity", snapshot.Identity)
			return true
		}
	} else {
		f.logger.Warnw("Observed state is neither GetTimestamp() nor Document, assuming fresh data",
			"identity", snapshot.Identity,
			"type", fmt.Sprintf("%T", snapshot.Observed))
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
	if snapshot.Observed == nil {
		return false
	}

	var collectedAt time.Time

	if timestampProvider, ok := snapshot.Observed.(interface{ GetTimestamp() time.Time }); ok {
		collectedAt = timestampProvider.GetTimestamp()
	} else if doc, ok := snapshot.Observed.(persistence.Document); ok {
		if ts, exists := doc["collectedAt"]; exists {
			if timestamp, ok := ts.(time.Time); ok {
				collectedAt = timestamp
			} else if timeStr, ok := ts.(string); ok {
				var err error
				collectedAt, err = time.Parse(time.RFC3339Nano, timeStr)
				if err != nil {
					f.logger.Warnw("collectedAt field is string but cannot parse as RFC3339 in timeout check",
						"identity", snapshot.Identity,
						"value", timeStr,
						"error", err)
					return false
				}
			} else {
				f.logger.Warnw("collectedAt field exists but is not time.Time or string in timeout check",
					"identity", snapshot.Identity,
					"type", fmt.Sprintf("%T", ts))
				return false
			}
		} else {
			f.logger.Warnw("Document does not have collectedAt field in timeout check, assuming not timed out",
				"identity", snapshot.Identity)
			return false
		}
	} else {
		f.logger.Warnw("Observed state is neither GetTimestamp() nor Document in timeout check, assuming not timed out",
			"identity", snapshot.Identity,
			"type", fmt.Sprintf("%T", snapshot.Observed))
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
