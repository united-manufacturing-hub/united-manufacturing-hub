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

package fsmv2benthos

import (
	"context"
	"errors"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2"
	fsmv2config "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/benthosmetrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	publicfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthosmonitorfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos_monitor"

	"gopkg.in/yaml.v3"
)

// stubManagerReader is a deps.StateReader that returns a fixed
// Observation[simple.Status[benthosmetrics.Sample]] for every ref.
type stubManagerReader struct {
	obs *fsmv2.Observation[simple.Status[benthosmetrics.Sample]]
	err error
}

func (s *stubManagerReader) LoadObservedTyped(_ context.Context, _, _ string, result any) error {
	if s.err != nil {
		return s.err
	}

	if s.obs == nil {
		return nil
	}

	out, ok := result.(*fsmv2.Observation[simple.Status[benthosmetrics.Sample]])
	if !ok {
		return nil
	}

	*out = *s.obs

	return nil
}

func (s *stubManagerReader) LoadDesiredTyped(_ context.Context, _, _ string, _ any) error {
	return nil
}

var _ = Describe("The manager", func() {
	const name = "benthos-1"

	// benthosConfig builds an enabled config.BenthosMonitorConfig named `name`.
	benthosConfig := func(desired string) config.BenthosMonitorConfig {
		return config.BenthosMonitorConfig{
			FSMInstanceConfig: config.FSMInstanceConfig{
				Name:            name,
				DesiredFSMState: desired,
			},
			MetricsPort: 4195,
		}
	}

	// snapshotWith puts the given configs at the real snapshot path the
	// manager's ExtractConfigs reads.
	snapshotWith := func(cfgs ...config.BenthosMonitorConfig) publicfsm.SystemSnapshot {
		return publicfsm.SystemSnapshot{
			CurrentConfig: config.FullConfig{
				Internal: config.InternalConfig{BenthosMonitor: cfgs},
			},
		}
	}

	// freshObs stamps CollectedAt inside fsmv2client.GetFresh's maxAge window,
	// so the observation reads Fresh, and carries the given verdict: the
	// degraded-verdict case and the mapFresh case come from the same stamp.
	freshObs := func(degraded bool, reason string) *fsmv2.Observation[simple.Status[benthosmetrics.Sample]] {
		return &fsmv2.Observation[simple.Status[benthosmetrics.Sample]]{
			CollectedAt: time.Now().Add(-100 * time.Millisecond),
			Status: simple.Status[benthosmetrics.Sample]{
				Result:   benthosmetrics.Sample{},
				Degraded: degraded,
				Reason:   reason,
			},
		}
	}

	AfterEach(func() {
		fsmv2client.SetClient(nil)
	})

	// The worker registers under WorkerType and the adapter resolves every
	// exit to a state word benthos_monitor's own FSM recognises.
	It("registers the worker and reports benthos_monitor state names", func() {
		// The worker registers itself on import: the worker type exists with a
		// positive observation interval, and WorkerType is the canonical
		// string.
		Expect(fsmv2.LookupInitialState(WorkerType)).NotTo(BeNil())
		interval, ok := fsmv2.ObservationIntervalFor(WorkerType)
		Expect(ok).To(BeTrue())
		Expect(interval).To(BeNumerically(">", 0))
		Expect(WorkerType).To(Equal("benthos_metrics"))

		// One registered client, held across the reads; only the reader's
		// observation changes. The ref stays registered, so the first read is
		// NeverObserved, not Unregistered.
		reader := &stubManagerReader{err: persistence.ErrNotFound}
		writer := dynamicchildren.NewWriter()
		fsmv2client.SetClient(fsmv2client.NewFSMv2Client(writer, reader))

		mgr := NewFsmv2BenthosMetricsManager("test")
		err, _ := mgr.Reconcile(context.Background(), snapshotWith(benthosConfig("active")), nil)
		Expect(err).NotTo(HaveOccurred())

		// BOOTSTRAP: no observation yet => the declared Starting state
		// ("benthos_monitoring_starting"). The adapter supplies no fallback
		// state of its own.
		state, gerr := mgr.GetCurrentFSMState(name)
		Expect(gerr).NotTo(HaveOccurred())
		Expect(state).To(Equal(benthosmonitorfsm.OperationalStateStarting))

		// DEGRADED verdict on a Fresh observation => the declared Degraded
		// word ("degraded"). The four declared states are pairwise distinct,
		// so this word cannot be reused for another exit.
		reader.err = nil
		reader.obs = freshObs(true, "poll error: x")

		state, gerr = mgr.GetCurrentFSMState(name)
		Expect(gerr).NotTo(HaveOccurred())
		Expect(state).To(Equal(benthosmonitorfsm.OperationalStateDegraded))

		// FRESH + healthy verdict => mapFresh's result ("active"), the state
		// this worker also declares as DesiredRunning.
		reader.obs = freshObs(false, "")

		state, gerr = mgr.GetCurrentFSMState(name)
		Expect(gerr).NotTo(HaveOccurred())
		Expect(state).To(Equal(benthosmonitorfsm.OperationalStateActive))

		// EMPTY desired state on a config => the declared DesiredRunning
		// state ("active"), because benthos_monitor's SetDesiredFSMState
		// accepts only active or stopped.
		mgr2 := NewFsmv2BenthosMetricsManager("test")
		err, _ = mgr2.Reconcile(context.Background(), snapshotWith(benthosConfig("")), nil)
		Expect(err).NotTo(HaveOccurred())

		inst, iok := mgr2.GetInstance(name)
		Expect(iok).To(BeTrue())
		Expect(inst.GetDesiredFSMState()).To(Equal(benthosmonitorfsm.OperationalStateActive))
	})

	// cfgFor's YAML rendering survives the adapter's Upsert and the worker's
	// unmarshal without losing Name, DesiredFSMState or MetricsPort.
	It("round-trips name, desired state and metrics port through ChildSpec YAML", func() {
		reader := &stubManagerReader{err: persistence.ErrNotFound}
		writer := dynamicchildren.NewWriter()
		fsmv2client.SetClient(fsmv2client.NewFSMv2Client(writer, reader))

		mgr := NewFsmv2BenthosMetricsManager("test")
		err, _ := mgr.Reconcile(context.Background(), snapshotWith(benthosConfig("active")), nil)
		Expect(err).NotTo(HaveOccurred())

		spec, ok := writer.Registry().Lookup(dynamicchildren.Ref{
			WorkerType: WorkerType,
			Name:       name,
		})
		Expect(ok).To(BeTrue(), "the enabled worker ref should be Upserted")

		var back config.BenthosMonitorConfig
		Expect(yaml.Unmarshal([]byte(spec.UserSpec.Config), &back)).To(Succeed())
		Expect(back.Name).To(Equal(name))
		Expect(back.DesiredFSMState).To(Equal("active"))
		Expect(back.MetricsPort).To(Equal(uint16(4195)))
	})

	// The framework degrades a worker after three missed scrapes: with one
	// scrape a second, an observation 2 s old is fresh and one 3.5 s old is
	// stale. The registered Interval is the only lever this worker has on that.
	It("degrades after three missed scrapes, not later", func() {
		reader := &stubManagerReader{}
		fsmv2client.SetClient(fsmv2client.NewFSMv2Client(dynamicchildren.NewWriter(), reader))

		mgr := NewFsmv2BenthosMetricsManager("test")
		err, _ := mgr.Reconcile(context.Background(), snapshotWith(benthosConfig("active")), nil)
		Expect(err).NotTo(HaveOccurred())

		stateAtAge := func(age time.Duration) string {
			reader.obs = &fsmv2.Observation[simple.Status[benthosmetrics.Sample]]{CollectedAt: time.Now().Add(-age)}
			state, gerr := mgr.GetCurrentFSMState(name)
			Expect(gerr).NotTo(HaveOccurred())

			return state
		}

		// 1 s of slack before the 3 s limit, so a slow runner cannot age it.
		Expect(stateAtAge(2*time.Second)).To(Equal(benthosmonitorfsm.OperationalStateActive),
			"two missed scrapes must not degrade")
		Expect(stateAtAge(3500*time.Millisecond)).To(Equal(benthosmonitorfsm.OperationalStateDegraded),
			"three missed scrapes must degrade")
	})
})

