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
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	agentservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/agent"
)

// The functions in this file define heavier, possibly fail-prone operations
// (for example, network or file I/O) that the Agent FSM might need to perform.
// They are intended to be called from Reconcile.
//
// IMPORTANT:
//   - Each action is expected to be idempotent, since it may be retried
//     multiple times due to transient failures.
//   - Each action takes a context.Context and can return an error if the operation fails.
//   - If an error occurs, the Reconcile function must handle
//     setting AgentInstance.lastError and scheduling a retry/backoff.

// UpdateErrorStatus updates the error status of the agent instance
// This can be used to track error conditions that might lead to a degraded state
func (a *AgentInstance) UpdateErrorStatus(errorMsg string) {
	a.ObservedState.ErrorCount++
	a.ObservedState.LastErrorTime = time.Now().Unix()
	a.baseFSMInstance.GetLogger().Warnf("Error in agent instance %s: %s (total errors: %d)",
		a.baseFSMInstance.GetID(), errorMsg, a.ObservedState.ErrorCount)
}

// ResetErrorStatus resets the error status of the agent instance
// This can be used to clear error conditions after recovery
func (a *AgentInstance) ResetErrorStatus() {
	a.ObservedState.ErrorCount = 0
	a.baseFSMInstance.GetLogger().Infof("Reset error status for agent instance %s",
		a.baseFSMInstance.GetID())
}

// recordHeartbeat updates the last heartbeat timestamp for the agent
func (a *AgentInstance) recordHeartbeat() {
	a.ObservedState.LastHeartbeat = time.Now().Unix()
	a.ObservedState.IsConnected = true
}

// startMonitoring starts the agent monitoring process
func (a *AgentInstance) startMonitoring() error {
	// Implement actual monitoring logic
	// This might involve:
	// - Setting up connections to the agent
	// - Starting heartbeat checks
	// - Initializing any agent-specific monitoring

	a.baseFSMInstance.GetLogger().Infof("Starting monitoring for agent %s (type: %s)",
		a.baseFSMInstance.GetID(), a.agentType)

	// Record initial heartbeat
	a.recordHeartbeat()
	a.ObservedState.IsMonitoring = true

	// In a real implementation, you might:
	// - Start a goroutine for continuous monitoring
	// - Establish connection to the agent
	// - Register for events or metrics from the agent

	return nil
}

// stopMonitoring stops the agent monitoring process
func (a *AgentInstance) stopMonitoring() error {
	// Implement actual monitoring termination logic
	// This might involve:
	// - Closing connections to the agent
	// - Stopping heartbeat checks
	// - Cleaning up any agent-specific monitoring resources

	a.baseFSMInstance.GetLogger().Infof("Stopping monitoring for agent %s (type: %s)",
		a.baseFSMInstance.GetID(), a.agentType)

	a.ObservedState.IsMonitoring = false
	a.ObservedState.IsConnected = false

	// In a real implementation, you might:
	// - Stop monitoring goroutines
	// - Close connections
	// - Unregister from events or metrics

	return nil
}

// checkAgentConnection verifies that the agent is still connected
// Returns an error if the agent is not connected
func (a *AgentInstance) checkAgentConnection(ctx context.Context) error {
	// Implement actual connection check logic
	// This might involve:
	// - Sending a ping to the agent
	// - Checking last heartbeat timestamp
	// - Verifying agent is responsive

	// For demonstration, we'll just check the last heartbeat time
	now := time.Now().Unix()
	heartbeatAge := now - a.ObservedState.LastHeartbeat

	if heartbeatAge > 60 { // More than 60 seconds since last heartbeat
		a.ObservedState.IsConnected = false
		return fmt.Errorf("agent %s has not sent a heartbeat in %d seconds",
			a.baseFSMInstance.GetID(), heartbeatAge)
	}

	// If this was a successful check, update status
	a.ObservedState.IsConnected = true
	return nil
}

