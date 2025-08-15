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

package benthos

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	benthosserviceconfig "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	logger "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	benthos_service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	standarderrors "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/standarderrors"
)

// The functions in this file define heavier, possibly fail-prone operations
// (for example, network or file I/O) that the Benthos FSM might need to perform.
// They are intended to be called from Reconcile.
//
// IMPORTANT:
//   - Each action is expected to be idempotent, since it may be retried
//     multiple times due to transient failures.
//   - Each action takes a context.Context and can return an error if the operation fails.
//   - If an error occurs, the Reconcile function must handle
//     setting error state and scheduling a retry/backoff.

// CreateInstance attempts to add the Benthos to the S6 manager.
func (b *BenthosInstance) CreateInstance(ctx context.Context, filesystemService filesystem.Service) error {
	b.baseFSMInstance.GetLogger().Debugf("Starting Action: Adding Benthos service %s to S6 manager ...", b.baseFSMInstance.GetID())

	err := b.service.AddBenthosToS6Manager(ctx, filesystemService, &b.config, b.baseFSMInstance.GetID())
	if err != nil {
		if errors.Is(err, benthos_service.ErrServiceAlreadyExists) {
			b.baseFSMInstance.GetLogger().Debugf("Benthos service %s already exists in S6 manager", b.baseFSMInstance.GetID())

			return nil // do not throw an error, as each action is expected to be idempotent
		}

		return fmt.Errorf("failed to add Benthos service %s to S6 manager: %w", b.baseFSMInstance.GetID(), err)
	}

	b.baseFSMInstance.GetLogger().Debugf("Benthos service %s added to S6 manager", b.baseFSMInstance.GetID())

	return nil
}

// RemoveInstance is executed while the Benthos FSM sits in the *removing*
// state.  The helper it calls (`RemoveBenthosFromS6Manager`) returns three
// kinds of answers:
//
//   - nil                    – all artefacts are gone  →  fire remove_done
//   - ErrServiceNotExist     – never created / already cleaned up
//     → success, idempotent
//   - ErrRemovalPending      – child S6-FSM is still deleting; *not* an error
//     → stay in removing and try again next tick
//   - everything else        – real failure  → bubble up so the back-off
//     decorator can suspend operations.
func (b *BenthosInstance) RemoveInstance(
	ctx context.Context,
	fs filesystem.Service,
) error {
	b.baseFSMInstance.GetLogger().
		Infof("Removing Benthos service %s from S6 manager …",
			b.baseFSMInstance.GetID())

	err := b.service.RemoveBenthosFromS6Manager(
		ctx, fs, b.baseFSMInstance.GetID())

	switch {
	// ---------------------------------------------------------------
	// happy paths
	// ---------------------------------------------------------------
	case err == nil: // fully removed
		b.baseFSMInstance.GetLogger().
			Infof("Benthos service %s removed from S6 manager",
				b.baseFSMInstance.GetID())

		return nil

	case errors.Is(err, benthos_service.ErrServiceNotExist):
		b.baseFSMInstance.GetLogger().
			Infof("Benthos service %s already removed from S6 manager",
				b.baseFSMInstance.GetID())
		// idempotent: was already gone
		return nil

	// ---------------------------------------------------------------
	// transient path – keep retrying
	// ---------------------------------------------------------------
	case errors.Is(err, standarderrors.ErrRemovalPending):
		b.baseFSMInstance.GetLogger().
			Infof("Benthos service %s removal still in progress",
				b.baseFSMInstance.GetID())
		// not an error from the FSM's perspective – just means "try again"
		return err

	// ---------------------------------------------------------------
	// real error – escalate
	// ---------------------------------------------------------------
	default:
		b.baseFSMInstance.GetLogger().
			Errorf("failed to remove service %s: %s",
				b.baseFSMInstance.GetID(), err)

		return fmt.Errorf("failed to remove service %s: %w",
			b.baseFSMInstance.GetID(), err)
	}
}

// StartInstance attempts to start the benthos by setting the desired state to running for the given instance.
func (b *BenthosInstance) StartInstance(ctx context.Context, filesystemService filesystem.Service) error {
	b.baseFSMInstance.GetLogger().Debugf("Starting Action: Starting Benthos service %s ...", b.baseFSMInstance.GetID())

	// TODO: Add pre-start validation

	// Set the desired state to running for the given instance
	err := b.service.StartBenthos(ctx, filesystemService, b.baseFSMInstance.GetID())
	if err != nil {
		// if the service is not there yet but we attempt to start it, we need to throw an error
		return fmt.Errorf("failed to start Benthos service %s: %w", b.baseFSMInstance.GetID(), err)
	}

	b.baseFSMInstance.GetLogger().Debugf("Benthos service %s start command executed", b.baseFSMInstance.GetID())

	return nil
}

