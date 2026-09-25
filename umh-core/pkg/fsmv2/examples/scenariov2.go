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
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
)

// Env is what a scenario's Run receives. It has no store handle and no
// supervisor handle, so a check reads only what a user of the client could read.
type Env struct {
	// Client is the migration-API client wired to the run's dynamicchildren
	// Writer and store.
	Client *fsmv2client.FSMv2Client

	// Logger wraps the logger RunConfig.Logger carries, so every error a Run
	// logs feeds the run's error checks while all output still flows to the
	// underlying logger.
	Logger deps.FSMLogger

	// Dependencies is the map the scenario's Dependencies returned, or nil when
	// the scenario declares none. The supervisor reads the same map while it
	// builds workers, so Run must not write to it: Run reads a mock out with
	// config.LookupDependency and changes the mock itself.
	Dependencies map[string]any

	// recorder carries the per-run state that Step and WaitFor share. Run
	// receives Env by value, so the state lives behind a pointer; the runner
	// sets it before a scenario's Run function sees this Env.
	recorder *runRecorder
}

// runRecorder holds the per-run state that Step and WaitFor share across the
// value copies of Env.
type runRecorder struct {
	// scenario is the name of the run's scenario, so a step line can be
	// attributed when several scenarios run in one process.
	scenario string

	// lastStep is the description of the last change Step announced.
	lastStep string

	// expectedErrors lists the message substrings the run's scenario
	// declared through ScenarioV2.ExpectedErrors.
	expectedErrors []string

	// expectedWarnings lists the message substrings the run's scenario
	// declared through ScenarioV2.ExpectedWarnings.
	expectedWarnings []string

	// loggedMu guards loggedErr and loggedWarn, because the supervisor's tick
	// loop logs on its own goroutines while WaitFor, the runner's post-Run
	// check and the teardown goroutine's end-of-run check read them. scenario,
	// lastStep, expectedErrors and expectedWarnings carry no lock: the runner
	// sets each of them once, before the supervisor starts, and nothing
	// changes them afterwards.
	loggedMu sync.Mutex

	// loggedErr is the first error the run's logger reported at error level
	// and the scenario does not expect, or nil when none was logged. Both
	// checks act on that first error, so later ones are not kept.
	loggedErr error

	// loggedWarn is the first warning the run's logger reported and the
	// scenario does not expect, or nil when none was logged. Only the first
	// warning is kept.
	loggedWarn error
}

// alwaysAllowedErrors lists the message substrings every run may log at error
// level without failing: the collector reports these while it does its job,
// so no scenario should have to declare them.
var alwaysAllowedErrors = []string{
	"data_stale",
	"collector_observation_failed",
	"collector_stop_skipped",
}

// recordLoggedError stores one error a SentryError call logged during the run,
// shaped so a wait or run failure names the logged message. An error whose
// message matches a substring the scenario expects, or one every run allows,
// is not stored: it does not fail the run.
func (r *runRecorder) recordLoggedError(err error, msg string) {
	if r.errorAllowed(msg) {
		return
	}

	r.loggedMu.Lock()
	defer r.loggedMu.Unlock()

	if r.loggedErr == nil {
		r.loggedErr = fmt.Errorf("the scenario does not expect this error: %s (%v)", msg, err)
	}
}

// errorAllowed reports whether a logged error's message contains a substring
// the scenario declared in ExpectedErrors, or one of the messages every run
// allows.
func (r *runRecorder) errorAllowed(msg string) bool {
	for _, substr := range r.expectedErrors {
		if strings.Contains(msg, substr) {
			return true
		}
	}

	for _, substr := range alwaysAllowedErrors {
		if strings.Contains(msg, substr) {
			return true
		}
	}

	return false
}

// recordLoggedWarning stores one warning a SentryWarn call logged during the
// run. A warning whose message matches a substring the scenario declared in
// ExpectedWarnings, or one every run allows, is not stored.
func (r *runRecorder) recordLoggedWarning(msg string) {
	if r.warningAllowed(msg) {
		return
	}

	r.loggedMu.Lock()
	defer r.loggedMu.Unlock()

	if r.loggedWarn == nil {
		r.loggedWarn = fmt.Errorf("the scenario does not expect this warning: %s", msg)
	}
}

// warningAllowed reports whether a logged warning's message contains a
// substring the scenario declared in ExpectedWarnings, or one of the
// messages every run allows.
func (r *runRecorder) warningAllowed(msg string) bool {
	for _, substr := range r.expectedWarnings {
		if strings.Contains(msg, substr) {
			return true
		}
	}

	for _, substr := range alwaysAllowedErrors {
		if strings.Contains(msg, substr) {
			return true
		}
	}

	return false
}

// loggedWarning returns the first warning the run logged and the scenario
// does not expect, or nil when none was logged.
func (r *runRecorder) loggedWarning() error {
	r.loggedMu.Lock()
	defer r.loggedMu.Unlock()

	return r.loggedWarn
}

// loggedError returns the first error the run logged at error level and the
// scenario does not expect, or nil when none was logged.
func (r *runRecorder) loggedError() error {
	r.loggedMu.Lock()
	defer r.loggedMu.Unlock()

	return r.loggedErr
}