// syncWithParent synchronizes agent state with parent core state
// This is typically called during reconciliation to ensure agent state
// is consistent with parent state
func (a *AgentInstance) syncWithParent(ctx context.Context) error {
	if a.parentCore == nil {
		return fmt.Errorf("parent core is nil")
	}

	parentState := a.parentCore.GetCurrentFSMState()
	a.baseFSMInstance.GetLogger().Debugf("Syncing agent %s with parent core (parent state: %s)",
		a.baseFSMInstance.GetID(), parentState)

	// Implement sync logic based on parent state
	// In a real implementation, this might:
	// - Update agent configuration based on parent settings
	// - Trigger specific actions based on parent state
	// - Perform cleanup if parent is being removed

	return nil
}

// initiateAgentStart attempts to start monitoring the agent.
func (a *AgentInstance) initiateAgentStart(ctx context.Context) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentAgentInstance, a.baseFSMInstance.GetID()+".initiateAgentStart", time.Since(start))
	}()

	a.baseFSMInstance.GetLogger().Debugf("Starting Action: Starting agent monitoring for %s ...", a.baseFSMInstance.GetID())

	err := a.service.Start(ctx, a.agentID)
	if err != nil {
		return fmt.Errorf("failed to start agent monitoring for %s: %w", a.baseFSMInstance.GetID(), err)
	}

	a.baseFSMInstance.GetLogger().Debugf("Agent %s monitoring started", a.baseFSMInstance.GetID())
	return nil
}

// initiateAgentStop attempts to stop monitoring the agent.
func (a *AgentInstance) initiateAgentStop(ctx context.Context) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentAgentInstance, a.baseFSMInstance.GetID()+".initiateAgentStop", time.Since(start))
	}()

	a.baseFSMInstance.GetLogger().Debugf("Starting Action: Stopping agent monitoring for %s ...", a.baseFSMInstance.GetID())

	err := a.service.Stop(ctx, a.agentID)
	if err != nil {
		return fmt.Errorf("failed to stop agent monitoring for %s: %w", a.baseFSMInstance.GetID(), err)
	}

	a.baseFSMInstance.GetLogger().Debugf("Agent %s monitoring stopped", a.baseFSMInstance.GetID())
	return nil
}

// initiateAgentRemove attempts to remove the agent from monitoring.
func (a *AgentInstance) initiateAgentRemove(ctx context.Context) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentAgentInstance, a.baseFSMInstance.GetID()+".initiateAgentRemove", time.Since(start))
	}()

	a.baseFSMInstance.GetLogger().Debugf("Starting Action: Removing agent %s ...", a.baseFSMInstance.GetID())

	// First ensure the agent monitoring is stopped
	if a.IsActive() {
		return fmt.Errorf("agent %s cannot be removed while active", a.baseFSMInstance.GetID())
	}

	// Remove the agent
	err := a.service.Remove(ctx, a.agentID)
	if err != nil {
		return fmt.Errorf("failed to remove agent %s: %w", a.baseFSMInstance.GetID(), err)
	}

	a.baseFSMInstance.GetLogger().Debugf("Agent %s removed", a.baseFSMInstance.GetID())
	return nil
}

// updateObservedState updates the observed state of the agent
func (a *AgentInstance) updateObservedState(ctx context.Context) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentAgentInstance, a.baseFSMInstance.GetID()+".updateObservedState", time.Since(start))
	}()

	// Get agent status
	info, err := a.service.Status(ctx, a.agentID)
	if err != nil {
		a.ObservedState.IsConnected = false
		a.ObservedState.LastHeartbeat = 0

		// If agent doesn't exist, we don't want to count this as an error
		if err == agentservice.ErrAgentNotExist {
			return err
		}

		// Otherwise, log and return the error
		a.baseFSMInstance.GetLogger().Errorf("error updating observed state for %s: %s", a.baseFSMInstance.GetID(), err)
		return err
	}

	// Update observed state with latest information
	a.ObservedState.IsConnected = info.IsConnected
	a.ObservedState.LastHeartbeat = info.LastHeartbeat
	a.ObservedState.ErrorCount = info.ErrorCount
	a.ObservedState.LastErrorTime = info.LastErrorTime
	a.ObservedState.IsMonitoring = (info.Status == agentservice.AgentActive || info.Status == agentservice.AgentDegraded)

	// Update location from parent core if available
	if a.parentCore != nil {
		// Location data would come from the parent core or config in a real implementation
		// For now, we just maintain what's already there
	}

	return nil
}
