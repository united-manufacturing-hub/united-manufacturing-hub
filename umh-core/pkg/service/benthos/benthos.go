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

package benthos

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"go.uber.org/zap"

	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	"gopkg.in/yaml.v3"

	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

// IBenthosService is the interface for managing Benthos services
type IBenthosService interface {
	// GenerateS6ConfigForBenthos generates a S6 config for a given benthos instance
	// Expects s6ServiceName (e.g. "benthos-myservice"), not the raw benthosName
	GenerateS6ConfigForBenthos(benthosConfig *benthosserviceconfig.BenthosServiceConfig, s6ServiceName string) (s6serviceconfig.S6ServiceConfig, error)
	// GetConfig returns the actual Benthos config from the S6 service
	// Expects benthosName (e.g. "myservice") as defined in the UMH config
	GetConfig(ctx context.Context, filesystemService filesystem.Service, benthosName string) (benthosserviceconfig.BenthosServiceConfig, error)
	// Status checks the status of a Benthos service
	// Expects benthosName (e.g. "myservice") as defined in the UMH config
	Status(ctx context.Context, filesystemService filesystem.Service, benthosName string, metricsPort int, tick uint64, loopStartTime time.Time) (ServiceInfo, error)
	// AddBenthosToS6Manager adds a Benthos instance to the S6 manager
	// Expects benthosName (e.g. "myservice") as defined in the UMH config
	AddBenthosToS6Manager(ctx context.Context, filesystemService filesystem.Service, cfg *benthosserviceconfig.BenthosServiceConfig, benthosName string) error
	// UpdateBenthosInS6Manager updates an existing Benthos instance in the S6 manager
	// Expects benthosName (e.g. "myservice") as defined in the UMH config
	UpdateBenthosInS6Manager(ctx context.Context, filesystemService filesystem.Service, cfg *benthosserviceconfig.BenthosServiceConfig, benthosName string) error
	// RemoveBenthosFromS6Manager removes a Benthos instance from the S6 manager
	// Expects benthosName (e.g. "myservice") as defined in the UMH config
	RemoveBenthosFromS6Manager(ctx context.Context, filesystemService filesystem.Service, benthosName string) error
	// StartBenthos starts a Benthos instance
	// Expects benthosName (e.g. "myservice") as defined in the UMH config
	StartBenthos(ctx context.Context, filesystemService filesystem.Service, benthosName string) error
	// StopBenthos stops a Benthos instance
	// Expects benthosName (e.g. "myservice") as defined in the UMH config
	StopBenthos(ctx context.Context, filesystemService filesystem.Service, benthosName string) error
	// ForceRemoveBenthos removes a Benthos instance from the S6 manager
	// Expects benthosName (e.g. "myservice") as defined in the UMH config
	ForceRemoveBenthos(ctx context.Context, filesystemService filesystem.Service, benthosName string) error
	// ServiceExists checks if a Benthos service exists
	// Expects benthosName (e.g. "myservice") as defined in the UMH config
	ServiceExists(ctx context.Context, filesystemService filesystem.Service, benthosName string) bool
	// ReconcileManager reconciles the Benthos manager
	// Expects tick (uint64) as the current tick
	ReconcileManager(ctx context.Context, filesystemService filesystem.Service, tick uint64) (error, bool)
	// IsLogsFine checks if the logs of a Benthos service are fine
	// Expects logs ([]s6service.LogEntry), currentTime (time.Time), and logWindow (time.Duration)
	IsLogsFine(logs []s6service.LogEntry, currentTime time.Time, logWindow time.Duration) bool
	// IsMetricsErrorFree checks if the metrics of a Benthos service are error-free
	IsMetricsErrorFree(metrics benthos_monitor.BenthosMetrics) bool
	// HasProcessingActivity checks if a Benthos service has processing activity
	HasProcessingActivity(status BenthosStatus) bool
}

// ServiceInfo contains information about a Benthos service
type ServiceInfo struct {
	// S6ObservedState contains information about the S6 service
	S6ObservedState s6fsm.S6ObservedState
	// S6FSMState contains the current state of the S6 FSM
	S6FSMState string
	// BenthosStatus contains information about the status of the Benthos service
	BenthosStatus BenthosStatus
}

