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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker"
)

var _ = Describe("WaitForWarning", func() {
	BeforeEach(func() {
		DeferCleanup(register.ClearGlobalDeps, configworker.WorkerTypeName)
	})

	It("waits for a Warn a goroutine logs after a delay, matching a field value", func() {
		logBuf := &logBuffer{}
		logger := deps.NewJSONFSMLogger(logBuf, deps.LevelDebug)
		store := examples.SetupStore(logger)

		waiting := examples.Scenario{
			Name:        "wait-for-warning",
			Description: "test-local Run for WaitForWarning polling",
			ExpectedWarnings: []string{
				"config_error",
			},
			Run: func(ctx context.Context, env examples.Env) error {
				env.Step("log a config_error warning 100 ms after the wait starts")

				time.AfterFunc(100*time.Millisecond, func() {
					env.Logger.Warn("config_error",
						deps.String("field", "variables.SLAVEID"))
				})

				return env.WaitForWarning(ctx, "config_error", "SLAVEID")
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			Scenario:     waiting,
			Duration:     50 * time.Millisecond,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred(),
			"the wait must pass once the delayed Warn is recorded")

		Eventually(result.Done, "55s").Should(BeClosed(),
			"the run must tear down once its short Duration ends")
	})

	It("fails naming msg and contains when no matching Warn is ever logged", func() {
		restore := examples.SetWaitForTimeoutForTest(200 * time.Millisecond)
		DeferCleanup(restore)

		logBuf := &logBuffer{}
		logger := deps.NewJSONFSMLogger(logBuf, deps.LevelDebug)
		store := examples.SetupStore(logger)

		var waitErr error

		waiting := examples.Scenario{
			Name:        "wait-for-warning-timeout",
			Description: "test-local Run for the WaitForWarning timeout",
			ExpectedWarnings: []string{
				"config_error",
			},
			Run: func(ctx context.Context, env examples.Env) error {
				env.Logger.Warn("config_error",
					deps.String("field", "variables.SLAVEID"))

				waitErr = env.WaitForWarning(ctx, "NOPE", "SLAVEID")

				return nil
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			Scenario:     waiting,
			Duration:     50 * time.Millisecond,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred(),
			"Run returns nil; the recorded wait error is asserted below")

		Eventually(result.Done, "55s").Should(BeClosed(),
			"the run must tear down once its short Duration ends")

		Expect(waitErr).To(HaveOccurred(),
			"a warning that never arrives must fail its wait once the timeout passes")
		Expect(waitErr.Error()).To(ContainSubstring("NOPE"),
			"the error must name the awaited message")
		Expect(waitErr.Error()).To(ContainSubstring("SLAVEID"),
			"the error must name the awaited field text")
	})

	It("fails the run for an unexpected Warn message and passes for an expected one", func() {
		runScenario := func(expected []string) error {
			logBuf := &logBuffer{}
			logger := deps.NewJSONFSMLogger(logBuf, deps.LevelDebug)
			store := examples.SetupStore(logger)

			warning := examples.Scenario{
				Name:             "warn-checked",
				Description:      "test-local Run for the end-of-run Warn check",
				ExpectedWarnings: expected,
				Run: func(ctx context.Context, env examples.Env) error {
					env.Logger.Warn("config_error",
						deps.String("field", "variables.SLAVEID"))

					return nil
				},
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			result, err := examples.Run(ctx, examples.RunConfig{
				Scenario:     warning,
				Duration:     50 * time.Millisecond,
				TickInterval: 50 * time.Millisecond,
				Logger:       logger,
				Store:        store,
			})
			Expect(err).NotTo(HaveOccurred())

			Eventually(result.Done, "55s").Should(BeClosed())

			return result.Err
		}

		err := runScenario(nil)
		Expect(err).To(HaveOccurred(),
			"an unexpected Warn must fail the run like an unexpected SentryWarn does")
		Expect(err.Error()).To(ContainSubstring("config_error"),
			"the failure must name the unexpected Warn message")

		Expect(runScenario([]string{"config_error"})).NotTo(HaveOccurred(),
			"a Warn listed in ExpectedWarnings must not fail the run")
	})
})
