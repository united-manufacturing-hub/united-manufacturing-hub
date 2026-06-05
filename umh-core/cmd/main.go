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
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/pprof"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/env"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/protocolconverter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/redpanda"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/streamprocessor"
	topicbrowserfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	fsmv2sentry "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/sentry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator"
	persistenceWorker "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence"
	transportWorker "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
	transportSnapshot "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/snapshot"
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

	// Set up global panic recovery to catch any unhandled panics
	defer sentry.HandleGlobalPanic(log)

	// Log using the component logger with structured fields
	log.Info("Starting umh-core...")

	// Start the pprof server (if enabled)
	pprof.StartPprofServer()

	// Register SIGINT/SIGTERM so the FSMv2 supervisor can complete its
	// shutdown sequence (workers and children) before the process exits.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// signal.NotifyContext is single-shot: it cancels ctx on the first signal
	// and exits, after which Go's stdlib silently drops further signals. To
	// give operators a real fast-exit on a second SIGTERM, we run a parallel
	// signal.Notify channel and close forceExit on the second receive. The
	// FSMv2 supervisor hierarchy shares forceExit via Config; closing it
	// breaks every drain loop at once. SIGKILL remains the final escalation.
	forceExit := make(chan struct{})
	sigCh := make(chan os.Signal, 2)

	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	go func() {
		// The first signal also goes to NotifyContext, which cancels ctx.
		// Wait unconditionally so we never miss it; the receive is buffered.
		<-sigCh
		// If the operator sends a second signal, broadcast force-exit. If
		// they don't, this goroutine sits idle until the process exits.
		<-sigCh
		close(forceExit)
	}()

	// Configure encoder for communication
	encoding.ChooseEncoder(encoding.EncodingCorev1)

	// Load the config
	configManager, err := config.NewFileConfigManagerWithBackoff()
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeFatal, log, "Failed to create config manager: %w", err)
		log.Errorf("Sleeping 60s before exit to rate-limit restart loop (ENG-4369)")
		sentry.Flush(5 * time.Second)
		time.Sleep(60 * time.Second)
		cancel()

		return
	}

	// Config backup feature flag: must be set before LoadConfigWithEnvOverrides,
	// which writes config on startup and should back up the pre-write state.
	// GetAsBool with required=false never returns an error (silently falls back
	// to the default on parse failure); see ENG-4809 for the signature fix.
	configBackupEnabled, _ := env.GetAsBool("ENABLE_CONFIG_BACKUP", false, true)

	configManager.SetConfigBackupEnabled(configBackupEnabled)

	// Load or create configuration with environment variable overrides
	// This loads the config file if it exists, applies any environment variables as overrides,
	// and persists the result back to the config file. See detailed docs in config.LoadConfigWithEnvOverrides.
	configData, err := config.LoadConfigWithEnvOverrides(ctx, configManager, log)
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeFatal, log, "Failed to load config: %w", err)
		log.Errorf("Sleeping 60s before exit to rate-limit restart loop (ENG-4369)")
		sentry.Flush(5 * time.Second)
		time.Sleep(60 * time.Second)

		return
	}

	// FSMv2 feature flags: read directly from env vars, not persisted to config.yaml.
	// These bypass the config manager intentionally — they are temporary migration flags
	// that will be replaced when the config manager becomes an FSMv2 worker.
	// GetAsBool with required=false never returns an error (silently falls back
	// to the default on parse failure); see ENG-4809 for the signature fix.
	transportEnabled, _ := env.GetAsBool("USE_FSMV2_TRANSPORT", false, true)
	memoryCleanupEnabled, _ := env.GetAsBool("USE_FSMV2_MEMORY_CLEANUP", false, true)

	// Memory cleanup is required whenever transport is on; running transport
	// without cleanup reintroduces the unbounded state-growth risk (ENG-4292).
	if transportEnabled {
		memoryCleanupEnabled = true
	}

	configData.Agent.UseFSMv2Transport = transportEnabled
	configData.Agent.UseFSMv2MemoryCleanup = memoryCleanupEnabled

	protocolConverterEnabled, _ := env.GetAsBool("USE_FSMV2_PROTOCOL_CONVERTER", false, false)
	configData.Agent.UseFSMv2ProtocolConverter = protocolConverterEnabled

	featureUsage := &models.FeatureUsage{
		ConfigBackupEnabled:           configBackupEnabled,
		FSMv2TransportEnabled:         configData.Agent.UseFSMv2Transport,
		FSMv2MemoryCleanupEnabled:     configData.Agent.UseFSMv2MemoryCleanup,
		FSMv2ProtocolConverterEnabled: configData.Agent.UseFSMv2ProtocolConverter,
		ResourceLimitBlockingEnabled:  configData.Agent.EnableResourceLimitBlocking,
	}

	// Ensure the S6 repository directory exists
	// This is particularly important when using /tmp/umh-core/services (the default)
	if err := ensureS6RepositoryDirectory(log); err != nil {
		sentry.ReportIssuef(sentry.IssueTypeFatal, log, "Failed to ensure S6 repository directory: %w", err)
		log.Errorf("Sleeping 60s before exit to rate-limit restart loop (ENG-4369)")
		sentry.Flush(5 * time.Second)
		time.Sleep(60 * time.Second)

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
		featureUsage,
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

	// fsmv2Done is closed when the FSMv2 supervisor has fully stopped.
	// main() waits on it at the bottom so the process does not exit before
	// the supervisor drains.  When FSMv2 transport is disabled (or the
	// API URL / token are absent), the channel is closed immediately so
	// the <-fsmv2Done below is a no-op.
	fsmv2Done := make(chan struct{})

	if configData.Agent.APIURL != "" && configData.Agent.AuthToken != "" {
		if configData.Agent.UseFSMv2Transport {
			log.Info("Using FSMv2 communicator (feature flag enabled)")

			appSup, channelAdapter, fsmv2Store, placeholderUUID, cleanup, buildErr := buildFSMv2Supervisor(ctx, &configData, communicationState, log, forceExit)
			if buildErr != nil {
				log.Errorw("Failed to build FSMv2 supervisor", "error", buildErr)
				close(fsmv2Done)
			} else {
				defer cleanup()

				go func() {
					defer close(fsmv2Done)

					if runErr := appSup.Run(ctx); runErr != nil {
						log.Errorw("FSMv2 supervisor Run error", "error", runErr)
					}
				}()

				sentry.SafeGoWithContext(ctx, func(ctx context.Context) {
					wireFSMv2Communicator(ctx, appSup, channelAdapter, fsmv2Store, placeholderUUID, &configData, communicationState, log)
				})
			}
		} else {
			close(fsmv2Done)
			sentry.SafeGoWithContext(ctx, func(ctx context.Context) {
				enableBackendConnection(ctx, &configData, communicationState, controlLoop, communicationState.Logger)
			})
		}
	} else {
		log.Warnf("No backend connection enabled, please set API_URL and AUTH_TOKEN")
		close(fsmv2Done)
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

	// Block until the FSMv2 supervisor has shut down. If it never started,
	// fsmv2Done was closed at creation and this is a no-op.
	<-fsmv2Done

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

// fsmv2Supervisor is the minimal interface required from the ApplicationSupervisor
// by main().  Using an interface keeps buildFSMv2Supervisor decoupled from the
// concrete generic type and avoids repeating the full type parameter list.
type fsmv2Supervisor interface {
	metrics.FSMv2DebugProvider
	Run(ctx context.Context) error
}

// buildFSMv2Supervisor constructs the ApplicationSupervisor and the channel
// adapter used in FSMv2 transport mode.  It returns a cleanup function that
// must be deferred by the caller to stop the SentryHook goroutine.
// The returned appSup has NOT been started yet; the caller is responsible for
// calling appSup.Run(ctx) (or Start+wait).
func buildFSMv2Supervisor(
	ctx context.Context,
	configData *config.FullConfig,
	communicationState *communication_state.CommunicationState,
	logger *zap.SugaredLogger,
	forceExit <-chan struct{},
) (appSup fsmv2Supervisor, channelAdapter *fsmv2_adapter.LegacyChannelBridge, store storage.TriangularStoreInterface, placeholderUUID string, cleanup func(), err error) {
	if configData == nil {
		return nil, nil, nil, "", func() {}, errors.New("config is nil, cannot build FSMv2 supervisor")
	}

	// Create channel adapter to bridge FSMv2 and legacy channels.
	// Buffer size 0 uses the default (DefaultBufferSize).
	channelAdapter = fsmv2_adapter.NewLegacyChannelBridge(
		communicationState.InboundChannel,
		communicationState.OutboundChannel,
		logger,
		0,
	)

	// Start conversion goroutines.
	channelAdapter.Start(ctx)

	// Set the global ChannelProvider singleton BEFORE creating the supervisor.
	// Phase 1 architecture: singleton is THE ONLY way to provide channels to the communicator.
	// The factory will panic if this is not set.
	communicator.SetChannelProvider(channelAdapter)
	transportWorker.SetChannelProvider(channelAdapter)

	// Build YAML config for FSMv2 ApplicationSupervisor.
	// instanceUUID is a placeholder — the real UUID is returned by the backend
	// and picked up by polling TransportWorker.ObservedState.AuthenticatedUUID below (Bug #6 fix).
	placeholderUUID = uuid.New().String()
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

	if configData.Agent.UseFSMv2MemoryCleanup {
		yamlConfig += `  - name: "persistence"
    workerType: "persistence"
`
	}

	// Setup store (in-memory for now).
	store = examples.SetupStore(deps.NewFSMLogger(logger))

	// Use Named("fsmv2") to create [fsmv2] prefix in logs for easy filtering.
	fsmv2Logger := logger.Named("fsmv2")
	// Wrap with FSMv2 SentryHook for automatic error capture to Sentry with:
	// - Per-fingerprint debouncing (5 min window)
	// - Error chain extraction for stable fingerprinting
	// - Feature-based routing for error ownership
	fsmv2Hook := fsmv2sentry.NewSentryHook(5 * time.Minute)

	// Release the Communicator action-handler SentryHook's debouncer
	// goroutine. The hook is lazy-initialized on first action; this
	// no-ops when nothing exercised the path. Symmetric with the
	// fsmv2Hook.Stop() above (pkg/communicator/actions/actions.go).
	defer actions.StopCommunicatorSentryHook()

	fsmv2Logger = fsmv2Logger.Desugar().WithOptions(zap.WrapCore(fsmv2Hook.Wrap)).Sugar()

	fsmv2Deps := map[string]any{}
	if configData.Agent.UseFSMv2MemoryCleanup {
		register.SetDeps[*persistenceWorker.PersistenceDependencies](persistenceWorker.WorkerTypeName, persistenceWorker.NewStoreOnlyDependencies(store))
	}

	appSup, err = application.NewApplicationSupervisor(application.SupervisorConfig{
		ID:           "application-fsmv2",
		Name:         "Application FSMv2",
		Store:        store,
		Logger:       deps.NewFSMLogger(fsmv2Logger),
		TickInterval: 100 * time.Millisecond,
		YAMLConfig:   yamlConfig,
		Dependencies: fsmv2Deps,
		ForceExit:    forceExit,
	})
	if err != nil {
		fsmv2Hook.Stop()

		return nil, nil, nil, "", func() {}, fmt.Errorf("failed to create FSMv2 supervisor: %w", err)
	}

	// Register supervisor for debug introspection (/debug/fsmv2 endpoint).
	metrics.RegisterFSMv2DebugProvider("application", appSup)

	cleanup = fsmv2Hook.Stop

	return appSup, channelAdapter, store, placeholderUUID, cleanup, nil
}

// wireFSMv2Communicator wires the legacy CommunicationState to the already-started
// FSMv2 supervisor.  It sets up the write-only pusher, subscriber handler, and
// FSMv2 router, then polls the TransportWorker's ObservedState until the real
// authenticated UUID is available.  This function blocks until ctx is cancelled.
func wireFSMv2Communicator(
	ctx context.Context,
	appSup fsmv2Supervisor,
	channelAdapter *fsmv2_adapter.LegacyChannelBridge,
	store storage.TriangularStoreInterface,
	placeholderUUID string,
	configData *config.FullConfig,
	communicationState *communication_state.CommunicationState,
	logger *zap.SugaredLogger,
) {
	_ = appSup // kept for future per-supervisor introspection

	// Initialize Router for FSMv2 mode:
	// 1. Create write-only Pusher (writes to channel, FSMv2 handles HTTP)
	// 2. Set LoginResponse with placeholder UUID (replaced when TransportWorker.ObservedState.AuthenticatedUUID is observed below)
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

	// Poll the TransportWorker's ObservedState for AuthenticatedUUID.
	// TransportWorker handles authentication (ENG-4264) and exposes the UUID
	// from the backend response. The transport child ID follows the supervisor
	// naming convention: spec.Name + "-001" = "transport-001".
	//
	// This goroutine exits as soon as the real UUID is detected or the context
	// is cancelled; it does not block the caller.
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				var observed fsmv2.Observation[transportSnapshot.TransportStatus]

				err := store.LoadObservedTyped(ctx, "transport", "transport-001", &observed)
				if err != nil {
					continue
				}

				if observed.Status.AuthenticatedUUID != "" && observed.Status.AuthenticatedUUID != placeholderUUID {
					logger.Infow("Detected real UUID from TransportWorker ObservedState, updating LoginResponse",
						"realUUID", observed.Status.AuthenticatedUUID,
						"placeholderUUID", placeholderUUID)
					communicationState.SetLoginResponseForFSMv2(observed.Status.AuthenticatedUUID)

					return
				}
			}
		}
	}()

	logger.Info("FSMv2 communicator wired, waiting for context cancellation")

	// Block until the caller's context is cancelled (e.g. main ctx cancelled).
	<-ctx.Done()

	logger.Info("FSMv2 communicator context cancelled")
}