type BenthosStatus struct {
	// HealthCheck contains information about the health of the Benthos service
	HealthCheck benthos_monitor.HealthCheck
	// BenthosMetrics contains information about the metrics of the Benthos service
	BenthosMetrics benthos_monitor.BenthosMetrics
	// BenthosLogs contains the logs of the Benthos service
	BenthosLogs []s6service.LogEntry
}

// BenthosService is the default implementation of the IBenthosService interface
type BenthosService struct {
	logger           *zap.SugaredLogger
	s6Manager        *s6fsm.S6Manager
	s6Service        s6service.Service // S6 service for direct S6 operations
	s6ServiceConfigs []config.S6FSMConfig
	metricsService   benthos_monitor.IBenthosMonitorService
	lastStatus       BenthosStatus
}

// BenthosServiceOption is a function that modifies a BenthosService
type BenthosServiceOption func(*BenthosService)

// WithS6Service sets a custom S6 service for the BenthosService
func WithS6Service(s6Service s6service.Service) BenthosServiceOption {
	return func(s *BenthosService) {
		s.s6Service = s6Service
	}
}

// WithMonitorService sets a custom monitor service for the BenthosService
func WithMonitorService(monitorService benthos_monitor.IBenthosMonitorService) BenthosServiceOption {
	return func(s *BenthosService) {
		s.metricsService = monitorService
	}
}

// NewDefaultBenthosService creates a new default Benthos service
// name is the name of the Benthos service as defined in the UMH config
func NewDefaultBenthosService(benthosName string, opts ...BenthosServiceOption) *BenthosService {
	managerName := fmt.Sprintf("%s%s", logger.ComponentBenthosService, benthosName)
	service := &BenthosService{
		logger:         logger.For(managerName),
		s6Manager:      s6fsm.NewS6Manager(managerName),
		s6Service:      s6service.NewDefaultService(),
		metricsService: benthos_monitor.NewBenthosMonitorService(benthosName),
	}

	// Apply options
	for _, opt := range opts {
		opt(service)
	}

	return service
}

// generateBenthosYaml generates a Benthos YAML configuration from a BenthosServiceConfig
func (s *BenthosService) generateBenthosYaml(config *benthosserviceconfig.BenthosServiceConfig) (string, error) {
	if config == nil {
		return "", fmt.Errorf("config is nil")
	}

	if config.LogLevel == "" {
		config.LogLevel = "INFO"
	}

	return benthosserviceconfig.RenderBenthosYAML(
		config.Input,
		config.Output,
		config.Pipeline,
		config.CacheResources,
		config.RateLimitResources,
		config.Buffer,
		config.MetricsPort,
		config.LogLevel,
	)
}

// getS6ServiceName converts a benthosName (e.g. "myservice") to its S6 service name (e.g. "benthos-myservice")
func (s *BenthosService) getS6ServiceName(benthosName string) string {
	return fmt.Sprintf("benthos-%s", benthosName)
}

// generateS6ConfigForBenthos creates a S6 config for a given benthos instance
// Expects s6ServiceName (e.g. "benthos-myservice"), not the raw benthosName
func (s *BenthosService) GenerateS6ConfigForBenthos(benthosConfig *benthosserviceconfig.BenthosServiceConfig, s6ServiceName string) (s6Config s6serviceconfig.S6ServiceConfig, err error) {
	configPath := fmt.Sprintf("%s/%s/config/%s", constants.S6BaseDir, s6ServiceName, constants.BenthosConfigFileName)

	yamlConfig, err := s.generateBenthosYaml(benthosConfig)
	if err != nil {
		return s6serviceconfig.S6ServiceConfig{}, err
	}

	s6Config = s6serviceconfig.S6ServiceConfig{
		Command: []string{
			"/usr/local/bin/benthos",
			"-c",
			configPath,
		},
		Env: map[string]string{},
		ConfigFiles: map[string]string{
			constants.BenthosConfigFileName: yamlConfig,
		},
	}

	return s6Config, nil
}

