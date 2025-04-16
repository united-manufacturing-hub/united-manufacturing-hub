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

package nmap

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	nmap_service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/nmap"
)

// In Benthos, actions.go contained idempotent operations (like starting/stopping a service).
// For the nmap monitor, we technically don't "start/stop" nmap itself—we're only
// enabling or disabling the monitoring. We'll keep placeholder actions for consistency.

// CreateInstance is called when the FSM transitions from to_be_created -> creating.
func (n *NmapInstance) CreateInstance(ctx context.Context, filesystemService filesystem.Service) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentNmapInstance, n.baseFSMInstance.GetID()+".initiateS6Create", time.Since(start))
	}()

	n.baseFSMInstance.GetLogger().Debugf("Starting Action: Adding Benthos service %s to S6 manager ...", n.baseFSMInstance.GetID())

	// Check if we have a config with command or other settings
	configEmpty := n.config.NmapServiceConfig.Port == 0 && n.config.NmapServiceConfig.Target == ""

	if !configEmpty {
		// Create service with custom configuration
		err := n.monitorService.AddNmapToS6Manager(ctx, &n.config.NmapServiceConfig, n.baseFSMInstance.GetID())
		if err != nil {
			if err == nmap_service.ErrServiceAlreadyExists {
				n.baseFSMInstance.GetLogger().Debugf("Nmap service %s already exists in S6 manager", n.baseFSMInstance.GetID())
				return nil // do not throw an error, as each action is expected to be idempotent
			}
			return fmt.Errorf("failed to create service with config for %s: %w", n.baseFSMInstance.GetID(), err)
		}

	}

	n.baseFSMInstance.GetLogger().Debugf("S6 service %s directory structure created", n.baseFSMInstance.GetID())
	return nil
}

// RemoveInstance attempts to remove the Nmap service.
// It requires the service to be stopped before removal.
func (n *NmapInstance) RemoveInstance(ctx context.Context, filesystemService filesystem.Service) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentNmapInstance, n.baseFSMInstance.GetID()+".initiateS6Remove", time.Since(start))
	}()

	n.baseFSMInstance.GetLogger().Debugf("Starting Action: Removing S6 service %s ...", n.baseFSMInstance.GetID())

	// Initiate the removal cycle
	err := n.monitorService.RemoveNmapFromS6Manager(ctx, n.baseFSMInstance.GetID())
	if err != nil {
		if err == nmap_service.ErrServiceNotExist {
			n.baseFSMInstance.GetLogger().Debugf("Nmap service %s not found in S6 manager", n.baseFSMInstance.GetID())
			return nil // do not throw an error, as each action is expected to be idempotent
		}
		return fmt.Errorf("failed to remove service for %s: %w", n.baseFSMInstance.GetID(), err)
	}

	n.baseFSMInstance.GetLogger().Debugf("S6 service %s removed", n.baseFSMInstance.GetID())
	return nil
}

// StartInstance attempts to start the Nmap by setting the desired state to running for the given instance
func (n *NmapInstance) StartInstance(ctx context.Context, filesystemService filesystem.Service) error {
	n.baseFSMInstance.GetLogger().Infof("Enabling monitoring for %s (no-op)", n.baseFSMInstance.GetID())

	err := n.monitorService.StartNmap(ctx, n.baseFSMInstance.GetID())
	if err != nil {
		// if the service is not there yet but we attempt to start it, we need to throw an error
		return fmt.Errorf("failed to start Nmap service %s: %w", n.baseFSMInstance.GetID(), err)
	}

	n.baseFSMInstance.GetLogger().Debugf("Nmap service %s start command executed", n.baseFSMInstance.GetID())
	return nil
}

// StopInstance attempts to stop the Nmap by setting the desired state to stopped for the given instance
func (n *NmapInstance) StopInstance(ctx context.Context, filesystemService filesystem.Service) error {
	n.baseFSMInstance.GetLogger().Infof("Disabling monitoring for %s (no-op)", n.baseFSMInstance.GetID())
	// Set the desired state to stopped for the given instance
	err := n.monitorService.StopNmap(ctx, n.baseFSMInstance.GetID())
	if err != nil {
		// if the service is not there yet but we attempt to stop it, we need to throw an error
		return fmt.Errorf("failed to stop Nmap service %s: %w", n.baseFSMInstance.GetID(), err)
	}

	n.baseFSMInstance.GetLogger().Debugf("Nmap service %s stop command executed", n.baseFSMInstance.GetID())
	return nil
}

// UpdateObservedStateOfInstance updates the observed state of the service
func (n *NmapInstance) UpdateObservedStateOfInstance(ctx context.Context, filesystemService filesystem.Service, tick uint64) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	start := time.Now()
	svcInfo, err := n.monitorService.Status(ctx, filesystemService, n.config.Name, tick)
	if err != nil {
		return fmt.Errorf("failed to get nmap metrics: %w", err)
	}
	metrics.ObserveReconcileTime(logger.ComponentNmapInstance, n.baseFSMInstance.GetID()+".getServiceStatus", time.Since(start))

	// Stores the raw service info
	n.ObservedState.ServiceInfo = svcInfo

	// Fetch the actual Nmap config from the service
	start = time.Now()
	observedConfig, err := n.monitorService.GetConfig(ctx, filesystemService, n.baseFSMInstance.GetID())
	metrics.ObserveReconcileTime(logger.ComponentNmapInstance, n.baseFSMInstance.GetID()+".getConfig", time.Since(start))
	if err == nil {
		// Only update if we successfully got the config
		n.ObservedState.ObservedNmapServiceConfig = observedConfig
	} else {
		if strings.Contains(err.Error(), nmap_service.ErrServiceNotExist.Error()) {
			// Log the error but don't fail - this might happen during creation when the config file doesn't exist yet
			n.baseFSMInstance.GetLogger().Debugf("Service not found, will be created during reconciliation: %v", err)
			return nil
		} else {
			return fmt.Errorf("failed to get observed Nmap config: %w", err)
		}
	}

	// Detect a config change - but let the S6 manager handle the actual reconciliation
	// Use new ConfigsEqual function that handles Nmap defaults properly
	if !n.config.NmapServiceConfig.Equal(n.ObservedState.ObservedNmapServiceConfig) {
		// Check if the service exists before attempting to update
		if n.monitorService.ServiceExists(ctx, filesystemService, n.baseFSMInstance.GetID()) {
			n.baseFSMInstance.GetLogger().Debugf("Observed Nmap config is different from desired config, updating S6 configuration")

			// Update the config in the S6 manager
			err := n.monitorService.UpdateNmapInS6Manager(ctx, &n.config.NmapServiceConfig, n.baseFSMInstance.GetID())
			if err != nil {
				return fmt.Errorf("failed to update Nmap service configuration: %w", err)
			}
		} else {
			n.baseFSMInstance.GetLogger().Debugf("Config differences detected but service does not exist yet, skipping update")
		}
	}

	return nil
}

// IsNmapRunning is called to check on the state of Nmap
func (n *NmapInstance) IsNmapRunning() bool {
	return n.ObservedState.ServiceInfo.NmapStatus.IsRunning
}
