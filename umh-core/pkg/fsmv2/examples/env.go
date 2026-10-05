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
	"strings"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
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
// of Env. The fields above mu are set before the supervisor starts and never
// change. The matched maps exist so postRunFailure can name every expected
// entry the run never logged.
type runRecorder struct {
	// scenario is the name of the run's scenario, so a step line can be
	// attributed when several scenarios run in one process.
	scenario string

	expectedErrors      []string
	expectedErrorCauses []error
	expectedWarnings    []string

	// mu guards the fields below. Step may run on a goroutine the scenario
	// starts, and any goroutine that logs writes the first unexpected values.
	mu                  sync.Mutex
	lastStep            string
	firstUnexpectedErr  error
	firstUnexpectedWarn error

	// matchedWarnings holds the expectedWarnings entries a logged warning
	// has matched.
	matchedWarnings map[string]bool

	// matchedErrors holds the expectedErrors entries a logged error
	// message has matched.
	matchedErrors map[string]bool

	// matchedCauses holds, by index, the expectedErrorCauses entries a
	// logged error has wrapped. Keyed by index because an error value is
	// not always comparable, so it is not a safe map key.
	matchedCauses map[int]bool
}

// alwaysAllowedMessages are logged by the collector in normal operation, so
// every run may log them at error or warning level without declaring them.
var alwaysAllowedMessages = []string{
	"data_stale",
	"collector_observation_failed",
	"collector_stop_skipped",
}

// recordLoggedError keeps the first error the scenario does not expect. The
// matchers run before the lock, because errors.Is calls user code that may
// log again.
func (r *runRecorder) recordLoggedError(err error, msg string) {
	matched := matchedEntries(r.expectedErrors, msg)
	caused := matchedCauseIndices(err, r.expectedErrorCauses)
	unexpected := !r.messageAllowed(msg, r.expectedErrors) && !r.errorCauseAllowed(err)

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, entry := range matched {
		r.matchedErrors[entry] = true
	}

	for _, i := range caused {
		r.matchedCauses[i] = true
	}

	if unexpected && r.firstUnexpectedErr == nil {
		r.firstUnexpectedErr = fmt.Errorf("the scenario does not expect this error: %s (%w)", msg, err)
	}
}

// messageMatchesEntry reports whether msg contains entry. An empty entry
// is never required and matches nothing, because strings.Contains would
// match every message.
func messageMatchesEntry(msg, entry string) bool {
	return entry != "" && strings.Contains(msg, entry)
}

