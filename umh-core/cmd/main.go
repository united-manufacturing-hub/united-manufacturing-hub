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
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/pprof"
	v2 "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/communication_state"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/fsmv2_adapter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/graphql"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/control"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/protocolconverter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/redpanda"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/streamprocessor"
	topicbrowserfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/version"
	"go.uber.org/zap"
)

// CommunicatorWorkerID is the ID used for the communicator child worker.
// This matches the pattern from reconciliation.go:1218 (childID := spec.Name + "-001")
// where spec.Name is "communicator".
const CommunicatorWorkerID = "communicator-001"

func main() {
	// Initialize the global logger first thing
	logger.Initialize()

	// Initialize Sentry
	sentry.InitSentry(version.GetAppVersion(), true)

	// Get a logger for the main component
	log := logger.For(logger.ComponentCore)

	// Set up global panic recovery to catch any unhandled panics
	defer sentry.HandleGlobalPanic(log)

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
		cancel()

		return
	}

	// Load or create configuration with environment variable overrides
	// This loads the config file if it exists, applies any environment variables as overrides,
	// and persists the result back to the config file. See detailed docs in config.LoadConfigWithEnvOverrides.
	configData, err := config.LoadConfigWithEnvOverrides(ctx, configManager, log)
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeFatal, log, "Failed to load config: %w", err)

		return
	}

	// Ensure the S6 repository directory exists
	// This is particularly important when using /tmp/umh-core/services (the default)
	if err := ensureS6RepositoryDirectory(log); err != nil {
		sentry.ReportIssuef(sentry.IssueTypeFatal, log, "Failed to ensure S6 repository directory: %w", err)

		return
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
		if configData.Agent.UseFSMv2Transport {
			log.Info("Using FSMv2 communicator (feature flag enabled)")
			sentry.SafeGoWithContext(ctx, func(ctx context.Context) {
				enableFSMv2BackendConnection(ctx, &configData, communicationState, log)
			})
		} else {
			sentry.SafeGoWithContext(ctx, func(ctx context.Context) {
				enableBackendConnection(ctx, &configData, communicationState, controlLoop, communicationState.Logger)
			})
		}
	} else {
		log.Warnf("No backend connection enabled, please set API_URL and AUTH_TOKEN")
	}

	// Start the system snapshot logger with automatic panic recovery
	sentry.SafeGoWithContext(ctx, func(ctx context.Context) {
		SystemSnapshotLogger(ctx, controlLoop)
	})

	// Start the control loop
	err = controlLoop.Execute(ctx)
	if err != nil {
		log.Errorf("Control loop failed: %w", err)
		sentry.ReportIssuef(sentry.IssueTypeFatal, log, "Control loop failed: %w", err)
	}

	log.Info("umh-core completed")
}

// ensureS6RepositoryDirectory ensures that the S6 repository directory exists with proper permissions.
// This is critical when using the temporary directory (/tmp/umh-core-services) which may not exist after container restart.
// The function creates the directory if it doesn't exist and logs whether persistent or temporary storage is being used.
func ensureS6RepositoryDirectory(log *zap.SugaredLogger) error {
	repoDir := constants.GetS6RepositoryBaseDir()

	// Check if directory exists
	if _, err := os.Stat(repoDir); os.IsNotExist(err) {
		// Create the directory with appropriate permissions
		if err := os.MkdirAll(repoDir, 0755); err != nil {
			return fmt.Errorf("failed to create S6 repository directory %s: %w", repoDir, err)
		}

		log.Infof("Created S6 repository directory: %s", repoDir)
	} else if err != nil {
		return fmt.Errorf("failed to check S6 repository directory %s: %w", repoDir, err)
	} else {
		log.Debugf("S6 repository directory already exists: %s", repoDir)
	}

	// Log whether we're using persistent or temporary storage
	persist, _ := strconv.ParseBool(os.Getenv("S6_PERSIST_DIRECTORY"))
	if persist {
		log.Infof("Using persistent S6 directory for debugging: %s", repoDir)
	} else {
		log.Infof("Using temporary S6 directory (cleared on container restart): %s", repoDir)
	}

	return nil
}

