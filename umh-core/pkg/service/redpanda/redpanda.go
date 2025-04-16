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
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/redpandaserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/httpclient"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda_monitor"
	"go.uber.org/zap"

	redpandayaml "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/redpandaserviceconfig"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

// IRedpandaService is the interface for managing Redpanda
type IRedpandaService interface {
	// GenerateS6ConfigForRedpanda generates a S6 config for a given redpanda instance
	GenerateS6ConfigForRedpanda(redpandaConfig *redpandaserviceconfig.RedpandaServiceConfig) (s6serviceconfig.S6ServiceConfig, error)
	// GetConfig returns the actual Redpanda config from the S6 service
	GetConfig(ctx context.Context, filesystemService filesystem.Service, tick uint64, loopStartTime time.Time) (redpandaserviceconfig.RedpandaServiceConfig, error)
	// Status checks the status of a Redpanda service
	Status(ctx context.Context, filesystemService filesystem.Service, tick uint64, loopStartTime time.Time) (ServiceInfo, error)
	// AddRedpandaToS6Manager adds a Redpanda instance to the S6 manager
	AddRedpandaToS6Manager(ctx context.Context, cfg *redpandaserviceconfig.RedpandaServiceConfig, filesystemService filesystem.Service) error
	// UpdateRedpandaInS6Manager updates an existing Redpanda instance in the S6 manager
	UpdateRedpandaInS6Manager(ctx context.Context, cfg *redpandaserviceconfig.RedpandaServiceConfig) error
	// RemoveRedpandaFromS6Manager removes a Redpanda instance from the S6 manager
	RemoveRedpandaFromS6Manager(ctx context.Context) error
	// StartRedpanda starts a Redpanda instance
	StartRedpanda(ctx context.Context) error
	// StopRedpanda stops a Redpanda instance
	StopRedpanda(ctx context.Context) error
	// ForceRemoveRedpanda removes a Redpanda instance from the S6 manager
	ForceRemoveRedpanda(ctx context.Context, filesystemService filesystem.Service) error
	// ServiceExists checks if a Redpanda service exists
	ServiceExists(ctx context.Context, filesystemService filesystem.Service) bool
	ReconcileManager(ctx context.Context, filesystemService filesystem.Service, tick uint64) (error, bool)
	// IsLogsFine checks if the logs of a Redpanda service are fine
	// Expects logs ([]s6service.LogEntry), currentTime (time.Time), and logWindow (time.Duration)
	IsLogsFine(logs []s6service.LogEntry, currentTime time.Time, logWindow time.Duration) bool
	// IsMetricsErrorFree checks if the metrics of a Redpanda service are error-free
	IsMetricsErrorFree(metrics redpanda_monitor.Metrics) bool
	// HasProcessingActivity checks if a Redpanda service has processing activity
	HasProcessingActivity(status RedpandaStatus) bool
}

// ServiceInfo contains information about a Redpanda service
type ServiceInfo struct {
	// S6ObservedState contains information about the S6 service
	S6ObservedState s6fsm.S6ObservedState
	// S6FSMState contains the current state of the S6 FSM
	S6FSMState string
	// RedpandaStatus contains information about the status of the Redpanda service
	RedpandaStatus RedpandaStatus
}

