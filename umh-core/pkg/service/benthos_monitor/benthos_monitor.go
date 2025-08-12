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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6_shared"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/standarderrors"

	dto "github.com/prometheus/client_model/go"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/monitor"
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6_orig"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// Metrics contains information about the metrics of the Benthos service
type Metrics struct {
	Process ProcessMetrics `json:"process,omitempty"`
	Output  OutputMetrics  `json:"output,omitempty"`
	Input   InputMetrics   `json:"input,omitempty"`
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
	// Version contains the version of the Benthos service
	Version string
	// ReadyError contains any error message from the ready check
	ReadyError string `json:"ready_error,omitempty"`
	// ConnectionStatuses contains the detailed connection status of inputs and outputs
	ConnectionStatuses []connStatus `json:"connection_statuses,omitempty"`
	// IsLive is true if the Benthos service is live
	IsLive bool
	// IsReady is true if the Benthos service is ready to process data
	IsReady bool
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
	Error     string `json:"error,omitempty"`
	Connected bool   `json:"connected"`
}

// BenthosMetrics contains information about the metrics of the Benthos service
type BenthosMetrics struct {
	// MetricsState contains the state of the metrics
	MetricsState *BenthosMetricsState
	// Metrics contains the metrics of the Benthos service
	Metrics Metrics
}

// ServiceInfo contains information about a benthos service
type ServiceInfo struct {
	// S6FSMState contains the current state of the S6 FSM of the benthos_monitorservice
	S6FSMState string
	// BenthosStatus contains information about the status of the benthos service
	BenthosStatus BenthosMonitorStatus
	// S6ObservedState contains information about the S6 benthos_monitor service
	S6ObservedState s6fsm.S6ObservedState
}

// BenthosMonitorStatus contains information about the status of the Benthos service
type BenthosMetricsScan struct {
	// LastUpdatedAt contains the last time the metrics were updated
	LastUpdatedAt time.Time
	// Metrics contains information about the metrics of the Benthos service
	BenthosMetrics *BenthosMetrics
	// HealthCheck contains information about the health of the Benthos service
	HealthCheck HealthCheck
}

