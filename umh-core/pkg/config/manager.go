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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
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
	// GetFileSystemService returns the filesystem service
	GetFileSystemService() filesystem.Service
	// AtomicSetLocation sets the location in the config atomically
	AtomicSetLocation(ctx context.Context, location models.EditInstanceLocationModel) error
	// AtomicAddDataflowcomponent adds a dataflowcomponent to the config atomically
	AtomicAddDataflowcomponent(ctx context.Context, dfc DataFlowComponentConfig) error
	// AtomicDeleteDataflowcomponent deletes a dataflowcomponent from the config atomically
	AtomicDeleteDataflowcomponent(ctx context.Context, componentUUID uuid.UUID) error
	// AtomicEditDataflowcomponent edits a dataflowcomponent in the config atomically
	AtomicEditDataflowcomponent(ctx context.Context, componentUUID uuid.UUID, dfc DataFlowComponentConfig) (DataFlowComponentConfig, error)
	// AtomicAddProtocolConverter adds a protocol converter to the config atomically
	AtomicAddProtocolConverter(ctx context.Context, pc ProtocolConverterConfig) error
	// AtomicEditProtocolConverter edits a protocol converter in the config atomically
	AtomicEditProtocolConverter(ctx context.Context, componentUUID uuid.UUID, pc ProtocolConverterConfig) (ProtocolConverterConfig, error)
	// GetConfigAsString returns the current config as a string
	// This function is used in the get-config-file action to retrieve the raw config file
	// without any yaml parsing applied. This allows to display yaml anchors and change them
	// via the frontend
	GetConfigAsString(ctx context.Context) (string, error)
	// GetCacheModTime returns the modification time of the config file
	GetCacheModTimeWithoutUpdate() time.Time
	// UpdateAndGetCacheModTime updates the cache and returns the modification time
	UpdateAndGetCacheModTime(ctx context.Context) (time.Time, error)
	// WriteConfigFromString writes a config from a string to the config file
	WriteConfigFromString(ctx context.Context, configStr string, expectedModTime string) error

	// TODO: Add AtomicUnlinkFromTemplate method
	// AtomicUnlinkFromTemplate converts a templated configuration (using YAML anchors/aliases)
	// to an inline template configuration, making it UI-editable while preserving all
	// current functionality. This addresses the UX gap where users hit "please edit the file manually"
	// errors when trying to customize templated configurations.
	// AtomicUnlinkFromTemplate(ctx context.Context, componentUUID uuid.UUID) error


	// TODO: Add AtomicUnlinkFromTemplate method
	// AtomicUnlinkFromTemplate converts a templated configuration (using YAML anchors/aliases)
	// to an inline template configuration, making it UI-editable while preserving all
	// current functionality. This addresses the UX gap where users hit "please edit the file manually"
	// errors when trying to customize templated configurations.
	// AtomicUnlinkFromTemplate(ctx context.Context, componentUUID uuid.UUID) error

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
	cacheMu        sync.RWMutex // guards the two fields below
	cacheModTime   time.Time    // mtime of last successfully parsed file
	cacheConfig    FullConfig   // struct obtained from that file
	cacheRawConfig string
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

// GetFileSystemService returns the filesystem service
func (m *FileConfigManager) GetFileSystemService() filesystem.Service {
	return m.fsService
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
//  2. Verifies the file exists, preserving the historical "config file
//     does not exist" error semantics expected by callers and tests.
//  3. Calls Stat() — an inexpensive syscall — and compares the file's
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
// bytes, so external callers still see a "latest-on-call" behaviour.
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

	config, err := parseConfig(data, false)
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

	// update all cache fields atomically in a single critical section
	m.cacheMu.Lock()
	m.cacheRawConfig = string(data)
	m.cacheModTime = info.ModTime()
	m.cacheConfig = config
	m.cacheMu.Unlock()

	return config, nil
}

