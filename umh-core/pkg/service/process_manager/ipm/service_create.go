//go:build internal_process_manager
// +build internal_process_manager

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

package ipm

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/process_manager_serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"go.uber.org/zap"
)

// createService orchestrates the complete creation of a service instance in the process manager.
// This involves validating the service configuration, creating the necessary directory structure,
// and writing all configuration files. The function is designed to be atomic - if any step fails,
// the entire operation is aborted. This ensures that partially created services don't exist in
// the system, which could cause confusion or errors during service management operations.
func (pm *ProcessManager) createService(ctx context.Context, identifier serviceIdentifier, fsService filesystem.Service) error {
	pm.Logger.Info("Creating service", zap.String("identifier", string(identifier)))

	// Validate service exists in our registry
	service, err := pm.getServiceConfig(identifier)
	if err != nil {
		pm.Logger.Error("Failed to get service config", zap.Error(err))
		return err
	}

	// Check if there's an old PID file from a previous service instance
	servicePath := filepath.Join(pm.serviceDirectory, string(identifier))
	pidFile := filepath.Join(servicePath, PidFileName)

	pm.Logger.Info("Checking for old PID file", zap.String("pidFile", pidFile))

	if _, err := fsService.Stat(ctx, pidFile); err == nil {
		pm.Logger.Info("Found old PID file, cleaning up previous service instance", zap.String("identifier", string(identifier)))

		// Clean up the old service instance
		if err := pm.removeService(ctx, identifier, fsService); err != nil {
			pm.Logger.Error("Error cleaning up old service instance", zap.Error(err))
			return fmt.Errorf("error cleaning up old service instance: %w", err)
		}

		// Return an error to allow retry in the next step after cleanup
		return fmt.Errorf("cleaned up old service instance, retry creation in next step")
	}

	pm.Logger.Info("Creating service directories", zap.String("servicePath", servicePath))

	// Create necessary directory structure
	if err := pm.createServiceDirectories(ctx, identifier, fsService); err != nil {
		pm.Logger.Error("Failed to create service directories", zap.Error(err))
		return err
	}

	pm.Logger.Info("Writing service config files")

	// Write all configuration files
	if err := pm.writeServiceConfigFiles(ctx, identifier, service.config, fsService); err != nil {
		pm.Logger.Error("Failed to write service config files", zap.Error(err))
		return err
	}

	pm.Logger.Info("Service created successfully", zap.String("identifier", string(identifier)))
	return nil
}

// getServiceConfig retrieves the service configuration from the internal registry.
// This function acts as a safety check to ensure that we only attempt to create services
// that have been properly registered in the ProcessManager. While this should never fail
// under normal circumstances (since we only add identifiers to queues for existing services),
// it provides a defensive programming measure against race conditions or programming errors.
func (pm *ProcessManager) getServiceConfig(identifier serviceIdentifier) (service, error) {
	service, exists := pm.services[identifier]
	if !exists {
		// This should never happen as we only add services to the list that exist
		return service, fmt.Errorf("service %s not found", identifier)
	}
	return service, nil
}

// createServiceDirectories creates the required directory structure for a service.
// Each service needs dedicated directories for logs and configuration files to maintain
// proper isolation and organization. The log directory will store service output and error logs,
// while the config directory will contain all service-specific configuration files.
// This separation ensures that service data is organized and accessible for debugging and monitoring.
func (pm *ProcessManager) createServiceDirectories(ctx context.Context, identifier serviceIdentifier, fsService filesystem.Service) error {
	servicePath := filepath.Join(pm.serviceDirectory, string(identifier))

	directories := []string{
		filepath.Join(servicePath, LogDirectoryName),
		filepath.Join(servicePath, ConfigDirectoryName),
	}

	for _, dir := range directories {
		if err := fsService.EnsureDirectory(ctx, dir); err != nil {
			return fmt.Errorf("error creating directory %s: %w", dir, err)
		}
	}

	return nil
}

