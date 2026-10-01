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
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker"
)

// In package main so it can hand a real run's result to shutdownExitCode.
var _ = Describe("Scenario warnings and late errors", func() {
	BeforeEach(func() {
		DeferCleanup(register.ClearGlobalDeps, configworker.WorkerTypeName)
	})

	runScenario := func(scenario examples.ScenarioV2, settle time.Duration) *examples.RunResult {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   scenario,
			Duration:     settle,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		ExpectWithOffset(1, err).NotTo(HaveOccurred(),
			"a warning or a late error must not fail the run itself; it surfaces through RunResult.Err")

		EventuallyWithOffset(1, result.Done, "55s").Should(BeClosed(),
			"the run must tear down once its settle window ends")

		return result
	}

	It("sets RunResult.Err on an unexpected warning, and the exit code follows Err", func() {
		warning := examples.ScenarioV2{
			Name:        "unexpected-warning",
			Description: "test-local Run for the unexpected-warning check",
			Run: func(_ context.Context, env examples.Env) error {
				env.Step("log a warning the scenario does not expect")

				env.Logger.SentryWarn(deps.FeatureExamples, "", "probe_unexpected_warning")

				return nil
			},
		}

		result := runScenario(warning, 300*time.Millisecond)
		Expect(result.Err).To(HaveOccurred(),
			"a warning the scenario does not expect must set RunResult.Err when the run ends")
		Expect(result.Err.Error()).To(ContainSubstring("probe_unexpected_warning"),
			"the set Err must name the warning that was logged")
		Expect(shutdownExitCode(result)).To(Equal(1),
			"the CLI must exit 1 when RunResult.Err is set")
	})

	It("does not set RunResult.Err on a warning the scenario expects", func() {
		expected := examples.ScenarioV2{
			Name:             "expected-warning",
			Description:      "test-local Run for the ExpectedWarnings check",
			ExpectedWarnings: []string{"probe_unexpected"},
			Run: func(_ context.Context, env examples.Env) error {
				env.Step("log a warning the scenario expects")

				env.Logger.SentryWarn(deps.FeatureExamples, "", "probe_unexpected_warning")

				return nil
			},
		}

		result := runScenario(expected, 300*time.Millisecond)
		Expect(result.Err).NotTo(HaveOccurred(),
			"a warning the scenario expects must not set RunResult.Err")
	})

	It("does not set RunResult.Err on a warning every run allows", func() {
		allowed := examples.ScenarioV2{
			Name:        "allowed-warning",
			Description: "test-local Run for the always-allowed warning check",
			Run: func(_ context.Context, env examples.Env) error {
				env.Step("log a warning every run allows")

				env.Logger.SentryWarn(deps.FeatureExamples, "", "probe_data_stale")

				return nil
			},
		}

		result := runScenario(allowed, 300*time.Millisecond)
		Expect(result.Err).NotTo(HaveOccurred(),
			"a warning every run allows must not set RunResult.Err")
	})

	It("sets RunResult.Err on an error logged after Run returned, and the exit code follows Err", func() {
		lateError := examples.ScenarioV2{
			Name:        "late-error",
			Description: "test-local Run for the late-error check",
			Run: func(_ context.Context, env examples.Env) error {
				env.Step("return, then let a goroutine log an error inside the settle window")

				go func() {
					time.Sleep(150 * time.Millisecond)

					env.Logger.SentryError(deps.FeatureExamples, "",
						errors.New("the mood file vanished"), "probe_late_error")
				}()

				return nil
			},
		}

		result := runScenario(lateError, 3*time.Second)
		Expect(result.Err).To(HaveOccurred(),
			"an error logged after Run returned must set RunResult.Err when the run ends")
		Expect(result.Err.Error()).To(ContainSubstring("probe_late_error"),
			"the set Err must name the error that was logged late")
		Expect(shutdownExitCode(result)).To(Equal(1),
			"the CLI must exit 1 when RunResult.Err is set")
	})
})
