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
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

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

func NewRedpandaMonitorService(fs filesystem.Service) *RedpandaMonitorService {
	log := logger.New(logger.ComponentRedpandaMonitorService, logger.FormatJSON)
	return &RedpandaMonitorService{
		fs:           fs,
		logger:       log,
		metricsState: NewRedpandaMetricsState(),
		s6Manager:    s6fsm.NewS6Manager(logger.ComponentRedpandaMonitorService),
		s6Service:    s6service.NewDefaultService(),
	}
}

func (s *RedpandaMonitorService) generateRedpandaScript() (string, error) {
	// Build the redpanda command - curl http://localhost:9644/public_metrics
	// We should ensure that each log line contains a full curl execution, therefore we will pipe the result of the curl command into base64
	// Create the script content with a loop that executes redpanda every second
	scriptContent := `#!/bin/sh
while true; do
  curl http://localhost:9644/public_metrics | base64
  sleep 1
done
`

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
func (s *RedpandaMonitorService) parseRedpandaLogs(logs []s6service.LogEntry, tick uint64) (*RedpandaMetrics, error) {
	if len(logs) == 0 {
		return nil, fmt.Errorf("no logs provided")
	}

	// Extract the last line of the logs
	lastLine := logs[len(logs)-1].Content

	// Decode the last line from base64
	decodedLine, err := base64.StdEncoding.DecodeString(lastLine)
	if err != nil {
		return nil, fmt.Errorf("failed to decode last line: %w", err)
	}

	// Parse the metrics
	metrics, err := parseMetrics(decodedLine)
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

// parseMetrics parses prometheus metrics into structured format
func parseMetrics(data []byte) (Metrics, error) {
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
	mf, err := parser.TextToMetricFamilies(bytes.NewReader(data))
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
