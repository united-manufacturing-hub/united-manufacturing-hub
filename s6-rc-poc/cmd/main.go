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
	"s6-rc-poc/cmd/configwatcher"
	"s6-rc-poc/cmd/manager"
	"s6-rc-poc/cmd/shared"

	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewProduction()
	watcher := configwatcher.NewFileWatcher(logger)
	svc := manager.NewS6RCService(logger, "", "", "")

	handleConfigEvents(logger, watcher, svc, "/config.yaml")
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
