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

package backoff

import (
	"fmt"
	"sync"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"go.uber.org/zap"
)

// Error message constants
const (
	// TemporaryBackoffError indicates a temporary failure with backoff in progress
	TemporaryBackoffError = "operation suspended due to temporary error"

	// PermanentFailureError indicates that max retries were reached
	PermanentFailureError = "operation permanently failed after max retries"
)

// TickClock implements backoff.Clock for tick-based timing
type TickClock struct {
	// Since we're not using time-based backoff, we just need a dummy implementation
	// that doesn't return nil when GetElapsedTime is called
}

// Now returns a dummy time for TickClock
func (t *TickClock) Now() time.Time {
	return time.Unix(0, 0) // Epoch time as a placeholder
}

// BackoffManager handles error backoff with exponential retries and permanent failure detection
type BackoffManager struct {

	// The last error that occurred
	lastError error

	// The backoff policy
	backoff backoff.BackOff

	// Logger
	logger *zap.SugaredLogger

	// Tick-based backoff properties
	suspendedUntilTick uint64
	ticksToWait        uint64

	// Mutex for thread safety
	mu sync.RWMutex

	// Flag indicating permanent failure state (max retries exceeded)
	permanentFailure bool
}

// Config holds configuration for creating a new BackoffManager
type Config struct {

	// Logger
	Logger *zap.SugaredLogger
	// Initial backoff interval in ticks
	InitialInterval uint64

	// Maximum backoff interval in ticks
	MaxInterval uint64

	// Maximum number of retries before permanent failure
	MaxRetries uint64
}

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig(componentName string, logger *zap.SugaredLogger) Config {
	return Config{
		InitialInterval: 1,   // Start with 1 tick backoff
		MaxInterval:     600, // Maximum of 600 ticks (1 minute at 100ms per tick)
		MaxRetries:      5,
		Logger:          logger,
	}
}

// NewBackoffConfig returns a Config with given values
func NewBackoffConfig(
	componentName string,
	initInterval uint64,
	maxInterval uint64,
	maxRetries uint64,
	logger *zap.SugaredLogger,
) Config {
	return Config{
		InitialInterval: initInterval,
		MaxInterval:     maxInterval,
		MaxRetries:      maxRetries,
		Logger:          logger,
	}
}

// NewBackoffManager creates a new BackoffManager with the given config
func NewBackoffManager(config Config) *BackoffManager {
	// Create exponential backoff with the provided settings
	baseBackoff := backoff.NewExponentialBackOff()
	// We're using ticks, so we use time.Duration(1) to represent 1 tick
	baseBackoff.InitialInterval = time.Duration(config.InitialInterval) * time.Millisecond
	baseBackoff.MaxInterval = time.Duration(config.MaxInterval) * time.Millisecond
	// Use our dummy clock instead of nil
	baseBackoff.Clock = &TickClock{}

	// Disable jitter for better determinism in tests
	baseBackoff.RandomizationFactor = 0

	// Wrap with max retries - after MaxRetries failures, it will return backoff.Stop
	backoffWithMaxRetries := backoff.WithMaxRetries(baseBackoff, config.MaxRetries)
	backoffWithMaxRetries.Reset()

	return &BackoffManager{
		backoff:          backoffWithMaxRetries,
		logger:           config.Logger,
		permanentFailure: false,
	}
}

// SetError records an error and updates the backoff state
// Returns true if the backoff has reached permanent failure state
func (m *BackoffManager) SetError(err error, currentTick uint64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.lastError = err

	// If already permanently failed, don't change the state
	if m.permanentFailure {
		return true
	}

	// Get the next backoff duration (in this case, tick count)
	next := m.backoff.NextBackOff()

	// Check if we've reached permanent failure (backoff.Stop)
	if next == backoff.Stop {
		sentry.ReportIssuef(sentry.IssueTypeError, m.logger, "Backoff manager has exceeded maximum retries, marking as permanently failed: %w", err)
		m.permanentFailure = true
		m.suspendedUntilTick = 0 // Clear suspension tick
		return true
	}

	// Extract tick count
	millis := next.Milliseconds()
	ticksToWait := uint64(millis)
	if ticksToWait < 1 {
		ticksToWait = 1 // Minimum of 1 tick
	}

	m.ticksToWait = ticksToWait
	// Set the suspension tick
	m.suspendedUntilTick = currentTick + ticksToWait
	m.logger.Debugf("Suspending operations for %d ticks because of error: %s", ticksToWait, err)

	return false
}

// Reset clears all error and backoff state
func (m *BackoffManager) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.lastError = nil
	m.backoff.Reset()
	m.suspendedUntilTick = 0
	m.permanentFailure = false
}

// ShouldSkipOperation returns true if operations should be skipped due to backoff
func (m *BackoffManager) ShouldSkipOperation(currentTick uint64) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// If permanently failed, operations should always be skipped
	if m.permanentFailure {
		return true
	}

	// If there is no error or no suspension tick, don't skip
	if m.lastError == nil || m.suspendedUntilTick == 0 {
		return false
	}

	// If the backoff period has not yet elapsed, skip the operation
	if currentTick < m.suspendedUntilTick {
		ticksRemaining := m.suspendedUntilTick - currentTick
		m.logger.Debugf("Skipping operation because of error: %s. Remaining ticks: %d",
			m.lastError, ticksRemaining)
		return true
	}

	// Backoff period has elapsed, we can proceed with the operation
	return false
}

// IsPermanentlyFailed returns true if the max retry count has been exceeded
func (m *BackoffManager) IsPermanentlyFailed() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.permanentFailure
}

// GetLastError returns the last error recorded
func (m *BackoffManager) GetLastError() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastError
}

// GetBackoffError returns an appropriate error message based on the current state:
// - For permanent failures, it returns a permanent failure error
// - For temporary backoffs, it returns a temporary backoff error with retry time
// - If no backoff is in progress, it returns nil
func (m *BackoffManager) GetBackoffError(currentTick uint64) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.permanentFailure {
		return fmt.Errorf("%s: %w", PermanentFailureError, m.lastError)
	}

	if m.lastError != nil && m.suspendedUntilTick > 0 && currentTick < m.suspendedUntilTick {
		ticksRemaining := m.suspendedUntilTick - currentTick
		return fmt.Errorf("%s (retry after %d ticks): %w", TemporaryBackoffError, ticksRemaining, m.lastError)
	}

	return nil
}

// SetErrorWithBackoffForTesting is a test-only helper that allows injection of a custom backoff policy
// This is used to create more deterministic tests
func (m *BackoffManager) SetErrorWithBackoffForTesting(err error, customBackoff backoff.BackOff, currentTick uint64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.lastError = err
	m.backoff = customBackoff

	// Set up the backoff period
	next := customBackoff.NextBackOff()
	if next == backoff.Stop {
		m.permanentFailure = true
		return true
	}

	// Extract tick count
	ticksToWait := uint64(next)
	if ticksToWait < 1 {
		ticksToWait = 1 // Minimum of 1 tick
	}

	m.ticksToWait = ticksToWait
	m.suspendedUntilTick = currentTick + ticksToWait
	return false
}