var _ = Describe("GetLastObservedState across a failing store read", func() {
	const name = "benthos-1"

	// liveObs is a Fresh observation of a live, ready Benthos whose scrape
	// began at at.
	liveObs := func(at time.Time) *fsmv2.Observation[simple.Status[benthosmetrics.Sample]] {
		return &fsmv2.Observation[simple.Status[benthosmetrics.Sample]]{
			CollectedAt: time.Now().Add(-100 * time.Millisecond),
			Status: simple.Status[benthosmetrics.Sample]{
				Result: benthosmetrics.Sample{At: at, PingAlive: true, Ready: true},
			},
		}
	}

	snapshot := publicfsm.SystemSnapshot{
		CurrentConfig: config.FullConfig{
			Internal: config.InternalConfig{BenthosMonitor: []config.BenthosMonitorConfig{{
				FSMInstanceConfig: config.FSMInstanceConfig{Name: name, DesiredFSMState: "active"},
				MetricsPort:       4195,
			}}},
		},
	}

	// One failed store read must not read as a dead Benthos, and the 3 s
	// rule must still age the held status out.
	It("holds the Benthos live through one failed read, and not past three seconds of them", func() {
		t0 := time.Now()
		reader := &stubManagerReader{}
		fsmv2client.SetClient(fsmv2client.NewFSMv2Client(dynamicchildren.NewWriter(), reader))
		DeferCleanup(func() { fsmv2client.SetClient(nil) })

		mgr := NewFsmv2BenthosMetricsManager("test")
		err, _ := mgr.Reconcile(context.Background(), snapshot, nil)
		Expect(err).NotTo(HaveOccurred())

		readIsLive := func() bool {
			observed, gerr := mgr.GetLastObservedState(name)
			Expect(gerr).NotTo(HaveOccurred())
			bm, ok := observed.(benthosmonitorfsm.BenthosMonitorObservedState)
			Expect(ok).To(BeTrue(), "GetLastObservedState returned %T", observed)

			return bm.ServiceInfo.BenthosStatus.LastScan.HealthCheck.IsLive
		}

		By("a good read is live and fills the hold")
		reader.obs = liveObs(t0)
		Expect(readIsLive()).To(BeTrue(), "a live Benthos on a good read")

		By("the next read fails: the Benthos stays live")
		reader.err = errors.New("store read timed out")
		Expect(readIsLive()).To(BeTrue(),
			"one failed store read must not read as a dead Benthos")

		By("the next good read is live again")
		reader.err = nil
		reader.obs = liveObs(t0.Add(time.Second))
		Expect(readIsLive()).To(BeTrue())

		By("failed reads keep vouching only until the last good sample ages out")
		reader.err = errors.New("store read timed out")
		restore := swapScrapeClock(func() time.Time { return t0.Add(3 * time.Second) })
		Expect(readIsLive()).To(BeTrue(),
			"two seconds of failed reads still hold: the last good sample is 2 s old")
		restore()

		restore = swapScrapeClock(func() time.Time {
			return t0.Add(time.Second + StaleAfter + time.Millisecond)
		})
		defer restore()
		Expect(readIsLive()).To(BeFalse(),
			"past three seconds of failed reads the Benthos must read not live")
	})
})

