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

package main

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	v2 "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/communication_state"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/control"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"go.uber.org/zap"
)

var appVersion = constants.DefaultAppVersion // set by the build system

func main() {
	// Initialize the global logger first thing
	logger.Initialize()

	// Initialize Sentry
	sentry.InitSentry(appVersion, true)

	// Get a logger for the main component
	log := logger.For(logger.ComponentCore)

	// Log using the component logger with structured fields
	log.Info("Starting umh-core...")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Load the config
	configManager := config.NewFileConfigManager()
	// this will check if the config at the given path exists and if not, it will be created with default values
	configData, err := configManager.GetConfig(ctx, 0)
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeFatal, log, "Failed to load config: %w", err)
		os.Exit(1)
	}

	// Start the metrics server
	server := metrics.SetupMetricsEndpoint(fmt.Sprintf(":%d", configData.Agent.MetricsPort))
	defer func() {
		// S6_KILL_FINISH_MAXTIME is 5 seconds, so we need to finish before that
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			sentry.ReportIssuef(sentry.IssueTypeError, log, "Failed to shutdown metrics server: %w", err)
		}
	}()

	// Start the control loop
	controlLoop := control.NewControlLoop()
	systemSnapshot := new(fsm.SystemSnapshot)
	systemMu := new(sync.Mutex)
	communicationState := communication_state.CommunicationState{
		Watchdog:        watchdog.NewWatchdog(ctx, time.NewTicker(time.Second*10), true),
		InboundChannel:  make(chan *models.UMHMessage, 100),
		OutboundChannel: make(chan *models.UMHMessage, 100),
		ReleaseChannel:  configData.Agent.ReleaseChannel,
	}
	go SystemSnapshotLogger(ctx, controlLoop, systemSnapshot, systemMu)

	enableBackendConnection(&configData, systemSnapshot, &communicationState, systemMu)
	controlLoop.Execute(ctx)

	log.Info("umh-core completed")
}

// SystemSnapshotLogger logs the system snapshot every 5 seconds
// It is an example on how to access the system snapshot and log it for communication with other components
func SystemSnapshotLogger(ctx context.Context, controlLoop *control.ControlLoop, systemSnapshot *fsm.SystemSnapshot, systemMu *sync.Mutex) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	logger := logger.For("SnapshotLogger")
	if logger == nil {
		logger = zap.NewNop().Sugar()
	}

	logger.Info("Starting system snapshot logger")

	for {
		select {
		case <-ctx.Done():
			logger.Info("Stopping system snapshot logger")
			return
		case <-ticker.C:
			snapshot := controlLoop.GetSystemSnapshot()
			if snapshot != nil {
				systemMu.Lock()
				*systemSnapshot = *snapshot
				systemMu.Unlock()
			}
			if snapshot == nil {
				sentry.ReportIssuef(sentry.IssueTypeWarning, logger, "No system snapshot available")
				continue
			}

			logger.Infof("System snapshot at tick %d, managers: %d",
				snapshot.Tick, len(snapshot.Managers))

			// Log manager information
			for managerName, manager := range snapshot.Managers {
				instances := manager.GetInstances()
				logger.Infof("Manager: %s, instances: %d, tick: %d",
					managerName, len(instances), manager.GetManagerTick())

				// Log instance information
				for instanceName, instance := range instances {
					logger.Infof("Instance: %s, current state: %s, desired state: %s",
						instanceName, instance.CurrentState, instance.DesiredState)

					// Log observed state if available
					if instance.LastObservedState != nil {
						logger.Debugf("Observed state: %v", instance.LastObservedState)
					}
				}
			}
		}
	}
}

func enableBackendConnection(config *config.FullConfig, state *fsm.SystemSnapshot, communicationState *communication_state.CommunicationState, systemMu *sync.Mutex) {
	logger := logger.For("enableBackendConnection")
	if logger == nil {
		logger = zap.NewNop().Sugar()
	}

	logger.Info("Enabling backend connection")
	// directly log the config to console, not to the logger
	if config == nil {
		logger.Warn("Config is nil, cannot enable backend connection")
		return
	}

	if config.Agent.CommunicatorConfig.APIURL != "" && config.Agent.CommunicatorConfig.AuthToken != "" {
		// This can temporarely deactivated, e.g., during integration tests where just the mgmtcompanion-config is changed directly

		login := v2.NewLogin(config.Agent.CommunicatorConfig.AuthToken, false)
		if login == nil {
			sentry.ReportIssuef(sentry.IssueTypeError, logger, "Failed to create login object")
			return
		}
		communicationState.LoginResponse = login
		logger.Info("Backend connection enabled, login response: ", zap.Any("login_name", login.Name))

		communicationState.InitialiseAndStartPuller()
		communicationState.InitialiseAndStartPusher()
		communicationState.InitialiseAndStartSubscriberHandler(time.Minute*5, time.Minute, config, state, systemMu)
		communicationState.InitialiseAndStartRouter()
	}
}
