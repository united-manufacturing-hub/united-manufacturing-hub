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

package benthos_monitor

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
	"time"

	"github.com/prometheus/common/expfmt"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"

	dto "github.com/prometheus/client_model/go"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	"go.uber.org/zap"
)

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

// BenthosMetrics contains information about the metrics of the Benthos service
type BenthosMetrics struct {
	// Metrics contains the metrics of the Benthos service
	Metrics Metrics
	// LastUpdatedAt contains the last time the metrics were updated
	MetricsState *BenthosMetricsState
}

// ServiceInfo contains information about a benthos service
type ServiceInfo struct {
	// S6ObservedState contains information about the S6 benthos_monitor service
	S6ObservedState s6fsm.S6ObservedState
	// S6FSMState contains the current state of the S6 FSM of the benthos_monitorservice
	S6FSMState string
	// BenthosStatus contains information about the status of the benthos service
	BenthosStatus BenthosMonitorStatus
}

// BenthosMonitorStatus contains information about the status of the Benthos service
type BenthosMetricsScan struct {
	// HealthCheck contains information about the health of the Benthos service
	HealthCheck HealthCheck
	// Metrics contains information about the metrics of the Benthos service
	BenthosMetrics *BenthosMetrics
	// MetricsState contains information about the metrics of the Benthos service
	MetricsState *BenthosMetricsState
	// LastUpdatedAt contains the last time the metrics were updated
	LastUpdatedAt time.Time
}

type BenthosMonitorStatus struct {
	// LastScan contains the result of the last scan
	// If this is nil, we never had a successfull scan
	LastScan *BenthosMetricsScan
	// IsRunning indicates whether the benthos_monitor service is running
	IsRunning bool
	// Logs contains the logs of the benthos_monitor service
	Logs []s6service.LogEntry
}

type IBenthosMonitorService interface {
	GenerateS6ConfigForBenthosMonitor(benthosName string, port uint16) (s6serviceconfig.S6ServiceConfig, error)
	Status(ctx context.Context, filesystemService filesystem.Service, tick uint64) (ServiceInfo, error)
	AddBenthosMonitorToS6Manager(ctx context.Context, port uint16) error
	RemoveBenthosMonitorFromS6Manager(ctx context.Context) error
	ForceRemoveBenthosMonitor(ctx context.Context, filesystemService filesystem.Service) error
	UpdateBenthosMonitorInS6Manager(ctx context.Context, port uint16) error
	StartBenthosMonitor(ctx context.Context) error
	StopBenthosMonitor(ctx context.Context) error
	ReconcileManager(ctx context.Context, filesystemService filesystem.Service, tick uint64) (error, bool)
	ServiceExists(ctx context.Context, filesystemService filesystem.Service) bool
}

// Ensure BenthosMonitorService implements IBenthosMonitorService
var _ IBenthosMonitorService = (*BenthosMonitorService)(nil)

type BenthosMonitorService struct {
	logger          *zap.SugaredLogger
	metricsState    *BenthosMetricsState
	s6Manager       *s6fsm.S6Manager
	s6Service       s6service.Service
	s6ServiceConfig *config.S6FSMConfig // There can only be one monitor per benthos instance
	benthosName     string              // normally a service can handle multiple instances, the service monitor here is different and can only handle one instance
}

// BenthosMonitorServiceOption is a function that modifies a BenthosMonitorService
type BenthosMonitorServiceOption func(*BenthosMonitorService)

// WithS6Service sets a custom S6 service for the RedpandaMonitorService
func WithS6Service(s6Service s6service.Service) BenthosMonitorServiceOption {
	return func(s *BenthosMonitorService) {
		s.s6Service = s6Service
	}
}

// WithS6Manager sets a custom S6 manager for the RedpandaMonitorService
func WithS6Manager(s6Manager *s6fsm.S6Manager) BenthosMonitorServiceOption {
	return func(s *BenthosMonitorService) {
		s.s6Manager = s6Manager
	}
}

func NewBenthosMonitorService(benthosName string, opts ...BenthosMonitorServiceOption) *BenthosMonitorService {
	managerName := fmt.Sprintf("%s%s", logger.ComponentBenthosService, "benthos-monitor-"+benthosName)
	service := &BenthosMonitorService{
		logger:       logger.For(managerName),
		metricsState: NewBenthosMetricsState(),
		s6Manager:    s6fsm.NewS6Manager(logger.ComponentBenthosMonitorService),
		s6Service:    s6service.NewDefaultService(),
		benthosName:  benthosName,
	}
	for _, opt := range opts {
		opt(service)
	}
	return service
}

