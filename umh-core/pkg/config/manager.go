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

package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/backoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/ctxutil/ctxmutex"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/ctxutil/ctxrwmutex"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	filesystem "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

const (
	// DefaultConfigPath is the default path to the config file
	DefaultConfigPath = "/data/config.yaml"
)

// singleton instance
// we avoid, having more than one instance of the config manager because it can lead to race conditions
// if we ensure, that we have only one instance, we can avoid race conditions by using mutexes in this single instance as we do here

// however, access from outside the package is not protected by mutexes (keep in mind e.g. when using GitOps on the config file)
var (
	instance ConfigManager
	once     sync.Once
)

// ConfigManager is the interface for config management
type ConfigManager interface {
	// GetConfig returns the current config
	GetConfig(ctx context.Context, tick uint64) (FullConfig, error)
	// AtomicSetLocation sets the location in the config atomically
	AtomicSetLocation(ctx context.Context, location models.EditInstanceLocationModel) error
	// AtomicAddDataflowcomponent adds a dataflowcomponent to the config atomically
	AtomicAddDataflowcomponent(ctx context.Context, dfc DataFlowComponentConfig) error
	// AtomicDeleteDataflowcomponent deletes a dataflowcomponent from the config atomically
	AtomicDeleteDataflowcomponent(ctx context.Context, componentUUID uuid.UUID) error
	// AtomicEditDataflowcomponent edits a dataflowcomponent in the config atomically
	AtomicEditDataflowcomponent(ctx context.Context, componentUUID uuid.UUID, dfc DataFlowComponentConfig) (DataFlowComponentConfig, error)
}

// FileConfigManager implements the ConfigManager interface by reading from a file
type FileConfigManager struct {
	// configPath is the path to the config file
	configPath string

	// fsService handles filesystem operations
	fsService filesystem.Service

	// logger is the logger for the config manager
	logger *zap.SugaredLogger

	// mutexAtomicUpdate for full cycle read and write access (atomic update) to the config file
	// all writes to the config need to happen under this mutex via a atomic set method -> writeConfig is therefore not exposed
	// the goal is to prevent two read/write cycles ("atomic updates") happening at the same time
	// we use our own implementation of a context aware mutex here to avoid deadlocks
	mutexAtomicUpdate ctxmutex.CtxMutex

	// simple mutex for read access or write access to the config file
	// it will be used by Getconfig and writeConfig
	// this mutex will allow multiple GetConfig calls to happen in parallel
	// it will prevent multiple reads or read/write cycles to happen at the same time
	// we use our own implementation of a context aware mutex here to avoid deadlocks
	mutexReadOrWrite ctxrwmutex.CtxRWMutex

	// ---------- in-memory cache (read-only after RLock) ----------
	cacheMu      sync.RWMutex // guards the two fields below
	cacheModTime time.Time    // mtime of last successfully parsed file
	cacheConfig  FullConfig   // struct obtained from that file
}

// NewFileConfigManager creates a new FileConfigManager
// Note: This should only be used in tests or if you need a custom config manager.
// Prefer NewFileConfigManagerWithBackoff() for application use.
func NewFileConfigManager() *FileConfigManager {

	configPath := DefaultConfigPath
	logger := logger.For(logger.ComponentConfigManager)

	return &FileConfigManager{
		configPath:        configPath,
		fsService:         filesystem.NewDefaultService(),
		logger:            logger,
		mutexAtomicUpdate: *ctxmutex.NewCtxMutex(),
		mutexReadOrWrite:  *ctxrwmutex.NewCtxRWMutex(),
	}
}

// WithFileSystemService allows setting a custom filesystem service
// useful for testing or advanced use cases
func (m *FileConfigManager) WithFileSystemService(fsService filesystem.Service) *FileConfigManager {
	m.fsService = fsService
	return m
}

