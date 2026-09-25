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

package examples_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
)

// scenarioErrorProbeType names the monitor worker the worker-logged error
// spec upserts. simple.Register wires its factory, supervisor and CSE type,
// so its constructor runs the way any worker a scenario creates runs.
const scenarioErrorProbeType = "scenarioerrorprobe"

type scenarioErrorProbeConfig struct{}

type scenarioErrorProbeDeps struct{}

type scenarioErrorProbeStatus struct{}

// The probe registers once for the test binary, not per spec. Its constructor
// logs one error through the logger the framework enriched with its identity
// (BaseDependencies stores a With-wrapped logger), so the spec can ask
// whether an error a worker logs reaches the run's checks through the
// supervisor's logger rather than only through Env.Logger.
func init() {
	simple.Register(simple.MonitorSpec[scenarioErrorProbeConfig, scenarioErrorProbeStatus, scenarioErrorProbeDeps]{
		WorkerType: scenarioErrorProbeType,
		NewDeps: func(_ deps.Identity, bd *deps.BaseDependencies, _ map[string]any) scenarioErrorProbeDeps {
			bd.GetLogger().SentryError(deps.FeatureExamples, "", errors.New("x"), "probe_worker_error")

			return scenarioErrorProbeDeps{}
		},
		Poll: func(_ context.Context, _ scenarioErrorProbeDeps, _ scenarioErrorProbeConfig) (scenarioErrorProbeStatus, error) {
			return scenarioErrorProbeStatus{}, nil
		},
	})
}