// GetConfig returns the actual Benthos config from the S6 service
// Expects benthosName (e.g. "myservice") as defined in the UMH config
func (s *BenthosService) GetConfig(ctx context.Context, filesystemService filesystem.Service, benthosName string) (benthosserviceconfig.BenthosServiceConfig, error) {
	if ctx.Err() != nil {
		return benthosserviceconfig.BenthosServiceConfig{}, ctx.Err()
	}

	s6ServiceName := s.getS6ServiceName(benthosName)
	s6ServicePath := filepath.Join(constants.S6BaseDir, s6ServiceName)

	// Request the config file from the S6 service
	yamlData, err := s.s6Service.GetS6ConfigFile(ctx, s6ServicePath, constants.BenthosConfigFileName, filesystemService)
	if err != nil {
		return benthosserviceconfig.BenthosServiceConfig{}, fmt.Errorf("failed to get benthos config file for service %s: %w", s6ServiceName, err)
	}

	// Parse the YAML into a config map
	var benthosConfig map[string]interface{}
	if err := yaml.Unmarshal(yamlData, &benthosConfig); err != nil {
		return benthosserviceconfig.BenthosServiceConfig{}, fmt.Errorf("error parsing benthos config file for service %s: %w", s6ServiceName, err)
	}

	// Extract sections into BenthosServiceConfig struct
	result := benthosserviceconfig.BenthosServiceConfig{}

	// Safely extract input config
	if inputConfig, ok := benthosConfig["input"].(map[string]interface{}); ok {
		result.Input = inputConfig
	}

	// Safely extract pipeline config
	if pipelineConfig, ok := benthosConfig["pipeline"].(map[string]interface{}); ok {
		result.Pipeline = pipelineConfig
	}

	// Safely extract output config
	if outputConfig, ok := benthosConfig["output"].(map[string]interface{}); ok {
		result.Output = outputConfig
	}

	// Safely extract buffer config
	if bufferConfig, ok := benthosConfig["buffer"].(map[string]interface{}); ok {
		result.Buffer = bufferConfig
	}

	// Safely extract cache_resources
	if cacheResources, ok := benthosConfig["cache_resources"].([]interface{}); ok {
		for _, res := range cacheResources {
			if resMap, ok := res.(map[string]interface{}); ok {
				result.CacheResources = append(result.CacheResources, resMap)
			}
		}
	}

	// Safely extract rate_limit_resources
	if rateLimitResources, ok := benthosConfig["rate_limit_resources"].([]interface{}); ok {
		for _, res := range rateLimitResources {
			if resMap, ok := res.(map[string]interface{}); ok {
				result.RateLimitResources = append(result.RateLimitResources, resMap)
			}
		}
	}

	// Safely extract metrics_port using helper function
	result.MetricsPort = s.extractMetricsPort(benthosConfig)

	// Safely extract log_level
	if logger, ok := benthosConfig["logger"].(map[string]interface{}); ok {
		if level, ok := logger["level"].(string); ok {
			result.LogLevel = level
		}
	}

	// Normalize the config to ensure consistent defaults
	return benthosserviceconfig.NormalizeBenthosConfig(result), nil
}

// extractMetricsPort safely extracts the metrics port from the config map
// Returns 0 if any part of the path is missing or invalid
func (s *BenthosService) extractMetricsPort(config map[string]interface{}) int {
	// Check each level of nesting
	metrics, ok := config["metrics"].(map[string]interface{})
	if !ok {
		return 0
	}

	http, ok := metrics["http"].(map[string]interface{})
	if !ok {
		return 0
	}

	address, ok := http["address"].(string)
	if !ok {
		return 0
	}

	// Parse port from address string (e.g., ":4195" or "0.0.0.0:4195")
	parts := strings.Split(address, ":")
	if len(parts) < 2 {
		return 0
	}

	portStr := parts[len(parts)-1]
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0
	}

	return port
}