var _ = Describe("The Failures lookup", func() {
	// Failures returns the failures of the named instance's stored sample,
	// false for an unknown instance, and false when the instance has no
	// sample yet.
	It("looks up one instance's stored failures", func() {
		// No client: no answer.
		f, ok := Failures(context.Background(), "benthos-1")
		Expect(ok).To(BeFalse(), "no client")
		Expect(f).To(BeNil())

		// A registered client whose store holds nothing for the instance: an
		// unknown instance, or a worker that stored no sample yet.
		reader := &stubManagerReader{err: persistence.ErrNotFound}
		fsmv2client.SetClient(fsmv2client.NewFSMv2Client(dynamicchildren.NewWriter(), reader))
		DeferCleanup(func() { fsmv2client.SetClient(nil) })

		f, ok = Failures(context.Background(), "benthos-1")
		Expect(ok).To(BeFalse(), "nothing stored for the instance")
		Expect(f).To(BeNil())

		// A stored sample carries its Failures.
		stored := &benthosmetrics.Failures{
			Entries: []benthosmetrics.FailureEntry{{
				Signal: "output_error", Label: "output error", Count: 1,
			}},
		}
		reader.err = nil
		reader.obs = &fsmv2.Observation[simple.Status[benthosmetrics.Sample]]{
			CollectedAt: time.Now().Add(-100 * time.Millisecond),
			Status: simple.Status[benthosmetrics.Sample]{
				Result: benthosmetrics.Sample{Failures: stored},
			},
		}

		f, ok = Failures(context.Background(), "benthos-1")
		Expect(ok).To(BeTrue(), "a stored sample answers")
		Expect(f).To(Equal(stored))

		// A minute-old sample still answers: the not-cleared count relies on
		// the lookup answering at any age.
		reader.obs.CollectedAt = time.Now().Add(-time.Minute)
		f, ok = Failures(context.Background(), "benthos-1")
		Expect(ok).To(BeTrue(), "a sample a minute old still answers")
		Expect(f).To(Equal(stored))

		// An observation whose sample has no Failures — a worker whose first
		// scrape failed — answers nothing.
		reader.obs.Status.Result = benthosmetrics.Sample{}
		f, ok = Failures(context.Background(), "benthos-1")
		Expect(ok).To(BeFalse(), "a sample without Failures answers nothing")
		Expect(f).To(BeNil())
	})
})

