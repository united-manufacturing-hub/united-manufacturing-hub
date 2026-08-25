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
	"fmt"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
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

// logsContainMsg reports whether any parsed entry has the given Msg value.
func logsContainMsg(entries []examples.LogEntry, msg string) bool {
	for _, entry := range entries {
		if entry.Msg == msg {
			return true
		}
	}

	return false
}

// logsContainMsgAtLevel reports whether any parsed entry has both the given
// Msg and the given Level.
func logsContainMsgAtLevel(entries []examples.LogEntry, msg, level string) bool {
	for _, entry := range entries {
		if entry.Msg == msg && entry.Level == level {
			return true
		}
	}

	return false
}

// v2LogKey identifies a log entry by the attributes the completeness
// comparison in the ScenarioV2 specs counts: the struct's fields below.
// Fields is what separates same-message copies that report different
// things, so a dropped copy cannot hide behind later copies that report
// something different. Copies with identical fields share one key, so a
// dropped copy can hide behind an identical later copy; the
// checkCapturedEntries doc names that limit.
type v2LogKey struct {
	Level  string
	Msg    string
	Worker string
	// Fields holds the entry's structured key-value pairs in v2FieldsKey's
	// canonical form, so the key stays comparable.
	Fields string
}

// v2FieldsKey renders one side's structured fields canonically, so the
// buffer side and the Logs side build the same key for the same entry.
// json.Marshal sorts map keys, and both sides hold values that came from
// JSON, so the rendering is deterministic and identical. Both parsers build
// the map even for an entry without fields, so such entries render as {} on
// either side.
func v2FieldsKey(fields map[string]any) string {
	encoded, _ := json.Marshal(fields)
	return string(encoded)
}

// parseV2LogKeys turns the caller's captured JSON lines into v2LogKeys. It
// reports whether every non-empty line parsed: the runner drops an
// unparseable line from result.Logs, so an unparseable line in the caller's
// buffer is a missing entry the caller must fail on, not noise to skip.
func parseV2LogKeys(logOutput string) (keys []v2LogKey, allParsed bool) {
	allParsed = true

	for _, line := range strings.Split(logOutput, "\n") {
		if line == "" {
			continue
		}

		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			allParsed = false

			continue
		}

		key := v2LogKey{}
		if level, ok := raw["level"].(string); ok {
			key.Level = level
		}

		if msg, ok := raw["msg"].(string); ok {
			key.Msg = msg
		}

		if worker, ok := raw["worker"].(string); ok {
			key.Worker = worker
		}

		// The runner's parseLogEntries keeps the reserved keys out of
		// Fields, so the buffer side drops the same keys here or the two
		// sides would never agree on one entry's fields.
		fields := make(map[string]any, len(raw))
		for k, val := range raw {
			switch k {
			case "level", "msg", "worker", "ts":
				continue
			}

			fields[k] = val
		}
		key.Fields = v2FieldsKey(fields)

		keys = append(keys, key)
	}

	return keys, allParsed
}

// v2LogKeyFromEntry builds the Logs-side key for one parsed entry, so the
// comparison in checkCapturedEntries counts entries by the same attributes
// on both sides.
func v2LogKeyFromEntry(entry examples.LogEntry) v2LogKey {
	return v2LogKey{
		Level:  entry.Level,
		Msg:    entry.Msg,
		Worker: entry.Worker,
		Fields: v2FieldsKey(entry.Fields),
	}
}

