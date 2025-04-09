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

package redpanda_monitor

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/prometheus/common/expfmt"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"

	dto "github.com/prometheus/client_model/go"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	"go.uber.org/zap"
)

// Ensure RedpandaMonitorService implements IRedpandaMonitorService
var _ IRedpandaMonitorService = (*RedpandaMonitorService)(nil)

type RedpandaMonitorService struct {
	fs              filesystem.Service
	logger          *zap.Logger
	metricsState    *RedpandaMetricsState
	s6Manager       *s6fsm.S6Manager
	s6Service       s6service.Service
	s6ServiceConfig *config.S6FSMConfig // There can only be one instance of this service (as there is also only one redpanda instance)
}

// RedpandaMonitorServiceOption is a function that modifies a RedpandaMonitorService
type RedpandaMonitorServiceOption func(*RedpandaMonitorService)

// WithS6Service sets a custom S6 service for the RedpandaMonitorService
func WithS6Service(s6Service s6service.Service) RedpandaMonitorServiceOption {
	return func(s *RedpandaMonitorService) {
		s.s6Service = s6Service
	}
}

// WithFilesystem sets a custom filesystem service for the RedpandaMonitorService
func WithFilesystem(fs filesystem.Service) RedpandaMonitorServiceOption {
	return func(s *RedpandaMonitorService) {
		s.fs = fs
	}
}

func NewRedpandaMonitorService(fs filesystem.Service, opts ...RedpandaMonitorServiceOption) *RedpandaMonitorService {
	log := logger.New(logger.ComponentRedpandaMonitorService, logger.FormatJSON)
	service := &RedpandaMonitorService{
		fs:           fs,
		logger:       log,
		metricsState: NewRedpandaMetricsState(),
		s6Manager:    s6fsm.NewS6Manager(logger.ComponentRedpandaMonitorService),
		s6Service:    s6service.NewDefaultService(),
	}
	for _, opt := range opts {
		opt(service)
	}
	return service
}

const START_MARKER = "BEGINBEGINBEGINBEGINBEGINBEGINBEGINBEGINBEGINBEGIN"
const MID_MARKER = "MIDMIDMIDMIDMIDMIDMIDMIDMIDMIDMIDMIDMIDMIDMIDMIDMID"
const END_MARKER = "ENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDEND"

func (s *RedpandaMonitorService) generateRedpandaScript() (string, error) {
	// Build the redpanda command - curl http://localhost:9644/public_metrics
	// Create the script content with a loop that executes redpanda every second
	// Also let's use gzip to compress the output & hex encode it
	// We use gzip here, to prevent the output from being rotated halfway through the logs & hex encode it to avoid issues with special characters
	// Max-time: https://everything.curl.dev/usingcurl/timeouts.html
	scriptContent := fmt.Sprintf(`#!/bin/sh
while true; do
  echo "%s"
  curl -sSL --max-time 1 http://localhost:9644/public_metrics | gzip -c | xxd -p
  echo "%s"
  curl -sSL --max-time 1 http://localhost:9644/v1/cluster_config | gzip -c | xxd -p
  echo "%s"
  sleep 1
done
`, START_MARKER, MID_MARKER, END_MARKER)

	return scriptContent, nil
}

func (s *RedpandaMonitorService) getS6ServiceName() string {
	return "redpanda-monitor"
}

func (s *RedpandaMonitorService) GenerateS6ConfigForRedpandaMonitor() (s6serviceconfig.S6ServiceConfig, error) {
	scriptContent, err := s.generateRedpandaScript()
	if err != nil {
		return s6serviceconfig.S6ServiceConfig{}, err
	}

	s6Config := s6serviceconfig.S6ServiceConfig{
		Command: []string{
			"/bin/sh",
			fmt.Sprintf("%s/%s/config/run_redpanda_monitor.sh", constants.S6BaseDir, s.getS6ServiceName()),
		},
		Env: map[string]string{},
		ConfigFiles: map[string]string{
			"run_redpanda_monitor.sh": scriptContent,
		},
	}

	return s6Config, nil
}

// GetConfig is not implemented, as the config is static

