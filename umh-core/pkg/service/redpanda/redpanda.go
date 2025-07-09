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

package redpanda

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/redpandaserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/httpclient"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	"go.uber.org/zap"

	redpanda_monitor_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/redpanda_monitor"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

// IRedpandaService is the interface for managing Redpanda
type IRedpandaService interface {
	// GenerateS6ConfigForRedpanda generates a S6 config for a given redpanda instance
	GenerateS6ConfigForRedpanda(redpandaConfig *redpandaserviceconfig.RedpandaServiceConfig, redpandaName string) (s6serviceconfig.S6ServiceConfig, error)
	// GetConfig returns the actual Redpanda config from the S6 service
	GetConfig(ctx context.Context, filesystemService filesystem.Service, redpandaName string, tick uint64, loopStartTime time.Time) (redpandaserviceconfig.RedpandaServiceConfig, error)
	// Status checks the status of a Redpanda service
	Status(ctx context.Context, filesystemService filesystem.Service, redpandaName string, tick uint64, loopStartTime time.Time) (ServiceInfo, error)
	// AddRedpandaToS6Manager adds a Redpanda instance to the S6 manager
	AddRedpandaToS6Manager(ctx context.Context, cfg *redpandaserviceconfig.RedpandaServiceConfig, filesystemService filesystem.Service, redpandaName string) error
	// UpdateRedpandaInS6Manager updates an existing Redpanda instance in the S6 manager
	UpdateRedpandaInS6Manager(ctx context.Context, cfg *redpandaserviceconfig.RedpandaServiceConfig, redpandaName string) error
	// RemoveRedpandaFromS6Manager removes a Redpanda instance from the S6 manager
	RemoveRedpandaFromS6Manager(ctx context.Context, redpandaName string) error
	// StartRedpanda starts a Redpanda instance
	StartRedpanda(ctx context.Context, redpandaName string) error
	// StopRedpanda stops a Redpanda instance
	StopRedpanda(ctx context.Context, redpandaName string) error
	// ForceRemoveRedpanda removes a Redpanda instance from the S6 manager
	ForceRemoveRedpanda(ctx context.Context, filesystemService filesystem.Service, redpandaName string) error
	// ServiceExists checks if a Redpanda service exists
	ServiceExists(ctx context.Context, filesystemService filesystem.Service, redpandaName string) bool
	ReconcileManager(ctx context.Context, services serviceregistry.Provider, snapshot fsm.SystemSnapshot) (error, bool)
	// IsLogsFine reports true when recent Redpanda logs (within logWindow) contain
	// no critical error patterns.
	//
	// It returns:
	//   ok    – true when logs look clean, false otherwise.
	//   entry – zero value when ok is true; otherwise the first offending log line.
	IsLogsFine(logs []s6service.LogEntry, currentTime time.Time, logWindow time.Duration, transitionToRunningTime time.Time) (bool, s6service.LogEntry)
	// IsMetricsErrorFree reports true when Redpanda metrics show no alerts or
	// cluster‑level errors.
	//
	// It returns:
	//   ok     – true when metrics are error‑free, false otherwise.
	//   reason – empty when ok is true; otherwise a short explanation (e.g.
	//            "storage free space alert: <free>/<total> bytes").
	IsMetricsErrorFree(metrics redpanda_monitor.Metrics) (bool, string)
	// HasProcessingActivity reports true when Redpanda metrics state indicates
	// active input/output throughput.
	//
	// It returns:
	//   ok     – true when activity is detected, false otherwise.
	//   reason – empty when ok is true; otherwise a brief throughput summary.
	HasProcessingActivity(status RedpandaStatus) (bool, string)
	// UpdateRedpandaClusterConfig updates the cluster config of a Redpanda service by sending a PUT request to the Redpanda API
	UpdateRedpandaClusterConfig(ctx context.Context, redpandaName string, configUpdates map[string]interface{}) error
}

// ServiceInfo contains information about a Redpanda service
type ServiceInfo struct {
	// S6ObservedState contains information about the S6 service
	S6ObservedState s6fsm.S6ObservedState
	// S6FSMState contains the current state of the S6 FSM
	S6FSMState string
	// RedpandaStatus contains information about the status of the Redpanda service
	RedpandaStatus RedpandaStatus
	// StatusReason contains the reason for the current state of the Redpanda service
	StatusReason string
}

// RedpandaStatus contains information about the status of the Redpanda service
type RedpandaStatus struct {
	// HealthCheck contains information about the health of the Redpanda service
	HealthCheck HealthCheck
	// Logs contains the logs of the Redpanda service
	//
	// **Performance consideration**
	//
	//   • Logs can grow quickly, and profiling shows that naïvely deep-copying
	//     this slice dominates CPU time (see https://flamegraph.com/share/592a6a59-25d1-11f0-86bc-aa320ab09ef2).
	//
	//   • The FSM needs read-only access to the logs for historical snapshots;
	//     it never mutates them.
	//
	//   • The s6 layer *always* allocates a brand-new slice when it returns
	//     logs (see DefaultService.GetLogs), so sharing the slice's backing
	//     array across snapshots cannot introduce data races.
	//
	// Therefore we override the default behaviour and copy only the 3-word
	// slice header (24 B on amd64) — see CopyLogs below.
	Logs []s6service.LogEntry
	// RedpandaMetrics contains information about the metrics of the Redpanda service
	RedpandaMetrics redpanda_monitor.RedpandaMetrics
}

// CopyLogs is a go-deepcopy override for the Logs field.
//
// go-deepcopy looks for a method with the signature
//
//	func (dst *T) Copy<FieldName>(src <FieldType>) error
//
// and, if present, calls it instead of performing its generic deep-copy logic.
// By assigning the slice directly we make a **shallow copy**: the header is
// duplicated but the underlying backing array is shared.
//
// Why this is safe:
//
//  1. The s6 service returns a fresh []LogEntry on every call, never reusing
//     or mutating a previously returned slice.
//  2. Logs is treated as immutable after the snapshot is taken.
//
// If either assumption changes, delete this method to fall back to the default
// deep-copy (O(n) but safe for mutable slices).
//
// See also: https://github.com/tiendc/go-deepcopy?tab=readme-ov-file#copy-struct-fields-via-struct-methods
func (rs *RedpandaStatus) CopyLogs(src []s6service.LogEntry) error {
	rs.Logs = src
	return nil
}

