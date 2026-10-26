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
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	fsmv2config "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/cse/storage"
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
// It returns a cleanup function that the caller must run after the supervisor
// has stopped. Cleanup clears the globals published here and stops the
// Sentry hooks' goroutines.
// The returned supervisor has NOT been started yet; the caller is responsible
// for calling its Run(ctx). Run detaches the tick loop from ctx and treats
// ctx cancellation (the first SIGTERM here) as the trigger for a graceful
// drain against the live loop. Do not substitute Start(ctx), which ties the
// loop's lifetime to the caller's cancel.
func buildFSMv2Supervisor(
	configManager config.ConfigManager,
	cpuMonitorEnabled bool,
	logger *zap.SugaredLogger,
	forceExit <-chan struct{},
	registered []fsmv2config.ChildSpec,
) (fsmv2Supervisor, func(), error) {
	childrenYAML, renderErr := renderSupervisorChildrenYAML(registered)
	if renderErr != nil {
		return nil, func() {}, fmt.Errorf("failed to render FSMv2 supervisor children: %w", renderErr)
	}

	store := newFSMv2Store(deps.NewFSMLogger(logger))

	dependencies, dependenciesErr := supervisorDependencies(store, registered)
	if dependenciesErr != nil {
		return nil, func() {}, fmt.Errorf("failed to add the Management Console module's dependencies: %w", dependenciesErr)
	}

	sentryHook := fsmv2sentry.NewSentryHook(5 * time.Minute)
	supervisorLogger := withSentryHook(logger.Named("fsmv2"), sentryHook)

	// The config worker reads these globals when it is constructed.
	publishConfigWorkerGlobals(store, configManager, cpuMonitorEnabled)
	cleanup := func() {
		clearConfigWorkerGlobals()
		sentryHook.Stop()
		// The action handlers create their own hook on the first action, so
		// it can only be stopped at shutdown.
		actions.StopCommunicatorSentryHook()
	}

	applicationSupervisor, createErr := application.NewApplicationSupervisor(supervisorConfig(store, supervisorLogger, childrenYAML, dependencies, forceExit))
	if createErr != nil {
		cleanup()

		return nil, func() {}, fmt.Errorf("failed to create FSMv2 supervisor: %w", createErr)
	}

	// Register supervisor for debug introspection (/debug/fsmv2 endpoint).
	metrics.RegisterFSMv2DebugProvider("application", applicationSupervisor)

	return applicationSupervisor, cleanup, nil
}

// supervisorDependencies builds the map the supervisor merges into every
// child, so each dependency reaches its worker however deep it sits. It holds
// the store, the process's one secrets store, and the dependencies of the
// module's children as live values.
func supervisorDependencies(store storage.TriangularStoreInterface, registered []fsmv2config.ChildSpec) (map[string]any, error) {
	dependencies := map[string]any{}
	fsmv2config.SetDependency(dependencies, persistenceWorker.StoreKey, store)
	fsmv2config.SetDependency(dependencies, secrets.StoreKey, secrets.NewStore().Handle())

	if err := addChildDependencies(dependencies, registered); err != nil {
		return nil, err
	}

	return dependencies, nil
}

// withSentryHook returns logger with its errors also sent to Sentry through
// hook, which debounces them per fingerprint.
func withSentryHook(logger *zap.SugaredLogger, hook *fsmv2sentry.SentryHook) *zap.SugaredLogger {
	return logger.Desugar().WithOptions(zap.WrapCore(hook.Wrap)).Sugar()
}

// publishConfigWorkerGlobals publishes what the config worker and FSMv1
// components read from process globals: the dynamic children registry, the
// fsmv2 client, the config manager and the CPU monitor flag. The config worker
// polls the config manager each tick to reconcile the historian monitor
// child. It must run before NewApplicationSupervisor, so the application
// worker's first observation sees the registry.
//
// TODO: stop publishing the config manager once the config worker reads
// config.yaml directly instead of polling the manager.
func publishConfigWorkerGlobals(store storage.TriangularStoreInterface, configManager config.ConfigManager, cpuMonitorEnabled bool) {
	dynamicChildrenWriter := dynamicchildren.NewWriter()
	register.SetGlobalDeps[*dynamicchildren.Registry](configworker.WorkerTypeName, dynamicChildrenWriter.Registry())
	fsmv2client.SetClient(fsmv2client.NewFSMv2Client(dynamicChildrenWriter, store))
	register.SetGlobalDeps[config.ConfigManager](configworker.ConfigManagerDepsKey, configManager)
	register.SetGlobalDeps[bool](configworker.CPUEnabledDepsKey, cpuMonitorEnabled)
}

// clearConfigWorkerGlobals clears what publishConfigWorkerGlobals published.
// It must run after the supervisor has stopped.
func clearConfigWorkerGlobals() {
	fsmv2client.SetClient(nil)
	register.ClearGlobalDeps(configworker.WorkerTypeName)
	register.ClearGlobalDeps(configworker.ConfigManagerDepsKey)
	register.ClearGlobalDeps(configworker.CPUEnabledDepsKey)
}

// supervisorConfig is the ApplicationSupervisor's configuration.
func supervisorConfig(
	store storage.TriangularStoreInterface,
	logger *zap.SugaredLogger,
	childrenYAML string,
	dependencies map[string]any,
	forceExit <-chan struct{},
) application.SupervisorConfig {
	return application.SupervisorConfig{
		ID:           "application-fsmv2",
		Name:         "Application FSMv2",
		Store:        store,
		Logger:       deps.NewFSMLogger(logger),
		TickInterval: 100 * time.Millisecond,
		YAMLConfig:   childrenYAML,
		Dependencies: dependencies,
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
	}
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

	applicationSupervisor, cleanup, buildErr := buildFSMv2SupervisorFn(communicationState.ConfigManager, configData.Agent.UseFSMv2CPU, log, forceExit, managementConsole.Children)
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
