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

	// Logger wraps RunConfig.Logger. All output still reaches that logger, and
	// an error or warning a Run logs also reaches the run's checks.
	Logger deps.FSMLogger

	// Dependencies is the map the scenario's Dependencies returned, or nil when
	// the scenario declares none. The supervisor reads the same map while it
	// builds workers, so Run must not write to it: Run reads a mock out with
	// config.LookupDependency and changes the mock itself.
	Dependencies map[string]any

	// recorder is set by the runner before Run sees this Env.
	recorder *runRecorder
}

// runRecorder is the per-run state that Step and WaitFor share across copies
// of Env. The fields above mu are set before the supervisor starts and never change.
type runRecorder struct {
	// scenario is the name of the run's scenario, so a step line can be
	// attributed when several scenarios run in one process.
	scenario string

	expectedErrors   []string
	expectedWarnings []string

	// mu guards the fields below. Step may run on a goroutine the scenario
	// starts, and any goroutine that logs writes the first unexpected values.
	mu                  sync.Mutex
	lastStep            string
	firstUnexpectedErr  error
	firstUnexpectedWarn error
}

// alwaysAllowedMessages lists the message substrings every run may log at
// error or warning level without failing: the collector reports these while
// it does its job, so no scenario should have to declare them.
var alwaysAllowedMessages = []string{
	"data_stale",
	"collector_observation_failed",
	"collector_stop_skipped",
}

// recordLoggedError keeps the first error whose message messageAllowed does
// not allow, wrapped so a failure names that message.
func (r *runRecorder) recordLoggedError(err error, msg string) {
	if r.messageAllowed(msg, r.expectedErrors) {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.firstUnexpectedErr == nil {
		r.firstUnexpectedErr = fmt.Errorf("the scenario does not expect this error: %s (%w)", msg, err)
	}
}

// messageAllowed reports whether a logged message contains a substring the
// scenario declared in ExpectedErrors or ExpectedWarnings, or one of the
// messages every run allows. An empty entry matches nothing, because
// strings.Contains would match every message.
func (r *runRecorder) messageAllowed(msg string, expected []string) bool {
	for _, substr := range expected {
		if substr != "" && strings.Contains(msg, substr) {
			return true
		}
	}

	for _, substr := range alwaysAllowedMessages {
		if strings.Contains(msg, substr) {
			return true
		}
	}

	return false
}

// recordLoggedWarning keeps the first warning whose message messageAllowed
// does not allow.
func (r *runRecorder) recordLoggedWarning(msg string) {
	if r.messageAllowed(msg, r.expectedWarnings) {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.firstUnexpectedWarn == nil {
		r.firstUnexpectedWarn = fmt.Errorf("the scenario does not expect this warning: %s", msg)
	}
}

// loggedWarning returns the first unexpected logged warning, or nil.
func (r *runRecorder) loggedWarning() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.firstUnexpectedWarn
}

// loggedError returns the first unexpected logged error, or nil.
func (r *runRecorder) loggedError() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.firstUnexpectedErr
}

// setLastStep remembers description as the last change Step announced.
func (r *runRecorder) setLastStep(description string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.lastStep = description
}

// lastStepDescription returns the last change Step announced.
func (r *runRecorder) lastStepDescription() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.lastStep
}

// recordingLogger passes every call to the run's logger and records each
// error and warning for the run's checks.
type recordingLogger struct {
	deps.FSMLogger
	recorder *runRecorder
}

// SentryError records the error for the run's checks, then delegates.
func (l *recordingLogger) SentryError(feature deps.Feature, hierarchyPath string, err error, msg string, fields ...deps.Field) {
	l.recorder.recordLoggedError(err, msg)

	l.FSMLogger.SentryError(feature, hierarchyPath, err, msg, fields...)
}

// SentryWarn records the warning for RunResult.Err, then delegates.
func (l *recordingLogger) SentryWarn(feature deps.Feature, hierarchyPath string, msg string, fields ...deps.Field) {
	l.recorder.recordLoggedWarning(msg)

	l.FSMLogger.SentryWarn(feature, hierarchyPath, msg, fields...)
}

// With wraps again, so a logger carrying context fields records too.
func (l *recordingLogger) With(fields ...deps.Field) deps.FSMLogger {
	return &recordingLogger{FSMLogger: l.FSMLogger.With(fields...), recorder: l.recorder}
}

const waitForPollInterval = 50 * time.Millisecond

// waitForTimeout bounds one WaitFor call. A poll that ignores its ctx can hold
// the wait past it.
var waitForTimeout = 30 * time.Second

// Step logs one line naming the change the scenario is about to make, and
// remembers it, so a later failed wait can name the change it followed.
func (e Env) Step(description string) {
	e.recorder.setLastStep(description)

	e.Logger.Info("scenario_step",
		deps.String("scenario", e.recorder.scenario),
		deps.String("step", description))
}

// WaitFor calls poll until it reports done, ctx ends, or waitForTimeout passes.
// An unexpected error the run logged fails the wait before the next poll.
// Every failure names check and the last Step. A timeout or a ctx end also
// names the last value poll saw.
func (e Env) WaitFor(ctx context.Context, check string, poll func(ctx context.Context) (done bool, seen string, err error)) error {
	waitCtx, cancel := context.WithTimeout(ctx, waitForTimeout)
	defer cancel()

	var lastSeen string

	for {
		if logged := e.recorder.loggedError(); logged != nil {
			return fmt.Errorf("wait %q after step %q: %w", check, e.recorder.lastStepDescription(), logged)
		}

		done, seen, err := poll(waitCtx)
		if err != nil {
			return fmt.Errorf("wait %q after step %q: %w", check, e.recorder.lastStepDescription(), err)
		}

		lastSeen = seen

		if done {
			return nil
		}

		select {
		case <-waitCtx.Done():
			if ctx.Err() != nil {
				return fmt.Errorf("wait %q after step %q did not complete before ctx ended: last seen %q: %w",
					check, e.recorder.lastStepDescription(), lastSeen, ctx.Err())
			}

			return fmt.Errorf("wait %q after step %q timed out after %s: last seen %q",
				check, e.recorder.lastStepDescription(), waitForTimeout, lastSeen)
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
	// expects. Any other error logged during the run fails it, or sets
	// RunResult.Err when it is logged after Run returned.
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
	"noop":       NoopScenarioV2,
	"helloworld": HelloworldScenarioV2,
	"dynamic":    DynamicScenarioV2,
}