// parseConfig unmarshals *data* (a YAML document) into a FullConfig.
//
// The YAML decoder is configured with KnownFields(true) by default so that any
// unknown or misspelled keys cause an immediate error, preventing silent
// misconfiguration. Setting allowUnknownFields to true allows YAML anchors and other
// custom fields to pass validation. No additional semantic validation is performed here;
// callers are responsible for deeper checks.
func parseConfig(data []byte, allowUnknownFields bool) (FullConfig, error) {
	var cfg FullConfig

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(!allowUnknownFields) // Only reject unknown keys if allowUnknownFields is false
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

// GetFileSystemService returns the filesystem service
func (m *FileConfigManagerWithBackoff) GetFileSystemService() filesystem.Service {
	return m.configManager.GetFileSystemService()
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

	// Use custom YAML marshaling if we have templates that need anchors
	var data []byte
	if len(config.Templates) > 0 {
		data, err = marshalConfigWithAnchors(config)
	} else {
		data, err = yaml.Marshal(config)
	}
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write the file (give everybody read & write access)
	if err := m.fsService.WriteFile(ctx, m.configPath, data, 0666); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	// Update the cache to reflect the new config
	info, err := m.fsService.Stat(ctx, m.configPath)
	if err != nil {
		return fmt.Errorf("failed to stat config file after write: %w", err)
	}

	// Update all cache fields atomically in a single critical section
	m.cacheMu.Lock()
	m.cacheRawConfig = string(data)
	m.cacheModTime = info.ModTime()
	m.cacheConfig = config
	m.cacheMu.Unlock()

	m.logger.Infof("Successfully wrote config to %s", m.configPath)
	return nil
}

// marshalConfigWithAnchors handles custom YAML marshaling to create anchors for templates
func marshalConfigWithAnchors(config FullConfig) ([]byte, error) {
	// Use yaml.Node to create proper anchors and aliases
	var rootNode yaml.Node

	// First, marshal the config to get the basic structure
	if err := rootNode.Encode(config); err != nil {
		return nil, fmt.Errorf("failed to encode config to yaml.Node: %w", err)
	}

	// Process the node tree to create anchors and aliases
	if err := processNodeForAnchors(&rootNode, config); err != nil {
		return nil, fmt.Errorf("failed to process nodes for anchors: %w", err)
	}

	// Marshal the processed node tree to YAML
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(&rootNode); err != nil {
		return nil, fmt.Errorf("failed to encode processed node: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("failed to close yaml encoder: %w", err)
	}

	return buf.Bytes(), nil
}

// processNodeForAnchors processes the yaml.Node tree to create anchors for templates
func processNodeForAnchors(node *yaml.Node, config FullConfig) error {
	if node == nil {
		return nil
	}

	// Find the templates section and add anchors
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		return processNodeForAnchors(node.Content[0], config)
	}

	if node.Kind == yaml.MappingNode {
		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valueNode := node.Content[i+1]

			// Process templates section
			if keyNode.Value == "templates" && valueNode.Kind == yaml.SequenceNode {
				if err := processTemplatesNode(valueNode, config); err != nil {
					return err
				}
			}

			// Process protocol converters section to create aliases
			if keyNode.Value == "protocolConverter" && valueNode.Kind == yaml.SequenceNode {
				if err := processProtocolConvertersNode(valueNode, config); err != nil {
					return err
				}
			}

			// Recursively process child nodes
			if err := processNodeForAnchors(valueNode, config); err != nil {
				return err
			}
		}
	}

	if node.Kind == yaml.SequenceNode {
		for _, child := range node.Content {
			if err := processNodeForAnchors(child, config); err != nil {
				return err
			}
		}
	}

	return nil
}

// processTemplatesNode adds anchors to template definitions
func processTemplatesNode(templatesNode *yaml.Node, config FullConfig) error {
	for _, templateNode := range templatesNode.Content {
		if templateNode.Kind == yaml.MappingNode {
			// Find template items and add anchors
			for i := 0; i < len(templateNode.Content); i += 2 {
				keyNode := templateNode.Content[i]
				valueNode := templateNode.Content[i+1]

				// Skip metadata fields
				if keyNode.Value == "_anchor" {
					continue
				}

				// Add anchor to the template value node
				if valueNode.Anchor == "" {
					valueNode.Anchor = keyNode.Value // Use the template name as anchor
				}
			}
		}
	}
	return nil
}