// perRefReader is a deps.StateReader that returns one observation per child
// id, so each registered worker can carry its own stored sample.
type perRefReader struct {
	obsByID map[string]*fsmv2.Observation[simple.Status[benthosmetrics.Sample]]
}

func (r *perRefReader) LoadObservedTyped(_ context.Context, _, id string, result any) error {
	obs, ok := r.obsByID[id]
	if !ok {
		return persistence.ErrNotFound
	}

	out, ok := result.(*fsmv2.Observation[simple.Status[benthosmetrics.Sample]])
	if !ok {
		return nil
	}

	*out = *obs

	return nil
}

func (r *perRefReader) LoadDesiredTyped(_ context.Context, _, _ string, _ any) error {
	return nil
}

// togglingReader answers every read with its one observation, or with a
// read error once failed is set. Both can change while other goroutines
// read, so a spec can exercise the Failures hold's lock.
type togglingReader struct {
	mu   sync.Mutex
	obs  *fsmv2.Observation[simple.Status[benthosmetrics.Sample]]
	fail bool
}

func (r *togglingReader) setFail(fail bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.fail = fail
}

func (r *togglingReader) LoadObservedTyped(_ context.Context, _, _ string, result any) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.fail {
		return errors.New("store read timed out")
	}

	out, ok := result.(*fsmv2.Observation[simple.Status[benthosmetrics.Sample]])
	if !ok {
		return nil
	}

	if r.obs != nil {
		*out = *r.obs
	}

	return nil
}

func (r *togglingReader) LoadDesiredTyped(_ context.Context, _, _ string, _ any) error {
	return nil
}