// get config or create new with given config parameters (communicator, release channel, location)
// if the config file does not exist, it will be created with default values and then overwritten with the given config parameters
func (m *FileConfigManager) GetConfigWithOverwritesOrCreateNew(ctx context.Context, configOverride FullConfig) (FullConfig, error) {
	// Check if context is already cancelled
	if ctx.Err() != nil {
		return FullConfig{}, ctx.Err()
	}

	var config FullConfig
	// default config value
	config.Agent.MetricsPort = 8080

	exists, err := m.fsService.FileExists(ctx, m.configPath)
	switch {
	case err != nil:
		m.logger.Warnf("failed to check if config file exists in %s: %v", m.configPath, err)
	case exists:
		config, err = m.GetConfig(ctx, 0)
		if err != nil {
			return FullConfig{}, fmt.Errorf("failed to get config that exists: %w", err)
		}

	}

	// Apply overrides
	if configOverride.Agent.MetricsPort > 0 {
		config.Agent.MetricsPort = configOverride.Agent.MetricsPort
	}

	if configOverride.Agent.APIURL != "" {
		config.Agent.APIURL = configOverride.Agent.APIURL
	}

	if configOverride.Agent.AuthToken != "" {
		config.Agent.AuthToken = configOverride.Agent.AuthToken
	}

	if configOverride.Agent.ReleaseChannel != "" {
		config.Agent.ReleaseChannel = configOverride.Agent.ReleaseChannel
	}

	if configOverride.Agent.Location != nil {
		location := configOverride.Agent.Location
		if location[0] != "" {
			config.Agent.Location = location
		}
	}

	// Enforce that redpanda has a desired state
	if config.Internal.Redpanda.DesiredFSMState == "" {
		config.Internal.Redpanda.DesiredFSMState = configOverride.Internal.Redpanda.DesiredFSMState
	}

	// Persist the updated config
	if err := m.writeConfig(ctx, config); err != nil {
		return FullConfig{}, fmt.Errorf("failed to write new config: %w", err)
	}

	m.logger.Infof("Successfully wrote config to %s", m.configPath)
	return config, nil
}

