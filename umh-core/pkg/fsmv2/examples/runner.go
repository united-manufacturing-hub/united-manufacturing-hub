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

package examples

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"

	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/state"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild/state"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplefailing"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplefailing/state"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplepanic"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplepanic/state"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent/state"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleslow"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleslow/state"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld/state"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/pull"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/pull/state"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push/state"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/state"
)

// RunResult contains the result of running a scenario.
//
// The two fields carry different guarantees per scenario form. Done closes
// when teardown is complete: for a v1 scenario that is when the supervisor
// has stopped and its cleanup ran (plus the dump when DumpStore is set); for
// a v2 scenario it additionally includes clearing the published configworker
// deps key, which is what makes back-to-back v2 runs safe. Shutdown initiates
// teardown: the v1 Shutdown does not wait for Done (the DumpStore summary may
// still be printing when it returns), while the v2 Shutdown blocks until Done
// so the deps key is already cleared when it returns.
type RunResult struct {
	Done     <-chan struct{}
	Shutdown func()
	// ShutdownClean reports whether the run's supervisor drained cleanly: true
	// if the most recent graceful shutdown reaped every worker within its
	// budget, or if no graceful shutdown ran (the v1 path). It is false only
	// when a drain phase warned graceful_shutdown_timeout or
	// graceful_shutdown_budget_exhausted. Read it after Done closes.
	ShutdownClean bool
}

// Run executes a scenario with the given configuration.
//
// For a v1 Scenario, creates an ApplicationSupervisor with the scenario's
// YAML config, or delegates to CustomRunner if set. For a ScenarioV2 (Driver
// set), takes the kernel-only v2 path (see runV2). Exactly one of Scenario
// and ScenarioV2 may be populated; setting both is an error.
//
// On both the YAML and v2 paths the supervisor's tick loop runs on a context
// detached from ctx: cancelling ctx triggers a graceful teardown against the
// live tick loop instead of killing the loop and forcing the drain to wait
// out its timeouts. CustomRunner scenarios own their supervisor lifecycle.
//
// If DumpStore is enabled, the YAML path prints a store changes summary
// after the run. The v2 path does not support DumpStore yet: runV2 logs a
// warning and ignores it. CustomRunner scenarios receive cfg.DumpStore and
// are responsible for their own dump handling; Run does not dump for them.
func Run(ctx context.Context, cfg RunConfig) (*RunResult, error) {
	hasYAML := cfg.Scenario.YAMLConfig != ""
	hasCustom := cfg.Scenario.CustomRunner != nil
	hasV1 := hasYAML || hasCustom
	hasV2 := cfg.ScenarioV2.Driver != nil || cfg.ScenarioV2.Name != ""

	if hasV1 && hasV2 {
		return nil, fmt.Errorf("conflicting configuration: both Scenario %q and ScenarioV2 %q are set (only one allowed)",
			cfg.Scenario.Name, cfg.ScenarioV2.Name)
	}

	if hasV2 && cfg.ScenarioV2.Driver == nil {
		return nil, fmt.Errorf("v2 scenario %q is not properly configured: Name is set but Driver is nil",
			cfg.ScenarioV2.Name)
	}

	if cfg.ScenarioV2.Driver != nil && cfg.ScenarioV2.Name == "" {
		return nil, errors.New("v2 scenario is not properly configured: " +
			"Driver is set but Name is empty, so logs and the supervisor ID could not name the scenario")
	}

	if cfg.ScenarioV2.Driver != nil {
		return runV2(ctx, cfg)
	}

	if !hasYAML && !hasCustom {
		return nil, fmt.Errorf("scenario %q is not properly configured: "+
			"neither YAMLConfig nor CustomRunner is set", cfg.Scenario.Name)
	}

	if hasYAML && hasCustom {
		return nil, fmt.Errorf("scenario %q has conflicting configuration: "+
			"both YAMLConfig and CustomRunner are set (only one allowed)", cfg.Scenario.Name)
	}

	if hasCustom {
		return cfg.Scenario.CustomRunner(ctx, cfg)
	}

	var startSyncID int64

	if cfg.DumpStore {
		var err error

		startSyncID, err = cfg.Store.GetLatestSyncID(ctx)
		if err != nil {
			cfg.Logger.SentryWarn(deps.FeatureExamples, "", "sync_id_fetch_failed",
				deps.Err(err),
				deps.String("impact", "dump_shows_all_changes"))
		}
	}

	appSup, err := application.NewApplicationSupervisor(application.SupervisorConfig{
		ID:                 "scenario-" + cfg.Scenario.Name,
		Name:               cfg.Scenario.Name,
		Store:              cfg.Store,
		Logger:             cfg.Logger,
		TickInterval:       cfg.TickInterval,
		YAMLConfig:         cfg.Scenario.YAMLConfig,
		EnableTraceLogging: cfg.EnableTraceLogging,
	})
	if err != nil {
		return nil, err
	}

	// Detached from the caller's ctx so cancelling the caller's ctx triggers
	// teardown (via the watcher below) instead of killing the tick loop;
	// Shutdown cancels the supervisor's own derived context in its final phase.
	supDone := appSup.Start(context.WithoutCancel(ctx))

	done := make(chan struct{})

	// Watcher: turns caller-ctx cancellation into a graceful teardown against
	// the LIVE tick loop. Shutdown runs unconditionally on both arms because
	// it is idempotent, and a supervisor that stopped on its own still needs
	// its executor and collectors stopped.
	go func() {
		select {
		case <-ctx.Done():
		case <-supDone:
		}

		appSup.Shutdown()
		<-supDone
		close(done)
	}()

	shutdownFn := func() {
		appSup.Shutdown()
	}

	if cfg.DumpStore {
		wrappedDone := make(chan struct{})

		go func() {
			<-done

			dumpCtx := context.Background()

			dump, err := DumpScenario(dumpCtx, cfg.Store, startSyncID)
			if err != nil {
				cfg.Logger.SentryWarn(deps.FeatureExamples, "", "scenario_dump_failed",
					deps.Err(err))
			} else {
				fmt.Print(dump.FormatHuman())
			}

			close(wrappedDone)
		}()

		return &RunResult{Done: wrappedDone, Shutdown: shutdownFn, ShutdownClean: true}, nil
	}

	return &RunResult{Done: done, Shutdown: shutdownFn, ShutdownClean: true}, nil
}