// HealthCheck contains information about the health of the Redpanda service
// https://docs.redpanda.com/redpanda-connect/guides/monitoring/
type HealthCheck struct {
	// IsLive is true if the Redpanda service is live
	IsLive bool
	// IsReady is true if the Redpanda service is ready to process data
	IsReady bool
	// Version contains the version of the Redpanda service
	Version string
}

// RedpandaService is the default implementation of the IRedpandaService interface
type RedpandaService struct {
	logger *zap.SugaredLogger

	s6Manager        *s6fsm.S6Manager
	s6Service        s6service.Service // S6 service for direct S6 operations
	s6ServiceConfigs []config.S6FSMConfig
	httpClient       httpclient.HTTPClient

	baseDir string

	redpandaMonitorManager *redpanda_monitor_fsm.RedpandaMonitorManager
	redpandaMonitorConfigs []config.RedpandaMonitorConfig

	schemaRegistryManager ISchemaRegistry
}

// RedpandaServiceOption is a function that modifies a RedpandaService
type RedpandaServiceOption func(*RedpandaService)

// WithHTTPClient sets a custom HTTP client for the RedpandaService
// This is only used for testing purposes
func WithHTTPClient(client httpclient.HTTPClient) RedpandaServiceOption {
	return func(s *RedpandaService) {
		s.httpClient = client
	}
}

// WithS6Service sets a custom S6 service for the RedpandaService
func WithS6Service(s6Service s6service.Service) RedpandaServiceOption {
	return func(s *RedpandaService) {
		s.s6Service = s6Service
	}
}

// WithS6Manager sets a custom S6 manager for the RedpandaService
func WithS6Manager(s6Manager *s6fsm.S6Manager) RedpandaServiceOption {
	return func(s *RedpandaService) {
		s.s6Manager = s6Manager
	}
}

// WithBaseDir sets the base directory for the RedpandaService
func WithBaseDir(baseDir string) RedpandaServiceOption {
	return func(s *RedpandaService) {
		s.baseDir = baseDir
	}
}

// WithMonitorManager sets a custom monitor manager for the RedpandaService
func WithMonitorManager(monitorManager *redpanda_monitor_fsm.RedpandaMonitorManager) RedpandaServiceOption {
	return func(s *RedpandaService) {
		s.redpandaMonitorManager = monitorManager
	}
}

// WithSchemaRegistryManager sets a custom schema registry manager for the RedpandaService
func WithSchemaRegistryManager(schemaRegistryManager ISchemaRegistry) RedpandaServiceOption {
	return func(s *RedpandaService) {
		s.schemaRegistryManager = schemaRegistryManager
	}
}

// NewDefaultRedpandaService creates a new default Redpanda service
// name is the name of the Redpanda service as defined in the UMH config
func NewDefaultRedpandaService(redpandaName string, opts ...RedpandaServiceOption) *RedpandaService {
	managerName := fmt.Sprintf("%s%s", logger.ComponentRedpandaService, redpandaName)
	service := &RedpandaService{
		logger:                 logger.For(managerName),
		s6Manager:              s6fsm.NewS6Manager(managerName),
		s6Service:              s6service.NewDefaultService(),
		httpClient:             httpclient.NewDefaultHTTPClient(),
		baseDir:                constants.DefaultRedpandaBaseDir,
		redpandaMonitorManager: redpanda_monitor_fsm.NewRedpandaMonitorManager(redpandaName),
		schemaRegistryManager:  NewSchemaRegistry(),
	}

	// Apply options
	for _, opt := range opts {
		opt(service)
	}

	return service
}

// generateRedpandaYaml generates a Redpanda YAML configuration from a RedpandaServiceConfig
func (s *RedpandaService) generateRedpandaYaml(config *redpandaserviceconfig.RedpandaServiceConfig) (string, error) {
	if config == nil {
		return "", fmt.Errorf("config is nil")
	}

	return redpandaserviceconfig.RenderRedpandaYAML(config.Topic.DefaultTopicRetentionMs, config.Topic.DefaultTopicRetentionBytes, config.Topic.DefaultTopicCompressionAlgorithm, config.Topic.DefaultTopicCleanupPolicy, config.Topic.DefaultTopicSegmentMs)
}

// generateS6ConfigForRedpanda creates a S6 config for a given redpanda instance
// Expects s6ServiceName (e.g. "redpanda-myservice"), not the raw redpandaName
func (s *RedpandaService) GenerateS6ConfigForRedpanda(redpandaConfig *redpandaserviceconfig.RedpandaServiceConfig, s6ServiceName string) (s6Config s6serviceconfig.S6ServiceConfig, err error) {
	configPath := fmt.Sprintf("%s/%s/config/%s", constants.S6BaseDir, s6ServiceName, constants.RedpandaConfigFileName)

	yamlConfig, err := s.generateRedpandaYaml(redpandaConfig)
	if err != nil {
		return s6serviceconfig.S6ServiceConfig{}, err
	}

	if redpandaConfig.Resources.MaxCores == 0 {
		redpandaConfig.Resources.MaxCores = 1
	}

	if redpandaConfig.Resources.MemoryPerCoreInBytes == 0 {
		redpandaConfig.Resources.MemoryPerCoreInBytes = 2048 * 1024 * 1024 // 2GB
	}

	s6Config = s6serviceconfig.S6ServiceConfig{
		Command: []string{
			"/opt/redpanda/bin/redpanda",
			"--redpanda-cfg",
			configPath,
			// --memory comes directly from seastar (you can find all redpanda seastar options by executing `redpanda --help` or `redpanda --seastar-help`)
			"--memory",
			formatMemory(redpandaConfig.Resources.MemoryPerCoreInBytes),
			// --smp comes directly from seastar (you can find all redpanda seastar options by executing `redpanda --help` or `redpanda --seastar-help`)
			"--smp",
			fmt.Sprintf("%d", redpandaConfig.Resources.MaxCores),
		},
		Env: map[string]string{},
		ConfigFiles: map[string]string{
			constants.RedpandaConfigFileName: yamlConfig,
		},
	}

	return s6Config, nil
}