var _ = Describe("The Failures hold across a failing store read", func() {
	const name = "benthos-held"

	// errorFreeObs is an observation whose sample carries empty Failures:
	// a healthy Benthos, stored as its worker last scraped it.
	errorFreeObs := func() *fsmv2.Observation[simple.Status[benthosmetrics.Sample]] {
		return &fsmv2.Observation[simple.Status[benthosmetrics.Sample]]{
			CollectedAt: time.Now().Add(-100 * time.Millisecond),
			Status: simple.Status[benthosmetrics.Sample]{
				Result: benthosmetrics.Sample{Failures: &benthosmetrics.Failures{}},
			},
		}
	}

	// While the store read keeps failing and recovering, many services read
	// their instance's Failures on their own ticks. The lookups must stay
	// safe under that concurrency, and a failed read of a worker that is
	// still registered must answer from the last good Failures.
	It("answers from the last good Failures under concurrent failing reads", func() {
		reader := &togglingReader{obs: errorFreeObs()}
		c := fsmv2client.NewFSMv2Client(dynamicchildren.NewWriter(), reader)
		Expect(c.Upsert(dynamicchildren.Ref{WorkerType: WorkerType, Name: name}, map[string]any{})).To(Succeed())
		fsmv2client.SetClient(c)
		DeferCleanup(func() { fsmv2client.SetClient(nil) })

		// One good read fills the hold.
		f, ok := Failures(context.Background(), name)
		Expect(ok).To(BeTrue(), "the good read answers")
		Expect(f).NotTo(BeNil())

		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 50; j++ {
					_, _ = Failures(context.Background(), name)
				}
			}()
		}

		// While the readers read, the store keeps failing and recovering.
		for j := 0; j < 50; j++ {
			reader.setFail(j%2 == 0)
		}
		wg.Wait()

		// A failed read of a registered worker answers from the hold.
		reader.setFail(true)
		f, ok = Failures(context.Background(), name)
		Expect(ok).To(BeTrue(), "a failed read answers from the last good Failures")
		Expect(f).NotTo(BeNil())
	})

	// A reader that changes the Failures it was handed — the good read's or
	// the hold's — cannot change what the hold later serves: the hold keeps
	// its own copy, entries included.
	It("serves the next read the held answer unchanged after a reader changes the returned Failures", func() {
		reader := &togglingReader{obs: &fsmv2.Observation[simple.Status[benthosmetrics.Sample]]{
			CollectedAt: time.Now().Add(-100 * time.Millisecond),
			Status: simple.Status[benthosmetrics.Sample]{
				Result: benthosmetrics.Sample{Failures: &benthosmetrics.Failures{
					Entries: []benthosmetrics.FailureEntry{
						{Signal: "output_error", Label: "output error", Count: 3},
					},
				}},
			},
		}}
		c := fsmv2client.NewFSMv2Client(dynamicchildren.NewWriter(), reader)
		Expect(c.Upsert(dynamicchildren.Ref{WorkerType: WorkerType, Name: name}, map[string]any{})).To(Succeed())
		fsmv2client.SetClient(c)
		DeferCleanup(func() { fsmv2client.SetClient(nil) })

		want := []benthosmetrics.FailureEntry{{Signal: "output_error", Label: "output error", Count: 3}}

		// One good read fills the hold with the entry above; changing the
		// Failures that read returned must not reach the hold's copy.
		f, ok := Failures(context.Background(), name)
		Expect(ok).To(BeTrue(), "the good read answers")
		f.Entries[0] = benthosmetrics.FailureEntry{Signal: "mutated", Count: 99}

		// From here on the store read fails, so every read answers from the
		// hold; changing the Failures a held read returned must not reach
		// the hold's copy either.
		reader.setFail(true)
		held, ok := Failures(context.Background(), name)
		Expect(ok).To(BeTrue(), "a failed read answers from the last good Failures")
		Expect(held.Entries).To(Equal(want), "the hold kept its own copy of the good read's entries")
		held.Entries[0] = benthosmetrics.FailureEntry{Signal: "mutated", Count: 99}

		again, ok := Failures(context.Background(), name)
		Expect(ok).To(BeTrue(), "the next failed read answers from the last good Failures")
		Expect(again.Entries).To(Equal(want), "the hold serves its own copy, not a reader's changes")
	})
})

