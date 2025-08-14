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

package starvationchecker

import (
	"context"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"go.uber.org/zap"
)

// StarvationChecker monitors the control loop's health by detecting periods when
// the system is unable to process reconciliation cycles in a timely manner.
//
// Why it matters:
// - Detects control loop blockages or slowdowns that could affect system reliability
// - Provides early warning of performance issues through metrics and logs
//
// It operates in dual modes:
// - As a standard manager in the reconciliation chain (updating timestamps)
// - Through a background goroutine that checks for missed reconciles every second
//
// When starvation is detected, it:
// - Increments Prometheus metrics for monitoring and alerting
// - Logs warnings with the starvation duration.
type StarvationChecker struct {
	lastReconcileTime   time.Time
	ctx                 context.Context //nolint:containedctx // This is intentional for background service lifecycle
	logger              *zap.SugaredLogger
	cancel              context.CancelFunc
	wg                  sync.WaitGroup
	starvationThreshold time.Duration
	mutex               sync.RWMutex
}

// NewStarvationChecker creates a starvation checker that monitors control loop health.
// It automatically starts a background goroutine that checks for starvation every second.
//
// Parameters:
//   - threshold: The duration after which a control loop is considered starved
//     (typically several times longer than the expected reconciliation interval)
//
// Returns a StarvationChecker that must be stopped with Stop() when no longer needed.
func NewStarvationChecker(threshold time.Duration) *StarvationChecker {
	ctx, cancel := context.WithCancel(context.Background())
	checker := &StarvationChecker{
		starvationThreshold: threshold,
		lastReconcileTime:   time.Now(),
		logger:              logger.For(logger.ComponentStarvationChecker),
		ctx:                 ctx,
		cancel:              cancel,
	}

	checker.wg.Add(1)

	go checker.checkStarvationLoop()

	checker.logger.Infof("Starvation checker created with threshold %s", threshold)

	return checker
}

// checkStarvationLoop continuously monitors the time since the last reconciliation
// and reports starvation events when they exceed the configured threshold.
// This background process ensures starvation is detected even if the main
// reconciliation loop is completely blocked.
func (s *StarvationChecker) checkStarvationLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.mutex.RLock()
			timeSinceLastReconcile := time.Since(s.lastReconcileTime)
			s.mutex.RUnlock()

			if timeSinceLastReconcile > s.starvationThreshold {
				starvationTime := timeSinceLastReconcile.Seconds()
				metrics.AddStarvationTime(starvationTime)
				sentry.ReportIssuef(sentry.IssueTypeWarning, s.logger, "[StarvationChecker.checkStarvationLoop] Control loop starvation detected: %.2f seconds since last reconcile", starvationTime)
			} else {
				s.logger.Infof("Control loop is healthy, last reconcile was %.2f seconds ago", timeSinceLastReconcile.Seconds())
			}
		}
	}
}

// Stop gracefully terminates the background starvation checker.
// This should be called during system shutdown to prevent goroutine leaks.
func (s *StarvationChecker) Stop() {
	s.logger.Info("Stopping starvation checker")
	s.cancel()
	s.wg.Wait()
	s.logger.Info("Starvation checker stopped")
}

// UpdateLastReconcileTime marks the current time as the most recent successful reconciliation.
// This should be called after each successful reconciliation cycle.
func (s *StarvationChecker) UpdateLastReconcileTime() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.lastReconcileTime = time.Now()
}

// GetLastReconcileTime returns the timestamp of the most recent successful reconciliation.
func (s *StarvationChecker) GetLastReconcileTime() time.Time {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return s.lastReconcileTime
}

// GetManagerName returns the component name for logging and metrics.
func (s *StarvationChecker) GetManagerName() string {
	return logger.ComponentStarvationChecker
}

// Reconcile checks if the control loop has been starved and updates metrics if needed.
// This implements the FSMManager interface, allowing the checker to be included in the
// control loop manager list.
//
// Returns:
// - error: Always nil in this implementation as starvation is a warning, not an error
// - bool: Always false, as there is no reconciliation to be done, but this needs to be implemented to satisfy the interface.
func (s *StarvationChecker) Reconcile(ctx context.Context, config config.FullConfig) (error, bool) {
	// We update the timestamp first to mark that the loop is running.
	// This ensures that even if nothing else in the control loop runs,
	// we still know the control loop itself is alive.
	s.UpdateLastReconcileTime()

	return nil, false
}
