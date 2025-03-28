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

package agent

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	agent_service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/agent"
)

// The functions in this file define heavier, potentially fail-prone operations
// that the Agent FSM might need to perform. They are intended to be called from Reconcile.
//
// Each action is expected to be idempotent, since it may be retried multiple times.
// Each action takes a context.Context and can return an error if the operation fails.

// initiateAgentActivation attempts to activate the agent monitoring.
func (a *AgentInstance) initiateAgentActivation(ctx context.Context) error {
	a.baseFSMInstance.GetLogger().Debugf("Starting Action: Activating agent monitoring...")

	// Initialize the agent with current configuration
	err := a.service.Initialize(ctx, a.config)
	if err != nil {
		return fmt.Errorf("failed to initialize agent: %w", err)
	}

	a.baseFSMInstance.GetLogger().Debugf("Agent monitoring activated successfully")
	return nil
}

// initiateAgentMonitoringStopped attempts to stop agent monitoring.
// In a real implementation, this would include cleanup and stopping resources.
func (a *AgentInstance) initiateAgentMonitoringStopped(ctx context.Context) error {
	a.baseFSMInstance.GetLogger().Debugf("Starting Action: Stopping agent monitoring...")

	// In a real implementation, you would call service methods to stop resources
	// For now, we'll just log it
	a.baseFSMInstance.GetLogger().Debugf("Agent monitoring stopped successfully")
	return nil
}

// checkAgentHealth verifies if the agent is healthy or degraded
func (a *AgentInstance) checkAgentHealth(ctx context.Context) (healthy bool, err error) {
	a.baseFSMInstance.GetLogger().Debugf("Checking agent health...")

	// In a real implementation, we would perform actual health checks
	// For now, we'll simulate based on the observed state
	if a.ObservedState.AgentInfo.Status == agent_service.AgentStatusError {
		return false, nil
	}

	return true, nil
}

// checkAndUpdateConfig checks for updated configuration and applies any changes
// to the agent service. This ensures the agent is always running with the latest config.
func (a *AgentInstance) checkAndUpdateConfig(ctx context.Context, tick uint64) error {
	a.baseFSMInstance.GetLogger().Debugf("Checking for configuration updates...")

	// In a real implementation, we would use the ConfigManager instead of loading directly from file
	// The ConfigManager is typically passed to the FSM when it's created
	if a.configManager == nil {
		// If no config manager is available, we can't check for updates
		a.baseFSMInstance.GetLogger().Warnf("No config manager available, skipping config update check")
		return nil
	}

	// Get the latest config from the manager
	fullConfig, err := a.configManager.GetConfig(ctx, tick)
	if err != nil {
		return fmt.Errorf("failed to get latest config: %w", err)
	}

	// Extract the agent config
	latestConfig := fullConfig.Agent

	// Check if the config has changed
	if !reflect.DeepEqual(a.config, latestConfig) {
		a.baseFSMInstance.GetLogger().Infof("Detected configuration change, updating agent...")

		// Update our stored config
		a.config = latestConfig

		// Update location if changed
		if !reflect.DeepEqual(a.ObservedState.AgentInfo.Location, latestConfig.Location) {
			if err := a.service.UpdateLocation(ctx, latestConfig.Location); err != nil {
				return fmt.Errorf("failed to update agent location: %w", err)
			}
		}

		// Update release channel if changed
		if a.ObservedState.AgentInfo.ReleaseChannel != latestConfig.ReleaseChannel {
			if err := a.service.UpdateReleaseChannel(ctx, latestConfig.ReleaseChannel); err != nil {
				return fmt.Errorf("failed to update agent release channel: %w", err)
			}
		}

		// If more substantial changes that require reinitialization, we could set a flag
		// to trigger a restart of the agent in the next reconcile cycle
	}

	return nil
}

// updateAgentObservedState updates the observed state of the agent by querying the service.
func (a *AgentInstance) updateAgentObservedState(ctx context.Context, tick uint64) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(ComponentAgentInstance, "updateAgentObservedState", time.Since(start))
	}()

	// First, check for configuration updates
	if err := a.checkAndUpdateConfig(ctx, tick); err != nil {
		// We now propagate config errors upward to trigger degraded state
		a.baseFSMInstance.GetLogger().Warnf("Failed to check for config updates: %v", err)
		return fmt.Errorf("configuration update failed: %w", err)
	}

	// Get the current agent status
	agentInfo, err := a.service.GetStatus(ctx)
	if err != nil {
		if err == agent_service.ErrAgentNotInitialized {
			// If agent is not initialized, this is expected in certain states
			a.baseFSMInstance.GetLogger().Debugf("Agent not initialized, will be initialized during reconciliation")
			return nil
		}
		return fmt.Errorf("failed to get agent status: %w", err)
	}

	// Update the observed state
	a.ObservedState.AgentInfo = agentInfo
	a.ObservedState.LastUpdatedAt = time.Now()

	return nil
}

// isAgentActive determines if the agent is active based on observed state.
func (a *AgentInstance) isAgentActive() bool {
	return a.ObservedState.AgentInfo.Status == agent_service.AgentStatusRunning
}

// isAgentMonitoringStopped determines if the agent monitoring is stopped based on observed state.
func (a *AgentInstance) isAgentMonitoringStopped() bool {
	return a.ObservedState.AgentInfo.Status == agent_service.AgentStatusStopped
}

// isAgentDegraded determines if the agent is in a degraded state based on observed state.
func (a *AgentInstance) isAgentDegraded() bool {
	return a.ObservedState.AgentInfo.Status == agent_service.AgentStatusError
}
