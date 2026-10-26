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
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	fsmv2config "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps/secrets"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/handoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/managementconsole"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/scenariorunner"
	fsmv2sentry "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/sentry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/communication_state"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker"
	persistenceWorker "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
)

// connectionBufferSize is the capacity of the inbound and outbound channels
// between umh-core and the Management Console module.
const connectionBufferSize = 100

// Test seams for main's startup order. A test replaces one with a probe and
// then drives the real startup path. Production never reassigns them.
var (
	startManagementConsoleConnection = managementconsole.Run
	newFSMv2Store                    = scenariorunner.SetupStore
	buildFSMv2SupervisorFn           = buildFSMv2Supervisor
	runFSMv2SupervisorFn             = runFSMv2SupervisorInBackground
	runRouterFn                      = runRouter
)

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

	// The module's children get their dependencies through this map, as live
	// values; the children document carries none.
	if err := addChildDependencies(fsmv2Deps, registered); err != nil {
		fsmv2Hook.Stop()

		return nil, func() {}, fmt.Errorf("failed to add the Management Console module's dependencies: %w", err)
	}

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
		// Each supervisor level may wait up to this base before it gives up on
		// a stuck child, so a full shutdown can take up to base x tree height
		// (see fsmv2/supervisor/doc.go, "Graceful shutdown budget"). The
		// height below the application depends on the children the Management
		// Console module returns, so umh-core does not set it. The base of 2s
		// assumes a tree of 4 levels, which gives at most 8s -- inside docker's
		// default 10s SIGTERM grace, with room for s6 teardown and the
		// redpanda disk sync. If the tree gets deeper, lower the base. Healthy drains
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

// runFSMv2SupervisorInBackground runs the supervisor in a goroutine and closes
// stopped when it has fully stopped.
func runFSMv2SupervisorInBackground(ctx context.Context, applicationSupervisor fsmv2Supervisor, stopped chan struct{}, log *zap.SugaredLogger) {
	go func() {
		defer close(stopped)

		if runErr := applicationSupervisor.Run(ctx); runErr != nil {
			log.Errorw("FSMv2 supervisor Run error", "error", runErr)
		}
	}()
}

// fsmv2Handle lets main wait for the FSMv2 supervisor to stop and then
// release what startFSMv2Runtime set up.
type fsmv2Handle struct {
	// stopped is closed when the FSMv2 supervisor has fully stopped.
	stopped chan struct{}
	// cleanup must run after stopped is closed.
	cleanup func()
}

// startFSMv2Runtime is main's FSMv2 startup path. It starts the connection to
// the Management Console if the Management Console module is linked in. It
// then builds and starts the supervisor, and runs the router when relay
// credentials are present. It returns the first error it meets; nothing later
// in the sequence runs after one.
func startFSMv2Runtime(
	ctx context.Context,
	configData *config.FullConfig,
	communicationState *communication_state.CommunicationState,
	log *zap.SugaredLogger,
	forceExit <-chan struct{},
) (fsmv2Handle, error) {
	// The instance UUID is a placeholder until the Management Console module
	// reports the one the relay returned at login.
	placeholderUUID := uuid.New().String()
	communicationState.SetLoginResponseForFSMv2(placeholderUUID)

	// inbound carries the messages users sent to this instance to the router.
	// outbound carries status messages and action replies to the module.
	inbound := make(chan handoff.Message, connectionBufferSize)
	outbound := make(chan handoff.Message, connectionBufferSize)

	moduleDeps := managementConsoleDeps(configData.Agent, placeholderUUID, inbound, outbound, communicationState, log)

	managementConsole, moduleErr := startManagementConsoleConnection(moduleDeps)
	if moduleErr != nil {
		return fsmv2Handle{}, fmt.Errorf("failed to start the Management Console module: %w", moduleErr)
	}

	applicationSupervisor, cleanup, buildErr := buildFSMv2SupervisorFn(ctx, configData, communicationState, log, forceExit, managementConsole.Children)
	if buildErr != nil {
		return fsmv2Handle{}, fmt.Errorf("failed to build FSMv2 supervisor: %w", buildErr)
	}

	stopped := make(chan struct{})
	runFSMv2SupervisorFn(ctx, applicationSupervisor, stopped, log)

	startRouterIfConfigured(ctx, inbound, outbound, configData, communicationState, log, managementConsole.Status)

	return fsmv2Handle{stopped: stopped, cleanup: cleanup}, nil
}

// managementConsoleDeps builds what the Management Console module gets at
// start. The module reports the relay's instance UUID through
// SetInstanceUUID; the placeholder and empty values are ignored.
func managementConsoleDeps(
	agent config.AgentConfig,
	placeholderUUID string,
	inbound chan handoff.Message,
	outbound chan handoff.Message,
	communicationState *communication_state.CommunicationState,
	log *zap.SugaredLogger,
) managementconsole.Deps {
	return managementconsole.Deps{
		Inbound:  inbound,
		Outbound: outbound,
		SetInstanceUUID: func(id string) {
			if isAuthenticatedUUID(id, placeholderUUID) {
				applyAuthenticatedUUID(id, communicationState)
			}
		},
		Config: managementconsole.Config{
			RelayURL:         agent.APIURL,
			AuthToken:        agent.AuthToken,
			InstanceUUID:     placeholderUUID,
			AllowInsecureTLS: agent.AllowInsecureTLS,
			Location:         agent.Location,
		},
		Logger: deps.NewFSMLogger(log),
	}
}

// startRouterIfConfigured runs the router in the background when relay
// credentials are set, and only logs that it is skipped otherwise.
func startRouterIfConfigured(
	ctx context.Context,
	inbound <-chan handoff.Message,
	outbound chan<- handoff.Message,
	configData *config.FullConfig,
	communicationState *communication_state.CommunicationState,
	log *zap.SugaredLogger,
	status managementconsole.StatusFunc,
) {
	if !hasRelayCredentials(configData.Agent) {
		log.Info("API_URL or AUTH_TOKEN is not set; running FSMv2 without the router")

		return
	}

	log.Info("API_URL and AUTH_TOKEN are set; starting the router for Management Console messages")

	// Read the seam now: tests reassign it after startup returns.
	run := runRouterFn
	sentry.SafeGoWithContext(ctx, func(ctx context.Context) {
		run(ctx, inbound, outbound, configData, communicationState, log, status)
	})
}

// hasRelayCredentials reports whether both the relay URL and the auth token
// are set. It gates only runRouter. The FSMv2 supervisor is built and run
// whether or not credentials are set, so this check must never gate
// buildFSMv2Supervisor or the supervisor's Run call.
func hasRelayCredentials(cfg config.AgentConfig) bool {
	return cfg.APIURL != "" && cfg.AuthToken != ""
}
