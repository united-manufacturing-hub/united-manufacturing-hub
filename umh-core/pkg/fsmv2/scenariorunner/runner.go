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

package scenariorunner

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
)

// RunResult contains the result of running a scenario.
type RunResult struct {
	// Done closes when teardown is complete: the supervisor has stopped, its
	// cleanup ran, the root's registry is cleared, and the
	// store dump printed when DumpStore is set.
	Done <-chan struct{}
	// Shutdown starts teardown and blocks until Done closes.
	Shutdown func()
	// ShutdownClean is the root supervisor's DrainOutcomeClean. Read it after
	// Done closes.
	ShutdownClean bool

	// Err is nil, or the first failure postRunFailure found once teardown
	// finished. Read it after Done closes.
	Err error
}

// ErrScenarioFailed marks an error from Run for a scenario that started and
// then failed. An error that kept the scenario from starting does not wrap it.
var ErrScenarioFailed = errors.New("failed")

func scenarioFailed(name string, err error) error {
	return fmt.Errorf("scenario %q %w: %w", name, ErrScenarioFailed, err)
}

// Run executes a scenario (see runScenario). With DumpStore set, the store dump
// prints after teardown: before Done closes, or, when the scenario's Run
// fails, before Run returns the error.
func Run(ctx context.Context, cfg RunConfig) (*RunResult, error) {
	if cfg.Scenario.Run == nil {
		return nil, fmt.Errorf("scenario %q is not properly configured: Run is nil",
			cfg.Scenario.Name)
	}

	if cfg.Scenario.Name == "" {
		return nil, errors.New("scenario is not properly configured: " +
			"Run is set but Name is empty, so logs and the supervisor ID could not name the scenario")
	}

	if cfg.Root == nil {
		return nil, fmt.Errorf("scenario %q cannot start: RunConfig.Root is nil", cfg.Scenario.Name)
	}

	return runScenario(ctx, cfg)
}