// checkCapturedEntries judges a run's captured entries against the caller's
// own buffer, with two comparisons. The prefix-containment check demands
// that every key the buffer holds before the sentinel appear in logs at
// least as often; the reverse check demands that no key appear in logs more
// often than in the whole buffer. It returns nil only when both hold.
//
// The sentinel is an entry the calling spec logs through the caller's
// logger at the moment it begins shutdown: it reaches the caller's buffer
// alone and never the run's capture, so it marks the shutdown boundary in
// the buffer instead of claiming capture. It must appear in the buffer
// exactly once, with at least one entry before it, or the
// prefix-containment check has no bounded range. A second entry carrying
// the sentinel's message would bound the range at the first and silently
// demand less than the caller marked. Every non-empty buffer line must
// parse, because the runner drops an unparseable line from result.Logs, so
// an unparseable line in the caller's buffer is a missing entry rather than
// noise.
//
// The prefix-containment check treats the buffer as the control: every
// entry the run emitted through its tee reached it, so a capture that
// dropped an entry the run emitted before shutdown began is a mismatch.
// Logs entries carry no position, so the check cannot bound the logs side
// at the shutdown boundary: it counts each key over all of logs, which
// makes a dropped copy detectable only when no copy after the sentinel
// carries an identical full key. A later tick emitting the same fields
// again stands in for the dropped copy, and the check passes.
//
// The reverse check holds because the specs that run this check hand the
// run a caller logger at the capture's own debug level, unsampled, so every
// entry teed to the caller also reached its buffer. Teeing alone would not
// imply receipt: a caller logging at info filters debug entries out of its
// buffer, and a sampling caller drops entries at random. Under that
// precondition, a key that appears in logs but never in the buffer means
// the runner captured an entry without teeing it to the caller.
//
// Both comparisons count entries per key rather than comparing positions,
// because the tee's two sinks take their locks independently and may order
// concurrent entries differently. The sentinel bounds only the
// prefix-containment check: writes that trail the teardown parse appear in
// the buffer but never in logs and would fail it on every correct run.
func checkCapturedEntries(logOutput string, logs []examples.LogEntry, sentinelMsg string) error {
	bufferKeys, allParsed := parseV2LogKeys(logOutput)
	if !allParsed {
		return errors.New("every line in the caller's buffer must parse, or the runner dropped an unparseable line from result.Logs")
	}

	sentinelCount := 0
	sentinelIdx := -1
	for i, key := range bufferKeys {
		if key.Msg != sentinelMsg {
			continue
		}

		sentinelCount++
		if sentinelIdx < 0 {
			sentinelIdx = i
		}
	}

	if sentinelCount == 0 {
		return errors.New("the sentinel must appear in the caller's buffer, or the prefix-containment check has no bounded range")
	}

	if sentinelCount > 1 {
		return fmt.Errorf("the sentinel must appear exactly once in the caller's buffer: %d entries carry its message, and the check bounds its range at the first",
			sentinelCount)
	}

	if sentinelIdx == 0 {
		return errors.New("entries must appear in the buffer before the sentinel, or the prefix-containment check is vacuous")
	}

	logsCounts := map[v2LogKey]int{}
	for _, entry := range logs {
		logsCounts[v2LogKeyFromEntry(entry)]++
	}

	prefixCounts := map[v2LogKey]int{}
	for _, key := range bufferKeys[:sentinelIdx] {
		prefixCounts[key]++
	}

	for key, want := range prefixCounts {
		if have := logsCounts[key]; have < want {
			return fmt.Errorf("an entry the run emitted before shutdown began is missing from result.Logs: level=%s msg=%s worker=%q appears %d times in the caller's buffer before the sentinel and only %d times in Logs",
				key.Level, key.Msg, key.Worker, want, have)
		}
	}

	fullBufferCounts := map[v2LogKey]int{}
	for _, key := range bufferKeys {
		fullBufferCounts[key]++
	}

	for key, have := range logsCounts {
		if seen := fullBufferCounts[key]; have > seen {
			return fmt.Errorf("result.Logs holds an entry the caller's buffer never received: level=%s msg=%s worker=%q appears %d times in Logs but only %d times in the caller's buffer",
				key.Level, key.Msg, key.Worker, have, seen)
		}
	}

	return nil
}

// waitForConfigworkerChildAdded blocks until the caller's buffer holds the
// application supervisor's child_reconciliation_completed report for the
// tick that added the configworker child: the entry carrying added=1. The
// observed store state cannot settle a spec on this: the document AddWorker
// saves for a just-added child already carries the child's registered
// initial state (Running for the configworker), so the store reports
// Running while the reconcile that adds the child is still mid-flight. The
// added=1 report is written at that reconcile's end, so once it is in the
// buffer the child is added rather than mid-spawn, and whatever the spec
// writes next (a sentinel, a cancel) lands after the report in the
// caller's buffer.
func waitForConfigworkerChildAdded(logOutput func() string, appWorker string) {
	Eventually(func(g Gomega) {
		found := false
		for _, line := range strings.Split(logOutput(), "\n") {
			if line == "" {
				continue
			}

			var raw map[string]any
			if err := json.Unmarshal([]byte(line), &raw); err != nil {
				continue
			}

			if raw["msg"] == "child_reconciliation_completed" &&
				raw["worker"] == appWorker &&
				raw["added"] == float64(1) {
				found = true

				break
			}
		}

		g.Expect(found).To(BeTrue(),
			"the caller's buffer must hold the application supervisor's added=1 reconciliation report")
	}, "30s").Should(Succeed(),
		"the run must finish the reconcile that adds the configworker child before the spec proceeds")
}