// GetConfig returns the current configuration.
//
// The function first takes a shared read lock so multiple callers can run
// concurrently.  It then:
//
//  1. Ensures the directory exists (harmless no-op if it already does).
//  2. Verifies the file exists, preserving the historical “config file
//     does not exist” error semantics expected by callers and tests.
//  3. Calls Stat() — an inexpensive syscall — and compares the file’s
//     ModTime with the timestamp stored in the cache.
//     • If identical, the file is guaranteed unchanged ⇒ return the
//     cached *FullConfig* immediately (no I/O, no YAML decode).
//     • Otherwise fall through to the slow path:
//     a) Read the file with a context deadline.
//     b) Unmarshal YAML into FullConfig (parseConfig).
//     c) Do basic sanity checks.
//     d) Atomically refresh the cache.
//
// Because the cache is keyed on ModTime, every observable write to the
// file (which always updates mtime) causes the next reader to parse fresh
// bytes, so external callers still see a “latest-on-call” behaviour.
func (m *FileConfigManager) GetConfig(ctx context.Context, tick uint64) (FullConfig, error) {
	// we use a read lock here, because we only read the config file
	err := m.mutexReadOrWrite.RLock(ctx)
	if err != nil {
		return FullConfig{}, fmt.Errorf("failed to lock config file: %w", err)
	}
	defer m.mutexReadOrWrite.RUnlock()

	// Check if context is already cancelled
	if ctx.Err() != nil {
		return FullConfig{}, ctx.Err()
	}

	// Create the directory if it doesn't exist
	dir := filepath.Dir(m.configPath)
	if err := m.fsService.EnsureDirectory(ctx, dir); err != nil {
		return FullConfig{}, fmt.Errorf("failed to create config directory: %w", err)
	}

	// Check if context is already cancelled
	if ctx.Err() != nil {
		return FullConfig{}, ctx.Err()
	}

	// QUICK existence check
	exists, err := m.fsService.FileExists(ctx, m.configPath)
	if err != nil {
		return FullConfig{}, fmt.Errorf("failed to check if config file exists in %s: %w", m.configPath, err)
	}
	if !exists {
		return FullConfig{}, fmt.Errorf("config file does not exist: %s", m.configPath)
	}

	// quick stat (µ-seconds, no disk I/O)
	info, err := m.fsService.Stat(ctx, m.configPath)
	switch {
	case err == nil:
		// file exists → continue with fast-/slow-path decision
	case errors.Is(err, os.ErrNotExist):
		return FullConfig{}, fmt.Errorf("config file does not exist: %s", m.configPath)
	default:
		return FullConfig{}, fmt.Errorf("failed to stat config file: %w", err)
	}

	// ---------- FAST PATH ----------
	m.cacheMu.RLock()
	if !m.cacheModTime.IsZero() && info.ModTime().Equal(m.cacheModTime) {
		cfg := m.cacheConfig // return cached struct
		m.cacheMu.RUnlock()
		return cfg, nil
	}
	m.cacheMu.RUnlock()
	// ---------- SLOW PATH (file changed) ----------

	// Read the file
	// Allow half of the timeout for the read operation
	readFileCtx, cancel := context.WithTimeout(ctx, constants.ConfigGetConfigTimeout/2)
	defer cancel()
	data, err := m.fsService.ReadFile(readFileCtx, m.configPath)
	if err != nil {
		return FullConfig{}, fmt.Errorf("failed to read config file: %w", err)
	}
	// This ensures that there is at least half of the timeout left for the parse operation

	// Check if context is already cancelled
	if ctx.Err() != nil {
		return FullConfig{}, ctx.Err()
	}

	config, err := parseConfig(data)
	if err != nil {
		return FullConfig{}, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Check if context is already cancelled
	if ctx.Err() != nil {
		return FullConfig{}, ctx.Err()
	}

	// If the config is empty, return an error
	// Note: sometimes it can happen that due to a filesystem error or maybe in the tests due to docker cp, the file is empty
	// In this case we want to return an error, which is then ignored by the control loop and will retry in the next cycle
	if reflect.DeepEqual(config, FullConfig{}) {
		return FullConfig{}, fmt.Errorf("config file is empty: %s", m.configPath)
	}

	// Validate the location map
	// This ensures downstream code doesn't panic when trying to access the location map
	if config.Agent.Location == nil {
		m.logger.Warnf("config file has no location map: %s", m.configPath)
		config.Agent.Location = make(map[int]string)
	}

	// Validate that the release channel is valid
	// This prevent weird values from being set by the user
	if config.Agent.ReleaseChannel != ReleaseChannelNightly && config.Agent.ReleaseChannel != ReleaseChannelStable && config.Agent.ReleaseChannel != ReleaseChannelEnterprise {
		m.logger.Warnf("config file has invalid release channel: %s", config.Agent.ReleaseChannel)
		config.Agent.ReleaseChannel = "n/a"
	}

	// update cache atomically
	m.cacheMu.Lock()
	m.cacheModTime = info.ModTime()
	m.cacheConfig = config
	m.cacheMu.Unlock()

	return config, nil
}

// parseConfig unmarshals *data* (a YAML document) into a FullConfig.
//
// The YAML decoder is configured with KnownFields(true) so that any
// unknown or misspelled keys cause an immediate error, preventing silent
// misconfiguration.  No additional semantic validation is performed here;
// callers are responsible for deeper checks.
func parseConfig(data []byte) (FullConfig, error) {
	var cfg FullConfig

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true) // ← refuses unknown keys
	if err := dec.Decode(&cfg); err != nil {
		return FullConfig{}, fmt.Errorf("failed to decode config: %w", err)
	}
	return cfg, nil
}