// RedpandaStatus contains information about the status of the Redpanda service
type RedpandaStatus struct {
	// HealthCheck contains information about the health of the Redpanda service
	HealthCheck HealthCheck
	// Logs contains the logs of the Redpanda service
	Logs []s6service.LogEntry
	// RedpandaMetrics contains information about the metrics of the Redpanda service
	RedpandaMetrics redpanda_monitor.RedpandaMetrics
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
	logger           *zap.SugaredLogger
	s6Manager        *s6fsm.S6Manager
	s6Service        s6service.Service // S6 service for direct S6 operations
	s6ServiceConfigs []config.S6FSMConfig
	httpClient       httpclient.HTTPClient
	baseDir          string
	lastStatus       RedpandaStatus
	metricsService   redpanda_monitor.IRedpandaMonitorService
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

// WithMonitorService sets a custom monitor service for the RedpandaService
func WithMonitorService(monitorService redpanda_monitor.IRedpandaMonitorService) RedpandaServiceOption {
	return func(s *RedpandaService) {
		s.metricsService = monitorService
	}
}

// NewDefaultRedpandaService creates a new default Redpanda service
// name is the name of the Redpanda service as defined in the UMH config
func NewDefaultRedpandaService(redpandaName string, opts ...RedpandaServiceOption) *RedpandaService {
	managerName := fmt.Sprintf("%s%s", logger.ComponentRedpandaService, redpandaName)
	service := &RedpandaService{
		logger:         logger.For(managerName),
		s6Manager:      s6fsm.NewS6Manager(managerName),
		s6Service:      s6service.NewDefaultService(),
		httpClient:     nil, // this is only for a mock in the tests
		baseDir:        constants.DefaultRedpandaBaseDir,
		metricsService: redpanda_monitor.NewRedpandaMonitorService(),
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

	return redpandayaml.RenderRedpandaYAML(config.Topic.DefaultTopicRetentionMs, config.Topic.DefaultTopicRetentionBytes)
}

// generateS6ConfigForRedpanda creates a S6 config for a given redpanda instance
// Expects s6ServiceName (e.g. "redpanda-myservice"), not the raw redpandaName
func (s *RedpandaService) GenerateS6ConfigForRedpanda(redpandaConfig *redpandaserviceconfig.RedpandaServiceConfig) (s6Config s6serviceconfig.S6ServiceConfig, err error) {
	configPath := fmt.Sprintf("%s/%s/config/%s", constants.S6BaseDir, constants.RedpandaServiceName, constants.RedpandaConfigFileName)

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

// GetConfig returns the actual Redpanda config from the S6 service
func (s *RedpandaService) GetConfig(ctx context.Context, filesystemService filesystem.Service, tick uint64, loopStartTime time.Time) (redpandaserviceconfig.RedpandaServiceConfig, error) {
	if ctx.Err() != nil {
		return redpandaserviceconfig.RedpandaServiceConfig{}, ctx.Err()
	}

	status, err := s.metricsService.Status(ctx, filesystemService, tick)
	if err != nil {
		return redpandaserviceconfig.RedpandaServiceConfig{}, fmt.Errorf("failed to get redpanda status: %w", err)
	}

	// If this is nil, we have not yet tried to scan for metrics and config, or there has been an error (but that one will be cached in the above Status() return)
	if status.RedpandaStatus.LastScan == nil {
		return redpandaserviceconfig.RedpandaServiceConfig{}, fmt.Errorf("last scan is nil")
	}

	// Check that the last scan is not older then RedpandaMaxMetricsAndConfigAge
	if loopStartTime.Sub(status.RedpandaStatus.LastScan.LastUpdatedAt) > constants.RedpandaMaxMetricsAndConfigAge {
		s.logger.Warnf("last scan is %s old, returning empty status", loopStartTime.Sub(status.RedpandaStatus.LastScan.LastUpdatedAt))
		return redpandaserviceconfig.RedpandaServiceConfig{}, fmt.Errorf("last scan is older than %s", constants.RedpandaMaxMetricsAndConfigAge)
	}

	lastScan := status.RedpandaStatus.LastScan

	var result redpandaserviceconfig.RedpandaServiceConfig
	result.Topic.DefaultTopicRetentionMs = lastScan.ClusterConfig.Topic.DefaultTopicRetentionMs
	result.Topic.DefaultTopicRetentionBytes = lastScan.ClusterConfig.Topic.DefaultTopicRetentionBytes

	// Safely extract MaxCores - this might not be available from the API
	// so we'll use a default value
	result.Resources.MaxCores = 1
	result.Resources.MemoryPerCoreInBytes = 2048 * 1024 * 1024 // 2GB

	return redpandayaml.NormalizeRedpandaConfig(result), nil
}

// Status checks the status of a Redpanda service
func (s *RedpandaService) Status(ctx context.Context, filesystemService filesystem.Service, tick uint64, loopStartTime time.Time) (ServiceInfo, error) {
	if ctx.Err() != nil {
		return ServiceInfo{}, ctx.Err()
	}

	// First, check if the service exists in the S6 manager
	// This is a crucial check that prevents "instance not found" errors
	// during reconciliation when a service is being created or removed
	if _, exists := s.s6Manager.GetInstance(constants.RedpandaServiceName); !exists {
		s.logger.Debugf("Service %s not found in S6 manager", constants.RedpandaServiceName)
		return ServiceInfo{}, ErrServiceNotExist
	}

	// Let's get the status of the underlying s6 service
	s6ServiceObservedStateRaw, err := s.s6Manager.GetLastObservedState(constants.RedpandaServiceName)
	if err != nil {
		// If we still get an "instance not found" error despite our earlier check,
		// it's likely that the service was removed between our check and this call
		if strings.Contains(err.Error(), "instance "+constants.RedpandaServiceName+" not found") ||
			strings.Contains(err.Error(), "not found") {
			s.logger.Debugf("Service %s was removed during status check", constants.RedpandaServiceName)
			return ServiceInfo{}, ErrServiceNotExist
		}
		return ServiceInfo{}, fmt.Errorf("failed to get last observed state: %w", err)
	}
	s6ServiceObservedState, ok := s6ServiceObservedStateRaw.(s6fsm.S6ObservedState)
	if !ok {
		return ServiceInfo{}, fmt.Errorf("observed state is not a S6ObservedState: %v", s6ServiceObservedStateRaw)
	}

	// Let's get the current FSM state of the underlying s6 FSM
	s6FSMState, err := s.s6Manager.GetCurrentFSMState(constants.RedpandaServiceName)
	if err != nil {
		// Similar to above, if the service was removed during our check
		if strings.Contains(err.Error(), "instance "+constants.RedpandaServiceName+" not found") ||
			strings.Contains(err.Error(), "not found") {
			s.logger.Debugf("Service %s was removed during status check", constants.RedpandaServiceName)
			return ServiceInfo{}, ErrServiceNotExist
		}
		return ServiceInfo{}, fmt.Errorf("failed to get current FSM state: %w", err)
	}

	// Let's get the logs of the Redpanda service
	s6ServicePath := filepath.Join(constants.S6BaseDir, constants.RedpandaServiceName)
	logs, err := s.s6Service.GetLogs(ctx, s6ServicePath, filesystemService)
	if err != nil {
		if errors.Is(err, s6service.ErrServiceNotExist) {
			s.logger.Debugf("Service %s does not exist, returning empty logs", constants.RedpandaServiceName)
			return ServiceInfo{}, ErrServiceNotExist
		} else if errors.Is(err, s6service.ErrLogFileNotFound) {
			s.logger.Debugf("Log file for service %s not found, returning empty logs", constants.RedpandaServiceName)
			return ServiceInfo{}, ErrServiceNotExist
		} else {
			return ServiceInfo{}, fmt.Errorf("failed to get logs: %w", err)
		}
	}
	// Let's get the health check of the Redpanda service
	redpandaStatus, err := s.GetHealthCheckAndMetrics(ctx, tick, logs, filesystemService, loopStartTime)
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
		if strings.Contains(err.Error(), redpanda_monitor.ErrServiceConnectionRefused.Error()) {
			return ServiceInfo{
				S6ObservedState: s6ServiceObservedState,
				S6FSMState:      s6FSMState,
				RedpandaStatus:  redpandaStatus,
			}, redpanda_monitor.ErrServiceConnectionRefused
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
func (s *RedpandaService) GetHealthCheckAndMetrics(ctx context.Context, tick uint64, logs []s6service.LogEntry, filesystemService filesystem.Service, loopStartTime time.Time) (RedpandaStatus, error) {

	s.logger.Infof("Getting health check and metrics for tick %d", tick)
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(logger.ComponentRedpandaService, constants.RedpandaServiceName, time.Since(start))
	}()

	if ctx.Err() != nil {
		return RedpandaStatus{}, ctx.Err()
	}

	// Skip health checks and metrics if the service doesn't exist yet
	// This avoids unnecessary errors in Status() when the service is still being created
	if _, exists := s.s6Manager.GetInstance(constants.RedpandaServiceName); !exists {
		s.lastStatus = RedpandaStatus{
			HealthCheck: HealthCheck{
				IsLive:  false,
				IsReady: false,
			},
			RedpandaMetrics: redpanda_monitor.RedpandaMetrics{
				Metrics:      redpanda_monitor.Metrics{},
				MetricsState: nil,
			},
			Logs: []s6service.LogEntry{},
		}
		return s.lastStatus, nil
	}

	redpandaStatus, err := s.metricsService.Status(ctx, filesystemService, tick)
	if err != nil {
		return RedpandaStatus{}, fmt.Errorf("failed to get redpanda status: %w", err)
	}

	// If this is nil, we have not yet tried to scan for metrics and config, or there has been an error (but that one will be cached in the above Status() return)
	if redpandaStatus.RedpandaStatus.LastScan == nil {
		return RedpandaStatus{}, fmt.Errorf("last scan is nil")
	}

	// Check that the last scan is not older then RedpandaMaxMetricsAndConfigAge
	if loopStartTime.Sub(redpandaStatus.RedpandaStatus.LastScan.LastUpdatedAt) > constants.RedpandaMaxMetricsAndConfigAge {
		s.logger.Warnf("last scan is %s old, returning empty status", loopStartTime.Sub(redpandaStatus.RedpandaStatus.LastScan.LastUpdatedAt))
		return RedpandaStatus{}, fmt.Errorf("last scan is older than %s", constants.RedpandaMaxMetricsAndConfigAge)
	}

	// Create health check structure
	healthCheck := HealthCheck{
		// Liveness is determined by a successful response
		IsLive: redpandaStatus.RedpandaStatus.IsRunning,
		// IsReady is the same as IsLive, as there is no distinct logic for a redpanda monitor services readiness
		IsReady: redpandaStatus.RedpandaStatus.IsRunning,
		// Redpanda version is constant
		Version: constants.RedpandaVersion,
	}

	s.lastStatus = RedpandaStatus{
		HealthCheck:     healthCheck,
		RedpandaMetrics: *redpandaStatus.RedpandaStatus.LastScan.Metrics,
		Logs:            logs,
	}

	return s.lastStatus, nil
}

// AddRedpandaToS6Manager adds a Redpanda instance to the S6 manager
func (s *RedpandaService) AddRedpandaToS6Manager(ctx context.Context, cfg *redpandaserviceconfig.RedpandaServiceConfig, filesystemService filesystem.Service) error {
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

	s6ServiceName := constants.RedpandaServiceName

	// Check whether s6ServiceConfigs already contains an entry for this instance
	for _, s6Config := range s.s6ServiceConfigs {
		if s6Config.Name == s6ServiceName {
			return ErrServiceAlreadyExists
		}
	}

	// Generate the S6 config for this instance
	s6Config, err := s.GenerateS6ConfigForRedpanda(cfg)
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

	// Also add the redpanda monitor to the S6 manager
	// This function is idempotent, so it's safe to call it multiple times
	if err := s.metricsService.AddRedpandaMonitorToS6Manager(ctx); err != nil {
		if !errors.Is(err, redpanda_monitor.ErrServiceAlreadyExists) {
			return fmt.Errorf("failed to add redpanda monitor to S6 manager: %w", err)
		}
	}

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
func (s *RedpandaService) UpdateRedpandaInS6Manager(ctx context.Context, cfg *redpandaserviceconfig.RedpandaServiceConfig) error {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	s6ServiceName := constants.RedpandaServiceName

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
	s6Config, err := s.GenerateS6ConfigForRedpanda(cfg)
	if err != nil {
		return fmt.Errorf("failed to generate S6 config for Redpanda service %s: %w", s6ServiceName, err)
	}

	// Update the S6 FSM config for this instance
	s.s6ServiceConfigs[index].S6ServiceConfig = s6Config

	return nil
}

// RemoveRedpandaFromS6Manager removes a Redpanda instance from the S6 manager
func (s *RedpandaService) RemoveRedpandaFromS6Manager(ctx context.Context) error {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	s6ServiceName := constants.RedpandaServiceName

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
	if err := s.metricsService.RemoveRedpandaMonitorFromS6Manager(ctx); err != nil {
		if !errors.Is(err, redpanda_monitor.ErrServiceNotExist) {
			return fmt.Errorf("failed to remove redpanda monitor from S6 manager: %w", err)
		}
	}

	return nil
}

// StartRedpanda starts a Redpanda instance
func (s *RedpandaService) StartRedpanda(ctx context.Context) error {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	s6ServiceName := constants.RedpandaServiceName

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

	return nil
}

// StopRedpanda stops a Redpanda instance
func (s *RedpandaService) StopRedpanda(ctx context.Context) error {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	s6ServiceName := constants.RedpandaServiceName

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

	return nil
}

// ReconcileManager reconciles the Redpanda manager
// This basically just calls the Reconcile method of the S6 manager, resulting in a (re)start of the Redpanda service with the latest configuration
func (s *RedpandaService) ReconcileManager(ctx context.Context, filesystemService filesystem.Service, tick uint64) (err error, reconciled bool) {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized"), false
	}

	if ctx.Err() != nil {
		return ctx.Err(), false
	}

	s6Err, s6Reconciled := s.s6Manager.Reconcile(ctx, config.FullConfig{Internal: config.InternalConfig{Services: s.s6ServiceConfigs}}, filesystemService, tick)
	if s6Err != nil {
		return s6Err, false
	}

	// Also reconcile the redpanda monitor service
	monitorErr, monitorReconciled := s.metricsService.ReconcileManager(ctx, filesystemService, tick)
	if monitorErr != nil {
		return monitorErr, false
	}

	// If either was reconciled, indicate that reconciliation occurred
	return nil, s6Reconciled || monitorReconciled
}

// IsLogsFine analyzes Redpanda logs to determine if there are any critical issues
func (s *RedpandaService) IsLogsFine(logs []s6service.LogEntry, currentTime time.Time, logWindow time.Duration) bool {
	failures := []string{
		"Address already in use", // https://github.com/redpanda-data/redpanda/issues/3763
		"Reactor stalled for",    // Multiple sources
	}

	// Check logs within the time window
	windowStart := currentTime.Add(-logWindow)

	for _, log := range logs {
		// Skip logs outside the time window
		// Note: This makes the window exclusive at the start boundary (logs exactly at windowStart are excluded)
		// and inclusive at the end boundary (up to currentTime)
		if log.Timestamp.Before(windowStart) {
			continue
		}

		for _, failure := range failures {
			if strings.Contains(log.Content, failure) {
				return false
			}
		}
	}
	return true
}

// IsMetricsErrorFree checks if the metrics of a Redpanda service are error-free
func (s *RedpandaService) IsMetricsErrorFree(metrics redpanda_monitor.Metrics) bool {
	// Check output errors
	if metrics.Infrastructure.Storage.FreeSpaceAlert {
		return false
	}

	if metrics.Cluster.UnavailableTopics > 0 {
		return false
	}

	return true
}

// HasProcessingActivity checks if a Redpanda service has processing activity
func (s *RedpandaService) HasProcessingActivity(status RedpandaStatus) bool {
	return status.RedpandaMetrics.MetricsState != nil && status.RedpandaMetrics.MetricsState.IsActive
}

// ServiceExists checks if a Redpanda service exists
func (s *RedpandaService) ServiceExists(ctx context.Context, filesystemService filesystem.Service) bool {
	s6ServiceName := constants.RedpandaServiceName
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
func (s *RedpandaService) ForceRemoveRedpanda(ctx context.Context, filesystemService filesystem.Service) error {
	return s.s6Service.ForceRemove(ctx, constants.RedpandaServiceName, filesystemService)
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
