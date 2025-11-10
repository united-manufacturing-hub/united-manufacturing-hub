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
	"sync"
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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/standarderrors"
	"go.uber.org/zap"

	benthos_monitor_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos_monitor"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	"gopkg.in/yaml.v3"

	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"

	"github.com/cespare/xxhash/v2"
)

const (
	// OPCDebugEnvVar is the environment variable name for enabling OPC UA debug logging.
	//
	// This variable is read by the gopcua library (github.com/gopcua/opcua/debug/debug.go)
	// and accepts space-separated flag names to control different debug output levels:
	//
	//   - "debug"  : Enables general debug logging with file:line prefixes (debug.Printf)
	//   - "codec"  : Prints detailed encoding/decoding information during binary marshaling
	//   - "packet" : Prints raw packet data being sent/received over the wire
	//
	// Multiple flags can be combined: OPC_DEBUG="debug codec"
	//
	// When debug_level is enabled in config, this is automatically set to "debug" to enable
	// general OPC UA debugging without the verbosity of codec/packet-level output.
	//
	// Reference: /Users/jeremytheocharis/umh-git/opcua/debug/debug.go.
	OPCDebugEnvVar = "OPC_DEBUG"
)

// IBenthosService is the interface for managing Benthos services.
type IBenthosService interface {
	// GenerateS6ConfigForBenthos generates a S6 config for a given benthos instance
	// Expects s6ServiceName (e.g. "benthos-myservice"), not the raw benthosName
	GenerateS6ConfigForBenthos(benthosConfig *benthosserviceconfig.BenthosServiceConfig, s6ServiceName string) (s6serviceconfig.S6ServiceConfig, error)
	// GetConfig returns the actual Benthos config from the S6 service
	// Expects benthosName (e.g. "myservice") as defined in the UMH config
	GetConfig(ctx context.Context, filesystemService filesystem.Service, benthosName string) (benthosserviceconfig.BenthosServiceConfig, error)
	// Status checks the status of a Benthos service
	// Expects benthosName (e.g. "myservice") as defined in the UMH config
	Status(ctx context.Context, services serviceregistry.Provider, benthosName string, metricsPort uint16, tick uint64, loopStartTime time.Time) (ServiceInfo, error)
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
	// Expects snapshot (fsm.SystemSnapshot) containing current state and timestamp
	ReconcileManager(ctx context.Context, services serviceregistry.Provider, snapshot fsm.SystemSnapshot) (error, bool)
	// IsLogsFine reports true when recent Benthos logs (within the supplied
	// window) contain no critical errors or warnings.
	//
	// It returns:
	//
	//	ok    – true when logs look clean, false otherwise.
	//	entry – zero value when ok is true; otherwise the first offending log line.
	IsLogsFine(logs []s6service.LogEntry, currentTime time.Time, logWindow time.Duration) (bool, s6service.LogEntry)
	// IsMetricsErrorFree reports true when Benthos metrics contain no error
	// counters.
	//
	// It returns:
	//
	//	ok     – true when metrics are error‑free, false otherwise.
	//	reason – empty when ok is true; otherwise a short explanation (e.g.
	//	         "benthos reported 3 processor errors").
	IsMetricsErrorFree(metrics benthos_monitor.BenthosMetrics) (bool, string)
	// HasProcessingActivity reports true when metrics (or other status signals)
	// indicate the instance is actively processing data.
	//
	// It returns:
	//
	//	ok     – true when activity is detected, false otherwise.
	//	reason – empty when ok is true; otherwise a short explanation such as
	//	         "no input throughput (in=0.00 msg/s, out=0.00 msg/s)".
	HasProcessingActivity(status BenthosStatus) (bool, string)
}

// ServiceInfo contains information about a Benthos service.
type ServiceInfo struct {
	// S6FSMState contains the current state of the S6 FSM
	S6FSMState string
	// S6ObservedState contains information about the S6 service
	S6ObservedState s6fsm.S6ObservedState
	// BenthosStatus contains information about the status of the Benthos service
	BenthosStatus BenthosStatus
}