// This spec pins the run's error check: an error logged during a run that the
// scenario does not expect fails the run twice, once at the next wait and once
// more when Run has returned. An error the scenario expects, or one every run
// allows, fails nothing.
var _ = Describe("ScenarioV2 error checks", func() {
	// loggedErrorRun drives one v2 scenario whose Run logs one error with the
	// given message and then waits for the grumpy-mood check (or returns at
	// once, when waitFor is false). It returns the run's result, the error
	// from examples.Run and the error the wait reported.
	//
	// The check reports done on its first poll, so a wait that ignored the
	// logged error would succeed: the wait fails only because it checks for
	// logged errors. The recorder keeps the logged error, so the check after
	// Run returns re-reports it even when the scenario swallows that wait
	// failure; a wait failure with no logged error behind it is not
	// re-caught there.
	//
	// The logger carries context fields the way a worker's logger does, because
	// every error a supervisor logs reaches the run's checks in that shape.
	loggedErrorRun := func(scenario examples.ScenarioV2, msg string, waitFor bool) (*examples.RunResult, error, error) {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		// Run runs on the caller's goroutine (as the ScenarioV2 steps and
		// waits specs note), so the spec reads waitErr without synchronization.
		var waitErr error

		scenario.Run = func(ctx context.Context, env examples.Env) error {
			env.Step("change the mood file to grumpy")

			env.Logger.With(deps.String("probe", "logger-context")).SentryError(
				deps.FeatureExamples, "",
				errors.New("the mood file is corrupt"),
				msg)

			if waitFor {
				waitErr = env.WaitFor(ctx, "store shows the grumpy mood",
					func(_ context.Context) (bool, string, error) {
						return true, "mood=grumpy", nil
					})
			}

			return nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		result, runErr := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   scenario,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})

		return result, runErr, waitErr
	}

	It("fails the next wait and the run when Run logs an error the scenario does not expect", func() {
		// The configworker deps key is process-global; a spec that fails
		// mid-run would otherwise leak it into every later spec.
		DeferCleanup(register.ClearGlobalDeps, configworker.WorkerTypeName)

		_, runErr, waitErr := loggedErrorRun(examples.ScenarioV2{
			Name:        "unexpected-error",
			Description: "test-local Run for the unexpected-error check",
		}, "probe_unexpected_error", true)

		// Part (a): the logged error failed the next wait before its check
		// completed, and the failure names the error, the step and the check,
		// as every wait failure does.
		Expect(waitErr).To(HaveOccurred(),
			"an error logged during a run must fail the next wait, even one whose check would succeed")
		Expect(waitErr.Error()).To(ContainSubstring("probe_unexpected_error"),
			"the wait's failure must name the error that was logged")
		Expect(waitErr.Error()).To(ContainSubstring("change the mood file to grumpy"),
			"the wait's failure must name the last Step before the wait")
		Expect(waitErr.Error()).To(ContainSubstring("store shows the grumpy mood"),
			"the wait's failure must name the check that was running")

		// Part (b): the recorder kept the logged error, so the run failed once
		// more after Run returned nil, even though the scenario swallowed the
		// wait failure the error caused.
		Expect(runErr).To(HaveOccurred(),
			"an error logged during a run must fail the run again once Run has returned")
		Expect(runErr.Error()).To(ContainSubstring("probe_unexpected_error"),
			"the run's failure must name the error that was logged")

		// The runner tears down synchronously before it returns the failure,
		// so the configworker deps key is already cleared here; a leaked key
		// would fail every later v2 run's already-published check.
		Expect(register.GlobalDeps[*dynamicchildren.Registry](configworker.WorkerTypeName)).To(BeNil(),
			"the run must tear down even when it failed on a logged error")
	})

	It("fails the run when Run logs an unexpected error and returns without waiting", func() {
		DeferCleanup(register.ClearGlobalDeps, configworker.WorkerTypeName)

		_, runErr, waitErr := loggedErrorRun(examples.ScenarioV2{
			Name:        "unexpected-error-no-wait",
			Description: "test-local Run for the post-Run error check",
		}, "probe_unexpected_error", false)

		Expect(waitErr).NotTo(HaveOccurred(),
			"the scenario never waited, so no wait failed")
		Expect(runErr).To(HaveOccurred(),
			"an error logged during a run must fail the run even when Run never waits")
		Expect(runErr.Error()).To(ContainSubstring("probe_unexpected_error"),
			"the run's failure must name the error that was logged")

		// The runner tears down synchronously before it returns the failure,
		// so the configworker deps key is already cleared here.
		Expect(register.GlobalDeps[*dynamicchildren.Registry](configworker.WorkerTypeName)).To(BeNil(),
			"the run must tear down even when it failed on a logged error")
	})

	It("does not fail the run when the logged error matches ExpectedErrors", func() {
		DeferCleanup(register.ClearGlobalDeps, configworker.WorkerTypeName)

		result, runErr, waitErr := loggedErrorRun(examples.ScenarioV2{
			Name:        "expected-error",
			Description: "test-local Run for the ExpectedErrors check",
			// A substring, not the whole message: matching is by substring.
			ExpectedErrors: []string{"probe_unexpected"},
		}, "probe_unexpected_error", true)

		Expect(waitErr).NotTo(HaveOccurred(),
			"an error the scenario expects must not fail the next wait")
		Expect(runErr).NotTo(HaveOccurred(),
			"an error the scenario expects must not fail the run")

		Expect(result).NotTo(BeNil())
		Eventually(result.Done, "55s").Should(BeClosed(),
			"the run must still tear down after a successful check")
	})

	It("does not fail the run when the logged error is one every run allows", func() {
		DeferCleanup(register.ClearGlobalDeps, configworker.WorkerTypeName)

		result, runErr, waitErr := loggedErrorRun(examples.ScenarioV2{
			Name:        "allowed-error",
			Description: "test-local Run for the always-allowed error check",
		}, "probe_data_stale", true)

		Expect(waitErr).NotTo(HaveOccurred(),
			"an error every run allows must not fail the next wait")
		Expect(runErr).NotTo(HaveOccurred(),
			"an error every run allows must not fail the run")

		Expect(result).NotTo(BeNil())
		Eventually(result.Done, "55s").Should(BeClosed(),
			"the run must still tear down after a successful check")
	})

	It("fails the run when a worker it creates logs an error the scenario does not expect", func() {
		DeferCleanup(register.ClearGlobalDeps, configworker.WorkerTypeName)

		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		workerError := examples.ScenarioV2{
			Name:        "worker-error",
			Description: "test-local Run for the worker-logged error check",
			Run: func(ctx context.Context, env examples.Env) error {
				ref := dynamicchildren.Ref{WorkerType: scenarioErrorProbeType, Name: "error-probe"}
				if err := env.Client.Upsert(ref, nil); err != nil {
					return err
				}

				// The poll is never done: only the worker's logged error
				// ends the wait, so a ctx timeout here would mean the error
				// never reached the recorder.
				return env.WaitFor(ctx, "store shows the grumpy mood",
					func(_ context.Context) (bool, string, error) {
						return false, "mood=still-happy", nil
					})
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   workerError,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).To(HaveOccurred(),
			"an error a worker the scenario creates logs must fail the run")
		Expect(err.Error()).To(ContainSubstring("probe_worker_error"),
			"the run's failure must name the error the worker logged")
		Expect(result).To(BeNil())
	})
})
