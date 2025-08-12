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

package s6

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6_shared"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// The functions in this file define heavier, possibly fail-prone operations
// (for example, network or file I/O) that the S6 FSM might need to perform.
// They are intended to be called from Reconcile.
//
// IMPORTANT:
//   - Each action is expected to be idempotent, since it may be retried
//     multiple times due to transient failures.
//   - Each action takes a context.Context and can return an error if the operation fails.
//   - If an error occurs, the Reconcile function must handle
//     setting S6Instance.lastError and scheduling a retry/backoff.

// CreateInstance attempts to create the S6 service directory structure.
func (s *S6Instance) CreateInstance(ctx context.Context, filesystemService filesystem.Service) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Instance, s.baseFSMInstance.GetID()+".CreateInstance", time.Since(start))
	}()

	s.baseFSMInstance.GetLogger().Debugf("Starting Action: Creating S6 service %s ...", s.baseFSMInstance.GetID())

	// Check if we have a config with command or other settings
	configEmpty := s.config.S6ServiceConfig.Command == nil && s.config.S6ServiceConfig.Env == nil && s.config.S6ServiceConfig.ConfigFiles == nil

	if !configEmpty {
		// Create service with custom configuration
		err := s.service.Create(ctx, s.servicePath, s.config.S6ServiceConfig, filesystemService)
		if err != nil {
			return fmt.Errorf("failed to create service with config for %s: %w", s.baseFSMInstance.GetID(), err)
		}
	} else {
		// Simple creation with no configuration, useful for testing
		err := s.service.Create(ctx, s.servicePath, s6serviceconfig.S6ServiceConfig{}, filesystemService)
		if err != nil {
			return fmt.Errorf("failed to create service directory for %s: %w", s.baseFSMInstance.GetID(), err)
		}
	}

	s.baseFSMInstance.GetLogger().Debugf("S6 service %s directory structure created", s.baseFSMInstance.GetID())
	return nil
}

// RemoveInstance attempts to remove the S6 service directory structure.
// It requires the service to be stopped before removal.
func (s *S6Instance) RemoveInstance(ctx context.Context, filesystemService filesystem.Service) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Instance, s.baseFSMInstance.GetID()+".RemoveInstance", time.Since(start))
	}()

	s.baseFSMInstance.GetLogger().Debugf("Starting Action: Removing S6 service %s ...", s.baseFSMInstance.GetID())

	// First ensure the service is stopped
	if s.IsS6Running() {
		return fmt.Errorf("service %s cannot be removed while running", s.baseFSMInstance.GetID())
	}

	// Remove the service directory
	err := s.service.Remove(ctx, s.servicePath, filesystemService)
	if err != nil {
		// If the service doesn't exist, consider removal successful
		if errors.Is(err, s6_shared.ErrServiceNotExist) {
			s.baseFSMInstance.GetLogger().Debugf("S6 service %s already removed", s.baseFSMInstance.GetID())
			return nil
		}
		return fmt.Errorf("failed to remove service directory for %s: %w", s.baseFSMInstance.GetID(), err)
	}

	s.baseFSMInstance.GetLogger().Debugf("S6 service %s removed", s.baseFSMInstance.GetID())
	return nil
}

// StartInstance attempts to start the S6 service.
func (s *S6Instance) StartInstance(ctx context.Context, filesystemService filesystem.Service) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Instance, s.baseFSMInstance.GetID()+".StartInstance", time.Since(start))
	}()

	s.baseFSMInstance.GetLogger().Debugf("Starting Action: Starting S6 service %s ...", s.baseFSMInstance.GetID())

	err := s.service.Start(ctx, s.servicePath, filesystemService)
	if err != nil {
		return fmt.Errorf("failed to start S6 service %s: %w", s.baseFSMInstance.GetID(), err)
	}

	s.baseFSMInstance.GetLogger().Debugf("S6 service %s start command executed", s.baseFSMInstance.GetID())
	return nil
}

// StopInstance attempts to stop the S6 service.
func (s *S6Instance) StopInstance(ctx context.Context, filesystemService filesystem.Service) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Instance, s.baseFSMInstance.GetID()+".StopInstance", time.Since(start))
	}()

	s.baseFSMInstance.GetLogger().Debugf("Starting Action: Stopping S6 service %s ...", s.baseFSMInstance.GetID())

	err := s.service.Stop(ctx, s.servicePath, filesystemService)
	if err != nil {
		return fmt.Errorf("failed to stop S6 service %s: %w", s.baseFSMInstance.GetID(), err)
	}

	s.baseFSMInstance.GetLogger().Debugf("S6 service %s stop command executed", s.baseFSMInstance.GetID())
	return nil
}

// CheckForCreation checks whether the creation was successful
func (s *S6Instance) CheckForCreation(ctx context.Context, filesystemService filesystem.Service) bool {
	servicePath := s.servicePath
	ready, err := s.service.EnsureSupervision(ctx, servicePath, filesystemService)
	if err != nil {
		s.baseFSMInstance.GetLogger().Warnf("Failed to ensure service supervision: %v", err)
		return false // Don't transition state yet, retry next reconcile
	}

	// Only transition if the supervise directory actually exists
	if !ready {
		s.baseFSMInstance.GetLogger().Debugf("Waiting for s6-svscan to create supervise directory")
		return false // Don't transition state yet, retry next reconcile
	}

	return true // Transition to the next state
}

