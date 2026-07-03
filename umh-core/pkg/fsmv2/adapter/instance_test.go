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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// -----------------------------------------------------------------------------
// ASSUMED CONSTRUCTOR SIGNATURE (drives GREEN — implement EXACTLY this):
//
//	func newAdaptedInstance[TConfig, TStatus any](
//	    ref             dynamicchildren.Ref,
//	    cfg             TConfig,
//	    desiredState    string,
//	    minRequiredTime time.Duration,
//	    mapFresh        func(cfg TConfig, status TStatus) string,
//	    mapObserved     func(cfg TConfig, status TStatus) publicfsm.ObservedState,
//	    isDisabled      bool,
//	) *AdaptedInstance[TConfig, TStatus]
//
// Type param ORDER is [TConfig, TStatus] (matches the brief's
// WorkerManagerSpec[TConfig, TStatus]), NOT the old skeleton's [TStatus, TConfig].
//
// staleAfter is NOT a constructor parameter. Per the brief it is framework-owned:
//   staleAfter = 3 × fsmv2.ObservationIntervalFor(ref.WorkerType); fallback 1s
//   when the worker type is unregistered.
// These specs use an UNREGISTERED worker type ("adapter-probe") so staleAfter is
// the 1s fallback, which the Fresh/Stale CollectedAt offsets below straddle.
//
// mapFresh / mapObserved take (cfg, status) ONLY — the verdict is NOT a mapper
// input. The framework reads the degraded verdict off the stored status via the
// adapter-defined HealthReporter interface, and the whole freshness ladder is
// framework-owned. mapFresh only handles the Fresh+healthy leaf.
// -----------------------------------------------------------------------------

// testConfig stands in for a domain TConfig flowing through the fsmv1 loop.
type testConfig struct {
	Target string
}

// probeStatus is the STORED status type (TStatus). It carries the health verdict
// and satisfies the adapter's HealthReporter interface structurally (value
// receiver), standing in for simple.Status[T] WITHOUT importing simple — the
// adapter is decoupled from simple.
type probeStatus struct {
	PortState string `json:"port_state"`
	Reason    string `json:"reason"`
	Degraded  bool   `json:"degraded"`
}

// HealthVerdict makes probeStatus a HealthReporter.
func (s probeStatus) HealthVerdict() (bool, string) { return s.Degraded, s.Reason }

// probeObserved is a concrete publicfsm.ObservedState returned by mapObserved.
type probeObserved struct {
	State string
}

func (probeObserved) IsObservedState() {}

// stubReader is a deps.StateReader the specs drive to return either a populated
// Observation[probeStatus], persistence.ErrNotFound (NeverObserved), or a
// generic error (Unknown). Mirrors fsmv2client's stubStateReader harness.
type stubReader struct {
	obs *fsmv2.Observation[probeStatus]
	err error
}

func (s *stubReader) LoadObservedTyped(_ context.Context, _, _ string, result any) error {
	if s.err != nil {
		return s.err
	}

	if s.obs == nil {
		return nil
	}

	out, ok := result.(*fsmv2.Observation[probeStatus])
	if !ok {
		return errors.New("stubReader: result is not *fsmv2.Observation[probeStatus]")
	}

	*out = *s.obs

	return nil
}