// StopInstance attempts to stop the Benthos by setting the desired state to stopped for the given instance.
func (b *BenthosInstance) StopInstance(ctx context.Context, filesystemService filesystem.Service) error {
	b.baseFSMInstance.GetLogger().Debugf("Starting Action: Stopping Benthos service %s ...", b.baseFSMInstance.GetID())

	// Set the desired state to stopped for the given instance
	err := b.service.StopBenthos(ctx, filesystemService, b.baseFSMInstance.GetID())
	if err != nil {
		// if the service is not there yet but we attempt to stop it, we need to throw an error
		return fmt.Errorf("failed to stop Benthos service %s: %w", b.baseFSMInstance.GetID(), err)
	}

	b.baseFSMInstance.GetLogger().Debugf("Benthos service %s stop command executed", b.baseFSMInstance.GetID())

	return nil
}

// CheckForCreation checks if the Benthos service should be created.
func (b *BenthosInstance) CheckForCreation(ctx context.Context, filesystemService filesystem.Service) bool {
	return true
}

// getServiceStatus gets the status of the Benthos service
// its main purpose is to handle the edge cases where the service is not yet created or not yet running.
func (b *BenthosInstance) getServiceStatus(ctx context.Context, services serviceregistry.Provider, tick uint64, loopStartTime time.Time) (benthos_service.ServiceInfo, error) {
	info, err := b.service.Status(ctx, services, b.baseFSMInstance.GetID(), b.config.MetricsPort, tick, loopStartTime)
	if err != nil {
		// If there's an error getting the service status, we need to distinguish between cases
		switch {
		case errors.Is(err, benthos_service.ErrServiceNotExist):
			// If the service is being created, we don't want to count this as an error
			// The instance is likely in Creating or ToBeCreated state, so service doesn't exist yet
			// This will be handled in the reconcileStateTransition where the service gets created
			if b.baseFSMInstance.GetCurrentFSMState() == internalfsm.LifecycleStateCreating ||
				b.baseFSMInstance.GetCurrentFSMState() == internalfsm.LifecycleStateToBeCreated {
				return benthos_service.ServiceInfo{}, benthos_service.ErrServiceNotExist
			}

			// Log the warning but don't treat it as a fatal error
			b.baseFSMInstance.GetLogger().Debugf("Service not found, will be created during reconciliation")

			return benthos_service.ServiceInfo{}, nil
		case errors.Is(err, benthos_service.ErrLastObservedStateNil):
			// If the last observed state is nil, we can ignore this error
			infoWithFailedHealthChecks := info
			infoWithFailedHealthChecks.BenthosStatus.HealthCheck.IsLive = false
			infoWithFailedHealthChecks.BenthosStatus.HealthCheck.IsReady = false

			return infoWithFailedHealthChecks, nil
		case errors.Is(err, benthos_service.ErrBenthosMonitorNotRunning):
			// If the metrics service is not running, we are unable to get the logs/metrics, therefore we must return an empty status
			if !IsRunningState(b.baseFSMInstance.GetCurrentFSMState()) {
				return info, nil
			}
		}

		// For other errors, log them and return
		b.baseFSMInstance.GetLogger().Errorf("error updating observed state for %s: %s", b.baseFSMInstance.GetID(), err)

		return benthos_service.ServiceInfo{}, err
	}

	return info, nil
}