type BenthosStatus struct {

	// StatusReason contains the reason for the status of the Benthos service
	// If the service is degraded, this will contain the log entry that caused the degradation together with the information that it is degraded because of the log entry
	// If the service is currently starting up, it will contain the s6 status of the service
	StatusReason string
	// BenthosLogs contains the structured s6 log entries emitted by the
	// Benthos service.
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
	//     logs (see DefaultService.GetLogs), so sharing the slice’s backing
	//     array across snapshots cannot introduce data races.
	//
	// Therefore we override the default behaviour and copy only the 3-word
	// slice header (24 B on amd64) — see CopyBenthosLogs below.
	BenthosLogs []s6service.LogEntry
	// HealthCheck contains information about the health of the Benthos service
	HealthCheck benthos_monitor.HealthCheck
	// BenthosMetrics contains information about the metrics of the Benthos service
	BenthosMetrics benthos_monitor.BenthosMetrics
}

// CopyBenthosLogs is a go-deepcopy override for the BenthosLogs field.
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
//  2. BenthosLogs is treated as immutable after the snapshot is taken.
//
// If either assumption changes, delete this method to fall back to the default
// deep-copy (O(n) but safe for mutable slices).
//
// See also: https://github.com/tiendc/go-deepcopy?tab=readme-ov-file#copy-struct-fields-via-struct-methods
func (bs *BenthosStatus) CopyBenthosLogs(src []s6service.LogEntry) error {
	bs.BenthosLogs = src

	return nil
}

// BenthosService is the default implementation of the IBenthosService interface.
type BenthosService struct {
	s6Service s6service.Service // S6 service for direct S6 operations
	logger    *zap.SugaredLogger

	s6Manager *s6fsm.S6Manager

	benthosMonitorManager *benthos_monitor_fsm.BenthosMonitorManager

	// -----------------------------------------------------------------------------
	// 🌶️  Hot-path YAML-parsing cache
	// -----------------------------------------------------------------------------

	// configCache stores one *normalised* Benthos config per logical Benthos
	// instance (“my-pipe”, “orders2db”, …).
	//
	//   • **Key**   – logical benthosName (without the "benthos-" prefix)
	//   • **Value** – configCacheEntry{hash, parsed}
	//
	// We use sync.Map instead of a plain map+mutex because:
	//
	//   1. GetConfig might be called concurrently;
	//      the cache is therefore *read-heavy* and *contention-light*.
	//   2. sync.Map gives us lock-free reads and amortised-O(1) writes, which
	//      is exactly what we need for a “mostly reads, very few writes” workload.
	configCache      sync.Map // map[string]configCacheEntry
	s6ServiceConfigs []config.S6FSMConfig

	benthosMonitorConfigs []config.BenthosMonitorConfig
}

// configCacheEntry is the value stored in configCache.
//
// A quick uint64 xxHash lets us tell in ~1 ns if the YAML has changed since
// the previous call.  If the hash is identical we can *skip* the expensive
// yaml.Unmarshal and simply hand the already-normalised struct back to the
// caller – a ~20× speed-up on the hot path.
type configCacheEntry struct {

	// parsed is the *fully normalised* BenthosServiceConfig that callers
	// expect.  It is treated as **read-only** after being cached; if callers
	// ever start mutating the struct, we must clone it before returning.
	parsed benthosserviceconfig.BenthosServiceConfig
	// hash is xxhash.Sum64(buf) of the raw YAML file.  Collisions are
	// vanishingly unlikely (2⁻⁶⁴), so equality is “good enough” to treat
	// the file as unchanged.
	hash uint64
}

// hash is a helper function for configCacheEntry.hash.
func hash(buf []byte) uint64 { return xxhash.Sum64(buf) }

