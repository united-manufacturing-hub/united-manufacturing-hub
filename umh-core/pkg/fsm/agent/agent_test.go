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

package agent_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/agent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/core"
)

func TestAgentStateMachine(t *testing.T) {
	// First, create a parent core instance
	parentCore := core.NewCoreInstance("test-core", "TestComponent")
	assert.NotNil(t, parentCore, "Parent core instance should not be nil")

	// Create an agent instance owned by the core
	agentInstance := agent.NewAgentInstance("test-agent", "TestAgentType", parentCore)
	assert.NotNil(t, agentInstance, "Agent instance should not be nil")

	// Check initial state
	assert.Equal(t, agent.OperationalStateMonitoringStopped, agentInstance.GetCurrentFSMState())
	assert.Equal(t, agent.OperationalStateMonitoringStopped, agentInstance.GetDesiredFSMState())

	// Verify parent relationship
	assert.Equal(t, parentCore, agentInstance.GetParentCore())

	ctx := context.Background()

	// Test 1: Agent can't activate if parent core is not active
	assert.Equal(t, core.OperationalStateMonitoringStopped, parentCore.GetCurrentFSMState())
	err := agentInstance.SetDesiredFSMState(agent.OperationalStateActive)
	assert.NoError(t, err, "Setting desired state to active should not error")

	// Try to reconcile agent state - should not transition to active since parent is not active
	err, changed := agentInstance.Reconcile(ctx, 1)
	assert.NoError(t, err, "Reconcile should not error")
	assert.False(t, changed, "State should not have changed") // Parent core constraints prevent state change
	assert.Equal(t, agent.OperationalStateMonitoringStopped, agentInstance.GetCurrentFSMState())

	// Test 2: Activate parent core first, then agent
	// Set parent core to active
	err = parentCore.SetDesiredFSMState(core.OperationalStateActive)
	assert.NoError(t, err, "Setting parent core desired state to active should not error")

	// Reconcile parent core to make it active
	err, changed = parentCore.Reconcile(ctx, 1)
	assert.NoError(t, err, "Parent core reconcile should not error")
	assert.True(t, changed, "Parent core state should have changed")
	assert.Equal(t, core.OperationalStateActive, parentCore.GetCurrentFSMState())

	// Now that parent is active, agent can transition to active
	err, changed = agentInstance.Reconcile(ctx, 2)
	assert.NoError(t, err, "Agent reconcile should not error")
	assert.True(t, changed, "Agent state should now change")
	assert.Equal(t, agent.OperationalStateActive, agentInstance.GetCurrentFSMState())

	// Test 3: When parent core transitions to stopped, agent should too
	// Set parent core back to monitoring_stopped
	err = parentCore.SetDesiredFSMState(core.OperationalStateMonitoringStopped)
	assert.NoError(t, err, "Setting parent core desired state to stopped should not error")

	// Reconcile parent core to stop it
	err, changed = parentCore.Reconcile(ctx, 3)
	assert.NoError(t, err, "Parent core reconcile should not error")
	assert.True(t, changed, "Parent core state should have changed")
	assert.Equal(t, core.OperationalStateMonitoringStopped, parentCore.GetCurrentFSMState())

	// Agent should transition to stopped based on parent state during reconcile
	err, changed = agentInstance.Reconcile(ctx, 4)
	assert.NoError(t, err, "Agent reconcile should not error")
	assert.True(t, changed, "Agent state should change due to parent state")
	assert.Equal(t, agent.OperationalStateMonitoringStopped, agentInstance.GetCurrentFSMState())

	// Test 4: When parent core is removed, agent should be removed too
	// Remove parent core
	err = parentCore.Remove(ctx)
	assert.NoError(t, err, "Removing parent core should not error")

	// Reconcile parent core to process removal
	err, changed = parentCore.Reconcile(ctx, 5)
	assert.NoError(t, err, "Parent core reconcile should not error")
	assert.True(t, changed, "Parent core state should have changed")
	assert.True(t, parentCore.IsRemoved(), "Parent core should be removed")

	// Agent should also be removed during reconcile
	err, changed = agentInstance.Reconcile(ctx, 6)
	assert.NoError(t, err, "Agent reconcile should not error")
	assert.True(t, changed, "Agent state should change due to parent removal")
	assert.True(t, agentInstance.IsRemoved(), "Agent should be removed")
}

func TestAgentDegradedState(t *testing.T) {
	// Create parent core and agent
	parentCore := core.NewCoreInstance("test-core-2", "TestComponent")
	agentInstance := agent.NewAgentInstance("test-agent-2", "TestAgentType", parentCore)

	ctx := context.Background()

	// Activate parent core first
	parentCore.SetDesiredFSMState(core.OperationalStateActive)
	parentCore.Reconcile(ctx, 1)
	assert.Equal(t, core.OperationalStateActive, parentCore.GetCurrentFSMState())

	// Activate agent
	agentInstance.SetDesiredFSMState(agent.OperationalStateActive)
	agentInstance.Reconcile(ctx, 2)
	assert.Equal(t, agent.OperationalStateActive, agentInstance.GetCurrentFSMState())

	// Test agent degradation based on its own conditions
	// Simulate errors to trigger degraded state
	for i := 0; i < 6; i++ {
		agentInstance.UpdateErrorStatus("Test error")
	}

	// Reconcile to detect degraded state
	err, changed := agentInstance.Reconcile(ctx, 3)
	assert.NoError(t, err, "Reconcile should not error")
	assert.True(t, changed, "State should have changed")

	// Check that we've moved to degraded state
	assert.Equal(t, agent.OperationalStateDegraded, agentInstance.GetCurrentFSMState())

	// Reset error count to recover
	agentInstance.ResetErrorStatus()
	agentInstance.ObservedState.IsConnected = true

	// Reconcile again to recover from degraded state
	err, changed = agentInstance.Reconcile(ctx, 4)
	assert.NoError(t, err, "Reconcile should not error")
	assert.True(t, changed, "State should have changed")

	// Check that we've recovered to active state
	assert.Equal(t, agent.OperationalStateActive, agentInstance.GetCurrentFSMState())

	// Test parent core degradation affecting agent
	// Degrade parent core
	for i := 0; i < 6; i++ {
		parentCore.UpdateErrorStatus("Test error")
	}
	parentCore.Reconcile(ctx, 5)
	assert.Equal(t, core.OperationalStateDegraded, parentCore.GetCurrentFSMState())

	// Reconcile agent to see parent degradation impact
	agentInstance.Reconcile(ctx, 6)

	// Agent's error count should have increased due to parent degradation
	assert.Greater(t, agentInstance.ObservedState.ErrorCount, 0)
}
