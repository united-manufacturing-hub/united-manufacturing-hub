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
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/pprof"
	v2 "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/communication_state"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/graphql"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/control"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/protocolconverter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/redpanda"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/version"
	"go.uber.org/zap"
)

func main() {
	// Initialize the global logger first thing
	logger.Initialize()

	// Initialize Sentry
	sentry.InitSentry(version.GetAppVersion(), true)

	// Get a logger for the main component
	log := logger.For(logger.ComponentCore)

	// Log using the component logger with structured fields
	log.Info("Starting umh-core...")

	// Start the pprof server (if enabled)
	pprof.StartPprofServer()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Configure encoder for communication
	encoding.ChooseEncoder(encoding.EncodingCorev1)

	// Load the config
	configManager, err := config.NewFileConfigManagerWithBackoff()
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeFatal, log, "Failed to create config manager: %w", err)
		os.Exit(1)
	}

	// Load or create configuration with environment variable overrides
	// This loads the config file if it exists, applies any environment variables as overrides,
	// and persists the result back to the config file. See detailed docs in config.LoadConfigWithEnvOverrides.
	configData, err := config.LoadConfigWithEnvOverrides(ctx, configManager, log)

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
	controlLoop := control.NewControlLoop(configManager)
	systemSnapshotManager := controlLoop.GetSnapshotManager()

	// Initialize the communication state
	communicationState := communication_state.NewCommunicationState(
		watchdog.NewWatchdog(ctx, time.NewTicker(time.Second*10), true, logger.For(logger.ComponentCommunicator)),
		make(chan *models.UMHMessage, 100),
		make(chan *models.UMHMessage, 100),
		configData.Agent.ReleaseChannel,
		systemSnapshotManager,
		configManager,
		configData.Agent.APIURL,
		logger.For(logger.ComponentCommunicator),
		configData.Agent.AllowInsecureTLS,
		topicbrowser.NewCache(),
	)

	// Initialize the topic browser simulator (cache update logic moved to subscriber notification pipeline)
	// This eliminates the redundant ticker and consolidates the architecture
	communicationState.InitializeTopicBrowserSimulator(configData.Agent.Simulator)

	// Start the GraphQL server if enabled
	var graphqlServer *graphql.Server
	if configData.Agent.GraphQLConfig.Enabled {
		// Set defaults for GraphQL config if not specified
		if configData.Agent.GraphQLConfig.Port == 0 {
			configData.Agent.GraphQLConfig.Port = 8090
		}
		if len(configData.Agent.GraphQLConfig.CORSOrigins) == 0 {
			configData.Agent.GraphQLConfig.CORSOrigins = []string{"*"}
		}

		// Create GraphQL resolver with necessary dependencies
		//
		// TopicBrowserCache: Required for GraphQL to access the unified namespace data
		// SnapshotManager: Required for GraphQL to access system configuration and state
		graphqlResolver := &graphql.Resolver{
			SnapshotManager:   systemSnapshotManager,
			TopicBrowserCache: communicationState.TopicBrowserCache,
		}

		// Start GraphQL server
		var err error
		graphqlServer, err = graphql.StartGraphQLServer(graphqlResolver, &configData.Agent.GraphQLConfig, log)
		if err != nil {
			sentry.ReportIssuef(sentry.IssueTypeFatal, log, "Failed to start GraphQL server: %w", err)
		}

		defer func() {
			if graphqlServer != nil {
				shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer shutdownCancel()
				if err := graphqlServer.Stop(shutdownCtx); err != nil {
					sentry.ReportIssuef(sentry.IssueTypeError, log, "Failed to shutdown GraphQL server: %w", err)
				}
			}
		}()
	} else {
		log.Info("GraphQL server disabled via configuration")
	}

	if configData.Agent.APIURL != "" && configData.Agent.AuthToken != "" {
		enableBackendConnection(&configData, communicationState, controlLoop, communicationState.Logger)
	} else {
		log.Warnf("No backend connection enabled, please set API_URL and AUTH_TOKEN")
	}

	// Start the system snapshot logger
	go SystemSnapshotLogger(ctx, controlLoop)

	// Start the control loop
	err = controlLoop.Execute(ctx)
	if err != nil {
		log.Errorf("Control loop failed: %w", err)
		sentry.ReportIssuef(sentry.IssueTypeFatal, log, "Control loop failed: %w", err)
	}

	log.Info("umh-core completed")
}