// GetS6ServiceName converts a logical Redpanda name ("my-pipe") into the
// canonical S6 service name ("redpanda-my-pipe").
//
// It is exported ONLY because tests and other packages need the mapping
func (s *RedpandaService) GetS6ServiceName(redpandaName string) string {
	return fmt.Sprintf("redpanda-%s", redpandaName)
}

// GetConfig returns the actual Redpanda config from the S6 service
func (s *RedpandaService) GetConfig(ctx context.Context, filesystemService filesystem.Service, redpandaName string, tick uint64, loopStartTime time.Time) (redpandaserviceconfig.RedpandaServiceConfig, error) {
	if ctx.Err() != nil {
		return redpandaserviceconfig.RedpandaServiceConfig{}, ctx.Err()
	}

	// Skip health checks and metrics if the service doesn't exist yet
	// This avoids unnecessary errors in Status() when the service is still being created
	if _, exists := s.s6Manager.GetInstance(s.GetS6ServiceName(redpandaName)); !exists {
		return redpandaserviceconfig.RedpandaServiceConfig{}, nil
	}

	// Get the last observed state of the redpanda monitor
	s6ServiceName := s.GetS6ServiceName(redpandaName)
	lastObservedState, err := s.redpandaMonitorManager.GetLastObservedState(s6ServiceName)
	if err != nil {
		return redpandaserviceconfig.RedpandaServiceConfig{}, fmt.Errorf("failed to get redpanda status: %w", err)
	}

	if lastObservedState == nil {
		return redpandaserviceconfig.RedpandaServiceConfig{}, ErrLastObservedStateNil
	}

	// Convert the last observed state of the redpanda monitor
	lastRedpandaMonitorObservedState, ok := lastObservedState.(redpanda_monitor_fsm.RedpandaMonitorObservedState)
	if !ok {
		return redpandaserviceconfig.RedpandaServiceConfig{}, fmt.Errorf("observed state is not a RedpandaMonitorObservedState: %v", lastObservedState)
	}

	var redpandaStatus redpandaserviceconfig.RedpandaServiceConfig

	if lastRedpandaMonitorObservedState.ServiceInfo == nil {
		return redpandaserviceconfig.RedpandaServiceConfig{}, ErrLastObservedStateNil
	}

	if lastRedpandaMonitorObservedState.ServiceInfo.RedpandaStatus.LastScan != nil {
		if lastRedpandaMonitorObservedState.ServiceInfo.RedpandaStatus.LastScan.ClusterConfig != nil {
			redpandaStatus.Topic.DefaultTopicRetentionMs = lastRedpandaMonitorObservedState.ServiceInfo.RedpandaStatus.LastScan.ClusterConfig.Topic.DefaultTopicRetentionMs
			redpandaStatus.Topic.DefaultTopicRetentionBytes = lastRedpandaMonitorObservedState.ServiceInfo.RedpandaStatus.LastScan.ClusterConfig.Topic.DefaultTopicRetentionBytes
			redpandaStatus.Topic.DefaultTopicCompressionAlgorithm = lastRedpandaMonitorObservedState.ServiceInfo.RedpandaStatus.LastScan.ClusterConfig.Topic.DefaultTopicCompressionAlgorithm
			redpandaStatus.Topic.DefaultTopicCleanupPolicy = lastRedpandaMonitorObservedState.ServiceInfo.RedpandaStatus.LastScan.ClusterConfig.Topic.DefaultTopicCleanupPolicy
			redpandaStatus.Topic.DefaultTopicSegmentMs = lastRedpandaMonitorObservedState.ServiceInfo.RedpandaStatus.LastScan.ClusterConfig.Topic.DefaultTopicSegmentMs
		} else {
			s.logger.Debugf("Cluster config is nil, skipping update")
		}
		redpandaStatus.Resources.MaxCores = 1
		redpandaStatus.Resources.MemoryPerCoreInBytes = 2048 * 1024 * 1024 // 2GB
	} else {
		return redpandaserviceconfig.RedpandaServiceConfig{}, fmt.Errorf("last scan is nil")
	}

	// If the current state is stopped, we can return immediately
	// There wont be any logs, metrics, etc. to check
	if !lastRedpandaMonitorObservedState.ServiceInfo.RedpandaStatus.IsRunning {
		return redpandaserviceconfig.RedpandaServiceConfig{}, ErrRedpandaMonitorNotRunning
	}

	return redpandaserviceconfig.NormalizeRedpandaConfig(redpandaStatus), nil
}

