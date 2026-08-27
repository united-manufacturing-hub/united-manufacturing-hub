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

package examples

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"

	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/state"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild/state"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplefailing"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplefailing/state"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplepanic"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplepanic/state"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent/state"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleslow"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleslow/state"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld/state"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/pull"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/pull/state"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push/state"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/state"
)

// RunResult contains the result of running a scenario.
//
// Done and Shutdown carry different guarantees per scenario form. Done closes
// when teardown is complete: for a v1 scenario that is when the supervisor
// has stopped and its cleanup ran (plus the dump when DumpStore is set); for
// a v2 scenario it additionally includes clearing the published configworker
// deps key, which is what makes back-to-back v2 runs safe. Shutdown initiates
// teardown: the v1 Shutdown does not wait for Done (the DumpStore summary may
// still be printing when it returns), while the v2 Shutdown blocks until Done
// so the deps key is already cleared when it returns.
type RunResult struct {
	Done     <-chan struct{}
	Shutdown func()
	// Logs holds every entry the capture had received when teardown parsed
	// it; writes that trail teardown are absent — runV2's teardown comment
	// names the three classes. It is nil on the v1 YAML and CustomRunner
	// paths, which emit to the caller's logger but are not captured. The
	// capture logs at debug level through an unsampled logger regardless of
	// the caller's logger configuration, so Logs can contain entries a
	// filtered or sampling caller sink never received. Store-originated
	// entries appear only when the store came from SetupStore. Read it after
	// Done closes.
	Logs []LogEntry
	// ShutdownClean reports whether the run's supervisor drained cleanly on
	// both the v1 and v2 paths: true if the graceful shutdown reaped every
	// worker within its budget. It is false only when a drain phase warned
	// graceful_shutdown_timeout or graceful_shutdown_budget_exhausted. Read it
	// after Done closes.
	ShutdownClean bool
}

// LogEntry is one entry a v2 run emitted to the caller's logger.
type LogEntry struct {
	// Fields holds the entry's structured key-value pairs, excluding the
	// keys parsed into Level, Msg and Worker, and the timestamp.
	Fields map[string]any
	// Level is the severity the entry was logged at ("debug", "info",
	// "warn", "error").
	Level string

	// Msg is the log message.
	Msg string

	// Worker names the worker that emitted the entry, from the "worker"
	// context field per-worker loggers carry. It is empty for runner-level
	// entries.
	Worker string
}

