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

package core_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/core"
)

func TestCoreStateMachine(t *testing.T) {
	// Create a new core instance
	instance := core.NewCoreInstance("test-core", "TestComponent")
	assert.NotNil(t, instance, "Instance should not be nil")

	// Check initial state
	assert.Equal(t, core.OperationalStateMonitoringStopped, instance.GetCurrentFSMState())
	assert.Equal(t, core.OperationalStateMonitoringStopped, instance.GetDesiredFSMState())

	// Set desired state to Active
	err := instance.SetDesiredFSMState(core.OperationalStateActive)
	assert.NoError(t, err, "Setting desired state to active should not error")

	// Reconcile to move toward desired state
	ctx := context.Background()
	err, changed := instance.Reconcile(ctx, 1)
	assert.NoError(t, err, "Reconcile should not error")
	assert.True(t, changed, "State should have changed")

	// Check that we've moved to active state
	assert.Equal(t, core.OperationalStateActive, instance.GetCurrentFSMState())

	// Update error count to trigger degraded state
	for i := 0; i < 6; i++ {
		instance.UpdateErrorStatus("Test error")
	}

	// Reconcile again to detect degraded state
	err, changed = instance.Reconcile(ctx, 2)
	assert.NoError(t, err, "Reconcile should not error")
	assert.True(t, changed, "State should have changed")

	// Check that we've moved to degraded state
	assert.Equal(t, core.OperationalStateDegraded, instance.GetCurrentFSMState())

	// Reset error count to recover
	instance.ResetErrorStatus()

	// Reconcile again to recover from degraded state
	err, changed = instance.Reconcile(ctx, 3)
	assert.NoError(t, err, "Reconcile should not error")
	assert.True(t, changed, "State should have changed")

	// Check that we've recovered to active state
	assert.Equal(t, core.OperationalStateActive, instance.GetCurrentFSMState())

	// Set desired state back to stopped
	err = instance.SetDesiredFSMState(core.OperationalStateMonitoringStopped)
	assert.NoError(t, err, "Setting desired state to stopped should not error")

	// Reconcile to stop
	err, changed = instance.Reconcile(ctx, 4)
	assert.NoError(t, err, "Reconcile should not error")
	assert.True(t, changed, "State should have changed")

	// Check that we've moved back to stopped state
	assert.Equal(t, core.OperationalStateMonitoringStopped, instance.GetCurrentFSMState())

	// Test removal
	err = instance.Remove(ctx)
	assert.NoError(t, err, "Remove should not error")

	// Reconcile to process removal
	err, changed = instance.Reconcile(ctx, 5)
	assert.NoError(t, err, "Reconcile should not error")
	assert.True(t, changed, "State should have changed")

	// Check that instance is removed
	assert.True(t, instance.IsRemoved(), "Instance should be removed")
}