// runScenario executes a scenario on the kernel-only application supervisor (no
// YAML children, so the config worker kernel is the only child).
//
// runScenario keeps the root's registry published for exactly the
// supervisor's lifetime: Root.Publish runs before the supervisor starts, and
// Root.Clear runs on EVERY exit path, including a Scenario.Run panic,
// strictly after the supervisor has stopped.
//
// The supervisor runs on a context detached from the caller's ctx. The
// caller's ctx drives Scenario.Run, the Duration wait, and the teardown
// trigger, but never the tick loop: if the tick loop shared the caller's
// ctx, cancelling it would stop ticking before Shutdown runs, and the
// graceful drain would wait out its full timeout against a stopped loop.
//
// Root.CheckIdle catches sequential overlap (a previous run whose teardown
// has not finished); it does not catch concurrent runScenario calls, because
// the check and the publish are two separate steps.
func runScenario(ctx context.Context, cfg RunConfig) (*RunResult, error) {
	if err := cfg.Root.CheckIdle(cfg.Scenario.Name); err != nil {
		return nil, err
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

	// Call Dependencies before Root.Publish, so a Dependencies error has nothing to clear.
	var scenarioDeps map[string]any

	releaseScenarioDeps := func() {}

	if cfg.Scenario.Dependencies != nil {
		depsMap, cleanup, err := cfg.Scenario.Dependencies()
		if err != nil {
			return nil, fmt.Errorf("scenario %q dependencies: %w", cfg.Scenario.Name, err)
		}

		scenarioDeps = depsMap

		if cleanup != nil {
			releaseScenarioDeps = cleanup
		}
	}

	client := cfg.Root.Publish(cfg.Store)

	// Built before the supervisor, so an error a worker logs on its first
	// tick also fails the run.
	recorder := &runRecorder{
		scenario:            cfg.Scenario.Name,
		expectedErrors:      slices.Clone(cfg.Scenario.ExpectedErrors),
		expectedErrorCauses: slices.Clone(cfg.Scenario.ExpectedErrorCauses),
		expectedWarnings:    slices.Clone(cfg.Scenario.ExpectedWarnings),
		matchedWarnings:     map[string]bool{},
		matchedErrors:       map[string]bool{},
		matchedCauses:       map[int]bool{},
	}
	runLogger := &recordingLogger{FSMLogger: cfg.Logger, recorder: recorder}

	appSup, err := cfg.Root.NewSupervisor(SupervisorConfig{
		ID:                      "scenario-" + cfg.Scenario.Name,
		Name:                    cfg.Scenario.Name,
		Store:                   cfg.Store,
		Logger:                  runLogger,
		TickInterval:            cfg.TickInterval,
		Dependencies:            scenarioDeps,
		EnableTraceLogging:      cfg.EnableTraceLogging,
		GracefulShutdownTimeout: cfg.GracefulShutdownTimeout,
	})
	if err != nil {
		cfg.Root.Clear()
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
		// Root.Clear strictly after supDone: clearing earlier changes what the
		// root's workers observe mid-shutdown.
		cfg.Root.Clear()
		releaseScenarioDeps()
	}

	printStoreDump := func() {
		if !cfg.DumpStore {
			return
		}

		// A cancelled caller ctx must not cut the dump short.
		dump, err := DumpScenario(context.Background(), cfg.Store, startSyncID)
		if err != nil {
			cfg.Logger.SentryWarn(deps.FeatureExamples, "", "scenario_dump_failed",
				deps.Err(err))

			return
		}

		fmt.Print(dump.FormatHuman())
	}

	// Until the teardown goroutine owns cleanup, every return below and a panic
	// in the scenario's Run tear down and print the dump here.
	teardownOwnedByGoroutine := false

	defer func() {
		if !teardownOwnedByGoroutine {
			teardown()
			printStoreDump()
		}
	}()

	if err := cfg.Scenario.Run(ctx, Env{Client: client, Logger: runLogger, Dependencies: scenarioDeps, recorder: recorder}); err != nil {
		return nil, scenarioFailed(cfg.Scenario.Name, err)
	}

	// Checked again: a Run that swallows a failed wait must still fail.
	if logged := recorder.loggedError(); logged != nil {
		return nil, scenarioFailed(cfg.Scenario.Name, logged)
	}

	cfg.Logger.Info("scenario_run_finished",
		deps.String("scenario", cfg.Scenario.Name))

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
			deps.String("scenario", cfg.Scenario.Name),
			deps.String("wake_reason", wakeReason))

		teardown()

		result.Err = postRunFailure(ctx, recorder, cfg.Store, cfg.ValidWorkerStates, cfg.Logger)

		printStoreDump()

		close(done)
	}()

	// Shutdown waits for Done: returning earlier would let this run's late
	// Root.Clear withdraw the registry the next run has just published.
	result.Shutdown = func() {
		appSup.Shutdown()
		<-done
	}

	return result, nil
}

const storedStateCheckTimeout = 10 * time.Second

// postRunFailure checks a finished run against its scenario's expectations
// and returns the first failure it finds: an error or warning the scenario
// did not expect, an expected entry the run never logged, or a stored
// worker state its type may not report.
func postRunFailure(ctx context.Context, recorder *runRecorder, store storage.TriangularStoreInterface, validStates map[string]map[string]bool, logger deps.FSMLogger) error {
	if err := recorder.loggedError(); err != nil {
		return err
	}

	if warn := recorder.loggedWarning(); warn != nil {
		return warn
	}

	if missing := recorder.missingExpectedEntries(); len(missing) > 0 {
		return errors.Join(missing...)
	}

	// WithoutCancel: a Ctrl+C after Run returned is not a failed store read.
	checkCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), storedStateCheckTimeout)
	defer cancel()

	return checkStoredWorkerStates(checkCtx, store, validStates, logger)
}

// SetupStore creates an in-memory TriangularStore for testing and CLI usage.
func SetupStore(logger deps.FSMLogger) storage.TriangularStoreInterface {
	basicStore := memory.NewInMemoryStore()

	return storage.NewTriangularStore(basicStore, logger)
}