// benthosLogRe is a helper function for BenthosService.IsLogsFine.
var benthosLogRe = regexp.MustCompile(`^level=(error|warning)\s+msg=(.+)`)

// BenthosServiceOption is a function that modifies a BenthosService.
type BenthosServiceOption func(*BenthosService)

// WithS6Service sets a custom S6 service for the BenthosService.
func WithS6Service(s6Service s6service.Service) BenthosServiceOption {
	return func(s *BenthosService) {
		s.s6Service = s6Service
	}
}

// WithMonitorManager sets a custom monitor manager for the BenthosService.
func WithMonitorManager(monitorManager *benthos_monitor_fsm.BenthosMonitorManager) BenthosServiceOption {
	return func(s *BenthosService) {
		s.benthosMonitorManager = monitorManager
	}
}

// WithS6Manager sets a custom S6 manager for the BenthosService.
func WithS6Manager(s6Manager *s6fsm.S6Manager) BenthosServiceOption {
	return func(s *BenthosService) {
		s.s6Manager = s6Manager
	}
}

// NewDefaultBenthosService creates a new default Benthos service
// name is the name of the Benthos service as defined in the UMH config.
func NewDefaultBenthosService(benthosName string, opts ...BenthosServiceOption) *BenthosService {
	managerName := fmt.Sprintf("%s%s", logger.ComponentBenthosService, benthosName)
	service := &BenthosService{
		logger:                logger.For(managerName),
		s6Manager:             s6fsm.NewS6Manager(managerName),
		s6Service:             s6service.NewDefaultService(),
		benthosMonitorManager: benthos_monitor_fsm.NewBenthosMonitorManager(benthosName),
	}

	// Apply options
	for _, opt := range opts {
		opt(service)
	}

	return service
}

// generateBenthosYaml generates a Benthos YAML configuration from a BenthosServiceConfig.
func (s *BenthosService) generateBenthosYaml(config *benthosserviceconfig.BenthosServiceConfig) (string, error) {
	if config == nil {
		return "", errors.New("config is nil")
	}

	return benthosserviceconfig.RenderBenthosYAML(
		config.Input,
		config.Output,
		config.Pipeline,
		config.CacheResources,
		config.RateLimitResources,
		config.Buffer,
		config.MetricsPort,
		config.DebugLevel,
	)
}

// GetS6ServiceName converts a logical Benthos name ("my-pipe") into the
// canonical S6 service name ("benthos-my-pipe").
//
// It is exported ONLY because tests and other packages need the mapping.
func (s *BenthosService) GetS6ServiceName(benthosName string) string {
	return "benthos-" + benthosName
}

// generateS6ConfigForBenthos creates a S6 config for a given benthos instance
// Expects s6ServiceName (e.g. "benthos-myservice"), not the raw benthosName.
func (s *BenthosService) GenerateS6ConfigForBenthos(benthosConfig *benthosserviceconfig.BenthosServiceConfig, s6ServiceName string) (s6Config s6serviceconfig.S6ServiceConfig, err error) {
	configPath := fmt.Sprintf("%s/%s/config/%s", constants.S6BaseDir, s6ServiceName, constants.BenthosConfigFileName)

	yamlConfig, err := s.generateBenthosYaml(benthosConfig)
	if err != nil {
		return s6serviceconfig.S6ServiceConfig{}, err
	}

	env := make(map[string]string)
	if benthosConfig.DebugLevel {
		env[OPCDebugEnvVar] = "debug"
	}

	s6Config = s6serviceconfig.S6ServiceConfig{
		Command: []string{
			"/usr/local/bin/benthos",
			"-c",
			configPath,
		},
		Env: env,
		ConfigFiles: map[string]string{
			constants.BenthosConfigFileName: yamlConfig,
		},
	}

	return s6Config, nil
}