var _ = Describe("The Failures hold's sweep and forget", func() {
	// sampleWith wraps empty Failures in a stored observation: a healthy
	// Benthos, stored as its worker last scraped it.
	sampleWith := func() *fsmv2.Observation[simple.Status[benthosmetrics.Sample]] {
		return &fsmv2.Observation[simple.Status[benthosmetrics.Sample]]{
			CollectedAt: time.Now().Add(-100 * time.Millisecond),
			Status:      simple.Status[benthosmetrics.Sample]{Result: benthosmetrics.Sample{Failures: &benthosmetrics.Failures{}}},
		}
	}

	// heldInFailuresHold reports whether the hold currently has an entry for
	// name. The specs peek because the hold itself is the behaviour under
	// test.
	heldInFailuresHold := func(name string) bool {
		lastGoodMu.Lock()
		defer lastGoodMu.Unlock()

		_, held := lastGoodFailures[name]
		return held
	}

	It("drops a name from the hold on a not-found read", func() {
		name := "benthos-forgotten"
		reader := &stubManagerReader{}
		c := fsmv2client.NewFSMv2Client(dynamicchildren.NewWriter(), reader)
		fsmv2client.SetClient(c)
		DeferCleanup(func() { fsmv2client.SetClient(nil) })

		// One good read fills the hold.
		reader.obs = sampleWith()
		_, ok := Failures(context.Background(), name)
		Expect(ok).To(BeTrue(), "the good read answers")
		Expect(heldInFailuresHold(name)).To(BeTrue(), "the good read fills the hold")

		// The worker is gone: the read is not found and fails closed, and it
		// drops the entry, so the hold cannot outlive the fleet it serves.
		reader.err = persistence.ErrNotFound
		_, ok = Failures(context.Background(), name)
		Expect(ok).To(BeFalse(), "a not-found read fails closed")
		Expect(heldInFailuresHold(name)).To(BeFalse(),
			"the not-found read drops the name from the hold")
	})

	It("sweeps out names whose worker is no longer registered", func() {
		oldName := "benthos-swept-out"
		newName := "benthos-sweeping-in"

		reader := &perRefReader{obsByID: map[string]*fsmv2.Observation[simple.Status[benthosmetrics.Sample]]{}}
		c := fsmv2client.NewFSMv2Client(dynamicchildren.NewWriter(), reader)
		fsmv2client.SetClient(c)
		DeferCleanup(func() { fsmv2client.SetClient(nil) })

		// A worker whose read answers fills the hold.
		Expect(c.Upsert(dynamicchildren.Ref{WorkerType: WorkerType, Name: oldName}, map[string]any{})).To(Succeed())
		reader.obsByID[fsmv2config.ChildID(oldName)] = sampleWith()
		_, ok := Failures(context.Background(), oldName)
		Expect(ok).To(BeTrue(), "the old worker's read answers")
		Expect(heldInFailuresHold(oldName)).To(BeTrue(), "the old worker is held")

		// The worker is deleted and a new one appears. The new name's good
		// read sweeps the hold, so the deleted worker's entry cannot linger
		// in it.
		c.Delete(dynamicchildren.Ref{WorkerType: WorkerType, Name: oldName})
		Expect(c.Upsert(dynamicchildren.Ref{WorkerType: WorkerType, Name: newName}, map[string]any{})).To(Succeed())
		reader.obsByID[fsmv2config.ChildID(newName)] = sampleWith()
		_, ok = Failures(context.Background(), newName)
		Expect(ok).To(BeTrue(), "the new worker's read answers")
		Expect(heldInFailuresHold(newName)).To(BeTrue(), "the new worker is held")
		Expect(heldInFailuresHold(oldName)).To(BeFalse(),
			"adding a new name sweeps out names whose worker is no longer registered")
	})
})