// UpdateObservedStateOfInstance updates the observed state of the service.
func (b *BenthosInstance) UpdateObservedStateOfInstance(ctx context.Context, services serviceregistry.Provider, snapshot fsm.SystemSnapshot) error {
	if ctx.Err() != nil {
		if b.baseFSMInstance.IsDeadlineExceededAndHandle(ctx.Err(), snapshot.Tick, "UpdateObservedStateOfInstance") {
			return nil
		}

		return ctx.Err()
	}

	// Use the context passed from reconcileExternalChanges, which already has proper timeout allocation
	// This context was created with CreateUpdateObservedStateContextWithMinimum to ensure
	// it has either 80% of manager time OR the minimum required time, whichever is larger

	start := time.Now()

	info, err := b.getServiceStatus(ctx, services, snapshot.Tick, snapshot.SnapshotTime)
	if err != nil {
		return err
	}

	metrics.ObserveReconcileTime(logger.ComponentBenthosInstance, b.baseFSMInstance.GetID()+".getServiceStatus", time.Since(start))
	// Store the raw service info
	b.ObservedState.ServiceInfo = info

	currentState := b.baseFSMInstance.GetCurrentFSMState()
	desiredState := b.baseFSMInstance.GetDesiredFSMState()
	// If both desired and current state are stopped, we can return immediately
	// There wont be any logs, metrics, etc. to check
	if desiredState == OperationalStateStopped && currentState == OperationalStateStopped {
		return nil
	}

	// Fetch the actual Benthos config from the service
	start = time.Now()
	observedConfig, err := b.service.GetConfig(ctx, services.GetFileSystem(), b.baseFSMInstance.GetID())
	metrics.ObserveReconcileTime(logger.ComponentBenthosInstance, b.baseFSMInstance.GetID()+".getConfig", time.Since(start))

	if err == nil {
		// Only update if we successfully got the config
		b.ObservedState.ObservedBenthosServiceConfig = observedConfig
	} else {
		if strings.Contains(err.Error(), benthos_service.ErrServiceNotExist.Error()) {
			// Log the error but don't fail - this might happen during creation when the config file doesn't exist yet
			b.baseFSMInstance.GetLogger().Debugf("Service not found, will be created during reconciliation: %v", err)

			return nil
		} else {
			return fmt.Errorf("failed to get observed Benthos config: %w", err)
		}
	}

	// Detect a config change - but let the S6 manager handle the actual reconciliation
	// Use new ConfigsEqual function that handles Benthos defaults properly
	if !benthosserviceconfig.ConfigsEqual(b.config, b.ObservedState.ObservedBenthosServiceConfig) {
		// Check if the service exists before attempting to update
		if b.service.ServiceExists(ctx, services.GetFileSystem(), b.baseFSMInstance.GetID()) {
			b.baseFSMInstance.GetLogger().Debugf("Observed Benthos config is different from desired config, updating S6 configuration")

			// Use the new ConfigDiff function for better debug output
			diffStr := benthosserviceconfig.ConfigDiff(b.config, b.ObservedState.ObservedBenthosServiceConfig)
			b.baseFSMInstance.GetLogger().Debugf("Configuration differences: %s", diffStr)

			// Update the config in the S6 manager
			err := b.service.UpdateBenthosInS6Manager(ctx, services.GetFileSystem(), &b.config, b.baseFSMInstance.GetID())
			if err != nil {
				return fmt.Errorf("failed to update Benthos service configuration: %w", err)
			}
		} else {
			b.baseFSMInstance.GetLogger().Debugf("Config differences detected but service does not exist yet, skipping update")
		}
	}

	return nil
}

// IsBenthosS6Running reports true when the FSM state is
// s6fsm.OperationalStateRunning.
//
// It returns:
//
//	ok     – true when running, false otherwise.
//	reason – empty when ok is true; otherwise a short operator-friendly
//	         explanation.
func (b *BenthosInstance) IsBenthosS6Running() (bool, string) {
	if b.ObservedState.ServiceInfo.S6FSMState == s6fsm.OperationalStateRunning {
		return true, ""
	}

	currentState := b.ObservedState.ServiceInfo.S6FSMState
	if currentState == "" {
		currentState = "not existing"
	}

	return false, "s6 is not running, current state: " + currentState
}

// IsBenthosS6Stopped reports true when the FSM state is
// s6fsm.OperationalStateStopped.
//
// It returns:
//
//	ok     – true when stopped, false otherwise.
//	reason – empty when ok is true; otherwise a short operator-friendly
//	         explanation.
func (b *BenthosInstance) IsBenthosS6Stopped() (bool, string) {
	// Check for both explicit stopped state and empty string.
	// Empty string occurs when a service crashes (e.g., due to invalid config) and gets
	// completely removed from S6, leaving no state. This is effectively "stopped" from
	// the FSM perspective and allows proper recovery from corrupted states (see ENG-3243).
	fsmState := b.ObservedState.ServiceInfo.S6FSMState
	switch fsmState {
	case "":
		fsmState = "not existing"
	case s6fsm.OperationalStateStopped:
		return true, ""
	}

	return false, "s6 is not stopped, current state: " + fsmState
}