// Status checks the status of a Redpanda service
func (s *RedpandaService) Status(ctx context.Context, filesystemService filesystem.Service, redpandaName string, tick uint64, loopStartTime time.Time) (ServiceInfo, error) {
	if ctx.Err() != nil {
		return ServiceInfo{}, ctx.Err()
	}

	s6ServiceName := s.GetS6ServiceName(redpandaName)

	// First, check if the service exists in the S6 manager
	// This is a crucial check that prevents "instance not found" errors
	// during reconciliation when a service is being created or removed
	if _, exists := s.s6Manager.GetInstance(s6ServiceName); !exists {
		s.logger.Debugf("Service %s not found in S6 manager: %+v", redpandaName, s.s6Manager.GetInstances())
		return ServiceInfo{}, ErrServiceNotExist
	}

	// Let's get the status of the underlying s6 service
	s6ServiceObservedStateRaw, err := s.s6Manager.GetLastObservedState(s6ServiceName)
	if err != nil {
		// If we still get an "instance not found" error despite our earlier check,
		// it's likely that the service was removed between our check and this call
		if strings.Contains(err.Error(), "instance "+redpandaName+" not found") ||
			strings.Contains(err.Error(), "not found") {
			s.logger.Debugf("Service %s was removed during status check", redpandaName)
			return ServiceInfo{}, ErrServiceNotExist
		}
		return ServiceInfo{}, fmt.Errorf("failed to get last observed state: %w", err)
	}
	s6ServiceObservedState, ok := s6ServiceObservedStateRaw.(s6fsm.S6ObservedState)
	if !ok {
		return ServiceInfo{}, fmt.Errorf("observed state is not a S6ObservedState: %v", s6ServiceObservedStateRaw)
	}

	// Let's get the current FSM state of the underlying s6 FSM
	s6FSMState, err := s.s6Manager.GetCurrentFSMState(s6ServiceName)
	if err != nil {
		// Similar to above, if the service was removed during our check
		if strings.Contains(err.Error(), "instance "+s6ServiceName+" not found") ||
			strings.Contains(err.Error(), "not found") {
			s.logger.Debugf("Service %s was removed during status check", s6ServiceName)
			return ServiceInfo{}, ErrServiceNotExist
		}
		return ServiceInfo{}, fmt.Errorf("failed to get current FSM state: %w", err)
	}
	// Let's get the logs of the Redpanda service
	s6ServicePath := filepath.Join(constants.S6BaseDir, s6ServiceName)
	logs, err := s.s6Service.GetLogs(ctx, s6ServicePath, filesystemService)
	if err != nil {
		if errors.Is(err, s6service.ErrServiceNotExist) {
			s.logger.Debugf("Service %s does not exist, returning empty logs", s6ServiceName)
			return ServiceInfo{}, ErrServiceNotExist
		} else if errors.Is(err, s6service.ErrLogFileNotFound) {
			s.logger.Debugf("Log file for service %s not found, returning empty logs", s6ServiceName)
			return ServiceInfo{}, ErrServiceNotExist
		} else {
			return ServiceInfo{}, fmt.Errorf("failed to get logs: %w", err)
		}
	}
	// Let's get the health check of the Redpanda service
	redpandaStatus, err := s.GetHealthCheckAndMetrics(ctx, tick, logs, filesystemService, redpandaName, loopStartTime)
	if err != nil {
		if strings.Contains(err.Error(), ErrServiceNoLogFile.Error()) {
			return ServiceInfo{
				S6ObservedState: s6ServiceObservedState,
				S6FSMState:      s6FSMState, // Note for state transitions: When a service is stopped and then reactivated,
				// this S6FSMState needs to be properly refreshed here.
				// Otherwise, the service can not transition from stopping to stopped state
				RedpandaStatus: RedpandaStatus{
					Logs: logs,
				},
			}, ErrServiceNoLogFile
		}
		if strings.Contains(err.Error(), monitor.ErrServiceConnectionRefused.Error()) {
			return ServiceInfo{
				S6ObservedState: s6ServiceObservedState,
				S6FSMState:      s6FSMState,
				RedpandaStatus:  redpandaStatus,
			}, monitor.ErrServiceConnectionRefused
		}

		if strings.Contains(err.Error(), ErrRedpandaMonitorNotRunning.Error()) {
			return ServiceInfo{
				S6ObservedState: s6ServiceObservedState,
				S6FSMState:      s6FSMState,
				RedpandaStatus:  redpandaStatus,
			}, ErrRedpandaMonitorNotRunning
		}

		if strings.Contains(err.Error(), ErrLastObservedStateNil.Error()) {
			return ServiceInfo{
				S6ObservedState: s6ServiceObservedState,
				S6FSMState:      s6FSMState,
				RedpandaStatus: RedpandaStatus{
					Logs: logs,
				},
			}, ErrLastObservedStateNil
		}

		if strings.Contains(err.Error(), "instance "+s6ServiceName+" not found") ||
			strings.Contains(err.Error(), "not found") {
			s.logger.Debugf("Service %s was removed during status check", s6ServiceName)
			return ServiceInfo{}, ErrServiceNotExist
		}

		return ServiceInfo{}, fmt.Errorf("failed to get health check: %w", err)
	}

	serviceInfo := ServiceInfo{
		S6ObservedState: s6ServiceObservedState,
		S6FSMState:      s6FSMState,
		RedpandaStatus:  redpandaStatus,
	}

	// set the logs to the service info
	// TODO: this is a hack to get the logs to the service info
	// we should find a better way to do this
	serviceInfo.RedpandaStatus.Logs = logs

	return serviceInfo, nil
}