// Status checks the status of a Benthos service and returns ServiceInfo
// Expects benthosName (e.g. "myservice") as defined in the UMH config
func (s *BenthosService) Status(ctx context.Context, filesystemService filesystem.Service, benthosName string, metricsPort int, tick uint64, loopStartTime time.Time) (ServiceInfo, error) {
	if ctx.Err() != nil {
		return ServiceInfo{}, ctx.Err()
	}

	s6ServiceName := s.getS6ServiceName(benthosName)

	// First, check if the service exists in the S6 manager
	// This is a crucial check that prevents "instance not found" errors
	// during reconciliation when a service is being created or removed
	if _, exists := s.s6Manager.GetInstance(s6ServiceName); !exists {
		s.logger.Debugf("Service %s not found in S6 manager", s6ServiceName)
		return ServiceInfo{}, ErrServiceNotExist
	}

	// Let's get the status of the underlying s6 service
	s6ServiceObservedStateRaw, err := s.s6Manager.GetLastObservedState(s6ServiceName)
	if err != nil {
		// If we still get an "instance not found" error despite our earlier check,
		// it's likely that the service was removed between our check and this call
		if strings.Contains(err.Error(), "instance "+s6ServiceName+" not found") ||
			strings.Contains(err.Error(), "not found") {
			s.logger.Debugf("Service %s was removed during status check", s6ServiceName)
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

	// Let's get the logs of the Benthos service
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

	benthosStatus, err := s.GetHealthCheckAndMetrics(ctx, filesystemService, tick, loopStartTime, benthosName, logs)
	if err != nil {
		if strings.Contains(err.Error(), benthos_monitor.ErrServiceNoLogFile.Error()) {
			return ServiceInfo{
				S6ObservedState: s6ServiceObservedState,
				S6FSMState:      s6FSMState, // Note for state transitions: When a service is stopped and then reactivated,
				// this S6FSMState needs to be properly refreshed here.
				// Otherwise, the service can not transition from stopping to stopped state
				BenthosStatus: BenthosStatus{
					BenthosLogs: logs,
				},
			}, ErrServiceNoLogFile
		}
		if strings.Contains(err.Error(), ErrHealthCheckConnectionRefused.Error()) {
			return ServiceInfo{
				S6ObservedState: s6ServiceObservedState,
				S6FSMState:      s6FSMState,
				BenthosStatus:   benthosStatus,
			}, ErrHealthCheckConnectionRefused
		}

		return ServiceInfo{}, fmt.Errorf("failed to get health check: %w", err)
	}

	serviceInfo := ServiceInfo{
		S6ObservedState: s6ServiceObservedState,
		S6FSMState:      s6FSMState,
		BenthosStatus:   benthosStatus,
	}

	// set the logs to the service info
	// TODO: this is a hack to get the logs to the service info
	// we should find a better way to do this
	serviceInfo.BenthosStatus.BenthosLogs = logs

	return serviceInfo, nil
}

func (s *BenthosService) GetHealthCheckAndMetrics(ctx context.Context, filesystemService filesystem.Service, tick uint64, loopStartTime time.Time, benthosName string, logs []s6service.LogEntry) (BenthosStatus, error) {

	s.logger.Debugf("Getting health check and metrics for tick %d", tick)
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(logger.ComponentBenthosService, metrics.ComponentBenthosService+"_get_health_check_and_metrics", time.Since(start))
	}()

	if ctx.Err() != nil {
		return BenthosStatus{}, ctx.Err()
	}

	// Skip health checks and metrics if the service doesn't exist yet
	// This avoids unnecessary errors in Status() when the service is still being created
	if _, exists := s.s6Manager.GetInstance(s.getS6ServiceName(benthosName)); !exists {
		s.lastStatus = BenthosStatus{
			HealthCheck: benthos_monitor.HealthCheck{
				IsLive:  false,
				IsReady: false,
			},
			BenthosMetrics: benthos_monitor.BenthosMetrics{
				Metrics:      benthos_monitor.Metrics{},
				MetricsState: nil,
			},
			BenthosLogs: []s6service.LogEntry{},
		}
		return s.lastStatus, nil
	}

	// Let's get the health check of the Benthos service
	benthosMonitorServiceInfo, err := s.metricsService.Status(ctx, filesystemService, tick)
	if err != nil {
		// If the health check failed because of a connection refused error,
		// we return a special error that can be used to trigger a state transition
		// to the stopped state
		if errors.Is(err, benthos_monitor.ErrServiceConnectionRefused) || errors.Is(err, benthos_monitor.ErrServiceNoLogFile) {
			return BenthosStatus{}, ErrHealthCheckConnectionRefused
		}
		return BenthosStatus{}, fmt.Errorf("failed to get health check: %w", err)
	}

	// If this is nil, we have not yet tried to scan for metrics and config, or there has been an error (but that one will be cached in the above Status() return)
	if benthosMonitorServiceInfo.BenthosStatus.LastScan == nil {
		return BenthosStatus{}, fmt.Errorf("last scan is nil")
	}

	// Check that the last scan is not older then RedpandaMaxMetricsAndConfigAge
	if loopStartTime.Sub(benthosMonitorServiceInfo.BenthosStatus.LastScan.LastUpdatedAt) > constants.BenthosMaxMetricsAndConfigAge {
		s.logger.Warnf("last scan is %s old, returning empty status", loopStartTime.Sub(benthosMonitorServiceInfo.BenthosStatus.LastScan.LastUpdatedAt))
		return BenthosStatus{}, fmt.Errorf("last scan is older than %s", constants.BenthosMaxMetricsAndConfigAge)
	}

	healthCheck := benthos_monitor.HealthCheck{
		IsLive:  benthosMonitorServiceInfo.BenthosStatus.LastScan.HealthCheck.IsLive,
		IsReady: benthosMonitorServiceInfo.BenthosStatus.LastScan.HealthCheck.IsReady,
		Version: benthosMonitorServiceInfo.BenthosStatus.LastScan.HealthCheck.Version,
	}

	if benthosMonitorServiceInfo.BenthosStatus.LastScan.BenthosMetrics == nil {
		return BenthosStatus{}, fmt.Errorf("benthos metrics are nil")
	}

	s.lastStatus = BenthosStatus{
		HealthCheck:    healthCheck,
		BenthosMetrics: *benthosMonitorServiceInfo.BenthosStatus.LastScan.BenthosMetrics,
		BenthosLogs:    logs,
	}

	// if everything is fine, set the status to the service info
	return s.lastStatus, nil

}

// AddBenthosToS6Manager adds a Benthos instance to the S6 manager
// Expects benthosName (e.g. "myservice") as defined in the UMH config
func (s *BenthosService) AddBenthosToS6Manager(ctx context.Context, filesystemService filesystem.Service, cfg *benthosserviceconfig.BenthosServiceConfig, benthosName string) error {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	s6ServiceName := s.getS6ServiceName(benthosName)

	// Check whether s6ServiceConfigs already contains an entry for this instance
	for _, s6Config := range s.s6ServiceConfigs {
		if s6Config.Name == s6ServiceName {
			return ErrServiceAlreadyExists
		}
	}

	// Generate the S6 config for this instance
	s6Config, err := s.GenerateS6ConfigForBenthos(cfg, s6ServiceName)
	if err != nil {
		return fmt.Errorf("failed to generate S6 config for Benthos service %s: %w", s6ServiceName, err)
	}

	// Create the S6 FSM config for this instance
	s6FSMConfig := config.S6FSMConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            s6ServiceName,
			DesiredFSMState: s6fsm.OperationalStateRunning,
		},
		S6ServiceConfig: s6Config,
	}

	// Add the S6 FSM config to the list of S6 FSM configs
	// so that the S6 manager will start the service
	s.s6ServiceConfigs = append(s.s6ServiceConfigs, s6FSMConfig)

	// Also add the benthos monitor to the S6 manager
	// This function is idempotent, so it's safe to call it multiple times
	if err := s.metricsService.AddBenthosMonitorToS6Manager(ctx, uint16(cfg.MetricsPort)); err != nil {
		if !errors.Is(err, benthos_monitor.ErrServiceAlreadyExists) {
			return fmt.Errorf("failed to add benthos monitor to S6 manager: %w", err)
		}
	}

	return nil
}