// parseRedpandaLogs parses the logs of a redpanda service and extracts metrics
func (s *RedpandaMonitorService) parseRedpandaLogs(logs []s6service.LogEntry, tick uint64) (*RedpandaMetricsAndClusterConfig, error) {
	/*
		A normal log entry looks like this:
		START_MARKER
		Hex encoded gzip data of the metrics
		MID_MARKER
		Hex encoded gzip data of the cluster config
		END_MARKER
	*/

	if len(logs) == 0 {
		return nil, fmt.Errorf("no logs provided")
	}
	// Find the markers in a single pass through the logs
	startMarkerIndex := -1
	midMarkerIndex := -1
	endMarkerIndex := -1

	for i := 0; i < len(logs); i++ {
		if strings.Contains(logs[i].Content, START_MARKER) {
			startMarkerIndex = i
		} else if strings.Contains(logs[i].Content, MID_MARKER) {
			midMarkerIndex = i
		} else if strings.Contains(logs[i].Content, END_MARKER) {
			// We dont break here, as there might be multiple end markers
			endMarkerIndex = i
		}
	}

	// Verify we found all markers in the correct order
	if startMarkerIndex == -1 {
		return nil, fmt.Errorf("could not parse redpanda metrics/configuration: no start marker found. This can happen when the redpanda service is not running, or the logs where rotated")
	}
	if midMarkerIndex == -1 {
		return nil, fmt.Errorf("could not parse redpanda metrics/configuration: no mid marker found. This can happen when the redpanda service is not running, or the logs where rotated")
	}
	if endMarkerIndex == -1 {
		return nil, fmt.Errorf("could not parse redpanda metrics/configuration: no end marker found. This can happen when the redpanda service is not running, or the logs where rotated")
	}
	if !(startMarkerIndex < midMarkerIndex && midMarkerIndex < endMarkerIndex) {
		return nil, fmt.Errorf("could not parse redpanda metrics/configuration: markers found in incorrect order. This can happen when the redpanda service is not running, or the logs where rotated")
	}

	// We need to extract the lines inbetween the start and mid marker & mid and end marker
	// Metrics is the first part, cluster config is the second part
	metricsData := logs[startMarkerIndex+1 : midMarkerIndex]
	clusterConfigData := logs[midMarkerIndex+1 : endMarkerIndex]

	var metricsDataBytes []byte
	var clusterConfigDataBytes []byte
	for _, log := range metricsData {
		metricsDataBytes = append(metricsDataBytes, log.Content...)
	}
	for _, log := range clusterConfigData {
		clusterConfigDataBytes = append(clusterConfigDataBytes, log.Content...)
	}

	// Remove the START_MARKER, MID_MARKER and END_MARKER
	metricsDataBytes = bytes.ReplaceAll(metricsDataBytes, []byte(START_MARKER), []byte{})
	metricsDataBytes = bytes.ReplaceAll(metricsDataBytes, []byte(MID_MARKER), []byte{})
	metricsDataBytes = bytes.ReplaceAll(metricsDataBytes, []byte(END_MARKER), []byte{})

	clusterConfigDataBytes = bytes.ReplaceAll(clusterConfigDataBytes, []byte(START_MARKER), []byte{})
	clusterConfigDataBytes = bytes.ReplaceAll(clusterConfigDataBytes, []byte(MID_MARKER), []byte{})
	clusterConfigDataBytes = bytes.ReplaceAll(clusterConfigDataBytes, []byte(END_MARKER), []byte{})

	var wg sync.WaitGroup
	var metrics *RedpandaMetrics
	var clusterConfig *ClusterConfig
	var metricsErr, clusterConfigErr error

	wg.Add(2)
	go func() {
		defer wg.Done()
		metrics, metricsErr = s.processMetricsDataBytes(metricsDataBytes, tick)
	}()

	go func() {
		defer wg.Done()
		clusterConfig, clusterConfigErr = s.processClusterConfigDataBytes(clusterConfigDataBytes, tick)
	}()

	wg.Wait()

	if metricsErr != nil {
		return nil, fmt.Errorf("failed to process metrics data: %w", metricsErr)
	}

	if clusterConfigErr != nil {
		return nil, fmt.Errorf("failed to process cluster config data: %w", clusterConfigErr)
	}

	return &RedpandaMetricsAndClusterConfig{
		Metrics:       metrics,
		ClusterConfig: clusterConfig,
	}, nil
}