// processProtocolConvertersNode creates aliases for protocol converters that reference templates
func processProtocolConvertersNode(pcNode *yaml.Node, config FullConfig) error {
	for _, converterNode := range pcNode.Content {
		if converterNode.Kind == yaml.MappingNode {
			// Look for protocol converters that have template references
			for i := 0; i < len(converterNode.Content); i += 2 {
				keyNode := converterNode.Content[i]
				valueNode := converterNode.Content[i+1]

				if keyNode.Value == "protocolConverterServiceConfig" && valueNode.Kind == yaml.MappingNode {
					if err := processProtocolConverterServiceConfig(valueNode); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// processProtocolConverterServiceConfig handles template references in protocol converter service config
func processProtocolConverterServiceConfig(serviceConfigNode *yaml.Node) error {
	var templateRefName string

	// First pass: find the template reference
	for i := 0; i < len(serviceConfigNode.Content); i += 2 {
		keyNode := serviceConfigNode.Content[i]
		valueNode := serviceConfigNode.Content[i+1]

		if keyNode.Value == "location" && valueNode.Kind == yaml.MappingNode {
			for j := 0; j < len(valueNode.Content); j += 2 {
				locationKeyNode := valueNode.Content[j]
				locationValueNode := valueNode.Content[j+1]

				if locationKeyNode.Value == "_templateRef" {
					templateRefName = locationValueNode.Value
					// Remove the metadata field
					valueNode.Content = append(valueNode.Content[:j], valueNode.Content[j+2:]...)
					break
				}
			}
		}
	}

	// Second pass: replace template with alias if we found a reference
	if templateRefName != "" {
		for i := 0; i < len(serviceConfigNode.Content); i += 2 {
			keyNode := serviceConfigNode.Content[i]

			if keyNode.Value == "template" {
				// Replace with alias node
				aliasNode := &yaml.Node{
					Kind:  yaml.AliasNode,
					Value: templateRefName,
				}
				serviceConfigNode.Content[i+1] = aliasNode
				break
			}
		}
	}

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

	// ------------------------------------------------------------------
	// Guard against overwriting a DFC that still relies on YAML
	// templating (anchors/aliases)
	//
	// Background
	// ----------
	// Operators may define *dataFlowComponentConfig* via an anchor:
	//
	//     templates:
	//       - &baseCfg { … }
	//     dataFlow:
	//       - name: dfc-1
	//         dataFlowComponentConfig: *baseCfg   # ← alias
	//
	// Policy
	// ------
	// If the component we are about to **edit** still hasAnchors == true
	// we MUST refuse to touch it; otherwise we would flatten or delete
	// the user's template when we rewrite the file.
	for _, c := range config.DataFlow {
		if dataflowcomponentserviceconfig.GenerateUUIDFromName(c.Name) == componentUUID {
			if c.HasAnchors() {
				return DataFlowComponentConfig{}, fmt.Errorf(
					"dataFlowComponentConfig for %s is defined via YAML anchors/aliases; "+
						"please edit the file manually or see https://docs.umh.app/reference/configuration-reference for more details", componentUUID)
			}
			break
		}
	}
	// End of guard

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

// AtomicAddProtocolConverter adds a protocol converter to the config atomically
// Creates an anchored template in the templates section and references it from the protocol converter
func (m *FileConfigManager) AtomicAddProtocolConverter(ctx context.Context, pc ProtocolConverterConfig) error {
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
	for _, cmp := range config.ProtocolConverter {
		if cmp.Name == pc.Name {
			return fmt.Errorf("another protocol converter with name %q already exists – choose a unique name", pc.Name)
		}
	}

	// Generate template name from protocol converter name
	templateName := generateTemplateAnchorName(pc.Name)

	// Check if template name already exists
	if templateExists(config.Templates, templateName) {
		return fmt.Errorf("template with anchor name %q already exists – choose a unique protocol converter name", templateName)
	}

	// Create the anchored template and add it to the templates section
	templateContent := createTemplateContent(pc.ProtocolConverterServiceConfig.Template)

	// Add template to config.Templates with anchor metadata
	templateWithAnchor := map[string]interface{}{
		templateName: templateContent,
		"_anchor":    templateName, // Metadata to indicate this should be an anchor
	}
	config.Templates = append(config.Templates, templateWithAnchor)

	// Add metadata to indicate this should reference a template
	// We'll use a special field or modify the YAML generation
	if pc.ProtocolConverterServiceConfig.Location == nil {
		pc.ProtocolConverterServiceConfig.Location = make(map[string]string)
	}
	pc.ProtocolConverterServiceConfig.Location["_templateRef"] = templateName

	// Add the protocol converter
	config.ProtocolConverter = append(config.ProtocolConverter, pc)

	// write the config
	if err := m.writeConfig(ctx, config); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// generateTemplateAnchorName creates a valid YAML anchor name from a protocol converter name
func generateTemplateAnchorName(pcName string) string {
	// Replace non-alphanumeric characters with underscores and add template suffix
	// YAML anchors must contain only alphanumeric characters
	result := ""
	for _, r := range pcName {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			result += string(r)
		} else {
			result += "_"
		}
	}
	return result + "_template"
}

// templateExists checks if a template with the given anchor name already exists
func templateExists(templates []map[string]interface{}, anchorName string) bool {
	for _, template := range templates {
		if _, exists := template[anchorName]; exists {
			return true
		}
	}
	return false
}

// createTemplateContent converts a template config to a map for YAML anchoring
func createTemplateContent(template protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate) map[string]interface{} {
	// Convert the template struct to a map that can be used in YAML templates section
	templateMap := make(map[string]interface{})

	// Add connection config using the proper struct - let YAML marshaler handle the tags
	if template.ConnectionServiceConfig.NmapTemplate != nil {
		templateMap["connection"] = template.ConnectionServiceConfig
	}

	// Add dataflow component configs (currently empty as per deploy action)
	// These will be populated later via edit actions
	if !isEmptyDataflowComponentConfig(template.DataflowComponentReadServiceConfig) {
		templateMap["dataflowcomponent_read"] = template.DataflowComponentReadServiceConfig
	}

	if !isEmptyDataflowComponentConfig(template.DataflowComponentWriteServiceConfig) {
		templateMap["dataflowcomponent_write"] = template.DataflowComponentWriteServiceConfig
	}

	return templateMap
}

// isEmptyDataflowComponentConfig checks if a dataflow component config is empty
func isEmptyDataflowComponentConfig(config dataflowcomponentserviceconfig.DataflowComponentServiceConfig) bool {
	// For now, assume it's empty if BenthosConfig is nil or empty
	return config.BenthosConfig.Input == nil && config.BenthosConfig.Output == nil
}

// AtomicAddProtocolConverter delegates to the underlying FileConfigManager
func (m *FileConfigManagerWithBackoff) AtomicAddProtocolConverter(ctx context.Context, pc ProtocolConverterConfig) error {

	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return m.configManager.AtomicAddProtocolConverter(ctx, pc)
}

// AtomicEditProtocolConverter edits a protocol converter in the config atomically
func (m *FileConfigManager) AtomicEditProtocolConverter(ctx context.Context, componentUUID uuid.UUID, pc ProtocolConverterConfig) (ProtocolConverterConfig, error) {
	err := m.mutexAtomicUpdate.Lock(ctx)
	if err != nil {
		return ProtocolConverterConfig{}, fmt.Errorf("failed to lock config file: %w", err)
	}
	defer m.mutexAtomicUpdate.Unlock()

	// get the current config
	config, err := m.GetConfig(ctx, 0)
	if err != nil {
		return ProtocolConverterConfig{}, fmt.Errorf("failed to get config: %w", err)
	}

	oldConfig := ProtocolConverterConfig{}

	// check for duplicate name before edit
	for _, cmp := range config.ProtocolConverter {
		if cmp.Name == pc.Name && dataflowcomponentserviceconfig.GenerateUUIDFromName(cmp.Name) != componentUUID {
			return ProtocolConverterConfig{}, fmt.Errorf("another protocol converter with name %q already exists – choose a unique name", pc.Name)
		}
	}

	// ------------------------------------------------------------------
	// Guard against overwriting a DFC that still relies on YAML
	// templating (anchors/aliases)
	//
	// Background
	// ----------
	// Operators may define *dataFlowComponentConfig* via an anchor:
	//
	//     templates:
	//       - &baseCfg { … }
	//     dataFlow:
	//       - name: dfc-1
	//         dataFlowComponentConfig: *baseCfg   # ← alias
	//
	// Policy
	// ------
	// If the component we are about to **edit** still hasAnchors == true
	// we MUST refuse to touch it; otherwise we would flatten or delete
	// the user's template when we rewrite the file.
	for _, c := range config.ProtocolConverter {
		if dataflowcomponentserviceconfig.GenerateUUIDFromName(c.Name) == componentUUID {
			if c.HasAnchors() {
				return ProtocolConverterConfig{}, fmt.Errorf(
					"dataFlowComponentConfig for %s is defined via YAML anchors/aliases; "+
						"please edit the file manually", componentUUID)
			}
			break
		}
	}
	// End of guard

	// Find the component with matching UUID
	found := false
	for i, component := range config.ProtocolConverter {
		componentID := dataflowcomponentserviceconfig.GenerateUUIDFromName(component.Name)
		if componentID == componentUUID {
			// Found the component to edit, update it
			oldConfig = config.ProtocolConverter[i]
			config.ProtocolConverter[i] = pc
			found = true
			break
		}
	}

	if !found {
		return ProtocolConverterConfig{}, fmt.Errorf("dataflow component with UUID %s not found", componentUUID)
	}

	// write the config
	if err := m.writeConfig(ctx, config); err != nil {
		return ProtocolConverterConfig{}, fmt.Errorf("failed to write config: %w", err)
	}

	return oldConfig, nil
}

// GetConfigAsString returns the current config file contents as a string
// This function is used in the get-config-file action to retrieve the raw config file
// without any yaml parsing applied. This allows to display yaml anchors and change them
// via the frontend
func (m *FileConfigManager) GetConfigAsString(ctx context.Context) (string, error) {
	// in the GetConfig method, we already read the file and cached the raw config to m.cacheRawConfig
	_, err := m.GetConfig(ctx, 0)
	if err != nil {
		return "", fmt.Errorf("failed to get config: %w", err)
	}

	m.cacheMu.RLock()
	rawConfig := m.cacheRawConfig
	m.cacheMu.RUnlock()

	return rawConfig, nil
}

// GetConfigAsString returns the current config as a string with backoff logic for failures
func (m *FileConfigManagerWithBackoff) GetConfigAsString(ctx context.Context) (string, error) {
	// in the GetConfig method, we already read the file and cached the raw config to m.cacheRawConfig
	_, err := m.GetConfig(ctx, 0)
	if err != nil {
		return "", fmt.Errorf("failed to get config: %w", err)
	}

	m.configManager.cacheMu.RLock()
	rawConfig := m.configManager.cacheRawConfig
	m.configManager.cacheMu.RUnlock()

	return rawConfig, nil
}

// GetCacheModTimeWithoutUpdate returns the modification time without updating the cache
func (m *FileConfigManager) GetCacheModTimeWithoutUpdate() time.Time {
	m.cacheMu.RLock()
	modTime := m.cacheModTime
	m.cacheMu.RUnlock()
	return modTime
}

// UpdateAndGetCacheModTime updates the cache and returns the modification time
func (m *FileConfigManager) UpdateAndGetCacheModTime(ctx context.Context) (time.Time, error) {
	// read config to update the cache mod time
	_, err := m.GetConfig(ctx, 0)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get config: %w", err)
	}

	return m.GetCacheModTimeWithoutUpdate(), nil
}

// GetCacheModTimeWithoutUpdate delegates to the underlying FileConfigManager
func (m *FileConfigManagerWithBackoff) GetCacheModTimeWithoutUpdate() time.Time {
	return m.configManager.GetCacheModTimeWithoutUpdate()
}

// UpdateAndGetCacheModTime delegates to the underlying FileConfigManager
func (m *FileConfigManagerWithBackoff) UpdateAndGetCacheModTime(ctx context.Context) (time.Time, error) {
	return m.configManager.UpdateAndGetCacheModTime(ctx)
}

// WriteConfigFromString writes a config from a string to the config file
// If expectedModTime is provided, it will check that the file hasn't been modified since that time
func (m *FileConfigManager) WriteConfigFromString(ctx context.Context, configStr string, expectedModTime string) error {
	// First parse the config with strict validation to detect syntax errors and schema problems
	_, err := parseConfig([]byte(configStr), false)
	if err != nil {
		// If strict parsing fails, try again with allowUnknownFields=true
		// This allows YAML anchors and other custom fields
		_, err = parseConfig([]byte(configStr), true)
		if err != nil {
			return fmt.Errorf("failed to parse config: %w", err)
		}
	}

	// We use a write lock here because we write the config file
	err = m.mutexReadOrWrite.Lock(ctx)
	if err != nil {
		return fmt.Errorf("failed to lock config file: %w", err)
	}
	defer m.mutexReadOrWrite.Unlock()

	if expectedModTime != "" {
		info, err := m.fsService.Stat(ctx, m.configPath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to stat config file: %w", err)
		}

		// If file exists, check modification time
		if err == nil && info.ModTime().Format(time.RFC3339) != expectedModTime {
			return fmt.Errorf("concurrent modification detected: file modified at %v, expected %v",
				info.ModTime().Format(time.RFC3339), expectedModTime)
		}
	}

	// Create the directory if it doesn't exist
	dir := filepath.Dir(m.configPath)
	if err := m.fsService.EnsureDirectory(ctx, dir); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write the raw string directly to file to preserve all YAML features
	if err := m.fsService.WriteFile(ctx, m.configPath, []byte(configStr), 0666); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	// Update the cache to reflect the new config
	info, err := m.fsService.Stat(ctx, m.configPath)
	if err != nil {
		return fmt.Errorf("failed to stat config file after write: %w", err)
	}

	// Parse the config for the cache
	parsedConfig, err := parseConfig([]byte(configStr), true)
	if err != nil {
		return fmt.Errorf("failed to parse config for cache update: %w", err)
	}

	// Update all cache fields atomically in a single critical section
	m.cacheMu.Lock()
	m.cacheRawConfig = configStr
	m.cacheModTime = info.ModTime()
	m.cacheConfig = parsedConfig
	m.cacheMu.Unlock()

	m.logger.Infof("Successfully wrote config to %s", m.configPath)
	return nil
}

// WriteConfigFromString delegates to the underlying FileConfigManager
func (m *FileConfigManagerWithBackoff) WriteConfigFromString(ctx context.Context, configStr string, expectedModTime string) error {
	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return m.configManager.WriteConfigFromString(ctx, configStr, expectedModTime)

}

// AtomicEditProtocolConverter delegates to the underlying FileConfigManager
func (m *FileConfigManagerWithBackoff) AtomicEditProtocolConverter(ctx context.Context, componentUUID uuid.UUID, pc ProtocolConverterConfig) (ProtocolConverterConfig, error) {
	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ProtocolConverterConfig{}, ctx.Err()
	}

	return m.configManager.AtomicEditProtocolConverter(ctx, componentUUID, pc)
	// If expectedModTime is provided, check for concurrent modification atomically under the lock

}