var _ = Describe("ErrorsNotClearedCount", func() {
	// notCleared is the Failures of a worker that holds an output error
	// from 12 minutes ago: more than the 10-minute window plus the 1-minute
	// margin, so it should have cleared.
	notCleared := func() *benthosmetrics.Failures {
		now := time.Now()

		return &benthosmetrics.Failures{
			ObservedAt: now,
			Entries: []benthosmetrics.FailureEntry{{
				Signal: "output_error", Label: "output error", Count: 1,
				Since: now.Add(-12 * time.Minute), LastErrorAt: now.Add(-12 * time.Minute),
			}},
		}
	}

	// sampleWith wraps one Failures in a stored observation.
	sampleWith := func(f *benthosmetrics.Failures) *fsmv2.Observation[simple.Status[benthosmetrics.Sample]] {
		return &fsmv2.Observation[simple.Status[benthosmetrics.Sample]]{
			CollectedAt: time.Now().Add(-100 * time.Millisecond),
			Status:      simple.Status[benthosmetrics.Sample]{Result: benthosmetrics.Sample{Failures: f}},
		}
	}

	It("counts every registered Benthos worker holding an error that should have cleared", func() {
		// No client: no count.
		Expect(ErrorsNotClearedCount(context.Background(), time.Minute)).To(BeZero(), "no client")

		// A registry holding the Benthos workers of a data flow component, a
		// protocol converter's read and write sides, a topic browser and a
		// stand-alone flow. Every one of them holds an error that should
		// have cleared, so all five count; a converter with both sides stuck
		// counts twice.
		names := []string{
			"benthos-dataflow-dfc-1",
			"benthos-dataflow-read-protocolconverter-pc-1",
			"benthos-dataflow-write-protocolconverter-pc-1",
			"benthos-topicbrowser-tb-1",
			"benthos-standalone-1",
		}

		reader := &perRefReader{obsByID: map[string]*fsmv2.Observation[simple.Status[benthosmetrics.Sample]]{}}
		c := fsmv2client.NewFSMv2Client(dynamicchildren.NewWriter(), reader)
		fsmv2client.SetClient(c)
		DeferCleanup(func() { fsmv2client.SetClient(nil) })

		for _, name := range names {
			Expect(c.Upsert(dynamicchildren.Ref{WorkerType: WorkerType, Name: name}, map[string]any{})).To(Succeed())
			reader.obsByID[fsmv2config.ChildID(name)] = sampleWith(notCleared())
		}

		Expect(ErrorsNotClearedCount(context.Background(), time.Minute)).To(Equal(5),
			"the DFC, both converter sides, the topic browser and the stand-alone flow all count")

		// A worker whose errors cleared in time does not count, and neither
		// does a worker with no sample.
		fresh := &benthosmetrics.Failures{
			ObservedAt: time.Now(),
			Entries: []benthosmetrics.FailureEntry{{
				Signal: "output_error", Label: "output error", Count: 1,
				Since: time.Now().Add(-time.Minute), LastErrorAt: time.Now().Add(-time.Minute),
			}},
		}
		reader.obsByID[fsmv2config.ChildID("benthos-standalone-1")] = sampleWith(fresh)
		Expect(ErrorsNotClearedCount(context.Background(), time.Minute)).To(Equal(4),
			"the stand-alone flow cleared in time")

		delete(reader.obsByID, fsmv2config.ChildID("benthos-topicbrowser-tb-1"))
		Expect(ErrorsNotClearedCount(context.Background(), time.Minute)).To(Equal(3),
			"a worker with no stored sample does not count")
	})
})