// FileConfigManagerWithBackoff wraps a FileConfigManager and implements backoff for GetConfig errors
type FileConfigManagerWithBackoff struct {
	// The wrapped file config manager
	configManager *FileConfigManager

	// Backoff manager
	backoffManager *backoff.BackoffManager

	// Logger
	logger *zap.SugaredLogger
}

// NewFileConfigManagerWithBackoff creates a new FileConfigManagerWithBackoff with exponential backoff
func NewFileConfigManagerWithBackoff() (*FileConfigManagerWithBackoff, error) {

	if instance != nil {
		return nil, fmt.Errorf("config manager already initialized, only one instance is allowed")

	}

	once.Do(func() {
		configManager := NewFileConfigManager()
		logger := logger.For(logger.ComponentConfigManager)

		// Create backoff manager with default settings
		backoffConfig := backoff.DefaultConfig("ConfigManager", logger)
		backoffConfig.MaxRetries = uint64((time.Minute * 10) / constants.DefaultTickerTime) //10 minutes
		backoffManager := backoff.NewBackoffManager(backoffConfig)

		instance = &FileConfigManagerWithBackoff{
			configManager:  configManager,
			backoffManager: backoffManager,
			logger:         logger,
		}
	})

	return instance.(*FileConfigManagerWithBackoff), nil
}

// GetConfigWithOverwritesOrCreateNew wraps the FileConfigManager's GetConfigWithOverwritesOrCreateNew method
// it is used in main.go to get the config with overwrites or create a new one on startup
func (m *FileConfigManagerWithBackoff) GetConfigWithOverwritesOrCreateNew(ctx context.Context, config FullConfig) (FullConfig, error) {
	return m.configManager.GetConfigWithOverwritesOrCreateNew(ctx, config)
}

// writeConfig writes the config to the file
// it should not be exposed or used outside of the config manager, due to potential race conditions
func (m *FileConfigManager) writeConfig(ctx context.Context, config FullConfig) error {
	// we use a write lock here, because we write the config file
	err := m.mutexReadOrWrite.Lock(ctx)
	if err != nil {
		return fmt.Errorf("failed to lock config file: %w", err)
	}
	defer m.mutexReadOrWrite.Unlock()

	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Create the directory if it doesn't exist
	dir := filepath.Dir(m.configPath)
	if err := m.fsService.EnsureDirectory(ctx, dir); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal the config to YAML
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write the file (give everybody read & write access)
	if err := m.fsService.WriteFile(ctx, m.configPath, data, 0666); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	m.logger.Infof("Successfully wrote config to %s", m.configPath)
	return nil
}

// WithFileSystemService allows setting a custom filesystem service on the wrapped FileConfigManager
// useful for testing or advanced use cases
func (m *FileConfigManagerWithBackoff) WithFileSystemService(fsService filesystem.Service) *FileConfigManagerWithBackoff {
	m.configManager.WithFileSystemService(fsService)
	return m
}

// GetConfig returns the current config with backoff logic for failures
// This is a wrapper around the FileConfigManager's GetConfig method
// It adds backoff logic to handle temporary and permanent failures
// It will return either a temporary backoff error or a permanent failure error
func (m *FileConfigManagerWithBackoff) GetConfig(ctx context.Context, tick uint64) (FullConfig, error) {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		metrics.ObserveReconcileTime(logger.ComponentConfigManager, "get_config", duration)
	}()

	// Check if context is already cancelled
	if ctx.Err() != nil {
		return FullConfig{}, ctx.Err()
	}

	// Check if we should skip operation due to backoff
	if m.backoffManager.ShouldSkipOperation(tick) {
		// Get appropriate backoff error (temporary or permanent)
		backoffErr := m.backoffManager.GetBackoffError(tick)

		// Log additional information for permanent failures
		if m.backoffManager.IsPermanentlyFailed() {
			sentry.ReportIssuef(sentry.IssueTypeError, m.logger, "ConfigManager is permanently failed. Last error: %v", m.backoffManager.GetLastError())
		}

		return FullConfig{}, backoffErr
	}

	// Try to fetch the config
	getConfigCtx, cancel := context.WithTimeout(ctx, constants.ConfigGetConfigTimeout)
	defer cancel()

	config, err := m.configManager.GetConfig(getConfigCtx, tick)
	if err != nil {
		m.backoffManager.SetError(err, tick)
		return FullConfig{}, err
	}

	// Reset backoff state on successful operation
	m.backoffManager.Reset()
	return config, nil
}