// BLOCK_START_MARKER marks the begin of a new data block inside the logs.
// Between it and MID_MARKER is the metrics data, between MID_MARKER and END_MARKER is the cluster config data.
const BLOCK_START_MARKER = "BEGINBEGINBEGINBEGINBEGINBEGINBEGINBEGINBEGINBEGINBEGINBEGINBEGINBEGINBEGINBEGINBEGINBEGIN"

// PING_END_MARKER marks the end of the /ping endpoint data.
const PING_END_MARKER = "PINGENDPINGENDPINGENDPINGENDPINGENDPINGENDPINGENDPINGENDPINGENDPINGENDPINGENDPINGENDPINGENDPINGENDPINGENDPINGENDPINGEND"

// READY_END marks the end of the /ready endpoint data.
const READY_END = "CONFIGENDCONFIGENDCONFIGENDCONFIGENDCONFIGENDCONFIGENDCONFIGENDCONFIGENDCONFIGENDCONFIGENDCONFIGEND"

// VERSION_END marks the end of the /version endpoint data.
const VERSION_END = "VERSIONENDVERSIONENDVERSIONENDVERSIONENDVERSIONENDVERSIONENDVERSIONENDVERSIONENDVERSIONENDVERSIONENDVERSIONEND"

// METRICS_END_MARKER marks the end of the metrics data and the beginning of the cluster config data.
const METRICS_END_MARKER = "METRICSENDMETRICSENDMETRICSENDMETRICSENDMETRICSENDMETRICSENDMETRICSENDMETRICSENDMETRICSENDMETRICSEND"

// BLOCK_END_MARKER marks the end of the cluster config data.
const BLOCK_END_MARKER = "ENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDEND"

func (s *BenthosMonitorService) generateBenthosScript(port uint16) (string, error) {
	// Build the benthos command - curl http://localhost:9644/public_metrics
	// Create the script content with a loop that executes benthos every second
	// Also let's use gzip to compress the output & hex encode it
	// We use gzip here, to prevent the output from being rotated halfway through the logs & hex encode it to avoid issues with special characters
	// Max-time: https://everything.curl.dev/usingcurl/timeouts.html
	// The timestamp here is the unix nanosecond timestamp of the current time
	// It is gathered AFTER the curl commands, preventing long curl execution times from affecting the timestamp
	// +%s%9N: %s is the unix timestamp in seconds with 9 decimal places for nanoseconds
	scriptContent := fmt.Sprintf(`#!/bin/sh
while true; do
  echo "%s"
  curl -sSL --max-time 1 http://localhost:%d/ping | gzip -c | xxd -p
  echo "%s"
  curl -sSL --max-time 1 http://localhost:%d/ready | gzip -c | xxd -p
  echo "%s"
  curl -sSL --max-time 1 http://localhost:%d/version | gzip -c | xxd -p
  echo "%s"
  curl -sSL --max-time 1 http://localhost:%d/metrics | gzip -c | xxd -p
  echo "%s"
  date +%%s%%9N
  echo "%s"
  sleep 1
done
`, BLOCK_START_MARKER, port, PING_END_MARKER, port, READY_END, port, VERSION_END, port, METRICS_END_MARKER, BLOCK_END_MARKER)

	return scriptContent, nil
}

// GetS6ServiceName converts the benthosName (e.g. "myservice") that was given during NewBenthosMonitorService to its S6 service name (e.g. "benthos-myservice")
func (s *BenthosMonitorService) GetS6ServiceName() string {
	return fmt.Sprintf("benthos-monitor-%s", s.benthosName)
}

func (s *BenthosMonitorService) GenerateS6ConfigForBenthosMonitor(s6ServiceName string, port uint16) (s6serviceconfig.S6ServiceConfig, error) {
	scriptContent, err := s.generateBenthosScript(port)
	if err != nil {
		return s6serviceconfig.S6ServiceConfig{}, err
	}

	s6Config := s6serviceconfig.S6ServiceConfig{
		Command: []string{
			"/bin/sh",
			fmt.Sprintf("%s/%s/config/run_benthos_monitor.sh", constants.S6BaseDir, s6ServiceName),
		},
		Env: map[string]string{},
		ConfigFiles: map[string]string{
			"run_benthos_monitor.sh": scriptContent,
		},
	}

	return s6Config, nil
}

// GetConfig is not implemented, as the config is static

type Section struct {
	StartMarkerIndex      int
	PingEndMarkerIndex    int
	ReadyEndMarkerIndex   int
	VersionEndMarkerIndex int
	MetricsEndMarkerIndex int
	BlockEndMarkerIndex   int
}

