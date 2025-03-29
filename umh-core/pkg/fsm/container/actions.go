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

package container

import (
	"context"
)

// In Benthos, actions.go contained idempotent operations (like starting/stopping a service).
// For the container monitor, we technically don't "start/stop" the container itself—we're only
// enabling or disabling the monitoring. We'll keep placeholder actions for consistency.

// initiateContainerCreate is called when the FSM transitions from to_be_created -> creating.
// For container monitoring, this is a no-op as there's no actual container to create.
// This function is present for structural consistency with other FSM packages.
func (c *ContainerInstance) initiateContainerCreate(ctx context.Context) error {
	c.baseFSMInstance.GetLogger().Debugf("Creating container monitor instance %s (no-op)", c.baseFSMInstance.GetID())
	return nil
}

// initiateContainerRemove is called when the FSM transitions to removing.
// For container monitoring, this is a no-op as we don't need to remove any resources.
// This function is present for structural consistency with other FSM packages.
func (c *ContainerInstance) initiateContainerRemove(ctx context.Context) error {
	c.baseFSMInstance.GetLogger().Debugf("Removing container monitor instance %s (no-op)", c.baseFSMInstance.GetID())
	return nil
}

// optionally, we might have something like "enableMonitoring" / "disableMonitoring" if
// you want actual side effects. For now, do no-ops or just logs.

// enableMonitoring is called when the container monitoring should be enabled.
// Currently this is a no-op as the monitoring service runs independently.
func (c *ContainerInstance) enableMonitoring(ctx context.Context) error {
	c.baseFSMInstance.GetLogger().Infof("Enabling monitoring for %s (no-op)", c.baseFSMInstance.GetID())
	return nil
}

// disableMonitoring is called when the container monitoring should be disabled.
// Currently this is a no-op as the monitoring service runs independently.
func (c *ContainerInstance) disableMonitoring(ctx context.Context) error {
	c.baseFSMInstance.GetLogger().Infof("Disabling monitoring for %s (no-op)", c.baseFSMInstance.GetID())
	return nil
}
