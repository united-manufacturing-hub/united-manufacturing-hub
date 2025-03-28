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

package core

import (
	"context"
	"fmt"
	"time"
)

// UpdateErrorStatus updates the error status of the core instance
// This can be used to track error conditions that might lead to a degraded state
func (c *CoreInstance) UpdateErrorStatus(errorMsg string) {
	c.ObservedState.ErrorCount++
	c.ObservedState.LastErrorTime = time.Now().Unix()
	c.baseFSMInstance.GetLogger().Warnf("Error in core instance %s: %s (total errors: %d)",
		c.baseFSMInstance.GetID(), errorMsg, c.ObservedState.ErrorCount)
}

// ResetErrorStatus resets the error status of the core instance
// This can be used to clear error conditions after recovery
func (c *CoreInstance) ResetErrorStatus() {
	c.ObservedState.ErrorCount = 0
	c.baseFSMInstance.GetLogger().Infof("Reset error status for core instance %s",
		c.baseFSMInstance.GetID())
}

// increaseMonitoringRate increases the monitoring rate for the core instance
// This can be used when the instance is in a degraded state to gather more information
func (c *CoreInstance) increaseMonitoringRate(ctx context.Context) error {
	// Implementation would depend on the specific monitoring mechanism
	c.baseFSMInstance.GetLogger().Infof("Increasing monitoring rate for %s due to degraded state",
		c.baseFSMInstance.GetID())

	// This function could adjust polling intervals, increase log verbosity, etc.
	return nil
}

// decreaseMonitoringRate decreases the monitoring rate to normal levels
// This can be used when returning to a normal active state from degraded
func (c *CoreInstance) decreaseMonitoringRate(ctx context.Context) error {
	// Implementation would depend on the specific monitoring mechanism
	c.baseFSMInstance.GetLogger().Infof("Decreasing monitoring rate for %s back to normal levels",
		c.baseFSMInstance.GetID())

	// This function could reset polling intervals, decrease log verbosity, etc.
	return nil
}

// performHealthCheck performs a health check on the monitored component
// Returns an error if the component is unhealthy
func (c *CoreInstance) performHealthCheck(ctx context.Context) error {
	// Implementation would depend on what is being monitored
	c.baseFSMInstance.GetLogger().Debugf("Performing health check for component %s",
		c.componentName)

	// Mock health check logic - in a real implementation, this would check the actual component health
	if c.ObservedState.ErrorCount > 10 {
		return fmt.Errorf("component is unhealthy: too many errors (%d)", c.ObservedState.ErrorCount)
	}

	return nil
}