// SystemSnapshotLogger logs the system snapshot every 5 seconds
// It is an example on how to access the system snapshot and log it for communication with other components.
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
							case "BenthosManagerCore":
								if benthosSnapshot, ok := instance.LastObservedState.(*benthos.BenthosObservedStateSnapshot); ok {
									statusReason = benthosSnapshot.ServiceInfo.BenthosStatus.StatusReason
								}
							case "StreamProcessorManagerCore":
								if spSnapshot, ok := instance.LastObservedState.(*streamprocessor.ObservedStateSnapshot); ok {
									statusReason = spSnapshot.ServiceInfo.StatusReason
								}
							case "TopicBrowserManagerCore":
								if tbSnapshot, ok := instance.LastObservedState.(*topicbrowserfsm.ObservedStateSnapshot); ok {
									statusReason = tbSnapshot.ServiceInfo.StatusReason
								}
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

func enableBackendConnection(ctx context.Context, config *config.FullConfig, communicationState *communication_state.CommunicationState, controlLoop *control.ControlLoop, logger *zap.SugaredLogger) {
	logger.Info("Enabling backend connection")
	// directly log the config to console, not to the logger
	if config == nil {
		logger.Warn("Config is nil, cannot enable backend connection")

		return
	}

	if config.Agent.APIURL != "" && config.Agent.AuthToken != "" {
		// This can temporarely deactivated, e.g., during integration tests where just the mgmtcompanion-config is changed directly
		// Call NewLogin in a goroutine since it blocks with retry logic
		loginChan := make(chan *v2.LoginResponse, 1)

		go func() {
			login := v2.NewLogin(ctx, config.Agent.AuthToken, config.Agent.AllowInsecureTLS, config.Agent.APIURL, logger)
			select {
			case loginChan <- login:
			case <-ctx.Done():
			}
		}()

		var login *v2.LoginResponse
		select {
		case login = <-loginChan:
			// Login completed
		case <-ctx.Done():
			// Context cancelled before login completed
			logger.Info("Backend connection context cancelled before login completed")

			return
		}

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
		communicationState.InitialiseAndStartSubscriberHandler(time.Minute*5, time.Minute, config, snapshotManager, configManager, nil) // nil = legacy mode uses Pusher
		communicationState.InitialiseAndStartRouter()
		communicationState.InitialiseReAuthHandler(config.Agent.AuthToken, config.Agent.AllowInsecureTLS)

		// Wait for context cancellation
		<-ctx.Done()
		logger.Info("Backend connection context cancelled, shutting down")
	}

	logger.Info("Backend connection enabled")
}

