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
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
)

// v2LogBuffer is a goroutine-safe buffer for capturing JSON log output in
// the ScenarioV2 specs.
type v2LogBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *v2LogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

func (b *v2LogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

// logContainsEvent reports whether any JSON log line has the given msg value.
func logContainsEvent(logOutput, msg string) bool {
	for _, line := range strings.Split(logOutput, "\n") {
		if line == "" {
			continue
		}

		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		if entry["msg"] == msg {
			return true
		}
	}

	return false
}

var _ = Describe("ScenarioV2 framework", func() {
	// The configworker deps key is process-global; a spec that fails mid-run
	// would otherwise leak it into every later spec in this process.
	BeforeEach(func() {
		DeferCleanup(func() {
			register.ClearDeps(configworker.WorkerTypeName)
		})
	})

	It("keeps v1 and v2 registry names disjoint", func() {
		// On a name collision, ListScenarios and the CLI --list silently
		// prefer the v2 entry, and --scenario resolves both forms so Run
		// rejects them with its conflicting-configuration error, making
		// both scenarios unrunnable.
		for name := range examples.RegistryV2 {
			Expect(examples.Registry).NotTo(HaveKey(name),
				"scenario name %q is registered in both Registry and RegistryV2", name)
		}
	})

	It("rejects a RunConfig with both a v1 and a v2 scenario set", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		result, err := examples.Run(context.Background(), examples.RunConfig{
			Scenario:   examples.Scenario{Name: "v1", YAMLConfig: "children: []"},
			ScenarioV2: examples.ScenarioV2{Name: "v2", Driver: func(_ context.Context, _ examples.Env) error { return nil }},
			Logger:     logger,
			Store:      store,
		})
		Expect(err).To(MatchError(ContainSubstring("conflicting configuration")))
		Expect(result).To(BeNil())
	})

	It("rejects a ScenarioV2 with a Name but no Driver, naming the scenario", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		result, err := examples.Run(context.Background(), examples.RunConfig{
			ScenarioV2: examples.ScenarioV2{Name: "driverless"},
			Logger:     logger,
			Store:      store,
		})
		Expect(err).To(MatchError(ContainSubstring("driverless")))
		Expect(result).To(BeNil())
	})

	It("rejects a ScenarioV2 with a Driver but no Name", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		// An anonymous run would produce the supervisor ID "scenariov2-" and
		// log lines naming an empty scenario, which post-run log checks
		// cannot attribute.
		driverRan := false
		result, err := examples.Run(context.Background(), examples.RunConfig{
			ScenarioV2: examples.ScenarioV2{
				Driver: func(_ context.Context, _ examples.Env) error {
					driverRan = true

					return nil
				},
			},
			Logger: logger,
			Store:  store,
		})
		Expect(err).To(MatchError(ContainSubstring("Driver is set but Name is empty")))
		Expect(result).To(BeNil())
		Expect(driverRan).To(BeFalse(),
			"a nameless v2 scenario must be rejected before its driver runs")
	})

	It("fails loudly when the configworker deps key is already published by an overlapping run", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		// Simulate a first v2 run that has not finished teardown: its
		// registry is still published under the process-global key. The
		// BeforeEach DeferCleanup clears the key after this spec.
		firstRunWriter := dynamicchildren.NewWriter()
		register.SetDeps[*dynamicchildren.Registry](configworker.WorkerTypeName, firstRunWriter.Registry())

		driverRan := false
		overlapping := examples.ScenarioV2{
			Name:        "overlapping",
			Description: "test-local driver that must never run",
			Driver: func(_ context.Context, _ examples.Env) error {
				driverRan = true

				return nil
			},
		}

		result, err := examples.Run(context.Background(), examples.RunConfig{
			ScenarioV2:   overlapping,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).To(MatchError(ContainSubstring("already published")),
			"the second run must fail loudly instead of silently overwriting the key")
		Expect(err.Error()).To(ContainSubstring("overlapping"),
			"the error must name the scenario that could not start")
		Expect(result).To(BeNil())
		Expect(driverRan).To(BeFalse(),
			"the overlapping run must fail before starting a supervisor or its driver")

		// The first run's registry must survive untouched: a replaced or
		// cleared key would cross-wire the still-active first run.
		Expect(register.GetDeps[*dynamicchildren.Registry](configworker.WorkerTypeName)).To(
			BeIdenticalTo(firstRunWriter.Registry()),
			"the failed run must not replace or clear the already-published registry")
	})

	It("warns and ignores DumpStore for a v2 scenario", func() {
		logBuf := &v2LogBuffer{}
		logger := deps.NewJSONFSMLogger(logBuf, deps.LevelDebug)
		store := examples.SetupStore(logger)

		dumpRequested := examples.ScenarioV2{
			Name:        "dump-requested",
			Description: "test-local driver for the DumpStore warning path",
			Driver: func(_ context.Context, _ examples.Env) error {
				return nil
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   dumpRequested,
			Duration:     time.Second,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
			DumpStore:    true,
		})
		Expect(err).NotTo(HaveOccurred(),
			"DumpStore must not break a v2 run, only warn")
		Eventually(result.Done, "55s").Should(BeClosed())

		// A silently ignored DumpStore lets a developer misread "no dump
		// printed" as "no store changes", so the gap must be logged.
		Expect(logContainsEvent(logBuf.String(), "dump_store_not_supported_for_v2")).To(BeTrue(),
			"runV2 must warn that DumpStore is ignored for v2 scenarios")
	})

	It("tears down gracefully on a live tick loop when the caller ctx is cancelled mid-run", func() {
		logBuf := &v2LogBuffer{}
		logger := deps.NewJSONFSMLogger(logBuf, deps.LevelDebug)
		store := examples.SetupStore(logger)

		cancelMidRun := examples.ScenarioV2{
			Name:        "cancel-mid-run",
			Description: "test-local driver for the caller-ctx cancellation path",
			Driver: func(_ context.Context, _ examples.Env) error {
				return nil
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   cancelMidRun,
			Duration:     5 * time.Minute,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred())

		// Settle gate: cancel only once the configworker child is observably
		// running, so the cancellation lands mid-run on a live tick loop. A
		// blind sleep can fire while the child is still mid-startup, and a
		// mid-spawn child intermittently cannot finish draining within one
		// graceful-shutdown phase budget.
		Eventually(func(g Gomega) {
			dump, err := examples.DumpScenario(context.Background(), store, 0)
			g.Expect(err).NotTo(HaveOccurred())

			observedStates := map[string]interface{}{}
			for _, w := range dump.Workers {
				observedStates[w.WorkerType] = w.Observed["state"]
			}

			g.Expect(observedStates).To(HaveKeyWithValue(configworker.WorkerTypeName, "Running"))
		}, "30s").Should(Succeed(),
			"the configworker child must be running before the mid-run cancellation")

		cancel()
		Eventually(result.Done, "55s").Should(BeClosed(),
			"cancelling the caller ctx must trigger a complete teardown")

		// The supervisor must be fully stopped: ClearDeps runs strictly
		// after supDone, so a cleared key proves the supervisor exited.
		Expect(register.GetDeps[*dynamicchildren.Registry](configworker.WorkerTypeName)).To(BeNil(),
			"the deps key must be cleared after the cancellation-triggered teardown")

		// The graceful drain must run against a LIVE tick loop. If the tick
		// loop shared the caller's ctx, the cancel would kill it before
		// Shutdown, and every drain phase would wait out its timeout and
		// emit this warning.
		Expect(logContainsEvent(logBuf.String(), "graceful_shutdown_timeout")).To(BeFalse(),
			"the supervisor must drain via a live tick loop, not time out against a dead one")
	})

	It("lists noop in the merged registry and runs a v2 driver end-to-end on the kernel-only supervisor", func() {
		// Part (a): the v2 noop scenario must appear in the same listing the
		// CLI reads, so --list and --scenario find v1 and v2 scenarios alike.
		listing := examples.ListScenarios()
		Expect(listing).To(HaveKey("noop"),
			"merged ListScenarios must contain the v2 noop scenario")

		// Part (b): run a test-local v2 scenario through the v2 runner. The
		// sentinel bool proves the runner actually invoked the driver; noop's
		// own driver returns nil immediately, so it cannot prove execution.
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		driverRan := false
		clientWasSet := false
		loggerWasSet := false
		sentinel := examples.ScenarioV2{
			Name:        "sentinel",
			Description: "test-local driver that records execution",
			Driver: func(_ context.Context, env examples.Env) error {
				driverRan = true
				// Upsert through the running supervisor is covered by the
				// dynamic-children scenario, not this test.
				clientWasSet = env.Client != nil
				loggerWasSet = env.Logger != nil

				return nil
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   sentinel,
			Duration:     2 * time.Second,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(result.Done, "55s").Should(BeClosed(),
			"the v2 runner must wait RunConfig.Duration and then tear down on its own")

		Expect(driverRan).To(BeTrue(),
			"the v2 runner must execute the scenario Driver")
		Expect(clientWasSet).To(BeTrue(),
			"Env must carry a non-nil fsmv2client for the driver")
		Expect(loggerWasSet).To(BeTrue(),
			"Env must carry the run's logger for the driver")

		// Store check: with no YAML children declared, the only child the
		// application supervisor spawns is the config worker kernel, so the
		// store must contain exactly the application and configworker types.
		dump, err := examples.DumpScenario(context.Background(), store, 0)
		Expect(err).NotTo(HaveOccurred())

		workerTypes := map[string]bool{}
		for _, w := range dump.Workers {
			workerTypes[w.WorkerType] = true
		}

		Expect(workerTypes).To(Equal(map[string]bool{
			application.WorkerTypeName:  true,
			configworker.WorkerTypeName: true,
		}), "the config worker must be the application supervisor's only child")

		// Teardown check: the runner published the dynamicchildren registry
		// under the configworker deps key, so after the run it must clear it,
		// otherwise the next scenario inherits a stale registry.
		Expect(register.GetDeps[*dynamicchildren.Registry](configworker.WorkerTypeName)).To(BeNil(),
			"the v2 runner must ClearDeps the configworker key during teardown")
	})

	It("reports ShutdownClean=true after a clean v2 run", func() {
		// The runner exposes the supervisor's drain outcome so the CLI can
		// exit non-zero on a degraded shutdown. A clean run must surface
		// true and never the zero-value false, which would prove the field
		// is unwired rather than genuinely clean.
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		cleanRun := examples.ScenarioV2{
			Name:        "clean-shutdown",
			Description: "test-local driver for the ShutdownClean plumbing",
			Driver: func(_ context.Context, _ examples.Env) error {
				return nil
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   cleanRun,
			Duration:     2 * time.Second,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(result.Done, "55s").Should(BeClosed())

		Expect(result.ShutdownClean).To(BeTrue(),
			"a clean v2 run must report ShutdownClean=true, proving the field is wired to the supervisor's drain outcome")
	})

	It("tears down and clears the deps key when the driver fails", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		driverErr := errors.New("boom")
		failing := examples.ScenarioV2{
			Name:        "failing-driver",
			Description: "test-local driver that returns an error",
			Driver: func(_ context.Context, _ examples.Env) error {
				return driverErr
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   failing,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).To(MatchError(driverErr),
			"the runner must wrap and propagate the driver error")
		Expect(err.Error()).To(ContainSubstring("failing-driver"),
			"the error must name the failing scenario")
		Expect(result).To(BeNil())

		// The error path is a full teardown path: a leaked key would
		// cross-wire every later v2 run in this process.
		Expect(register.GetDeps[*dynamicchildren.Registry](configworker.WorkerTypeName)).To(BeNil(),
			"the v2 runner must ClearDeps the configworker key on driver failure")
	})

	It("runs forever with Duration=0 and tears down on context cancellation", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		runForever := examples.ScenarioV2{
			Name:        "run-forever",
			Description: "test-local driver for the Duration=0 ctx-cancel path",
			Driver: func(_ context.Context, _ examples.Env) error {
				return nil
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   runForever,
			Duration:     0,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred())

		// Caller-ctx cancellation is the only teardown path for a Duration=0
		// run; a regression here leaves such a run hanging forever.
		cancel()
		Eventually(result.Done, "55s").Should(BeClosed(),
			"the v2 runner must tear down when the context is cancelled")

		// Shutdown waits on Done, so after Done is closed it must return
		// promptly with the deps key already cleared.
		result.Shutdown()
		Expect(register.GetDeps[*dynamicchildren.Registry](configworker.WorkerTypeName)).To(BeNil(),
			"the v2 runner must ClearDeps the configworker key after ctx cancellation")
	})

	It("tears down and clears the deps key when the driver panics", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		panicking := examples.ScenarioV2{
			Name:        "panicking-driver",
			Description: "test-local driver that panics",
			Driver: func(_ context.Context, _ examples.Env) error {
				panic("driver exploded")
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		Expect(func() {
			_, _ = examples.Run(ctx, examples.RunConfig{
				ScenarioV2:   panicking,
				TickInterval: 50 * time.Millisecond,
				Logger:       logger,
				Store:        store,
			})
		}).To(PanicWith("driver exploded"),
			"the runner must not swallow a driver panic")

		// A panic is a full teardown path too: a leaked key would
		// cross-wire every later v2 run in this process.
		Expect(register.GetDeps[*dynamicchildren.Registry](configworker.WorkerTypeName)).To(BeNil(),
			"the v2 runner must ClearDeps the configworker key on driver panic")
	})

	It("blocks a mid-Duration Shutdown until the deps key is cleared", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		longRun := examples.ScenarioV2{
			Name:        "long-run",
			Description: "test-local driver for the mid-Duration Shutdown path",
			Driver: func(_ context.Context, _ examples.Env) error {
				return nil
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   longRun,
			Duration:     5 * time.Minute,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred())

		// A mid-Duration Shutdown must block until teardown is complete, so
		// the next run cannot cross-wire with this one through a
		// still-published deps key.
		result.Shutdown()
		Expect(result.Done).To(BeClosed(),
			"Shutdown must not return before Done is closed")
		Expect(register.GetDeps[*dynamicchildren.Registry](configworker.WorkerTypeName)).To(BeNil(),
			"the deps key must already be cleared when Shutdown returns")
	})

	It("supports back-to-back sequential v2 runs in one process", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		scenario := examples.ScenarioV2{
			Name:        "back-to-back",
			Description: "test-local driver for sequential v2 runs",
			Driver: func(_ context.Context, _ examples.Env) error {
				return nil
			},
		}

		// Run 1 tears down via ctx cancellation while Duration is pending,
		// which is the only branch of the Duration select the other specs
		// do not reach.
		ctx1, cancel1 := context.WithCancel(context.Background())
		defer cancel1()

		result1, err := examples.Run(ctx1, examples.RunConfig{
			ScenarioV2:   scenario,
			Duration:     5 * time.Minute,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred())

		cancel1()
		Eventually(result1.Done, "55s").Should(BeClosed(),
			"run 1 must tear down on ctx cancellation during the Duration wait")

		// Run 2 must start cleanly: run 1's teardown cleared the deps key,
		// so the fail-loud overlap guard must not fire.
		ctx2, cancel2 := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel2()

		result2, err := examples.Run(ctx2, examples.RunConfig{
			ScenarioV2:   scenario,
			Duration:     time.Second,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(result2.Done, "55s").Should(BeClosed(),
			"run 2 must complete after run 1 in the same process")

		Expect(register.GetDeps[*dynamicchildren.Registry](configworker.WorkerTypeName)).To(BeNil(),
			"run 2 must clear the deps key just like run 1 did")
	})
})
