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

package adapter

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	publicfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// -----------------------------------------------------------------------------
// ASSUMED API (drives GREEN — implement EXACTLY this):
//
//	type WorkerManagerSpec[TConfig, TStatus any] struct {
//	    WorkerType      string                                                     // required; builds the ref + names the manager
//	    ExtractConfigs  func(publicfsm.SystemSnapshot) []TConfig                   // required
//	    NameOf          func(cfg TConfig) string                                   // required
//	    MapFresh        func(cfg TConfig, status TStatus) string                   // required
//	    MapObserved     func(cfg TConfig, status TStatus) publicfsm.ObservedState  // required
//	    ConfigEqual     func(a, b TConfig) bool                                     // optional, default reflect.DeepEqual
//	    CfgFor          func(cfg TConfig) (map[string]any, error)                  // optional, default JSON round-trip
//	    IsEnabled       func(cfg TConfig) bool                                     // optional, default from desired state
//	    MinRequiredTime time.Duration                                             // optional, default 0
//	    Log             deps.FSMLogger                                            // logger (nil => no-op)
//	}
//	func NewWorkerManager[TConfig, TStatus any](spec WorkerManagerSpec[TConfig, TStatus]) *WorkerManager[TConfig, TStatus]
//
// Baked-in assumptions for GREEN to satisfy:
//   - Ref is DERIVED internally: dynamicchildren.Ref{WorkerType: spec.WorkerType,
//     Name: spec.NameOf(cfg)}. There is NO RefFor and NO NewInstance field; the
//     manager constructs each AdaptedInstance via the internal newAdaptedInstance.
//   - GetManagerName() returns spec.WorkerType (there is no ManagerName field).
//   - Every config must satisfy adapter.StateConfig (GetDesiredFSMState()). The
//     default IsEnabled (when nil) reads it: GetDesiredFSMState()=="stopped" =>
//     disabled, otherwise enabled. mgrConfig below implements it.
//   - The instance's desired state is derived from GetDesiredFSMState() (the
//     disabled shortcut in AdaptedInstance returns desiredState verbatim). A
//     config with State "running" yields DesiredState "running".
//   - RETRY FIX: on a CfgFor/Upsert failure in the config-CHANGE branch the
//     manager must NOT advance its stored domainConfig for that name, so the
//     next Reconcile retries (mirrors the new-worker branch's `continue`).
//   - reflect.DeepEqual is the default ConfigEqual; a JSON round-trip is the
//     default CfgFor.
//
// TStatus reuses probeStatus / probeObserved / stubReader from instance_test.go
// (same package). mgrConfig is local because it must satisfy adapter.StateConfig,
// which testConfig (Target-only) does not.
// -----------------------------------------------------------------------------

// mgrConfig is a domain TConfig satisfying adapter.StateConfig so the default
// IsEnabled (GetDesiredFSMState()=="stopped" => disabled) resolves. Value forces a
// ConfigEqual difference without touching name or state.
type mgrConfig struct {
	Name  string `json:"name"`
	State string `json:"state"`
	Value string `json:"value"`
}

// GetDesiredFSMState makes mgrConfig satisfy adapter.StateConfig, the constraint
// every managed config must meet; the default IsEnabled and desiredStateOf read it.
func (c mgrConfig) GetDesiredFSMState() string { return c.State }

// countingLogger wraps deps.FSMLogger and captures every SentryWarn so a test
// can assert the missing-runtime path warns exactly once per worker name for
// the manager's lifetime (never once per reconcile tick) and that each warn
// carries the worker name. The manager logs directly on spec.Log (never through
// With), so capturing the wrapper's SentryWarn sees every warn the missing-
// runtime path can emit.
type countingLogger struct {
	deps.FSMLogger
	warns    int
	features []deps.Feature // feature tag of each warn, in order
	names    []string       // hierarchyPath of each warn, in order
	workers  []string       // values of the "worker" field, in warn order
}

