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
)

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