// GetConfig returns the actual Benthos config from the S6 service
// Expects benthosName (e.g. "myservice") as defined in the UMH config.
func (s *BenthosService) GetConfig(ctx context.Context, filesystemService filesystem.Service, benthosName string) (benthosserviceconfig.BenthosServiceConfig, error) {
	if ctx.Err() != nil {
		return benthosserviceconfig.BenthosServiceConfig{}, ctx.Err()
	}

	s6ServiceName := s.GetS6ServiceName(benthosName)
	s6ServicePath := filepath.Join(constants.S6BaseDir, s6ServiceName)

	// Request the config file from the S6 service
	yamlData, err := s.s6Service.GetS6ConfigFile(ctx, s6ServicePath, constants.BenthosConfigFileName, filesystemService)
	if err != nil {
		return benthosserviceconfig.BenthosServiceConfig{}, fmt.Errorf("failed to get benthos config file for service %s: %w", s6ServiceName, err)
	}

	h := hash(yamlData)

	// ---------- fast path: YAML identical to last call ----------
	if v, ok := s.configCache.Load(benthosName); ok {
		entry := v.(configCacheEntry)
		if entry.hash == h {
			// Nothing changed – return the cached, already-normalised struct
			return entry.parsed, nil
		}
	}

	// ---------- slow path: YAML changed ----------
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
			result.DebugLevel = (level == "DEBUG")
		}
	}

	parsed := benthosserviceconfig.NormalizeBenthosConfig(result)

	// Store the parsed config in the cache
	s.configCache.Store(benthosName, configCacheEntry{
		hash:   h,
		parsed: parsed,
	})

	// Normalize the config to ensure consistent defaults
	return parsed, nil
}

// extractMetricsPort safely extracts the metrics port from the config map
// Returns 0 if any part of the path is missing or invalid.
func (s *BenthosService) extractMetricsPort(config map[string]interface{}) uint16 {
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

	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return 0
	}
	// This cast is safe because we know the port is a valid uint16
	return uint16(port)
}