// UpdateObservedStateOfInstance updates the observed state of the service
func (s *S6Instance) UpdateObservedStateOfInstance(ctx context.Context, services serviceregistry.Provider, snapshot fsm.SystemSnapshot) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Instance, s.baseFSMInstance.GetID()+".updateObservedState", time.Since(start))
	}()

	observedStateCtx, cancel := context.WithTimeout(ctx, constants.S6UpdateObservedStateTimeout)
	defer cancel()

	//nolint:gosec
	serviceInfo, err := s.service.Status(observedStateCtx, s.servicePath, services.GetFileSystem())
	if err != nil {
		return fmt.Errorf("failed to get service status: %w", err)
	}

	s.ObservedState.ServiceInfo = serviceInfo

	if s.ObservedState.LastStateChange == 0 {
		s.ObservedState.LastStateChange = time.Now().Unix()
	}

	// Fetch the actual service config from s6
	config, err := s.service.GetConfig(ctx, s.servicePath, services.GetFileSystem())
	if err != nil {
		return fmt.Errorf("failed to get service config: %w", err)
	}
	s.ObservedState.ObservedS6ServiceConfig = config

	// the easiest way to do this is causing this instance to be removed, which will trigger a re-create by the manager
	if !reflect.DeepEqual(s.ObservedState.ObservedS6ServiceConfig, s.config.S6ServiceConfig) {
		s.baseFSMInstance.GetLogger().Debugf("Observed config is different from desired config, triggering a re-create")
		s.logConfigDifferences(s.config.S6ServiceConfig, s.ObservedState.ObservedS6ServiceConfig)
		err := s.baseFSMInstance.Remove(ctx)
		if err != nil {
			s.baseFSMInstance.GetLogger().Errorf("error removing S6 instance %s: %v", s.baseFSMInstance.GetID(), err)
			return err
		}
	}

	return nil
}

// IsS6Running checks if the S6 service is running.
func (s *S6Instance) IsS6Running() bool {
	return s.ObservedState.ServiceInfo.Status == s6_shared.ServiceUp
}

// IsS6Stopped checks if the S6 service is stopped.
func (s *S6Instance) IsS6Stopped() bool {
	return s.ObservedState.ServiceInfo.Status == s6_shared.ServiceDown
}

// GetServicePid gets the process ID of the running service.
// Returns -1 if the service is not running.
func (s *S6Instance) GetServicePid() int {
	if s.IsS6Running() {
		return s.ObservedState.ServiceInfo.Pid
	}
	return -1
}

// GetServiceUptime gets the uptime of the service in seconds.
// Returns -1 if the service is not running.
func (s *S6Instance) GetServiceUptime() int64 {
	if s.IsS6Running() {
		return s.ObservedState.ServiceInfo.Uptime
	}
	return -1
}

// GetExitCode gets the last exit code of the service.
// Returns -1 if the service is running.
func (s *S6Instance) GetExitCode() int {
	if s.IsS6Running() {
		return -1
	}
	return s.ObservedState.ServiceInfo.ExitCode
}

// IsServiceWantingUp checks if the service is attempting to start.
func (s *S6Instance) IsServiceWantingUp() bool {
	return s.ObservedState.ServiceInfo.WantUp
}

// GetExitHistory gets the history of service exit events.
func (s *S6Instance) GetExitHistory() []s6_shared.ExitEvent {
	return s.ObservedState.ServiceInfo.ExitHistory
}

// logConfigDifferences logs the specific differences between desired and observed configurations
func (s *S6Instance) logConfigDifferences(desired, observed s6serviceconfig.S6ServiceConfig) {
	s.baseFSMInstance.GetLogger().Infof("Configuration differences for %s:", s.baseFSMInstance.GetID())

	// Command differences
	if !reflect.DeepEqual(desired.Command, observed.Command) {
		s.baseFSMInstance.GetLogger().Infof("Command - want: %v", desired.Command)
		s.baseFSMInstance.GetLogger().Infof("Command - is:   %v", observed.Command)
	}

	// Environment variables differences
	if !reflect.DeepEqual(desired.Env, observed.Env) {
		s.baseFSMInstance.GetLogger().Infof("Environment variables differences:")

		// Check for keys in desired that are missing or different in observed
		for k, v := range desired.Env {
			if observedVal, ok := observed.Env[k]; !ok {
				s.baseFSMInstance.GetLogger().Infof("   - %s: want: %q, is: <missing>", k, v)
			} else if v != observedVal {
				s.baseFSMInstance.GetLogger().Infof("   - %s: want: %q, is: %q", k, v, observedVal)
			}
		}

		// Check for keys in observed that are not in desired
		for k, v := range observed.Env {
			if _, ok := desired.Env[k]; !ok {
				s.baseFSMInstance.GetLogger().Infof("   - %s: want: <missing>, is: %q", k, v)
			}
		}
	}

	// Config files differences
	if !reflect.DeepEqual(desired.ConfigFiles, observed.ConfigFiles) {
		s.baseFSMInstance.GetLogger().Infof("Config files differences:")

		// Check config files in desired that are missing or different in observed
		for path, content := range desired.ConfigFiles {
			if observedContent, ok := observed.ConfigFiles[path]; !ok {
				s.baseFSMInstance.GetLogger().Infof("   - %s: want: present, is: <missing>", path)
			} else if content != observedContent {
				// For large config files, we don't want to log the entire content
				// Just log that they're different
				s.baseFSMInstance.GetLogger().Infof("   - %s: content differs", path)
			}
		}

		// Check for config files in observed that are not in desired
		for path := range observed.ConfigFiles {
			if _, ok := desired.ConfigFiles[path]; !ok {
				s.baseFSMInstance.GetLogger().Infof("   - %s: want: <missing>, is: present", path)
			}
		}
	}

	// Memory limit differences
	if desired.MemoryLimit != observed.MemoryLimit {
		s.baseFSMInstance.GetLogger().Infof("Memory limit - want: %d, is: %d", desired.MemoryLimit, observed.MemoryLimit)
	}
}
