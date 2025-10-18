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
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/persistence"
)

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
	worker          fsmv2.Worker          // Worker implementation
	identity        fsmv2.Identity        // Worker's immutable identity
	store           persistence.Store     // State persistence layer
	currentState    fsmv2.State           // Current FSM state
	logger          *zap.SugaredLogger    // Logger for supervisor operations
	tickInterval    time.Duration         // How often to evaluate state transitions
	collectorHealth CollectorHealth       // Collector health tracking
}

// CollectorHealthConfig configures observation collector health monitoring.
// The collector runs in a separate goroutine and may fail due to network issues,
// blocked operations, or infrastructure problems. These settings control when to
// pause the FSM (stale data) and when to restart the collector (timeout).
type CollectorHealthConfig struct {
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

	return &Supervisor{
		worker:       cfg.Worker,
		identity:     cfg.Identity,
		store:        cfg.Store,
		currentState: cfg.Worker.GetInitialState(),
		logger:       cfg.Logger,
		tickInterval: tickInterval,
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

func (s *Supervisor) CheckDataFreshness(snapshot *fsmv2.Snapshot) bool {
	age := time.Since(snapshot.Observed.GetTimestamp())

	if age > s.collectorHealth.timeout {
		s.logger.Warnf("Data timeout: observation is %v old (threshold: %v)", age, s.collectorHealth.timeout)
		return false
	}

	if age > s.collectorHealth.staleThreshold {
		s.logger.Warnf("Data stale: observation is %v old (threshold: %v)", age, s.collectorHealth.staleThreshold)
		return false
	}

	return true
}

// Start starts the supervisor goroutines.
// Returns a channel that will be closed when the supervisor stops.
func (s *Supervisor) Start(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})

	// Start observation loop
	go s.observationLoop(ctx)

	// Start main tick loop
	go func() {
		defer close(done)
		s.tickLoop(ctx)
	}()

	return done
}

func (s *Supervisor) observationLoop(ctx context.Context) {
	s.logger.Infof("Starting observation loop for worker %s", s.identity.ID)

	ticker := time.NewTicker(DefaultObservationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Infof("Observation loop stopped for worker %s", s.identity.ID)
			return
		case <-ticker.C:
			if err := s.collectAndSaveObservedState(ctx); err != nil {
				s.logger.Errorf("Failed to collect observed state: %v", err)
				// Continue anyway - errors are expected (network issues, etc.)
			}
		}
	}
}

// collectAndSaveObservedState calls the worker and saves to database.
func (s *Supervisor) collectAndSaveObservedState(ctx context.Context) error {
	// Collect from worker
	observed, err := s.worker.CollectObservedState(ctx)
	if err != nil {
		return fmt.Errorf("worker.CollectObservedState failed: %w", err)
	}

	// Save to store
	// TODO: Extract workerType from identity or config
	if err := s.store.SaveObserved(ctx, "container", s.identity.ID, observed); err != nil {
		return fmt.Errorf("store.SaveObserved failed: %w", err)
	}

	return nil
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

	// Check data freshness BEFORE calling state.Next()
	// This is the trust boundary: states assume data is always fresh
	if !s.CheckDataFreshness(snapshot) {
		// Data is stale or timeout - pause FSM
		s.logger.Debug("Pausing FSM due to stale/timeout data")
		return nil
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
		return fmt.Errorf("worker removal requested (not yet implemented)")
	case fsmv2.SignalNeedsRestart:
		s.logger.Infof("Worker %s requested restart", s.identity.ID)
		// TODO: Implement restart logic
		return fmt.Errorf("worker restart requested (not yet implemented)")
	default:
		return fmt.Errorf("unknown signal: %d", signal)
	}
}

// RequestShutdown sets the shutdown flag in desired state.
// This triggers graceful shutdown through state transitions.
func (s *Supervisor) RequestShutdown(ctx context.Context) error {
	// Load current desired state
	// TODO: Extract workerType from identity or config
	desired, err := s.store.LoadDesired(ctx, "container", s.identity.ID)
	if err != nil {
		return fmt.Errorf("failed to load desired state: %w", err)
	}

	// Set shutdown flag
	// TODO: This requires a SetShutdownRequested method on DesiredState
	// For now, this is a placeholder
	_ = desired

	// Save updated desired state
	// if err := s.store.SaveDesired(ctx, "container", s.identity.ID, desired); err != nil {
	// 	return fmt.Errorf("failed to save desired state: %w", err)
	// }

	s.logger.Infof("Shutdown requested for worker %s", s.identity.ID)
	return nil
}

// GetCurrentState returns the current state name.
func (s *Supervisor) GetCurrentState() string {
	return s.currentState.String()
}

// Tick performs one FSM tick (for testing).
func (s *Supervisor) Tick(ctx context.Context) error {
	return s.tick(ctx)
}