// Status checks the status of a Benthos service and returns ServiceInfo
// Expects benthosName (e.g. "myservice") as defined in the UMH config.
func (s *BenthosService) Status(ctx context.Context, services serviceregistry.Provider, benthosName string, metricsPort uint16, tick uint64, loopStartTime time.Time) (ServiceInfo, error) {
	if ctx.Err() != nil {
		return ServiceInfo{}, ctx.Err()
	}

	s6ServiceName := s.GetS6ServiceName(benthosName)

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

	logs, err := s.s6Service.GetLogs(ctx, s6ServicePath, services.GetFileSystem())
	if err != nil {
		switch {
		case errors.Is(err, s6service.ErrServiceNotExist):
			s.logger.Debugf("Service %s does not exist, returning empty logs", s6ServiceName)

			return ServiceInfo{}, ErrServiceNotExist
		case errors.Is(err, s6service.ErrLogFileNotFound):
			s.logger.Debugf("Log file for service %s not found, returning empty logs", s6ServiceName)

			return ServiceInfo{}, ErrServiceNotExist
		default:
			return ServiceInfo{}, fmt.Errorf("failed to get logs: %w", err)
		}
	}

	benthosStatus, err := s.GetHealthCheckAndMetrics(ctx, services.GetFileSystem(), tick, loopStartTime, benthosName, logs)
	if err != nil {
		errStr := err.Error()
		switch {
		case strings.Contains(errStr, ErrLastObservedStateNil.Error()):
			return ServiceInfo{
				S6ObservedState: s6ServiceObservedState,
				S6FSMState:      s6FSMState,
				BenthosStatus: BenthosStatus{
					BenthosLogs: logs,
				},
			}, ErrLastObservedStateNil
		case strings.Contains(errStr, "instance "+s6ServiceName+" not found") ||
			strings.Contains(errStr, "not found"):
			s.logger.Debugf("Service %s was removed during status check", s6ServiceName)

			return ServiceInfo{}, ErrServiceNotExist
		case strings.Contains(errStr, ErrBenthosMonitorNotRunning.Error()):
			s.logger.Debugf("Service %s is not running, returning empty logs", s6ServiceName)

			return ServiceInfo{
				S6ObservedState: s6ServiceObservedState,
				S6FSMState:      s6FSMState,
				BenthosStatus:   benthosStatus,
			}, ErrBenthosMonitorNotRunning
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
	start := time.Now()

	defer func() {
		metrics.ObserveReconcileTime(logger.ComponentBenthosService, metrics.ComponentBenthosService+"_get_health_check_and_metrics", time.Since(start))
	}()

	if ctx.Err() != nil {
		return BenthosStatus{}, ctx.Err()
	}

	// Skip health checks and metrics if the service doesn't exist yet
	// This avoids unnecessary errors in Status() when the service is still being created
	if _, exists := s.s6Manager.GetInstance(s.GetS6ServiceName(benthosName)); !exists {
		return BenthosStatus{}, nil
	}

	// Get the last observed state of the benthos monitor
	s6ServiceName := s.GetS6ServiceName(benthosName)

	lastObservedState, err := s.benthosMonitorManager.GetLastObservedState(s6ServiceName)
	if err != nil {
		return BenthosStatus{}, fmt.Errorf("failed to get last observed state in GetHealthCheckAndMetrics: %w", err)
	}

	if lastObservedState == nil {
		return BenthosStatus{}, ErrLastObservedStateNil
	}

	// Convert the last observed state to a BenthosMonitorObservedState
	lastBenthosMonitorObservedState, ok := lastObservedState.(benthos_monitor_fsm.BenthosMonitorObservedState)
	if !ok {
		return BenthosStatus{}, fmt.Errorf("last observed state is not a BenthosMonitorObservedState: %v", lastObservedState)
	}

	var benthosStatus BenthosStatus

	if lastBenthosMonitorObservedState.ServiceInfo == nil {
		return BenthosStatus{}, ErrLastObservedStateNil
	}

	// if everything is fine, set the status to the service info
	if lastBenthosMonitorObservedState.ServiceInfo.BenthosStatus.LastScan != nil {
		benthosStatus.HealthCheck = lastBenthosMonitorObservedState.ServiceInfo.BenthosStatus.LastScan.HealthCheck
		benthosStatus.BenthosMetrics = *lastBenthosMonitorObservedState.ServiceInfo.BenthosStatus.LastScan.BenthosMetrics
		benthosStatus.BenthosLogs = lastBenthosMonitorObservedState.ServiceInfo.BenthosStatus.Logs
	} else {
		return BenthosStatus{}, errors.New("last scan is nil")
	}

	// If the service is not running, we can return immediately
	// There wont be any logs, metrics, etc. to check
	if !lastBenthosMonitorObservedState.ServiceInfo.BenthosStatus.IsRunning {
		return BenthosStatus{}, ErrBenthosMonitorNotRunning
	}

	return benthosStatus, nil
}

// AddBenthosToS6Manager adds a Benthos instance to the S6 manager
// Expects benthosName (e.g. "myservice") as defined in the UMH config.
func (s *BenthosService) AddBenthosToS6Manager(ctx context.Context, filesystemService filesystem.Service, cfg *benthosserviceconfig.BenthosServiceConfig, benthosName string) error {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	s.logger.Debugf("Adding benthos to S6 manager with port: %d", cfg.MetricsPort)

	s6ServiceName := s.GetS6ServiceName(benthosName)

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
			DesiredFSMState: s6fsm.OperationalStateStopped,
		},
		S6ServiceConfig: s6Config,
	}

	// Add the S6 FSM config to the list of S6 FSM configs
	// so that the S6 manager will start the service
	s.s6ServiceConfigs = append(s.s6ServiceConfigs, s6FSMConfig)

	// Now do it the same with the benthos monitor
	benthosMonitorConfig := config.BenthosMonitorConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            s6ServiceName,
			DesiredFSMState: benthos_monitor_fsm.OperationalStateStopped,
		},
		MetricsPort: cfg.MetricsPort,
	}

	s.benthosMonitorConfigs = append(s.benthosMonitorConfigs, benthosMonitorConfig)

	return nil
}