// Run executes a scenario with the given configuration.
//
// For a v1 Scenario, creates an ApplicationSupervisor with the scenario's
// YAML config, or delegates to CustomRunner if set. For a ScenarioV2 (Driver
// set), takes the kernel-only v2 path (see runV2). Exactly one of Scenario
// and ScenarioV2 may be populated; setting both is an error.
//
// On both the YAML and v2 paths the supervisor's tick loop runs on a context
// detached from ctx: cancelling ctx triggers a graceful teardown against the
// live tick loop instead of killing the loop and forcing the drain to wait
// out its timeouts. CustomRunner scenarios own their supervisor lifecycle.
//
// If DumpStore is enabled, the YAML path prints a store changes summary
// after the run. The v2 path does not support DumpStore yet: runV2 logs a
// warning and ignores it. CustomRunner scenarios receive cfg.DumpStore and
// are responsible for their own dump handling; Run does not dump for them.
func Run(ctx context.Context, cfg RunConfig) (*RunResult, error) {
	hasYAML := cfg.Scenario.YAMLConfig != ""
	hasCustom := cfg.Scenario.CustomRunner != nil
	hasV1 := hasYAML || hasCustom
	hasV2 := cfg.ScenarioV2.Driver != nil || cfg.ScenarioV2.Name != ""

	if hasV1 && hasV2 {
		return nil, fmt.Errorf("conflicting configuration: both Scenario %q and ScenarioV2 %q are set (only one allowed)",
			cfg.Scenario.Name, cfg.ScenarioV2.Name)
	}

	if hasV2 && cfg.ScenarioV2.Driver == nil {
		return nil, fmt.Errorf("v2 scenario %q is not properly configured: Name is set but Driver is nil",
			cfg.ScenarioV2.Name)
	}

	if cfg.ScenarioV2.Driver != nil && cfg.ScenarioV2.Name == "" {
		return nil, errors.New("v2 scenario is not properly configured: " +
			"Driver is set but Name is empty, so logs and the supervisor ID could not name the scenario")
	}

	if cfg.ScenarioV2.Driver != nil {
		return runV2(ctx, cfg)
	}

	if !hasYAML && !hasCustom {
		return nil, fmt.Errorf("scenario %q is not properly configured: "+
			"neither YAMLConfig nor CustomRunner is set", cfg.Scenario.Name)
	}

	if hasYAML && hasCustom {
		return nil, fmt.Errorf("scenario %q has conflicting configuration: "+
			"both YAMLConfig and CustomRunner are set (only one allowed)", cfg.Scenario.Name)
	}

	if hasCustom {
		return cfg.Scenario.CustomRunner(ctx, cfg)
	}

	var startSyncID int64

	if cfg.DumpStore {
		var err error

		startSyncID, err = cfg.Store.GetLatestSyncID(ctx)
		if err != nil {
			cfg.Logger.SentryWarn(deps.FeatureExamples, "", "sync_id_fetch_failed",
				deps.Err(err),
				deps.String("impact", "dump_shows_all_changes"))
		}
	}

	appSup, err := application.NewApplicationSupervisor(application.SupervisorConfig{
		ID:                      "scenario-" + cfg.Scenario.Name,
		Name:                    cfg.Scenario.Name,
		Store:                   cfg.Store,
		Logger:                  cfg.Logger,
		TickInterval:            cfg.TickInterval,
		YAMLConfig:              cfg.Scenario.YAMLConfig,
		EnableTraceLogging:      cfg.EnableTraceLogging,
		GracefulShutdownTimeout: cfg.GracefulShutdownTimeout,
	})
	if err != nil {
		return nil, err
	}

	// Detached from the caller's ctx so cancelling the caller's ctx triggers
	// teardown (via the watcher below) instead of killing the tick loop;
	// Shutdown cancels the supervisor's own derived context in its final phase.
	supDone := appSup.Start(context.WithoutCancel(ctx))

	done := make(chan struct{})

	shutdownFn := func() {
		appSup.Shutdown()
	}

	// result is updated by the watcher goroutine before it closes done, so a
	// caller that reads result.ShutdownClean after <-result.Done observes the
	// supervisor's drain outcome. The close(done) at the end of the goroutine
	// establishes the happens-before edge: the field write precedes the close,
	// and the caller's receive synchronizes-with it.
	result := &RunResult{Shutdown: shutdownFn}

	// Watcher: turns caller-ctx cancellation into a graceful teardown against
	// the LIVE tick loop. Shutdown runs unconditionally on both arms because
	// it is idempotent, and a supervisor that stopped on its own still needs
	// its executor and collectors stopped.
	go func() {
		select {
		case <-ctx.Done():
		case <-supDone:
		}

		appSup.Shutdown()
		<-supDone
		// DrainOutcomeClean is valid only after supDone: the drain budget is
		// spent during Shutdown's synchronous phases, which complete before
		// the tick loop signals supDone.
		result.ShutdownClean = appSup.DrainOutcomeClean()

		close(done)
	}()

	if cfg.DumpStore {
		wrappedDone := make(chan struct{})

		// wrappedDone waits on done, so the watcher's ShutdownClean write and
		// close(done) happen-before this goroutine runs; a caller reading
		// result.ShutdownClean after <-wrappedDone observes the drain outcome.
		go func() {
			<-done

			dumpCtx := context.Background()

			dump, err := DumpScenario(dumpCtx, cfg.Store, startSyncID)
			if err != nil {
				cfg.Logger.SentryWarn(deps.FeatureExamples, "", "scenario_dump_failed",
					deps.Err(err))
			} else {
				fmt.Print(dump.FormatHuman())
			}

			close(wrappedDone)
		}()

		result.Done = wrappedDone

		return result, nil
	}

	result.Done = done

	return result, nil
}