// ParseBenthosLogs parses the logs of a benthos service and extracts metrics
func (s *BenthosMonitorService) ParseBenthosLogs(ctx context.Context, logs []s6service.LogEntry, tick uint64) (*BenthosMetricsScan, error) {
	/*
		A normal log entry looks like this:
		BLOCK_START_MARKER
		Hex encoded gzip data of the /ping endpoint
		PING_END_MARKER
		Hex encoded gzip data of the /ready endpoint
		READY_END
		Hex encoded gzip data of the /version endpoint
		VERSION_END
		Hex encoded gzip data of the /metrics endpoint
		METRICS_END_MARKER
		Timestamp data
		BLOCK_END_MARKER
	*/

	if len(logs) == 0 {
		return nil, fmt.Errorf("no logs provided")
	}
	// Find the markers in a single pass through the logs
	sections := make([]Section, 0)

	currentSection := Section{
		StartMarkerIndex:      -1,
		PingEndMarkerIndex:    -1,
		ReadyEndMarkerIndex:   -1,
		VersionEndMarkerIndex: -1,
		MetricsEndMarkerIndex: -1,
		BlockEndMarkerIndex:   -1,
	}

	// This implementation scans the logs in a single pass, which is more efficient than scanning for each marker separately
	// If the there are multiple sections, we will have multiple entries in the sections list
	// This ensures that we always have a valid section, even if the markers of later sections are missing (e.g the end marker for example was not yet written)
	for i := 0; i < len(logs); i++ {
		if strings.Contains(logs[i].Content, BLOCK_START_MARKER) {
			currentSection.StartMarkerIndex = i
		} else if strings.Contains(logs[i].Content, PING_END_MARKER) {
			// Dont even try to find an end marker, if we dont have a start marker
			if currentSection.StartMarkerIndex == -1 {
				continue
			}
			currentSection.PingEndMarkerIndex = i
		} else if strings.Contains(logs[i].Content, READY_END) {
			// Dont even try to find an end marker, if we dont have a start marker
			if currentSection.StartMarkerIndex == -1 && currentSection.PingEndMarkerIndex == -1 {
				continue
			}
			currentSection.ReadyEndMarkerIndex = i
		} else if strings.Contains(logs[i].Content, VERSION_END) {
			// Dont even try to find an end marker, if we dont have a start marker
			if currentSection.StartMarkerIndex == -1 && currentSection.PingEndMarkerIndex == -1 && currentSection.ReadyEndMarkerIndex == -1 {
				continue
			}
			currentSection.VersionEndMarkerIndex = i
		} else if strings.Contains(logs[i].Content, METRICS_END_MARKER) {
			// Dont even try to find an end marker, if we dont have a start marker
			if currentSection.StartMarkerIndex == -1 && currentSection.PingEndMarkerIndex == -1 && currentSection.ReadyEndMarkerIndex == -1 && currentSection.VersionEndMarkerIndex == -1 {
				continue
			}
			currentSection.MetricsEndMarkerIndex = i
		} else if strings.Contains(logs[i].Content, BLOCK_END_MARKER) {
			// We dont break here, as there might be multiple end markers
			currentSection.BlockEndMarkerIndex = i

			// If we have all sections add it to the list, otherwise discard !
			if currentSection.StartMarkerIndex != -1 && currentSection.MetricsEndMarkerIndex != -1 && currentSection.VersionEndMarkerIndex != -1 && currentSection.BlockEndMarkerIndex != -1 {
				// Check if the order makes sense, otherwise discard
				if currentSection.StartMarkerIndex < currentSection.PingEndMarkerIndex && currentSection.PingEndMarkerIndex < currentSection.ReadyEndMarkerIndex && currentSection.ReadyEndMarkerIndex < currentSection.VersionEndMarkerIndex && currentSection.VersionEndMarkerIndex < currentSection.MetricsEndMarkerIndex && currentSection.MetricsEndMarkerIndex < currentSection.BlockEndMarkerIndex {
					sections = append(sections, currentSection)
				}
			}

			// Reset the current section
			currentSection = Section{
				StartMarkerIndex:      -1,
				PingEndMarkerIndex:    -1,
				ReadyEndMarkerIndex:   -1,
				VersionEndMarkerIndex: -1,
				MetricsEndMarkerIndex: -1,
				BlockEndMarkerIndex:   -1,
			}
		}
	}

	if len(sections) == 0 {
		return nil, fmt.Errorf("could not parse benthos metrics/configuration: no sections found. This can happen when the benthos service is not running, or the logs where rotated")
	}

	// Find the latest section that is fully constructed (e.g the latest entry in the list)
	actualSection := sections[len(sections)-1]

	// We need to extract the lines between the markers
	// Ping is the first part, ready is the second part, version is the third part, metrics is the fourth part
	pingData := logs[actualSection.StartMarkerIndex+1 : actualSection.PingEndMarkerIndex]
	readyData := logs[actualSection.PingEndMarkerIndex+1 : actualSection.ReadyEndMarkerIndex]
	versionData := logs[actualSection.ReadyEndMarkerIndex+1 : actualSection.VersionEndMarkerIndex]
	metricsData := logs[actualSection.VersionEndMarkerIndex+1 : actualSection.MetricsEndMarkerIndex]
	timestampData := logs[actualSection.MetricsEndMarkerIndex+1 : actualSection.BlockEndMarkerIndex]

	var pingDataBytes []byte
	var readyDataBytes []byte
	var versionDataBytes []byte
	var metricsDataBytes []byte
	var timestampDataBytes []byte

	for _, log := range pingData {
		pingDataBytes = append(pingDataBytes, log.Content...)
	}

	for _, log := range readyData {
		readyDataBytes = append(readyDataBytes, log.Content...)
	}

	for _, log := range versionData {
		versionDataBytes = append(versionDataBytes, log.Content...)
	}

	for _, log := range metricsData {
		metricsDataBytes = append(metricsDataBytes, log.Content...)
	}

	for _, log := range timestampData {
		timestampDataBytes = append(timestampDataBytes, log.Content...)
	}

	// Remove any markers that might be in the data

	pingDataBytes = bytes.ReplaceAll(pingDataBytes, []byte(BLOCK_START_MARKER), []byte{})
	pingDataBytes = bytes.ReplaceAll(pingDataBytes, []byte(PING_END_MARKER), []byte{})
	pingDataBytes = bytes.ReplaceAll(pingDataBytes, []byte(READY_END), []byte{})
	pingDataBytes = bytes.ReplaceAll(pingDataBytes, []byte(VERSION_END), []byte{})
	pingDataBytes = bytes.ReplaceAll(pingDataBytes, []byte(METRICS_END_MARKER), []byte{})
	pingDataBytes = bytes.ReplaceAll(pingDataBytes, []byte(BLOCK_END_MARKER), []byte{})

	readyDataBytes = bytes.ReplaceAll(readyDataBytes, []byte(BLOCK_START_MARKER), []byte{})
	readyDataBytes = bytes.ReplaceAll(readyDataBytes, []byte(PING_END_MARKER), []byte{})
	readyDataBytes = bytes.ReplaceAll(readyDataBytes, []byte(READY_END), []byte{})
	readyDataBytes = bytes.ReplaceAll(readyDataBytes, []byte(VERSION_END), []byte{})
	readyDataBytes = bytes.ReplaceAll(readyDataBytes, []byte(METRICS_END_MARKER), []byte{})
	readyDataBytes = bytes.ReplaceAll(readyDataBytes, []byte(BLOCK_END_MARKER), []byte{})

	versionDataBytes = bytes.ReplaceAll(versionDataBytes, []byte(BLOCK_START_MARKER), []byte{})
	versionDataBytes = bytes.ReplaceAll(versionDataBytes, []byte(PING_END_MARKER), []byte{})
	versionDataBytes = bytes.ReplaceAll(versionDataBytes, []byte(READY_END), []byte{})
	versionDataBytes = bytes.ReplaceAll(versionDataBytes, []byte(VERSION_END), []byte{})
	versionDataBytes = bytes.ReplaceAll(versionDataBytes, []byte(METRICS_END_MARKER), []byte{})
	versionDataBytes = bytes.ReplaceAll(versionDataBytes, []byte(BLOCK_END_MARKER), []byte{})

	metricsDataBytes = bytes.ReplaceAll(metricsDataBytes, []byte(BLOCK_START_MARKER), []byte{})
	metricsDataBytes = bytes.ReplaceAll(metricsDataBytes, []byte(PING_END_MARKER), []byte{})
	metricsDataBytes = bytes.ReplaceAll(metricsDataBytes, []byte(READY_END), []byte{})
	metricsDataBytes = bytes.ReplaceAll(metricsDataBytes, []byte(VERSION_END), []byte{})
	metricsDataBytes = bytes.ReplaceAll(metricsDataBytes, []byte(METRICS_END_MARKER), []byte{})
	metricsDataBytes = bytes.ReplaceAll(metricsDataBytes, []byte(BLOCK_END_MARKER), []byte{})

	timestampDataBytes = bytes.ReplaceAll(timestampDataBytes, []byte(BLOCK_START_MARKER), []byte{})
	timestampDataBytes = bytes.ReplaceAll(timestampDataBytes, []byte(PING_END_MARKER), []byte{})
	timestampDataBytes = bytes.ReplaceAll(timestampDataBytes, []byte(READY_END), []byte{})
	timestampDataBytes = bytes.ReplaceAll(timestampDataBytes, []byte(VERSION_END), []byte{})
	timestampDataBytes = bytes.ReplaceAll(timestampDataBytes, []byte(METRICS_END_MARKER), []byte{})
	timestampDataBytes = bytes.ReplaceAll(timestampDataBytes, []byte(BLOCK_END_MARKER), []byte{})

	var metrics *BenthosMetrics
	var healthCheck HealthCheck

	// Step 1: Process Liveness from /ping endpoint
	livenessResp, err := s.ProcessPingData(pingDataBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to process ping data: %w", err)
	}
	healthCheck.IsLive = livenessResp

	// Step 2: Process Readiness from /ready endpoint
	isReady, readyResp, err := s.ProcessReadyData(readyDataBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to process ready data: %w", err)
	}
	healthCheck.IsReady = isReady
	healthCheck.ReadyError = readyResp.Error
	healthCheck.ConnectionStatuses = readyResp.Statuses

	// Step 3: Process Version from /version endpoint
	versionResp, err := s.ProcessVersionData(versionDataBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to process version data: %w", err)
	}
	healthCheck.Version = versionResp.Version

	// Step 4: Process the Metrics and update the metrics state
	metrics, err = s.ProcessMetricsData(metricsDataBytes, tick)
	if err != nil {
		return nil, fmt.Errorf("failed to parse metrics: %w", err)
	}

	timestampDataString := string(timestampDataBytes)
	// If the system resolution is to small, we need to pad the timestamp with zeros
	// Good: 1744199140749598341
	// Bad: 1744199121
	if len(timestampDataString) < 19 {
		timestampDataString = fmt.Sprintf("%s%s", timestampDataString, strings.Repeat("0", 19-len(timestampDataString)))
	}

	// Parse the timestamp data from the timestampDataBytes (that we already extracted)
	timestampNs, err := strconv.ParseUint(timestampDataString, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse timestamp data: %w", err)
	}

	lastUpdatedAt := time.Unix(0, int64(timestampNs))
	return &BenthosMetricsScan{
		HealthCheck:    healthCheck,
		BenthosMetrics: metrics,
		LastUpdatedAt:  lastUpdatedAt,
	}, nil
}

