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
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	hello_world "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld"
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

var scenarioDepsProbeLabelKey = config.NewDependencyKey[string]("examples.test.scenario_deps")

const scenarioDepsProbeType = "scenariov2-deps-probe"

type scenarioDepsProbeConfig struct{}

type scenarioDepsProbeDeps struct {
	label string
}

type scenarioDepsProbeStatus struct {
	ReceivedLabel string `json:"receivedLabel"`
}

type scenarioDepsProbeRecord struct {
	label   string
	present bool
}

// scenarioDepsProbeSeen carries the record across goroutines: the constructor
// runs on the supervisor's tick loop while the spec reads it from the test
// goroutine.
var scenarioDepsProbeSeen atomic.Pointer[scenarioDepsProbeRecord]

type scenarioEnvMock struct {
	touched bool
}

var scenarioEnvMockKey = config.NewDependencyKey[*scenarioEnvMock]("examples.test.env_deps_mock")

// Registered in init: register.Worker panics on a duplicate worker type,
// so a per-spec registration would panic under go test -count=2.
func init() {
	simple.Register(simple.MonitorSpec[scenarioDepsProbeConfig, scenarioDepsProbeStatus, scenarioDepsProbeDeps]{
		WorkerType: scenarioDepsProbeType,
		NewDeps: func(_ deps.Identity, _ *deps.BaseDependencies, rd map[string]any) scenarioDepsProbeDeps {
			label, ok := config.LookupDependency(rd, scenarioDepsProbeLabelKey)
			scenarioDepsProbeSeen.Store(&scenarioDepsProbeRecord{label: label, present: ok})

			return scenarioDepsProbeDeps{label: label}
		},
		Poll: func(_ context.Context, d scenarioDepsProbeDeps, _ scenarioDepsProbeConfig) (scenarioDepsProbeStatus, error) {
			return scenarioDepsProbeStatus{ReceivedLabel: d.label}, nil
		},
	})
}