// runV2 executes a v2 scenario on the kernel-only application supervisor (no
// YAML children, so the config worker kernel is the only child).
//
// runV2 publishes the process-global configworker deps key before it builds
// the supervisor, because the application worker reads the dynamicchildren
// registry under that key every tick. If construction fails after that
// publish, the construction error path clears the key itself: no supervisor
// ever came to exist, so no teardown will. Once the supervisor exists, the
// key is cleared on every exit path, including a Driver panic, strictly
// after the supervisor has stopped. Clearing the key earlier flips the
// application worker's RegistryConfigured observation mid-shutdown; a key
// that is never cleared makes every later runV2 in the same process fail
// its already-published check below.
//
// The supervisor runs on a context detached from the caller's ctx. The
// caller's ctx drives the Driver, the Duration wait, and the teardown
// trigger, but never the tick loop: if the tick loop shared the caller's
// ctx, cancelling it would stop ticking before Shutdown runs, and the
// graceful drain would wait out its full timeout against a stopped loop.
//
// After the Driver returns nil, the runner waits RunConfig.Duration —
// ScenarioV2.Driver's doc states what 0 means and when the wait can end
// early — then shuts the supervisor down.
//
// Because the deps key is process-global, v2 runs must not overlap within a
// process. The already-published check below catches sequential overlap (a
// previous run whose teardown has not finished); it does not catch truly
// concurrent runV2 calls, because the check and the publish are two separate
// lock acquisitions. Concurrent runV2 calls are not supported.
func runV2(ctx context.Context, cfg RunConfig) (*RunResult, error) {
	if register.GetDeps[*dynamicchildren.Registry](configworker.WorkerTypeName) != nil {
		return nil, fmt.Errorf("v2 scenario %q cannot start: the configworker deps key is already published, "+
			"so another v2 run is still active in this process", cfg.ScenarioV2.Name)
	}

	// runLogger is the run's tee logger: one arm is the caller's logger,
	// the other captures every entry the run emits so RunResult.Logs can
	// hold them parsed. Teeing (rather than replacing the caller's logger)
	// keeps every entry flowing to whatever sink the caller passed.
	capture := &syncBuffer{}
	runLogger := teeLogger{
		a: cfg.Logger,
		b: deps.NewJSONFSMLogger(capture, deps.LevelDebug),
	}

	if cfg.DumpStore {
		runLogger.SentryWarn(deps.FeatureExamples, "", "dump_store_not_supported_for_v2",
			deps.String("scenario", cfg.ScenarioV2.Name),
			deps.String("impact", "no_store_dump_printed"))
	}

	writer := dynamicchildren.NewWriter()
	register.SetDeps[*dynamicchildren.Registry](configworker.WorkerTypeName, writer.Registry())

	// The store logs its own entries (observed_changed and friends) through
	// the logger it was built with, which is the caller's, so without this
	// swap those entries would reach the caller's sink but not Logs. The swap
	// precedes supervisor construction because construction already saves the
	// application worker's documents; the restore in teardown runs strictly
	// after the supervisor stopped. Store activity is not over by then —
	// swappableLogger's doc says what makes that overlap safe.
	restoreStoreLogger := swapStoreLogger(cfg.Store, runLogger)

	appSup, err := application.NewApplicationSupervisor(application.SupervisorConfig{
		ID:                      "scenariov2-" + cfg.ScenarioV2.Name,
		Name:                    cfg.ScenarioV2.Name,
		Store:                   cfg.Store,
		Logger:                  runLogger,
		TickInterval:            cfg.TickInterval,
		EnableTraceLogging:      cfg.EnableTraceLogging,
		GracefulShutdownTimeout: cfg.GracefulShutdownTimeout,
	})
	if err != nil {
		// This return is reached before the teardown closure below and
		// the defer that calls it are, so nothing else clears the deps
		// key that register.SetDeps published above: this ClearDeps is
		// the only cleanup that key gets on this path.
		register.ClearDeps(configworker.WorkerTypeName)
		return nil, fmt.Errorf("scenario %q supervisor construction failed: %w", cfg.ScenarioV2.Name, err)
	}

	// Detached from the caller's ctx so cancelling the caller's ctx triggers
	// teardown (via the selects below) instead of killing the tick loop;
	// Shutdown cancels the supervisor's own derived context in its final phase.
	supDone := appSup.Start(context.WithoutCancel(ctx))

	// result is updated by the teardown goroutine before it closes done, so a
	// caller that reads result.ShutdownClean after <-result.Done observes the
	// supervisor's drain outcome. The close(done) at the end of the goroutine
	// establishes the happens-before edge: the field write precedes the close,
	// and the caller's receive synchronizes-with it.
	result := &RunResult{}

	// teardown is the single cleanup path: Shutdown is idempotent, so every
	// exit calls it unconditionally rather than guessing whether the
	// supervisor already stopped.
	teardown := func() {
		appSup.Shutdown()
		<-supDone
		// DrainOutcomeClean is valid only after supDone: the drain budget is
		// spent during Shutdown's synchronous phases, which complete before
		// the tick loop signals supDone.
		result.ShutdownClean = appSup.DrainOutcomeClean()
		// ClearDeps strictly after supDone: clearing earlier flips the
		// application worker's RegistryConfigured observation mid-shutdown.
		register.ClearDeps(configworker.WorkerTypeName)

		if restoreStoreLogger != nil {
			restoreStoreLogger()
		}

		// The parse runs after the appSup.Shutdown() call above returned. In
		// this goroutine that call runs supervisor/lifecycle.go's Phase 4
		// itself, so the entries those phases write are in the capture by
		// then. Writes that trail this point are not captured: the
		// collector's final collector_loop_stopped debug trails the channel
		// close its Stop waits on; a driver goroutine still holding
		// Env.Logger can keep writing; and a caller-initiated
		// result.Shutdown() runs Phase 4 on the caller's goroutine while the
		// call above early-returns on an already-stopped supervisor.
		result.Logs = parseLogEntries(capture.lines())
	}

	// The Driver is user-authored code, so it may return an error or panic.
	// Either way the supervisor must stop and the deps key must be cleared
	// before runV2's frame unwinds, otherwise every later runV2 in this
	// process fails its already-published check. The flag stays false until
	// the teardown goroutine takes ownership of cleanup.
	teardownOwnedByGoroutine := false

	defer func() {
		if !teardownOwnedByGoroutine {
			teardown()
		}
	}()

	client := fsmv2client.NewFSMv2Client(writer, cfg.Store)
	if err := cfg.ScenarioV2.Driver(ctx, Env{Client: client, Logger: runLogger}); err != nil {
		return nil, fmt.Errorf("scenario %q driver failed: %w", cfg.ScenarioV2.Name, err)
	}

	teardownOwnedByGoroutine = true

	done := make(chan struct{})
	result.Done = done

	go func() {
		// The select only decides the wake-up reason; teardown then runs
		// unconditionally, because skipping Shutdown on any arm would skip
		// the supervisor's synchronous cleanup phases. The reason is logged
		// so a supervisor that stopped on its own (supDone) is visible in
		// the run output instead of looking like a clean Duration run.
		var wakeReason string

		if cfg.Duration > 0 {
			select {
			case <-time.After(cfg.Duration):
				wakeReason = "duration_elapsed"
			case <-ctx.Done():
				wakeReason = "ctx_cancelled"
			case <-supDone:
				wakeReason = "supervisor_stopped"
			}
		} else {
			select {
			case <-ctx.Done():
				wakeReason = "ctx_cancelled"
			case <-supDone:
				wakeReason = "supervisor_stopped"
			}
		}

		runLogger.Info("v2_run_teardown_starting",
			deps.String("scenario", cfg.ScenarioV2.Name),
			deps.String("wake_reason", wakeReason))

		teardown()
		close(done)
	}()

	// Shutdown waits for Done so the deps key is already cleared when the
	// caller starts the next v2 run; returning earlier would let this run's
	// late ClearDeps delete the next run's freshly published key.
	result.Shutdown = func() {
		appSup.Shutdown()
		<-done
	}

	return result, nil
}