func parseCurlError(errorString string) error {
	if !strings.Contains(errorString, "curl") {
		return nil
	}

	knownErrors := map[string]error{
		"curl: (7)":  fmt.Errorf("connection refused, while attempting to fetch metrics/configuration from redpanda. This is expected during the startup phase of the redpanda service, when the service is not yet ready to receive connections"),
		"curl: (28)": fmt.Errorf("connection timed out, while attempting to fetch metrics/configuration from redpanda. This can happen if the redpanda service or the system is experiencing high load"),
	}

	for knownError, err := range knownErrors {
		if strings.Contains(errorString, knownError) {
			return err
		}
	}

	return fmt.Errorf("unknown curl error: %s", errorString)
}

func (s *RedpandaMonitorService) processMetricsDataBytes(metricsDataBytes []byte, tick uint64) (*RedpandaMetrics, error) {

	curlError := parseCurlError(string(metricsDataBytes))
	if curlError != nil {
		return nil, curlError
	}

	// Decode the hex encoded metrics data
	decodedMetricsDataBytes, err := hex.DecodeString(string(metricsDataBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to decode metrics data: %w", err)
	}

	// Decompress the metrics data
	gzipReader, err := gzip.NewReader(bytes.NewReader(decodedMetricsDataBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to decompress metrics data: %w", err)
	}
	defer gzipReader.Close()

	// Parse the metrics
	metrics, err := parseMetrics(gzipReader)
	if err != nil {
		return nil, fmt.Errorf("failed to parse metrics: %w", err)
	}

	// Update the metrics state
	s.metricsState.UpdateFromMetrics(metrics, tick)

	return &RedpandaMetrics{
		Metrics:      metrics,
		MetricsState: s.metricsState,
	}, nil
}

func (s *RedpandaMonitorService) processClusterConfigDataBytes(clusterConfigDataBytes []byte, tick uint64) (*ClusterConfig, error) {

	curlError := parseCurlError(string(clusterConfigDataBytes))
	if curlError != nil {
		return nil, curlError
	}

	// Decode the hex encoded metrics data
	decodedMetricsDataBytes, err := hex.DecodeString(string(clusterConfigDataBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to decode metrics data: %w", err)
	}

	// Decompress the metrics data
	gzipReader, err := gzip.NewReader(bytes.NewReader(decodedMetricsDataBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to decompress metrics data: %w", err)
	}
	defer gzipReader.Close()

	// Parse the JSON response
	var redpandaConfig map[string]interface{}
	if err := json.NewDecoder(gzipReader).Decode(&redpandaConfig); err != nil {
		return nil, fmt.Errorf("failed to parse cluster config data: %w", err)
	}

	var result ClusterConfig

	// Extract the values we need from the JSON
	if value, ok := redpandaConfig["log_retention_ms"]; ok {
		if floatVal, ok := value.(float64); ok {
			result.Topic.DefaultTopicRetentionMs = int(floatVal)
		} else if intVal, ok := value.(int); ok {
			result.Topic.DefaultTopicRetentionMs = intVal
		} else if strVal, ok := value.(string); ok {
			if intVal, err := strconv.Atoi(strVal); err == nil {
				result.Topic.DefaultTopicRetentionMs = intVal
			}
		}
	}

	if value, ok := redpandaConfig["retention_bytes"]; ok {
		if floatVal, ok := value.(float64); ok {
			result.Topic.DefaultTopicRetentionBytes = int(floatVal)
		} else if intVal, ok := value.(int); ok {
			result.Topic.DefaultTopicRetentionBytes = intVal
		} else if strVal, ok := value.(string); ok {
			if intVal, err := strconv.Atoi(strVal); err == nil {
				result.Topic.DefaultTopicRetentionBytes = intVal
			}
		}
	}

	return &result, nil
}

// parseMetrics parses prometheus metrics into structured format
func parseMetrics(dataReader io.Reader) (Metrics, error) {
	var parser expfmt.TextParser
	metrics := Metrics{
		Infrastructure: InfrastructureMetrics{},
		Cluster:        ClusterMetrics{},
		Throughput:     ThroughputMetrics{},
		Topic: TopicMetrics{
			TopicPartitionMap: make(map[string]int64), // Pre-allocate map to avoid nil check later
		},
	}
	// Parse the metrics text into prometheus format
	mf, err := parser.TextToMetricFamilies(dataReader)
	if err != nil {
		return metrics, fmt.Errorf("failed to parse metrics: %w", err)
	}

	// Directly extract only the metrics we need instead of iterating all metrics

	// Infrastructure metrics - Storage
	if family, ok := mf["redpanda_storage_disk_free_bytes"]; ok && len(family.Metric) > 0 {
		metrics.Infrastructure.Storage.FreeBytes = getMetricValue(family.Metric[0])
	}

	if family, ok := mf["redpanda_storage_disk_total_bytes"]; ok && len(family.Metric) > 0 {
		metrics.Infrastructure.Storage.TotalBytes = getMetricValue(family.Metric[0])
	}

	if family, ok := mf["redpanda_storage_disk_free_space_alert"]; ok && len(family.Metric) > 0 {
		// Any non-zero value indicates an alert condition
		metrics.Infrastructure.Storage.FreeSpaceAlert = getMetricValue(family.Metric[0]) != 0
	}

	// Infrastructure metrics - Uptime
	if family, ok := mf["redpanda_uptime_seconds_total"]; ok && len(family.Metric) > 0 {
		metrics.Infrastructure.Uptime.Uptime = getMetricValue(family.Metric[0])
	}

	// Cluster metrics
	if family, ok := mf["redpanda_cluster_topics"]; ok && len(family.Metric) > 0 {
		metrics.Cluster.Topics = getMetricValue(family.Metric[0])
	}

	if family, ok := mf["redpanda_cluster_unavailable_partitions"]; ok && len(family.Metric) > 0 {
		metrics.Cluster.UnavailableTopics = getMetricValue(family.Metric[0])
	}

	// Throughput metrics
	if family, ok := mf["redpanda_kafka_request_bytes_total"]; ok {
		// Process only produce/consume metrics in a single pass
		for _, metric := range family.Metric {
			if label := getLabel(metric, "redpanda_request"); label != "" {
				if label == "produce" {
					metrics.Throughput.BytesIn = getMetricValue(metric)
				} else if label == "consume" {
					metrics.Throughput.BytesOut = getMetricValue(metric)
				}
			}
		}
	}

	// Topic metrics
	if family, ok := mf["redpanda_kafka_partitions"]; ok {
		for _, metric := range family.Metric {
			if topic := getLabel(metric, "redpanda_topic"); topic != "" {
				metrics.Topic.TopicPartitionMap[topic] = getMetricValue(metric)
			}
		}
	}

	return metrics, nil
}

// getMetricValue extracts numeric value from a metric
func getMetricValue(m *dto.Metric) int64 {
	if m.Counter != nil {
		return int64(m.Counter.GetValue())
	}
	if m.Gauge != nil {
		return int64(m.Gauge.GetValue())
	}
	if m.Untyped != nil {
		return int64(m.Untyped.GetValue())
	}
	return 0
}

// getLabel extracts a label value from a metric
func getLabel(m *dto.Metric, name string) string {
	for _, label := range m.Label {
		if label.GetName() == name {
			return label.GetValue()
		}
	}
	return ""
}

// Status checks the status of a redpanda service
func (s *RedpandaMonitorService) Status(ctx context.Context, filesystemService filesystem.Service, tick uint64) (ServiceInfo, error) {
	if ctx.Err() != nil {
		return ServiceInfo{}, ctx.Err()
	}

	s6ServiceName := s.getS6ServiceName()

	// First, check if the service exists in the S6 manager
	// This is a crucial check that prevents "instance not found" errors
	// during reconciliation when a service is being created or removed
	if _, exists := s.s6Manager.GetInstance(s6ServiceName); !exists {
		return ServiceInfo{}, ErrServiceNotExist
	}

	// Get S6 state
	s6StateRaw, err := s.s6Manager.GetLastObservedState(s6ServiceName)
	if err != nil {
		if strings.Contains(err.Error(), "instance "+s6ServiceName+" not found") ||
			strings.Contains(err.Error(), "not found") {
			return ServiceInfo{}, ErrServiceNotExist
		}
	}

	s6State, ok := s6StateRaw.(s6fsm.S6ObservedState)
	if !ok {
		return ServiceInfo{}, fmt.Errorf("observed state is not a S6ObservedState: %v", s6StateRaw)
	}

	// Get FSM state
	fsmState, err := s.s6Manager.GetCurrentFSMState(s6ServiceName)
	if err != nil {
		if strings.Contains(err.Error(), "instance "+s6ServiceName+" not found") ||
			strings.Contains(err.Error(), "not found") {
			return ServiceInfo{}, ErrServiceNotExist
		}
		return ServiceInfo{}, fmt.Errorf("failed to get current FSM state: %w", err)
	}

	// Get logs
	s6ServicePath := filepath.Join(constants.S6BaseDir, s6ServiceName)
	logs, err := s.s6Service.GetLogs(ctx, s6ServicePath, filesystemService)
	if err != nil {
		return ServiceInfo{}, fmt.Errorf("failed to get logs: %w", err)
	}

	// Get metrics
	metrics, err := s.parseRedpandaLogs(logs, tick)
	if err != nil {
		return ServiceInfo{}, fmt.Errorf("failed to parse metrics: %w", err)
	}

	return ServiceInfo{
		S6ObservedState: s6State,
		S6FSMState:      fsmState,
		RedpandaStatus: RedpandaMonitorStatus{
			LastScan:  metrics,
			IsRunning: fsmState == s6fsm.OperationalStateRunning,
			Logs:      logs,
		},
	}, nil
}

// AddRedpandaMonitorToS6Manager adds a redpanda instance to the S6 manager
func (s *RedpandaMonitorService) AddRedpandaMonitorToS6Manager(ctx context.Context) error {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	s6ServiceName := s.getS6ServiceName()

	// Check whether s6ServiceConfigs already contains an entry for this instance
	if s.s6ServiceConfig != nil {
		return ErrServiceAlreadyExists
	}

	// Generate the S6 config for this instance
	s6Config, err := s.GenerateS6ConfigForRedpandaMonitor()
	if err != nil {
		return fmt.Errorf("failed to generate S6 config for RedpandaMonitor service %s: %w", s6ServiceName, err)
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
	s.s6ServiceConfig = &s6FSMConfig

	return nil
}

//  There is no need for an UpdateRedpandaInS6Manager, as the S6 config is static

// RemoveRedpandaMonitorFromS6Manager removes a redpanda instance from the S6 manager
func (s *RedpandaMonitorService) RemoveRedpandaMonitorFromS6Manager(ctx context.Context) error {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	if s.s6ServiceConfig == nil {
		return ErrServiceNotExist
	}

	s.s6ServiceConfig = nil

	// Clean up the metrics state
	s.metricsState = NewRedpandaMetricsState()

	return nil
}

// StartRedpandaMonitor starts a redpanda instance
func (s *RedpandaMonitorService) StartRedpandaMonitor(ctx context.Context) error {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	if s.s6ServiceConfig == nil {
		return ErrServiceNotExist
	}

	s.s6ServiceConfig.DesiredFSMState = s6fsm.OperationalStateRunning

	return nil
}

// StopRedpandaMonitor stops a redpanda instance
func (s *RedpandaMonitorService) StopRedpandaMonitor(ctx context.Context) error {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	if s.s6ServiceConfig == nil {
		return ErrServiceNotExist
	}

	s.s6ServiceConfig.DesiredFSMState = s6fsm.OperationalStateStopped

	return nil
}

// ReconcileManager reconciles the Redpanda manager
func (s *RedpandaMonitorService) ReconcileManager(ctx context.Context, filesystemService filesystem.Service, tick uint64) (err error, reconciled bool) {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized"), false
	}

	if ctx.Err() != nil {
		return ctx.Err(), false
	}

	if s.s6ServiceConfig == nil {
		return ErrServiceNotExist, false
	}

	return s.s6Manager.Reconcile(ctx, config.FullConfig{Internal: config.InternalConfig{Services: []config.S6FSMConfig{*s.s6ServiceConfig}}}, filesystemService, tick)
}

// ServiceExists checks if a redpanda instance exists
func (s *RedpandaMonitorService) ServiceExists(ctx context.Context, filesystemService filesystem.Service) bool {
	if s.s6Manager == nil {
		return false
	}

	if ctx.Err() != nil {
		return false
	}

	exists, err := s.s6Service.ServiceExists(ctx, filepath.Join(constants.S6BaseDir, s.getS6ServiceName()), filesystemService)
	if err != nil {
		return false
	}

	return exists
}
