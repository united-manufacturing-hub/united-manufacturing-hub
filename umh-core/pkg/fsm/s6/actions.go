package s6

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/united-manufacturing-hub/benthos-umh/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/metrics"
	s6service "github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/service/s6"
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

// InitiateS6Create attempts to create the S6 service directory structure.
func (s *S6Instance) initiateS6Create(ctx context.Context) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Instance, s.baseFSMInstance.GetID()+".initiateS6Create", time.Since(start))
	}()

	s.baseFSMInstance.GetLogger().Debugf("Starting Action: Creating S6 service %s ...", s.baseFSMInstance.GetID())

	// Check if we have a config with command or other settings
	configEmpty := s.config.S6ServiceConfig.Command == nil && s.config.S6ServiceConfig.Env == nil && s.config.S6ServiceConfig.ConfigFiles == nil

	if !configEmpty {
		// Create service with custom configuration
		err := s.service.Create(ctx, s.servicePath, s.config.S6ServiceConfig)
		if err != nil {
			return fmt.Errorf("failed to create service with config for %s: %w", s.baseFSMInstance.GetID(), err)
		}
	} else {
		// Simple creation with no configuration, useful for testing
		err := s.service.Create(ctx, s.servicePath, config.S6ServiceConfig{})
		if err != nil {
			return fmt.Errorf("failed to create service directory for %s: %w", s.baseFSMInstance.GetID(), err)
		}
	}

	s.baseFSMInstance.GetLogger().Debugf("S6 service %s directory structure created", s.baseFSMInstance.GetID())
	return nil
}

// InitiateS6Remove attempts to remove the S6 service directory structure.
// It requires the service to be stopped before removal.
func (s *S6Instance) initiateS6Remove(ctx context.Context) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Instance, s.baseFSMInstance.GetID()+".initiateS6Remove", time.Since(start))
	}()

	s.baseFSMInstance.GetLogger().Debugf("Starting Action: Removing S6 service %s ...", s.baseFSMInstance.GetID())

	// First ensure the service is stopped
	if s.IsS6Running() {
		return fmt.Errorf("service %s cannot be removed while running", s.baseFSMInstance.GetID())
	}

	// Remove the service directory
	err := s.service.Remove(ctx, s.servicePath)
	if err != nil {
		return fmt.Errorf("failed to remove service directory for %s: %w", s.baseFSMInstance.GetID(), err)
	}

	s.baseFSMInstance.GetLogger().Debugf("S6 service %s removed", s.baseFSMInstance.GetID())
	return nil
}

// InitiateS6Start attempts to start the S6 service.
func (s *S6Instance) initiateS6Start(ctx context.Context) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Instance, s.baseFSMInstance.GetID()+".initiateS6Start", time.Since(start))
	}()

	s.baseFSMInstance.GetLogger().Debugf("Starting Action: Starting S6 service %s ...", s.baseFSMInstance.GetID())

	err := s.service.Start(ctx, s.servicePath)
	if err != nil {
		return fmt.Errorf("failed to start S6 service %s: %w", s.baseFSMInstance.GetID(), err)
	}

	s.baseFSMInstance.GetLogger().Debugf("S6 service %s start command executed", s.baseFSMInstance.GetID())
	return nil
}

// InitiateS6Stop attempts to stop the S6 service.
func (s *S6Instance) initiateS6Stop(ctx context.Context) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Instance, s.baseFSMInstance.GetID()+".initiateS6Stop", time.Since(start))
	}()

	s.baseFSMInstance.GetLogger().Debugf("Starting Action: Stopping S6 service %s ...", s.baseFSMInstance.GetID())

	err := s.service.Stop(ctx, s.servicePath)
	if err != nil {
		return fmt.Errorf("failed to stop S6 service %s: %w", s.baseFSMInstance.GetID(), err)
	}

	s.baseFSMInstance.GetLogger().Debugf("S6 service %s stop command executed", s.baseFSMInstance.GetID())
	return nil
}