// GetHealthCheckAndMetrics returns the health check and metrics of a Redpanda service
func (s *RedpandaService) GetHealthCheckAndMetrics(ctx context.Context, tick uint64, logs []s6service.LogEntry, filesystemService filesystem.Service, redpandaName string, loopStartTime time.Time) (RedpandaStatus, error) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(logger.ComponentRedpandaService, redpandaName, time.Since(start))
	}()

	if ctx.Err() != nil {
		return RedpandaStatus{}, ctx.Err()
	}

	// Skip health checks and metrics if the service doesn't exist yet
	// This avoids unnecessary errors in Status() when the service is still being created
	if _, exists := s.s6Manager.GetInstance(s.GetS6ServiceName(redpandaName)); !exists {
		return RedpandaStatus{
			HealthCheck: HealthCheck{
				IsLive:  false,
				IsReady: false,
			},
			RedpandaMetrics: redpanda_monitor.RedpandaMetrics{
				Metrics:      redpanda_monitor.Metrics{},
				MetricsState: nil,
			},
			Logs: []s6service.LogEntry{},
		}, nil
	}

	// Get the last observed state of the redpanda monitor
	s6ServiceName := s.GetS6ServiceName(redpandaName)
	lastObservedState, err := s.redpandaMonitorManager.GetLastObservedState(s6ServiceName)
	if err != nil {
		return RedpandaStatus{}, fmt.Errorf("failed to get redpanda status: %w", err)
	}

	if lastObservedState == nil {
		return RedpandaStatus{}, ErrLastObservedStateNil
	}

	// Convert the last observed state of the redpanda monitor
	lastRedpandaMonitorObservedState, ok := lastObservedState.(redpanda_monitor_fsm.RedpandaMonitorObservedState)
	if !ok {
		return RedpandaStatus{}, fmt.Errorf("observed state is not a RedpandaMonitorObservedState: %v", lastObservedState)
	}

	var redpandaStatus RedpandaStatus

	if lastRedpandaMonitorObservedState.ServiceInfo == nil {
		return RedpandaStatus{}, ErrLastObservedStateNil
	}

	// if everything is fine, set the status to the service info

	if lastRedpandaMonitorObservedState.ServiceInfo.RedpandaStatus.LastScan != nil {
		// Create health check structure
		healthCheck := HealthCheck{
			// Liveness is determined by a successful response
			IsLive: lastRedpandaMonitorObservedState.ServiceInfo.RedpandaStatus.LastScan.HealthCheck.IsLive,
			// IsReady is the same as IsLive, as there is no distinct logic for a redpanda monitor services readiness
			IsReady: lastRedpandaMonitorObservedState.ServiceInfo.RedpandaStatus.LastScan.HealthCheck.IsReady,
			// Redpanda version is constant
			Version: constants.RedpandaVersion,
		}

		redpandaStatus.HealthCheck = healthCheck

		// Check if RedpandaMetrics is not nil before dereferencing
		if lastRedpandaMonitorObservedState.ServiceInfo.RedpandaStatus.LastScan.RedpandaMetrics != nil {
			redpandaStatus.RedpandaMetrics = *lastRedpandaMonitorObservedState.ServiceInfo.RedpandaStatus.LastScan.RedpandaMetrics
		} else {
			// Set empty metrics if nil
			redpandaStatus.RedpandaMetrics = redpanda_monitor.RedpandaMetrics{
				Metrics:      redpanda_monitor.Metrics{},
				MetricsState: nil,
			}
		}

		redpandaStatus.Logs = lastRedpandaMonitorObservedState.ServiceInfo.RedpandaStatus.Logs
	} else {
		return RedpandaStatus{}, fmt.Errorf("last scan is nil")
	}

	// If the service is not running, we can return immediately
	// There wont be any logs, metrics, etc. to check
	if !lastRedpandaMonitorObservedState.ServiceInfo.RedpandaStatus.IsRunning {
		return RedpandaStatus{}, ErrRedpandaMonitorNotRunning
	}

	return redpandaStatus, nil
}

// AddRedpandaToS6Manager adds a Redpanda instance to the S6 manager
func (s *RedpandaService) AddRedpandaToS6Manager(ctx context.Context, cfg *redpandaserviceconfig.RedpandaServiceConfig, filesystemService filesystem.Service, redpandaName string) error {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Ensure the required directories exist
	if err := s.ensureRedpandaDirectories(ctx, cfg.BaseDir, filesystemService); err != nil {
		return err
	}

	s6ServiceName := s.GetS6ServiceName(redpandaName)

	// Check whether s6ServiceConfigs already contains an entry for this instance
	for _, s6Config := range s.s6ServiceConfigs {
		if s6Config.Name == s6ServiceName {
			return ErrServiceAlreadyExists
		}
	}

	// Generate the S6 config for this instance
	s6Config, err := s.GenerateS6ConfigForRedpanda(cfg, s6ServiceName)
	if err != nil {
		return fmt.Errorf("failed to generate S6 config for Redpanda service %s: %w", s6ServiceName, err)
	}

	// Create the S6 FSM config for this instance
	s6FSMConfig := config.S6FSMConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            s6ServiceName,
			DesiredFSMState: s6fsm.OperationalStateStopped, // Ensure we start with a stopped service, so we can start it later using the cfg
		},
		S6ServiceConfig: s6Config,
	}

	// Add the S6 FSM config to the list of S6 FSM configs
	s.s6ServiceConfigs = append(s.s6ServiceConfigs, s6FSMConfig)

	// Now do it the same with the redpanda monitor
	redpandaMonitorConfig := config.RedpandaMonitorConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            s6ServiceName,
			DesiredFSMState: redpanda_monitor_fsm.OperationalStateStopped, // Ensure we start with a stopped service, so we can start it later
		},
	}
	s.redpandaMonitorConfigs = append(s.redpandaMonitorConfigs, redpandaMonitorConfig)

	return nil
}

// ensureRedpandaDirectories creates all necessary directories for Redpanda
func (s *RedpandaService) ensureRedpandaDirectories(ctx context.Context, baseDir string, filesystemService filesystem.Service) error {
	if baseDir == "" {
		s.logger.Warnf("baseDir is empty, using default value %s", constants.DefaultRedpandaBaseDir)
		baseDir = constants.DefaultRedpandaBaseDir
	}

	// Ensure main data directory
	if err := filesystemService.EnsureDirectory(ctx, filepath.Join(baseDir, "redpanda")); err != nil {
		return fmt.Errorf("failed to ensure %s/redpanda directory exists: %w", baseDir, err)
	}

	// Ensure coredump directory
	// By default redpanda will generate coredumps when crashing
	if err := filesystemService.EnsureDirectory(ctx, filepath.Join(baseDir, "redpanda", "coredump")); err != nil {
		return fmt.Errorf("failed to ensure %s/redpanda/coredump directory exists: %w", baseDir, err)
	}

	return nil
}

