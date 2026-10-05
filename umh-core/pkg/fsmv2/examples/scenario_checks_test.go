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
	"fmt"
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

const scenarioErrorProbeType = "scenarioerrorprobe"

type scenarioErrorProbeConfig struct{}

type scenarioErrorProbeDeps struct{}

type scenarioErrorProbeStatus struct{}

// The probe's constructor logs one error through its BaseDependencies
// logger, the With-wrapped logger the supervisor gives every worker.
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

var _ = Describe("Scenario error checks", func() {
	BeforeEach(func() {
		DeferCleanup(register.ClearGlobalDeps, configworker.WorkerTypeName)
	})

	// loggedErrorRun runs scenario with a Run that logs msg as an error and,
	// when waitFor is set, waits on a check that is done at once.
	loggedErrorRun := func(scenario examples.Scenario, msg string, waitFor bool) (result *examples.RunResult, runErr, waitErr error) {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		scenario.Run = func(ctx context.Context, env examples.Env) error {
			env.Step("change the mood file to grumpy")

			// With, as the supervisor's worker loggers do.
			env.Logger.With(deps.String("probe", "logger-context")).SentryError(
				deps.FeatureExamples, "",
				errors.New("the mood file is corrupt"),
				msg)

			// Only the logged error can fail this wait.
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

		result, runErr = examples.Run(ctx, examples.RunConfig{
			Scenario:     scenario,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})

		return result, runErr, waitErr
	}

	It("fails the next wait and the run when Run logs an error the scenario does not expect", func() {
		_, runErr, waitErr := loggedErrorRun(examples.Scenario{
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

		Expect(runErr).To(HaveOccurred(),
			"an error logged during a run must fail the run again once Run has returned")
		Expect(runErr.Error()).To(ContainSubstring("probe_unexpected_error"),
			"the run's failure must name the error that was logged")

		Expect(register.GlobalDeps[*dynamicchildren.Registry](configworker.WorkerTypeName)).To(BeNil(),
			"the run must tear down even when it failed on a logged error")
	})

	It("fails the run when Run logs an unexpected error and returns without waiting", func() {
		_, runErr, waitErr := loggedErrorRun(examples.Scenario{
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
		result, runErr, waitErr := loggedErrorRun(examples.Scenario{
			Name:           "expected-error",
			Description:    "test-local Run for the ExpectedErrors check",
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
		result, runErr, waitErr := loggedErrorRun(examples.Scenario{
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

		workerError := examples.Scenario{
			Name:        "worker-error",
			Description: "test-local Run for the worker-logged error check",
			Run: func(ctx context.Context, env examples.Env) error {
				ref := dynamicchildren.Ref{WorkerType: scenarioErrorProbeType, Name: "error-probe"}
				if err := env.Client.Upsert(ref, nil); err != nil {
					return err
				}

				// Only the worker's logged error can end this wait; a ctx timeout
				// means the error never reached the run's checks.
				return env.WaitFor(ctx, "store shows the grumpy mood",
					func(_ context.Context) (bool, string, error) {
						return false, "mood=still-happy", nil
					})
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			Scenario:     workerError,
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

var _ = Describe("Scenario stored-state check", func() {
	BeforeEach(func() {
		DeferCleanup(register.ClearGlobalDeps, configworker.WorkerTypeName)
	})

	It("sets RunResult.Err when the store holds a state that is not a valid state name for its worker type", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		const badStateName = "NotAValidState"
		const probeWorkerID = "bad-state-probe"

		storingBadState := examples.Scenario{
			Name:        "invalid-stored-state",
			Description: "test-local Run for the stored-state check",
			Run: func(ctx context.Context, env examples.Env) error {
				// So the stored-state check also meets a valid state it must not flag.
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
					"hierarchy_path": "scenario-invalid-stored-state/" + probeWorkerID,
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
			Scenario:     storingBadState,
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

		noop := examples.Scenario{
			Name:        "valid-stored-state",
			Description: "test-local Run for the clean stored-state check",
			Run: func(_ context.Context, _ examples.Env) error {
				return nil
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			Scenario:     noop,
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

type failingDeltasStore struct {
	storage.TriangularStoreInterface
}

func (failingDeltasStore) GetDeltas(_ context.Context, _ storage.Subscription) (storage.DeltasResponse, error) {
	return storage.DeltasResponse{}, errors.New("the deltas are unreadable")
}

var _ = Describe("Scenario store-read failure", func() {
	BeforeEach(func() {
		DeferCleanup(register.ClearGlobalDeps, configworker.WorkerTypeName)
	})

	It("sets RunResult.Err when the stored-state check cannot read the store", func() {
		logger := deps.NewNopFSMLogger()
		inner := examples.SetupStore(logger)
		store := failingDeltasStore{TriangularStoreInterface: inner}

		noop := examples.Scenario{
			Name:        "store-read-fails",
			Description: "test-local Run for the store-read failure check",
			Run: func(_ context.Context, _ examples.Env) error {
				return nil
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			Scenario:     noop,
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

var _ = Describe("Scenario missing expected warnings", func() {
	BeforeEach(func() {
		DeferCleanup(register.ClearGlobalDeps, configworker.WorkerTypeName)
	})

	It("sets RunResult.Err when a warning the scenario expects is never logged", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		silent := examples.Scenario{
			Name:             "missing-expected-warning",
			Description:      "test-local Run for the missing expected warning check",
			ExpectedWarnings: []string{"probe_never_logged"},
			Run: func(_ context.Context, _ examples.Env) error {
				return nil
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			Scenario:     silent,
			Duration:     300 * time.Millisecond,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred(),
			"a run whose Run returned nil and logged nothing must succeed")

		Eventually(result.Done, "55s").Should(BeClosed(),
			"the run must tear down once its settle window ends")

		Expect(result.Err).To(HaveOccurred(),
			"a warning listed in ExpectedWarnings must set RunResult.Err when the run never logs it")
		Expect(result.Err.Error()).To(ContainSubstring("the scenario expects this warning, but the run never logged it: probe_never_logged"),
			"the set Err must name the listed warning the run never logged")
	})

	It("keeps RunResult.Err nil when the run logs a listed warning", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		talking := examples.Scenario{
			Name:             "matched-expected-warning",
			Description:      "test-local Run for the matched expected warning check",
			ExpectedWarnings: []string{"probe_logged"},
			Run: func(_ context.Context, env examples.Env) error {
				env.Logger.SentryWarn(deps.FeatureExamples, "", "probe_logged")

				return nil
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			Scenario:     talking,
			Duration:     300 * time.Millisecond,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred(),
			"a run that logs a listed warning must succeed")

		Eventually(result.Done, "55s").Should(BeClosed(),
			"the run must tear down once its settle window ends")

		Expect(result.Err).NotTo(HaveOccurred(),
			"a listed warning the run logs must not set RunResult.Err")
	})
})

var _ = Describe("Scenario missing expected errors", func() {
	BeforeEach(func() {
		DeferCleanup(register.ClearGlobalDeps, configworker.WorkerTypeName)
	})

	It("sets RunResult.Err when an error the scenario expects is never logged", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		silent := examples.Scenario{
			Name:           "missing-expected-error",
			Description:    "test-local Run for the missing expected error check",
			ExpectedErrors: []string{"probe_never_logged"},
			Run: func(_ context.Context, _ examples.Env) error {
				return nil
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			Scenario:     silent,
			Duration:     300 * time.Millisecond,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred(),
			"a run whose Run returned nil and logged nothing must succeed")

		Eventually(result.Done, "55s").Should(BeClosed(),
			"the run must tear down once its settle window ends")

		Expect(result.Err).To(HaveOccurred(),
			"an error listed in ExpectedErrors must set RunResult.Err when the run never logs it")
		Expect(result.Err.Error()).To(ContainSubstring("the scenario expects this error, but the run never logged it: probe_never_logged"),
			"the set Err must name the listed error the run never logged")
	})

	It("keeps RunResult.Err nil when the run logs a listed error", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		talking := examples.Scenario{
			Name:           "matched-expected-error",
			Description:    "test-local Run for the matched expected error check",
			ExpectedErrors: []string{"probe_logged"},
			Run: func(_ context.Context, env examples.Env) error {
				env.Logger.SentryError(deps.FeatureExamples, "",
					errors.New("the logged probe failed"), "probe_logged")

				return nil
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			Scenario:     talking,
			Duration:     300 * time.Millisecond,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred(),
			"a run that logs a listed error must succeed")

		Eventually(result.Done, "55s").Should(BeClosed(),
			"the run must tear down once its settle window ends")

		Expect(result.Err).NotTo(HaveOccurred(),
			"a listed error the run logs must not set RunResult.Err")
	})

	It("sets RunResult.Err when the run logs only one of two listed errors", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		partiallyTalking := examples.Scenario{
			Name:           "partially-matched-expected-error",
			Description:    "test-local Run for the per-entry expected error check",
			ExpectedErrors: []string{"probe_logged", "probe_never_logged"},
			Run: func(_ context.Context, env examples.Env) error {
				env.Logger.SentryError(deps.FeatureExamples, "",
					errors.New("the logged probe failed"), "probe_logged")

				return nil
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			Scenario:     partiallyTalking,
			Duration:     300 * time.Millisecond,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred(),
			"a run that logs one of its listed errors and nothing unexpected must succeed")

		Eventually(result.Done, "55s").Should(BeClosed(),
			"the run must tear down once its settle window ends")

		Expect(result.Err).To(HaveOccurred(),
			"an entry of ExpectedErrors the run never logs must set RunResult.Err, even when another entry was logged")
		Expect(result.Err.Error()).To(ContainSubstring("the scenario expects this error, but the run never logged it: probe_never_logged"),
			"the set Err must name the listed entry the run never logged")
	})
})

var _ = Describe("Scenario empty expected entries", func() {
	BeforeEach(func() {
		DeferCleanup(register.ClearGlobalDeps, configworker.WorkerTypeName)
	})

	It("fails the run when an expected entry is the empty string", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		emptyExpectation := examples.Scenario{
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
			Scenario:     emptyExpectation,
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

// ctxHonouringStore fails GetDeltas once ctx is done, as a store behind a
// network or disk would. The in-memory store ignores ctx.
type ctxHonouringStore struct {
	storage.TriangularStoreInterface
}

func (s ctxHonouringStore) GetDeltas(ctx context.Context, sub storage.Subscription) (storage.DeltasResponse, error) {
	if err := ctx.Err(); err != nil {
		return storage.DeltasResponse{}, err
	}

	return s.TriangularStoreInterface.GetDeltas(ctx, sub)
}

var _ = Describe("Scenario cancelled after Run returned", func() {
	BeforeEach(func() {
		DeferCleanup(register.ClearGlobalDeps, configworker.WorkerTypeName)
	})

	It("ends with no stored-state failure when the caller cancels after Run returned", func() {
		logger := deps.NewNopFSMLogger()
		store := ctxHonouringStore{examples.SetupStore(logger)}

		returning := examples.Scenario{
			Name:        "cancel-after-return",
			Description: "test-local Run for the post-return cancellation",
			Run: func(_ context.Context, _ examples.Env) error {
				return nil
			},
		}

		ctx, cancel := context.WithCancel(context.Background())

		// No Duration: the run ends only when the caller cancels.
		result, err := examples.Run(ctx, examples.RunConfig{
			Scenario:     returning,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred(),
			"a run whose Run returned must succeed")

		cancel()

		Eventually(result.Done, "55s").Should(BeClosed(),
			"the run must tear down once its ctx is cancelled")

		Expect(result.Err).NotTo(HaveOccurred(),
			"cancelling after Run returned must not turn into a failed store read; a Ctrl+C is not a failed run")
	})
})

var _ = Describe("Scenario expected error causes", func() {
	// causeRun returns a nil result when the logged error fails the run.
	causeRun := func(name string, errValue error, causes []error) (*examples.RunResult, error) {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		scenario := examples.Scenario{
			Name:                name,
			Description:         "test-local Run for the expected-error-cause check",
			ExpectedErrorCauses: causes,
			Run: func(_ context.Context, env examples.Env) error {
				env.Logger.SentryError(deps.FeatureExamples, "", errValue, "action_failed")

				return nil
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		return examples.Run(ctx, examples.RunConfig{
			Scenario:     scenario,
			Duration:     300 * time.Millisecond,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
	}

	var errProbe = errors.New("the simulated failure")

	It("allows an error whose cause the scenario expects under a generic message", func() {
		DeferCleanup(register.ClearGlobalDeps, configworker.WorkerTypeName)

		result, err := causeRun("expected-cause", fmt.Errorf("wrap: %w", errProbe), []error{errProbe})
		Expect(err).NotTo(HaveOccurred(),
			"an error whose cause the scenario declared must not fail the run, even under the generic action_failed message")

		Eventually(result.Done, "55s").Should(BeClosed(),
			"the run must tear down once its settle window ends")

		Expect(result.Err).NotTo(HaveOccurred(),
			"an allowed error must not set RunResult.Err either")
	})

	It("fails the run when the logged error's cause is not expected under the same message", func() {
		DeferCleanup(register.ClearGlobalDeps, configworker.WorkerTypeName)

		_, err := causeRun("unexpected-cause", fmt.Errorf("wrap: %w", errors.New("a different failure")), []error{errProbe})
		Expect(err).To(HaveOccurred(),
			"expecting a cause must not allow every error logged as action_failed")
		Expect(err.Error()).To(ContainSubstring("action_failed"),
			"the failure must name the generic message the error was logged under")
	})

	It("allows nothing when the expected cause entry is nil", func() {
		DeferCleanup(register.ClearGlobalDeps, configworker.WorkerTypeName)

		_, err := causeRun("nil-cause", nil, []error{nil})
		Expect(err).To(HaveOccurred(),
			"a nil expected cause must match nothing, not even a nil error, the same as an empty expected message")
		Expect(err.Error()).To(ContainSubstring("action_failed"),
			"the failure must name the generic message the error was logged under")
	})
})