// InitiateS6Restart attempts to restart the S6 service.
func (s *S6Instance) initiateS6Restart(ctx context.Context) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Instance, s.baseFSMInstance.GetID()+".initiateS6Restart", time.Since(start))
	}()

	s.baseFSMInstance.GetLogger().Debugf("Starting Action: Restarting S6 service %s ...", s.baseFSMInstance.GetID())

	err := s.service.Restart(ctx, s.servicePath)
	if err != nil {
		return fmt.Errorf("failed to restart S6 service %s: %w", s.baseFSMInstance.GetID(), err)
	}

	s.baseFSMInstance.GetLogger().Debugf("S6 service %s restart command executed", s.baseFSMInstance.GetID())
	return nil
}

// UpdateObservedState updates the observed state of the service
func (s *S6Instance) updateObservedState(ctx context.Context) error {

	// Measure status time
	info, err := s.service.Status(ctx, s.servicePath)
	if err != nil {
		s.ObservedState.ServiceInfo.Status = s6service.ServiceUnknown

		if s.baseFSMInstance.GetCurrentFSMState() == fsm.LifecycleStateCreating || s.baseFSMInstance.GetCurrentFSMState() == fsm.LifecycleStateToBeCreated {
			// If the service is being created, we don't want to count this as an error
			return s6service.ErrServiceNotExist
		}

		// Otherwise, we count this as an error
		s.baseFSMInstance.GetLogger().Errorf("error updating observed state for %s: %s", s.baseFSMInstance.GetID(), err)
		return err
	}

	// Store the raw service info
	s.ObservedState.ServiceInfo = info

	// Map the service status to FSM status
	switch info.Status {
	case s6service.ServiceUp:
		s.ObservedState.ServiceInfo.Status = s6service.ServiceUp
	case s6service.ServiceDown:
		s.ObservedState.ServiceInfo.Status = s6service.ServiceDown
	case s6service.ServiceRestarting:
		s.ObservedState.ServiceInfo.Status = s6service.ServiceRestarting
	default:
		s.ObservedState.ServiceInfo.Status = s6service.ServiceUnknown
	}

	// Set LastStateChange time if this is the first update
	if s.ObservedState.LastStateChange == 0 {
		s.ObservedState.LastStateChange = time.Now().Unix()
	}

	// Fetch the actual service config from s6
	config, err := s.service.GetConfig(ctx, s.servicePath)
	if err != nil {
		return fmt.Errorf("failed to get S6 service config for %s: %w", s.baseFSMInstance.GetID(), err)
	}
	s.ObservedState.ObservedS6ServiceConfig = config

	// the easiest way to do this is causing this instance to be removed, which will trigger a re-create by the manager
	if !reflect.DeepEqual(s.ObservedState.ObservedS6ServiceConfig, s.config.S6ServiceConfig) {
		s.baseFSMInstance.GetLogger().Debugf("Observed config is different from desired config, triggering a re-create")
		s.logConfigDifferences(s.config.S6ServiceConfig, s.ObservedState.ObservedS6ServiceConfig)
		s.baseFSMInstance.Remove(ctx)
	}

	return nil
}

// IsS6Running checks if the S6 service is running.
func (s *S6Instance) IsS6Running() bool {
	return s.ObservedState.ServiceInfo.Status == s6service.ServiceUp
}

// IsS6Stopped checks if the S6 service is stopped.
func (s *S6Instance) IsS6Stopped() bool {
	return s.ObservedState.ServiceInfo.Status == s6service.ServiceDown
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
func (s *S6Instance) GetExitHistory() []s6service.ExitEvent {
	return s.ObservedState.ServiceInfo.ExitHistory
}

// logConfigDifferences logs the specific differences between desired and observed configurations
func (s *S6Instance) logConfigDifferences(desired, observed config.S6ServiceConfig) {
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