// SystemSnapshotLogger logs the system snapshot every 5 seconds
// It is an example on how to access the system snapshot and log it for communication with other components
func SystemSnapshotLogger(ctx context.Context, controlLoop *control.ControlLoop) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	snap_logger := logger.For("SnapshotLogger")
	if snap_logger == nil {
		snap_logger = zap.NewNop().Sugar()
	}

	snap_logger.Info("Starting system snapshot logger")

	for {
		select {
		case <-ctx.Done():
			snap_logger.Info("Stopping system snapshot logger")
			return
		case <-ticker.C:
			snapshot := controlLoop.GetSystemSnapshot()
			if snapshot == nil {
				sentry.ReportIssuef(sentry.IssueTypeWarning, snap_logger, "[SystemSnapshotLogger] No system snapshot available")
				continue
			}

			snap_logger.Infof("=== System Snapshot (Tick %d) - %d Managers ===",
				snapshot.Tick, len(snapshot.Managers))

			// Log manager information
			for managerName, manager := range snapshot.Managers {
				instances := manager.GetInstances()

				if len(instances) == 0 {
					snap_logger.Infof("📁 %s (tick: %d) - No instances",
						managerName, manager.GetManagerTick())
				} else {
					snap_logger.Infof("📁 %s (tick: %d) - %d instance(s):",
						managerName, manager.GetManagerTick(), len(instances))

					// Log instance information with indentation
					for instanceName, instance := range instances {
						statusReason := ""

						// Extract StatusReason from LastObservedState based on manager type
						if instance.LastObservedState != nil {
							switch managerName {
							case "DataFlowCompManagerCore":
								if dfcSnapshot, ok := instance.LastObservedState.(*dataflowcomponent.DataflowComponentObservedStateSnapshot); ok {
									statusReason = dfcSnapshot.ServiceInfo.StatusReason
								}
							case "ProtocolConverterManagerCore":
								if pcSnapshot, ok := instance.LastObservedState.(*protocolconverter.ProtocolConverterObservedStateSnapshot); ok {
									statusReason = pcSnapshot.ServiceInfo.StatusReason
								}
							case "RedpandaManagerCore":
								if redpandaSnapshot, ok := instance.LastObservedState.(*redpanda.RedpandaObservedStateSnapshot); ok {
									statusReason = redpandaSnapshot.ServiceInfoSnapshot.StatusReason
								}
							}
						}

						// Format state with emojis for better visibility
						stateIcon := "⚠️"
						switch instance.CurrentState {
						case "active":
							stateIcon = "✅"
						case "stopped":
							stateIcon = "⏹️"
						case "idle":
							stateIcon = "💤"
						case "degraded":
							stateIcon = "⚠️"
						}

						if statusReason != "" {
							snap_logger.Infof("  └─ %s %s: %s → %s | %s",
								stateIcon, instanceName, instance.CurrentState, instance.DesiredState, statusReason)
						} else {
							snap_logger.Infof("  └─ %s %s: %s → %s",
								stateIcon, instanceName, instance.CurrentState, instance.DesiredState)
						}
					}
				}
			}
		}
	}
}

func enableBackendConnection(config *config.FullConfig, communicationState *communication_state.CommunicationState, controlLoop *control.ControlLoop, logger *zap.SugaredLogger) {

	logger.Info("Enabling backend connection")
	// directly log the config to console, not to the logger
	if config == nil {
		logger.Warn("Config is nil, cannot enable backend connection")
		return
	}

	if config.Agent.APIURL != "" && config.Agent.AuthToken != "" {
		// This can temporarely deactivated, e.g., during integration tests where just the mgmtcompanion-config is changed directly

		login := v2.NewLogin(config.Agent.AuthToken, config.Agent.AllowInsecureTLS, config.Agent.APIURL, logger)
		if login == nil {
			sentry.ReportIssuef(sentry.IssueTypeError, logger, "[v2.NewLogin] Failed to create login object")
			return
		}
		communicationState.LoginResponseMu.Lock()
		communicationState.LoginResponse = login
		communicationState.LoginResponseMu.Unlock()
		logger.Info("Backend connection enabled, login response: ", zap.Any("login_name", login.Name))

		// Get the config manager from the control loop
		configManager := controlLoop.GetConfigManager()
		snapshotManager := controlLoop.GetSnapshotManager()
		communicationState.InitialiseAndStartPuller()
		communicationState.InitialiseAndStartPusher()
		communicationState.InitialiseAndStartSubscriberHandler(time.Minute*5, time.Minute, config, snapshotManager, configManager)
		communicationState.InitialiseAndStartRouter()
		communicationState.InitialiseReAuthHandler(config.Agent.AuthToken, config.Agent.AllowInsecureTLS)

	}

	logger.Info("Backend connection enabled")
}