// UpdateBenthosInS6Manager updates an existing Benthos instance in the S6 manager
// Expects benthosName (e.g. "myservice") as defined in the UMH config
func (s *BenthosService) UpdateBenthosInS6Manager(ctx context.Context, filesystemService filesystem.Service, cfg *benthosserviceconfig.BenthosServiceConfig, benthosName string) error {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	s6ServiceName := s.getS6ServiceName(benthosName)

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
	s6Config, err := s.GenerateS6ConfigForBenthos(cfg, s6ServiceName)
	if err != nil {
		return fmt.Errorf("failed to generate S6 config for Benthos service %s: %w", s6ServiceName, err)
	}

	// Update the S6 service config while preserving the desired state
	currentDesiredState := s.s6ServiceConfigs[index].DesiredFSMState
	s.s6ServiceConfigs[index] = config.S6FSMConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            s6ServiceName,
			DesiredFSMState: currentDesiredState,
		},
		S6ServiceConfig: s6Config,
	}

	// Also update the benthos monitor config
	if err := s.metricsService.UpdateBenthosMonitorInS6Manager(ctx, uint16(cfg.MetricsPort)); err != nil {
		if !errors.Is(err, benthos_monitor.ErrServiceAlreadyExists) {
			return fmt.Errorf("failed to update benthos monitor in S6 manager: %w", err)
		}
	}

	// The next reconciliation of the S6 manager will detect the config change
	// and update the service
	return nil
}

