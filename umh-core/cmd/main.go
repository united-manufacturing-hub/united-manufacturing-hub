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
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/pprof"
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

	fsmv2, err := startFSMv2Runtime(ctx, &configData, communicationState, log, forceExit)
	if err != nil {
		log.Errorw("FSMv2 startup failed", "error", err)
		_ = log.Sync()
		os.Exit(1)
	}
	defer fsmv2.cleanup()

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

	// Wait until the FSMv2 supervisor has stopped, so the process does not
	// exit before the supervisor drains.
	<-fsmv2.done

	log.Info("umh-core completed")
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