// UpdateRedpandaInS6Manager updates an existing Redpanda instance in the S6 manager
func (s *RedpandaService) UpdateRedpandaInS6Manager(ctx context.Context, cfg *redpandaserviceconfig.RedpandaServiceConfig, redpandaName string) error {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	s6ServiceName := s.GetS6ServiceName(redpandaName)

	// Check if the service exists
	found := false
	index := -1
	for i, s6Config := range s.s6ServiceConfigs {
		if s6Config.Name == s6ServiceName {
			found = true
			index = i
			break
		}
	}

	if !found {
		return ErrServiceNotExist
	}

	// Generate the new S6 config for this instance
	s6Config, err := s.GenerateS6ConfigForRedpanda(cfg, s6ServiceName)
	if err != nil {
		return fmt.Errorf("failed to generate S6 config for Redpanda service %s: %w", s6ServiceName, err)
	}

	// Update the S6 FSM config for this instance
	currentDesiredState := s.s6ServiceConfigs[index].DesiredFSMState
	s.s6ServiceConfigs[index] = config.S6FSMConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            s6ServiceName,
			DesiredFSMState: currentDesiredState,
		},
		S6ServiceConfig: s6Config,
	}

	// Now update the redpanda monitor config
	redpandaMonitorDesiredState := redpanda_monitor_fsm.OperationalStateActive
	if currentDesiredState == s6fsm.OperationalStateStopped {
		redpandaMonitorDesiredState = redpanda_monitor_fsm.OperationalStateStopped
	}
	s.redpandaMonitorConfigs[index] = config.RedpandaMonitorConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            s6ServiceName,
			DesiredFSMState: redpandaMonitorDesiredState,
		},
	}

	return nil
}