func parseCurlError(errorString string) error {
	if !strings.Contains(errorString, "curl") {
		return nil
	}

	knownErrors := map[string]error{
		"curl: (7)":  ErrServiceConnectionRefused,
		"curl: (28)": ErrServiceConnectionTimedOut,
	}

	for knownError, err := range knownErrors {
		if strings.Contains(errorString, knownError) {
			return err
		}
	}

	return fmt.Errorf("unknown curl error: %s", errorString)
}

// ProcessPingData processes the ping data and returns whether the service is live
// It returns false if the service is not live, and an error if there is an error parsing the ping data
// It is live, when it contains the string "pong"
func (s *BenthosMonitorService) ProcessPingData(pingDataBytes []byte) (bool, error) {

	curlError := parseCurlError(string(pingDataBytes))
	if curlError != nil {
		return false, curlError
	}

	pingDataString := string(pingDataBytes)
	// Strip any newlines
	pingDataString = strings.ReplaceAll(pingDataString, "\n", "")

	// Decode the hex encoded ping data
	decodedPingDataBytes, err := hex.DecodeString(pingDataString)
	if err != nil {
		return false, fmt.Errorf("failed to decode ping data: %w", err)
	}

	// Decompress the ping data
	gzipReader, err := gzip.NewReader(bytes.NewReader(decodedPingDataBytes))
	if err != nil {
		return false, fmt.Errorf("failed to decompress ping data: %w", err)
	}
	defer func() {
		if err := gzipReader.Close(); err != nil {
			sentry.ReportIssue(fmt.Errorf("failed to close gzip reader: %w", err), sentry.IssueTypeError, s.logger)
		}
	}()

	// Parse the ping data
	isLive, err := ParsePingData(gzipReader)
	if err != nil {
		return false, fmt.Errorf("failed to parse ping data: %w", err)
	}

	return isLive, nil
}

