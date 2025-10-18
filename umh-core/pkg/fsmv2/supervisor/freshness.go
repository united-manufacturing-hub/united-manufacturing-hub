// Copyright 2025 UMH Systems GmbH
package supervisor

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
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
	observed := snapshot.Observed
	if observed == nil {
		return false
	}

	timestampProvider, ok := observed.(interface{ GetTimestamp() time.Time })
	if !ok {
		return false
	}

	age := time.Since(timestampProvider.GetTimestamp())

	return age <= f.staleThreshold
}

// IsTimeout checks if observation data has exceeded the timeout threshold.
// Returns true if data is critically old and requires collector restart.
func (f *FreshnessChecker) IsTimeout(snapshot *fsmv2.Snapshot) bool {
	observed := snapshot.Observed
	if observed == nil {
		return false
	}

	timestampProvider, ok := observed.(interface{ GetTimestamp() time.Time })
	if !ok {
		return false
	}

	age := time.Since(timestampProvider.GetTimestamp())

	return age > f.timeout
}