// SetupStore creates an in-memory TriangularStore for testing and CLI usage.
func SetupStore(logger deps.FSMLogger) storage.TriangularStoreInterface {
	swapLogger := &swappableLogger{l: logger}

	return &swappableStore{
		TriangularStore: storage.NewTriangularStore(memory.NewInMemoryStore(), swapLogger),
		logger:          swapLogger,
	}
}

// teeLogger is an FSMLogger that forwards every entry to two loggers.
type teeLogger struct {
	a deps.FSMLogger
	b deps.FSMLogger
}

func (t teeLogger) Debug(msg string, fields ...deps.Field) {
	t.a.Debug(msg, fields...)
	t.b.Debug(msg, fields...)
}

func (t teeLogger) Info(msg string, fields ...deps.Field) {
	t.a.Info(msg, fields...)
	t.b.Info(msg, fields...)
}

func (t teeLogger) SentryWarn(feature deps.Feature, hierarchyPath string, msg string, fields ...deps.Field) {
	t.a.SentryWarn(feature, hierarchyPath, msg, fields...)
	t.b.SentryWarn(feature, hierarchyPath, msg, fields...)
}

func (t teeLogger) SentryError(feature deps.Feature, hierarchyPath string, err error, msg string, fields ...deps.Field) {
	t.a.SentryError(feature, hierarchyPath, err, msg, fields...)
	t.b.SentryError(feature, hierarchyPath, err, msg, fields...)
}

