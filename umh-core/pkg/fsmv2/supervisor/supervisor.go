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

package supervisor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/persistence"
)

// =============================================================================
// SUPERVISOR INVARIANTS
// =============================================================================
//
// The supervisor maintains the following invariants to ensure correct operation.
// Violations indicate programming errors (bugs in supervisor logic or incorrect usage).
//
// I1: restartCount range
//     MUST: 0 <= restartCount <= maxRestartAttempts
//     WHY:  Prevents infinite restart loops and ensures bounded recovery
//     ENFORCED: RestartCollector() panics if called when count >= max
//
// I2: threshold ordering
//     MUST: 0 < staleThreshold < timeout
//     WHY:  Stale detection must occur before timeout triggers collector restart
//     ENFORCED: NewSupervisor() panics if configuration violates this
//
// I3: trust boundary (data freshness)
//     MUST: state.Next() only called when CheckDataFreshness() returns true
//     WHY:  States assume observation data is always fresh (supervisor's responsibility)
//     ENFORCED: tick() checks freshness and pauses FSM if data is stale
//
// I4: bounded retry (escalation)
//     MUST: RestartCollector() not called when restartCount >= maxRestartAttempts
//     WHY:  Must escalate to shutdown after max attempts, not retry forever
//     ENFORCED: tick() logic ensures this + RestartCollector() panics if violated
//
// I7: timeout ordering validation
//     MUST: ObservationTimeout < StaleThreshold < CollectorTimeout
//     WHY:  Observation failures must not trigger stale detection, and stale detection
//           must occur before collector restart
//     ENFORCED: NewSupervisor() panics if configuration violates this ordering
//
// =============================================================================

// CollectorHealth tracks the health and restart state of the observation collector.
// The collector runs in a separate goroutine and may fail due to network issues,
// blocked operations, or other infrastructure problems.
//
// This struct is internal to Supervisor and tracks runtime state.
// Configuration is provided via CollectorHealthConfig.
type CollectorHealth struct {
	staleThreshold     time.Duration // Age when data considered stale (pause FSM)
	timeout            time.Duration // Age when collector considered broken (trigger restart)
	maxRestartAttempts int           // Maximum restart attempts before escalation
	restartCount       int           // Current restart attempt counter
	lastRestart        time.Time     // Timestamp of last restart attempt
}

// Supervisor manages the lifecycle of a single worker.
// It runs two goroutines:
//   1. Observation loop: Continuously calls worker.CollectObservedState()
//   2. Main tick loop: Calls state.Next() and executes actions
//
// The supervisor implements the 4-layer defense for data freshness:
//   - Layer 1: Pause FSM when data is stale (>10s by default)
//   - Layer 2: Restart collector when data times out (>20s by default)
//   - Layer 3: Request graceful shutdown after max restart attempts
//   - Layer 4: Comprehensive logging and metrics (observability)
type Supervisor struct {
	worker           fsmv2.Worker          // Worker implementation
	identity         fsmv2.Identity        // Worker's immutable identity
	store            persistence.Store     // State persistence layer
	currentState     fsmv2.State           // Current FSM state
	logger           *zap.SugaredLogger    // Logger for supervisor operations
	tickInterval     time.Duration         // How often to evaluate state transitions
	collectorHealth  CollectorHealth       // Collector health tracking
	collector        *Collector            // Observation data collector
	freshnessChecker *FreshnessChecker     // Data freshness validator
}

// CollectorHealthConfig configures observation collector health monitoring.
// The collector runs in a separate goroutine and may fail due to network issues,
// blocked operations, or infrastructure problems. These settings control when to
// pause the FSM (stale data) and when to restart the collector (timeout).
type CollectorHealthConfig struct {
	// ObservationTimeout is the maximum time allowed for a single observation operation.
	// If an observation takes longer than this, it is cancelled and considered failed.
	// Must be less than StaleThreshold to ensure observation failures don't trigger stale detection.
	// Default: ~1.3 seconds (see DefaultObservationTimeout in constants.go)
	ObservationTimeout time.Duration

	// StaleThreshold is how old observation data can be before FSM pauses.
	// When exceeded, supervisor stops calling state.Next() but does not restart collector.
	// Default: 10 seconds (see DefaultStaleThreshold in constants.go)
	StaleThreshold time.Duration

	// Timeout is how old observation data can be before collector is considered broken.
	// When exceeded, supervisor triggers collector restart with exponential backoff.
	// Should be significantly larger than StaleThreshold to avoid restart thrashing.
	// Default: 20 seconds (see DefaultCollectorTimeout in constants.go)
	Timeout time.Duration

	// MaxRestartAttempts is the maximum number of collector restart attempts.
	// After this many failed restarts, supervisor escalates to graceful FSM shutdown.
	// Each restart uses exponential backoff: attempt N waits N*2 seconds.
	// Default: 3 attempts (see DefaultMaxRestartAttempts in constants.go)
	MaxRestartAttempts int
}

