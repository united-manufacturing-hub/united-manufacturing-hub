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
	"context"
	"fmt"
	"path/filepath"
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/backoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	filesystem "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

const (
	// DefaultConfigPath is the default path to the config file
	DefaultConfigPath = "/data/config.yaml"
)

// ConfigManager is the interface for config management
type ConfigManager interface {
	// GetConfig returns the current config
	GetConfig(ctx context.Context, tick uint64) (FullConfig, error)
}

// FileConfigManager implements the ConfigManager interface by reading from a file
type FileConfigManager struct {
	// configPath is the path to the config file
	configPath string

	// fsService handles filesystem operations
	fsService filesystem.Service

	// logger is the logger for the config manager
	logger *zap.SugaredLogger
}

// NewFileConfigManager creates a new FileConfigManager
func NewFileConfigManager() *FileConfigManager {

	configPath := DefaultConfigPath
	logger := logger.For(logger.ComponentConfigManager)

	return &FileConfigManager{
		configPath: configPath,
		fsService:  filesystem.NewDefaultService(),
		logger:     logger,
	}
}

// WithFileSystemService allows setting a custom filesystem service
// useful for testing or advanced use cases
func (m *FileConfigManager) WithFileSystemService(fsService filesystem.Service) *FileConfigManager {
	m.fsService = fsService
	return m
}

// GetConfig returns the current config, always reading fresh from disk
func (m *FileConfigManager) GetConfig(ctx context.Context, tick uint64) (FullConfig, error) {
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

	// Check if the file exists
	exists, err := m.fsService.FileExists(ctx, m.configPath)
	if err != nil {
		return FullConfig{}, err
	}

	// Return empty config if the file doesn't exist
	if !exists {
		return FullConfig{}, fmt.Errorf("config file does not exist: %s", m.configPath)
	}

	// Check if context is already cancelled
	if ctx.Err() != nil {
		return FullConfig{}, ctx.Err()
	}

	// Read the file
	data, err := m.fsService.ReadFile(ctx, m.configPath)
	if err != nil {
		return FullConfig{}, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse the YAML
	var config FullConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return FullConfig{}, fmt.Errorf("failed to parse config file: %w", err)
	}

	return config, nil
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
func NewFileConfigManagerWithBackoff() *FileConfigManagerWithBackoff {
	configManager := NewFileConfigManager()
	logger := logger.For(logger.ComponentConfigManager)

	// Create backoff manager with default settings
	backoffConfig := backoff.DefaultConfig("ConfigManager", logger)
	backoffManager := backoff.NewBackoffManager(backoffConfig)

	return &FileConfigManagerWithBackoff{
		configManager:  configManager,
		backoffManager: backoffManager,
		logger:         logger,
	}
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