// With keeps both arms enriched, so a per-worker logger derived from the tee
// still writes to the caller's logger and the capture alike.
func (t teeLogger) With(fields ...deps.Field) deps.FSMLogger {
	return teeLogger{a: t.a.With(fields...), b: t.b.With(fields...)}
}

// syncBuffer is a goroutine-safe bytes.Buffer: the capture is written by the
// run's goroutines and read once after the run joined them, and the read must
// not race a straggling write.
type syncBuffer struct {
	buf bytes.Buffer
	mu  sync.Mutex
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

func (b *syncBuffer) lines() []string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return strings.Split(b.buf.String(), "\n")
}

// parseLogEntries turns captured JSON log lines into LogEntry values. It is
// pure: bytes in, structs out. A line that does not parse as JSON is
// dropped; the capture's only writer is the JSON logger built in runV2,
// which writes one complete line per entry, so a dropped line cannot occur
// by construction.
func parseLogEntries(lines []string) []LogEntry {
	entries := make([]LogEntry, 0, len(lines))

	for _, line := range lines {
		if line == "" {
			continue
		}

		var raw map[string]any
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}

		entry := LogEntry{Fields: make(map[string]any, len(raw))}

		if level, ok := raw["level"].(string); ok {
			entry.Level = level
		}

		if msg, ok := raw["msg"].(string); ok {
			entry.Msg = msg
		}

		if worker, ok := raw["worker"].(string); ok {
			entry.Worker = worker
		}

		for key, val := range raw {
			switch key {
			case "level", "msg", "worker", "ts":
				continue
			}

			entry.Fields[key] = val
		}

		entries = append(entries, entry)
	}

	return entries
}

// swappableLogger forwards to a logger that can be exchanged mid-flight, so a
// store built before a run can log through the run's tee logger once the run
// starts. The store calls only the four log methods; a logger derived via With
// is not swappable, and nothing in pkg/cse/storage derives one.
//
// The mutex is what makes the exchange safe to overlap with logging: the
// restore in teardown can run while a straggling store entry is still being
// logged. The caller-initiated Shutdown that produces that overlap is one of
// the trailing-write classes runV2's teardown comment names.
type swappableLogger struct {
	l  deps.FSMLogger
	mu sync.RWMutex
}

func (s *swappableLogger) swap(l deps.FSMLogger) deps.FSMLogger {
	s.mu.Lock()
	defer s.mu.Unlock()

	prev := s.l
	s.l = l

	return prev
}

func (s *swappableLogger) get() deps.FSMLogger {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.l
}

func (s *swappableLogger) Debug(msg string, fields ...deps.Field) {
	s.get().Debug(msg, fields...)
}

func (s *swappableLogger) Info(msg string, fields ...deps.Field) {
	s.get().Info(msg, fields...)
}

func (s *swappableLogger) SentryWarn(feature deps.Feature, hierarchyPath string, msg string, fields ...deps.Field) {
	s.get().SentryWarn(feature, hierarchyPath, msg, fields...)
}

func (s *swappableLogger) SentryError(feature deps.Feature, hierarchyPath string, err error, msg string, fields ...deps.Field) {
	s.get().SentryError(feature, hierarchyPath, err, msg, fields...)
}

func (s *swappableLogger) With(fields ...deps.Field) deps.FSMLogger {
	return s.get().With(fields...)
}

// swappableStore is a TriangularStore whose logger can be swapped for a run.
type swappableStore struct {
	*storage.TriangularStore
	logger *swappableLogger
}

// swapStoreLogger points a swappable store's logging at l for the duration of
// a run. It returns a restore function, or nil when the store cannot be
// swapped (a caller-built store that is not from SetupStore).
func swapStoreLogger(store storage.TriangularStoreInterface, l deps.FSMLogger) func() {
	swappable, ok := store.(*swappableStore)
	if !ok {
		return nil
	}

	prev := swappable.logger.swap(l)

	return func() {
		swappable.logger.swap(prev)
	}
}
