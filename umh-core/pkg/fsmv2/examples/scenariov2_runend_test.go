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
	"encoding/json"
	"errors"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker"
)

type logEntry struct {
	Level    string `json:"level"`
	Msg      string `json:"msg"`
	Scenario string `json:"scenario"`
	Check    string `json:"check"`
	Seen     string `json:"seen"`
}

// parseLogEntries decodes each JSON line of logOutput, in log order, and skips lines that do not decode.
func parseLogEntries(logOutput string) []logEntry {
	var entries []logEntry

	for _, line := range strings.Split(logOutput, "\n") {
		var entry logEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		entries = append(entries, entry)
	}

	return entries
}

func entriesWithMsg(entries []logEntry, msg string) []logEntry {
	var matching []logEntry

	for _, entry := range entries {
		if entry.Msg == msg {
			matching = append(matching, entry)
		}
	}

	return matching
}

func msgIndexes(entries []logEntry, msg string) []int {
	var indexes []int

	for i, entry := range entries {
		if entry.Msg == msg {
			indexes = append(indexes, i)
		}
	}

	return indexes
}

var _ = Describe("ScenarioV2 run end", func() {
	It("logs one scenario_run_finished line after a Run that returned nil", func() {
		DeferCleanup(register.ClearGlobalDeps, configworker.WorkerTypeName)

		logBuf := &v2LogBuffer{}
		logger := deps.NewJSONFSMLogger(logBuf, deps.LevelDebug)
		store := examples.SetupStore(logger)

		finishing := examples.ScenarioV2{
			Name:        "run-finished-logging",
			Description: "test-local Run for the run-finished log line",
			Run: func(ctx context.Context, env examples.Env) error {
				return env.WaitFor(ctx, "one check",
					func(_ context.Context) (bool, string, error) {
						return true, "seen=one", nil
					})
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   finishing,
			Duration:     50 * time.Millisecond,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred(),
			"a Run whose only wait passes must let examples.Run succeed")

		Eventually(result.Done, "55s").Should(BeClosed(),
			"the run must tear down once its short Duration ends")

		entries := parseLogEntries(logBuf.String())
		finished := entriesWithMsg(entries, "scenario_run_finished")

		Expect(finished).To(HaveLen(1),
			"a run whose Run returned nil must log exactly one scenario_run_finished line")

		Expect(finished[0].Level).To(Equal("info"),
			"the line must be logged at info, the runner CLI's default level, or it disappears from run output")
		Expect(finished[0].Scenario).To(Equal("run-finished-logging"),
			"the line must name the scenario that finished")

		waitIdxs := msgIndexes(entries, "scenario_wait_passed")
		finishedIdx := msgIndexes(entries, "scenario_run_finished")[0]
		teardownIdxs := msgIndexes(entries, "v2_run_teardown_starting")

		Expect(waitIdxs).NotTo(BeEmpty(),
			"the passing wait must log its scenario_wait_passed line before the run ends")
		Expect(waitIdxs).To(HaveEach(BeNumerically("<", finishedIdx)),
			"the scenario_run_finished line must come after every scenario_wait_passed line, so the log reads in the order the run happened")
		Expect(teardownIdxs).To(HaveEach(BeNumerically(">", finishedIdx)),
			"the scenario_run_finished line must come before the teardown starts, so it marks the end of Run itself")
	})

	It("logs no scenario_run_finished line for a Run that returns an error", func() {
		DeferCleanup(register.ClearGlobalDeps, configworker.WorkerTypeName)

		logBuf := &v2LogBuffer{}
		logger := deps.NewJSONFSMLogger(logBuf, deps.LevelDebug)
		store := examples.SetupStore(logger)

		refusing := examples.ScenarioV2{
			Name:        "run-refused-logging",
			Description: "test-local Run for the missing run-finished line",
			Run: func(_ context.Context, _ examples.Env) error {
				return errors.New("scenario says no")
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   refusing,
			Duration:     50 * time.Millisecond,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).To(HaveOccurred(),
			"a Run that returns an error must fail examples.Run")
		Expect(err.Error()).To(ContainSubstring("scenario says no"),
			"the failure must carry the error Run returned")

		Expect(parseLogEntries(logBuf.String())).NotTo(BeEmpty(),
			"the failing run must have logged something, so the absence below observed a run that actually ran")
		Expect(entriesWithMsg(parseLogEntries(logBuf.String()), "scenario_run_finished")).To(BeEmpty(),
			"a run whose Run returned an error must not log a scenario_run_finished line")
	})

	It("logs no scenario_run_finished line for a Run whose logged-error check fails", func() {
		DeferCleanup(register.ClearGlobalDeps, configworker.WorkerTypeName)

		logBuf := &v2LogBuffer{}
		logger := deps.NewJSONFSMLogger(logBuf, deps.LevelDebug)
		store := examples.SetupStore(logger)

		swallowing := examples.ScenarioV2{
			Name:        "run-swallowed-error",
			Description: "test-local Run that logs an error and still returns nil",
			Run: func(_ context.Context, env examples.Env) error {
				env.Logger.SentryError(deps.FeatureExamples, "",
					errors.New("the mood file is corrupt"), "probe_run_error")

				return nil
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   swallowing,
			Duration:     50 * time.Millisecond,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).To(HaveOccurred(),
			"a Run whose logged-error check fails must fail examples.Run")
		Expect(errors.Is(err, examples.ErrScenarioFailed)).To(BeTrue(),
			"the failure must wrap ErrScenarioFailed, so the CLI reports a failed scenario and not a failed start")
		Expect(err.Error()).To(ContainSubstring("probe_run_error"),
			"the failure must name the error the scenario logged")

		Expect(parseLogEntries(logBuf.String())).NotTo(BeEmpty(),
			"the failing run must have logged something, so the absence below observed a run that actually ran")
		Expect(entriesWithMsg(parseLogEntries(logBuf.String()), "scenario_run_finished")).To(BeEmpty(),
			"a run whose logged-error check fails must not log a scenario_run_finished line")
	})
})
