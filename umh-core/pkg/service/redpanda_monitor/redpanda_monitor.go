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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/process_shared"
	"io"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/prometheus/common/expfmt"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/process_manager_serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/standarderrors"

	dto "github.com/prometheus/client_model/go"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager"
	"go.uber.org/zap"

	"github.com/cespare/xxhash/v2"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/textparse"
)

// Metrics contains information about the metrics of the Redpanda service
type Metrics struct {
	Infrastructure InfrastructureMetrics
	Cluster        ClusterMetrics
	Throughput     ThroughputMetrics
	Topic          TopicMetrics
}

// InfrastructureMetrics contains information about the infrastructure metrics of the Redpanda service
type InfrastructureMetrics struct {
	Storage StorageMetrics
}

// StorageMetrics contains information about the storage metrics of the Redpanda service
type StorageMetrics struct {
	// redpanda_storage_disk_free_bytes
	// type: gauge
	// Docs: https://docs.redpanda.com/current/reference/public-metrics-reference/#redpanda_storage_disk_free_bytes
	FreeBytes int64
	// redpanda_storage_disk_total_bytes
	// type: gauge
	// Docs: https://docs.redpanda.com/current/reference/public-metrics-reference/#redpanda_storage_disk_total_bytes
	TotalBytes int64
	// redpanda_storage_disk_free_space_alert (0 == false, everything else == true)
	// type: gauge
	FreeSpaceAlert bool
}

// ClusterMetrics contains information about the cluster metrics of the Redpanda service
type ClusterMetrics struct {
	// redpanda_cluster_topics
	// type: gauge
	// Docs: https://docs.redpanda.com/current/reference/public-metrics-reference/#redpanda_cluster_topics
	Topics int64
	// redpanda_cluster_unavailable_partitions
	// type: gauge
	// Docs: https://docs.redpanda.com/current/reference/public-metrics-reference/#redpanda_cluster_unavailable_partitions
	UnavailableTopics int64
}

// ThroughputMetrics contains information about the throughput metrics of the Redpanda service
type ThroughputMetrics struct {
	// redpanda_kafka_request_bytes_total over all redpanda_namespace and redpanda_topic labels using redpanda_request=("produce")
	// type: counter
	// Docs: https://docs.redpanda.com/current/reference/public-metrics-reference/#redpanda_kafka_request_bytes_total
	BytesIn int64
	// redpanda_kafka_request_bytes_total over all redpanda_namespace and redpanda_topic labels using redpanda_request=("consume")
	// type: counter
	// Docs: https://docs.redpanda.com/current/reference/public-metrics-reference/#redpanda_kafka_request_bytes_total
	BytesOut int64
}

// TopicMetrics contains information about the topic metrics of the Redpanda service
type TopicMetrics struct {
	// redpanda_kafka_partitions
	// type: gauge
	// Docs: https://docs.redpanda.com/current/reference/public-metrics-reference/#redpanda_kafka_partitions
	TopicPartitionMap map[string]int64
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
	// ReadyError contains any error message from the ready check
	ReadyError string `json:"ready_error,omitempty"`
}

type ClusterConfig struct {
	Topic TopicConfig
}

type TopicConfig struct {
	DefaultTopicRetentionMs          int64
	DefaultTopicRetentionBytes       int64
	DefaultTopicCompressionAlgorithm string
	DefaultTopicCleanupPolicy        string
	DefaultTopicSegmentMs            int64
}

// RedpandaMetrics contains information about the metrics of the Redpanda service
type RedpandaMetrics struct {
	// Metrics contains the metrics of the Redpanda service
	Metrics Metrics
	// MetricsState contains the last observed state of the metrics
	MetricsState *RedpandaMetricsState
}

// ServiceInfo contains information about a redpanda service
type ServiceInfo struct {
	// S6ObservedState contains information about the S6 redpanda_monitor service
	S6ObservedState s6fsm.S6ObservedState
	// S6FSMState contains the current state of the S6 FSM of the redpanda_monitor service
	S6FSMState string
	// RedpandaStatus contains information about the status of the redpanda service
	RedpandaStatus RedpandaMonitorStatus
}

// RedpandaMonitorScan contains information about the status of the Redpanda service
type RedpandaMetricsScan struct {
	// HealthCheck contains information about the health of the Redpanda service
	HealthCheck HealthCheck
	// Metrics contains information about the metrics of the Redpanda service
	RedpandaMetrics *RedpandaMetrics
	// ClusterConfig contains information about the cluster config of the Redpanda service
	ClusterConfig *ClusterConfig
	// LastUpdatedAt contains the last time the metrics were updated
	LastUpdatedAt time.Time
}