// ParsePingData parses the ping data and returns whether the service is live
func ParsePingData(dataReader io.Reader) (bool, error) {
	data, err := io.ReadAll(dataReader)
	if err != nil {
		return false, fmt.Errorf("failed to read ping data: %w", err)
	}

	if strings.Contains(string(data), "pong") {
		return true, nil
	}

	return false, nil
}

// ProcessReadyData processes the ready data and returns whether the service is ready
// It returns false if the service is not ready, and an error if there is an error parsing the ready data
// It is ready, when the error field is empty
func (s *BenthosMonitorService) ProcessReadyData(readyDataBytes []byte) (bool, readyResponse, error) {

	curlError := parseCurlError(string(readyDataBytes))
	if curlError != nil {
		return false, readyResponse{}, curlError
	}

	readyDataString := string(readyDataBytes)
	// Strip any newlines
	readyDataString = strings.ReplaceAll(readyDataString, "\n", "")

	// Decode the hex encoded ping data
	decodedReadyDataBytes, err := hex.DecodeString(readyDataString)
	if err != nil {
		return false, readyResponse{}, fmt.Errorf("failed to decode ready data: %w", err)
	}

	// Decompress the ping data
	gzipReader, err := gzip.NewReader(bytes.NewReader(decodedReadyDataBytes))
	if err != nil {
		return false, readyResponse{}, fmt.Errorf("failed to decompress ready data: %w", err)
	}
	defer func() {
		if err := gzipReader.Close(); err != nil {
			sentry.ReportIssue(fmt.Errorf("failed to close gzip reader: %w", err), sentry.IssueTypeError, s.logger)
		}
	}()

	// Parse the ping data
	isReady, readyResp, err := ParseReadyData(gzipReader)
	if err != nil {
		return false, readyResponse{}, fmt.Errorf("failed to parse ready data: %w", err)
	}

	return isReady, readyResp, nil
}