// errStoreSaveFailed is the error failingSavesStore's SaveIdentity returns,
// so a spec can recognise its own injected failure in the error Run
// propagates.
var errStoreSaveFailed = errors.New("store save failed")

// failingSavesStore is a TriangularStore whose SaveIdentity fails, so
// building a supervisor against it cannot persist the application worker's
// initial documents and construction fails.
type failingSavesStore struct {
	storage.TriangularStoreInterface
}

func (s *failingSavesStore) SaveIdentity(_ context.Context, _ string, _ string, _ persistence.Document) error {
	return errStoreSaveFailed
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

	It("clears the configworker deps key when supervisor construction fails, so a later run can publish it", func() {
		logger := deps.NewNopFSMLogger()

		// Run 1 fails while the supervisor is being built: construction
		// cannot persist the application worker's initial documents. By
		// then runV2 has already published the deps key, so this is the
		// path that leaks it.
		failingConstruction := examples.ScenarioV2{
			Name:        "construction-fails",
			Description: "test-local driver for the supervisor-construction failure path",
			Driver: func(_ context.Context, _ examples.Env) error {
				return nil
			},
		}

		// Cancellable so a regression that lets construction succeed still
		// tears run 1 down: with no Duration, ctx cancellation is the only
		// teardown trigger.
		ctx1, cancel1 := context.WithCancel(context.Background())
		defer cancel1()

		result1, err := examples.Run(ctx1, examples.RunConfig{
			ScenarioV2:   failingConstruction,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        &failingSavesStore{TriangularStoreInterface: examples.SetupStore(logger)},
		})
		Expect(err).To(MatchError(ContainSubstring(errStoreSaveFailed.Error())),
			"the run must fail on the injected store failure, proving construction was reached and failed")
		Expect(result1).To(BeNil())

		// The key must be cleared even though the supervisor never came to exist.
		Expect(register.GetDeps[*dynamicchildren.Registry](configworker.WorkerTypeName)).To(BeNil(),
			"a failed construction must clear the configworker deps key")

		// A nil key cannot distinguish cleared from never-published; only a
		// later run proves publication works. It must share this spec,
		// because the suite's DeferCleanup clears the key between specs.
		laterRun := examples.ScenarioV2{
			Name:        "later-run",
			Description: "test-local driver proving a run can follow a failed construction",
			Driver: func(_ context.Context, _ examples.Env) error {
				return nil
			},
		}

		ctx2, cancel2 := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel2()

		result2, err := examples.Run(ctx2, examples.RunConfig{
			ScenarioV2:   laterRun,
			Duration:     time.Second,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        examples.SetupStore(logger),
		})
		Expect(err).NotTo(HaveOccurred(),
			"a later v2 run must start once the failed construction cleared the key")
		Eventually(result2.Done, "55s").Should(BeClosed(),
			"the later run must complete end-to-end")
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

		// The warning is routed through the run's tee logger, so it must
		// also sit in result.Logs; routing it to RunConfig.Logger alone
		// keeps the buffer assertion above passing while dropping it from
		// Logs.
		Expect(logsContainMsg(result.Logs, "dump_store_not_supported_for_v2")).To(BeTrue(),
			"the DumpStore warning must reach result.Logs")
	})

	It("delivers the run's log entries to the caller as result.Logs, complete and typed", func() {
		logBuf := &v2LogBuffer{}
		logger := deps.NewJSONFSMLogger(logBuf, deps.LevelDebug)
		store := examples.SetupStore(logger)

		// The driver logs through Env.Logger: the run hands the driver its
		// tee logger, so the entry must reach the caller's sink AND
		// result.Logs. A driver that logs nothing (NoopScenarioV2's) leaves
		// that routing unobserved. A runner that hands the driver
		// RunConfig.Logger instead leaves the entry in the caller's sink
		// but out of Logs, which the prefix-containment check already
		// fails; the driver-entry check is what catches an entry lost to
		// both sinks.
		loggingDriver := examples.ScenarioV2{
			Name:        "logs-complete",
			Description: "test-local driver that logs one entry through Env.Logger",
			Driver: func(_ context.Context, env examples.Env) error {
				env.Logger.Info("driver_logged_entry")

				return nil
			},
		}

		// Duration=0 leaves ctx-cancellation as the only teardown trigger
		// (ScenarioV2.Driver's doc), so this spec decides when shutdown
		// begins and can mark that moment in the caller's buffer.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   loggingDriver,
			Duration:     0,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred())

		const appWorker = "scenariov2-logs-complete(application)"

		// Settle gate: the claims below need the prefix to hold a live tick
		// loop's output rather than startup alone, and the added=1 report
		// is that output, so wait for it before marking shutdown.
		waitForConfigworkerChildAdded(logBuf.String, appWorker)

		// The sentinel marks the shutdown boundary in the caller's buffer
		// (checkCapturedEntries' doc holds its contract), logged right
		// before the cancel that begins shutdown.
		logger.Info("spec_sentinel_before_shutdown")

		cancel()
		// Once Done closes, result.Logs is final (RunResult.Logs' doc:
		// teardown parsed the capture first, and trailing writes are that
		// doc's named exception), which is why the claim below stops at the
		// sentinel and asserts nothing about what follows it.
		Eventually(result.Done, "55s").Should(BeClosed())

		// Completeness, judged against the caller's own sink; the
		// checkCapturedEntries doc holds the contract.
		Expect(checkCapturedEntries(logBuf.String(), result.Logs, "spec_sentinel_before_shutdown")).To(Succeed(),
			"every entry the caller's buffer holds before the sentinel must appear in result.Logs, and result.Logs must hold no entry the whole buffer lacks")

		// The driver's entry: proves Env.Logger is the run's tee, so driver
		// entries reach the caller's sink and Logs alike.
		Expect(logsContainMsg(result.Logs, "driver_logged_entry")).To(BeTrue(),
			"an entry the driver logs through Env.Logger must reach result.Logs")

		// The store's entry: SetupStore's logger swap points the store at
		// the run's tee, so the store's identity_created at debug lands in
		// Logs. The level is what makes the check discriminate: the
		// supervisor also emits identity_created, at info and through the
		// tee by construction, so matching the message alone still passes
		// with the swap disabled; only the level check distinguishes the
		// store's entry from the supervisor's.
		Expect(logsContainMsgAtLevel(result.Logs, "identity_created", "debug")).To(BeTrue(),
			"the store's identity_created at debug must reach result.Logs through the swapped store logger")

		// One known entry arrives typed. Every run drives the config worker
		// kernel child through state transitions, and the supervisor logs
		// each one as a state_transition at info, naming the child in the
		// worker field and carrying from_state/to_state/reason. The entry
		// must reach the caller parsed, not merely present: exact level,
		// exact worker, its transition keys, and no reserved key leaking
		// into Fields.
		const configWorkerChild = "scenariov2-logs-complete(application)/config-worker-001(configworker)"
		var typed *examples.LogEntry
		for i := range result.Logs {
			if result.Logs[i].Msg == "state_transition" && result.Logs[i].Worker == configWorkerChild {
				entry := result.Logs[i]
				typed = &entry

				break
			}
		}
		Expect(typed).NotTo(BeNil(),
			"result.Logs must contain the config worker child's state_transition entry")
		Expect(typed.Level).To(Equal("info"),
			"state_transition is logged at info, so a parsed entry carries its exact level")
		Expect(typed.Worker).To(Equal(configWorkerChild),
			"a parsed entry names the worker that emitted it, by its hierarchy path")
		Expect(typed.Fields).To(HaveKey("from_state"),
			"state_transition carries from_state")
		Expect(typed.Fields).To(HaveKey("to_state"),
			"state_transition carries to_state")
		Expect(typed.Fields).NotTo(BeEmpty(),
			"a parsed entry carries its structured fields")
		for _, reserved := range []string{"ts", "level", "msg", "worker"} {
			Expect(typed.Fields).NotTo(HaveKey(reserved),
				"the parser must lift %s out of Fields, not copy it", reserved)
		}
	})

	It("detects a dropped entry whose key recurs after the sentinel, and an entry Logs holds that the caller's buffer never received", func() {
		logBuf := &v2LogBuffer{}
		logger := deps.NewJSONFSMLogger(logBuf, deps.LevelDebug)
		store := examples.SetupStore(logger)

		quietDriver := examples.ScenarioV2{
			Name:        "capture-check",
			Description: "test-local driver for the corrupted-capture checks",
			Driver: func(_ context.Context, _ examples.Env) error {
				return nil
			},
		}

		// Duration=0 leaves ctx-cancellation as the only teardown trigger
		// (ScenarioV2.Driver's doc), so this spec decides when shutdown
		// begins and can mark that moment in the caller's buffer.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   quietDriver,
			Duration:     0,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred())

		const appWorker = "scenariov2-capture-check(application)"

		// Settle gate: the entry this spec drops is the reconciliation
		// report for the tick that added the configworker child, so that
		// report must appear in the caller's buffer before the sentinel,
		// or the drop removes an entry the sentinel does not bound.
		waitForConfigworkerChildAdded(logBuf.String, appWorker)

		// The sentinel marks the shutdown boundary in the caller's buffer
		// (checkCapturedEntries' doc holds its contract): the drop below
		// must remove an entry that precedes it.
		logger.Info("spec_sentinel_before_shutdown")

		cancel()
		// Once Done closes, result.Logs is final (RunResult.Logs' doc), so
		// the corruptions below run on a frozen slice.
		Eventually(result.Done, "55s").Should(BeClosed())

		// Positive control: the intact pair must pass, or the two
		// corruptions below cannot prove the check detects them.
		Expect(checkCapturedEntries(logBuf.String(), result.Logs, "spec_sentinel_before_shutdown")).To(Succeed(),
			"the intact run must satisfy the captured-entry check")

		// Corruption one: drop the reconciliation entry for the tick that
		// added the configworker child. Its level, message and worker also
		// appear after the sentinel: the pre-sentinel copy carries
		// added=1, while the copies teardown emits after the sentinel
		// report an updated child instead. Match the entry by its added
		// field rather than by position, so the drop finds the
		// pre-sentinel copy whatever order Logs holds.
		dropped := make([]examples.LogEntry, 0, len(result.Logs))
		droppedAdded := 0
		for _, entry := range result.Logs {
			if entry.Msg == "child_reconciliation_completed" &&
				entry.Worker == appWorker &&
				entry.Fields["added"] == float64(1) {
				droppedAdded++

				continue
			}

			dropped = append(dropped, entry)
		}
		Expect(droppedAdded).To(Equal(1),
			"Logs must hold exactly one reconciliation entry reporting the added child, or the drop has no single target")

		// The drop must leave the hole this spec guards against: an entry
		// whose level, message and worker recur after the sentinel. Counted
		// by those three alone, the later copies stand in for the dropped
		// copy and the check cannot see the drop; the entry's structured
		// fields are what separate it from the later copies. Only buffer
		// entries carry position, so the recurrence is counted there, past
		// the sentinel; counting logs entries instead would let pre-sentinel
		// copies satisfy a claim about post-sentinel ones.
		bufferKeys, _ := parseV2LogKeys(logBuf.String())
		sentinelIdx := -1
		for i, key := range bufferKeys {
			if key.Msg == "spec_sentinel_before_shutdown" {
				sentinelIdx = i

				break
			}
		}

		recurring := 0
		for _, key := range bufferKeys[sentinelIdx+1:] {
			if key.Msg == "child_reconciliation_completed" && key.Worker == appWorker {
				recurring++
			}
		}
		Expect(recurring).To(BeNumerically(">=", 1),
			"the buffer must hold another child_reconciliation_completed entry after the sentinel, or the dropped key would not recur and the count would already catch the drop")

		dropErr := checkCapturedEntries(logBuf.String(), dropped, "spec_sentinel_before_shutdown")
		Expect(dropErr).To(HaveOccurred(),
			"dropping the pre-sentinel reconciliation entry must fail the check, because its structured fields distinguish it from the later copies")
		Expect(dropErr.Error()).To(ContainSubstring("child_reconciliation_completed"),
			"the failure must name the entry the check found missing")

		// Corruption two: add an entry only Logs holds. A run entry reaches
		// the caller's buffer and the capture together or not at all, so an
		// entry Logs holds that the buffer never received means the runner
		// captured without teeing to the caller. Build the pair from the
		// intact Logs, so the fabricated entry is the only difference from
		// the passing control.
		withForeign := append(append([]examples.LogEntry{}, result.Logs...), examples.LogEntry{
			Level:  "info",
			Msg:    "entry_no_run_emits",
			Worker: appWorker,
		})
		foreignErr := checkCapturedEntries(logBuf.String(), withForeign, "spec_sentinel_before_shutdown")
		Expect(foreignErr).To(HaveOccurred(),
			"an entry Logs holds that the caller's buffer never received must fail the check")
		Expect(foreignErr.Error()).To(ContainSubstring("entry_no_run_emits"),
			"the failure must name the entry the caller's buffer never received")
	})

	It("errors on an unparseable buffer line and on a missing, leading, or duplicated sentinel", func() {
		logs := []examples.LogEntry{{Level: "info", Msg: "real_entry", Worker: "worker"}}

		// One non-JSON line: the runner drops unparseable lines from
		// result.Logs, so a buffer line that cannot parse is a missing
		// entry, not noise.
		unparseable := "{\"level\":\"info\",\"msg\":\"real_entry\",\"worker\":\"worker\"}\nnot json\n"
		Expect(checkCapturedEntries(unparseable, logs, "spec_sentinel_before_shutdown")).
			To(MatchError(ContainSubstring("must parse")))

		// No sentinel: the prefix-containment check has no bound, so the
		// comparison cannot claim anything.
		noSentinel := "{\"level\":\"info\",\"msg\":\"real_entry\",\"worker\":\"worker\"}\n"
		Expect(checkCapturedEntries(noSentinel, logs, "spec_sentinel_before_shutdown")).
			To(MatchError(ContainSubstring("no bounded range")))

		// Sentinel first: the prefix is empty and the prefix-containment
		// check is vacuous.
		sentinelFirst := "{\"level\":\"info\",\"msg\":\"spec_sentinel_before_shutdown\"}\n" +
			"{\"level\":\"info\",\"msg\":\"real_entry\",\"worker\":\"worker\"}\n"
		Expect(checkCapturedEntries(sentinelFirst, logs, "spec_sentinel_before_shutdown")).
			To(MatchError(ContainSubstring("vacuous")))

		// Two entries carrying the sentinel's message: the check would
		// bound its range at the first and silently demand less than the
		// caller marked, so a duplicate must fail loudly.
		duplicateSentinel := "{\"level\":\"info\",\"msg\":\"real_entry\",\"worker\":\"worker\"}\n" +
			"{\"level\":\"info\",\"msg\":\"spec_sentinel_before_shutdown\"}\n" +
			"{\"level\":\"info\",\"msg\":\"real_entry\",\"worker\":\"worker\"}\n" +
			"{\"level\":\"info\",\"msg\":\"spec_sentinel_before_shutdown\"}\n"
		Expect(checkCapturedEntries(duplicateSentinel, logs, "spec_sentinel_before_shutdown")).
			To(MatchError(ContainSubstring("exactly once")))
	})

	It("holds debug entries in result.Logs even when the caller's logger drops them", func() {
		// The capture side of the run's tee is a debug-level logger by
		// construction, whatever sink the caller passed. A caller logging at
		// LevelInfo never receives a debug entry, so a debug entry in
		// result.Logs proves the capture is not filtered through the
		// caller's logger: a runner that captured from the caller's sink
		// would hold none.
		logBuf := &v2LogBuffer{}
		logger := deps.NewJSONFSMLogger(logBuf, deps.LevelInfo)
		store := examples.SetupStore(logger)

		quietDriver := examples.ScenarioV2{
			Name:        "info-level-caller",
			Description: "test-local driver for the capture-level guarantee",
			Driver: func(_ context.Context, _ examples.Env) error {
				return nil
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   quietDriver,
			Duration:     time.Second,
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(result.Done, "55s").Should(BeClosed())

		hasDebugEntry := false
		for _, entry := range result.Logs {
			if entry.Level == "debug" {
				hasDebugEntry = true

				break
			}
		}
		Expect(hasDebugEntry).To(BeTrue(),
			"result.Logs must hold a debug entry even though the caller's LevelInfo logger drops every debug entry it receives")
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

		const appWorker = "scenariov2-cancel-mid-run(application)"

		// Settle gate: the cancellation must land past the child's spawn,
		// because a mid-spawn child intermittently cannot finish draining
		// within one graceful-shutdown phase budget.
		waitForConfigworkerChildAdded(logBuf.String, appWorker)

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
		// application supervisor spawns is the config worker kernel.
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
		// run (ScenarioV2.Driver's doc); a regression here leaves such a run
		// hanging forever.
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