// Config contains supervisor configuration.
// All fields except Worker, Identity, and Store have sensible defaults.
type Config struct {
	// Worker implements the FSM worker interface.
	// Required - no default.
	Worker fsmv2.Worker

	// Identity contains the worker's immutable ID and name.
	// Required - no default.
	Identity fsmv2.Identity

	// Store persists FSM state (identity, desired, observed).
	// Required - no default.
	Store persistence.Store

	// Logger for supervisor operations.
	// Required - no default (use zap.NewNop().Sugar() for tests).
	Logger *zap.SugaredLogger

	// TickInterval is how often supervisor evaluates FSM state transitions.
	// Optional - defaults to DefaultTickInterval (1 second).
	TickInterval time.Duration

	// CollectorHealth configures observation collector monitoring.
	// The collector runs in a separate goroutine collecting observed state.
	// These settings control when to pause the FSM (stale data) and when to
	// restart the collector (timeout).
	// Optional - all fields default to values in constants.go.
	CollectorHealth CollectorHealthConfig
}

func NewSupervisor(cfg Config) *Supervisor {
	tickInterval := cfg.TickInterval
	if tickInterval == 0 {
		tickInterval = DefaultTickInterval
	}

	// NOTE (Invariant I13): Zero timeouts default to safe values.
	// This prevents invalid configurations where timeouts of 0 would
	// cause immediate failures or infinite loops.
	observationTimeout := cfg.CollectorHealth.ObservationTimeout
	if observationTimeout == 0 {
		observationTimeout = DefaultObservationTimeout
	}

	staleThreshold := cfg.CollectorHealth.StaleThreshold
	if staleThreshold == 0 {
		staleThreshold = DefaultStaleThreshold
	}

	timeout := cfg.CollectorHealth.Timeout
	if timeout == 0 {
		timeout = DefaultCollectorTimeout
	}

	maxRestartAttempts := cfg.CollectorHealth.MaxRestartAttempts
	if maxRestartAttempts == 0 {
		maxRestartAttempts = DefaultMaxRestartAttempts
	}

	// I2: Validate threshold ordering (invariant enforcement)
	if staleThreshold <= 0 {
		panic(fmt.Sprintf("supervisor config error: staleThreshold must be positive, got %v", staleThreshold))
	}

	if timeout <= staleThreshold {
		panic(fmt.Sprintf("supervisor config error: timeout (%v) must be greater than staleThreshold (%v)", timeout, staleThreshold))
	}

	if maxRestartAttempts <= 0 {
		panic(fmt.Sprintf("supervisor config error: maxRestartAttempts must be positive, got %d", maxRestartAttempts))
	}

	// I7: Validate timeout ordering (ObservationTimeout < StaleThreshold < CollectorTimeout)
	if observationTimeout >= staleThreshold {
		panic(fmt.Sprintf("supervisor config error: observationTimeout (%v) must be less than staleThreshold (%v)", observationTimeout, staleThreshold))
	}

	if staleThreshold >= timeout {
		panic(fmt.Sprintf("supervisor config error: staleThreshold (%v) must be less than collectorTimeout (%v)", staleThreshold, timeout))
	}

	cfg.Logger.Infof("Supervisor timeout configuration: ObservationTimeout=%v, StaleThreshold=%v, CollectorTimeout=%v",
		observationTimeout, staleThreshold, timeout)

	freshnessChecker := NewFreshnessChecker(staleThreshold, timeout, cfg.Logger)

	collector := NewCollector(CollectorConfig{
		Worker:              cfg.Worker,
		Identity:            cfg.Identity,
		Store:               cfg.Store,
		Logger:              cfg.Logger,
		ObservationInterval: DefaultObservationInterval,
		ObservationTimeout:  observationTimeout,
	})

	return &Supervisor{
		worker:           cfg.Worker,
		identity:         cfg.Identity,
		store:            cfg.Store,
		currentState:     cfg.Worker.GetInitialState(),
		logger:           cfg.Logger,
		tickInterval:     tickInterval,
		collector:        collector,
		freshnessChecker: freshnessChecker,
		collectorHealth: CollectorHealth{
			staleThreshold:     staleThreshold,
			timeout:            timeout,
			maxRestartAttempts: maxRestartAttempts,
			restartCount:       0,
		},
	}
}

func (s *Supervisor) GetStaleThreshold() time.Duration {
	return s.collectorHealth.staleThreshold
}

func (s *Supervisor) GetCollectorTimeout() time.Duration {
	return s.collectorHealth.timeout
}