var _ = Describe("ScenarioV2 framework", func() {
	// The configworker deps key is process-global; a spec that fails mid-run
	// would otherwise leak it into every later spec in this process.
	BeforeEach(func() {
		DeferCleanup(func() {
			register.ClearGlobalDeps(configworker.WorkerTypeName)
		})
	})

	It("keeps the v2 registries' names disjoint", func() {
		// On a name collision, --list shows the LiveRegistryV2 description
		// while --scenario runs the RegistryV2 scenario. The LiveRegistryV2
		// scenario can then not be run from the CLI.
		for name := range examples.LiveRegistryV2 {
			Expect(examples.RegistryV2).NotTo(HaveKey(name),
				"scenario name %q is registered in both RegistryV2 and LiveRegistryV2", name)
		}
	})

	It("rejects a ScenarioV2 with a Name but no Run, naming the scenario", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		result, err := examples.Run(context.Background(), examples.RunConfig{
			ScenarioV2: examples.ScenarioV2{Name: "no-run"},
			Logger:     logger,
			Store:      store,
		})
		Expect(err).To(MatchError(ContainSubstring("no-run")))
		Expect(result).To(BeNil())
	})

	It("rejects a ScenarioV2 with a Run but no Name", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		// An anonymous run would produce the supervisor ID "scenariov2-" and
		// log lines naming an empty scenario, which post-run log checks
		// cannot attribute.
		runRan := false
		result, err := examples.Run(context.Background(), examples.RunConfig{
			ScenarioV2: examples.ScenarioV2{
				Run: func(_ context.Context, _ examples.Env) error {
					runRan = true

					return nil
				},
			},
			Logger: logger,
			Store:  store,
		})
		Expect(err).To(MatchError(ContainSubstring("Run is set but Name is empty")))
		Expect(result).To(BeNil())
		Expect(runRan).To(BeFalse(),
			"a nameless v2 scenario must be rejected before its Run runs")
	})

	It("fails loudly when the configworker deps key is already published by an overlapping run", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		// Simulate a first v2 run that has not finished teardown: its
		// registry is still published under the process-global key. The
		// BeforeEach DeferCleanup clears the key after this spec.
		firstRunWriter := dynamicchildren.NewWriter()
		register.SetGlobalDeps[*dynamicchildren.Registry](configworker.WorkerTypeName, firstRunWriter.Registry())

		var depsCalled atomic.Bool
		runRan := false
		overlapping := examples.ScenarioV2{
			Name:        "overlapping",
			Description: "test-local Run that must never run",
			Dependencies: func() (map[string]any, func(), error) {
				depsCalled.Store(true)

				return nil, nil, nil
			},
			Run: func(_ context.Context, _ examples.Env) error {
				runRan = true

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
		Expect(runRan).To(BeFalse(),
			"the overlapping run must fail before starting a supervisor or its Run")
		Expect(depsCalled.Load()).To(BeFalse(),
			"a run blocked by the already-published key must not call the scenario's Dependencies")

		// The first run's registry must stay untouched: a replaced or
		// cleared key would cross-wire the still-active first run.
		Expect(register.GlobalDeps[*dynamicchildren.Registry](configworker.WorkerTypeName)).To(
			BeIdenticalTo(firstRunWriter.Registry()),
			"the failed run must not replace or clear the already-published registry")
	})

	It("prints the store dump when a v2 scenario's Run fails", func() {
		logBuf := &v2LogBuffer{}
		logger := deps.NewJSONFSMLogger(logBuf, deps.LevelDebug)
		store := examples.SetupStore(logger)

		failing := examples.ScenarioV2{
			Name:        "dump-after-failure",
			Description: "test-local Run that creates a worker and then fails",
			Run: func(ctx context.Context, env examples.Env) error {
				ref := dynamicchildren.Ref{WorkerType: "helloworld", Name: "dump-failed-hello"}

				env.Step("create a helloworld child")

				if err := env.Client.Upsert(ref, map[string]any{"state": "running"}); err != nil {
					return err
				}

				if err := env.WaitFor(ctx, "the helloworld child reaches Running",
					func(ctx context.Context) (bool, string, error) {
						obs, err := fsmv2client.Get[hello_world.HelloworldStatus](ctx, env.Client, ref)
						if err != nil {
							if errors.Is(err, fsmv2client.ErrNotObserved) {
								return false, "the child has not published an observation yet", nil
							}

							return false, "", err
						}

						return obs.State == "Running", "state=" + obs.State, nil
					}); err != nil {
					return err
				}

				return errors.New("scenario gave up")
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		origStdout := os.Stdout
		reader, writer, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())

		var out bytes.Buffer

		drainDone := make(chan struct{})
		go func() {
			defer close(drainDone)

			_, _ = io.Copy(&out, reader)
		}()

		DeferCleanup(func() {
			os.Stdout = origStdout
			_ = writer.Close()
			_ = reader.Close()
		})

		os.Stdout = writer

		// When the scenario's Run fails, examples.Run tears down and prints
		// the dump before it returns, so the dump is complete when the call
		// returns.
		_, err = examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   failing,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
			DumpStore:    true,
		})
		Expect(err).To(MatchError(ContainSubstring("scenario gave up")))

		os.Stdout = origStdout

		Expect(writer.Close()).To(Succeed())
		Eventually(drainDone, "5s").Should(BeClosed())

		Expect(out.String()).To(ContainSubstring("CSE SCENARIO DUMP"),
			"a failed run must still print the store dump, because that is when it is most needed")
		Expect(out.String()).To(ContainSubstring("dump-failed-hello"),
			"the dump must list the worker the scenario created before it failed")
	})

	It("prints the store dump after a v2 scenario run", func() {
		logBuf := &v2LogBuffer{}
		logger := deps.NewJSONFSMLogger(logBuf, deps.LevelDebug)
		store := examples.SetupStore(logger)

		dumpRequested := examples.ScenarioV2{
			Name:        "dump-requested",
			Description: "test-local Run for the DumpStore print path",
			Run: func(ctx context.Context, env examples.Env) error {
				ref := dynamicchildren.Ref{WorkerType: "helloworld", Name: "dump-hello"}

				env.Step("create a helloworld child")

				if err := env.Client.Upsert(ref, map[string]any{"state": "running"}); err != nil {
					return err
				}

				return env.WaitFor(ctx, "the helloworld child reaches Running",
					func(ctx context.Context) (bool, string, error) {
						obs, err := fsmv2client.Get[hello_world.HelloworldStatus](ctx, env.Client, ref)
						if err != nil {
							if errors.Is(err, fsmv2client.ErrNotObserved) {
								return false, "the child has not published an observation yet", nil
							}

							return false, "", err
						}

						return obs.State == "Running", "state=" + obs.State, nil
					})
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		// The dump goes to os.Stdout, so the spec captures stdout for the
		// run's lifetime.
		origStdout := os.Stdout
		reader, writer, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())

		// The drain goroutine reads the pipe while the run writes to it. A
		// dump larger than the OS pipe buffer would otherwise block the
		// teardown goroutine's print, so Done would never close.
		var out bytes.Buffer

		drainDone := make(chan struct{})
		go func() {
			defer close(drainDone)

			_, _ = io.Copy(&out, reader)
		}()

		// DeferCleanup restores the original stdout and closes both pipe
		// ends even if an assertion fails during the redirect. Later specs
		// keep their output.
		DeferCleanup(func() {
			os.Stdout = origStdout
			_ = writer.Close()
			_ = reader.Close()
		})

		os.Stdout = writer

		result, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   dumpRequested,
			Duration:     time.Second,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
			DumpStore:    true,
		})
		Expect(err).NotTo(HaveOccurred(),
			"DumpStore must not break a v2 run")
		Eventually(result.Done, "55s").Should(BeClosed())

		// Closing the write end makes the drain goroutine see EOF and finish.
		os.Stdout = origStdout

		Expect(writer.Close()).To(Succeed())
		Eventually(drainDone, "5s").Should(BeClosed())

		Expect(out.String()).To(ContainSubstring("CSE SCENARIO DUMP"),
			"runV2 must print the store dump when DumpStore is set")
		Expect(out.String()).To(ContainSubstring("dump-hello"),
			"the dump must list the worker the scenario created")
		Expect(result.Err).NotTo(HaveOccurred(),
			"a clean dump run must not report a failure")
		// Every run that returns a RunResult logs v2_run_teardown_starting
		// during teardown. This positive control makes an empty or malformed
		// log capture fail the spec before the absence check below runs.
		Expect(logContainsEvent(logBuf.String(), "v2_run_teardown_starting")).To(BeTrue(),
			"the log capture must contain the teardown event that every run returning a RunResult emits")
		Expect(logContainsEvent(logBuf.String(), "dump_store_not_supported_for_v2")).To(BeFalse(),
			"the v2 path must not warn that DumpStore is unsupported")
	})

	It("tears down gracefully on a live tick loop when the caller ctx is cancelled mid-run", func() {
		logBuf := &v2LogBuffer{}
		logger := deps.NewJSONFSMLogger(logBuf, deps.LevelDebug)
		store := examples.SetupStore(logger)

		cancelMidRun := examples.ScenarioV2{
			Name:        "cancel-mid-run",
			Description: "test-local Run for the caller-ctx cancellation path",
			Run: func(_ context.Context, _ examples.Env) error {
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

		Expect(register.GlobalDeps[*dynamicchildren.Registry](configworker.WorkerTypeName)).To(BeNil(),
			"the deps key must be cleared: ClearGlobalDeps runs only after supDone, so a set key means the supervisor did not exit")

		// The graceful drain must run against a LIVE tick loop. If the tick
		// loop shared the caller's ctx, the cancel would kill it before
		// Shutdown, and every drain phase would wait out its timeout and
		// emit this warning.
		Expect(logContainsEvent(logBuf.String(), "graceful_shutdown_timeout")).To(BeFalse(),
			"the supervisor must drain via a live tick loop, not time out against a dead one")
	})

	It("lists noop in the merged registry and runs a v2 scenario end-to-end on the kernel-only supervisor", func() {
		// The v2 scenarios must appear in the same listing the CLI reads, so
		// --list and --scenario find every registered scenario.
		listing := examples.ListScenarios()
		Expect(listing).To(HaveKey("noop"),
			"merged ListScenarios must contain the v2 noop scenario")
		Expect(listing).To(HaveKey("helloworld"),
			"merged ListScenarios must contain the v2 helloworld scenario")

		// The sentinel bool proves the runner invoked Run; noop's own Run
		// returns nil at once, so it cannot.
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		runRan := false
		clientWasSet := false
		loggerWasSet := false
		sentinel := examples.ScenarioV2{
			Name:        "sentinel",
			Description: "test-local Run that records execution",
			Run: func(_ context.Context, env examples.Env) error {
				runRan = true
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

		Expect(runRan).To(BeTrue(),
			"the v2 runner must execute the scenario Run")
		Expect(clientWasSet).To(BeTrue(),
			"Env must carry a non-nil fsmv2client for Run")
		Expect(loggerWasSet).To(BeTrue(),
			"Env must carry the run's logger for Run")

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
		Expect(register.GlobalDeps[*dynamicchildren.Registry](configworker.WorkerTypeName)).To(BeNil(),
			"the v2 runner must ClearGlobalDeps the configworker key during teardown")
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
			Description: "test-local Run for the ShutdownClean plumbing",
			Run: func(_ context.Context, _ examples.Env) error {
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

	It("reports ShutdownClean=false when a v2 run's drain budget is exhausted", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		// A real worker must be running when teardown starts, so the drain
		// has a worker to stop.
		degradedDrain := examples.ScenarioV2{
			Name:        "degraded-drain",
			Description: "test-local Run for the exhausted drain budget",
			// A 1ns budget runs out before the drain can stop even one
			// worker, so the drain warns graceful_shutdown_timeout. The v2
			// teardown puts that warning into RunResult.Err, and this spec
			// asserts only the drain outcome, so the scenario declares the
			// warning here.
			ExpectedWarnings: []string{
				"graceful_shutdown_timeout",
				"graceful_shutdown_budget_exhausted",
			},
			Run: func(ctx context.Context, env examples.Env) error {
				ref := dynamicchildren.Ref{WorkerType: "helloworld", Name: "hello-1"}

				env.Step("create a helloworld child")

				if err := env.Client.Upsert(ref, map[string]any{"state": "running"}); err != nil {
					return err
				}

				return env.WaitFor(ctx, "the helloworld child reaches Running",
					func(ctx context.Context) (bool, string, error) {
						obs, err := fsmv2client.Get[hello_world.HelloworldStatus](ctx, env.Client, ref)
						if err != nil {
							if errors.Is(err, fsmv2client.ErrNotObserved) {
								return false, "the child has not published an observation yet", nil
							}

							return false, "", err
						}

						return obs.State == "Running", "state=" + obs.State, nil
					})
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   degradedDrain,
			Duration:     time.Second,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,

			GracefulShutdownTimeout: time.Nanosecond,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(result.Done, "55s").Should(BeClosed())

		Expect(result.ShutdownClean).To(BeFalse(),
			"a v2 run whose graceful drain budget is exhausted must report ShutdownClean=false")
		Expect(result.Err).NotTo(HaveOccurred(),
			"the drain warnings are in ExpectedWarnings, so they must not fail the run")
	})

	It("tears down and clears the deps key when Run fails", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		runErr := errors.New("boom")
		failing := examples.ScenarioV2{
			Name:        "failing-run",
			Description: "test-local Run that returns an error",
			Run: func(_ context.Context, _ examples.Env) error {
				return runErr
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
		Expect(err).To(MatchError(runErr),
			"the runner must wrap and propagate the Run error")
		Expect(err.Error()).To(ContainSubstring("failing-run"),
			"the error must name the failing scenario")
		Expect(result).To(BeNil())

		// The error path is a full teardown path: a leaked key would
		// cross-wire every later v2 run in this process.
		Expect(register.GlobalDeps[*dynamicchildren.Registry](configworker.WorkerTypeName)).To(BeNil(),
			"the v2 runner must ClearGlobalDeps the configworker key on Run failure")
	})

	It("runs forever with Duration=0 and tears down on context cancellation", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		runForever := examples.ScenarioV2{
			Name:        "run-forever",
			Description: "test-local Run for the Duration=0 ctx-cancel path",
			Run: func(_ context.Context, _ examples.Env) error {
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
		Expect(register.GlobalDeps[*dynamicchildren.Registry](configworker.WorkerTypeName)).To(BeNil(),
			"the v2 runner must ClearGlobalDeps the configworker key after ctx cancellation")
	})

	It("tears down and clears the deps key when Run panics", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		panicking := examples.ScenarioV2{
			Name:        "panicking-run",
			Description: "test-local Run that panics",
			Run: func(_ context.Context, _ examples.Env) error {
				panic("Run exploded")
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
		}).To(PanicWith("Run exploded"),
			"the runner must not swallow a Run panic")

		// A panic is a full teardown path too: a leaked key would
		// cross-wire every later v2 run in this process.
		Expect(register.GlobalDeps[*dynamicchildren.Registry](configworker.WorkerTypeName)).To(BeNil(),
			"the v2 runner must ClearGlobalDeps the configworker key on Run panic")
	})

	It("blocks a mid-Duration Shutdown until the deps key is cleared", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		longRun := examples.ScenarioV2{
			Name:        "long-run",
			Description: "test-local Run for the mid-Duration Shutdown path",
			Run: func(_ context.Context, _ examples.Env) error {
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
		Expect(register.GlobalDeps[*dynamicchildren.Registry](configworker.WorkerTypeName)).To(BeNil(),
			"the deps key must already be cleared when Shutdown returns")
	})

	It("supports back-to-back sequential v2 runs in one process", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		scenario := examples.ScenarioV2{
			Name:        "back-to-back",
			Description: "test-local Run for sequential v2 runs",
			Run: func(_ context.Context, _ examples.Env) error {
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

		Expect(register.GlobalDeps[*dynamicchildren.Registry](configworker.WorkerTypeName)).To(BeNil(),
			"run 2 must clear the deps key just like run 1 did")
	})

	It("hands a worker created with env.Client.Upsert the value Dependencies returned", func() {
		// Reset the record, so one from an earlier run of this spec cannot satisfy the wait below.
		scenarioDepsProbeSeen.Store(nil)

		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		delivering := examples.ScenarioV2{
			Name:        "deps-delivery",
			Description: "test-local Run for the dependency-delivery path",
			Dependencies: func() (map[string]any, func(), error) {
				scenarioDeps := map[string]any{}
				config.SetDependency(scenarioDeps, scenarioDepsProbeLabelKey, "from-the-scenario")

				return scenarioDeps, nil, nil
			},
			Run: func(_ context.Context, env examples.Env) error {
				ref := dynamicchildren.Ref{WorkerType: scenarioDepsProbeType, Name: "probe"}
				return env.Client.Upsert(ref, nil)
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   delivering,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred())

		// Settle gate: wait for the child's constructor, so teardown cannot race the Upsert.
		Eventually(scenarioDepsProbeSeen.Load, "30s").ShouldNot(BeNil(),
			"the upserted child's constructor must run while the scenario's supervisor is live")

		cancel()
		Eventually(result.Done, "55s").Should(BeClosed())

		record := scenarioDepsProbeSeen.Load()
		Expect(record.present).To(BeTrue(),
			"the value ScenarioV2.Dependencies returned must reach the constructor of a worker the Run upserts")
		Expect(record.label).To(Equal("from-the-scenario"))
	})

	It("hands Run the map Dependencies returned, so Run can reach and change its mocks", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		// The runner calls Dependencies and Run on the caller's goroutine, so the
		// spec reads mock.touched without synchronization.
		returnedDeps := map[string]any{}
		mock := &scenarioEnvMock{}

		reaching := examples.ScenarioV2{
			Name:        "env-deps-reach",
			Description: "test-local Run for the Env.Dependencies handoff",
			Dependencies: func() (map[string]any, func(), error) {
				config.SetDependency(returnedDeps, scenarioEnvMockKey, mock)

				return returnedDeps, nil, nil
			},
			Run: func(_ context.Context, env examples.Env) error {
				if env.Dependencies == nil {
					return errors.New("env.Dependencies is nil")
				}

				reached, ok := config.LookupDependency(env.Dependencies, scenarioEnvMockKey)
				if !ok {
					return errors.New("the mock key is missing from env.Dependencies")
				}

				reached.touched = true

				return nil
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   reaching,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred())

		cancel()
		Eventually(result.Done, "55s").Should(BeClosed())

		Expect(mock.touched).To(BeTrue(),
			"Run must reach the mock Dependencies built through env.Dependencies")
	})

	It("hands Run a nil map when the scenario declares no Dependencies", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		var recordedDeps map[string]any

		envNone := examples.ScenarioV2{
			Name:        "env-deps-none",
			Description: "test-local Run for the nil-Dependencies handoff",
			Run: func(_ context.Context, env examples.Env) error {
				recordedDeps = env.Dependencies

				return nil
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   envNone,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred())

		cancel()
		Eventually(result.Done, "55s").Should(BeClosed())
		Expect(recordedDeps).To(BeNil())
	})
})
