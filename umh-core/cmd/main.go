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
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	fsmv2config "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps/secrets"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/handoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/managementconsole"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/scenariorunner"
	fsmv2sentry "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/sentry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/pprof"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/communication_state"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/graphql"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/control"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/env"
	fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/protocolconverter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/redpanda"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/streamprocessor"
	topicbrowserfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker"
	persistenceWorker "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/version"
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
	protocolConverterEnabled, _ := env.GetAsBool("USE_FSMV2_PROTOCOL_CONVERTER", false, false)

	cpuMonitorEnabled, _ := env.GetAsBool("USE_FSMV2_CPU", false, false)
	configData.Agent.UseFSMv2CPU = cpuMonitorEnabled

	featureUsage := &models.FeatureUsage{
		ConfigBackupEnabled:           configBackupEnabled,
		FSMv2CPUEnabled:               models.FSMv2CPUEnabled(cpuMonitorEnabled),
		FSMv2TransportEnabled:         true, // FSMv2 is the only bring-up path
		FSMv2MemoryCleanupEnabled:     true, // persistence runs unconditionally
		FSMv2ProtocolConverterEnabled: protocolConverterEnabled,
		ResourceLimitBlockingEnabled:  configData.Agent.EnableResourceLimitBlocking,
		HistorianConfigured:           configData.Historian != nil,
		HistorianBridgeCount:          countHistorianBridges(configData),
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
		logger.For(logger.ComponentCommunicator),
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
	// the supervisor drains.  If startup fails before the supervisor runs,
	// the process exits above and never reaches this wait.
	transport, transportErr := startFSMv2Transport(ctx, &configData, communicationState, log, forceExit)
	if transportErr != nil {
		log.Errorw("FSMv2 startup failed", "error", transportErr)
		_ = log.Sync()
		os.Exit(1)
	}
	defer transport.cleanup()
	fsmv2Done := transport.done

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

// The seams below make main's startup order observable to tests without
// copying it: a test replaces a function variable with a probe, then drives
// the real startup path. Production keeps the real implementations assigned
// here, and the variables are unexported.

// runHook is the seam for starting the Management Console module, which the
// official image registers. It runs before the supervisor starts.
var runHook = managementconsole.Run

// connectionBufferSize is the capacity of the inbound and outbound channels
// between umh-core and the Management Console module.
const connectionBufferSize = 100

// newFSMv2Store is the seam for the store constructor. buildFSMv2Supervisor
// creates its store by calling it, so a test's constructor can keep the
// pointer it returns.
var newFSMv2Store = scenariorunner.SetupStore

// buildFSMv2SupervisorFn is the seam for the FSMv2 build: a test can inject a
// build that fails.
var buildFSMv2SupervisorFn = buildFSMv2Supervisor

// startFSMv2SupervisorFn is the seam for the supervisor start: a test can
// inject a start that records whether the supervisor has started.
var startFSMv2SupervisorFn = defaultStartFSMv2Supervisor

// wireFSMv2CommunicatorFn is the seam for the communicator wiring: a test can
// inject a no-op, so startup runs without the real communication state.
var wireFSMv2CommunicatorFn = wireFSMv2Communicator

// defaultStartFSMv2Supervisor runs the supervisor and closes done when it has
// fully stopped.
func defaultStartFSMv2Supervisor(appSup fsmv2Supervisor, ctx context.Context, done chan struct{}, log *zap.SugaredLogger) {
	go func() {
		defer close(done)

		if runErr := appSup.Run(ctx); runErr != nil {
			log.Errorw("FSMv2 supervisor Run error", "error", runErr)
		}
	}()
}

// fsmv2StartupResult carries what main needs from one attempt to build and
// start the FSMv2 application supervisor.
type fsmv2StartupResult struct {
	// done is closed when the FSMv2 supervisor has fully stopped, or at once
	// when it was never started.
	done chan struct{}
	// cleanup must run after the supervisor has stopped.
	cleanup func()
}

// startFSMv2Transport is main's FSMv2 startup path: run the registered
// startup functions, build and start the supervisor, and wire the
// communicator when backend credentials are present. It returns the first
// error it meets; nothing later in the sequence runs after one.
func startFSMv2Transport(
	ctx context.Context,
	configData *config.FullConfig,
	communicationState *communication_state.CommunicationState,
	log *zap.SugaredLogger,
	forceExit <-chan struct{},
) (fsmv2StartupResult, error) {
	done := make(chan struct{})

	// The instance UUID is a placeholder until the Management Console module
	// reports the one the relay returned at login.
	placeholderUUID := uuid.New().String()
	communicationState.SetLoginResponseForFSMv2(placeholderUUID)

	// inbound carries the messages users sent to this instance to the router.
	// outbound carries status messages and action replies to the module.
	inbound := make(chan handoff.Message, connectionBufferSize)
	outbound := make(chan handoff.Message, connectionBufferSize)

	mc, mcErr := runHook(managementconsole.Deps{
		Inbound:  inbound,
		Outbound: outbound,
		SetInstanceUUID: func(id string) {
			if isAuthenticatedUUID(id, placeholderUUID) {
				applyAuthenticatedUUID(id, communicationState)
			}
		},
		Config: managementconsole.Config{
			RelayURL:         configData.Agent.APIURL,
			AuthToken:        configData.Agent.AuthToken,
			InstanceUUID:     placeholderUUID,
			AllowInsecureTLS: configData.Agent.AllowInsecureTLS,
			Location:         configData.Agent.Location,
		},
		Logger: deps.NewFSMLogger(log),
	})
	if mcErr != nil {
		close(done)

		return fsmv2StartupResult{done: done, cleanup: func() {}}, fmt.Errorf("failed to start the Management Console module: %w", mcErr)
	}

	appSup, cleanup, buildErr := buildFSMv2SupervisorFn(ctx, configData, communicationState, log, forceExit, mc.Children)
	if buildErr != nil {
		close(done)

		return fsmv2StartupResult{done: done, cleanup: func() {}}, fmt.Errorf("failed to build FSMv2 supervisor: %w", buildErr)
	}

	startFSMv2SupervisorFn(appSup, ctx, done, log)

	if communicatorEnabled(configData.Agent) {
		log.Info("Starting FSMv2 communicator because backend credentials are present")
		wire := wireFSMv2CommunicatorFn
		status := mc.Status
		sentry.SafeGoWithContext(ctx, func(ctx context.Context) {
			wire(ctx, inbound, outbound, configData, communicationState, log, status)
		})
	} else {
		log.Info("No backend credentials configured; running FSMv2 runtime without communicator")
	}

	return fsmv2StartupResult{done: done, cleanup: cleanup}, nil
}

// communicatorEnabled reports whether the FSMv2 communicator can run, which
// requires backend credentials. It gates only wireFSMv2Communicator: the FSMv2
// runtime is built and run regardless of credentials, so this must never gate
// its construction or its Run call.
func communicatorEnabled(cfg config.AgentConfig) bool {
	return cfg.APIURL != "" && cfg.AuthToken != ""
}

// countHistorianBridges returns the number of bridges whose write DFC targets the historian output.
func countHistorianBridges(cfg config.FullConfig) int {
	n := 0

	for _, pc := range cfg.ProtocolConverter {
		if pc.ProtocolConverterServiceConfig.GetDFCWriteServiceConfig().WritesToHistorian() {
			n++
		}
	}

	return n
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

// snapshotFingerprint builds a sorted, deterministic string from the current snapshot
// state. Two fingerprints that are equal mean no meaningful change occurred.
func snapshotFingerprint(snapshot *fsm.SystemSnapshot, extractStatusReason func(managerName string, instance *fsm.FSMInstanceSnapshot) string) string {
	lines := make([]string, 0, 32)
	managerNames := make([]string, 0, len(snapshot.Managers))
	for name := range snapshot.Managers {
		managerNames = append(managerNames, name)
	}
	sort.Strings(managerNames)

	for _, managerName := range managerNames {
		manager := snapshot.Managers[managerName]
		if manager == nil {
			continue
		}
		instances := manager.GetInstances()

		instanceNames := make([]string, 0, len(instances))
		for name := range instances {
			instanceNames = append(instanceNames, name)
		}
		sort.Strings(instanceNames)

		for _, instanceName := range instanceNames {
			inst := instances[instanceName]
			if inst == nil {
				continue
			}
			reason := extractStatusReason(managerName, inst)
			lines = append(lines, fmt.Sprintf("%s|%s|%s|%s|%s",
				managerName, instanceName, inst.CurrentState, inst.DesiredState, reason))
		}

		lines = append(lines, fmt.Sprintf("%s|count|%d", managerName, len(instances)))
	}
	return strings.Join(lines, "\n")
}

// SystemSnapshotLogger logs the system snapshot every 5 seconds, but only when
// the state has meaningfully changed since the last log.
func SystemSnapshotLogger(ctx context.Context, controlLoop *control.ControlLoop) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	snap_logger := logger.For("SnapshotLogger")
	if snap_logger == nil {
		snap_logger = zap.NewNop().Sugar()
	}

	snap_logger.Info("Starting system snapshot logger")

	var lastFingerprint string

	extractStatusReason := func(managerName string, instance *fsm.FSMInstanceSnapshot) string {
		if instance.LastObservedState == nil {
			return ""
		}
		switch managerName {
		case "BenthosManagerCore":
			if s, ok := instance.LastObservedState.(*benthos.BenthosObservedStateSnapshot); ok {
				return s.ServiceInfo.BenthosStatus.StatusReason
			}
		case "StreamProcessorManagerCore":
			if s, ok := instance.LastObservedState.(*streamprocessor.ObservedStateSnapshot); ok {
				return s.ServiceInfo.StatusReason
			}
		case "TopicBrowserManagerCore":
			if s, ok := instance.LastObservedState.(*topicbrowserfsm.ObservedStateSnapshot); ok {
				return s.ServiceInfo.StatusReason
			}
		case "DataFlowCompManagerCore":
			if s, ok := instance.LastObservedState.(*dataflowcomponent.DataflowComponentObservedStateSnapshot); ok {
				return s.ServiceInfo.StatusReason
			}
		case "ProtocolConverterManagerCore":
			if s, ok := instance.LastObservedState.(*protocolconverter.ProtocolConverterObservedStateSnapshot); ok {
				return s.ServiceInfo.StatusReason
			}
		case "RedpandaManagerCore":
			if s, ok := instance.LastObservedState.(*redpanda.RedpandaObservedStateSnapshot); ok {
				return s.ServiceInfoSnapshot.StatusReason
			}
		}
		return ""
	}

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

			fingerprint := snapshotFingerprint(snapshot, extractStatusReason)
			if fingerprint == lastFingerprint {
				continue
			}
			lastFingerprint = fingerprint

			snap_logger.Infof("=== System Snapshot (Tick %d) - %d Managers ===",
				snapshot.Tick, len(snapshot.Managers))

			// Log manager information
			managerNames := make([]string, 0, len(snapshot.Managers))
			for name := range snapshot.Managers {
				managerNames = append(managerNames, name)
			}
			sort.Strings(managerNames)

			for _, managerName := range managerNames {
				manager := snapshot.Managers[managerName]
				if manager == nil {
					continue
				}
				instances := manager.GetInstances()

				if len(instances) == 0 {
					snap_logger.Infof("📁 %s (tick: %d) - No instances",
						managerName, manager.GetManagerTick())
				} else {
					snap_logger.Infof("📁 %s (tick: %d) - %d instance(s):",
						managerName, manager.GetManagerTick(), len(instances))

					instanceNames := make([]string, 0, len(instances))
					for name := range instances {
						instanceNames = append(instanceNames, name)
					}
					sort.Strings(instanceNames)

					for _, instanceName := range instanceNames {
						instance := instances[instanceName]
						if instance == nil {
							continue
						}
						statusReason := extractStatusReason(managerName, instance)

						stateIcon := "⚠"
						switch instance.CurrentState {
						case "active":
							stateIcon = "✅"
						case "stopped":
							stateIcon = "⏹"
						case "idle":
							stateIcon = "💤"
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

// fsmv2Supervisor is the minimal interface required from the ApplicationSupervisor
// by main().  Using an interface keeps buildFSMv2Supervisor decoupled from the
// concrete generic type and avoids repeating the full type parameter list.
type fsmv2Supervisor interface {
	metrics.FSMv2DebugProvider
	Run(ctx context.Context) error
}

// buildFSMv2Supervisor constructs the ApplicationSupervisor with the
// persistence child and the children the Management Console module returned.
// It returns a cleanup function that must be deferred by the caller to stop
// the SentryHook goroutine.
// The returned appSup has NOT been started yet; the caller is responsible for
// calling appSup.Run(ctx). Run detaches the tick loop from ctx and treats
// ctx cancellation (the first SIGTERM here) as the trigger for a graceful
// drain against the live loop (ENG-4971) — do not substitute Start(ctx),
// which ties the loop's lifetime to the caller's cancel.
func buildFSMv2Supervisor(
	ctx context.Context,
	configData *config.FullConfig,
	communicationState *communication_state.CommunicationState,
	logger *zap.SugaredLogger,
	forceExit <-chan struct{},
	registered []fsmv2config.ChildSpec,
) (appSup fsmv2Supervisor, cleanup func(), err error) {
	if configData == nil {
		return nil, func() {}, errors.New("config is nil, cannot build FSMv2 supervisor")
	}

	yamlConfig, err := renderSupervisorChildrenYAML(registered)
	if err != nil {
		return nil, func() {}, fmt.Errorf("failed to render FSMv2 supervisor children: %w", err)
	}

	// The historian monitor is a dynamic child, upserted and deleted at runtime
	// by the config worker's per-tick reconcile so live config edits apply
	// without restarting umh-core.

	// Setup store (in-memory for now).
	store := newFSMv2Store(deps.NewFSMLogger(logger))

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

	// The supervisor merges this map into every child, so each dependency
	// reaches its worker however deep it sits.
	fsmv2Deps := map[string]any{}
	fsmv2config.SetDependency(fsmv2Deps, persistenceWorker.StoreKey, store)

	// One secrets store per process; see package secrets.
	secretsStore := secrets.NewStore()
	fsmv2config.SetDependency(fsmv2Deps, secrets.StoreKey, secretsStore.Handle())

	// Publish the dynamicchildren registry under the configworker deps key and
	// the process-scoped fsmv2client before constructing the supervisor, so the
	// application worker's first CollectObservedState observes
	// RegistryConfigured=true (the configworker kernel child spawns and dynamic
	// child spawning is enabled) and FSMv1 components can reach the client via
	// fsmv2client.GetClient(). Both must be published before
	// NewApplicationSupervisor so the application worker's first COS sees the
	// registry; the cleanup clears both and must run after the supervisor has
	// stopped (the caller runs cleanup after appSup.Run returns).
	dynWriter := dynamicchildren.NewWriter()
	register.SetGlobalDeps[*dynamicchildren.Registry](configworker.WorkerTypeName, dynWriter.Registry())
	fsmv2client.SetClient(fsmv2client.NewFSMv2Client(dynWriter, store))

	// Publish the config manager the config worker polls each tick to reconcile
	// the historian monitor child (replacing the standalone watchConfig
	// goroutine). Published before NewApplicationSupervisor so the config
	// worker's first CollectObservedState sees it; cleared in cleanup and the
	// error path below alongside the other deps keys.
	// TODO(ENG-4400): remove this wiring once the config worker reads
	// config.yaml directly instead of polling the manager.
	register.SetGlobalDeps[config.ConfigManager](configworker.ConfigManagerDepsKey, communicationState.ConfigManager)

	// Published before NewApplicationSupervisor, like the keys above: the
	// config worker reads this key when it is constructed.
	register.SetGlobalDeps[bool](configworker.CPUEnabledDepsKey, configData.Agent.UseFSMv2CPU)

	appSup, err = application.NewApplicationSupervisor(application.SupervisorConfig{
		ID:           "application-fsmv2",
		Name:         "Application FSMv2",
		Store:        store,
		Logger:       deps.NewFSMLogger(fsmv2Logger),
		TickInterval: 100 * time.Millisecond,
		YAMLConfig:   yamlConfig,
		Dependencies: fsmv2Deps,
		ForceExit:    forceExit,
		// The production tree is 4 levels (application -> communicator ->
		// transport -> push/pull). The drain budget cascades base x subtree
		// height (see fsmv2/supervisor/doc.go,
		// "Graceful shutdown budget"), so a base of 2s bounds the worst-case
		// chain drain at 8s -- inside docker's default 10s SIGTERM grace, with
		// headroom for s6 teardown and redpanda disk sync. Healthy drains
		// complete in ticks and never touch the budget; this only shortens how
		// long a STUCK worker delays shutdown before its level warns and moves
		// on. The base propagates to every child supervisor unchanged.
		GracefulShutdownTimeout: 2 * time.Second,
	})
	if err != nil {
		fsmv2client.SetClient(nil)
		register.ClearGlobalDeps(configworker.WorkerTypeName)
		register.ClearGlobalDeps(configworker.ConfigManagerDepsKey)
		register.ClearGlobalDeps(configworker.CPUEnabledDepsKey)
		fsmv2Hook.Stop()

		return nil, func() {}, fmt.Errorf("failed to create FSMv2 supervisor: %w", err)
	}

	// Register supervisor for debug introspection (/debug/fsmv2 endpoint).
	metrics.RegisterFSMv2DebugProvider("application", appSup)

	cleanup = func() {
		// Clear the client and the configworker deps key after the supervisor
		// has stopped (the caller runs cleanup after appSup.Run returns).
		fsmv2client.SetClient(nil)
		register.ClearGlobalDeps(configworker.WorkerTypeName)
		register.ClearGlobalDeps(configworker.ConfigManagerDepsKey)
		register.ClearGlobalDeps(configworker.CPUEnabledDepsKey)
		fsmv2Hook.Stop()
	}

	return appSup, cleanup, nil
}

// wireFSMv2Communicator starts the subscriber handler and the router. The
// router reads the messages users sent to this instance from inbound. Status
// messages from the subscriber handler and the router's action replies go to
// outbound, where the Management Console module reads them. This function
// blocks until ctx is cancelled.
func wireFSMv2Communicator(
	ctx context.Context,
	inbound <-chan handoff.Message,
	outbound chan<- handoff.Message,
	configData *config.FullConfig,
	communicationState *communication_state.CommunicationState,
	logger *zap.SugaredLogger,
	status managementconsole.StatusFunc,
) {
	communicationState.InitialiseAndStartSubscriberHandler(
		5*time.Minute, // TTL: time until subscriber considered dead
		1*time.Minute, // Cull: cycle time to remove dead subscribers
		configData,
		communicationState.SystemSnapshotManager,
		communicationState.ConfigManager,
		outbound,
		status,
	)
	communicationState.InitializeRouterForFSMv2(inbound)

	go forwardReplies(ctx, communicationState.OutboundChannel, outbound)

	logger.Info("FSMv2 communicator wired, waiting for context cancellation")

	// Block until the caller's context is cancelled (e.g. main ctx cancelled).
	<-ctx.Done()

	logger.Info("FSMv2 communicator context cancelled")
}

// forwardReplies moves the router's action replies from replies to outbound
// until ctx is cancelled. A reply waits while outbound is full, because a
// dropped reply leaves the user's action without an answer.
func forwardReplies(ctx context.Context, replies <-chan *models.UMHMessage, outbound chan<- handoff.Message) {
	for {
		select {
		case <-ctx.Done():
			return
		case reply, ok := <-replies:
			if !ok {
				return
			}

			if reply == nil {
				continue
			}

			select {
			case outbound <- replyMessage(reply):
			case <-ctx.Done():
				return
			}
		}
	}
}

// replyMessage turns an action reply into the message the Management Console
// module sends: the encoded content, the user it goes to, and the trace ID.
func replyMessage(reply *models.UMHMessage) handoff.Message {
	m := handoff.Message{Content: []byte(reply.Content), SenderEmail: reply.Email}
	if reply.Metadata != nil && reply.Metadata.TraceID != uuid.Nil {
		m.TraceID = reply.Metadata.TraceID.String()
	}

	return m
}

// isAuthenticatedUUID reports whether observed is the instance UUID returned by
// the backend at login, rather than empty or the random placeholder.
func isAuthenticatedUUID(observed, placeholder string) bool {
	return observed != "" && observed != placeholder
}

// applyAuthenticatedUUID hands the backend's instance UUID to the communicator and to Sentry.
func applyAuthenticatedUUID(uuid string, cs *communication_state.CommunicationState) {
	cs.SetLoginResponseForFSMv2(uuid)
	sentry.SetInstanceUUID(uuid)
}