// RemoveRedpandaFromS6Manager removes a Redpanda instance from the S6 manager
func (s *RedpandaService) RemoveRedpandaFromS6Manager(ctx context.Context, redpandaName string) error {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	s6ServiceName := s.GetS6ServiceName(redpandaName)

	found := false

	// Remove the S6 FSM config from the list of S6 FSM configs
	// so that the S6 manager will stop the service
	// The S6 manager itself will handle a graceful shutdown of the udnerlying S6 service
	for i, s6Config := range s.s6ServiceConfigs {
		if s6Config.Name == s6ServiceName {
			s.s6ServiceConfigs = append(s.s6ServiceConfigs[:i], s.s6ServiceConfigs[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		return ErrServiceNotExist
	}

	// Also remove the redpanda monitor from the S6 manager
	for i, redpandaMonitorConfig := range s.redpandaMonitorConfigs {
		if redpandaMonitorConfig.Name == s6ServiceName {
			s.redpandaMonitorConfigs = append(s.redpandaMonitorConfigs[:i], s.redpandaMonitorConfigs[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		return ErrServiceNotExist
	}

	return nil
}

// StartRedpanda starts a Redpanda instance
func (s *RedpandaService) StartRedpanda(ctx context.Context, redpandaName string) error {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	s6ServiceName := s.GetS6ServiceName(redpandaName)

	found := false

	// Set the desired state to running for the given instance
	for i, s6Config := range s.s6ServiceConfigs {
		if s6Config.Name == s6ServiceName {
			s.s6ServiceConfigs[i].DesiredFSMState = s6fsm.OperationalStateRunning
			found = true
			break
		}
	}

	if !found {
		return ErrServiceNotExist
	}
	// Reset found
	found = false

	// Also start the redpanda monitor
	for i, redpandaMonitorConfig := range s.redpandaMonitorConfigs {
		if redpandaMonitorConfig.Name == s6ServiceName {
			s.redpandaMonitorConfigs[i].DesiredFSMState = redpanda_monitor_fsm.OperationalStateActive
			found = true
			break
		}
	}

	if !found {
		return ErrServiceNotExist
	}

	return nil
}

// StopRedpanda stops a Redpanda instance
func (s *RedpandaService) StopRedpanda(ctx context.Context, redpandaName string) error {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	s6ServiceName := s.GetS6ServiceName(redpandaName)

	found := false

	// Set the desired state to stopped for the given instance
	for i, s6Config := range s.s6ServiceConfigs {
		if s6Config.Name == s6ServiceName {
			s.s6ServiceConfigs[i].DesiredFSMState = s6fsm.OperationalStateStopped
			found = true
			break
		}
	}

	if !found {
		return ErrServiceNotExist
	}
	// Reset found
	found = false

	// Also stop the redpanda monitor
	for i, redpandaMonitorConfig := range s.redpandaMonitorConfigs {
		if redpandaMonitorConfig.Name == s6ServiceName {
			s.redpandaMonitorConfigs[i].DesiredFSMState = redpanda_monitor_fsm.OperationalStateStopped
			found = true
			break
		}
	}

	if !found {
		return ErrServiceNotExist
	}

	return nil
}

// ReconcileManager reconciles the Redpanda manager
// This basically just calls the Reconcile method of the S6 manager, resulting in a (re)start of the Redpanda service with the latest configuration
func (s *RedpandaService) ReconcileManager(ctx context.Context, services serviceregistry.Provider, snapshot fsm.SystemSnapshot) (err error, reconciled bool) {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized"), false
	}

	if ctx.Err() != nil {
		return ctx.Err(), false
	}

	// Create a snapshot from the full config
	// Note: therefore, the S6 manager will not have access to the full observed state
	s6Snapshot := fsm.SystemSnapshot{
		CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: s.s6ServiceConfigs}},
		Tick:          snapshot.Tick,
	}

	s6Err, s6Reconciled := s.s6Manager.Reconcile(ctx, s6Snapshot, services)
	if s6Err != nil {
		return s6Err, false
	}

	// Also reconcile the redpanda monitor

	redpandaMonitorSnapshot := fsm.SystemSnapshot{
		CurrentConfig: config.FullConfig{Internal: config.InternalConfig{RedpandaMonitor: s.redpandaMonitorConfigs}},
		Tick:          snapshot.Tick,
	}

	monitorErr, monitorReconciled := s.redpandaMonitorManager.Reconcile(ctx, redpandaMonitorSnapshot, services)
	if monitorErr != nil {
		return fmt.Errorf("failed to reconcile redpanda monitor: %w", monitorErr), false
	}

	// Extract schema registry data from the SystemSnapshot
	// This replaces the previous approach of using yaml:"-" tags in RedpandaConfig
	dataModels := snapshot.CurrentConfig.DataModels
	dataContracts := snapshot.CurrentConfig.DataContracts
	payloadShapes := snapshot.CurrentConfig.PayloadShapes

	// Only reconcile schema registry when Redpanda is running
	// This prevents connection refused errors when the service is stopped
	redpandaConfig := snapshot.CurrentConfig.Internal.Redpanda
	if redpandaConfig.DesiredFSMState == "active" || redpandaConfig.DesiredFSMState == "idle" || redpandaConfig.DesiredFSMState == "degraded" {
		schemaRegistryErr := s.schemaRegistryManager.Reconcile(ctx, dataModels, dataContracts, payloadShapes)
		if schemaRegistryErr != nil {
			return fmt.Errorf("failed to reconcile schema registry: %w", schemaRegistryErr), false
		}
	}

	// If either was reconciled, indicate that reconciliation occurred
	return nil, s6Reconciled || monitorReconciled
}

// IsLogsFine reports true when recent Redpanda logs (within logWindow) contain
// no critical error patterns.
//
// It returns:
//
//	ok    – true when logs look clean, false otherwise.
//	entry – zero value when ok is true; otherwise the first offending log line.
func (s *RedpandaService) IsLogsFine(logs []s6service.LogEntry, currentTime time.Time, logWindow time.Duration, transitionToRunningTime time.Time) (bool, s6service.LogEntry) {
	// Check logs within the time window
	windowStart := currentTime.Add(-logWindow)

	for _, log := range logs {
		// Skip logs outside the time window
		// Note: This makes the window exclusive at the start boundary (logs exactly at windowStart are excluded)
		// and inclusive at the end boundary (up to currentTime)
		if log.Timestamp.Before(windowStart) {
			continue
		}

		// Check if the log contains failure, by applying all failure detectors (see redpanda_log_failures.go)
		for _, failureDetector := range RedpandaFailures {
			if failureDetector.IsFailure(log, transitionToRunningTime) {
				return false, log
			}
		}
	}

	return true, s6service.LogEntry{}
}

// IsMetricsErrorFree reports true when Redpanda metrics show no alerts or
// cluster‑level errors.
//
// It returns:
//
//	ok     – true when metrics are error‑free, false otherwise.
//	reason – empty when ok is true; otherwise a short explanation (e.g.
//	         "storage free space alert: <free>/<total> bytes").
func (s *RedpandaService) IsMetricsErrorFree(metrics redpanda_monitor.Metrics) (bool, string) {
	// Check output errors
	if metrics.Infrastructure.Storage.FreeSpaceAlert {
		return false, fmt.Sprintf("storage free space alert: %d free bytes (total: %d bytes)", metrics.Infrastructure.Storage.FreeBytes, metrics.Infrastructure.Storage.TotalBytes)
	}

	if metrics.Cluster.UnavailableTopics > 0 {
		return false, fmt.Sprintf("unavailable topics: %d", metrics.Cluster.UnavailableTopics)
	}

	return true, ""
}

// HasProcessingActivity reports true when Redpanda metrics state indicates
// active input/output throughput.
//
// It returns:
//
//	ok     – true when activity is detected, false otherwise.
//	reason – empty when ok is true; otherwise a brief throughput summary.
func (s *RedpandaService) HasProcessingActivity(status RedpandaStatus) (bool, string) {
	if status.RedpandaMetrics.MetricsState == nil {
		return false, ""
	}
	if status.RedpandaMetrics.MetricsState.IsActive {
		return true, ""
	}
	return false, fmt.Sprintf("no input bytes activity, (in=%.2f bytes/s, out=%.2f bytes/s)", status.RedpandaMetrics.MetricsState.Input.BytesPerTick, status.RedpandaMetrics.MetricsState.Output.BytesPerTick)
}

// ServiceExists checks if a Redpanda service exists
func (s *RedpandaService) ServiceExists(ctx context.Context, filesystemService filesystem.Service, benthosName string) bool {
	s6ServiceName := s.GetS6ServiceName(benthosName)
	s6ServicePath := filepath.Join(constants.S6BaseDir, s6ServiceName)

	exists, err := s.s6Service.ServiceExists(ctx, s6ServicePath, filesystemService)
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeError, s.logger, "Error checking if service exists for %s: %v", s6ServiceName, err)
		return false
	}

	return exists
}

// ForceRemoveRedpanda removes a Redpanda instance from the S6 manager
// This should only be called if the Redpanda instance is in a permanent failure state
// and the instance itself cannot be stopped or removed
func (s *RedpandaService) ForceRemoveRedpanda(ctx context.Context, filesystemService filesystem.Service, redpandaName string) error {
	s6ServiceName := s.GetS6ServiceName(redpandaName)
	s6ServicePath := filepath.Join(constants.S6BaseDir, s6ServiceName)
	return s.s6Service.ForceRemove(ctx, s6ServicePath, filesystemService)
}

func formatMemory(memory int) string {
	// Convert bytes to B, K, M, G, T
	units := []string{"B", "K", "M", "G", "T"}
	unitIndex := 0

	for memory >= 1024 {
		memory /= 1024
		unitIndex++
	}

	return fmt.Sprintf("%d%s", memory, units[unitIndex])
}

// setRedpandaClusterConfig sends a PUT request to update Redpanda's cluster config
func (s *RedpandaService) setRedpandaClusterConfig(ctx context.Context, configUpdates map[string]interface{}) (err error) {
	if s.httpClient == nil {
		return fmt.Errorf("http client not initialized")
	}

	// Construct the request body
	requestBody := map[string]interface{}{
		"upsert": configUpdates,
		"remove": []string{},
	}

	// Convert to JSON
	var jsonBody []byte
	jsonBody, err = json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Create the request
	var req *http.Request
	req, err = http.NewRequestWithContext(ctx, http.MethodPut, fmt.Sprintf("http://127.0.0.1:%d/v1/cluster_config", constants.AdminAPIPort), bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")

	// Send the request
	var resp *http.Response
	resp, err = s.httpClient.Do(req)
	if err != nil {
		// If we get a connection refused error, it means that the Redpanda service is not yet ready, so we just ignore it
		if strings.Contains(err.Error(), "connection refused") {
			return nil
		}
		return fmt.Errorf("failed to send request: %w", err)
	}
	if resp == nil {
		return fmt.Errorf("received nil response")
	}

	defer func() {
		if resp != nil {
			closeErr := resp.Body.Close()
			if closeErr != nil {
				err = fmt.Errorf("failed to close response body: %w", closeErr)
			}
		}
	}()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var response struct {
		ConfigVersion int `json:"config_version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}

// verifyRedpandaClusterConfig reads back the cluster config and verifies that the updates were applied correctly
func (s *RedpandaService) verifyRedpandaClusterConfig(ctx context.Context, redpandaName string, configUpdates map[string]interface{}) error {
	if s.httpClient == nil {
		return fmt.Errorf("http client not initialized")
	}

	readbackReq, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/v1/cluster_config", constants.AdminAPIPort), nil)
	if err != nil {
		return fmt.Errorf("failed to create readback request: %w", err)
	}

	readbackResp, err := s.httpClient.Do(readbackReq)
	if err != nil {
		// If we get a connection refused error, it means that the Redpanda service is not yet ready (or is in the progress of restarting), so we just ignore it
		if strings.Contains(err.Error(), "connection refused") {
			return nil
		}
		return fmt.Errorf("failed to send readback request: %w", err)
	}
	if readbackResp == nil {
		return fmt.Errorf("received nil readback response")
	}

	readbackBody, err := io.ReadAll(readbackResp.Body)
	if err != nil {
		return fmt.Errorf("failed to read readback response body: %w", err)
	}

	err = readbackResp.Body.Close()
	if err != nil {
		return fmt.Errorf("failed to close readback response body: %w", err)
	}

	// Parse as JSON
	var readbackConfig map[string]interface{}
	if err := json.Unmarshal(readbackBody, &readbackConfig); err != nil {
		return fmt.Errorf("failed to unmarshal readback response body: %w", err)
	}

	// Validate all keys against the readback config
	// Since the types of our request and the response might differ we need to handle the different types
	for key, value := range configUpdates {
		readbackValue, ok := readbackConfig[key]
		if !ok {
			s.logger.Debugf("Key %s not found in Redpanda cluster config for %s", key, redpandaName)
			return fmt.Errorf("key %s not found in Redpanda cluster config for %s", key, redpandaName)
		}

		// Convert both values to strings for comparison to handle type differences
		expectedStr, err := anyToString(value)
		if err != nil {
			s.logger.Debugf("Failed to convert expected value for key %s: %v", key, err)
			return fmt.Errorf("failed to convert expected value for key %s: %w", key, err)
		}

		actualStr, err := anyToString(readbackValue)
		if err != nil {
			s.logger.Debugf("Failed to convert actual value for key %s: %v", key, err)
			return fmt.Errorf("failed to convert actual value for key %s: %w", key, err)
		}

		if expectedStr != actualStr {
			s.logger.Debugf("Config verification failed for key %s: expected %s, got %s", key, expectedStr, actualStr)
			return fmt.Errorf("config verification failed for key %s: expected %s, got %s", key, expectedStr, actualStr)
		}
	}

	s.logger.Debugf("Config verification passed for %s", redpandaName)
	return nil
}

// UpdateRedpandaClusterConfig updates Redpanda's cluster configuration through its admin API.
// The function performs two key operations:
//  1. Sends a PUT request to update the cluster configuration
//  2. Verifies the changes by reading back and comparing the new values
//
// Note on restarts:
//   - This function does not directly trigger a Redpanda restart
//   - Restarts are handled by the reconcile loop when S6 service config changes
//   - This ensures all configuration changes are applied, including those requiring restarts
//
// The reconcile loop continues smoothly because:
//   - Configuration readback confirms changes are applied
//   - Redpanda metrics reflect updated values (therefore not blocking the loop)
//   - Both HTTP operations complete within the standard context timeout
func (s *RedpandaService) UpdateRedpandaClusterConfig(ctx context.Context, redpandaName string, configUpdates map[string]interface{}) error {
	// If the parent context is done, return an error
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Since we are doing 2 API calls, we just allocate double the time
	// This also helps smooth out the time, as one of them then can take slightly longer, but both still are bound by the same timeout
	innerCtx, innerCtxCancel := context.WithTimeout(ctx, constants.AdminAPITimeout*2)
	defer innerCtxCancel()

	// Set the cluster config
	s.logger.Debugf("Setting Redpanda cluster config: %v", configUpdates)
	if err := s.setRedpandaClusterConfig(innerCtx, configUpdates); err != nil {
		return err
	}

	// If the parent context is done, return an error
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Verify the config was applied correctly
	s.logger.Debugf("Verifying Redpanda cluster config: %v", configUpdates)
	return s.verifyRedpandaClusterConfig(innerCtx, redpandaName, configUpdates)
}

// anyToString converts common data types to string for easier comparison
func anyToString(input any) (result string, err error) {
	switch v := input.(type) {
	case string:
		return v, nil
	case int:
		return fmt.Sprintf("%d", v), nil
	case int64:
		return fmt.Sprintf("%d", v), nil
	case float64:
		// Check if it's actually an integer represented as float
		if v == float64(int64(v)) {
			return fmt.Sprintf("%.0f", v), nil
		}
		return fmt.Sprintf("%g", v), nil
	case float32:
		if v == float32(int32(v)) {
			return fmt.Sprintf("%.0f", v), nil
		}
		return fmt.Sprintf("%g", v), nil
	case bool:
		if v {
			return "true", nil
		}
		return "false", nil
	default:
		return result, fmt.Errorf("cannot convert %T to string", input)
	}
}