// ParseReadyData parses the ready data and returns whether the service is ready
// It also returns the ready response
// the service is ready, when the error field is empty
func ParseReadyData(dataReader io.Reader) (bool, readyResponse, error) {
	data, err := io.ReadAll(dataReader)
	if err != nil {
		return false, readyResponse{}, fmt.Errorf("failed to read ready data: %w", err)
	}

	var readyResp readyResponse
	if err := json.Unmarshal(data, &readyResp); err != nil {
		return false, readyResponse{}, fmt.Errorf("failed to unmarshal ready data: %w", err)
	}

	return readyResp.Error == "", readyResp, nil
}

// ProcessVersionData processes the version data and returns the version response
// It returns an error if there is an error parsing the version data
func (s *BenthosMonitorService) ProcessVersionData(versionDataBytes []byte) (versionResponse, error) {

	curlError := parseCurlError(string(versionDataBytes))
	if curlError != nil {
		return versionResponse{}, curlError
	}

	versionDataString := string(versionDataBytes)
	// Strip any newlines
	versionDataString = strings.ReplaceAll(versionDataString, "\n", "")

	// Decode the hex encoded ping data
	decodedVersionDataBytes, err := hex.DecodeString(versionDataString)
	if err != nil {
		return versionResponse{}, fmt.Errorf("failed to decode version data: %w", err)
	}

	// Decompress the ping data
	gzipReader, err := gzip.NewReader(bytes.NewReader(decodedVersionDataBytes))
	if err != nil {
		return versionResponse{}, fmt.Errorf("failed to decompress version data: %w", err)
	}
	defer func() {
		if err := gzipReader.Close(); err != nil {
			sentry.ReportIssue(fmt.Errorf("failed to close gzip reader: %w", err), sentry.IssueTypeError, s.logger)
		}
	}()

	// Parse the ping data
	versionResp, err := ParseVersionData(gzipReader)
	if err != nil {
		return versionResponse{}, fmt.Errorf("failed to parse version data: %w", err)
	}

	return versionResp, nil
}

// ParseVersionData parses the version data and returns the version response
func ParseVersionData(dataReader io.Reader) (versionResponse, error) {
	data, err := io.ReadAll(dataReader)
	if err != nil {
		return versionResponse{}, fmt.Errorf("failed to read version data: %w", err)
	}

	var versionResp versionResponse
	if err := json.Unmarshal(data, &versionResp); err != nil {
		return versionResponse{}, fmt.Errorf("failed to unmarshal version data: %w", err)
	}

	return versionResp, nil
}