func enableFSMv2BackendConnection(
	ctx context.Context,
	configData *config.FullConfig,
	communicationState *communication_state.CommunicationState,
	logger *zap.SugaredLogger,
) {
	logger.Info("Enabling FSMv2 backend connection")

	if configData == nil {
		logger.Warn("Config is nil, cannot enable FSMv2 backend connection")

		return
	}

	// Create channel adapter to bridge FSMv2 and legacy channels
	channelAdapter := fsmv2_adapter.NewLegacyChannelBridge(
		communicationState.InboundChannel,
		communicationState.OutboundChannel,
		logger,
	)

	// Start conversion goroutines
	channelAdapter.Start(ctx)

	// Set the global ChannelProvider singleton BEFORE creating the supervisor.
	// Phase 1 architecture: singleton is THE ONLY way to provide channels to the communicator.
	// The factory will panic if this is not set.
	communicator.SetChannelProvider(channelAdapter)

	// Build YAML config for FSMv2 ApplicationSupervisor
	// Note: instanceUUID in config is a placeholder - the real UUID is returned by the backend
	// and will be set via onAuthSuccessCallback (Bug #6 fix)
	placeholderUUID := uuid.New().String()
	yamlConfig := fmt.Sprintf(`
children:
  - name: "communicator"
    workerType: "communicator"
    userSpec:
      config: |
        relayURL: "%s"
        instanceUUID: "%s"
        authToken: "%s"
        timeout: "10s"
        state: "running"
`, configData.Agent.APIURL, placeholderUUID, configData.Agent.AuthToken)

	// Setup store (in-memory for now)
	store := examples.SetupStore(logger)

	// Create callback to update LoginResponse with real UUID from backend (Bug #6 fix)
	// This is called by AuthenticateAction after successful authentication
	onAuthSuccessCallback := func(realUUID, name string) {
		logger.Infow("Authentication succeeded, updating LoginResponse with backend UUID",
			"realUUID", realUUID, "name", name, "placeholderUUID", placeholderUUID)
		communicationState.SetLoginResponseForFSMv2(realUUID)
	}

	// Create ApplicationSupervisor with channel provider and auth callback injected via Dependencies
	// This avoids global state and enables proper testing
	// Use Named("fsmv2") to create [fsmv2] prefix in logs for easy filtering
	fsmv2Logger := logger.Named("fsmv2")
	appSup, err := application.NewApplicationSupervisor(application.SupervisorConfig{
		ID:           "fsmv2-communicator",
		Name:         "FSMv2 Communicator",
		Store:        store,
		Logger:       fsmv2Logger,
		TickInterval: 100 * time.Millisecond,
		YAMLConfig:   yamlConfig,
		Dependencies: map[string]any{
			"channelProvider":       channelAdapter,
			"onAuthSuccessCallback": onAuthSuccessCallback,
		},
	})
	if err != nil {
		logger.Errorw("Failed to create FSMv2 supervisor", "error", err)

		return
	}

	// Start supervisor (non-blocking - returns done channel)
	done := appSup.Start(ctx)

	// Initialize Router for FSMv2 mode:
	// 1. Create write-only Pusher (writes to channel, FSMv2 handles HTTP)
	// 2. Set LoginResponse with placeholder UUID (will be updated by onAuthSuccessCallback)
	// 3. Initialize SubscriberHandler (generates status messages)
	// 4. Start Router (processes inbound messages, generates status via Subscriber)
	communicationState.InitializeWriteOnlyPusher(placeholderUUID)
	communicationState.SetLoginResponseForFSMv2(placeholderUUID)
	communicationState.InitialiseAndStartSubscriberHandler(
		5*time.Minute, // TTL: time until subscriber considered dead
		1*time.Minute, // Cull: cycle time to remove dead subscribers
		configData,
		communicationState.SystemSnapshotManager,
		communicationState.ConfigManager,
		channelAdapter.GetOutboundWriteChannel(), // FSMv2 mode: bypass Pusher, write directly to FSMv2 transport
	)
	communicationState.InitializeRouterForFSMv2()

	// Poll ObservedState for AuthenticatedUUID and update SetLoginResponseForFSMv2.
	// Phase 2 architecture: UUID is read from ObservedState instead of callback.
	// This goroutine runs until the real UUID is received from the backend.
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Load the communicator's observed state from the store
				var observed snapshot.CommunicatorObservedState
				err := store.LoadObservedTyped(ctx, "communicator", CommunicatorWorkerID, &observed)
				if err != nil {
					// Not found yet or error - keep polling
					continue
				}

				if observed.AuthenticatedUUID != "" && observed.AuthenticatedUUID != placeholderUUID {
					logger.Infow("Detected real UUID from ObservedState, updating LoginResponse",
						"realUUID", observed.AuthenticatedUUID,
						"placeholderUUID", placeholderUUID)
					communicationState.SetLoginResponseForFSMv2(observed.AuthenticatedUUID)
					return
				}
			}
		}
	}()

	logger.Info("FSMv2 communicator started, waiting for shutdown")

	// Wait for shutdown
	<-done

	logger.Info("FSMv2 backend connection shutdown complete")
}