// IsBenthosConfigLoaded reports true once Benthos uptime is at least
// constants.BenthosTimeUntilConfigLoadedInSeconds, indicating the
// configuration was parsed without panic.
//
// It returns:
//
//	ok     – true when config is considered loaded, false otherwise.
//	reason – empty when ok is true; otherwise the current uptime versus the
//	         threshold.
func (b *BenthosInstance) IsBenthosConfigLoaded() (bool, string) {
	currentUptime := b.ObservedState.ServiceInfo.S6ObservedState.ServiceInfo.Uptime
	if currentUptime >= constants.BenthosTimeUntilConfigLoadedInSeconds {
		return true, ""
	}

	return false, fmt.Sprintf("uptime %d s (< %d s threshold)", currentUptime, constants.BenthosTimeUntilConfigLoadedInSeconds)
}

// IsBenthosHealthchecksPassed debounces Benthos liveness/readiness.
//
// Historical context:
//
//  1. **Original behaviour** – We returned *true* as soon as Benthos reported
//     `IsLive && IsReady`.  A one-tick spike of readiness was enough to push
//     the FSM from *starting_waiting_for_healthchecks* to *idle/active*, even
//     if the connection was not really up and would be marked as failed a ms later.
//
//  2. **Counter experiment** – We tried redefining “connected” with
//     connection_up > connection_failed + connection_lost
//     for input and output.  Turned out Benthos sets `IsReady` by checking
//     *those same counters*, so the experiment yielded identical results and
//     added no value.
//
//  3. **Time-based debounce (rejected)** – Holding a `time.Time` inside the
//     function and comparing with `time.Now()` fixed flapping but forced every
//     caller to take its own wall-clock sample – awkward in unit tests.
//
//  4. **Current tick-based debounce** – The function is now handed the current
//     *tick* (`currentTick uint64`).  We remember the first tick at which
//     Benthos became healthy and only report `ok=true` once
//
//     currentTick - healthChecksPassingSinceTick >= BenthosHealthCheckStableDuration
//
//     If health ever drops back to false, `healthChecksPassingSinceTick` is
//     reset and the timer restarts.
//
//     *Pros:*
//     • deterministic in tests (we control ticks)
//     • zero extra `time.Now()` calls during reconcile
//     • behaviour is still “≈ 5 s of stability” in production
//
// Return values:
//
//	ok     – true after the stable-duration requirement is met
//	reason – empty on success; otherwise why we’re still waiting or which probe
//	         failed (“healthchecks passing but not stable yet …”, or
//	         “healthchecks did not pass: live=false, ready=true”, etc.)
func (b *BenthosInstance) IsBenthosHealthchecksPassed(currentTick uint64, currentTickerTimer time.Duration) (bool, string) {
	// Check if all health checks are currently passing
	allChecksPassing := b.ObservedState.ServiceInfo.BenthosStatus.HealthCheck.IsLive &&
		b.ObservedState.ServiceInfo.BenthosStatus.HealthCheck.IsReady

	// If health checks are passing, update or initialize the timestamp
	if allChecksPassing {
		if b.healthChecksPassingSinceTick == 0 {
			b.healthChecksPassingSinceTick = currentTick
		}
	} else {
		// Reset the timestamp if any check fails
		b.healthChecksPassingSinceTick = 0

		return false, fmt.Sprintf("healthchecks did not pass: live=%t, ready=%t",
			b.ObservedState.ServiceInfo.BenthosStatus.HealthCheck.IsLive,
			b.ObservedState.ServiceInfo.BenthosStatus.HealthCheck.IsReady)
	}

	// If we have a timestamp and enough time has passed, return success
	if b.healthChecksPassingSinceTick != 0 {
		elapsedTicks := currentTick - b.healthChecksPassingSinceTick

		elapsedDurationInSeconds := float64(elapsedTicks) * currentTickerTimer.Seconds()

		if elapsedDurationInSeconds >= constants.BenthosHealthCheckStableDuration.Seconds() {
			return true, ""
		}

		percentage := math.Round((100 * elapsedDurationInSeconds) / constants.BenthosHealthCheckStableDuration.Seconds())

		return false, fmt.Sprintf("healthchecks passing but not stable yet (%d %%)",
			int(percentage))
	}

	return false, fmt.Sprintf("healthchecks not passing: live=%t, ready=%t",
		b.ObservedState.ServiceInfo.BenthosStatus.HealthCheck.IsLive,
		b.ObservedState.ServiceInfo.BenthosStatus.HealthCheck.IsReady)
}