func (s *Supervisor) GetMaxRestartAttempts() int {
	return s.collectorHealth.maxRestartAttempts
}

func (s *Supervisor) GetRestartCount() int {
	return s.collectorHealth.restartCount
}

func (s *Supervisor) SetRestartCount(count int) {
	s.collectorHealth.restartCount = count
}

func (s *Supervisor) RestartCollector(ctx context.Context) error {
	// I1 & I4: This should never be called with restartCount >= maxRestartAttempts.
	// If it is, that's a programming error (bug in tick() logic).
	if s.collectorHealth.restartCount >= s.collectorHealth.maxRestartAttempts {
		panic(fmt.Sprintf("supervisor bug: RestartCollector called with restartCount=%d >= maxRestartAttempts=%d (should have escalated to shutdown)",
			s.collectorHealth.restartCount, s.collectorHealth.maxRestartAttempts))
	}

	s.collectorHealth.restartCount++
	s.collectorHealth.lastRestart = time.Now()

	backoff := time.Duration(s.collectorHealth.restartCount*2) * time.Second
	s.logger.Warnf("Restarting collector (attempt %d/%d) after %v backoff",
		s.collectorHealth.restartCount, s.collectorHealth.maxRestartAttempts, backoff)

	time.Sleep(backoff)

	s.collector.Restart()

	return nil
}

func (s *Supervisor) CheckDataFreshness(snapshot *fsmv2.Snapshot) bool {
	age := time.Since(snapshot.Observed.GetTimestamp())

	// GRACEFUL DEGRADATION (Invariant I15): Tight timeouts cause pausing, not crashes
	// If timeouts are configured too tight (e.g., ObservationTimeout < actual operation time),
	// the FSM will pause frequently but won't crash. Warning logs help diagnose misconfigurations.
	//
	// Design choice: Pause > Crash
	// - Pausing allows manual intervention and configuration fixes
	// - Crashing would require restart and potentially lose state

	if s.freshnessChecker.IsTimeout(snapshot) {
		s.logger.Warnf("Data timeout: observation is %v old (threshold: %v)", age, s.collectorHealth.timeout)

		return false
	}

	if !s.freshnessChecker.Check(snapshot) {
		s.logger.Warnf("Data stale: observation is %v old (threshold: %v)", age, s.collectorHealth.staleThreshold)

		return false
	}

	return true
}

// Start starts the supervisor goroutines.
// Returns a channel that will be closed when the supervisor stops.
func (s *Supervisor) Start(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})

	// Start observation collector
	if err := s.collector.Start(ctx); err != nil {
		s.logger.Errorf("Failed to start collector: %v", err)
	}

	// Start main tick loop
	go func() {
		defer close(done)

		s.tickLoop(ctx)
	}()

	return done
}

// tickLoop is the main FSM loop.
// Calls state.Next() and executes actions.
func (s *Supervisor) tickLoop(ctx context.Context) {
	s.logger.Infof("Starting tick loop for worker %s", s.identity.ID)

	ticker := time.NewTicker(s.tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Infof("Tick loop stopped for worker %s", s.identity.ID)

			return
		case <-ticker.C:
			if err := s.tick(ctx); err != nil {
				s.logger.Errorf("Tick error: %v", err)
			}
		}
	}
}

// tick performs one FSM tick.
func (s *Supervisor) tick(ctx context.Context) error {
	// Load latest snapshot from database
	// TODO: Extract workerType from identity or config
	snapshot, err := s.store.LoadSnapshot(ctx, "container", s.identity.ID)
	if err != nil {
		return fmt.Errorf("failed to load snapshot: %w", err)
	}

	// I3: Check data freshness BEFORE calling state.Next()
	// This is the trust boundary: states assume data is always fresh
	if !s.CheckDataFreshness(snapshot) {
		if s.freshnessChecker.IsTimeout(snapshot) {
			// I4: Check if we've exhausted restart attempts
			if s.collectorHealth.restartCount >= s.collectorHealth.maxRestartAttempts {
				// Max attempts reached - escalate to shutdown (Layer 3)
				s.logger.Errorf("Collector unresponsive after %d restart attempts", s.collectorHealth.maxRestartAttempts)

				if shutdownErr := s.RequestShutdown(ctx,
					fmt.Sprintf("collector unresponsive after %d restart attempts", s.collectorHealth.maxRestartAttempts)); shutdownErr != nil {
					s.logger.Errorf("Failed to request shutdown: %v", shutdownErr)
				}

				return errors.New("collector unresponsive, shutdown requested")
			}

			// I4: Safe to restart (restartCount < maxRestartAttempts)
			// RestartCollector will panic if invariant violated (defensive check)
			if err := s.RestartCollector(ctx); err != nil {
				return fmt.Errorf("failed to restart collector: %w", err)
			}
		}

		s.logger.Debug("Pausing FSM due to stale/timeout data")

		return nil
	}

	if s.collectorHealth.restartCount > 0 {
		s.logger.Infof("Collector recovered after %d restart attempts", s.collectorHealth.restartCount)
		s.collectorHealth.restartCount = 0
	}

	// Data is fresh - safe to progress FSM
	// Call state transition
	nextState, signal, action := s.currentState.Next(*snapshot)

	// VALIDATION: Cannot switch state AND emit action simultaneously
	if nextState != s.currentState && action != nil {
		panic(fmt.Sprintf("invalid state transition: state %s tried to switch to %s AND emit action %s",
			s.currentState.String(), nextState.String(), action.Name()))
	}

	// Execute action if present
	if action != nil {
		s.logger.Infof("Executing action: %s", action.Name())

		if err := s.executeActionWithRetry(ctx, action); err != nil {
			return fmt.Errorf("action execution failed: %w", err)
		}
	}

	// Transition to next state
	if nextState != s.currentState {
		s.logger.Infof("State transition: %s -> %s (reason: %s)",
			s.currentState.String(), nextState.String(), nextState.Reason())
		s.currentState = nextState
	}

	// Process signal
	if err := s.processSignal(ctx, signal); err != nil {
		return fmt.Errorf("signal processing failed: %w", err)
	}

	return nil
}