func (l *countingLogger) SentryWarn(feature deps.Feature, hierarchyPath string, _ string, fields ...deps.Field) {
	l.warns++
	l.features = append(l.features, feature)
	l.names = append(l.names, hierarchyPath)

	for _, f := range fields {
		if f.Key == "worker" {
			if s, ok := f.Value.(string); ok {
				l.workers = append(l.workers, s)
			}
		}
	}
}

var _ = Describe("WorkerManager", func() {
	const workerType = "adapter-mgr-probe" // unregistered => staleAfter 1s fallback

	ctx := context.Background()

	var desired []mgrConfig

	nameOf := func(c mgrConfig) string { return c.Name }
	mapFresh := func(_ mgrConfig, s probeStatus) string { return s.PortState }
	mapObserved := func(_ mgrConfig, s probeStatus) publicfsm.ObservedState {
		return probeObserved{State: s.PortState}
	}
	extractConfigs := func(_ publicfsm.SystemSnapshot) []mgrConfig { return desired }

	refFor := func(name string) dynamicchildren.Ref {
		return dynamicchildren.Ref{WorkerType: workerType, Name: name}
	}

	// baseSpec supplies the required fields, today's four fsmv1 literals declared
	// as the vocabulary, and a no-op logger. The optional fields
	// (ConfigEqual/CfgFor/IsEnabled/MinRequiredTime) are left nil to apply their
	// defaults. The vocabulary words are inlined, not the package constants, so
	// the specs see stable values independent of any constant the implementation
	// may later delete.
	baseSpec := func() WorkerManagerSpec[mgrConfig, probeStatus] {
		return WorkerManagerSpec[mgrConfig, probeStatus]{
			WorkerType:     workerType,
			ExtractConfigs: extractConfigs,
			NameOf:         nameOf,
			MapFresh:       mapFresh,
			MapObserved:    mapObserved,
			States: StateVocabulary{
				Starting:       "starting",
				Degraded:       "degraded",
				Stopped:        "stopped",
				DesiredRunning: "running",
			},
			Log: deps.NewNopFSMLogger(),
		}
	}

	// setupClient publishes a global client backed by a fresh writer (whose
	// registry the specs inspect via Contains) and the given reader.
	setupClient := func(sr *stubReader) *dynamicchildren.Writer {
		w := dynamicchildren.NewWriter()
		fsmv2client.SetClient(fsmv2client.NewFSMv2Client(w, sr))

		return w
	}

	freshReader := func(status probeStatus) *stubReader {
		return &stubReader{obs: &fsmv2.Observation[probeStatus]{
			CollectedAt: time.Now().Add(-50 * time.Millisecond),
			Status:      status,
		}}
	}

	BeforeEach(func() {
		desired = nil
	})

	AfterEach(func() {
		fsmv2client.SetClient(nil)
	})

	It("NewWorkerManager panics when WorkerType is empty", func() {
		spec := baseSpec()
		spec.WorkerType = ""

		Expect(func() { NewWorkerManager(spec) }).To(PanicWith(ContainSubstring("WorkerType is required")))
	})

	It("NewWorkerManager panics when a required function is nil", func() {
		spec := baseSpec()
		spec.MapObserved = nil

		Expect(func() { NewWorkerManager(spec) }).To(PanicWith(ContainSubstring("requires ExtractConfigs")))
	})

	It("NewWorkerManager panics when the StateVocabulary is incomplete, for each of the four fields", func() {
		fields := []struct {
			name  string
			blank func(*StateVocabulary)
		}{
			{"Starting", func(v *StateVocabulary) { v.Starting = "" }},
			{"Degraded", func(v *StateVocabulary) { v.Degraded = "" }},
			{"Stopped", func(v *StateVocabulary) { v.Stopped = "" }},
			{"DesiredRunning", func(v *StateVocabulary) { v.DesiredRunning = "" }},
		}

		for _, f := range fields {
			spec := baseSpec()
			f.blank(&spec.States)

			Expect(func() { NewWorkerManager(spec) }).To(PanicWith(ContainSubstring("StateVocabulary")), "field: %s", f.name)
		}
	})

	It("NewWorkerManager panics when the StateVocabulary words are not distinct", func() {
		// A vocabulary that reuses one word for two exits silently collapses the
		// adapter-decided states together; reject it at construction.
		spec := baseSpec()
		spec.States.Starting = "monitor"
		spec.States.Degraded = "monitor"

		Expect(func() { NewWorkerManager(spec) }).To(PanicWith(ContainSubstring("distinct")))
	})

	It("Reconcile adds a new worker: instance registered and ref Upserted", func() {
		w := setupClient(&stubReader{})
		mgr := NewWorkerManager(baseSpec())

		desired = []mgrConfig{{Name: "alpha", State: "running"}}

		err, changed := mgr.Reconcile(ctx, publicfsm.SystemSnapshot{}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeTrue())

		inst, ok := mgr.GetInstance("alpha")
		Expect(ok).To(BeTrue())
		Expect(inst).NotTo(BeNil())
		Expect(mgr.GetInstances()).To(HaveKey("alpha"))

		Expect(w.Registry().Contains(refFor("alpha"))).To(BeTrue())
	})

	It("does not orphan an enabled worker created while the client is nil", func() {
		// Client not yet published (fsmv2 runtime starting): an enabled worker
		// cannot be Upserted, so it must NOT be recorded as done — else it is
		// orphaned when the client appears and its config hasn't changed.
		fsmv2client.SetClient(nil)

		mgr := NewWorkerManager(baseSpec())
		desired = []mgrConfig{{Name: "alpha", State: "running"}}

		_, changed := mgr.Reconcile(ctx, publicfsm.SystemSnapshot{}, nil)
		Expect(changed).To(BeFalse(), "nothing registered yet")

		_, ok := mgr.GetInstance("alpha")
		Expect(ok).To(BeFalse(), "enabled worker not recorded until it can be Upserted")

		// Client appears; the same unchanged config must now converge.
		w := setupClient(&stubReader{})

		_, changed = mgr.Reconcile(ctx, publicfsm.SystemSnapshot{}, nil)
		Expect(changed).To(BeTrue())
		Expect(w.Registry().Contains(refFor("alpha"))).To(BeTrue(), "worker Upserted once the client is available")

		inst, ok := mgr.GetInstance("alpha")
		Expect(ok).To(BeTrue())
		Expect(inst).NotTo(BeNil())
	})

	It("a nil client warns exactly once per enabled worker name, never per tick, with control flow unchanged", func() {
		logger := &countingLogger{FSMLogger: deps.NewNopFSMLogger()}

		spec := baseSpec()
		spec.Log = logger
		mgr := NewWorkerManager(spec)

		// No FSMv2 runtime: the global client stays nil for the manager's lifetime.
		fsmv2client.SetClient(nil)

		// Five ticks with an enabled worker whose Upsert can never land. The empty
		// snapshot plus ctx.Err nil keep every tick on the same new-worker branch.
		for i := 0; i < 5; i++ {
			desired = []mgrConfig{{Name: "alpha", State: "running"}}
			_, _ = mgr.Reconcile(ctx, publicfsm.SystemSnapshot{}, nil)
		}

		// An enabled worker with a nil client must warn exactly once, never once per
		// tick, and the warn must name the worker so the event is attributable.
		Expect(logger.warns).To(Equal(1),
			"an enabled worker with a nil client must warn exactly once, not once per tick (got %d)", logger.warns)
		Expect(logger.names).To(Equal([]string{"alpha"}),
			"the warn must carry the worker name as its hierarchy path")
		Expect(logger.workers).To(Equal([]string{"alpha"}),
			"the warn must carry the worker name as a field")
		// The literal tag, not FeatureForWorker: the helper and the constant would
		// flip together, leaving the test green while the wrong feature shipped to
		// Sentry. The routing feature is the manager's worker type verbatim.
		Expect(logger.features).To(Equal([]deps.Feature{"adapter-mgr-probe"}),
			"the warn must route under the manager's worker-type feature tag, verbatim")

		// Control flow unchanged: the enabled worker is still not recorded, so the
		// next tick retries it instead of orphaning it.
		_, ok := mgr.GetInstance("alpha")
		Expect(ok).To(BeFalse(), "enabled worker with nil client is not recorded until it can be Upserted")

		// A disabled worker needs no Upsert: it is recorded, and must not warn.
		desired = []mgrConfig{{Name: "beta", State: "stopped"}}
		_, _ = mgr.Reconcile(ctx, publicfsm.SystemSnapshot{}, nil)

		_, ok = mgr.GetInstance("beta")
		Expect(ok).To(BeTrue(), "disabled worker with nil client IS recorded (no Upsert needed)")

		Expect(logger.warns).To(Equal(1),
			"a disabled worker must add zero warns (got %d)", logger.warns)

		// A second distinct enabled worker name coercively pins the per-name dedup:
		// a single process-global latch would silently suppress gamma's warn
		// forever, so the count must grow to two here, not stay at one.
		desired = []mgrConfig{{Name: "gamma", State: "running"}}
		_, _ = mgr.Reconcile(ctx, publicfsm.SystemSnapshot{}, nil)

		Expect(logger.warns).To(Equal(2),
			"a second distinct enabled name must warn once for its own name (got %d)", logger.warns)
		Expect(logger.names).To(Equal([]string{"alpha", "gamma"}))
		Expect(logger.workers).To(Equal([]string{"alpha", "gamma"}))

		// Gamma obeys the same once-per-name cap, not once per tick.
		desired = []mgrConfig{{Name: "gamma", State: "running"}}
		_, _ = mgr.Reconcile(ctx, publicfsm.SystemSnapshot{}, nil)

		Expect(logger.warns).To(Equal(2),
			"gamma must also warn exactly once, not once per tick (got %d)", logger.warns)

		// "For the manager's lifetime", not per nil-client episode: removing alpha
		// from config and re-adding it under the still-nil client must not warn
		// again. This pins the intended-silent contract and distinguishes the
		// permanent per-name latch from a per-name throttle that re-arms after a
		// few ticks (such a throttle would re-warn here, well past its window),
		// and the test would catch the regression the dedup exists to prevent.
		desired = nil
		_, _ = mgr.Reconcile(ctx, publicfsm.SystemSnapshot{}, nil)

		desired = []mgrConfig{{Name: "alpha", State: "running"}}
		_, _ = mgr.Reconcile(ctx, publicfsm.SystemSnapshot{}, nil)

		Expect(logger.warns).To(Equal(2),
			"re-adding a name under a still-nil client must not re-warn (got %d)", logger.warns)
	})

	It("a nil client warns once per name on the config-change branch too, and does not advance the stored config until the runtime returns", func() {
		// The nil-client warn contract has a second call site: applyDesired from
		// the config-change branch (manager.go), reached only after a worker is
		// already recorded. If the once-per-name dedup were ever scoped to the
		// new-worker branch alone, this branch would warn on every retry tick.
		logger := &countingLogger{FSMLogger: deps.NewNopFSMLogger()}
		spec := baseSpec()
		spec.Log = logger

		// Create the worker under a live client: v1 is Upserted and recorded.
		w := setupClient(&stubReader{})
		mgr := NewWorkerManager(spec)

		desired = []mgrConfig{{Name: "alpha", State: "running", Value: "v1"}}
		_, changed := mgr.Reconcile(ctx, publicfsm.SystemSnapshot{}, nil)
		Expect(changed).To(BeTrue())
		Expect(w.Registry().Contains(refFor("alpha"))).To(BeTrue())

		// The runtime goes away and the config changes: same name, different
		// config, so every tick takes the config-change branch into applyDesired
		// with a nil client.
		fsmv2client.SetClient(nil)
		desired = []mgrConfig{{Name: "alpha", State: "running", Value: "v2"}}

		for i := 0; i < 5; i++ {
			_, _ = mgr.Reconcile(ctx, publicfsm.SystemSnapshot{}, nil)
		}

		Expect(logger.warns).To(Equal(1),
			"the config-change branch must warn exactly once per name under a nil client, never per tick (got %d)", logger.warns)
		Expect(logger.names).To(Equal([]string{"alpha"}))

		// The stored config is not advanced while the client is nil: applyDesired
		// returned !enabled, so the manager keeps retrying v2 and the registry
		// (last Upserted spec) still reflects v1.
		recorded, ok := w.Registry().Lookup(refFor("alpha"))
		Expect(ok).To(BeTrue())
		Expect(recorded.UserSpec.Config).To(ContainSubstring("v1"))
		Expect(recorded.UserSpec.Config).NotTo(ContainSubstring("v2"))

		// The runtime returns: the retried v2 change now lands and converges,
		// and convergence adds no warn.
		w = setupClient(&stubReader{})
		_, changed = mgr.Reconcile(ctx, publicfsm.SystemSnapshot{}, nil)
		Expect(changed).To(BeTrue(), "the pending config change converges once the client returns")

		recorded, ok = w.Registry().Lookup(refFor("alpha"))
		Expect(ok).To(BeTrue())
		Expect(recorded.UserSpec.Config).To(ContainSubstring("v2"))
		Expect(recorded.UserSpec.Config).NotTo(ContainSubstring("v1"))

		Expect(logger.warns).To(Equal(1), "convergence adds no warn")
	})

	It("Reconcile removes a worker no longer in config: dropped from map and Delete called", func() {
		w := setupClient(&stubReader{})
		mgr := NewWorkerManager(baseSpec())

		desired = []mgrConfig{{Name: "alpha", State: "running"}}
		_, changed := mgr.Reconcile(ctx, publicfsm.SystemSnapshot{}, nil)
		Expect(changed).To(BeTrue())
		Expect(w.Registry().Contains(refFor("alpha"))).To(BeTrue())

		// Second tick: config no longer lists alpha.
		desired = nil
		_, changed = mgr.Reconcile(ctx, publicfsm.SystemSnapshot{}, nil)
		Expect(changed).To(BeTrue())

		_, ok := mgr.GetInstance("alpha")
		Expect(ok).To(BeFalse())
		Expect(w.Registry().Contains(refFor("alpha"))).To(BeFalse())
	})

	It("Reconcile updates a changed config: re-Upserted and instance replaced", func() {
		w := setupClient(&stubReader{})
		mgr := NewWorkerManager(baseSpec())

		desired = []mgrConfig{{Name: "alpha", State: "running", Value: "v1"}}
		_, _ = mgr.Reconcile(ctx, publicfsm.SystemSnapshot{}, nil)
		first, ok := mgr.GetInstance("alpha")
		Expect(ok).To(BeTrue())

		// Same name, different config (default ConfigEqual = DeepEqual => not equal).
		desired = []mgrConfig{{Name: "alpha", State: "running", Value: "v2"}}
		_, changed := mgr.Reconcile(ctx, publicfsm.SystemSnapshot{}, nil)
		Expect(changed).To(BeTrue())

		second, ok := mgr.GetInstance("alpha")
		Expect(ok).To(BeTrue())
		Expect(second).NotTo(BeIdenticalTo(first)) // rebuilt, not mutated in place
		Expect(w.Registry().Contains(refFor("alpha"))).To(BeTrue())
	})

	It("retry fix: CfgFor failure on the change branch does not advance stored config, so the next tick retries", func() {
		w := setupClient(&stubReader{})

		var cfgForErr error

		cfgForCalls := 0

		spec := baseSpec()
		spec.CfgFor = func(c mgrConfig) (map[string]any, error) {
			cfgForCalls++

			if cfgForErr != nil {
				return nil, cfgForErr
			}

			return map[string]any{"name": c.Name, "value": c.Value}, nil
		}
		mgr := NewWorkerManager(spec)

		// Tick 1: create v1 (success).
		desired = []mgrConfig{{Name: "alpha", State: "running", Value: "v1"}}
		_, _ = mgr.Reconcile(ctx, publicfsm.SystemSnapshot{}, nil)

		Expect(cfgForCalls).To(Equal(1))

		// Tick 2: change to v2, but CfgFor fails => must NOT record v2 as done.
		cfgForErr = errors.New("cfgfor boom")
		desired = []mgrConfig{{Name: "alpha", State: "running", Value: "v2"}}
		_, _ = mgr.Reconcile(ctx, publicfsm.SystemSnapshot{}, nil)

		Expect(cfgForCalls).To(Equal(2))

		// Tick 3: same v2, CfgFor now succeeds. If tick 2 wrongly advanced the
		// stored config, alpha would look equal and CfgFor would NOT be called
		// again (stays at 2). A third call proves the change was retried.
		cfgForErr = nil
		_, changed := mgr.Reconcile(ctx, publicfsm.SystemSnapshot{}, nil)
		Expect(changed).To(BeTrue())
		Expect(cfgForCalls).To(Equal(3))
		Expect(w.Registry().Contains(refFor("alpha"))).To(BeTrue())
	})

	It("defaults: nil ConfigEqual/CfgFor/IsEnabled still work (DeepEqual, JSON round-trip, desired-state)", func() {
		w := setupClient(&stubReader{})
		mgr := NewWorkerManager(baseSpec()) // all optionals nil

		desired = []mgrConfig{{Name: "alpha", State: "running", Value: "v1"}}
		_, changed := mgr.Reconcile(ctx, publicfsm.SystemSnapshot{}, nil)
		Expect(changed).To(BeTrue())
		Expect(w.Registry().Contains(refFor("alpha"))).To(BeTrue())

		// Identical config again => default ConfigEqual (DeepEqual) sees no change.
		_, changed = mgr.Reconcile(ctx, publicfsm.SystemSnapshot{}, nil)
		Expect(changed).To(BeFalse())

		// Changed config => default ConfigEqual detects the difference.
		desired = []mgrConfig{{Name: "alpha", State: "running", Value: "v2"}}
		_, changed = mgr.Reconcile(ctx, publicfsm.SystemSnapshot{}, nil)
		Expect(changed).To(BeTrue())
	})

	It("disabled config (default IsEnabled, state stopped): kept in map, not Upserted; existing registration Deleted", func() {
		w := setupClient(&stubReader{})
		mgr := NewWorkerManager(baseSpec())

		// Brand-new disabled worker: visible in the map, never Upserted.
		desired = []mgrConfig{{Name: "beta", State: "stopped"}}
		_, changed := mgr.Reconcile(ctx, publicfsm.SystemSnapshot{}, nil)
		Expect(changed).To(BeTrue())

		_, ok := mgr.GetInstance("beta")
		Expect(ok).To(BeTrue())
		Expect(w.Registry().Contains(refFor("beta"))).To(BeFalse())

		// A worker that starts enabled and later becomes disabled: registration Deleted.
		desired = []mgrConfig{{Name: "gamma", State: "running"}}
		_, _ = mgr.Reconcile(ctx, publicfsm.SystemSnapshot{}, nil)

		Expect(w.Registry().Contains(refFor("gamma"))).To(BeTrue())

		desired = []mgrConfig{{Name: "gamma", State: "stopped"}}
		_, _ = mgr.Reconcile(ctx, publicfsm.SystemSnapshot{}, nil)

		Expect(w.Registry().Contains(refFor("gamma"))).To(BeFalse())

		_, ok = mgr.GetInstance("gamma")
		Expect(ok).To(BeTrue()) // still visible in the map
	})

	It("a disabled instance reports its actual desired state, not a running fallback (nmap removal fix)", func() {
		// The StateConfig constraint guarantees desiredStateOf reads the real
		// desired state, so a stopped entry reports "stopped". Before the guard a
		// config the adapter could not introspect fell back to "running": a
		// stopped nmap was reported running and the connection FSM hung in
		// stopping, never completing protocol-converter removal.
		setupClient(&stubReader{})
		mgr := NewWorkerManager(baseSpec())

		desired = []mgrConfig{{Name: "beta", State: "stopped"}}
		_, _ = mgr.Reconcile(ctx, publicfsm.SystemSnapshot{}, nil)

		state, err := mgr.GetCurrentFSMState("beta")
		Expect(err).ToNot(HaveOccurred())
		Expect(state).To(Equal("stopped"),
			"a disabled instance must report its actual desired state, not the running fallback")
	})

	It("GetManagerName returns WorkerType; CreateSnapshot exposes per-worker current/desired state", func() {
		setupClient(freshReader(probeStatus{PortState: "open"}))

		mgr := NewWorkerManager(baseSpec())

		Expect(mgr.GetManagerName()).To(Equal(workerType))

		desired = []mgrConfig{{Name: "alpha", State: "running"}}
		_, _ = mgr.Reconcile(ctx, publicfsm.SystemSnapshot{}, nil)

		snap := mgr.CreateSnapshot()
		Expect(snap.GetName()).To(Equal(workerType))

		instances := snap.GetInstances()
		Expect(instances).To(HaveKey("alpha"))
		Expect(instances["alpha"].CurrentState).To(Equal("open")) // Fresh healthy => mapFresh
		Expect(instances["alpha"].DesiredState).To(Equal("running"))
	})

	It("GetCurrentFSMState and GetLastObservedState delegate to the instance; a missing name errors", func() {
		setupClient(freshReader(probeStatus{PortState: "open"}))

		mgr := NewWorkerManager(baseSpec())

		desired = []mgrConfig{{Name: "alpha", State: "running"}}
		_, _ = mgr.Reconcile(ctx, publicfsm.SystemSnapshot{}, nil)

		state, err := mgr.GetCurrentFSMState("alpha")
		Expect(err).NotTo(HaveOccurred())
		Expect(state).To(Equal("open"))

		obs, err := mgr.GetLastObservedState("alpha")
		Expect(err).NotTo(HaveOccurred())
		Expect(obs).To(Equal(probeObserved{State: "open"}))

		_, err = mgr.GetCurrentFSMState("missing")
		Expect(err).To(HaveOccurred())

		_, err = mgr.GetLastObservedState("missing")
		Expect(err).To(HaveOccurred())
	})

	It("prefixed Starting and Degraded vocabulary words reach all four adapter-decided exits as raw strings", func() {
		// Vocabulary whose words share no prefix with the current adapter's
		// literals ("starting"/"degraded"), so the bare constant returning values
		// cannot satisfy any assertion here.
		spec := baseSpec()
		spec.States.Starting = "benthos_monitoring_starting"
		spec.States.Degraded = "benthos_monitoring_degraded"
		mgr := NewWorkerManager(spec)

		reconcile := func(name string) {
			desired = []mgrConfig{{Name: name, State: "running"}}
			err, _ := mgr.Reconcile(ctx, publicfsm.SystemSnapshot{}, nil)
			Expect(err).NotTo(HaveOccurred())
		}

		stateOf := func(name string) string {
			inst, ok := mgr.GetInstance(name)
			Expect(ok).To(BeTrue())

			return inst.GetCurrentFSMState()
		}

		// (1) BOOTSTRAP (via NeverObserved): reader returns persistence.ErrNotFound
		// => NeverObserved => the worker's declared Starting word.
		setupClient(&stubReader{err: persistence.ErrNotFound})
		reconcile("r1-bootstrap")
		Expect(stateOf("r1-bootstrap")).To(Equal("benthos_monitoring_starting"))

		// (2) UNKNOWN-WITH-EMPTY-LAST-STATE: reconcile creates the instance, then
		// the reader errs on the FIRST GetCurrentFSMState read so lastState is
		// still empty => the worker's declared Starting word.
		sr := &stubReader{}
		setupClient(sr)
		reconcile("r1-unknown")
		sr.err = errors.New("generic read boom")
		Expect(stateOf("r1-unknown")).To(Equal("benthos_monitoring_starting"))

		// (3) DEGRADED VERDICT: Fresh observation whose stored status reports
		// degraded => the worker's declared Degraded word.
		setupClient(freshReader(probeStatus{PortState: "open", Degraded: true, Reason: "boom"}))
		reconcile("r1-degraded")
		Expect(stateOf("r1-degraded")).To(Equal("benthos_monitoring_degraded"))

		// (4) STALE: observation older than the 1s unregistered fallback staleAfter
		// => the worker's declared Degraded word.
		setupClient(&stubReader{obs: &fsmv2.Observation[probeStatus]{
			CollectedAt: time.Now().Add(-5 * time.Second),
			Status:      probeStatus{PortState: "open"},
		}})
		reconcile("r1-stale")
		Expect(stateOf("r1-stale")).To(Equal("benthos_monitoring_degraded"))

		// (5) FRESH+HEALTHY is outside the vocabulary: it returns the developer's
		// MapFresh output untouched, not the declared Starting/Degraded words.
		setupClient(freshReader(probeStatus{PortState: "open"}))
		reconcile("r1-fresh")
		Expect(stateOf("r1-fresh")).To(Equal("open"))
	})

	It("a config whose desired state is the declared Stopped word is disabled, not Upserted", func() {
		w := setupClient(&stubReader{})
		spec := baseSpec()
		spec.States.Stopped = "benthos_monitoring_stopped"
		mgr := NewWorkerManager(spec)

		desired = []mgrConfig{{Name: "alpha", State: "benthos_monitoring_stopped"}}

		err, changed := mgr.Reconcile(ctx, publicfsm.SystemSnapshot{}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeTrue())

		// A disabled worker is not Upserted into the fsmv2 runtime...
		Expect(w.Registry().Contains(refFor("alpha"))).To(BeFalse())

		// ...but it stays visible, reading as the desired (stopped) state.
		inst, ok := mgr.GetInstance("alpha")
		Expect(ok).To(BeTrue())
		Expect(inst.GetCurrentFSMState()).To(Equal("benthos_monitoring_stopped"))
	})

	It("a config with an empty desired state reports the declared DesiredRunning word", func() {
		setupClient(&stubReader{})
		spec := baseSpec()
		spec.States.DesiredRunning = "benthos_monitoring_active"
		mgr := NewWorkerManager(spec)

		desired = []mgrConfig{{Name: "alpha", State: ""}}

		err, _ := mgr.Reconcile(ctx, publicfsm.SystemSnapshot{}, nil)
		Expect(err).NotTo(HaveOccurred())

		inst, ok := mgr.GetInstance("alpha")
		Expect(ok).To(BeTrue())
		Expect(inst.GetDesiredFSMState()).To(Equal("benthos_monitoring_active"))
	})
})

// Compile-time proof that WorkerManager satisfies the fsmv1 FSMManager
// interface (parameterised on the domain config type).
var _ publicfsm.FSMManager[mgrConfig] = (*WorkerManager[mgrConfig, probeStatus])(nil)