// AnyRestartsSinceCreation determines if the Benthos service has restarted since its creation.
func (b *BenthosInstance) AnyRestartsSinceCreation() bool {
	// We can analyse the S6 ExitHistory to determine if the service has restarted in the last seconds
	// We need to check if any of the exit codes are 0 (which means a restart)
	// and if the time of the restart is within the last seconds
	if len(b.ObservedState.ServiceInfo.S6ObservedState.ServiceInfo.ExitHistory) == 0 {
		return false
	}

	return true
}

// IsBenthosRunningForSomeTimeWithoutErrors reports true when Benthos has been
// up for at least constants.BenthosTimeUntilRunningInSeconds, recent logs show
// no issues, and metrics are error‑free.
//
// It returns:
//
//	ok     – true when all conditions pass, false otherwise.
//	reason – empty when ok is true; otherwise the first detected failure.
func (b *BenthosInstance) IsBenthosRunningForSomeTimeWithoutErrors(currentTime time.Time, logWindow time.Duration) (bool, string) {
	currentUptime := b.ObservedState.ServiceInfo.S6ObservedState.ServiceInfo.Uptime
	if currentUptime < constants.BenthosTimeUntilRunningInSeconds {
		return false, fmt.Sprintf("uptime %d s (< %d s threshold)", currentUptime, constants.BenthosTimeUntilRunningInSeconds)
	}

	// Check if there are any issues in the Benthos logs
	logsFine, reason := b.IsBenthosLogsFine(currentTime, logWindow)
	if !logsFine {
		return false, reason
	}

	// Check if there are any errors in the Benthos metrics
	metricsFine, reason := b.IsBenthosMetricsErrorFree()
	if !metricsFine {
		return false, reason
	}

	return true, ""
}

// IsBenthosLogsFine reports true when no suspicious entries are found in the
// recent log window.
//
// It returns:
//
//	ok     – true when logs look clean, false otherwise.
//	reason – empty when ok is true; otherwise the first offending log line.
func (b *BenthosInstance) IsBenthosLogsFine(currentTime time.Time, logWindow time.Duration) (bool, string) {
	logsFine, logEntry := b.service.IsLogsFine(b.ObservedState.ServiceInfo.BenthosStatus.BenthosLogs, currentTime, logWindow)
	if !logsFine {
		return false, fmt.Sprintf("log issue: [%s] %s", logEntry.Timestamp.Format(time.RFC3339), logEntry.Content)
	}

	return true, ""
}

// IsBenthosMetricsErrorFree wraps service.IsMetricsErrorFree.
//
// It returns:
//
//	ok     – true when metrics contain no errors, false otherwise.
//	reason – empty when ok is true; otherwise a service‑provided explanation.
func (b *BenthosInstance) IsBenthosMetricsErrorFree() (bool, string) {
	return b.service.IsMetricsErrorFree(b.ObservedState.ServiceInfo.BenthosStatus.BenthosMetrics)
}

// IsBenthosDegraded reports true when a previously healthy instance has
// degraded (i.e. any of the startup predicates now fail).
//
// It returns:
//
//	degraded – true when degraded, false when still healthy.
//	reason   – empty when degraded is false; otherwise the first failure cause.
func (b *BenthosInstance) IsBenthosDegraded(currentTime time.Time, logWindow time.Duration, currentTick uint64, tickerTime time.Duration) (bool, string) {
	// Same order as during starting phase
	running, reason := b.IsBenthosS6Running()
	if !running {
		return true, reason
	}

	loaded, reason := b.IsBenthosConfigLoaded()
	if !loaded {
		return true, reason
	}

	logsFine, reason := b.IsBenthosLogsFine(currentTime, logWindow)
	if !logsFine {
		return true, reason
	}

	healthy, reason := b.IsBenthosHealthchecksPassed(currentTick, tickerTime)
	if !healthy {
		return true, reason
	}

	return false, ""
}

// IsBenthosWithProcessingActivity reports true when metrics (or other signals)
// show that the instance is actively processing data.
//
// It returns:
//
//	ok     – true when processing activity is detected, false otherwise.
//	reason – empty when ok is true; otherwise a service‑provided explanation.
func (b *BenthosInstance) IsBenthosWithProcessingActivity(tickerTime time.Duration) (bool, string) {
	hasActivity, reason := b.service.HasProcessingActivity(b.ObservedState.ServiceInfo.BenthosStatus, tickerTime)
	if !hasActivity {
		return false, reason
	}

	return true, ""
}
