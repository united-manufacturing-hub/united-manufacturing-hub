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

	// baseSpec supplies only the required fields plus a no-op logger, leaving the
	// optional fields (ConfigEqual/CfgFor/IsEnabled/MinRequiredTime) nil so the
	// defaults apply unless a spec overrides them.
	baseSpec := func() WorkerManagerSpec[mgrConfig, probeStatus] {
		return WorkerManagerSpec[mgrConfig, probeStatus]{
			WorkerType:     workerType,
			ExtractConfigs: extractConfigs,
			NameOf:         nameOf,
			MapFresh:       mapFresh,
			MapObserved:    mapObserved,
			Log:            deps.NewNopFSMLogger(),
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
})

// Compile-time proof that WorkerManager satisfies the fsmv1 FSMManager
// interface (parameterised on the domain config type).
var _ publicfsm.FSMManager[mgrConfig] = (*WorkerManager[mgrConfig, probeStatus])(nil)