// ProcessMetricsData processes the metrics data and returns the metrics response
// It returns an error if there is an error parsing the metrics data
func (s *BenthosMonitorService) ProcessMetricsData(metricsDataBytes []byte, tick uint64) (*BenthosMetrics, error) {

	curlError := parseCurlError(string(metricsDataBytes))
	if curlError != nil {
		return nil, curlError
	}

	metricsDataString := string(metricsDataBytes)
	// Strip any newlines
	metricsDataString = strings.ReplaceAll(metricsDataString, "\n", "")

	// Decode the hex encoded ping data
	decodedMetricsDataBytes, err := hex.DecodeString(metricsDataString)
	if err != nil {
		return nil, fmt.Errorf("failed to decode metrics data: %w", err)
	}

	// Decompress the ping data
	gzipReader, err := gzip.NewReader(bytes.NewReader(decodedMetricsDataBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to decompress metrics data: %w", err)
	}
	defer func() {
		if err := gzipReader.Close(); err != nil {
			sentry.ReportIssue(fmt.Errorf("failed to close gzip reader: %w", err), sentry.IssueTypeError, s.logger)
		}
	}()

	// Parse the ping data
	metrics, err := ParseMetricsData(gzipReader)
	if err != nil {
		return nil, fmt.Errorf("failed to parse metrics data: %w", err)
	}

	// Update the metrics state
	s.metricsState.UpdateFromMetrics(metrics, tick)

	return &BenthosMetrics{
		Metrics:      metrics,
		MetricsState: s.metricsState,
	}, nil
}

// ParseMetricsData parses the metrics data and returns the metrics response
func ParseMetricsData(dataReader io.Reader) (Metrics, error) {
	data, err := io.ReadAll(dataReader)
	if err != nil {
		return Metrics{}, fmt.Errorf("failed to read metrics data: %w", err)
	}

	metrics, err := ParseMetricsFromBytes(data)
	if err != nil {
		return Metrics{}, fmt.Errorf("failed to parse metrics data: %w", err)
	}

	return metrics, nil
}

// ParseMetricsFromBytes parses prometheus metrics into structured format
func ParseMetricsFromBytes(data []byte) (Metrics, error) {
	var parser expfmt.TextParser
	metrics := Metrics{
		Input:  InputMetrics{},
		Output: OutputMetrics{},
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
		switch name {
		// Input metrics
		case "input_connection_failed":
			if len(family.Metric) > 0 {
				metrics.Input.ConnectionFailed = int64(getValue(family.Metric[0]))
			}
		case "input_connection_lost":
			if len(family.Metric) > 0 {
				metrics.Input.ConnectionLost = int64(getValue(family.Metric[0]))
			}
		case "input_connection_up":
			if len(family.Metric) > 0 {
				metrics.Input.ConnectionUp = int64(getValue(family.Metric[0]))
			}
		case "input_received":
			if len(family.Metric) > 0 {
				metrics.Input.Received = int64(getValue(family.Metric[0]))
			}
		case "input_latency_ns":
			updateLatencyFromFamily(&metrics.Input.LatencyNS, family)

		// Output metrics
		case "output_batch_sent":
			if len(family.Metric) > 0 {
				metrics.Output.BatchSent = int64(getValue(family.Metric[0]))
			}
		case "output_connection_failed":
			if len(family.Metric) > 0 {
				metrics.Output.ConnectionFailed = int64(getValue(family.Metric[0]))
			}
		case "output_connection_lost":
			if len(family.Metric) > 0 {
				metrics.Output.ConnectionLost = int64(getValue(family.Metric[0]))
			}
		case "output_connection_up":
			if len(family.Metric) > 0 {
				metrics.Output.ConnectionUp = int64(getValue(family.Metric[0]))
			}
		case "output_error":
			if len(family.Metric) > 0 {
				metrics.Output.Error = int64(getValue(family.Metric[0]))
			}
		case "output_sent":
			if len(family.Metric) > 0 {
				metrics.Output.Sent = int64(getValue(family.Metric[0]))
			}
		case "output_latency_ns":
			updateLatencyFromFamily(&metrics.Output.LatencyNS, family)

		// Process metrics
		case "processor_received", "processor_batch_received",
			"processor_sent", "processor_batch_sent",
			"processor_error", "processor_latency_ns":
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

// Status checks the status of a benthos service
func (s *BenthosMonitorService) Status(ctx context.Context, filesystemService filesystem.Service, tick uint64) (ServiceInfo, error) {
	if ctx.Err() != nil {
		return ServiceInfo{}, ctx.Err()
	}

	s6ServiceName := s.GetS6ServiceName()

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
		return ServiceInfo{}, fmt.Errorf("failed to get S6 state: %w", err)
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

	if len(logs) == 0 {
		return ServiceInfo{}, ErrServiceNoLogFile
	}

	// Parse the logs
	metrics, err := s.ParseBenthosLogs(ctx, logs, tick)
	if err != nil {
		return ServiceInfo{}, fmt.Errorf("failed to parse metrics: %w", err)
	}

	return ServiceInfo{
		S6ObservedState: s6State,
		S6FSMState:      fsmState,
		BenthosStatus: BenthosMonitorStatus{
			LastScan: metrics,
			Logs:     logs,
		},
	}, nil
}

// AddBenthosMonitorToS6Manager adds a benthos instance to the S6 manager
func (s *BenthosMonitorService) AddBenthosMonitorToS6Manager(ctx context.Context, port uint16) error {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	s.logger.Debugf("Adding benthos monitor to S6 manager with port %d", port)

	s6ServiceName := s.GetS6ServiceName()

	// Check whether s6ServiceConfigs already contains an entry for this instance
	if s.s6ServiceConfig != nil {
		return ErrServiceAlreadyExists
	}

	// Generate the S6 config for this instance
	s6Config, err := s.GenerateS6ConfigForBenthosMonitor(s6ServiceName, port)
	if err != nil {
		return fmt.Errorf("failed to generate S6 config for BenthosMonitor service %s: %w", s6ServiceName, err)
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

// UpdateBenthosMonitorInS6Manager updates the benthos monitor in the S6 manager
// It will do it by re-generating the S6 config for the benthos instance
// and letting the S6 manager reconcile the instance
func (s *BenthosMonitorService) UpdateBenthosMonitorInS6Manager(ctx context.Context, port uint16) error {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	if s.s6ServiceConfig == nil {
		return ErrServiceNotExist
	}

	// Generate the S6 config for this instance
	s6Config, err := s.GenerateS6ConfigForBenthosMonitor(s.benthosName, port)
	if err != nil {
		return fmt.Errorf("failed to generate S6 config for BenthosMonitor service %s: %w", s.benthosName, err)
	}

	s.s6ServiceConfig.S6ServiceConfig = s6Config

	return nil
}

// RemoveBenthosMonitorFromS6Manager removes a benthos instance from the S6 manager
func (s *BenthosMonitorService) RemoveBenthosMonitorFromS6Manager(ctx context.Context) error {
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
	s.metricsState = NewBenthosMetricsState()

	return nil
}

// StartBenthosMonitor starts a benthos instance
func (s *BenthosMonitorService) StartBenthosMonitor(ctx context.Context) error {
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

// StopBenthosMonitor stops a benthos instance
func (s *BenthosMonitorService) StopBenthosMonitor(ctx context.Context) error {
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

// ReconcileManager reconciles the Benthos manager
func (s *BenthosMonitorService) ReconcileManager(ctx context.Context, filesystemService filesystem.Service, tick uint64) (err error, reconciled bool) {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized"), false
	}

	if ctx.Err() != nil {
		return ctx.Err(), false
	}

	if s.s6ServiceConfig == nil {
		return ErrServiceNotExist, false
	}

	return s.s6Manager.Reconcile(ctx, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: []config.S6FSMConfig{*s.s6ServiceConfig}}}}, filesystemService)
}

// ServiceExists checks if a benthos instance exists
func (s *BenthosMonitorService) ServiceExists(ctx context.Context, filesystemService filesystem.Service) bool {
	if s.s6Manager == nil {
		return false
	}

	if ctx.Err() != nil {
		return false
	}

	exists, err := s.s6Service.ServiceExists(ctx, filepath.Join(constants.S6BaseDir, s.GetS6ServiceName()), filesystemService)
	if err != nil {
		return false
	}

	return exists
}

// ForceRemoveBenthosMonitor removes a Benthos monitor from the S6 manager
// This should only be called if the Benthos monitor is in a permanent failure state
// and the instance itself cannot be stopped or removed
// Expects benthosName (e.g. "myservice") as defined in the UMH config
func (s *BenthosMonitorService) ForceRemoveBenthosMonitor(ctx context.Context, filesystemService filesystem.Service) error {
	s6ServiceName := s.GetS6ServiceName()
	s6ServicePath := filepath.Join(constants.S6BaseDir, s6ServiceName)
	return s.s6Service.ForceRemove(ctx, s6ServicePath, filesystemService)
}
