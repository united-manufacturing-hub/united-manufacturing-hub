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
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/prometheus/common/expfmt"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/redpandaserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	dto "github.com/prometheus/client_model/go"
	redpandayaml "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/redpandaserviceconfig"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

// IRedpandaService is the interface for managing Redpanda
type IRedpandaService interface {
	// GenerateS6ConfigForRedpanda generates a S6 config for a given redpanda instance
	GenerateS6ConfigForRedpanda(redpandaConfig *redpandaserviceconfig.RedpandaServiceConfig) (s6serviceconfig.S6ServiceConfig, error)
	// GetConfig returns the actual Redpanda config from the S6 service
	GetConfig(ctx context.Context) (redpandaserviceconfig.RedpandaServiceConfig, error)
	// Status checks the status of a Redpanda service
	Status(ctx context.Context, tick uint64) (ServiceInfo, error)
	// AddRedpandaToS6Manager adds a Redpanda instance to the S6 manager
	AddRedpandaToS6Manager(ctx context.Context, cfg *redpandaserviceconfig.RedpandaServiceConfig, redpandaName string) error
	// UpdateRedpandaInS6Manager updates an existing Redpanda instance in the S6 manager
	UpdateRedpandaInS6Manager(ctx context.Context, cfg *redpandaserviceconfig.RedpandaServiceConfig) error
	// RemoveRedpandaFromS6Manager removes a Redpanda instance from the S6 manager
	RemoveRedpandaFromS6Manager(ctx context.Context) error
	// StartRedpanda starts a Redpanda instance
	StartRedpanda(ctx context.Context) error
	// StopRedpanda stops a Redpanda instance
	StopRedpanda(ctx context.Context) error
	// ForceRemoveRedpanda removes a Redpanda instance from the S6 manager
	ForceRemoveRedpanda(ctx context.Context) error
	// ServiceExists checks if a Redpanda service exists
	ServiceExists(ctx context.Context) bool
	ReconcileManager(ctx context.Context, tick uint64) (error, bool)
	// IsLogsFine checks if the logs of a Redpanda service are fine
	// Expects logs ([]s6service.LogEntry), currentTime (time.Time), and logWindow (time.Duration)
	IsLogsFine(logs []s6service.LogEntry, currentTime time.Time, logWindow time.Duration) bool
	// IsMetricsErrorFree checks if the metrics of a Redpanda service are error-free
	IsMetricsErrorFree(metrics Metrics) bool
	// HasProcessingActivity checks if a Redpanda service has processing activity
	HasProcessingActivity(status RedpandaStatus) bool
}

// HTTPClient interface for making HTTP requests
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// defaultHTTPClient is the default implementation of HTTPClient
type defaultHTTPClient struct {
	client *http.Client
}

func newDefaultHTTPClient() *defaultHTTPClient {
	transport := &http.Transport{
		MaxIdleConns:      10,
		IdleConnTimeout:   30 * time.Second,
		DisableKeepAlives: false,
	}

	return &defaultHTTPClient{
		client: &http.Client{
			Transport: transport,
		},
	}
}