// UpdateBenthosInS6Manager updates an existing Benthos instance in the S6 manager
// Expects benthosName (e.g. "myservice") as defined in the UMH config.
func (s *BenthosService) UpdateBenthosInS6Manager(ctx context.Context, filesystemService filesystem.Service, cfg *benthosserviceconfig.BenthosServiceConfig, benthosName string) error {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	s6ServiceName := s.GetS6ServiceName(benthosName)

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

	// Now update the benthos monitor config
	benthosMonitorDesiredState := benthos_monitor_fsm.OperationalStateActive
	if currentDesiredState == s6fsm.OperationalStateStopped {
		benthosMonitorDesiredState = benthos_monitor_fsm.OperationalStateStopped
	}

	s.benthosMonitorConfigs[index] = config.BenthosMonitorConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            s6ServiceName,
			DesiredFSMState: benthosMonitorDesiredState,
		},
		MetricsPort: cfg.MetricsPort,
	}

	// The next reconciliation of the S6 manager will detect the config change
	// and update the service
	return nil
}

// RemoveBenthosFromS6Manager drives the *gradual* teardown of a Benthos
// service and its monitor:
//
//  1. Delete the entry from `s6ServiceConfigs` and `benthosMonitorConfigs`
//     → at the next reconcile cycle the child managers will move the
//     respective S6 FSMs to *removing*.
//  2. Return ErrRemovalPending until **both** child instances have
//     completely vanished from their managers.
//  3. Once neither instance exists anymore, return nil.
//
// The method is **idempotent** and **non-blocking**; callers are supposed
// to invoke it every cycle while the parent FSM is in the removing state.
//
// Notice that we *do not* try to stop the underlying S6 service ourselves –
// that is entirely the responsibility of the S6-manager + S6-FSM that own
// the child instance.  We only manipulate *desired* config and observe the
// effect.
func (s *BenthosService) RemoveBenthosFromS6Manager(
	ctx context.Context,
	fs filesystem.Service,
	benthosName string,
) error {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized")
	}

	if ctx.Err() != nil { // context already cancelled / timed-out
		return ctx.Err()
	}

	s.logger.Infof("Removing benthos from S6 manager with name: %s", benthosName)

	// ------------------------------------------------------------------
	// 1) Delete the *desired* config entry so the S6-manager will stop it
	// ------------------------------------------------------------------
	s6Name := s.GetS6ServiceName(benthosName)

	// Helper that deletes exactly one element *without* reallocating when the
	// element is already missing – keeps the call idempotent and allocation-free.
	sliceRemoveByName := func(arr []config.S6FSMConfig, name string) []config.S6FSMConfig {
		for i, cfg := range arr {
			if cfg.Name == name {
				return append(arr[:i], arr[i+1:]...)
			}
		}

		return arr
	}

	// helper for BenthosMonitorConfig slices
	sliceRemoveMonitorByName := func(arr []config.BenthosMonitorConfig, name string) []config.BenthosMonitorConfig {
		for i, cfg := range arr {
			if cfg.Name == name {
				return append(arr[:i], arr[i+1:]...)
			}
		}

		return arr
	}

	s.s6ServiceConfigs = sliceRemoveByName(s.s6ServiceConfigs, s6Name)
	s.benthosMonitorConfigs = sliceRemoveMonitorByName(s.benthosMonitorConfigs, s6Name)

	// ------------------------------------------------------------------
	// 2) Are the instances already gone?
	// ------------------------------------------------------------------
	if inst, ok := s.s6Manager.GetInstance(s6Name); ok {
		return fmt.Errorf("%w: S6 instance state=%s",
			standarderrors.ErrRemovalPending, inst.GetCurrentFSMState())
	}

	if inst, ok := s.benthosMonitorManager.GetInstance(s6Name); ok {
		return fmt.Errorf("%w: monitor instance state=%s",
			standarderrors.ErrRemovalPending, inst.GetCurrentFSMState())
	}

	// Everything really removed ➜ success, idempotent
	return nil
}