// RemoveBenthosFromS6Manager removes a Benthos instance from the S6 manager
// Expects benthosName (e.g. "myservice") as defined in the UMH config
func (s *BenthosService) RemoveBenthosFromS6Manager(ctx context.Context, filesystemService filesystem.Service, benthosName string) error {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	s6ServiceName := s.getS6ServiceName(benthosName)

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

	// Also remove the benthos monitor from the S6 manager
	if err := s.metricsService.RemoveBenthosMonitorFromS6Manager(ctx); err != nil {
		return fmt.Errorf("failed to remove benthos monitor from S6 manager: %w", err)
	}

	return nil
}

// StartBenthos starts a Benthos instance
// Expects benthosName (e.g. "myservice") as defined in the UMH config
func (s *BenthosService) StartBenthos(ctx context.Context, filesystemService filesystem.Service, benthosName string) error {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	s6ServiceName := s.getS6ServiceName(benthosName)

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

	// Also start the benthos monitor
	if err := s.metricsService.StartBenthosMonitor(ctx); err != nil {
		return fmt.Errorf("failed to start benthos monitor: %w", err)
	}

	return nil
}

// StopBenthos stops a Benthos instance
// Expects benthosName (e.g. "myservice") as defined in the UMH config
func (s *BenthosService) StopBenthos(ctx context.Context, filesystemService filesystem.Service, benthosName string) error {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	s6ServiceName := s.getS6ServiceName(benthosName)

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

	// Also stop the benthos monitor
	if err := s.metricsService.StopBenthosMonitor(ctx); err != nil {
		return fmt.Errorf("failed to stop benthos monitor: %w", err)
	}

	return nil
}

// ReconcileManager reconciles the Benthos manager
func (s *BenthosService) ReconcileManager(ctx context.Context, filesystemService filesystem.Service, tick uint64) (err error, reconciled bool) {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized"), false
	}

	if ctx.Err() != nil {
		return ctx.Err(), false
	}

	// Create a new snapshot with the current S6 service configs
	// Note: therefore, the S6 manager will not have access to the full observed state
	snapshot := fsm.SystemSnapshot{
		CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: s.s6ServiceConfigs}},
		Tick:          tick,
	}

	s6Err, s6Reconciled := s.s6Manager.Reconcile(ctx, snapshot, filesystemService)
	if s6Err != nil {
		return fmt.Errorf("failed to reconcile S6 manager: %w", s6Err), false
	}

	// Also reconcile the benthos monitor
	monitorErr, monitorReconciled := s.metricsService.ReconcileManager(ctx, filesystemService, tick)
	if monitorErr != nil {
		return fmt.Errorf("failed to reconcile benthos monitor: %w", monitorErr), false
	}

	// If either was reconciled, indicate that reconciliation occurred
	return nil, s6Reconciled || monitorReconciled
}