func (r *runRecorder) messageAllowed(msg string, expected []string) bool {
	for _, substr := range expected {
		if messageMatchesEntry(msg, substr) {
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

// errorMatchesCause reports whether err is or wraps cause. A nil cause is
// never required and matches nothing.
func errorMatchesCause(err, cause error) bool {
	return cause != nil && errors.Is(err, cause)
}

// errorCauseAllowed reports whether err is or wraps one of the scenario's
// expected causes.
func (r *runRecorder) errorCauseAllowed(err error) bool {
	for _, cause := range r.expectedErrorCauses {
		if errorMatchesCause(err, cause) {
			return true
		}
	}

	return false
}

func (r *runRecorder) recordLoggedWarning(msg string) {
	matched := matchedEntries(r.expectedWarnings, msg)
	unexpected := !r.messageAllowed(msg, r.expectedWarnings)

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, entry := range matched {
		r.matchedWarnings[entry] = true
	}

	if unexpected && r.firstUnexpectedWarn == nil {
		r.firstUnexpectedWarn = fmt.Errorf("the scenario does not expect this warning: %s", msg)
	}
}

// matchedEntries returns the entries of expected that msg contains, in
// expected's order.
func matchedEntries(expected []string, msg string) []string {
	var matched []string

	for _, entry := range expected {
		if messageMatchesEntry(msg, entry) {
			matched = append(matched, entry)
		}
	}

	return matched
}

// matchedCauseIndices returns the indices of causes that err is or wraps.
func matchedCauseIndices(err error, causes []error) []int {
	var matched []int

	for i, cause := range causes {
		if errorMatchesCause(err, cause) {
			matched = append(matched, i)
		}
	}

	return matched
}

// unmatchedEntries returns the entries of expected that no logged message
// matched, in expected's order. An entry the list repeats is returned once.
func (r *runRecorder) unmatchedEntries(expected []string, matched map[string]bool) []string {
	var missing []string

	seen := make(map[string]bool, len(expected))

	for _, want := range expected {
		if want == "" || seen[want] {
			continue
		}

		if !matched[want] {
			seen[want] = true
			missing = append(missing, want)
		}
	}

	return missing
}

func (r *runRecorder) loggedWarning() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.firstUnexpectedWarn
}

// missingExpectedEntries returns one error per entry of ExpectedErrors,
// ExpectedErrorCauses and ExpectedWarnings that the run never logged: the
// errors first, then the causes, then the warnings, each group in the order
// the scenario listed it. An entry the scenario lists twice is named once,
// except a cause whose type cannot be compared, which is named each time
// it is listed.
func (r *runRecorder) missingExpectedEntries() []error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var missing []error

	for _, entry := range r.unmatchedEntries(r.expectedErrors, r.matchedErrors) {
		missing = append(missing, fmt.Errorf("the scenario expects this error, but the run never logged it: %s", entry))
	}

	var missingCauses []error

	for i, cause := range r.expectedErrorCauses {
		if cause == nil || r.matchedCauses[i] {
			continue
		}

		if containsSameCause(missingCauses, cause) {
			continue
		}

		missingCauses = append(missingCauses, cause)
	}

	for _, cause := range missingCauses {
		missing = append(missing, fmt.Errorf("the scenario expects this error cause, but the run never logged it: %w", cause))
	}

	for _, entry := range r.unmatchedEntries(r.expectedWarnings, r.matchedWarnings) {
		missing = append(missing, fmt.Errorf("the scenario expects this warning, but the run never logged it: %s", entry))
	}

	return missing
}

// containsSameCause reports whether earlier holds the same cause value. A
// cause whose type cannot be compared is never the same entry.
func containsSameCause(earlier []error, cause error) bool {
	for _, seen := range earlier {
		if sameCause(seen, cause) {
			return true
		}
	}

	return false
}

// sameCause reports whether a and b hold the same error value. Comparing
// two causes whose values cannot be compared panics, so a panic counts as
// different entries.
func sameCause(a, b error) (same bool) {
	defer func() {
		if recover() != nil {
			same = false
		}
	}()

	return a == b //nolint:errorlint // the sameness of two listed cause entries is their values' ==, not errors.Is
}

func (r *runRecorder) loggedError() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.firstUnexpectedErr
}

func (r *runRecorder) setLastStep(description string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.lastStep = description
}

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

func (l *recordingLogger) SentryError(feature deps.Feature, hierarchyPath string, err error, msg string, fields ...deps.Field) {
	l.recorder.recordLoggedError(err, msg)

	l.FSMLogger.SentryError(feature, hierarchyPath, err, msg, fields...)
}

func (l *recordingLogger) SentryWarn(feature deps.Feature, hierarchyPath string, msg string, fields ...deps.Field) {
	l.recorder.recordLoggedWarning(msg)

	l.FSMLogger.SentryWarn(feature, hierarchyPath, msg, fields...)
}

// With wraps again, so a logger carrying context fields records too.
func (l *recordingLogger) With(fields ...deps.Field) deps.FSMLogger {
	return &recordingLogger{FSMLogger: l.FSMLogger.With(fields...), recorder: l.recorder}
}

const waitForPollInterval = 50 * time.Millisecond

// A poll that ignores its ctx can hold a WaitFor past waitForTimeout.
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
// A passed wait logs one scenario_wait_passed line.
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
			e.Logger.Info("scenario_wait_passed",
				deps.String("scenario", e.recorder.scenario),
				deps.String("check", check),
				deps.String("seen", seen))
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