// RedpandaMonitorStatus contains status information about the redpanda service
type RedpandaMonitorStatus struct {
	// LastScan contains the result of the last scan
	// If this is nil, we never had a successfull scan
	LastScan *RedpandaMetricsScan
	// IsRunning indicates whether the redpanda_monitor service is running
	IsRunning bool
	// Logs contains the structured s6 log entries emitted by the
	// redpanda_monitor service.
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
	Logs []process_shared.LogEntry
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
func (rms *RedpandaMonitorStatus) CopyLogs(src []process_shared.LogEntry) error {
	rms.Logs = src
	return nil
}

type IRedpandaMonitorService interface {
	GenerateS6ConfigForRedpandaMonitor(s6ServiceName string) (process_manager_serviceconfig.ProcessManagerServiceConfig, error)
	Status(ctx context.Context, filesystemService filesystem.Service, tick uint64) (ServiceInfo, error)
	AddRedpandaMonitorToS6Manager(ctx context.Context) error
	RemoveRedpandaMonitorFromS6Manager(ctx context.Context) error
	StartRedpandaMonitor(ctx context.Context) error
	StopRedpandaMonitor(ctx context.Context) error
	ReconcileManager(ctx context.Context, services serviceregistry.Provider, tick uint64) (error, bool)
	ServiceExists(ctx context.Context, filesystemService filesystem.Service) bool
	ForceRemoveRedpandaMonitor(ctx context.Context, filesystemService filesystem.Service) error
}

// Ensure RedpandaMonitorService implements IRedpandaMonitorService
var _ IRedpandaMonitorService = (*RedpandaMonitorService)(nil)

type RedpandaMonitorService struct {
	logger          *zap.SugaredLogger
	metricsState    *RedpandaMetricsState
	s6Manager       *s6fsm.S6Manager
	s6Service       s6service.Service
	s6ServiceConfig *config.S6FSMConfig // There can only be one instance of this service (as there is also only one redpanda instance)
	redpandaName    string              // normally a service can handle multiple instances, the service monitor here is different and can only handle one instance

	// Cache metrics/clusterconfig/timestamp data
	previousMetricsDataByteHash       uint64
	previousClusterConfigDataByteHash uint64
	previousTimestampDataByteHash     uint64
	previousMetrics                   *RedpandaMetrics
	previousClusterConfig             *ClusterConfig
	previousLastUpdatedAt             time.Time
	// cacheMutex is used to synchronize access to above cache variables
	cacheMutex sync.Mutex
}

// RedpandaMonitorServiceOption is a function that modifies a RedpandaMonitorService
type RedpandaMonitorServiceOption func(*RedpandaMonitorService)

// WithS6Service sets a custom S6 service for the RedpandaMonitorService
func WithS6Service(s6Service s6service.Service) RedpandaMonitorServiceOption {
	return func(s *RedpandaMonitorService) {
		s.s6Service = s6Service
	}
}

// WithS6Manager sets a custom S6 manager for the RedpandaMonitorService
func WithS6Manager(s6Manager *s6fsm.S6Manager) RedpandaMonitorServiceOption {
	return func(s *RedpandaMonitorService) {
		s.s6Manager = s6Manager
	}
}

func NewRedpandaMonitorService(redpandaName string, opts ...RedpandaMonitorServiceOption) *RedpandaMonitorService {
	managerName := fmt.Sprintf("%s%s", logger.ComponentRedpandaService, redpandaName)
	service := &RedpandaMonitorService{
		logger:       logger.For(managerName),
		metricsState: NewRedpandaMetricsState(),
		s6Manager:    s6fsm.NewS6Manager(logger.ComponentRedpandaMonitorService),
		s6Service:    s6service.NewDefaultService(),
		redpandaName: redpandaName,
	}
	for _, opt := range opts {
		opt(service)
	}
	return service
}

// BLOCK_START_MARKER marks the begin of a new data block inside the logs.
// Between it and MID_MARKER is the metrics data, between MID_MARKER and END_MARKER is the cluster config data.
const BLOCK_START_MARKER = "BEGINBEGINBEGINBEGINBEGINBEGINBEGINBEGINBEGINBEGINBEGINBEGINBEGINBEGINBEGINBEGINBEGIN"

// METRICS_END_MARKER marks the end of the metrics data and the beginning of the cluster config data.
const METRICS_END_MARKER = "METRICSENDMETRICSENDMETRICSENDMETRICSENDMETRICSENDMETRICSENDMETRICSENDMETRICSENDMETRICSENDMETRICSEND"

// CLUSTERCONFIG_END_MARKER marks the end of the cluster config data and the beginning of the timestamp data.
const CLUSTERCONFIG_END_MARKER = "CONFIGENDCONFIGENDCONFIGENDCONFIGENDCONFIGENDCONFIGENDCONFIGENDCONFIGENDCONFIGENDCONFIGENDCONFIGEND"

// READYNESS_END_MARKER marks the end of the readyness data and the beginning of the timestamp data.
const READYNESS_END_MARKER = "READYNESSENDREADYNESSENDREADYNESSENDREADYNESSENDREADYNESSENDREADYNESSENDREADYNESSENDREADYNESSENDREADYNESSENDREADYNESSEND"

// BLOCK_END_MARKER marks the end of the cluster config data.
const BLOCK_END_MARKER = "ENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDEND"

func (s *RedpandaMonitorService) generateRedpandaScript() (string, error) {
	// Build the redpanda command - curl http://localhost:9644/public_metrics
	// Create the script content with a loop that executes redpanda every second
	// Also let's use gzip to compress the output & hex encode it
	// We use gzip here, to prevent the output from being rotated halfway through the logs & hex encode it to avoid issues with special characters
	// Max-time: https://everything.monitor.dev/usingcurl/timeouts.html
	// The timestamp here is the unix nanosecond timestamp of the current time
	// It is gathered AFTER the curl commands, preventing long curl execution times from affecting the timestamp
	// +%s%9N: %s is the unix timestamp in seconds with 9 decimal places for nanoseconds
	scriptContent := fmt.Sprintf(`#!/bin/sh
while true; do
  echo "%s"
  curl -sSL --max-time 1 http://localhost:%d/public_metrics 2>&1 | gzip -c | xxd -p
  echo "%s"
  curl -sSL --max-time 1 http://localhost:%d/v1/cluster_config 2>&1 | gzip -c | xxd -p
  echo "%s"
  curl -sSL --max-time 1 http://localhost:%d/v1/status/ready 2>&1 | gzip -c | xxd -p
  echo "%s"
  date +%%s%%9N
  echo "%s"
  sleep 1
done
`, BLOCK_START_MARKER, constants.AdminAPIPort, METRICS_END_MARKER, constants.AdminAPIPort, CLUSTERCONFIG_END_MARKER, constants.AdminAPIPort, READYNESS_END_MARKER, BLOCK_END_MARKER)

	return scriptContent, nil
}

func (s *RedpandaMonitorService) GetS6ServiceName() string {
	return fmt.Sprintf("redpanda-monitor-%s", s.redpandaName)
}

func (s *RedpandaMonitorService) GenerateS6ConfigForRedpandaMonitor(s6ServiceName string) (process_manager_serviceconfig.ProcessManagerServiceConfig, error) {
	scriptContent, err := s.generateRedpandaScript()
	if err != nil {
		return process_manager_serviceconfig.ProcessManagerServiceConfig{}, err
	}

	s6Config := process_manager_serviceconfig.ProcessManagerServiceConfig{
		Command: []string{
			"/bin/sh",
			fmt.Sprintf("%s/%s/config/run_redpanda_monitor.sh", constants.S6BaseDir, s6ServiceName),
		},
		Env: map[string]string{},
		ConfigFiles: map[string]string{
			"run_redpanda_monitor.sh": scriptContent,
		},
	}

	return s6Config, nil
}

// GetConfig is not implemented, as the config is static

type Section struct {
	StartMarkerIndex            int
	MetricsEndMarkerIndex       int
	ClusterConfigEndMarkerIndex int
	ReadynessEndMarkerIndex     int
	BlockEndMarkerIndex         int
}

// ConcatContent concatenates the content of a slice of LogEntry objects into a single byte slice.
// It calculates the total size of the content, allocates a buffer of that size, and then copies each LogEntry's content into the buffer.
// This approach is more efficient than using multiple strings.ReplaceAll calls or regex operations.
func ConcatContent(logs []process_shared.LogEntry) []byte {
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
	METRICS_END_MARKER, "",
	CLUSTERCONFIG_END_MARKER, "",
	READYNESS_END_MARKER, "",
	BLOCK_END_MARKER, "",
)

// stripMarkers returns a copy of b with every marker removed.
// If you're on Go ≥ 1.20 you can avoid the extra string-copy with
//
//	unsafe.String and unsafe.Slice, but the plain version is often fast enough.
func StripMarkers(b []byte) []byte {
	return []byte(markerReplacer.Replace(string(b)))
}

// ParseRedpandaLogs parses the logs of a redpanda service and extracts metrics
func (s *RedpandaMonitorService) ParseRedpandaLogs(ctx context.Context, logs []process_shared.LogEntry, tick uint64) (*RedpandaMetricsScan, error) {
	/*
		A normal log entry looks like this:
		BLOCK_START_MARKER
		Hex encoded gzip data of the metrics
		METRICS_END_MARKER
		Hex encoded gzip data of the cluster config
		CLUSTERCONFIG_END_MARKER
		Hex encoded gzip data of the readyness
		READYNESS_END_MARKER
		Timestamp data
		BLOCK_END_MARKER
	*/

	if len(logs) == 0 {
		s.logger.Debugf("No logs provided")
		return nil, fmt.Errorf("no logs provided")
	}
	// Find the markers in a single pass through the logs
	// Pre-allocate the slice to avoid reallocations (we usually expect ~4-7 sections)
	sections := make([]Section, 0, 10)

	currentSection := Section{
		StartMarkerIndex:            -1,
		MetricsEndMarkerIndex:       -1,
		ClusterConfigEndMarkerIndex: -1,
		ReadynessEndMarkerIndex:     -1,
		BlockEndMarkerIndex:         -1,
	}

	// This implementation scans the logs in a single pass, which is more efficient than scanning for each marker separately
	// If the there are multiple sections, we will have multiple entries in the sections list
	// This ensures that we always have a valid section, even if the markers of later sections are missing (e.g the end marker for example was not yet written)
	for i := 0; i < len(logs); i++ {
		if strings.Contains(logs[i].Content, BLOCK_START_MARKER) {
			currentSection.StartMarkerIndex = i
		} else if strings.Contains(logs[i].Content, METRICS_END_MARKER) {
			// Dont even try to find an end marker, if we dont have a start marker
			if currentSection.StartMarkerIndex == -1 {
				continue
			}
			currentSection.MetricsEndMarkerIndex = i
		} else if strings.Contains(logs[i].Content, CLUSTERCONFIG_END_MARKER) {
			// Dont even try to find an end marker, if we dont have a start marker
			if currentSection.StartMarkerIndex == -1 {
				continue
			}
			currentSection.ClusterConfigEndMarkerIndex = i
		} else if strings.Contains(logs[i].Content, READYNESS_END_MARKER) {
			// Dont even try to find an end marker, if we dont have a start marker
			if currentSection.StartMarkerIndex == -1 {
				continue
			}
			currentSection.ReadynessEndMarkerIndex = i
		} else if strings.Contains(logs[i].Content, BLOCK_END_MARKER) {
			// We dont break here, as there might be multiple end markers
			currentSection.BlockEndMarkerIndex = i

			// If we have all sections add it to the list, otherwise discard !
			if currentSection.StartMarkerIndex != -1 && currentSection.MetricsEndMarkerIndex != -1 && currentSection.ClusterConfigEndMarkerIndex != -1 && currentSection.ReadynessEndMarkerIndex != -1 && currentSection.BlockEndMarkerIndex != -1 {
				// Check if the order makes sense, otherwise discard
				if currentSection.StartMarkerIndex < currentSection.MetricsEndMarkerIndex && currentSection.MetricsEndMarkerIndex < currentSection.ClusterConfigEndMarkerIndex && currentSection.ClusterConfigEndMarkerIndex < currentSection.ReadynessEndMarkerIndex && currentSection.ReadynessEndMarkerIndex < currentSection.BlockEndMarkerIndex {
					sections = append(sections, currentSection)
				}
			}

			// Reset the current section
			currentSection = Section{
				StartMarkerIndex:            -1,
				MetricsEndMarkerIndex:       -1,
				ClusterConfigEndMarkerIndex: -1,
				BlockEndMarkerIndex:         -1,
			}
		}
	}

	if len(sections) == 0 {
		s.logger.Debugf("No sections found in logs")
		return nil, fmt.Errorf("could not parse redpanda metrics/configuration: no sections found. This can happen when the redpanda service is not running, or the logs where rotated")
	}

	// Find the latest section that is fully constructed (e.g the latest entry in the list)
	actualSection := sections[len(sections)-1]

	// We need to extract the lines between the markers
	// Metrics is the first part, cluster config is the second part, timestamp is the third part
	metricsData := logs[actualSection.StartMarkerIndex+1 : actualSection.MetricsEndMarkerIndex]
	clusterConfigData := logs[actualSection.MetricsEndMarkerIndex+1 : actualSection.ClusterConfigEndMarkerIndex]
	readynessData := logs[actualSection.ClusterConfigEndMarkerIndex+1 : actualSection.ReadynessEndMarkerIndex]
	timestampData := logs[actualSection.ReadynessEndMarkerIndex+1 : actualSection.BlockEndMarkerIndex]

	var metricsDataBytes []byte
	var clusterConfigDataBytes []byte
	var readynessDataBytes []byte
	var timestampDataBytes []byte

	var readyness bool

	metricsDataBytes = ConcatContent(metricsData)
	clusterConfigDataBytes = ConcatContent(clusterConfigData)
	readynessDataBytes = ConcatContent(readynessData)
	timestampDataBytes = ConcatContent(timestampData)

	// Remove any markers that might be in the data
	metricsDataBytes = StripMarkers(metricsDataBytes)
	clusterConfigDataBytes = StripMarkers(clusterConfigDataBytes)
	readynessDataBytes = StripMarkers(readynessDataBytes)
	timestampDataBytes = StripMarkers(timestampDataBytes)

	metricsDataByteHash := xxhash.Sum64(metricsDataBytes)
	clusterConfigDataByteHash := xxhash.Sum64(clusterConfigDataBytes)
	timestampDataByteHash := xxhash.Sum64(timestampDataBytes)

	metricsChanged := s.previousMetricsDataByteHash != metricsDataByteHash
	clusterConfigChanged := s.previousClusterConfigDataByteHash != clusterConfigDataByteHash
	timestampChanged := s.previousTimestampDataByteHash != timestampDataByteHash

	// Now parse the readyness data
	readyness, readynessError, err := s.parseReadynessData(readynessDataBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse readyness data: %w", err)
	}

	if !metricsChanged && !clusterConfigChanged && !timestampChanged {
		healthCheck := HealthCheck{
			IsLive:     readyness,
			IsReady:    readyness,
			Version:    constants.RedpandaVersion,
			ReadyError: readynessError,
		}
		// No changes, return the cached values
		return &RedpandaMetricsScan{
			RedpandaMetrics: s.previousMetrics,
			ClusterConfig:   s.previousClusterConfig,
			LastUpdatedAt:   s.previousLastUpdatedAt,
			HealthCheck:     healthCheck,
		}, nil
	}

	var metrics *RedpandaMetrics
	var clusterConfig *ClusterConfig
	var lastUpdatedAt time.Time
	// Processing the Metrics & cluster config takes time (especially the metrics parsing) each, therefore process them in parallel

	ctxProcessMetrics, cancelProcessMetrics := context.WithTimeout(ctx, constants.RedpandaMonitorProcessMetricsTimeout)
	defer cancelProcessMetrics()
	g, _ := errgroup.WithContext(ctxProcessMetrics)

	if metricsChanged {
		g.Go(func() error {
			var err error
			metrics, err = s.processMetricsDataBytes(metricsDataBytes, tick)
			return err
		})
	} else {
		metrics = s.previousMetrics
	}

	if clusterConfigChanged {
		g.Go(func() error {
			var err error
			clusterConfig, err = s.processClusterConfigDataBytes(clusterConfigDataBytes, tick)
			return err
		})
	} else {
		clusterConfig = s.previousClusterConfig
	}

	if metricsChanged || clusterConfigChanged {
		// Create a buffered channel to receive the result from g.Wait().
		// The channel is buffered so that the goroutine sending on it doesn't block.
		errc := make(chan error, 1)

		// Run g.Wait() in a separate goroutine.
		// This allows us to use a select statement to return early if the context is canceled.
		go func() {
			// g.Wait() blocks until all goroutines launched with g.Go() have returned.
			// It returns the first non-nil error, if any.
			errc <- g.Wait()
		}()

		// Use a select statement to wait for either the g.Wait() result or the context's cancellation.
		select {
		case err := <-errc:
			// g.Wait() has finished, so check if any goroutine returned an error.
			if err != nil {
				// If there was an error in any sub-call, return that error.
				return nil, err
			}
			// If err is nil, all goroutines completed successfully.
		case <-ctxProcessMetrics.Done():
			// The context was canceled or its deadline was exceeded before all goroutines finished.
			// Although some goroutines might still be running in the background,
			// they use a context (gctx) that should cause them to terminate promptly.
			return nil, ctxProcessMetrics.Err()
		}
	}

	if timestampChanged {
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
		// Convert the nano seconds to a time.Time object
		lastUpdatedAt = time.Unix(0, int64(timestampNs))
	}

	// Update previous values
	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()
	s.previousMetricsDataByteHash = metricsDataByteHash
	s.previousClusterConfigDataByteHash = clusterConfigDataByteHash
	s.previousTimestampDataByteHash = timestampDataByteHash
	s.previousMetrics = metrics
	s.previousClusterConfig = clusterConfig
	s.previousLastUpdatedAt = lastUpdatedAt

	healthCheck := HealthCheck{
		IsLive:     readyness,
		IsReady:    readyness,
		Version:    constants.RedpandaVersion,
		ReadyError: readynessError,
	}

	return &RedpandaMetricsScan{
		HealthCheck:     healthCheck,
		RedpandaMetrics: metrics,
		ClusterConfig:   clusterConfig,
		LastUpdatedAt:   lastUpdatedAt,
	}, nil
}

func (s *RedpandaMonitorService) processMetricsDataBytes(metricsDataBytes []byte, tick uint64) (*RedpandaMetrics, error) {

	gzr, err := gzip.NewReader(hex.NewDecoder(bytes.NewReader(metricsDataBytes)))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer func() {
		if err := gzr.Close(); err != nil {
			s.logger.Error("failed to close gzip reader", zap.Error(err))
		}
	}()

	metrics, err := parseMetricsBlob(gzr)
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

// parseMetricsBlob converts a single gzip-compressed + hex-encoded response
// from the Redpanda monitoring side-car into a `Metrics` struct.
//
// The helper exists to keep the higher-level log parser readable: decompression
// and hex-decoding are detail noise, while *this* function encapsulates all
// error handling related to corrupt or timed-out `curl` replies.
func parseMetricsBlob(r io.Reader) (Metrics, error) {
	payload, err := io.ReadAll(r)
	if err != nil {
		return Metrics{}, err
	}
	if curl := monitor.ParseCurlError(string(payload)); curl != nil {
		return Metrics{}, curl
	}
	return ParseMetricsFast(payload)
}

// ParseMetricsFast is a zero-allocation Prometheus text parser tuned for
// **exactly** the subset of Redpanda metrics the UMH-core needs for
// reconciliation decisions:
//
//   - disk-space signals (free/total/alert)
//   - cluster health (topic & partition counts)
//   - throughput counters (`produce` / `consume`)
//   - per-topic partition cardinality
//
// By specialising the parser we squeeze > 10× more throughput out of the
// reconciler loop compared to the generic `expfmt` decoder, which in turn
// allows the agent to monitor dozens of brokers on sub-millisecond budgets.
func ParseMetricsFast(b []byte) (Metrics, error) {
	var (
		m Metrics

		foundFreeBytes, foundTotalBytes, foundFreeSpaceAlert bool
		foundClusterTopics, foundUnavailablePartitions       bool
		foundProduce, foundConsume                           bool
		sawPartitionsMetric                                  bool
	)

	p := textparse.NewPromParser(b, labels.NewSymbolTable())

	for {
		typ, err := p.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return m, fmt.Errorf("iterating metric stream: %w", err)
		}
		if typ != textparse.EntrySeries {
			continue
		}

		metricBytes, _, val := p.Series()
		mName := seriesName(metricBytes) // ← metric id without labels

		// Allocate a new slice only when we need labels.
		var lbls labels.Labels
		if mName == "redpanda_kafka_request_bytes_total" ||
			mName == "redpanda_kafka_partitions" {
			p.Labels(&lbls) // fills lbls; never nil afterwards
		}

		switch mName {
		// ── infrastructure ────────────────────────────────────────────────
		case "redpanda_storage_disk_free_bytes":
			m.Infrastructure.Storage.FreeBytes = int64(val)
			foundFreeBytes = true

		case "redpanda_storage_disk_total_bytes":
			m.Infrastructure.Storage.TotalBytes = int64(val)
			foundTotalBytes = true

		case "redpanda_storage_disk_free_space_alert":
			m.Infrastructure.Storage.FreeSpaceAlert = val != 0
			foundFreeSpaceAlert = true

		// ── cluster ───────────────────────────────────────────────────────
		case "redpanda_cluster_topics":
			m.Cluster.Topics = int64(val)
			foundClusterTopics = true

		case "redpanda_cluster_unavailable_partitions":
			m.Cluster.UnavailableTopics = int64(val)
			foundUnavailablePartitions = true

		// ── throughput  (needs a label) ───────────────────────────────────
		case "redpanda_kafka_request_bytes_total":
			/*
				We need to sum up the values over all topics

					# HELP redpanda_kafka_request_bytes_total Total number of bytes produced per topic
					# TYPE redpanda_kafka_request_bytes_total counter
					redpanda_kafka_request_bytes_total{redpanda_namespace="kafka",redpanda_request="produce",redpanda_topic="test"} 27768
					redpanda_kafka_request_bytes_total{redpanda_namespace="kafka",redpanda_request="follower_consume",redpanda_topic="test"} 0
					redpanda_kafka_request_bytes_total{redpanda_namespace="kafka",redpanda_request="follower_consume",redpanda_topic="messages"} 0
					redpanda_kafka_request_bytes_total{redpanda_namespace="kafka",redpanda_request="consume",redpanda_topic="test"} 24564
					redpanda_kafka_request_bytes_total{redpanda_namespace="redpanda",redpanda_request="produce",redpanda_topic="controller"} 0
					redpanda_kafka_request_bytes_total{redpanda_namespace="redpanda",redpanda_request="follower_consume",redpanda_topic="controller"} 0
					redpanda_kafka_request_bytes_total{redpanda_namespace="kafka",redpanda_request="produce",redpanda_topic="messages"} 0
					redpanda_kafka_request_bytes_total{redpanda_namespace="redpanda",redpanda_request="consume",redpanda_topic="controller"} 0
					redpanda_kafka_request_bytes_total{redpanda_namespace="kafka",redpanda_request="consume",redpanda_topic="messages"} 0
			*/
			if lbls == nil {
				return m, fmt.Errorf("metric redpanda_kafka_request_bytes_total has no labels")
			}
			switch lbls.Get("redpanda_request") {
			case "produce":
				m.Throughput.BytesIn += int64(val)
				foundProduce = true
			case "consume":
				m.Throughput.BytesOut += int64(val)
				foundConsume = true
			}

		// ── per-topic (needs a label) ─────────────────────────────────────
		case "redpanda_kafka_partitions":
			if lbls == nil {
				return m, fmt.Errorf("metric redpanda_kafka_partitions has no labels")
			}
			if topic := lbls.Get("redpanda_topic"); topic != "" {
				if m.Topic.TopicPartitionMap == nil {
					// a small, non-zero initial capacity
					m.Topic.TopicPartitionMap = make(map[string]int64, 16)
				}
				m.Topic.TopicPartitionMap[topic] = int64(val)
				sawPartitionsMetric = true
			}
		}
	}

	// ── validation – mirrors the old expfmt version ──────────────────────
	switch {
	case !foundFreeBytes:
		return m, fmt.Errorf("metric redpanda_storage_disk_free_bytes not found")
	case !foundTotalBytes:
		return m, fmt.Errorf("metric redpanda_storage_disk_total_bytes not found")
	case !foundFreeSpaceAlert:
		return m, fmt.Errorf("metric redpanda_storage_disk_free_space_alert not found")
	case !foundClusterTopics:
		return m, fmt.Errorf("metric redpanda_cluster_topics not found")
	case !foundUnavailablePartitions:
		return m, fmt.Errorf("metric redpanda_cluster_unavailable_partitions not found")
	case !foundProduce:
		return m, fmt.Errorf(`metric redpanda_kafka_request_bytes_total with label redpanda_request="produce" not found`)
	case !foundConsume:
		return m, fmt.Errorf(`metric redpanda_kafka_request_bytes_total with label redpanda_request="consume" not found`)
	case m.Cluster.Topics > 0 && !sawPartitionsMetric:
		return m, fmt.Errorf("metric redpanda_kafka_partitions not found but redpanda_cluster_topics reports %d topics",
			m.Cluster.Topics)
	}

	return m, nil
}

// helper – cheap split without allocations
func seriesName(b []byte) string {
	if b == nil {
		return ""
	}
	if i := bytes.IndexByte(b, '{'); i > 0 {
		return string(b[:i])
	}
	return string(b)
}

func (s *RedpandaMonitorService) parseReadynessData(readynessDataBytes []byte) (bool, string, error) {
	readynessDataString := string(readynessDataBytes)

	// Strip any newlines
	readynessDataString = strings.ReplaceAll(readynessDataString, "\n", "")

	// Decode the hex encoded metrics data
	decodedMetricsDataBytes, err := hex.DecodeString(readynessDataString)
	if err != nil {
		return false, "", fmt.Errorf("failed to decode readyness data: %w", err)
	}

	// Decompress the metrics data
	gzipReader, err := gzip.NewReader(bytes.NewReader(decodedMetricsDataBytes))
	if err != nil {
		return false, "", fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer func() {
		err := gzipReader.Close()
		if err != nil {
			sentry.ReportIssuef(sentry.IssueTypeError, s.logger, "failed to close gzip reader: %v", err)
		}
	}()

	data, err := io.ReadAll(gzipReader)
	if err != nil {
		return false, "", fmt.Errorf("failed to read readyness data: %w", err)
	}

	readyness := strings.Contains(string(data), "{\"status\":\"ready\"}")
	return readyness, string(data), nil
}

func (s *RedpandaMonitorService) processClusterConfigDataBytes(clusterConfigDataBytes []byte, tick uint64) (*ClusterConfig, error) {

	clusterConfigDataString := string(clusterConfigDataBytes)
	// Strip any newlines
	clusterConfigDataString = strings.ReplaceAll(clusterConfigDataString, "\n", "")

	// Decode the hex encoded metrics data
	decodedMetricsDataBytes, err := hex.DecodeString(clusterConfigDataString)
	if err != nil {
		return nil, fmt.Errorf("failed to decode metrics data: %w", err)
	}

	// Decompress the metrics data
	gzipReader, err := gzip.NewReader(bytes.NewReader(decodedMetricsDataBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to decompress metrics data: %w", err)
	}
	defer func() {
		err := gzipReader.Close()
		if err != nil {
			sentry.ReportIssuef(sentry.IssueTypeError, s.logger, "failed to close gzip reader: %v", err)
		}
	}()

	data, err := io.ReadAll(gzipReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read cluster config data: %w", err)
	}

	curlError := monitor.ParseCurlError(string(data))
	if curlError != nil {
		return nil, curlError
	}

	// Parse the JSON response
	var redpandaConfig map[string]interface{}
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&redpandaConfig); err != nil {
		return nil, fmt.Errorf("failed to parse cluster config data: %w", err)
	}

	var result ClusterConfig

	// Extract the values we need from the JSON
	if value, ok := redpandaConfig["log_retention_ms"]; ok {
		result.Topic.DefaultTopicRetentionMs, err = ParseRedpandaIntegerlikeValue(value)
		if err != nil {
			return nil, fmt.Errorf("failed to parse cluster config data: log_retention_ms is not a number: %w", err)
		}
	} else {
		return nil, fmt.Errorf("failed to parse cluster config data: no log_retention_ms found")
	}

	if value, ok := redpandaConfig["retention_bytes"]; ok {
		result.Topic.DefaultTopicRetentionBytes, err = ParseRedpandaIntegerlikeValue(value)
		if err != nil {
			return nil, fmt.Errorf("failed to parse cluster config data: retention_bytes is not a number: %w", err)
		}
	} else {
		return nil, fmt.Errorf("failed to parse cluster config data: no retention_bytes found")
	}

	if value, ok := redpandaConfig["log_compression_type"]; ok {
		if strValue, ok := value.(string); ok {
			result.Topic.DefaultTopicCompressionAlgorithm = strValue
		} else {
			return nil, fmt.Errorf("failed to parse cluster config data: log_compression_type is not a string")
		}
	} else {
		return nil, fmt.Errorf("failed to parse cluster config data: no log_compression_type found")
	}

	if value, ok := redpandaConfig["log_cleanup_policy"]; ok {
		if strValue, ok := value.(string); ok {
			result.Topic.DefaultTopicCleanupPolicy = strValue
		} else {
			return nil, fmt.Errorf("failed to parse cluster config data: log_cleanup_policy is not a string")
		}
	} else {
		return nil, fmt.Errorf("failed to parse cluster config data: no log_cleanup_policy found")
	}

	if value, ok := redpandaConfig["log_segment_ms"]; ok {
		result.Topic.DefaultTopicSegmentMs, err = ParseRedpandaIntegerlikeValue(value)
		if err != nil {
			return nil, fmt.Errorf("failed to parse cluster config data: log_segment_ms is not a number: %w", err)
		}
	} else {
		return nil, fmt.Errorf("failed to parse cluster config data: no log_segment_ms found")
	}

	s.logger.Debugf("Cluster config [log_retention_ms: %d, retention_bytes: %d, log_compression_type: %s, log_cleanup_policy: %s, log_segment_ms: %d]",
		result.Topic.DefaultTopicRetentionMs,
		result.Topic.DefaultTopicRetentionBytes,
		result.Topic.DefaultTopicCompressionAlgorithm,
		result.Topic.DefaultTopicCleanupPolicy,
		result.Topic.DefaultTopicSegmentMs)

	return &result, nil
}

// ParseMetrics parses prometheus metrics into structured format
func ParseMetrics(dataReader io.Reader) (Metrics, error) {
	var parser expfmt.TextParser
	metrics := Metrics{
		Infrastructure: InfrastructureMetrics{},
		Cluster:        ClusterMetrics{},
		Throughput:     ThroughputMetrics{},
		Topic: TopicMetrics{
			TopicPartitionMap: make(map[string]int64), // Pre-allocate map to avoid nil check later
		},
	}
	data, err := io.ReadAll(dataReader)
	if err != nil {
		return Metrics{}, fmt.Errorf("failed to read metrics data: %w", err)
	}

	curlError := monitor.ParseCurlError(string(data))
	if curlError != nil {
		return Metrics{}, curlError
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
	} else {
		return metrics, fmt.Errorf("metric redpanda_storage_disk_free_bytes not found")
	}

	if family, ok := mf["redpanda_storage_disk_total_bytes"]; ok && len(family.Metric) > 0 {
		metrics.Infrastructure.Storage.TotalBytes = getMetricValue(family.Metric[0])
	} else {
		return metrics, fmt.Errorf("metric redpanda_storage_disk_total_bytes not found")
	}

	if family, ok := mf["redpanda_storage_disk_free_space_alert"]; ok && len(family.Metric) > 0 {
		// Any non-zero value indicates an alert condition
		metrics.Infrastructure.Storage.FreeSpaceAlert = getMetricValue(family.Metric[0]) != 0
	} else {
		return metrics, fmt.Errorf("metric redpanda_storage_disk_free_space_alert not found")
	}

	// Cluster metrics
	if family, ok := mf["redpanda_cluster_topics"]; ok && len(family.Metric) > 0 {
		metrics.Cluster.Topics = getMetricValue(family.Metric[0])
	} else {
		return metrics, fmt.Errorf("metric redpanda_cluster_topics not found")
	}

	if family, ok := mf["redpanda_cluster_unavailable_partitions"]; ok && len(family.Metric) > 0 {
		metrics.Cluster.UnavailableTopics = getMetricValue(family.Metric[0])
	} else {
		return metrics, fmt.Errorf("metric redpanda_cluster_unavailable_partitions not found")
	}

	// Throughput metrics
	if family, ok := mf["redpanda_kafka_request_bytes_total"]; ok {
		// Process only produce/consume metrics in a single pass
		produceFound := false
		consumeFound := false
		for _, metric := range family.Metric {
			if label := getLabel(metric, "redpanda_request"); label != "" {
				switch label {
				case "produce":
					metrics.Throughput.BytesIn = getMetricValue(metric)
					produceFound = true
				case "consume":
					metrics.Throughput.BytesOut = getMetricValue(metric)
					consumeFound = true
				}
			}
		}
		if !produceFound {
			return metrics, fmt.Errorf("metric redpanda_kafka_request_bytes_total with label redpanda_request=produce not found")
		}
		if !consumeFound {
			return metrics, fmt.Errorf("metric redpanda_kafka_request_bytes_total with label redpanda_request=consume not found")
		}
	} else {
		return metrics, fmt.Errorf("metric redpanda_kafka_request_bytes_total not found")
	}

	// Topic metrics
	// If we have topics, then topic metrics should be available
	if family, ok := mf["redpanda_kafka_partitions"]; ok {
		for _, metric := range family.Metric {
			if topic := getLabel(metric, "redpanda_topic"); topic != "" {
				metrics.Topic.TopicPartitionMap[topic] = getMetricValue(metric)
			}
		}
	} else if metrics.Cluster.Topics > 0 {
		// Only fail if we have topics but can't find the partition metrics
		return metrics, fmt.Errorf("metric redpanda_kafka_partitions not found but redpanda_cluster_topics reports %d topics", metrics.Cluster.Topics)
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

	// If the current state is stopped or stopping, we can return immediately
	// There wont be any logs, metrics, etc. to check
	if fsmState == s6fsm.OperationalStateStopped || fsmState == s6fsm.OperationalStateStopping {
		return ServiceInfo{
			S6ObservedState: s6State,
			S6FSMState:      fsmState,
			RedpandaStatus: RedpandaMonitorStatus{
				IsRunning: false,
			},
		}, monitor.ErrServiceStopped
	}

	// Get logs
	s6ServicePath := filepath.Join(constants.S6BaseDir, s6ServiceName)
	logs, err := s.s6Service.GetLogs(ctx, s6ServicePath, filesystemService)
	if err != nil {
		return ServiceInfo{
			S6ObservedState: s6State,
			S6FSMState:      fsmState,
			RedpandaStatus: RedpandaMonitorStatus{
				IsRunning: false,
			},
		}, fmt.Errorf("failed to get logs: %w", err)
	}

	if len(logs) == 0 {
		return ServiceInfo{
			S6ObservedState: s6State,
			S6FSMState:      fsmState,
			RedpandaStatus: RedpandaMonitorStatus{
				IsRunning: false,
			},
		}, ErrServiceNoLogFile
	}

	// Parse the logs
	metrics, err := s.ParseRedpandaLogs(ctx, logs, tick)
	if err != nil {
		return ServiceInfo{
			S6ObservedState: s6State,
			S6FSMState:      fsmState,
			RedpandaStatus: RedpandaMonitorStatus{
				IsRunning: false,
			},
		}, fmt.Errorf("failed to parse metrics: %w", err)
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

	s6ServiceName := s.GetS6ServiceName()

	// Check whether s6ServiceConfigs already contains an entry for this instance
	if s.s6ServiceConfig != nil {
		return ErrServiceAlreadyExists
	}

	// Generate the S6 config for this instance
	s6Config, err := s.GenerateS6ConfigForRedpandaMonitor(s6ServiceName)
	if err != nil {
		return fmt.Errorf("failed to generate S6 config for RedpandaMonitor service %s: %w", s6ServiceName, err)
	}

	// Create the S6 FSM config for this instance
	s6FSMConfig := config.S6FSMConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            s6ServiceName,
			DesiredFSMState: s6fsm.OperationalStateStopped, // Ensure we start with a stopped service, so we can start it later
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

	s.s6ServiceConfig = nil

	// Check that the instance was actually removed
	s6Name := s.GetS6ServiceName()
	if inst, ok := s.s6Manager.GetInstance(s6Name); ok {
		return fmt.Errorf("%w: S6 instance state=%s",
			standarderrors.ErrRemovalPending, inst.GetCurrentFSMState())
	}

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
func (s *RedpandaMonitorService) ReconcileManager(ctx context.Context, services serviceregistry.Provider, tick uint64) (err error, reconciled bool) {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized"), false
	}

	if ctx.Err() != nil {
		return ctx.Err(), false
	}

	if s.s6ServiceConfig == nil {
		return ErrServiceNotExist, false
	}

	return s.s6Manager.Reconcile(ctx, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: []config.S6FSMConfig{*s.s6ServiceConfig}}}}, services)
}

// ServiceExists checks if a redpanda instance exists
func (s *RedpandaMonitorService) ServiceExists(ctx context.Context, filesystemService filesystem.Service) bool {
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

// ForceRemoveRedpandaMonitor removes a Redpanda monitor from the S6 manager
// This should only be called if the Redpanda monitor is in a permanent failure state
// and the instance itself cannot be stopped or removed
func (s *RedpandaMonitorService) ForceRemoveRedpandaMonitor(ctx context.Context, filesystemService filesystem.Service) error {
	s6ServiceName := s.GetS6ServiceName()
	s6ServicePath := filepath.Join(constants.S6BaseDir, s6ServiceName)
	return s.s6Service.ForceRemove(ctx, s6ServicePath, filesystemService)
}

func ParseRedpandaIntegerlikeValue(value interface{}) (int64, error) {
	// This can be a very large value (18446744073709552000 or 18446744073709551615) if set to 0 via the config.
	// We need to handle this, by saying that everything larger then 9223372036854775807 (max int64) is 0
	// Our generate handles 0 correctly (either as -1 or null, depending on the value)
	v, err := ParseValue(value)
	if err != nil {
		// If "value is nil", return 0, nil, as redpanda for "some" values returns nil instead of a high value :/
		if strings.Contains(err.Error(), "value is nil") || strings.Contains(err.Error(), "value is negative") {
			return 0, nil
		}
		return 0, err
	}
	if v > math.MaxInt64 {
		return 0, nil
	}
	// We can now safely cast to int64, as we checked above
	return int64(v), nil
}

func ParseValue(value interface{}) (uint64, error) {
	var result uint64

	switch v := value.(type) {
	case uint64:
		result = v
	case float64:
		// If v is negative, return 0, with "value is negative"
		if v < 0 {
			return 0, fmt.Errorf("value is negative")
		}
		// We remove fractional parts, as redpanda uses integer values only
		result = uint64(v)
	case int:
		// If v is negative, return 0, with "value is negative"
		if v < 0 {
			return 0, fmt.Errorf("value is negative")
		}
		result = uint64(v)
	case int64:
		// If v is negative, return 0, with "value is negative"
		if v < 0 {
			return 0, fmt.Errorf("value is negative")
		}
		result = uint64(v)
	case string:
		// Try to parse the string as a number
		parsed, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			// Try to parse the string as a float
			parsedFloat, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return 0, fmt.Errorf("failed to parse string value as uint64: %w", err)
			}
			// If v is negative, return 0, with "value is negative"
			if parsedFloat < 0 {
				return 0, fmt.Errorf("value is negative")
			}
			result = uint64(parsedFloat)
		} else {
			result = parsed
		}
	case nil:
		return 0, fmt.Errorf("value is nil")
	default:
		return 0, fmt.Errorf("unsupported value type for conversion to uint64: %T", value)
	}

	return result, nil
}