// StartBenthos starts a Benthos instance
// Expects benthosName (e.g. "myservice") as defined in the UMH config.
func (s *BenthosService) StartBenthos(ctx context.Context, filesystemService filesystem.Service, benthosName string) error {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	s6ServiceName := s.GetS6ServiceName(benthosName)

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

	// Also start the benthos monitor
	for i, benthosMonitorConfig := range s.benthosMonitorConfigs {
		if benthosMonitorConfig.Name == s6ServiceName {
			s.benthosMonitorConfigs[i].DesiredFSMState = benthos_monitor_fsm.OperationalStateActive
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
// Expects benthosName (e.g. "myservice") as defined in the UMH config.
func (s *BenthosService) StopBenthos(ctx context.Context, filesystemService filesystem.Service, benthosName string) error {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	s6ServiceName := s.GetS6ServiceName(benthosName)

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

	// Also stop the benthos monitor
	for i, benthosMonitorConfig := range s.benthosMonitorConfigs {
		if benthosMonitorConfig.Name == s6ServiceName {
			s.benthosMonitorConfigs[i].DesiredFSMState = benthos_monitor_fsm.OperationalStateStopped
			found = true

			break
		}
	}

	if !found {
		return ErrServiceNotExist
	}

	return nil
}

// ReconcileManager reconciles the Benthos manager.
func (s *BenthosService) ReconcileManager(ctx context.Context, services serviceregistry.Provider, snapshot fsm.SystemSnapshot) (err error, reconciled bool) {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized"), false
	}

	if ctx.Err() != nil {
		return ctx.Err(), false
	}

	// Create a new snapshot with the current S6 service configs
	// Note: therefore, the S6 manager will not have access to the full observed state
	// Pass the snapshot time from parent to maintain "one time.Now() per tick" principle
	s6Snapshot := fsm.SystemSnapshot{
		CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: s.s6ServiceConfigs}},
		Tick:          snapshot.Tick,
		SnapshotTime:  snapshot.SnapshotTime,
	}

	s6Err, s6Reconciled := s.s6Manager.Reconcile(ctx, s6Snapshot, services)
	if s6Err != nil {
		return fmt.Errorf("failed to reconcile S6 manager: %w", s6Err), false
	}

	// Also reconcile the benthos monitor

	// Pass the snapshot time from parent to maintain "one time.Now() per tick" principle
	benthosMonitorSnapshot := fsm.SystemSnapshot{
		CurrentConfig: config.FullConfig{Internal: config.InternalConfig{BenthosMonitor: s.benthosMonitorConfigs}},
		Tick:          snapshot.Tick,
		SnapshotTime:  snapshot.SnapshotTime,
	}

	monitorErr, monitorReconciled := s.benthosMonitorManager.Reconcile(ctx, benthosMonitorSnapshot, services)
	if monitorErr != nil {
		return fmt.Errorf("failed to reconcile benthos monitor: %w", monitorErr), false
	}

	// If either was reconciled, indicate that reconciliation occurred
	return nil, s6Reconciled || monitorReconciled
}