// timesEntered returns how many times the worker has entered state. The count
// keeps its value after the worker leaves the state, so a wait still sees a
// state the worker has already left.
func timesEntered[TStatus any](obs fsmv2.Observation[TStatus], state string) int64 {
	return obs.Metrics.Framework.TransitionsByState[state]
}

// Scenario is a scenario that drives the kernel-only supervisor.
type Scenario struct {
	// Run creates workers through env.Client, changes the mocks, and checks the
	// result. When a check fails, Run returns an error that names the check.
	// After a nil return, the runner waits RunConfig.Duration, then shuts the
	// supervisor down. Run must honor ctx cancellation: teardown cannot start
	// until Run returns.
	Run func(ctx context.Context, env Env) error

	// ExpectedErrors lists substrings of error messages this scenario
	// expects. Every listed entry must appear in an error message the
	// run logs, or RunResult.Err names the entry; only the message text
	// is matched, so an error's value counts only through
	// ExpectedErrorCauses. Any other error logged during the run fails
	// it, or sets RunResult.Err when it is logged after Run returned.
	// An error logged through the run's logger counts, whether it comes
	// from the scenario's workers, their supervisors or its Run; the
	// runner's own errors and the store's errors do not.
	ExpectedErrors []string

	// ExpectedErrorCauses lists error values this scenario expects, matched
	// with errors.Is. Use it when the message is generic: ActionExecutor
	// (supervisor/internal/execution) logs every failed action as
	// action_failed, so expecting that message would let any failed action
	// pass. Every listed cause must appear in an error the run logs, or
	// RunResult.Err names the cause. A nil entry matches nothing and is
	// never required.
	ExpectedErrorCauses []error

	// ExpectedWarnings lists substrings of warning log messages this
	// scenario expects. Every listed entry must appear in a warning the
	// run logs, or RunResult.Err names the entry. A warning no entry
	// lists also sets RunResult.Err. A warning logged through the run's
	// logger counts, whether it comes from the scenario's workers, their
	// supervisors or its Run; the runner's own warnings and the store's
	// warnings do not.
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

// NoopScenario changes nothing, so the application worker spawns only its
// config worker kernel child.
var NoopScenario = Scenario{
	Name:        "noop",
	Description: "Runs the kernel-only supervisor and changes nothing",
	Run: func(_ context.Context, _ Env) error {
		return nil
	},
}

// Registry contains all available scenarios, merged into ListScenarios
// alongside LiveRegistry. A name may be in only one of the two; the spec
// "keeps the registries' names disjoint" in scenario_test.go says why.
var Registry = map[string]Scenario{
	"noop":         NoopScenario,
	"helloworld":   HelloworldScenario,
	"failing":      FailingScenario,
	"slow":         SlowScenario,
	"timeout":      TimeoutScenario,
	"panic":        PanicScenario,
	"dynamic":      DynamicScenario,
	"nmap":         NmapScenario,
	"transport":    TransportScenario,
	"concurrent":   ConcurrentScenario,
	"simple":       SimpleScenario,
	"cascade":      CascadeScenario,
	"configerror":  ConfigErrorScenario,
	"inheritance":  InheritanceScenario,
	"communicator": CommunicatorScenario,
	"persistence":  PersistenceScenario,

	"certfetcher-healthy":        CertFetcherHealthyScenario,
	"certfetcher-degraded":       CertFetcherDegradedScenario,
	"certfetcher-no-subscribers": CertFetcherNoSubscribersScenario,
	"historian":                  HistorianScenario,

	"cpu-pressure": CPUPressureScenario,
	"cpu-blind":    CPUBlindScenario,
	"cpu-stall":    CPUStallScenario,
	"cpu-filling":  CPUFillingScenario,
	"cpu-latch":    CPULatchScenario,
	"cpu-v1":       CPUV1Scenario,
}

// LiveRegistry holds scenarios that read the real machine the runner runs
// on, so their result depends on that machine's CPU load and cgroup files. The
// CLI runs them. The registry spec does not, because CI does not control those.
var LiveRegistry = map[string]Scenario{
	"cpu-host": CPUHostScenario,
}
