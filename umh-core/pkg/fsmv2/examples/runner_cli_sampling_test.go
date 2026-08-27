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
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker"
)

// countConsoleLines counts output lines that carry msg anywhere on the line.
// The CLI renders one log entry per line, so an entry whose fields also
// mention the message name still counts once.
func countConsoleLines(output, msg string) int {
	count := 0

	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, msg) {
			count++
		}
	}

	return count
}

// countWatchedEntries counts parsed entries whose Msg equals msg. The
// predicate is narrower than countConsoleLines's substring match: it demands
// equality with the message field. The two counts still agree on the watched
// message because no other line the run emits carries that name.
func countWatchedEntries(entries []examples.LogEntry, msg string) int {
	count := 0

	for _, entry := range entries {
		if entry.Msg == msg {
			count++
		}
	}

	return count
}

var _ = Describe("Scenario runner CLI log stream", func() {
	// The configworker deps key is process-global; a control run (the
	// in-process baseline execution through examples.Run, built below) that
	// fails mid-run would otherwise leak it into every later v2 spec in this
	// process.
	BeforeEach(func() {
		DeferCleanup(func() {
			register.ClearDeps(configworker.WorkerTypeName)
		})
	})

	It("delivers every child_reconciliation_completed entry to the stream a developer watches", func() {
		const (
			// watchedMsg is the entry both runs count:
			// child_reconciliation_completed, the report a supervisor logs at
			// debug level after each reconciling tick that touched a child
			// (Supervisor.reconcileChildren). It fires roughly once per tick
			// for most of a run, so at the pinned settings below a complete
			// run emits it hundreds of times and a stream that drops entries
			// drops a large, countable share of them.
			watchedMsg = "child_reconciliation_completed"

			// dynamicScenarioName selects the registered dynamic scenario
			// (examples.DynamicScenarioV2), which drives one helloworld child
			// through create, update and delete against a live supervisor.
			dynamicScenarioName = "dynamic"

			// pinnedTick and pinnedDuration hold both runs to one cadence and
			// one span, so their emission counts are comparable. The values
			// also sharpen the measurement: at a 50ms tick the scenario emits
			// roughly twenty watchedMsg entries per second, and
			// pinnedDuration is the settle window the runner waits after the
			// scenario's driver returns (RunConfig.Duration), which
			// contributes a fixed, tick-accurate block of entries to both
			// runs.
			pinnedTick     = 50 * time.Millisecond
			pinnedDuration = 10 * time.Second

			// samplingTolerance is how far the two counts may differ and
			// still agree. The two runs are separate executions, and the
			// dynamic scenario's driver phase takes about 3.1s or about 4.1s
			// depending on the run (measured), which at a 50ms tick moves the
			// count by roughly 20 entries; tick alignment adds a few more.
			// The tolerance sits well above that spread. It sits well below
			// what a regression to sampling removes: building the run's
			// logger with deps.NewFSMLogger again would apply the sampling
			// contract stated at that constructor's doc, and the sampled
			// stream would keep about a quarter of the entries (measured
			// before this change: 70 of 264, the closest pair observed;
			// retuning the sampler moves this arithmetic). No run-to-run
			// variance reaches that far.
			samplingTolerance = 80
		)

		// The stream a developer watches exists only inside the scenario
		// runner binary (pkg/fsmv2/cmd/runner): the binary builds the run's
		// logger with deps.NewUnsampledFSMLogger, and package main cannot be
		// imported. So the spec builds the real binary and runs the same
		// scenario through it, the way a developer would.
		_, thisFile, _, _ := runtime.Caller(0)
		runnerDir := filepath.Join(filepath.Dir(thisFile), "..", "cmd", "runner")

		binDir, err := os.MkdirTemp("", "scenario-runner-cli")
		Expect(err).NotTo(HaveOccurred())

		DeferCleanup(func() { _ = os.RemoveAll(binDir) })

		binPath := filepath.Join(binDir, "scenario-runner")

		// runtime.Caller yields this file's source path, so the build target
		// resolves whatever directory the test process happens to run in.
		buildCtx, cancelBuild := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancelBuild()

		buildCmd := exec.CommandContext(buildCtx, "go", "build", "-o", binPath, runnerDir)
		buildCmd.Dir = filepath.Dir(thisFile)

		buildOut, err := buildCmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(),
			"building the scenario runner binary failed: %s", buildOut)

		// The control run (the ctl* variables below) measures what the
		// scenario emits: the same registered scenario through examples.Run,
		// counted from RunResult.Logs. Those entries are captured by the tee
		// runV2 builds inside examples.Run (a deps.NewJSONFSMLogger over an
		// in-memory buffer), not by ctlLogger: ctlLogger is the run's
		// caller-facing sink, which discards here. The capture arm applies no
		// sampler, so its count is every entry the run emitted, which is the
		// number any developer-facing stream must match.
		ctlLogger := deps.NewJSONFSMLogger(io.Discard, deps.LevelDebug)
		ctlStore := examples.SetupStore(ctlLogger)

		ctlCtx, cancelCtl := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancelCtl()

		ctlResult, err := examples.Run(ctlCtx, examples.RunConfig{
			ScenarioV2:   examples.RegistryV2[dynamicScenarioName],
			Duration:     pinnedDuration,
			TickInterval: pinnedTick,
			Logger:       ctlLogger,
			Store:        ctlStore,
		})
		Expect(err).NotTo(HaveOccurred(),
			"the control run must complete; its error carries the scenario failure")

		Eventually(ctlResult.Done, "85s").Should(BeClosed(),
			"the control run must tear down on its own after the settle window")

		ctlCount := countWatchedEntries(ctlResult.Logs, watchedMsg)

		// The agreement check below can only detect a loss when the control
		// emits more than the tolerance: a control run at or below it could
		// lose every excess entry and still agree. This gate fails loudly
		// when emission collapses (a renamed message, a scenario that drives
		// no children) instead of passing vacuously on two quiet streams.
		Expect(ctlCount).To(BeNumerically(">", samplingTolerance),
			"the control run emitted only %d %s entries; the agreement check needs a run that emits well beyond the tolerance", ctlCount, watchedMsg)

		// The CLI run is the developer's stream: the binary's console output.
		// -log-level debug is required because the watched entry is a debug
		// entry; the default info level would filter it out. -tick and
		// -duration are pinned to the control run's values.
		runCtx, cancelRun := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancelRun()

		cliCmd := exec.CommandContext(runCtx, binPath,
			"-scenario", dynamicScenarioName,
			"-duration", pinnedDuration.String(),
			"-tick", pinnedTick.String(),
			"-log-level", "debug",
		)

		var stdout, stderr bytes.Buffer
		cliCmd.Stdout = &stdout
		cliCmd.Stderr = &stderr

		runErr := cliCmd.Run()

		// The spec sends no signal, so a nil runErr is exactly a clean run:
		// a degraded shutdown (the runner exits 1), a panic (exit 2) and the
		// 120s runCtx killing the binary all surface as a non-nil runErr.
		Expect(runErr).NotTo(HaveOccurred(),
			"the runner must exit zero on a clean run; stderr: %s", stderr.String())

		cliCount := countConsoleLines(stdout.String(), watchedMsg)

		// A zero count means the binary never produced the stream (a crash or
		// a startup failure), not that entries were lost mid-stream; the run
		// error and stderr distinguish the two.
		Expect(cliCount).To(BeNumerically(">", 0),
			"the CLI run must deliver some entries; run error: %v, stderr: %s", runErr, stderr.String())

		Expect(cliCount).To(BeNumerically("~", ctlCount, samplingTolerance),
			"the stream a developer watches must hold the same entries as an unsampled control run of the same scenario, tick and duration: the CLI run delivered %d %s entries, the control run emitted %d (run error: %v, stderr: %s)",
			cliCount, watchedMsg, ctlCount, runErr, stderr.String())
	})
})
