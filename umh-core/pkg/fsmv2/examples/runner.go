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

	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/certfetcher"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/certfetcher/state"
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
type RunResult struct {
	// Done closes when teardown is complete: the supervisor has stopped, its
	// cleanup ran, the store dump printed when DumpStore is set, and the
	// published configworker deps key is cleared. The cleared key is what
	// makes back-to-back runs in one process safe.
	Done <-chan struct{}
	// Shutdown initiates teardown and blocks until Done closes, so the deps
	// key is already cleared when the caller starts the next run.
	Shutdown func()
	// ShutdownClean is the root supervisor's DrainOutcomeClean. Read it after
	// Done closes.
	ShutdownClean bool

	// Err is nil, or the first failure postRunFailure found once teardown
	// finished. Read it after Done closes.
	Err error
}

// ErrScenarioFailed marks an error from Run for a v2 scenario that started and
// then failed. An error that kept the scenario from starting does not wrap it.
var ErrScenarioFailed = errors.New("failed")

func scenarioFailed(name string, err error) error {
	return fmt.Errorf("scenario %q %w: %w", name, ErrScenarioFailed, err)
}

// Run executes a v2 scenario with the given configuration (see runV2).
//
// The supervisor's tick loop runs on a context detached from ctx: cancelling
// ctx triggers a graceful teardown against the live tick loop instead of
// killing the loop and forcing the drain to wait out its timeouts.
//
// If DumpStore is set, Run prints the store dump after teardown, before Done
// closes. When the scenario's Run fails, the dump is printed before Run
// returns its error.
func Run(ctx context.Context, cfg RunConfig) (*RunResult, error) {
	if cfg.ScenarioV2.Run == nil {
		return nil, fmt.Errorf("v2 scenario %q is not properly configured: Run is nil",
			cfg.ScenarioV2.Name)
	}

	if cfg.ScenarioV2.Name == "" {
		return nil, errors.New("v2 scenario is not properly configured: " +
			"Run is set but Name is empty, so logs and the supervisor ID could not name the scenario")
	}

	return runV2(ctx, cfg)
}