// writeServiceConfigFiles writes all configuration files for a service to the filesystem.
// Configuration files are essential for service operation as they contain settings, parameters,
// and other data that the service needs to function correctly. By writing these files during
// service creation, we ensure that when the service process starts, it has access to all
// necessary configuration data. Each file is written with appropriate permissions to maintain
// security while allowing the service to read its configuration.
func (pm *ProcessManager) writeServiceConfigFiles(ctx context.Context, identifier serviceIdentifier, config process_manager_serviceconfig.ProcessManagerServiceConfig, fsService filesystem.Service) error {
	configDirectory := filepath.Join(pm.serviceDirectory, string(identifier), ConfigDirectoryName)

	// First, write all user-provided configuration files
	for configFileName, configFileContent := range config.ConfigFiles {
		configFilePath := filepath.Join(configDirectory, configFileName)
		if err := fsService.WriteFile(ctx, configFilePath, []byte(configFileContent), ConfigFilePermission); err != nil {
			return fmt.Errorf("error writing config file %s: %w", configFileName, err)
		}
	}

	// Generate and write the run.sh script from the Command field
	if err := pm.generateRunScript(ctx, identifier, config, fsService); err != nil {
		return fmt.Errorf("error generating run script: %w", err)
	}

	return nil
}

// generateRunScript creates a run.sh script from the ProcessManagerServiceConfig.Command field.
// This script serves as the entry point for the service process and handles command execution
// with proper argument passing and environment variable setup.
func (pm *ProcessManager) generateRunScript(ctx context.Context, identifier serviceIdentifier, config process_manager_serviceconfig.ProcessManagerServiceConfig, fsService filesystem.Service) error {
	configDirectory := filepath.Join(pm.serviceDirectory, string(identifier), ConfigDirectoryName)
	runScriptPath := filepath.Join(configDirectory, RunScriptFileName)

	// Build the shell script content
	var scriptBuilder strings.Builder
	scriptBuilder.WriteString("#!/bin/bash\n")
	scriptBuilder.WriteString("# Auto-generated run script for service\n")
	scriptBuilder.WriteString("set -e\n\n")

	// Add environment variables if any
	if len(config.Env) > 0 {
		scriptBuilder.WriteString("# Set environment variables\n")
		for key, value := range config.Env {
			// Simple shell escaping - wrap values in single quotes and escape any single quotes
			escapedValue := strings.ReplaceAll(value, "'", "'\"'\"'")
			scriptBuilder.WriteString(fmt.Sprintf("export %s='%s'\n", key, escapedValue))
		}
		scriptBuilder.WriteString("\n")
	}

	// Add the command execution
	if len(config.Command) == 0 {
		return fmt.Errorf("no command specified in service configuration")
	}

	scriptBuilder.WriteString("# Execute the service command\n")
	scriptBuilder.WriteString("exec")

	// Add each command argument with proper shell escaping
	for _, arg := range config.Command {
		// Remove /run/service/... from the argument (everything until /config/)
		configIndex := strings.Index(arg, "/config/")
		if configIndex != -1 {
			arg = arg[configIndex:]
			arg = filepath.Join(pm.serviceDirectory, string(identifier), arg)
		}
		// Simple shell escaping - wrap arguments in single quotes and escape any single quotes
		escapedArg := strings.ReplaceAll(arg, "'", "'\"'\"'")
		scriptBuilder.WriteString(fmt.Sprintf(" '%s'", escapedArg))
	}
	scriptBuilder.WriteString("\n")

	// Write the script file with executable permissions
	scriptContent := scriptBuilder.String()
	if err := fsService.WriteFile(ctx, runScriptPath, []byte(scriptContent), ScriptFilePermission); err != nil {
		return fmt.Errorf("error writing run script: %w", err)
	}

	pm.Logger.Info("Generated run script for service",
		zap.String("identifier", string(identifier)),
		zap.String("scriptPath", runScriptPath),
		zap.Strings("command", config.Command))

	return nil
}