// IsLogsFine reports true when recent Benthos logs (within the supplied
// window) contain no critical errors or warnings.
//
// It returns:
//
//	ok    – true when logs look clean, false otherwise.
//	entry – zero value when ok is true; otherwise the first offending log line.
func (s *BenthosService) IsLogsFine(
	logs []s6service.LogEntry,
	now time.Time,
	window time.Duration,
) (bool, s6service.LogEntry) {
	if len(logs) == 0 {
		return true, s6service.LogEntry{}
	}

	cutoff := now.Add(-window)         // pre-compute once
	critWarnSubstrings := [...]string{ // small, fixed slice
		"failed to", "connection lost", "unable to",
	}

	for _, l := range logs {
		if l.Timestamp.Before(cutoff) {
			continue // outside the window
		}

		// Very cheap checks first — just bytes/prefix matches.
		switch {
		case strings.HasPrefix(l.Content, "configuration file read error:"),
			strings.HasPrefix(l.Content, "failed to create logger:"),
			strings.HasPrefix(l.Content, "Config lint error:"):
			return false, l
		}

		// Only one regexp call per line.
		if m := benthosLogRe.FindStringSubmatch(l.Content); m != nil {
			level, msg := m[1], m[2]

			if level == "error" {
				return false, l
			}

			if level == "warning" {
				for _, sub := range critWarnSubstrings {
					if strings.Contains(msg, sub) {
						return false, l
					}
				}
			}
		}
	}

	return true, s6service.LogEntry{}
}

// IsMetricsErrorFree reports true when Benthos metrics contain no error
// counters.
//
// It returns:
//
//	ok     – true when metrics are error‑free, false otherwise.
//	reason – empty when ok is true; otherwise a short explanation (e.g.
//	         "benthos reported 3 processor errors").
func (s *BenthosService) IsMetricsErrorFree(metrics benthos_monitor.BenthosMetrics) (bool, string) {
	// Check output errors
	if metrics.Metrics.Output.Error > 0 {
		return false, fmt.Sprintf("benthos reported %d output errors", metrics.Metrics.Output.Error)
	}

	// Check processor errors
	for _, proc := range metrics.Metrics.Process.Processors {
		if proc.Error > 0 {
			return false, fmt.Sprintf("benthos reported %d processor errors", proc.Error)
		}
	}

	return true, ""
}

// HasProcessingActivity reports true when metrics (or other status signals)
// indicate the instance is actively processing data.
//
// It returns:
//
//	ok     – true when activity is detected, false otherwise.
//	reason – empty when ok is true; otherwise a short explanation such as
//	         "no input throughput (in=0.00 msg/s, out=0.00 msg/s)".
func (s *BenthosService) HasProcessingActivity(status BenthosStatus) (bool, string) {
	if status.BenthosMetrics.MetricsState == nil {
		return false, "benthos metrics state is nil"
	}

	if status.BenthosMetrics.MetricsState.IsActive {
		return true, ""
	}

	msgPerSecInput := status.BenthosMetrics.MetricsState.Input.MessagesPerTick / constants.DefaultTickerTime.Seconds()
	msgPerSecOutput := status.BenthosMetrics.MetricsState.Output.MessagesPerTick / constants.DefaultTickerTime.Seconds()

	return false, fmt.Sprintf("no input throughput (in=%.2f msg/s, out=%.2f msg/s)",
		msgPerSecInput, msgPerSecOutput)
}

// ServiceExists checks if a Benthos service exists in the S6 manager.
func (s *BenthosService) ServiceExists(ctx context.Context, filesystemService filesystem.Service, benthosName string) bool {
	s6ServiceName := s.GetS6ServiceName(benthosName)
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
// Expects benthosName (e.g. "myservice") as defined in the UMH config.
func (s *BenthosService) ForceRemoveBenthos(ctx context.Context, filesystemService filesystem.Service, benthosName string) error {
	s6ServiceName := s.GetS6ServiceName(benthosName)
	s6ServicePath := filepath.Join(constants.S6BaseDir, s6ServiceName)

	return s.s6Service.ForceRemove(ctx, s6ServicePath, filesystemService)
}