// IsLogsFine analyzes Benthos logs to determine if there are any critical issues
func (s *BenthosService) IsLogsFine(logs []s6service.LogEntry, currentTime time.Time, logWindow time.Duration) bool {
	if len(logs) == 0 {
		return true
	}

	// First, filter out by timestamp and then geenate []string from it
	filteredLogs := []string{}
	for _, log := range logs {
		if log.Timestamp.After(currentTime.Add(-1 * logWindow)) {
			filteredLogs = append(filteredLogs, log.Content)
		}
	}

	// Compile regex patterns for different types of logs
	benthosLogRegex := regexp.MustCompile(`^level=(error|warn)\s+msg="(.+)"`)
	configErrorRegex := regexp.MustCompile(`^configuration file read error:`)
	loggerErrorRegex := regexp.MustCompile(`^failed to create logger:`)
	linterErrorRegex := regexp.MustCompile(`^Config lint error:`)

	for _, log := range filteredLogs {
		// Check for critical system errors first
		if configErrorRegex.MatchString(log) ||
			loggerErrorRegex.MatchString(log) ||
			linterErrorRegex.MatchString(log) {
			return false
		}

		// Parse structured Benthos logs
		if matches := benthosLogRegex.FindStringSubmatch(log); matches != nil {
			level := matches[1]
			message := matches[2]

			// Error logs are always critical
			if level == "error" {
				return false
			}

			// For warnings, only consider them critical if they indicate serious issues
			if level == "warn" {
				criticalWarnings := []string{
					"failed to",
					"connection lost",
					"unable to",
				}

				for _, criticalPattern := range criticalWarnings {
					if strings.Contains(message, criticalPattern) {
						return false
					}
				}
			}
		}
	}

	return true
}

// IsMetricsErrorFree checks if there are any errors in the Benthos metrics
func (s *BenthosService) IsMetricsErrorFree(metrics benthos_monitor.BenthosMetrics) bool {
	// Check output errors
	if metrics.Metrics.Output.Error > 0 {
		return false
	}

	// Check processor errors
	for _, proc := range metrics.Metrics.Process.Processors {
		if proc.Error > 0 {
			return false
		}
	}

	return true
}

// HasProcessingActivity checks if the Benthos instance has active data processing based on metrics state
func (s *BenthosService) HasProcessingActivity(status BenthosStatus) bool {
	if status.BenthosMetrics.MetricsState == nil {
		return false
	}

	return status.BenthosMetrics.MetricsState.IsActive
}

// ServiceExists checks if a Benthos service exists in the S6 manager
func (s *BenthosService) ServiceExists(ctx context.Context, filesystemService filesystem.Service, benthosName string) bool {
	s6ServiceName := s.getS6ServiceName(benthosName)
	s6ServicePath := filepath.Join(constants.S6BaseDir, s6ServiceName)

	exists, err := s.s6Service.ServiceExists(ctx, s6ServicePath, filesystemService)
	if err != nil {
		return false
	}

	return exists
}

// ForceRemoveBenthos removes a Benthos instance from the S6 manager
// This should only be called if the Benthos instance is in a permanent failure state
// and the instance itself cannot be stopped or removed
// Expects benthosName (e.g. "myservice") as defined in the UMH config
func (s *BenthosService) ForceRemoveBenthos(ctx context.Context, filesystemService filesystem.Service, benthosName string) error {
	s6ServiceName := s.getS6ServiceName(benthosName)
	s6ServicePath := filepath.Join(constants.S6BaseDir, s6ServiceName)
	return s.s6Service.ForceRemove(ctx, s6ServicePath, filesystemService)
}
