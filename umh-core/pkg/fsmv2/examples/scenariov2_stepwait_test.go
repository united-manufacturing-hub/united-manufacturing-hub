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
	"strings"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker"
)

// logLinesNaming counts the JSON log lines that mention s.
func logLinesNaming(logOutput, s string) int {
	count := 0

	for _, line := range strings.Split(logOutput, "\n") {
		if line != "" && strings.Contains(line, s) {
			count++
		}
	}

	return count
}

var _ = Describe("ScenarioV2 steps and waits", func() {
	BeforeEach(func() {
		DeferCleanup(register.ClearGlobalDeps, configworker.WorkerTypeName)
	})

	It("logs each step once, and fails a never-done wait naming the last step, the check and the last value seen", func() {
		logBuf := &v2LogBuffer{}
		logger := deps.NewJSONFSMLogger(logBuf, deps.LevelDebug)
		store := examples.SetupStore(logger)

		// examples.Run calls Run on the caller's goroutine, so the spec reads
		// waitErr without a lock.
		var waitErr error
		var firstWaitPolls atomic.Int32

		stepping := examples.ScenarioV2{
			Name:        "step-and-wait",
			Description: "test-local Run for Step logging and WaitFor polling",
			Run: func(ctx context.Context, env examples.Env) error {
				env.Step("change the mood file to grumpy")

				// The first check reports done on its third poll, so a WaitFor
				// that gives up after one poll fails here and fails the run.
				if err := env.WaitFor(ctx, "store shows the grumpy mood",
					func(_ context.Context) (bool, string, error) {
						n := firstWaitPolls.Add(1)
						if n < 3 {
							return false, "mood=still-happy", nil
						}

						return true, "mood=grumpy", nil
					}); err != nil {
					return err
				}

				env.Step("remove the mood file")

				// The second check never reports done, so ctx must end first.
				// The wait's error is recorded instead of returned, so the
				// spec can assert on the error text while the run itself
				// still finishes cleanly.
				waitErr = env.WaitFor(ctx, "store shows an empty mood",
					func(_ context.Context) (bool, string, error) {
						return false, "mood=grumpy", nil
					})

				return nil
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   stepping,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred(),
			"the first wait must finish once its check reports done, so Run returns nil")

		Eventually(result.Done, "55s").Should(BeClosed(),
			"the run must tear down once its ctx ends")

		Expect(waitErr).To(HaveOccurred(),
			"a check that never reports done must fail its wait when ctx ends first")

		Expect(waitErr.Error()).To(ContainSubstring("remove the mood file"),
			"the error must name the last Step before the failing wait")
		Expect(waitErr.Error()).To(ContainSubstring("store shows an empty mood"),
			"the error must name the check that never completed")
		Expect(waitErr.Error()).To(ContainSubstring("mood=grumpy"),
			"the error must name the last value the check saw")

		Expect(firstWaitPolls.Load()).To(BeNumerically(">=", int32(3)),
			"WaitFor must poll its check until the check reports done")

		// Exactly one line for the first step, because its wait succeeded and
		// no failure quotes it. The second step's failing wait may quote it,
		// so that count is a floor.
		Expect(logLinesNaming(logBuf.String(), "change the mood file to grumpy")).To(Equal(1),
			"Step must log exactly one line naming the change")
		Expect(logLinesNaming(logBuf.String(), "remove the mood file")).To(BeNumerically(">=", 1),
			"Step must log a line naming the change")

		var stepLine string

		for _, line := range strings.Split(logBuf.String(), "\n") {
			if strings.Contains(line, "scenario_step") && strings.Contains(line, "change the mood file to grumpy") {
				stepLine = line

				break
			}
		}

		Expect(stepLine).To(ContainSubstring("step-and-wait"),
			"the step line must name the scenario that announced the change")
	})

	It("fails a wait whose poll errors, naming the last step, the check and the poll's error", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		pollErr := errors.New("store read broke")

		erroring := examples.ScenarioV2{
			Name:        "wait-poll-error",
			Description: "test-local Run for the poll-error wait",
			Run: func(ctx context.Context, env examples.Env) error {
				env.Step("probe step")

				return env.WaitFor(ctx, "store read works",
					func(_ context.Context) (bool, string, error) {
						return false, "", pollErr
					})
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   erroring,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).To(HaveOccurred(),
			"a poll that errors must fail its wait and the run")
		Expect(err.Error()).To(ContainSubstring("probe step"),
			"the error must name the last Step before the failing wait")
		Expect(err.Error()).To(ContainSubstring("store read works"),
			"the error must name the check whose poll errored")
		Expect(err.Error()).To(ContainSubstring("store read broke"),
			"the error must carry the poll's own error")
		Expect(errors.Is(err, pollErr)).To(BeTrue(),
			"the poll's error must stay findable through the wrapping")
		Expect(errors.Is(err, examples.ErrScenarioFailed)).To(BeTrue(),
			"a Run that returned an error must wrap ErrScenarioFailed, so the CLI reports a failed scenario and not a failed start")
		Expect(result).To(BeNil())
	})

	It("lets a goroutine the scenario starts call Step while Run waits", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		var polls atomic.Int32

		concurrent := examples.ScenarioV2{
			Name:        "step-from-goroutine",
			Description: "test-local Run that calls Step from a second goroutine",
			Run: func(ctx context.Context, env examples.Env) error {
				stop := make(chan struct{})
				stopped := make(chan struct{})

				go func() {
					defer close(stopped)

					for {
						select {
						case <-stop:
							return
						default:
							env.Step("background step")
						}
					}
				}()

				// The poll errors on its fifth call, so WaitFor reads the last
				// step while the goroutine is still writing it. The race
				// detector fails the spec if that read is not synchronised.
				err := env.WaitFor(ctx, "background steps ran",
					func(_ context.Context) (bool, string, error) {
						if polls.Add(1) < 5 {
							return false, "", nil
						}

						return false, "", errors.New("poll gave up")
					})

				close(stop)
				<-stopped

				return err
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   concurrent,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("background step"),
			"the failed wait must name the step the goroutine announced")
	})

	It("returns nil from a wait whose check reports done on its third poll", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		var polls atomic.Int32

		counting := examples.ScenarioV2{
			Name:        "wait-third-poll",
			Description: "test-local Run for the done-on-third-poll wait",
			Run: func(ctx context.Context, env examples.Env) error {
				return env.WaitFor(ctx, "store shows the grumpy mood",
					func(_ context.Context) (bool, string, error) {
						n := polls.Add(1)
						if n < 3 {
							return false, "mood=still-happy", nil
						}

						return true, "mood=grumpy", nil
					})
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   counting,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred(),
			"a check that reports done on its third poll must let the run succeed")
		Expect(result).NotTo(BeNil())

		Eventually(result.Done, "55s").Should(BeClosed(),
			"the run must tear down on its own once the check reports done")
		Expect(polls.Load()).To(Equal(int32(3)),
			"WaitFor must poll exactly until the check reports done")
	})

	It("fails a wait that never completes within its own timeout, with no deadline on the caller's ctx", func() {
		restore := examples.SetWaitForTimeoutForTest(300 * time.Millisecond)
		defer restore()

		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		neverDone := examples.ScenarioV2{
			Name:        "wait-never-done",
			Description: "test-local Run for the wait timeout",
			Run: func(ctx context.Context, env examples.Env) error {
				env.Step("wait for a check that never passes")

				return env.WaitFor(ctx, "store shows the grumpy mood",
					func(_ context.Context) (bool, string, error) {
						return false, "mood=still-happy", nil
					})
			},
		}

		// The caller's ctx has no deadline, so only the wait's own timeout can
		// end it. The examples.Run call sits in a goroutine so a hung wait
		// fails this spec through Eventually.
		type runOutcome struct {
			result *examples.RunResult
			err    error
		}

		outcome := make(chan runOutcome, 1)

		go func() {
			result, err := examples.Run(context.Background(), examples.RunConfig{
				ScenarioV2:   neverDone,
				TickInterval: 50 * time.Millisecond,
				Logger:       logger,
				Store:        store,
			})
			outcome <- runOutcome{result: result, err: err}
		}()

		var done runOutcome
		Eventually(outcome, "10s").Should(Receive(&done),
			"a wait whose check never passes must end its run on its own, not hang forever")

		Expect(done.err).To(HaveOccurred(),
			"a wait that never completes must fail the run")
		Expect(done.err.Error()).To(ContainSubstring("store shows the grumpy mood"),
			"the failure must name the check that never completed")
		Expect(done.err.Error()).To(ContainSubstring("timed out after 300ms"),
			"the failure must name the wait's own timeout")
	})
})

var _ = Describe("ScenarioV2 wait context", func() {
	BeforeEach(func() {
		DeferCleanup(register.ClearGlobalDeps, configworker.WorkerTypeName)
	})

	It("keeps ctx.Err in a wait the caller's ctx cancelled", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		// examples.Run calls Run on the caller's goroutine, so the spec reads
		// waitErr without a lock.
		var waitErr error

		cancelled := examples.ScenarioV2{
			Name:        "wait-ctx-cancelled",
			Description: "test-local Run for the ctx-cancelled wait",
			Run: func(ctx context.Context, env examples.Env) error {
				env.Step("wait on a check that never passes")

				// The poll never reports done, so the ctx the spec cancels
				// is the only thing that can end this wait.
				waitErr = env.WaitFor(ctx, "store shows the grumpy mood",
					func(_ context.Context) (bool, string, error) {
						return false, "mood=still-happy", nil
					})

				return waitErr
			},
		}

		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			time.Sleep(150 * time.Millisecond)
			cancel()
		}()

		result, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   cancelled,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).To(HaveOccurred(),
			"a wait the caller's ctx cancelled must fail the run")
		Expect(err.Error()).To(ContainSubstring("store shows the grumpy mood"),
			"the failure must name the check that never completed")
		Expect(errors.Is(err, context.Canceled)).To(BeTrue(),
			"the wrapped error must still be findable as context.Canceled, so the CLI can read an interrupt as a clean exit")
		Expect(result).To(BeNil())
	})
})
