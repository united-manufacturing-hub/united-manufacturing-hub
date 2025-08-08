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
	"fmt"
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

func logError(logger *zap.Logger, action, service string, err error) error {
	if logger != nil {
		logger.Error("manager call failed", zap.String("action", action), zap.String("service", service), zap.Error(err))
	}
	// also return for potential future handling
	return fmt.Errorf("%s %s failed: %w", action, service, err)
}

func handleConfigEvents(logger *zap.Logger, watcher configwatcher.ConfigWatcher, svc manager.Service, configPath string) {
	// Track desired state so ConfigChanged can reuse it without race/goroutines
	desiredStateByService := map[string]shared.State{}

	if err := watcher.Start(configPath); err != nil {
		panic(err)
	}

	for {
		event := <-watcher.Events()

		switch e := event.(type) {
		case configwatcher.EventCreated:
			desiredStateByService[e.Name.String()] = e.DesiredState
			if err := svc.Create(e.Name.String(), e.DesiredState, e.Executable, e.Parameters); err != nil {
				_ = logError(logger, "create", e.Name.String(), err)
			}

		case configwatcher.EventDeleted:
			delete(desiredStateByService, e.Name.String())
			if err := svc.Remove(e.Name.String()); err != nil {
				_ = logError(logger, "remove", e.Name.String(), err)
			}

		case configwatcher.EventStateChanged:
			desiredStateByService[e.Name.String()] = e.DesiredState
			var err error
			if e.DesiredState == shared.Up {
				err = svc.Start(e.Name.String())
			} else {
				err = svc.Stop(e.Name.String())
			}
			if err != nil {
				_ = logError(logger, "state-change", e.Name.String(), err)
			}

		case configwatcher.EventConfigChanged:
			// Re-create with same desired state as last known
			ds, ok := desiredStateByService[e.Name.String()]
			if !ok {
				// default conservatively to Down
				ds = shared.Down
			}
			if err := svc.Create(e.Name.String(), ds, e.Executable, e.Parameters); err != nil {
				_ = logError(logger, "config-change", e.Name.String(), err)
			}
		}
	}
}