// runV2 executes a v2 scenario on the kernel-only application supervisor (no
// YAML children, so the config worker kernel is the only child).
//
// runV2 keeps the process-global configworker deps key published for exactly
// the supervisor's lifetime: the dynamicchildren registry is published under
// the key before the supervisor starts (the application worker reads it every
// tick), and the key is cleared on EVERY exit path, including a
// ScenarioV2.Run panic, strictly after the supervisor has stopped. Clearing the key earlier flips
// the application worker's RegistryConfigured observation mid-shutdown; a key
// that is never cleared makes every later runV2 in the same process fail its
// already-published check below.
//
// The supervisor runs on a context detached from the caller's ctx. The
// caller's ctx drives ScenarioV2.Run, the Duration wait, and the teardown
// trigger, but never the tick loop: if the tick loop shared the caller's
// ctx, cancelling it would stop ticking before Shutdown runs, and the
// graceful drain would wait out its full timeout against a stopped loop.
//
// Because the deps key is process-global, v2 runs must not overlap within a
// process. The already-published check below catches sequential overlap (a
// previous run whose teardown has not finished); it does not catch truly
// concurrent runV2 calls, because the check and the publish are two separate
// lock acquisitions. Concurrent runV2 calls are not supported.
func runV2(ctx context.Context, cfg RunConfig) (*RunResult, error) {
	if register.GlobalDeps[*dynamicchildren.Registry](configworker.WorkerTypeName) != nil {
		return nil, fmt.Errorf("v2 scenario %q cannot start: the configworker deps key is already published, "+
			"so another v2 run is still active in this process", cfg.ScenarioV2.Name)
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

	// Call Dependencies before publishing the deps key, so a Dependencies error has nothing to clear.
	var scenarioDeps map[string]any

	releaseScenarioDeps := func() {}

	if cfg.ScenarioV2.Dependencies != nil {
		depsMap, cleanup, err := cfg.ScenarioV2.Dependencies()
		if err != nil {
			return nil, fmt.Errorf("scenario %q dependencies: %w", cfg.ScenarioV2.Name, err)
		}

		scenarioDeps = depsMap

		if cleanup != nil {
			releaseScenarioDeps = cleanup
		}
	}

	writer := dynamicchildren.NewWriter()
	register.SetGlobalDeps[*dynamicchildren.Registry](configworker.WorkerTypeName, writer.Registry())

	// Built before the supervisor, so an error a worker logs on its first
	// tick also fails the run.
	recorder := &runRecorder{
		scenario:            cfg.ScenarioV2.Name,
		expectedErrors:      cfg.ScenarioV2.ExpectedErrors,
		expectedErrorCauses: cfg.ScenarioV2.ExpectedErrorCauses,
		expectedWarnings:    cfg.ScenarioV2.ExpectedWarnings,
	}
	runLogger := &recordingLogger{FSMLogger: cfg.Logger, recorder: recorder}

	appSup, err := application.NewApplicationSupervisor(application.SupervisorConfig{
		ID:                      "scenariov2-" + cfg.ScenarioV2.Name,
		Name:                    cfg.ScenarioV2.Name,
		Store:                   cfg.Store,
		Logger:                  runLogger,
		TickInterval:            cfg.TickInterval,
		Dependencies:            scenarioDeps,
		EnableTraceLogging:      cfg.EnableTraceLogging,
		GracefulShutdownTimeout: cfg.GracefulShutdownTimeout,
	})
	if err != nil {
		register.ClearGlobalDeps(configworker.WorkerTypeName)
		releaseScenarioDeps()

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
		// ClearGlobalDeps strictly after supDone: clearing earlier flips the
		// application worker's RegistryConfigured observation mid-shutdown.
		register.ClearGlobalDeps(configworker.WorkerTypeName)
		releaseScenarioDeps()
	}

	// ScenarioV2.Run is user-authored code, so it may return an error or panic.
	// Either way the supervisor must stop and the deps key must be cleared
	// before runV2's frame unwinds, otherwise every later runV2 in this
	// process fails its already-published check. The flag stays false until
	// the teardown goroutine takes ownership of cleanup.
	// printStoreDump prints the store's changes since the run started when
	// DumpStore is set. It runs on a fresh context, so a caller cancelling
	// after Run returned cannot fail the store read.
	printStoreDump := func() {
		if !cfg.DumpStore {
			return
		}

		dump, err := DumpScenario(context.Background(), cfg.Store, startSyncID)
		if err != nil {
			cfg.Logger.SentryWarn(deps.FeatureExamples, "", "scenario_dump_failed",
				deps.Err(err))

			return
		}

		fmt.Print(dump.FormatHuman())
	}

	teardownOwnedByGoroutine := false

	// A failed Run prints the dump here, because a failed run is when the
	// dump is most useful.
	defer func() {
		if !teardownOwnedByGoroutine {
			teardown()
			printStoreDump()
		}
	}()

	client := fsmv2client.NewFSMv2Client(writer, cfg.Store)
	if err := cfg.ScenarioV2.Run(ctx, Env{Client: client, Logger: runLogger, Dependencies: scenarioDeps, recorder: recorder}); err != nil {
		return nil, scenarioFailed(cfg.ScenarioV2.Name, err)
	}

	// Checked again: a Run that swallows a failed wait must still fail.
	if logged := recorder.loggedError(); logged != nil {
		return nil, scenarioFailed(cfg.ScenarioV2.Name, logged)
	}

	cfg.Logger.Info("scenario_run_finished",
		deps.String("scenario", cfg.ScenarioV2.Name))

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

		result.Err = postRunFailure(ctx, recorder, cfg.Store, cfg.Logger)

		printStoreDump()

		close(done)
	}()

	// Shutdown waits for Done: returning earlier would let this run's late
	// ClearGlobalDeps delete the key the next v2 run has just stored.
	result.Shutdown = func() {
		appSup.Shutdown()
		<-done
	}

	return result, nil
}

const storedStateCheckTimeout = 10 * time.Second

func postRunFailure(ctx context.Context, recorder *runRecorder, store storage.TriangularStoreInterface, logger deps.FSMLogger) error {
	if err := recorder.loggedError(); err != nil {
		return err
	}

	if warn := recorder.loggedWarning(); warn != nil {
		return warn
	}

	// WithoutCancel: a Ctrl+C after Run returned is not a failed store read.
	checkCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), storedStateCheckTimeout)
	defer cancel()

	return checkStoredWorkerStates(checkCtx, store, logger)
}

// SetupStore creates an in-memory TriangularStore for testing and CLI usage.
func SetupStore(logger deps.FSMLogger) storage.TriangularStoreInterface {
	basicStore := memory.NewInMemoryStore()

	return storage.NewTriangularStore(basicStore, logger)
}
