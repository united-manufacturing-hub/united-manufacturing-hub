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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
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

var _ = Describe("ScenarioV2 error checks", func() {
	BeforeEach(func() {
		DeferCleanup(register.ClearGlobalDeps, configworker.WorkerTypeName)
	})

	// loggedErrorRun runs a v2 scenario whose Run logs msg as an error, then
	// waits on a check that is done at once when waitFor is true. It returns
	// the run's result, the error from examples.Run and the wait's error.
	loggedErrorRun := func(scenario examples.ScenarioV2, msg string, waitFor bool) (*examples.RunResult, error, error) {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		// examples.Run calls Run on the caller's goroutine, so the spec reads
		// waitErr without a lock.
		var waitErr error

		scenario.Run = func(ctx context.Context, env examples.Env) error {
			env.Step("change the mood file to grumpy")

			// With adds context fields, the shape a supervisor's worker logger
			// has.
			env.Logger.With(deps.String("probe", "logger-context")).SentryError(
				deps.FeatureExamples, "",
				errors.New("the mood file is corrupt"),
				msg)

			// The check is done on its first poll, so only the logged error
			// can fail this wait.
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
		_, runErr, waitErr := loggedErrorRun(examples.ScenarioV2{
			Name:        "unexpected-error",
			Description: "test-local Run for the unexpected-error check",
		}, "probe_unexpected_error", true)

		Expect(waitErr).To(HaveOccurred(),
			"an error logged during a run must fail the next wait, even one whose check would succeed")
		Expect(waitErr.Error()).To(ContainSubstring("probe_unexpected_error"),
			"the wait's failure must name the error that was logged")
		Expect(waitErr.Error()).To(ContainSubstring("change the mood file to grumpy"),
			"the wait's failure must name the last Step before the wait")
		Expect(waitErr.Error()).To(ContainSubstring("store shows the grumpy mood"),
			"the wait's failure must name the check that was running")

		// Run returned nil after swallowing the wait failure; the runner fails
		// it anyway.
		Expect(runErr).To(HaveOccurred(),
			"an error logged during a run must fail the run again once Run has returned")
		Expect(runErr.Error()).To(ContainSubstring("probe_unexpected_error"),
			"the run's failure must name the error that was logged")

		Expect(register.GlobalDeps[*dynamicchildren.Registry](configworker.WorkerTypeName)).To(BeNil(),
			"the run must tear down even when it failed on a logged error")
	})

	It("fails the run when Run logs an unexpected error and returns without waiting", func() {
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
		Expect(errors.Is(runErr, examples.ErrScenarioFailed)).To(BeTrue(),
			"a run that failed on a logged error must wrap ErrScenarioFailed, so the CLI reports a failed scenario and not a failed start")

		Expect(register.GlobalDeps[*dynamicchildren.Registry](configworker.WorkerTypeName)).To(BeNil(),
			"the run must tear down even when it failed on a logged error")
	})

	It("does not fail the run when the logged error matches ExpectedErrors", func() {
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

var _ = Describe("ScenarioV2 stored-state check", func() {
	BeforeEach(func() {
		DeferCleanup(register.ClearGlobalDeps, configworker.WorkerTypeName)
	})

	It("sets RunResult.Err when the store holds a state that is not a valid state name for its worker type", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		// badStateName is in no worker type's validWorkerStates entry.
		const badStateName = "NotAValidState"
		const probeWorkerID = "bad-state-probe"

		storingBadState := examples.ScenarioV2{
			Name:        "invalid-stored-state",
			Description: "test-local Run for the stored-state check",
			Run: func(ctx context.Context, env examples.Env) error {
				// Wait for the config worker's real Running state, so the check
				// also meets a valid state it must not flag.
				if err := env.WaitFor(ctx, "the kernel config worker reports Running",
					func(ctx context.Context) (bool, string, error) {
						dump, err := examples.DumpScenario(ctx, store, 0)
						if err != nil {
							return false, "", err
						}

						for _, w := range dump.Workers {
							if w.WorkerType == configworker.WorkerTypeName && w.Observed["state"] == "Running" {
								return true, "state=Running", nil
							}
						}

						return false, "the config worker has not reported Running yet", nil
					}); err != nil {
					return err
				}

				env.Step("store a config worker state that is not a valid state name")

				// A managed worker's next tick would overwrite the bad state, so
				// it goes on a worker only the store knows. SaveIdentity comes
				// first: delta discovery and LoadSnapshot both need it.
				identityDoc := persistence.Document{
					"id":             probeWorkerID,
					"name":           probeWorkerID,
					"worker_type":    configworker.WorkerTypeName,
					"hierarchy_path": "scenariov2-invalid-stored-state/" + probeWorkerID,
				}
				if err := store.SaveIdentity(ctx, configworker.WorkerTypeName, probeWorkerID, identityDoc); err != nil {
					return err
				}

				_, err := store.SaveObserved(ctx, configworker.WorkerTypeName, probeWorkerID, persistence.Document{
					"state": badStateName,
				})

				return err
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   storingBadState,
			Duration:     300 * time.Millisecond,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred(),
			"a stored state that is not a valid state name must not fail the run; it surfaces through RunResult.Err when the run ends")

		Eventually(result.Done, "55s").Should(BeClosed(),
			"the run must tear down once its settle window ends, which is when the stored-state check runs")

		Expect(result.Err).To(HaveOccurred(),
			"a stored state that is not a valid state name for its worker type must set RunResult.Err when the run ends")
		Expect(result.Err.Error()).To(ContainSubstring(badStateName),
			"the set Err must name the invalid state the store held")
		Expect(result.Err.Error()).To(ContainSubstring(probeWorkerID),
			"the set Err must name the worker whose stored state is not valid")
	})

	It("leaves RunResult.Err nil after a run that stores only valid states", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		noop := examples.ScenarioV2{
			Name:        "valid-stored-state",
			Description: "test-local Run for the clean stored-state check",
			Run: func(_ context.Context, _ examples.Env) error {
				return nil
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   noop,
			Duration:     300 * time.Millisecond,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred(),
			"a run that stores only valid states must succeed")

		Eventually(result.Done, "55s").Should(BeClosed(),
			"the run must tear down once its settle window ends")

		Expect(result.Err).NotTo(HaveOccurred(),
			"a run whose store holds only states its workers may report must leave RunResult.Err nil")
	})
})

// failingDeltasStore wraps a real store and fails every GetDeltas call, so a
// spec can drive the stored-state check into its read-error branch.
type failingDeltasStore struct {
	storage.TriangularStoreInterface
}

func (failingDeltasStore) GetDeltas(_ context.Context, _ storage.Subscription) (storage.DeltasResponse, error) {
	return storage.DeltasResponse{}, errors.New("the deltas are unreadable")
}

var _ = Describe("ScenarioV2 store-read failure", func() {
	BeforeEach(func() {
		DeferCleanup(register.ClearGlobalDeps, configworker.WorkerTypeName)
	})

	It("sets RunResult.Err when the stored-state check cannot read the store", func() {
		logger := deps.NewNopFSMLogger()
		inner := examples.SetupStore(logger)
		store := failingDeltasStore{TriangularStoreInterface: inner}

		noop := examples.ScenarioV2{
			Name:        "store-read-fails",
			Description: "test-local Run for the store-read failure check",
			Run: func(_ context.Context, _ examples.Env) error {
				return nil
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   noop,
			Duration:     300 * time.Millisecond,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred(),
			"a store the check cannot read must not fail the run; it surfaces through RunResult.Err")

		Eventually(result.Done, "55s").Should(BeClosed(),
			"the run must tear down once its settle window ends")

		Expect(result.Err).To(HaveOccurred(),
			"a store the stored-state check cannot read must set RunResult.Err")
		Expect(result.Err.Error()).To(ContainSubstring("read stored workers"),
			"the set Err must name the failed store read")
	})
})

var _ = Describe("ScenarioV2 empty expected entries", func() {
	BeforeEach(func() {
		DeferCleanup(register.ClearGlobalDeps, configworker.WorkerTypeName)
	})

	It("fails the run when an expected entry is the empty string", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		emptyExpectation := examples.ScenarioV2{
			Name:           "empty-expected",
			Description:    "test-local Run for the empty expected entry check",
			ExpectedErrors: []string{""},
			Run: func(_ context.Context, env examples.Env) error {
				env.Logger.SentryError(deps.FeatureExamples, "",
					errors.New("the mood file is corrupt"), "probe_unexpected_error")

				return nil
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   emptyExpectation,
			Duration:     300 * time.Millisecond,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).To(HaveOccurred(),
			"an empty expected entry must not make every error expected")
		Expect(err.Error()).To(ContainSubstring("probe_unexpected_error"),
			"the failure must name the error that was logged")
		Expect(result).To(BeNil())
	})
})

var _ = Describe("ScenarioV2 cancelled after Run returned", func() {
	BeforeEach(func() {
		DeferCleanup(register.ClearGlobalDeps, configworker.WorkerTypeName)
	})

	It("ends with no stored-state failure when the caller cancels after Run returned", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		// Duration 0: the run ends when the caller cancels, so the stored-state
		// check starts after ctx is cancelled.
		returning := examples.ScenarioV2{
			Name:        "cancel-after-return",
			Description: "test-local Run for the post-return cancellation",
			Run: func(_ context.Context, _ examples.Env) error {
				return nil
			},
		}

		ctx, cancel := context.WithCancel(context.Background())

		result, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   returning,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred(),
			"a run whose Run returned must succeed")

		// The caller cancels after Run returned; the teardown wakes on that
		// cancellation and reads the store for the state check.
		cancel()

		Eventually(result.Done, "55s").Should(BeClosed(),
			"the run must tear down once its ctx is cancelled")

		Expect(result.Err).NotTo(HaveOccurred(),
			"cancelling after Run returned must not turn into a failed store read; a Ctrl+C is not a failed run")
	})
})