// runErrorLogger wraps the run's logger so every error the run logs, from
// the scenario's own code or from the supervisor's workers, reaches the
// recorder, while all output still flows to the underlying logger.
type runErrorLogger struct {
	deps.FSMLogger
	recorder *runRecorder
}

// SentryError records the error for the run's checks before delegating, so
// both the wait and the post-Run check can fail on it.
func (l *runErrorLogger) SentryError(feature deps.Feature, hierarchyPath string, err error, msg string, fields ...deps.Field) {
	l.recorder.recordLoggedError(err, msg)

	l.FSMLogger.SentryError(feature, hierarchyPath, err, msg, fields...)
}

// SentryWarn records the warning for the run's end-of-run check before
// delegating, so a warning the scenario does not expect reaches RunResult.Err.
func (l *runErrorLogger) SentryWarn(feature deps.Feature, hierarchyPath string, msg string, fields ...deps.Field) {
	l.recorder.recordLoggedWarning(msg)

	l.FSMLogger.SentryWarn(feature, hierarchyPath, msg, fields...)
}

// With wraps again, so a logger carrying context fields records too.
func (l *runErrorLogger) With(fields ...deps.Field) deps.FSMLogger {
	return &runErrorLogger{FSMLogger: l.FSMLogger.With(fields...), recorder: l.recorder}
}

// waitForPollInterval is how long WaitFor waits between two polls of its
// check.
const waitForPollInterval = 50 * time.Millisecond

// Step logs one line naming the change the scenario is about to make, and
// remembers it, so a later failed wait can name the change it followed.
func (e Env) Step(description string) {
	e.recorder.lastStep = description

	e.Logger.Info("scenario_step",
		deps.String("scenario", e.recorder.scenario),
		deps.String("step", description))
}

// WaitFor polls check until it reports done or ctx ends. check returns what it
// last saw, so a failure can say it. An error the run logged before or during
// the wait fails it on the next poll, before the check runs again. When ctx
// ends first, the returned error names the last Step, the check and the last
// value seen.
func (e Env) WaitFor(ctx context.Context, check string, poll func(ctx context.Context) (done bool, seen string, err error)) error {
	var lastSeen string

	for {
		if logged := e.recorder.loggedError(); logged != nil {
			return fmt.Errorf("wait %q after step %q: %w", check, e.recorder.lastStep, logged)
		}

		done, seen, err := poll(ctx)
		if err != nil {
			return fmt.Errorf("wait %q after step %q: %w", check, e.recorder.lastStep, err)
		}

		lastSeen = seen

		if done {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("wait %q after step %q did not complete before ctx ended: last seen %q",
				check, e.recorder.lastStep, lastSeen)
		case <-time.After(waitForPollInterval):
		}
	}
}

// ScenarioV2 is a scenario that drives the kernel-only supervisor.
type ScenarioV2 struct {
	// Run creates workers through env.Client, changes the mocks, and checks the
	// result. When a check fails, Run returns an error that names the check.
	// After a nil return, the runner waits RunConfig.Duration, then shuts the
	// supervisor down. Run must honor ctx cancellation: teardown cannot start
	// until Run returns.
	Run func(ctx context.Context, env Env) error

	// ExpectedErrors lists substrings of error log messages this scenario
	// expects. Any other error logged during the run fails it.
	ExpectedErrors []string

	// ExpectedWarnings lists substrings of warning log messages this
	// scenario expects. Any other warning logged during the run sets
	// RunResult.Err once the run has ended.
	ExpectedWarnings []string

	// Name is the identifier for this scenario (used in CLI --scenario flag).
	Name string

	// Description explains what this scenario tests (shown in CLI output).
	Description string

	// Dependencies optionally builds the scenario's mocks. Every worker Run
	// upserts through env.Client receives the map in its constructor. Write and
	// read it with config.SetDependency and config.LookupDependency.
	//
	// On error, Dependencies must release what it built, and the runner starts
	// nothing. On success, the runner calls cleanup once if it is non-nil: after
	// the supervisor stops, or at once if the supervisor fails to build.
	Dependencies func() (depsMap map[string]any, cleanup func(), err error)
}

// NoopScenarioV2 starts the kernel-only supervisor and drives nothing: the
// application worker spawns only its config worker kernel child.
//
// This scenario is kept permanently as the copy-paste template for scenario
// authors: copy it, rename it, put your mocks in Dependencies, and put the
// steps and checks in Run.
var NoopScenarioV2 = ScenarioV2{
	Name:        "noop",
	Description: "Runs the kernel-only supervisor and changes nothing (v2)",
	Run: func(_ context.Context, _ Env) error {
		return nil
	},
}

// RegistryV2 contains all available v2 scenarios, merged into ListScenarios
// alongside the v1 Registry. Names must not collide with v1 Registry names
// (enforced by the disjointness test in scenariov2_test.go, which documents
// what breaks on a collision).
var RegistryV2 = map[string]ScenarioV2{
	"noop":    NoopScenarioV2,
	"dynamic": DynamicScenarioV2,
}