// Reset forcefully resets the config manager's state, including permanent failure status
// This should be called when the parent component has taken action to address the failure
func (m *FileConfigManagerWithBackoff) Reset() {
	m.backoffManager.Reset()
}

// IsPermanentFailure returns true if the config manager has permanently failed
// This can be used by consumers to distinguish between temporary and permanent failures
func (m *FileConfigManagerWithBackoff) IsPermanentFailure() bool {
	return m.backoffManager.IsPermanentlyFailed()
}

// GetLastError returns the last error that occurred when fetching the config
func (m *FileConfigManagerWithBackoff) GetLastError() error {
	return m.backoffManager.GetLastError()
}

// AtomicSetLocation sets the location in the config atomically
func (m *FileConfigManager) AtomicSetLocation(ctx context.Context, location models.EditInstanceLocationModel) error {
	err := m.mutexAtomicUpdate.Lock(ctx)
	if err != nil {
		return fmt.Errorf("failed to lock config file: %w", err)
	}
	defer m.mutexAtomicUpdate.Unlock()

	// get the current config
	config, err := m.GetConfig(ctx, 0)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	// Always create a new location map like in the mock implementation
	config.Agent.Location = make(map[int]string)

	// Location is a hierarchical structure represented as map[int]string
	// 0: Enterprise, 1: Site, 2: Area, 3: Line, 4: WorkCell
	config.Agent.Location[0] = location.Enterprise

	// Update optional fields if they exist
	if location.Site != nil {
		config.Agent.Location[1] = *location.Site
	}
	if location.Area != nil {
		config.Agent.Location[2] = *location.Area
	}
	if location.Line != nil {
		config.Agent.Location[3] = *location.Line
	}
	if location.WorkCell != nil {
		config.Agent.Location[4] = *location.WorkCell
	}

	// write the config
	if err := m.writeConfig(ctx, config); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// AtomicSetLocation delegates to the underlying FileConfigManager
func (m *FileConfigManagerWithBackoff) AtomicSetLocation(ctx context.Context, location models.EditInstanceLocationModel) error {
	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return m.configManager.AtomicSetLocation(ctx, location)
}

// AtomicAddDataflowcomponent adds a dataflowcomponent to the config atomically
func (m *FileConfigManager) AtomicAddDataflowcomponent(ctx context.Context, dfc DataFlowComponentConfig) error {
	err := m.mutexAtomicUpdate.Lock(ctx)
	if err != nil {
		return fmt.Errorf("failed to lock config file: %w", err)
	}
	defer m.mutexAtomicUpdate.Unlock()

	// get the current config
	config, err := m.GetConfig(ctx, 0)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	// check for duplicate name before add
	for _, cmp := range config.DataFlow {
		if cmp.Name == dfc.Name {
			return fmt.Errorf("another dataflow component with name %q already exists – choose a unique name", dfc.Name)
		}
	}

	// edit the config
	config.DataFlow = append(config.DataFlow, dfc)

	// write the config
	if err := m.writeConfig(ctx, config); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// AtomicAddDataflowcomponent delegates to the underlying FileConfigManager
func (m *FileConfigManagerWithBackoff) AtomicAddDataflowcomponent(ctx context.Context, dfc DataFlowComponentConfig) error {
	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return m.configManager.AtomicAddDataflowcomponent(ctx, dfc)
}

// AtomicDeleteDataflowcomponent deletes a dataflowcomponent from the config atomically
func (m *FileConfigManager) AtomicDeleteDataflowcomponent(ctx context.Context, componentUUID uuid.UUID) error {
	err := m.mutexAtomicUpdate.Lock(ctx)
	if err != nil {
		return fmt.Errorf("failed to lock config file: %w", err)
	}
	defer m.mutexAtomicUpdate.Unlock()

	// get the current config
	config, err := m.GetConfig(ctx, 0)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	// Find and remove the component with matching UUID
	found := false
	filteredComponents := make([]DataFlowComponentConfig, 0, len(config.DataFlow))

	for _, component := range config.DataFlow {
		componentID := dataflowcomponentserviceconfig.GenerateUUIDFromName(component.Name)
		if componentID != componentUUID {
			filteredComponents = append(filteredComponents, component)
		} else {
			found = true
		}
	}

	if !found {
		return fmt.Errorf("dataflow component with UUID %s not found", componentUUID)
	}

	// Update config with filtered components
	config.DataFlow = filteredComponents

	// write the config
	if err := m.writeConfig(ctx, config); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// AtomicDeleteDataflowcomponent delegates to the underlying FileConfigManager
func (m *FileConfigManagerWithBackoff) AtomicDeleteDataflowcomponent(ctx context.Context, componentUUID uuid.UUID) error {
	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return m.configManager.AtomicDeleteDataflowcomponent(ctx, componentUUID)
}

// AtomicEditDataflowcomponent edits a dataflowcomponent in the config atomically
func (m *FileConfigManager) AtomicEditDataflowcomponent(ctx context.Context, componentUUID uuid.UUID, dfc DataFlowComponentConfig) (DataFlowComponentConfig, error) {
	err := m.mutexAtomicUpdate.Lock(ctx)
	if err != nil {
		return DataFlowComponentConfig{}, fmt.Errorf("failed to lock config file: %w", err)
	}
	defer m.mutexAtomicUpdate.Unlock()

	// get the current config
	config, err := m.GetConfig(ctx, 0)
	if err != nil {
		return DataFlowComponentConfig{}, fmt.Errorf("failed to get config: %w", err)
	}

	oldConfig := DataFlowComponentConfig{}

	// check for duplicate name before edit
	for _, cmp := range config.DataFlow {
		if cmp.Name == dfc.Name && dataflowcomponentserviceconfig.GenerateUUIDFromName(cmp.Name) != componentUUID {
			return DataFlowComponentConfig{}, fmt.Errorf("another dataflow component with name %q already exists – choose a unique name", dfc.Name)
		}
	}

	// Find the component with matching UUID
	found := false
	for i, component := range config.DataFlow {
		componentID := dataflowcomponentserviceconfig.GenerateUUIDFromName(component.Name)
		if componentID == componentUUID {
			// Found the component to edit, update it
			oldConfig = config.DataFlow[i]
			config.DataFlow[i] = dfc
			found = true
			break
		}
	}

	if !found {
		return DataFlowComponentConfig{}, fmt.Errorf("dataflow component with UUID %s not found", componentUUID)
	}

	// write the config
	if err := m.writeConfig(ctx, config); err != nil {
		return DataFlowComponentConfig{}, fmt.Errorf("failed to write config: %w", err)
	}

	return oldConfig, nil
}

// AtomicEditDataflowcomponent delegates to the underlying FileConfigManager
func (m *FileConfigManagerWithBackoff) AtomicEditDataflowcomponent(ctx context.Context, componentUUID uuid.UUID, dfc DataFlowComponentConfig) (DataFlowComponentConfig, error) {
	// Check if context is already cancelled
	if ctx.Err() != nil {
		return DataFlowComponentConfig{}, ctx.Err()
	}

	return m.configManager.AtomicEditDataflowcomponent(ctx, componentUUID, dfc)
}
