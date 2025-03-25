package benthos

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/logger"
	"go.uber.org/zap"

	s6fsm "github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/fsm/s6"
	"gopkg.in/yaml.v3"

	benthosyaml "github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/service/benthos/yaml"
	s6service "github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/service/s6"
)

// IBenthosService is the interface for managing Benthos services
type IBenthosService interface {
	// GenerateS6ConfigForBenthos generates a S6 config for a given benthos instance
	// Expects s6ServiceName (e.g. "benthos-myservice"), not the raw benthosName
	GenerateS6ConfigForBenthos(benthosConfig *config.BenthosServiceConfig, s6ServiceName string) (config.S6ServiceConfig, error)
	// GetConfig returns the actual Benthos config from the S6 service
	// Expects benthosName (e.g. "myservice") as defined in the UMH config
	GetConfig(ctx context.Context, benthosName string) (config.BenthosServiceConfig, error)
	// Status checks the status of a Benthos service
	// Expects benthosName (e.g. "myservice") as defined in the UMH config
	Status(ctx context.Context, benthosName string, metricsPort int, tick uint64) (ServiceInfo, error)
	// AddBenthosToS6Manager adds a Benthos instance to the S6 manager
	// Expects benthosName (e.g. "myservice") as defined in the UMH config
	AddBenthosToS6Manager(ctx context.Context, cfg *config.BenthosServiceConfig, benthosName string) error
	// UpdateBenthosInS6Manager updates an existing Benthos instance in the S6 manager
	// Expects benthosName (e.g. "myservice") as defined in the UMH config
	UpdateBenthosInS6Manager(ctx context.Context, cfg *config.BenthosServiceConfig, benthosName string) error
	// RemoveBenthosFromS6Manager removes a Benthos instance from the S6 manager
	// Expects benthosName (e.g. "myservice") as defined in the UMH config
	RemoveBenthosFromS6Manager(ctx context.Context, benthosName string) error
	// StartBenthos starts a Benthos instance
	// Expects benthosName (e.g. "myservice") as defined in the UMH config
	StartBenthos(ctx context.Context, benthosName string) error
	// StopBenthos stops a Benthos instance
	// Expects benthosName (e.g. "myservice") as defined in the UMH config
	StopBenthos(ctx context.Context, benthosName string) error
	// ForceRemoveBenthos removes a Benthos instance from the S6 manager
	// Expects benthosName (e.g. "myservice") as defined in the UMH config
	ForceRemoveBenthos(ctx context.Context, benthosName string) error
	// ServiceExists checks if a Benthos service exists
	// Expects benthosName (e.g. "myservice") as defined in the UMH config
	ServiceExists(ctx context.Context, benthosName string) bool
	ReconcileManager(ctx context.Context, tick uint64) (error, bool)
	// IsLogsFine checks if the logs of a Benthos service are fine
	// Expects logs ([]s6service.LogEntry), currentTime (time.Time), and logWindow (time.Duration)
	IsLogsFine(logs []s6service.LogEntry, currentTime time.Time, logWindow time.Duration) bool
	// IsMetricsErrorFree checks if the metrics of a Benthos service are error-free
	IsMetricsErrorFree(metrics Metrics) bool
	// HasProcessingActivity checks if a Benthos service has processing activity
	HasProcessingActivity(status BenthosStatus) bool
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

// ServiceInfo contains information about a Benthos service
type ServiceInfo struct {
	// S6ObservedState contains information about the S6 service
	S6ObservedState s6fsm.S6ObservedState
	// S6FSMState contains the current state of the S6 FSM
	S6FSMState string
	// BenthosStatus contains information about the status of the Benthos service
	BenthosStatus BenthosStatus
}

// BenthosStatus contains information about the status of the Benthos service
type BenthosStatus struct {
	// HealthCheck contains information about the health of the Benthos service
	HealthCheck HealthCheck
	// Metrics contains information about the metrics of the Benthos service
	Metrics Metrics
	// MetricsState contains information about the metrics of the Benthos service
	MetricsState *BenthosMetricsState
	// Logs contains the logs of the Benthos service
	Logs []s6service.LogEntry
}

// Metrics contains information about the metrics of the Benthos service
type Metrics struct {
	Input   InputMetrics   `json:"input,omitempty"`
	Output  OutputMetrics  `json:"output,omitempty"`
	Process ProcessMetrics `json:"process,omitempty"`
}

// InputMetrics contains input-specific metrics
type InputMetrics struct {
	ConnectionFailed int64   `json:"connection_failed"`
	ConnectionLost   int64   `json:"connection_lost"`
	ConnectionUp     int64   `json:"connection_up"`
	LatencyNS        Latency `json:"latency_ns"`
	Received         int64   `json:"received"`
}

// OutputMetrics contains output-specific metrics
type OutputMetrics struct {
	BatchSent        int64   `json:"batch_sent"`
	ConnectionFailed int64   `json:"connection_failed"`
	ConnectionLost   int64   `json:"connection_lost"`
	ConnectionUp     int64   `json:"connection_up"`
	Error            int64   `json:"error"`
	LatencyNS        Latency `json:"latency_ns"`
	Sent             int64   `json:"sent"`
}

// ProcessMetrics contains processor-specific metrics
type ProcessMetrics struct {
	Processors map[string]ProcessorMetrics `json:"processors"` // key is the processor path (e.g. "root.pipeline.processors.0")
}

// ProcessorMetrics contains metrics for a single processor
type ProcessorMetrics struct {
	Label         string  `json:"label"`
	Received      int64   `json:"received"`
	BatchReceived int64   `json:"batch_received"`
	Sent          int64   `json:"sent"`
	BatchSent     int64   `json:"batch_sent"`
	Error         int64   `json:"error"`
	LatencyNS     Latency `json:"latency_ns"`
}

// Latency contains latency metrics
type Latency struct {
	P50   float64 `json:"p50"`   // 50th percentile
	P90   float64 `json:"p90"`   // 90th percentile
	P99   float64 `json:"p99"`   // 99th percentile
	Sum   float64 `json:"sum"`   // Total sum
	Count int64   `json:"count"` // Number of samples
}

// HealthCheck contains information about the health of the Benthos service
// https://docs.redpanda.com/redpanda-connect/guides/monitoring/
type HealthCheck struct {
	// IsLive is true if the Benthos service is live
	IsLive bool
	// IsReady is true if the Benthos service is ready to process data
	IsReady bool
	// Version contains the version of the Benthos service
	Version string
	// ReadyError contains any error message from the ready check
	ReadyError string `json:"ready_error,omitempty"`
	// ConnectionStatuses contains the detailed connection status of inputs and outputs
	ConnectionStatuses []connStatus `json:"connection_statuses,omitempty"`
}

// versionResponse represents the JSON structure returned by the /version endpoint
type versionResponse struct {
	Version string `json:"version"`
	Built   string `json:"built"`
}

// readyResponse represents the JSON structure returned by the /ready endpoint
type readyResponse struct {
	Error    string       `json:"error,omitempty"`
	Statuses []connStatus `json:"statuses"`
}

type connStatus struct {
	Label     string `json:"label"`
	Path      string `json:"path"`
	Connected bool   `json:"connected"`
	Error     string `json:"error,omitempty"`
}

// BenthosService is the default implementation of the IBenthosService interface
type BenthosService struct {
	logger           *zap.SugaredLogger
	s6Manager        *s6fsm.S6Manager
	s6Service        s6service.Service // S6 service for direct S6 operations
	s6ServiceConfigs []config.S6FSMConfig
	httpClient       HTTPClient
	metricsState     *BenthosMetricsState
}

// BenthosServiceOption is a function that modifies a BenthosService
type BenthosServiceOption func(*BenthosService)

// WithHTTPClient sets a custom HTTP client for the BenthosService
func WithHTTPClient(client HTTPClient) BenthosServiceOption {
	return func(s *BenthosService) {
		s.httpClient = client
	}
}

// WithS6Service sets a custom S6 service for the BenthosService
func WithS6Service(s6Service s6service.Service) BenthosServiceOption {
	return func(s *BenthosService) {
		s.s6Service = s6Service
	}
}

// NewDefaultBenthosService creates a new default Benthos service
// name is the name of the Benthos service as defined in the UMH config
func NewDefaultBenthosService(benthosName string, opts ...BenthosServiceOption) *BenthosService {
	managerName := fmt.Sprintf("%s%s", logger.ComponentBenthosService, benthosName)
	service := &BenthosService{
		logger:       logger.For(managerName),
		s6Manager:    s6fsm.NewS6Manager(managerName),
		s6Service:    s6service.NewDefaultService(),
		httpClient:   newDefaultHTTPClient(),
		metricsState: NewBenthosMetricsState(),
	}

	// Apply options
	for _, opt := range opts {
		opt(service)
	}

	return service
}

// generateBenthosYaml generates a Benthos YAML configuration from a BenthosServiceConfig
func (s *BenthosService) generateBenthosYaml(config *config.BenthosServiceConfig) (string, error) {
	if config == nil {
		return "", fmt.Errorf("config is nil")
	}

	if config.LogLevel == "" {
		config.LogLevel = "INFO"
	}

	return benthosyaml.RenderBenthosYAML(
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
func (s *BenthosService) GenerateS6ConfigForBenthos(benthosConfig *config.BenthosServiceConfig, s6ServiceName string) (s6Config config.S6ServiceConfig, err error) {
	configPath := fmt.Sprintf("%s/%s/config/%s", constants.S6BaseDir, s6ServiceName, constants.BenthosConfigFileName)

	yamlConfig, err := s.generateBenthosYaml(benthosConfig)
	if err != nil {
		return config.S6ServiceConfig{}, err
	}

	s6Config = config.S6ServiceConfig{
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
func (s *BenthosService) GetConfig(ctx context.Context, benthosName string) (config.BenthosServiceConfig, error) {
	if ctx.Err() != nil {
		return config.BenthosServiceConfig{}, ctx.Err()
	}

	s6ServiceName := s.getS6ServiceName(benthosName)
	s6ServicePath := filepath.Join(constants.S6BaseDir, s6ServiceName)

	// Request the config file from the S6 service
	yamlData, err := s.s6Service.GetS6ConfigFile(ctx, s6ServicePath, constants.BenthosConfigFileName)
	if err != nil {
		return config.BenthosServiceConfig{}, fmt.Errorf("failed to get benthos config file for service %s: %w", s6ServiceName, err)
	}

	// Parse the YAML into a config map
	var benthosConfig map[string]interface{}
	if err := yaml.Unmarshal(yamlData, &benthosConfig); err != nil {
		return config.BenthosServiceConfig{}, fmt.Errorf("error parsing benthos config file for service %s: %w", s6ServiceName, err)
	}

	// Extract sections into BenthosServiceConfig struct
	result := config.BenthosServiceConfig{}

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
	return benthosyaml.NormalizeBenthosConfig(result), nil
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
func (s *BenthosService) Status(ctx context.Context, benthosName string, metricsPort int, tick uint64) (ServiceInfo, error) {
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
	logs, err := s.s6Service.GetLogs(ctx, s6ServicePath)
	if err != nil {
		if errors.Is(err, s6service.ErrServiceNotExist) {
			s.logger.Debugf("Service %s does not exist, returning empty logs", s6ServiceName)
			return ServiceInfo{}, ErrServiceNotExist
		} else {
			return ServiceInfo{}, fmt.Errorf("failed to get logs: %w", err)
		}
	}

	// Let's get the health check of the Benthos service
	benthosStatus, err := s.GetHealthCheckAndMetrics(ctx, s6ServiceName, metricsPort, tick)
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") {
			return ServiceInfo{
				S6ObservedState: s6ServiceObservedState,
				S6FSMState:      s6FSMState, // Note for state transitions: When a service is stopped and then reactivated,
				// this S6FSMState needs to be properly refreshed here.
				// Otherwise, the service can not transition from stopping to stopped state
				BenthosStatus: BenthosStatus{
					Logs: logs,
				},
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
	serviceInfo.BenthosStatus.Logs = logs

	return serviceInfo, nil
}

// parseMetrics parses prometheus metrics into structured format
func parseMetrics(data []byte) (Metrics, error) {
	var parser expfmt.TextParser
	metrics := Metrics{
		Process: ProcessMetrics{
			Processors: make(map[string]ProcessorMetrics),
		},
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

	// Process each metric family
	for name, family := range mf {
		switch {
		// Input metrics
		case name == "input_connection_failed":
			if len(family.Metric) > 0 {
				metrics.Input.ConnectionFailed = int64(getValue(family.Metric[0]))
			}
		case name == "input_connection_lost":
			if len(family.Metric) > 0 {
				metrics.Input.ConnectionLost = int64(getValue(family.Metric[0]))
			}
		case name == "input_connection_up":
			if len(family.Metric) > 0 {
				metrics.Input.ConnectionUp = int64(getValue(family.Metric[0]))
			}
		case name == "input_received":
			if len(family.Metric) > 0 {
				metrics.Input.Received = int64(getValue(family.Metric[0]))
			}
		case name == "input_latency_ns":
			updateLatencyFromFamily(&metrics.Input.LatencyNS, family)

		// Output metrics
		case name == "output_batch_sent":
			if len(family.Metric) > 0 {
				metrics.Output.BatchSent = int64(getValue(family.Metric[0]))
			}
		case name == "output_connection_failed":
			if len(family.Metric) > 0 {
				metrics.Output.ConnectionFailed = int64(getValue(family.Metric[0]))
			}
		case name == "output_connection_lost":
			if len(family.Metric) > 0 {
				metrics.Output.ConnectionLost = int64(getValue(family.Metric[0]))
			}
		case name == "output_connection_up":
			if len(family.Metric) > 0 {
				metrics.Output.ConnectionUp = int64(getValue(family.Metric[0]))
			}
		case name == "output_error":
			if len(family.Metric) > 0 {
				metrics.Output.Error = int64(getValue(family.Metric[0]))
			}
		case name == "output_sent":
			if len(family.Metric) > 0 {
				metrics.Output.Sent = int64(getValue(family.Metric[0]))
			}
		case name == "output_latency_ns":
			updateLatencyFromFamily(&metrics.Output.LatencyNS, family)

		// Process metrics
		case name == "processor_received", name == "processor_batch_received",
			name == "processor_sent", name == "processor_batch_sent",
			name == "processor_error", name == "processor_latency_ns":
			for _, metric := range family.Metric {
				path := getLabel(metric, "path")
				if path == "" {
					continue
				}

				// Initialize processor metrics if not exists
				if _, exists := metrics.Process.Processors[path]; !exists {
					metrics.Process.Processors[path] = ProcessorMetrics{
						Label: getLabel(metric, "label"),
					}
				}

				proc := metrics.Process.Processors[path]
				switch name {
				case "processor_received":
					proc.Received = int64(getValue(metric))
				case "processor_batch_received":
					proc.BatchReceived = int64(getValue(metric))
				case "processor_sent":
					proc.Sent = int64(getValue(metric))
				case "processor_batch_sent":
					proc.BatchSent = int64(getValue(metric))
				case "processor_error":
					proc.Error = int64(getValue(metric))
				case "processor_latency_ns":
					updateLatencyFromMetric(&proc.LatencyNS, metric)
				}
				metrics.Process.Processors[path] = proc
			}
		}
	}

	return metrics, nil
}

func updateLatencyFromFamily(latency *Latency, family *dto.MetricFamily) {
	for _, metric := range family.Metric {
		if metric.Summary == nil {
			continue
		}

		latency.Sum = metric.Summary.GetSampleSum()
		latency.Count = int64(metric.Summary.GetSampleCount())

		for _, quantile := range metric.Summary.Quantile {
			switch quantile.GetQuantile() {
			case 0.5:
				latency.P50 = quantile.GetValue()
			case 0.9:
				latency.P90 = quantile.GetValue()
			case 0.99:
				latency.P99 = quantile.GetValue()
			}
		}
	}
}

func updateLatencyFromMetric(latency *Latency, metric *dto.Metric) {
	if metric.Summary == nil {
		return
	}

	latency.Sum = metric.Summary.GetSampleSum()
	latency.Count = int64(metric.Summary.GetSampleCount())

	for _, quantile := range metric.Summary.Quantile {
		switch quantile.GetQuantile() {
		case 0.5:
			latency.P50 = quantile.GetValue()
		case 0.9:
			latency.P90 = quantile.GetValue()
		case 0.99:
			latency.P99 = quantile.GetValue()
		}
	}
}

// GetHealthCheckAndMetrics returns the health check and metrics of a Benthos service
// Expects s6ServiceName (e.g. "benthos-myservice"), not the raw benthosName
func (s *BenthosService) GetHealthCheckAndMetrics(ctx context.Context, s6ServiceName string, metricsPort int, tick uint64) (BenthosStatus, error) {
	if ctx.Err() != nil {
		return BenthosStatus{}, ctx.Err()
	}

	// Skip health checks and metrics if the service doesn't exist yet
	// This avoids unnecessary errors in Status() when the service is still being created
	if _, exists := s.s6Manager.GetInstance(s6ServiceName); !exists {
		return BenthosStatus{
			HealthCheck: HealthCheck{
				IsLive:  false,
				IsReady: false,
			},
			Metrics: Metrics{},
			Logs:    []s6service.LogEntry{},
		}, nil
	}

	if metricsPort == 0 {
		return BenthosStatus{}, fmt.Errorf("could not find metrics port for service %s", s6ServiceName)
	}

	baseURL := fmt.Sprintf("http://localhost:%d", metricsPort)

	// Helper function to make HTTP requests with context
	doRequest := func(endpoint string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request for %s: %w", endpoint, err)
		}
		resp, err := s.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to execute request for %s: %w", endpoint, err)
		}
		return resp, nil
	}

	var healthCheck HealthCheck

	// Check liveness
	if resp, err := doRequest("/ping"); err == nil {
		healthCheck.IsLive = resp.StatusCode == http.StatusOK
		resp.Body.Close()
	} else {
		return BenthosStatus{}, fmt.Errorf("failed to check liveness: %w", err)
	}

	// Check readiness
	if resp, err := doRequest("/ready"); err == nil {
		defer resp.Body.Close()

		// Even if status is 503, we still want to read the body to get the detailed status
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return BenthosStatus{}, fmt.Errorf("failed to read ready response body: %w", err)
		}

		var readyResp readyResponse
		if err := json.Unmarshal(body, &readyResp); err != nil {
			return BenthosStatus{}, fmt.Errorf("failed to unmarshal ready response: %w", err)
		}

		// Service is ready if status is 200 and there's no error
		healthCheck.IsReady = resp.StatusCode == http.StatusOK && readyResp.Error == ""
		healthCheck.ReadyError = readyResp.Error
		healthCheck.ConnectionStatuses = readyResp.Statuses

		// Log detailed status if not ready
		if !healthCheck.IsReady {
			s.logger.Debugw("Service not ready",
				"service", s6ServiceName,
				"error", readyResp.Error,
				"statuses", readyResp.Statuses)
		}
	} else {
		return BenthosStatus{}, fmt.Errorf("failed to check readiness: %w", err)
	}

	// Get version
	if resp, err := doRequest("/version"); err == nil {
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return BenthosStatus{}, fmt.Errorf("failed to read version response body: %w", err)
		}

		var vData versionResponse
		if err := json.Unmarshal(body, &vData); err != nil {
			return BenthosStatus{}, fmt.Errorf("failed to unmarshal version response: %w", err)
		}
		healthCheck.Version = vData.Version
	} else {
		return BenthosStatus{}, fmt.Errorf("failed to get version: %w", err)
	}

	var metrics Metrics

	// Get metrics
	if resp, err := doRequest("/metrics"); err == nil {
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return BenthosStatus{}, fmt.Errorf("failed to read metrics response body: %w", err)
		}

		metrics, err = parseMetrics(body)
		if err != nil {
			return BenthosStatus{}, fmt.Errorf("failed to parse metrics: %w", err)
		}
	}

	// Update the metrics state
	if s.metricsState == nil {
		return BenthosStatus{}, fmt.Errorf("metrics state not initialized")
	}

	s.metricsState.UpdateFromMetrics(metrics, tick)

	return BenthosStatus{
		HealthCheck:  healthCheck,
		Metrics:      metrics,
		MetricsState: s.metricsState,
	}, nil
}

// AddBenthosToS6Manager adds a Benthos instance to the S6 manager
// Expects benthosName (e.g. "myservice") as defined in the UMH config
func (s *BenthosService) AddBenthosToS6Manager(ctx context.Context, cfg *config.BenthosServiceConfig, benthosName string) error {
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

	return nil
}

// UpdateBenthosInS6Manager updates an existing Benthos instance in the S6 manager
// Expects benthosName (e.g. "myservice") as defined in the UMH config
func (s *BenthosService) UpdateBenthosInS6Manager(ctx context.Context, cfg *config.BenthosServiceConfig, benthosName string) error {
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

	// The next reconciliation of the S6 manager will detect the config change
	// and update the service
	return nil
}

// RemoveBenthosFromS6Manager removes a Benthos instance from the S6 manager
// Expects benthosName (e.g. "myservice") as defined in the UMH config
func (s *BenthosService) RemoveBenthosFromS6Manager(ctx context.Context, benthosName string) error {
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

	return nil
}

// StartBenthos starts a Benthos instance
// Expects benthosName (e.g. "myservice") as defined in the UMH config
func (s *BenthosService) StartBenthos(ctx context.Context, benthosName string) error {
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

	return nil
}

// StopBenthos stops a Benthos instance
// Expects benthosName (e.g. "myservice") as defined in the UMH config
func (s *BenthosService) StopBenthos(ctx context.Context, benthosName string) error {
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

	return nil
}

// ReconcileManager reconciles the Benthos manager
func (s *BenthosService) ReconcileManager(ctx context.Context, tick uint64) (err error, reconciled bool) {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized"), false
	}

	if ctx.Err() != nil {
		return ctx.Err(), false
	}

	return s.s6Manager.Reconcile(ctx, config.FullConfig{Services: s.s6ServiceConfigs}, tick)
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
func (s *BenthosService) IsMetricsErrorFree(metrics Metrics) bool {
	// Check output errors
	if metrics.Output.Error > 0 {
		return false
	}

	// Check processor errors
	for _, proc := range metrics.Process.Processors {
		if proc.Error > 0 {
			return false
		}
	}

	return true
}

// HasProcessingActivity checks if the Benthos instance has active data processing based on metrics state
func (s *BenthosService) HasProcessingActivity(status BenthosStatus) bool {
	return status.MetricsState.IsActive
}

// ServiceExists checks if a Benthos service exists in the S6 manager
func (s *BenthosService) ServiceExists(ctx context.Context, benthosName string) bool {
	s6ServiceName := s.getS6ServiceName(benthosName)
	s6ServicePath := filepath.Join(constants.S6BaseDir, s6ServiceName)

	exists, err := s.s6Service.ServiceExists(ctx, s6ServicePath)
	if err != nil {
		s.logger.Error("Error checking if service exists", zap.String("service", s6ServiceName), zap.Error(err))
		return false
	}

	return exists
}

// ForceRemoveBenthos removes a Benthos instance from the S6 manager
// This should only be called if the Benthos instance is in a permanent failure state
// and the instance itself cannot be stopped or removed
// Expects benthosName (e.g. "myservice") as defined in the UMH config
func (s *BenthosService) ForceRemoveBenthos(ctx context.Context, benthosName string) error {
	return s.s6Service.ForceRemove(ctx, s.getS6ServiceName(benthosName))
}