type BenthosMonitorStatus struct {
	// LastScan contains the result of the last scan
	// If this is nil, we never had a successfull scan
	LastScan *BenthosMetricsScan
	// Logs contains the structured s6 log entries emitted by the
	// benthos_monitor service.
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
	Logs []s6_shared.LogEntry
	// IsRunning indicates whether the benthos_monitor service is running
	IsRunning bool
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
func (bms *BenthosMonitorStatus) CopyLogs(src []s6_shared.LogEntry) error {
	bms.Logs = src
	return nil
}

type IBenthosMonitorService interface {
	GenerateS6ConfigForBenthosMonitor(benthosName string, port uint16) (s6serviceconfig.S6ServiceConfig, error)
	Status(ctx context.Context, services serviceregistry.Provider, tick uint64) (ServiceInfo, error)
	AddBenthosMonitorToS6Manager(ctx context.Context, port uint16) error
	RemoveBenthosMonitorFromS6Manager(ctx context.Context) error
	ForceRemoveBenthosMonitor(ctx context.Context, services serviceregistry.Provider) error
	UpdateBenthosMonitorInS6Manager(ctx context.Context, port uint16) error
	StartBenthosMonitor(ctx context.Context) error
	StopBenthosMonitor(ctx context.Context) error
	ReconcileManager(ctx context.Context, services serviceregistry.Provider, tick uint64) (error, bool)
	ServiceExists(ctx context.Context, services serviceregistry.Provider) bool
}

// Ensure BenthosMonitorService implements IBenthosMonitorService
var _ IBenthosMonitorService = (*BenthosMonitorService)(nil)

type BenthosMonitorService struct {
	logger          *zap.SugaredLogger
	metricsState    *BenthosMetricsState
	s6Manager       *s6fsm.S6Manager
	s6Service       s6_shared.Service
	s6ServiceConfig *config.S6FSMConfig // There can only be one monitor per benthos instance
	benthosName     string              // normally a service can handle multiple instances, the service monitor here is different and can only handle one instance
}

// BenthosMonitorServiceOption is a function that modifies a BenthosMonitorService
type BenthosMonitorServiceOption func(*BenthosMonitorService)

// WithS6Service sets a custom S6 service for the RedpandaMonitorService
func WithS6Service(s6Service s6_shared.Service) BenthosMonitorServiceOption {
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
const PING_END_MARKER = "PINGENDPINGENDPINGENDPINGENDPINGENDPINGENDPINGENDPINGENDPINGENDPINGENDPINGENDPINGENDPINGENDPINGENDPINGENDPINGEND"

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
	// Max-time: https://everything.monitor.dev/usingcurl/timeouts.html
	// The timestamp here is the unix nanosecond timestamp of the current time
	// It is gathered AFTER the curl commands, preventing long curl execution times from affecting the timestamp
	// +%s%9N: %s is the unix timestamp in seconds with 9 decimal places for nanoseconds
	scriptContent := fmt.Sprintf(`#!/bin/sh
while true; do
  echo "%s"
  curl -sSL --max-time 1 http://localhost:%d/ping 2>&1 | gzip -c | xxd -p
  echo "%s"
  curl -sSL --max-time 1 http://localhost:%d/ready 2>&1 | gzip -c | xxd -p
  echo "%s"
  curl -sSL --max-time 1 http://localhost:%d/version 2>&1 | gzip -c | xxd -p
  echo "%s"
  curl -sSL --max-time 1 http://localhost:%d/metrics 2>&1 | gzip -c | xxd -p
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
		LogFilesize: 65536, // 64kB maximum log file size. Each monitoring tick must read and parse the entire log file (which can take >8ms for parsing and up to 20ms for reading). A simple Benthos input/output generates ~5KB of logs for two executions, so 64KB provides adequate space while minimizing performance impact.
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

// ConcatContent concatenates the content of a slice of LogEntry objects into a single byte slice.
// It calculates the total size of the content, allocates a buffer of that size, and then copies each LogEntry's content into the buffer.
// This approach is more efficient than using multiple strings.ReplaceAll calls or regex operations.
func ConcatContent(logs []s6_shared.LogEntry) []byte {
	// 1st pass: exact size
	size := 0
	for i := range logs {
		size += len(logs[i].Content)
	}

	// 1 alloc, no re-growth
	buf := make([]byte, size)
	off := 0
	for i := range logs {
		off += copy(buf[off:], logs[i].Content)
	}
	return buf
}

// build one DFA-based replacer at package-init;
// one pass over the input, no regex, no 4×ReplaceAll
var markerReplacer = strings.NewReplacer(
	BLOCK_START_MARKER, "",
	PING_END_MARKER, "",
	READY_END, "",
	VERSION_END, "",
	METRICS_END_MARKER, "",
	BLOCK_END_MARKER, "",
)

// stripMarkers returns a copy of b with every marker removed.
// If you’re on Go ≥ 1.20 you can avoid the extra string-copy with
//
//	unsafe.String and unsafe.Slice, but the plain version is often fast enough.
func StripMarkers(b []byte) []byte {
	return []byte(markerReplacer.Replace(string(b)))
}

// ParseBenthosLogs parses the logs of a benthos service and extracts metrics
func (s *BenthosMonitorService) ParseBenthosLogs(ctx context.Context, logs []s6_shared.LogEntry, tick uint64) (*BenthosMetricsScan, error) {
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
		return nil, ErrServiceNoSectionsFound
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

	pingDataBytes = ConcatContent(pingData)
	readyDataBytes = ConcatContent(readyData)
	versionDataBytes = ConcatContent(versionData)
	metricsDataBytes = ConcatContent(metricsData)
	timestampDataBytes = ConcatContent(timestampData)

	// Remove any markers that might be in the data
	pingDataBytes = StripMarkers(pingDataBytes)
	readyDataBytes = StripMarkers(readyDataBytes)
	versionDataBytes = StripMarkers(versionDataBytes)
	metricsDataBytes = StripMarkers(metricsDataBytes)
	timestampDataBytes = StripMarkers(timestampDataBytes)

	var metrics *BenthosMetrics
	var healthCheck HealthCheck
	var isLive bool
	var readyResp readyResponse
	var isReady bool
	var versionResp versionResponse

	// Process all data in parallel using errgroup
	ctxBenthosMonitor, cancelBenthosMonitor := context.WithTimeout(ctx, constants.BenthosMonitorProcessMetricsTimeout)
	defer cancelBenthosMonitor()
	g, _ := errgroup.WithContext(ctxBenthosMonitor)

	// Step 1: Process Liveness from /ping endpoint
	g.Go(func() error {
		var err error
		isLive, err = s.ProcessPingData(pingDataBytes)
		if err != nil {
			return fmt.Errorf("failed to process ping data: %w", err)
		}
		return nil
	})

	// Step 2: Process Readiness from /ready endpoint
	g.Go(func() error {
		var err error
		isReady, readyResp, err = s.ProcessReadyData(readyDataBytes)
		if err != nil {
			return fmt.Errorf("failed to process ready data: %w", err)
		}
		return nil
	})

	// Step 3: Process Version from /version endpoint
	g.Go(func() error {
		var err error
		versionResp, err = s.ProcessVersionData(versionDataBytes)
		if err != nil {
			return fmt.Errorf("failed to process version data: %w", err)
		}
		return nil
	})

	// Step 4: Process the Metrics and update the metrics state
	g.Go(func() error {
		var err error
		metrics, err = s.ProcessMetricsData(metricsDataBytes, tick)
		if err != nil {
			return fmt.Errorf("failed to parse metrics: %w", err)
		}
		return nil
	})

	// Create a buffered channel to receive the result from g.Wait()
	errc := make(chan error, 1)

	// Run g.Wait() in a separate goroutine
	go func() {
		errc <- g.Wait()
	}()

	// Use a select statement to wait for either the g.Wait() result or the context's cancellation
	select {
	case err := <-errc:
		if err != nil {
			return nil, err
		}
		// All goroutines completed successfully
	case <-ctxBenthosMonitor.Done():
		return nil, ctxBenthosMonitor.Err()
	}

	// Update healthCheck with the results from the parallel operations
	healthCheck.IsLive = isLive
	healthCheck.IsReady = isReady
	healthCheck.ReadyError = readyResp.Error
	healthCheck.ConnectionStatuses = readyResp.Statuses
	healthCheck.Version = versionResp.Version

	timestampDataString := strings.TrimSpace(string(timestampDataBytes))
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

// ProcessPingData processes the ping data and returns whether the service is live
// It returns false if the service is not live, and an error if there is an error parsing the ping data
// It is live, when it contains the string "pong"
func (s *BenthosMonitorService) ProcessPingData(pingDataBytes []byte) (bool, error) {

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

	curlError := monitor.ParseCurlError(string(data))
	if curlError != nil {
		// If we have any curl error, we can assume the service is not live (but we do not need to return the error)
		return false, nil
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

	curlError := monitor.ParseCurlError(string(data))
	if curlError != nil {
		// If we have any curl error, we can assume the service is not ready (but we do not need to return the error)
		return false, readyResponse{}, nil
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

	curlError := monitor.ParseCurlError(string(data))
	if curlError != nil {
		return versionResponse{}, curlError
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

	curlError := monitor.ParseCurlError(string(data))
	if curlError != nil {
		return Metrics{}, curlError
	}

	metrics, err := ParseMetricsFromBytes(data)
	if err != nil {
		return Metrics{}, fmt.Errorf("failed to parse metrics data: %w", err)
	}

	return metrics, nil
}

// --- helpers ---------------------------------------------------------------

// TailInt returns the integer found **after the final space** in a Prometheus
// text-formatted line.
//
// Benthos' metric stream is deliberately terse; for counters and gauges the
// value we need is always the token after the last space.  A micro-parser that
// walks backwards from the end lets us avoid a full `strings.Fields` split
// (~3× faster and zero allocations on the hot-path).
func TailInt(line []byte) (int64, error) {
	i := bytes.LastIndexByte(line, ' ')
	if i == -1 {
		return 0, fmt.Errorf("failed to find space in line")
	}
	s := string(line[i+1:])
	if strings.ContainsAny(s, "eE") {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, fmt.Errorf("failed to parse float: %w", err)
		}
		return int64(f), nil
	}
	var v int64
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		v = v*10 + int64(c-'0')
	}
	return v, nil
}

// ---------------------------------------------------------------------------

// ParseMetricsFromBytesOpt – fixed & quantile aware
func ParseMetricsFromBytes(raw []byte) (Metrics, error) {
	var m Metrics
	m.Process.Processors = make(map[string]ProcessorMetrics, 8)

	lineStart := 0
	for i, b := range raw {
		if b != '\n' && i != len(raw)-1 {
			continue
		}
		end := i
		if b != '\n' { // EOF w/o \n
			end++
		}
		line := raw[lineStart:end]
		lineStart = i + 1
		if len(line) == 0 || line[0] == '#' {
			continue
		}

		nameEnd := bytes.IndexByte(line, '{')
		if nameEnd == -1 {
			nameEnd = bytes.IndexByte(line, ' ')
		}
		if nameEnd == -1 {
			continue
		}
		//name := unsafeString(line[:nameEnd])
		name := string(line[:nameEnd])
		switch name {

		// ---------- input ----------
		case "input_connection_failed":
			count, err := TailInt(line)
			if err != nil {
				return Metrics{}, fmt.Errorf("failed to parse input connection failed: %w", err)
			}
			m.Input.ConnectionFailed = count
		case "input_connection_lost":
			count, err := TailInt(line)
			if err != nil {
				return Metrics{}, fmt.Errorf("failed to parse input connection lost: %w", err)
			}
			m.Input.ConnectionLost = count
		case "input_connection_up":
			count, err := TailInt(line)
			if err != nil {
				return Metrics{}, fmt.Errorf("failed to parse input connection up: %w", err)
			}
			m.Input.ConnectionUp = count
		case "input_received":
			count, err := TailInt(line)
			if err != nil {
				return Metrics{}, fmt.Errorf("failed to parse input received: %w", err)
			}
			m.Input.Received = count
		case "input_latency_ns":
			switch extractLabel(line[nameEnd:], "quantile") {
			case "0.5":
				p50, err := TailInt(line)
				if err != nil {
					return Metrics{}, fmt.Errorf("failed to parse input latency ns p50: %w", err)
				}
				m.Input.LatencyNS.P50 = float64(p50)
			case "0.9":
				p90, err := TailInt(line)
				if err != nil {
					return Metrics{}, fmt.Errorf("failed to parse input latency ns p90: %w", err)
				}
				m.Input.LatencyNS.P90 = float64(p90)
			case "0.99":
				p99, err := TailInt(line)
				if err != nil {
					return Metrics{}, fmt.Errorf("failed to parse input latency ns p99: %w", err)
				}
				m.Input.LatencyNS.P99 = float64(p99)
			}
		case "input_latency_ns_sum":
			sum, err := TailInt(line)
			if err != nil {
				return Metrics{}, fmt.Errorf("failed to parse input latency ns sum: %w", err)
			}
			m.Input.LatencyNS.Sum = float64(sum)
		case "input_latency_ns_count":
			count, err := TailInt(line)
			if err != nil {
				return Metrics{}, fmt.Errorf("failed to parse input latency ns count: %w", err)
			}
			m.Input.LatencyNS.Count = count

		// ---------- output ----------
		case "output_batch_sent":
			count, err := TailInt(line)
			if err != nil {
				return Metrics{}, fmt.Errorf("failed to parse output batch sent: %w", err)
			}
			m.Output.BatchSent = count
		case "output_connection_failed":
			count, err := TailInt(line)
			if err != nil {
				return Metrics{}, fmt.Errorf("failed to parse output connection failed: %w", err)
			}
			m.Output.ConnectionFailed = count
		case "output_connection_lost":
			count, err := TailInt(line)
			if err != nil {
				return Metrics{}, fmt.Errorf("failed to parse output connection lost: %w", err)
			}
			m.Output.ConnectionLost = count
		case "output_connection_up":
			count, err := TailInt(line)
			if err != nil {
				return Metrics{}, fmt.Errorf("failed to parse output connection up: %w", err)
			}
			m.Output.ConnectionUp = count
		case "output_error":
			count, err := TailInt(line)
			if err != nil {
				return Metrics{}, fmt.Errorf("failed to parse output error: %w", err)
			}
			m.Output.Error = count
		case "output_sent":
			count, err := TailInt(line)
			if err != nil {
				return Metrics{}, fmt.Errorf("failed to parse output sent: %w", err)
			}
			m.Output.Sent = count
		case "output_latency_ns":
			switch extractLabel(line[nameEnd:], "quantile") {
			case "0.5":
				p50, err := TailInt(line)
				if err != nil {
					return Metrics{}, fmt.Errorf("failed to parse output latency ns p50: %w", err)
				}
				m.Output.LatencyNS.P50 = float64(p50)
			case "0.9":
				p90, err := TailInt(line)
				if err != nil {
					return Metrics{}, fmt.Errorf("failed to parse output latency ns p90: %w", err)
				}
				m.Output.LatencyNS.P90 = float64(p90)
			case "0.99":
				p99, err := TailInt(line)
				if err != nil {
					return Metrics{}, fmt.Errorf("failed to parse output latency ns p99: %w", err)
				}
				m.Output.LatencyNS.P99 = float64(p99)
			}
		case "output_latency_ns_sum":
			sum, err := TailInt(line)
			if err != nil {
				return Metrics{}, fmt.Errorf("failed to parse output latency ns sum: %w", err)
			}
			m.Output.LatencyNS.Sum = float64(sum)
		case "output_latency_ns_count":
			count, err := TailInt(line)
			if err != nil {
				return Metrics{}, fmt.Errorf("failed to parse output latency ns count: %w", err)
			}
			m.Output.LatencyNS.Count = count

		// ---------- processors ----------
		default:
			if !bytes.HasPrefix(line, []byte("processor_")) {
				continue
			}
			path := extractLabel(line[nameEnd:], "path")
			if path == "" {
				continue
			}
			pm := m.Process.Processors[path]
			if pm.Label == "" {
				pm.Label = extractLabel(line[nameEnd:], "label")
			}

			switch name {
			case "processor_received":
				count, err := TailInt(line)
				if err != nil {
					return Metrics{}, fmt.Errorf("failed to parse processor received: %w", err)
				}
				pm.Received = count
			case "processor_batch_received":
				count, err := TailInt(line)
				if err != nil {
					return Metrics{}, fmt.Errorf("failed to parse processor batch received: %w", err)
				}
				pm.BatchReceived = count
			case "processor_sent":
				count, err := TailInt(line)
				if err != nil {
					return Metrics{}, fmt.Errorf("failed to parse processor sent: %w", err)
				}
				pm.Sent = count
			case "processor_batch_sent":
				count, err := TailInt(line)
				if err != nil {
					return Metrics{}, fmt.Errorf("failed to parse processor batch sent: %w", err)
				}
				pm.BatchSent = count
			case "processor_error":
				count, err := TailInt(line)
				if err != nil {
					return Metrics{}, fmt.Errorf("failed to parse processor error: %w", err)
				}
				pm.Error = count
			case "processor_latency_ns":
				switch extractLabel(line[nameEnd:], "quantile") {
				case "0.5":
					p50, err := TailInt(line)
					if err != nil {
						return Metrics{}, fmt.Errorf("failed to parse processor latency ns p50: %w", err)
					}
					pm.LatencyNS.P50 = float64(p50)
				case "0.9":
					p90, err := TailInt(line)
					if err != nil {
						return Metrics{}, fmt.Errorf("failed to parse processor latency ns p90: %w", err)
					}
					pm.LatencyNS.P90 = float64(p90)
				case "0.99":
					p99, err := TailInt(line)
					if err != nil {
						return Metrics{}, fmt.Errorf("failed to parse processor latency ns p99: %w", err)
					}
					pm.LatencyNS.P99 = float64(p99)
				}
			case "processor_latency_ns_sum":
				sum, err := TailInt(line)
				if err != nil {
					return Metrics{}, fmt.Errorf("failed to parse processor latency ns sum: %w", err)
				}
				pm.LatencyNS.Sum = float64(sum)
			case "processor_latency_ns_count":
				count, err := TailInt(line)
				if err != nil {
					return Metrics{}, fmt.Errorf("failed to parse processor latency ns count: %w", err)
				}
				pm.LatencyNS.Count = count
			}
			m.Process.Processors[path] = pm
		}
	}
	return m, nil
}

// extractLabel scans the raw label-set of a Prometheus series (the `{…}`
// portion) and returns the *value* of a given label key.
//
// We read **only** the labels we care about (e.g. `path` or `quantile`), so
// this hand-rolled matcher is an order of magnitude faster than allocating a
// full `map[string]string` for every series.
func extractLabel(b []byte, key string) string {
	// expects {label="...",path="..."} order irrelevant
	key += "=\""
	i := bytes.Index(b, []byte(key))
	if i == -1 {
		return ""
	}
	i += len(key)
	j := bytes.IndexByte(b[i:], '"')
	if j == -1 {
		return ""
	}
	//return unsafeString(b[i : i+j])
	return string(b[i : i+j])
}

// unsafeString converts a []byte to string without allocation.
// Use only for read-only parsing.
//func unsafeString(b []byte) string { return *(*string)(unsafe.Pointer(&b)) }

// ParseMetricsFromBytes parses prometheus metrics into structured format
// Deprecated: Use ParseMetricsFromBytes instead
func ParseMetricsFromBytesSlow(data []byte) (Metrics, error) {
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
func (s *BenthosMonitorService) Status(ctx context.Context, services serviceregistry.Provider, tick uint64) (ServiceInfo, error) {
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

	// If the current state is stopped, we can return immediately
	// There wont be any logs, metrics, etc. to check
	if fsmState == s6fsm.OperationalStateStopped || fsmState == s6fsm.OperationalStateStopping {
		return ServiceInfo{}, monitor.ErrServiceStopped
	}

	// Get logs
	s6ServicePath := filepath.Join(constants.S6BaseDir, s6ServiceName)
	logs, err := s.s6Service.GetLogs(ctx, s6ServicePath, services.GetFileSystem())
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
			LastScan:  metrics,
			IsRunning: fsmState == s6fsm.OperationalStateRunning,
			Logs:      logs,
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
	s6ServiceName := s.GetS6ServiceName()
	s6Config, err := s.GenerateS6ConfigForBenthosMonitor(s6ServiceName, port)
	if err != nil {
		return fmt.Errorf("failed to generate S6 config for BenthosMonitor service %s: %w", s6ServiceName, err)
	}

	s.s6ServiceConfig.S6ServiceConfig = s6Config

	return nil
}

// RemoveBenthosMonitorFromS6Manager is the monitor counterpart of
// RemoveBenthosFromS6Manager.
//
// Implementation is kept **symmetrical** on purpose so both removal paths
// behave identically.  Any change in semantics here most likely has to be
// done in the main Benthos method as well.
//
// Returns:
//
//   - nil                    – monitor config + instance are gone
//   - ErrServiceNotExist     – never created / already gone
//   - ErrRemovalPending      – child S6-instance still busy
//   - other error            – unrecoverable I/O problem
func (s *BenthosMonitorService) RemoveBenthosMonitorFromS6Manager(ctx context.Context) error {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	s.s6ServiceConfig = nil

	// Check that the instance was actually removed
	s6Name := s.GetS6ServiceName()
	if inst, ok := s.s6Manager.GetInstance(s6Name); ok {
		return fmt.Errorf("%w: S6 instance state=%s",
			standarderrors.ErrRemovalPending, inst.GetCurrentFSMState())
	}

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
func (s *BenthosMonitorService) ReconcileManager(ctx context.Context, services serviceregistry.Provider, tick uint64) (err error, reconciled bool) {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized"), false
	}

	if ctx.Err() != nil {
		return ctx.Err(), false
	}

	serviceMap := config.FullConfig{Internal: config.InternalConfig{Services: []config.S6FSMConfig{}}}

	if s.s6ServiceConfig != nil {
		serviceMap.Internal.Services = []config.S6FSMConfig{*s.s6ServiceConfig}
	}

	return s.s6Manager.Reconcile(ctx, fsm.SystemSnapshot{CurrentConfig: serviceMap}, services)
}

// ServiceExists checks if a benthos instance exists
func (s *BenthosMonitorService) ServiceExists(ctx context.Context, services serviceregistry.Provider) bool {
	if s.s6Manager == nil {
		return false
	}

	if ctx.Err() != nil {
		return false
	}

	exists, err := s.s6Service.ServiceExists(ctx, filepath.Join(constants.S6BaseDir, s.GetS6ServiceName()), services.GetFileSystem())
	if err != nil {
		return false
	}

	return exists
}

// ForceRemoveBenthosMonitor removes a Benthos monitor from the S6 manager
// This should only be called if the Benthos monitor is in a permanent failure state
// and the instance itself cannot be stopped or removed
// Expects benthosName (e.g. "myservice") as defined in the UMH config
func (s *BenthosMonitorService) ForceRemoveBenthosMonitor(ctx context.Context, services serviceregistry.Provider) error {
	s6ServiceName := s.GetS6ServiceName()
	s6ServicePath := filepath.Join(constants.S6BaseDir, s6ServiceName)
	return services.GetFileSystem().RemoveAll(ctx, s6ServicePath)
}