// executeActionWithRetry executes an action with exponential backoff retry.
func (s *Supervisor) executeActionWithRetry(ctx context.Context, action fsmv2.Action) error {
	maxRetries := 3
	backoff := 1 * time.Second

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			s.logger.Warnf("Retrying action %s (attempt %d/%d)", action.Name(), attempt+1, maxRetries)
			time.Sleep(backoff)
			backoff *= 2
		}

		if err := action.Execute(ctx); err != nil {
			lastErr = err
			s.logger.Errorf("Action %s failed (attempt %d/%d): %v", action.Name(), attempt+1, maxRetries, err)

			continue
		}

		// Success
		return nil
	}

	return fmt.Errorf("action %s failed after %d attempts: %w", action.Name(), maxRetries, lastErr)
}

// processSignal handles signals from states.
func (s *Supervisor) processSignal(ctx context.Context, signal fsmv2.Signal) error {
	switch signal {
	case fsmv2.SignalNone:
		// Normal operation
		return nil
	case fsmv2.SignalNeedsRemoval:
		s.logger.Infof("Worker %s requested removal", s.identity.ID)
		// TODO: Notify manager to remove this worker
		return errors.New("worker removal requested (not yet implemented)")
	case fsmv2.SignalNeedsRestart:
		s.logger.Infof("Worker %s requested restart", s.identity.ID)
		// TODO: Implement restart logic
		return errors.New("worker restart requested (not yet implemented)")
	default:
		return fmt.Errorf("unknown signal: %d", signal)
	}
}

// RequestShutdown sets the shutdown flag in desired state.
// This triggers graceful shutdown through state transitions.
// This implements Layer 3 (Graceful Shutdown) of the 4-layer defense.
func (s *Supervisor) RequestShutdown(ctx context.Context, reason string) error {
	s.logger.Warnf("Requesting shutdown for worker %s: %s", s.identity.ID, reason)

	// Load current desired state
	// TODO: Extract workerType from identity or config
	desired, err := s.store.LoadDesired(ctx, "container", s.identity.ID)
	if err != nil {
		return fmt.Errorf("failed to load desired state: %w", err)
	}

	// NOTE: Current DesiredState interface doesn't have SetShutdownRequested()
	// For this implementation, we create a shutdownDesiredState wrapper that
	// marks shutdown as requested. This is a temporary solution until the
	// DesiredState interface is extended to support mutation.
	shutdownDesired := &shutdownDesiredState{inner: desired}

	// Save updated desired state with shutdown flag set
	if err := s.store.SaveDesired(ctx, "container", s.identity.ID, shutdownDesired); err != nil {
		return fmt.Errorf("failed to save desired state: %w", err)
	}

	return nil
}

// shutdownDesiredState wraps a DesiredState and overrides ShutdownRequested to return true.
// This is a temporary wrapper until DesiredState interface supports mutation.
type shutdownDesiredState struct {
	inner fsmv2.DesiredState
}

func (s *shutdownDesiredState) ShutdownRequested() bool {
	return true
}

// GetCurrentState returns the current state name.
func (s *Supervisor) GetCurrentState() string {
	return s.currentState.String()
}

// Tick performs one FSM tick (for testing).
func (s *Supervisor) Tick(ctx context.Context) error {
	return s.tick(ctx)
}