var _ = Describe("AdaptedInstance", func() {
	// Unregistered worker type → staleAfter falls back to 1s.
	ref := dynamicchildren.Ref{WorkerType: "adapter-probe", Name: "probe-1"}

	cfg := testConfig{Target: "192.0.2.1"}

	mapFresh := func(_ testConfig, s probeStatus) string { return s.PortState }
	mapObserved := func(_ testConfig, s probeStatus) publicfsm.ObservedState {
		return probeObserved{State: s.PortState}
	}

	// newInstance builds the instance under test with the assumed constructor.
	newInstance := func(desiredState string, isDisabled bool) *AdaptedInstance[testConfig, probeStatus] {
		return newAdaptedInstance(
			ref, cfg, desiredState, 0, mapFresh, mapObserved, isDisabled,
		)
	}

	// stageClient publishes a global client whose store returns the given
	// observation/error for ref. upsert controls Unregistered vs NeverObserved.
	stageClient := func(upsert bool, obs *fsmv2.Observation[probeStatus], err error) *stubReader {
		writer := dynamicchildren.NewWriter()
		if upsert {
			Expect(writer.Upsert(ref, map[string]any{})).To(Succeed())
		}

		sr := &stubReader{obs: obs, err: err}
		fsmv2client.SetClient(fsmv2client.NewFSMv2Client(writer, sr))

		return sr
	}

	freshObs := func(status probeStatus) *fsmv2.Observation[probeStatus] {
		return &fsmv2.Observation[probeStatus]{
			CollectedAt: time.Now().Add(-100 * time.Millisecond),
			Status:      status,
		}
	}

	AfterEach(func() {
		// Reset the process-scoped singleton so specs don't leak into each other.
		fsmv2client.SetClient(nil)
	})

	// --- GetCurrentFSMState ladder (brief precedence, one It per rung) ---

	It("rung 1: isDisabled returns desiredState verbatim without reading the client", func() {
		// Even a degraded observation staged in the client must be ignored.
		stageClient(true, freshObs(probeStatus{PortState: "open", Degraded: true, Reason: "boom"}), nil)

		inst := newInstance("stopped", true)

		Expect(inst.GetCurrentFSMState()).To(Equal("stopped"))
	})

	It("rung 2: GetClient()==nil with no prior state returns running", func() {
		fsmv2client.SetClient(nil)

		inst := newInstance("running", false)

		Expect(inst.GetCurrentFSMState()).To(Equal("running"))
	})

	It("rung 3: a degraded verdict on a Fresh observation returns degraded", func() {
		stageClient(true, freshObs(probeStatus{PortState: "open", Degraded: true, Reason: "boom"}), nil)

		inst := newInstance("running", false)

		Expect(inst.GetCurrentFSMState()).To(Equal("degraded"))
	})

	It("rung 4: Unregistered (ref never upserted) returns running (bootstrap, not starting)", func() {
		stageClient(false, nil, nil)

		inst := newInstance("running", false)

		Expect(inst.GetCurrentFSMState()).To(Equal("running"))
	})

	It("rung 4: NeverObserved (upserted but store ErrNotFound) returns running", func() {
		stageClient(true, nil, persistence.ErrNotFound)

		inst := newInstance("running", false)

		Expect(inst.GetCurrentFSMState()).To(Equal("running"))
	})

	It("rung 5: a Stale observation (older than staleAfter) returns degraded", func() {
		stale := &fsmv2.Observation[probeStatus]{
			CollectedAt: time.Now().Add(-5 * time.Second), // > 1s fallback staleAfter
			Status:      probeStatus{PortState: "open"},
		}
		stageClient(true, stale, nil)

		inst := newInstance("running", false)

		Expect(inst.GetCurrentFSMState()).To(Equal("degraded"))
	})

	It("rung 6: a Fresh healthy observation returns the mapFresh output", func() {
		stageClient(true, freshObs(probeStatus{PortState: "open"}), nil)

		inst := newInstance("running", false)

		Expect(inst.GetCurrentFSMState()).To(Equal("open"))
	})

	It("Unknown-hold-last: a read hiccup returns the last mapped state, not a flap", func() {
		sr := stageClient(true, freshObs(probeStatus{PortState: "open"}), nil)

		inst := newInstance("running", false)

		// First tick: Fresh healthy → "open" (cached as lastState).
		Expect(inst.GetCurrentFSMState()).To(Equal("open"))

		// Second tick: store read errors → Unknown → hold last known "open".
		sr.err = errors.New("transient store read boom")

		Expect(inst.GetCurrentFSMState()).To(Equal("open"))
	})

	// --- GetLastObservedState ---

	It("GetLastObservedState maps a Fresh observation to the developer's type", func() {
		stageClient(true, freshObs(probeStatus{PortState: "open"}), nil)

		inst := newInstance("running", false)

		Expect(inst.GetLastObservedState()).To(Equal(probeObserved{State: "open"}))
	})

	It("GetLastObservedState returns the same developer type (empty) when non-Fresh", func() {
		stageClient(false, nil, nil) // Unregistered → zero observation

		inst := newInstance("running", false)

		// Same concrete type as the Fresh path, not a foreign framework struct, so
		// a consumer's type assertion holds on every tick.
		Expect(inst.GetLastObservedState()).To(Equal(probeObserved{State: ""}))
	})

	// --- fsmv1 semantics ---

	It("SetDesiredFSMState returns a non-nil error (desired is config-driven)", func() {
		inst := newInstance("running", false)

		Expect(inst.SetDesiredFSMState("stopped")).To(HaveOccurred())
	})

	It("GetDesiredFSMState returns the configured desired state", func() {
		inst := newInstance("running", false)

		Expect(inst.GetDesiredFSMState()).To(Equal("running"))
	})

	It("Remove deletes the ref from the shared registry", func() {
		writer := dynamicchildren.NewWriter()
		Expect(writer.Upsert(ref, map[string]any{})).To(Succeed())
		fsmv2client.SetClient(fsmv2client.NewFSMv2Client(writer, &stubReader{}))

		inst := newInstance("running", false)

		Expect(inst.Remove(context.Background())).To(Succeed())
		Expect(writer.Registry().Contains(ref)).To(BeFalse())
	})

	// --- no-op lifecycle (monitor-only) ---

	It("lifecycle methods are no-ops and Reconcile/CheckForCreation match the monitor contract", func() {
		inst := newInstance("running", false)

		Expect(inst.CreateInstance(context.Background(), nil)).To(Succeed())
		Expect(inst.StartInstance(context.Background(), nil)).To(Succeed())
		Expect(inst.StopInstance(context.Background(), nil)).To(Succeed())
		Expect(inst.RemoveInstance(context.Background(), nil)).To(Succeed())
		Expect(inst.UpdateObservedStateOfInstance(context.Background(), nil, publicfsm.SystemSnapshot{})).To(Succeed())

		Expect(inst.CheckForCreation(context.Background(), nil)).To(BeTrue())
		Expect(inst.IsTransientStreakCounterMaxed()).To(BeFalse())
		Expect(inst.GetMinimumRequiredTime()).To(Equal(time.Duration(0)))

		err, changed := inst.Reconcile(context.Background(), publicfsm.SystemSnapshot{}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeFalse())
	})
})

// Compile-time proof that AdaptedInstance satisfies the fsmv1 FSMInstance
// interface (which embeds FSMInstanceActions).
var _ publicfsm.FSMInstance = (*AdaptedInstance[testConfig, probeStatus])(nil)
