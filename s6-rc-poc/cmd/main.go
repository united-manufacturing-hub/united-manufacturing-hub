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

// Package main is a simple implementation using s6-rc to
// demonstrate its reliability, scalability, ease of use and maintenance.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"s6-rc-poc/cmd/configwatcher"
	"s6-rc-poc/cmd/manager"
	"s6-rc-poc/cmd/shared"
	"time"

	"go.uber.org/zap"
)

const (
	s6InitializationDelay = 2 * time.Second
	commandTimeout        = 10 * time.Second
	svscanDirPerm         = 0o750
	sigtermFilePerm       = 0o700
)

var (
	errS6SvscanNotStarted = errors.New("s6-svscan process not started")
)

func main() {
	logger, _ := zap.NewDevelopment()

	// Initialize s6 and s6-rc
	if err := initializeS6(logger); err != nil {
		logger.Fatal("Failed to initialize s6", zap.Error(err))
	}

	watcher := configwatcher.NewFileWatcher(logger)
	svc := manager.NewS6RCService(logger, "", "", "")

	handleConfigEvents(logger, watcher, svc, "/config.yaml")
}

func initializeS6(logger *zap.Logger) error {
	logger.Info("Initializing s6 and s6-rc")

	// Create basic .s6-svscan directory structure
	svscanDir := "/run/service/.s6-svscan"

	if err := os.MkdirAll(svscanDir, svscanDirPerm); err != nil {
		return fmt.Errorf("create s6-svscan dir: %w", err)
	}

	// Create SIGTERM handler for clean shutdown
	sigterm := filepath.Join(svscanDir, "SIGTERM")

	if err := os.WriteFile(sigterm, []byte("#!/bin/sh\ns6-svscanctl -t /run/service\n"), sigtermFilePerm); err != nil {
		return fmt.Errorf("create SIGTERM handler: %w", err)
	}

	// Start s6-svscan in background
	logger.Info("Starting s6-svscan")

	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "s6-svscan", "/run/service")

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start s6-svscan: %w", err)
	}

	// Wait for s6-svscan to be ready
	logger.Info("Waiting for s6-svscan to be ready")
	time.Sleep(s6InitializationDelay)

	// Check if s6-svscan is running
	if cmd.Process == nil {
		return fmt.Errorf("%w", errS6SvscanNotStarted)
	}

	// Initialize s6-rc database
	logger.Info("Compiling s6-rc database")

	if err := runCommand("s6-rc-compile", "/run/s6-rc-compiled", "/etc/s6-rc/source"); err != nil {
		return fmt.Errorf("compile s6-rc database: %w", err)
	}

	logger.Info("Initializing s6-rc live database")

	if err := runCommand("s6-rc-init", "-c", "/run/s6-rc-compiled", "-l", "/run/s6-rc", "/run/service"); err != nil {
		return fmt.Errorf("initialize s6-rc: %w", err)
	}

	logger.Info("s6 and s6-rc initialization complete")

	return nil
}

func runCommand(name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command %s failed: %w", name, err)
	}

	return nil
}

func logError(logger *zap.Logger, action, service string, err error) {
	if logger == nil {
		return
	}

	logger.Error(
		"manager call failed",
		zap.String("action", action),
		zap.String("service", service),
		zap.Error(err),
	)
}

func handleConfigEvents(
	logger *zap.Logger,
	watcher configwatcher.ConfigWatcher,
	svc manager.Service,
	configPath string,
) {
	// Track desired state so ConfigChanged can reuse it without race/goroutines
	desiredStateByService := map[string]shared.State{}

	if err := watcher.Start(configPath); err != nil {
		panic(err)
	}

	for {
		event := <-watcher.Events()

		switch configEvent := event.(type) {
		case configwatcher.EventCreated:
			if logger != nil {
				logger.Info("event: created",
					zap.String("service", configEvent.Name.String()),
					zap.String("desired_state", configEvent.DesiredState.String()),
					zap.String("executable", configEvent.Executable),
					zap.Any("parameters", configEvent.Parameters))
			}

			desiredStateByService[configEvent.Name.String()] = configEvent.DesiredState

			if err := svc.Create(
				configEvent.Name.String(),
				configEvent.DesiredState,
				configEvent.Executable,
				configEvent.Parameters,
			); err != nil {
				logError(logger, "create", configEvent.Name.String(), err)
			}

		case configwatcher.EventDeleted:
			if logger != nil {
				logger.Info("event: deleted", zap.String("service", configEvent.Name.String()))
			}

			delete(desiredStateByService, configEvent.Name.String())

			if err := svc.Remove(configEvent.Name.String()); err != nil {
				logError(logger, "remove", configEvent.Name.String(), err)
			}

		case configwatcher.EventStateChanged:
			if logger != nil {
				logger.Info("event: state-changed",
					zap.String("service", configEvent.Name.String()),
					zap.String("desired_state", configEvent.DesiredState.String()))
			}

			desiredStateByService[configEvent.Name.String()] = configEvent.DesiredState

			var err error
			if configEvent.DesiredState == shared.Up {
				err = svc.Start(configEvent.Name.String())
			} else {
				err = svc.Stop(configEvent.Name.String())
			}

			if err != nil {
				logError(logger, "state-change", configEvent.Name.String(), err)
			}

		case configwatcher.EventConfigChanged:
			if logger != nil {
				logger.Info("event: config-changed",
					zap.String("service", configEvent.Name.String()),
					zap.String("executable", configEvent.Executable),
					zap.Any("parameters", configEvent.Parameters))
			}
			// Re-create with same desired state as last known
			desiredState, ok := desiredStateByService[configEvent.Name.String()]
			if !ok {
				// default conservatively to Down
				desiredState = shared.Down
			}

			if err := svc.Create(
				configEvent.Name.String(),
				desiredState,
				configEvent.Executable,
				configEvent.Parameters,
			); err != nil {
				logError(logger, "config-change", configEvent.Name.String(), err)
			}
		}
	}
}