// runV2 executes a v2 scenario on the kernel-only application supervisor (no
// YAML children, so the config worker kernel is the only child).
//
// runV2 keeps the process-global configworker deps key published for exactly
// the supervisor's lifetime: the dynamicchildren registry is published under
// the key before the supervisor starts (the application worker reads it every
// tick), and the key is cleared on EVERY exit path, including a Driver panic,
// strictly after the supervisor has stopped. Clearing the key earlier flips
// the application worker's RegistryConfigured observation mid-shutdown; a key
// that is never cleared makes every later runV2 in the same process fail its
// already-published check below.
//
// The supervisor runs on a context detached from the caller's ctx. The
// caller's ctx drives the Driver, the Duration wait, and the teardown
// trigger, but never the tick loop: if the tick loop shared the caller's
// ctx, cancelling it would stop ticking before Shutdown runs, and the
// graceful drain would wait out its full timeout against a stopped loop.
//
// After the Driver returns nil, the runner waits RunConfig.Duration (or
// until ctx is cancelled; 0 means ctx-only), then shuts the supervisor down.
//
// Because the deps key is process-global, v2 runs must not overlap within a
// process. The already-published check below catches sequential overlap (a
// previous run whose teardown has not finished); it does not catch truly
// concurrent runV2 calls, because the check and the publish are two separate
// lock acquisitions. Concurrent runV2 calls are not supported.
func runV2(ctx context.Context, cfg RunConfig) (*RunResult, error) {
	if register.GetDeps[*dynamicchildren.Registry](configworker.WorkerTypeName) != nil {
		return nil, fmt.Errorf("v2 scenario %q cannot start: the configworker deps key is already published, "+
			"so another v2 run is still active in this process", cfg.ScenarioV2.Name)
	}

	if cfg.DumpStore {
		cfg.Logger.SentryWarn(deps.FeatureExamples, "", "dump_store_not_supported_for_v2",
			deps.String("scenario", cfg.ScenarioV2.Name),
			deps.String("impact", "no_store_dump_printed"))
	}

	writer := dynamicchildren.NewWriter()
	register.SetDeps[*dynamicchildren.Registry](configworker.WorkerTypeName, writer.Registry())

	appSup, err := application.NewApplicationSupervisor(application.SupervisorConfig{
		ID:                 "scenariov2-" + cfg.ScenarioV2.Name,
		Name:               cfg.ScenarioV2.Name,
		Store:              cfg.Store,
		Logger:             cfg.Logger,
		TickInterval:       cfg.TickInterval,
		EnableTraceLogging: cfg.EnableTraceLogging,
	})
	if err != nil {
		register.ClearDeps(configworker.WorkerTypeName)

		return nil, err
	}

	// Detached from the caller's ctx so cancelling the caller's ctx triggers
	// teardown (via the selects below) instead of killing the tick loop;
	// Shutdown cancels the supervisor's own derived context in its final phase.
	supDone := appSup.Start(context.WithoutCancel(ctx))

	// result is updated by the teardown goroutine before it closes done, so a
	// caller that reads result.ShutdownClean after <-result.Done observes the
	// supervisor's drain outcome. The close(done) at the end of the goroutine
	// establishes the happens-before edge: the field write precedes the close,
	// and the caller's receive synchronizes-with it.
	result := &RunResult{}

	// teardown is the single cleanup path: Shutdown is idempotent, so every
	// exit calls it unconditionally rather than guessing whether the
	// supervisor already stopped.
	teardown := func() {
		appSup.Shutdown()
		<-supDone
		// DrainOutcomeClean is valid only after supDone: the drain budget is
		// spent during Shutdown's synchronous phases, which complete before
		// the tick loop signals supDone.
		result.ShutdownClean = appSup.DrainOutcomeClean()
		// ClearDeps strictly after supDone: clearing earlier flips the
		// application worker's RegistryConfigured observation mid-shutdown.
		register.ClearDeps(configworker.WorkerTypeName)
	}

	// The Driver is user-authored code, so it may return an error or panic.
	// Either way the supervisor must stop and the deps key must be cleared
	// before runV2's frame unwinds, otherwise every later runV2 in this
	// process fails its already-published check. The flag stays false until
	// the teardown goroutine takes ownership of cleanup.
	teardownOwnedByGoroutine := false

	defer func() {
		if !teardownOwnedByGoroutine {
			teardown()
		}
	}()

	client := fsmv2client.NewFSMv2Client(writer, cfg.Store)
	if err := cfg.ScenarioV2.Driver(ctx, Env{Client: client, Logger: cfg.Logger}); err != nil {
		return nil, fmt.Errorf("scenario %q driver failed: %w", cfg.ScenarioV2.Name, err)
	}

	teardownOwnedByGoroutine = true

	done := make(chan struct{})
	result.Done = done

	go func() {
		// The select only decides the wake-up reason; teardown then runs
		// unconditionally, because skipping Shutdown on any arm would skip
		// the supervisor's synchronous cleanup phases. The reason is logged
		// so a supervisor that stopped on its own (supDone) is visible in
		// the run output instead of looking like a clean Duration run.
		var wakeReason string

		if cfg.Duration > 0 {
			select {
			case <-time.After(cfg.Duration):
				wakeReason = "duration_elapsed"
			case <-ctx.Done():
				wakeReason = "ctx_cancelled"
			case <-supDone:
				wakeReason = "supervisor_stopped"
			}
		} else {
			select {
			case <-ctx.Done():
				wakeReason = "ctx_cancelled"
			case <-supDone:
				wakeReason = "supervisor_stopped"
			}
		}

		cfg.Logger.Info("v2_run_teardown_starting",
			deps.String("scenario", cfg.ScenarioV2.Name),
			deps.String("wake_reason", wakeReason))

		teardown()
		close(done)
	}()

	// Shutdown waits for Done so the deps key is already cleared when the
	// caller starts the next v2 run; returning earlier would let this run's
	// late ClearDeps delete the next run's freshly published key.
	result.Shutdown = func() {
		appSup.Shutdown()
		<-done
	}

	return result, nil
}

// SetupStore creates an in-memory TriangularStore for testing and CLI usage.
func SetupStore(logger deps.FSMLogger) storage.TriangularStoreInterface {
	basicStore := memory.NewInMemoryStore()

	return storage.NewTriangularStore(basicStore, logger)
}