func (c *defaultHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return c.client.Do(req)
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
	// Metrics contains information about the metrics of the Redpanda service
	Metrics Metrics
	// MetricsState contains information about the metrics of the Redpanda service
	MetricsState *RedpandaMetricsState
	// Logs contains the logs of the Redpanda service
	Logs []s6service.LogEntry
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

// Metrics contains information about the metrics of the Redpanda service
type Metrics struct {
	Infrastructure InfrastructureMetrics
	Cluster        ClusterMetrics
	Throughput     ThroughputMetrics
}

// InfrastructureMetrics contains information about the infrastructure metrics of the Redpanda service
type InfrastructureMetrics struct {
	Storage StorageMetrics
	Uptime  UptimeMetrics
}

// StorageMetrics contains information about the storage metrics of the Redpanda service
type StorageMetrics struct {
	// redpanda_storage_disk_free_bytes
	// type: gauge
	FreeBytes int64
	// redpanda_storage_disk_total_bytes
	// type: gauge
	TotalBytes int64
	// redpanda_storage_disk_free_space_alert (0 == false, everything else == true)
	// type: gauge
	FreeSpaceAlert bool
}

// UptimeMetrics contains information about the uptime metrics of the Redpanda service
type UptimeMetrics struct {
	// redpanda_uptime_seconds_total
	// type: gauge
	Uptime int64
}

// ClusterMetrics contains information about the cluster metrics of the Redpanda service
type ClusterMetrics struct {
	// redpanda_cluster_topics
	// type: gauge
	Topics int64
	// redpanda_cluster_unavailable_partitions
	// type: gauge
	UnavailableTopics int64
}

// ThroughputMetrics contains information about the throughput metrics of the Redpanda service
type ThroughputMetrics struct {
	// redpanda_kafka_request_bytes_total over all redpanda_namespace and redpanda_topic labels using redpanda_request=("produce")
	// type: counter
	BytesIn int64
	// redpanda_kafka_request_bytes_total over all redpanda_namespace and redpanda_topic labels using redpanda_request=("consume")
	// type: counter
	BytesOut int64
}

// RedpandaService is the default implementation of the IRedpandaService interface
type RedpandaService struct {
	logger           *zap.SugaredLogger
	s6Manager        *s6fsm.S6Manager
	s6Service        s6service.Service // S6 service for direct S6 operations
	s6ServiceConfigs []config.S6FSMConfig
	httpClient       HTTPClient
	metricsState     *RedpandaMetricsState
}

// RedpandaServiceOption is a function that modifies a RedpandaService
type RedpandaServiceOption func(*RedpandaService)

// WithHTTPClient sets a custom HTTP client for the BenthosService
func WithHTTPClient(client HTTPClient) RedpandaServiceOption {
	return func(s *RedpandaService) {
		s.httpClient = client
	}
}

// WithS6Service sets a custom S6 service for the BenthosService
func WithS6Service(s6Service s6service.Service) RedpandaServiceOption {
	return func(s *RedpandaService) {
		s.s6Service = s6Service
	}
}

// NewDefaultRedpandaService creates a new default Redpanda service
// name is the name of the Redpanda service as defined in the UMH config
func NewDefaultRedpandaService(redpandaName string, opts ...RedpandaServiceOption) *RedpandaService {
	managerName := fmt.Sprintf("%s%s", logger.ComponentRedpandaService, redpandaName)
	service := &RedpandaService{
		logger:       logger.For(managerName),
		s6Manager:    s6fsm.NewS6Manager(managerName),
		s6Service:    s6service.NewDefaultService(),
		httpClient:   newDefaultHTTPClient(),
		metricsState: NewRedpandaMetricsState(),
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

	return redpandayaml.RenderRedpandaYAML(config.DataDirectory)
}

// generateS6ConfigForBenthos creates a S6 config for a given benthos instance
// Expects s6ServiceName (e.g. "benthos-myservice"), not the raw benthosName
func (s *RedpandaService) GenerateS6ConfigForRedpanda(redpandaConfig *redpandaserviceconfig.RedpandaServiceConfig) (s6Config s6serviceconfig.S6ServiceConfig, err error) {
	configPath := fmt.Sprintf("%s/%s/config/%s", constants.S6BaseDir, constants.RedpandaServiceName, constants.RedpandaConfigFileName)

	yamlConfig, err := s.generateRedpandaYaml(redpandaConfig)
	if err != nil {
		return s6serviceconfig.S6ServiceConfig{}, err
	}

	s6Config = s6serviceconfig.S6ServiceConfig{
		Command: []string{
			"/opt/redpanda/bin/redpanda",
			"--redpanda-config",
			configPath,
		},
		Env: map[string]string{},
		ConfigFiles: map[string]string{
			constants.RedpandaConfigFileName: yamlConfig,
		},
	}

	return s6Config, nil
}

// GetConfig returns the actual Redpanda config from the S6 service
func (s *RedpandaService) GetConfig(ctx context.Context) (redpandaserviceconfig.RedpandaServiceConfig, error) {
	if ctx.Err() != nil {
		return redpandaserviceconfig.RedpandaServiceConfig{}, ctx.Err()
	}

	s6ServicePath := filepath.Join(constants.S6BaseDir, constants.RedpandaServiceName)

	// Request the config file from the S6 service
	yamlData, err := s.s6Service.GetS6ConfigFile(ctx, s6ServicePath, constants.RedpandaConfigFileName)
	if err != nil {
		return redpandaserviceconfig.RedpandaServiceConfig{}, fmt.Errorf("failed to get redpanda config file: %w", err)
	}

	// Parse the YAML into a config map
	var redpandaConfig map[string]interface{}
	if err := yaml.Unmarshal(yamlData, &redpandaConfig); err != nil {
		return redpandaserviceconfig.RedpandaServiceConfig{}, fmt.Errorf("error parsing redpanda config file: %w", err)
	}

	result := redpandaserviceconfig.RedpandaServiceConfig{}

	// Safely extract data_directory
	if dataDirectory, ok := redpandaConfig["data_directory"].(string); ok {
		result.DataDirectory = dataDirectory
	}

	return redpandayaml.NormalizeRedpandaConfig(result), nil
}

// Status checks the status of a Redpanda service
func (s *RedpandaService) Status(ctx context.Context, tick uint64) (ServiceInfo, error) {
	if ctx.Err() != nil {
		return ServiceInfo{}, ctx.Err()
	}

	// First, check if the service exists in the S6 manager
	// This is a crucial check that prevents "instance not found" errors
	// during reconciliation when a service is being created or removed
	if _, exists := s.s6Manager.GetInstance(constants.RedpandaServiceName); !exists {
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

	// Let's get the logs of the Benthos service
	s6ServicePath := filepath.Join(constants.S6BaseDir, constants.RedpandaServiceName)
	logs, err := s.s6Service.GetLogs(ctx, s6ServicePath)
	if err != nil {
		if errors.Is(err, s6service.ErrServiceNotExist) {
			s.logger.Debugf("Service %s does not exist, returning empty logs", constants.RedpandaServiceName)
			return ServiceInfo{}, ErrServiceNotExist
		} else {
			return ServiceInfo{}, fmt.Errorf("failed to get logs: %w", err)
		}
	}

	// Let's get the health check of the Benthos service
	redpandaStatus, err := s.GetHealthCheckAndMetrics(ctx, constants.RedpandaServiceName, tick)
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") {
			return ServiceInfo{
				S6ObservedState: s6ServiceObservedState,
				S6FSMState:      s6FSMState, // Note for state transitions: When a service is stopped and then reactivated,
				// this S6FSMState needs to be properly refreshed here.
				// Otherwise, the service can not transition from stopping to stopped state
				RedpandaStatus: RedpandaStatus{
					Logs: logs,
				},
			}, ErrHealthCheckConnectionRefused
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

// parseMetrics parses prometheus metrics into structured format
func parseMetrics(data []byte) (Metrics, error) {
	var parser expfmt.TextParser
	metrics := Metrics{
		Infrastructure: InfrastructureMetrics{},
		Cluster:        ClusterMetrics{},
		Throughput:     ThroughputMetrics{},
	}

	// Parse the metrics text into prometheus format
	mf, err := parser.TextToMetricFamilies(bytes.NewReader(data))
	if err != nil {
		return metrics, fmt.Errorf("failed to parse metrics: %w", err)
	}

	// Helper function to get metric value
	getValue := func(m *dto.Metric) float64 {
		if m.Counter != nil {
			return m.Counter.GetValue()
		}
		if m.Gauge != nil {
			return m.Gauge.GetValue()
		}
		if m.Untyped != nil {
			return m.Untyped.GetValue()
		}
		return 0
	}

	// Helper function to get label value
	getLabel := func(m *dto.Metric, name string) string {
		for _, label := range m.Label {
			if label.GetName() == name {
				return label.GetValue()
			}
		}
		return ""
	}

}
