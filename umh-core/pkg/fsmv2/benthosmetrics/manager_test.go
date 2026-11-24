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

package fsmv2benthosmetrics

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2"
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
// Observation[simple.Status[Scrape]] for every ref.
type stubManagerReader struct {
	obs *fsmv2.Observation[simple.Status[Scrape]]
	err error
}

func (s *stubManagerReader) LoadObservedTyped(_ context.Context, _, _ string, result any) error {
	if s.err != nil {
		return s.err
	}

	if s.obs == nil {
		return nil
	}

	out, ok := result.(*fsmv2.Observation[simple.Status[Scrape]])
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
	freshObs := func(degraded bool, reason string) *fsmv2.Observation[simple.Status[Scrape]] {
		return &fsmv2.Observation[simple.Status[Scrape]]{
			CollectedAt: time.Now().Add(-100 * time.Millisecond),
			Status: simple.Status[Scrape]{
				Result:   Scrape{},
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

		mgr := NewManager("test")
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
		mgr2 := NewManager("test")
		err, _ = mgr2.Reconcile(context.Background(), snapshotWith(benthosConfig("")), nil)
		Expect(err).NotTo(HaveOccurred())

		inst, iok := mgr2.GetInstance(name)
		Expect(iok).To(BeTrue())
		Expect(inst.GetDesiredFSMState()).To(Equal(benthosmonitorfsm.OperationalStateActive))
	})

	// The framework reads the child config with yaml.Unmarshal, so a key that
	// does not match Config's yaml tag would reach Poll as port 0.
	It("hands the worker the metrics port through the Upserted child spec", func() {
		reader := &stubManagerReader{err: persistence.ErrNotFound}
		writer := dynamicchildren.NewWriter()
		fsmv2client.SetClient(fsmv2client.NewFSMv2Client(writer, reader))

		mgr := NewManager("test")
		err, _ := mgr.Reconcile(context.Background(), snapshotWith(benthosConfig("active")), nil)
		Expect(err).NotTo(HaveOccurred())

		spec, ok := writer.Registry().Lookup(dynamicchildren.Ref{
			WorkerType: WorkerType,
			Name:       name,
		})
		Expect(ok).To(BeTrue(), "the enabled worker ref should be Upserted")

		var back Config
		Expect(yaml.Unmarshal([]byte(spec.UserSpec.Config), &back)).To(Succeed())
		Expect(back).To(Equal(Config{MetricsPort: 4195}))
	})

	// The framework degrades a worker after three missed scrapes: with one
	// scrape a second, an observation 2 s old is fresh and one 3.5 s old is
	// stale. The registered Interval is the only lever this worker has on that.
	It("degrades after three missed scrapes, not later", func() {
		reader := &stubManagerReader{}
		fsmv2client.SetClient(fsmv2client.NewFSMv2Client(dynamicchildren.NewWriter(), reader))

		mgr := NewManager("test")
		err, _ := mgr.Reconcile(context.Background(), snapshotWith(benthosConfig("active")), nil)
		Expect(err).NotTo(HaveOccurred())

		stateAtAge := func(age time.Duration) string {
			reader.obs = &fsmv2.Observation[simple.Status[Scrape]]{CollectedAt: time.Now().Add(-age)}
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

	// liveObs is a Fresh observation of a live, ready Benthos scraped at
	// takenAt.
	liveObs := func(takenAt time.Time) *fsmv2.Observation[simple.Status[Scrape]] {
		return &fsmv2.Observation[simple.Status[Scrape]]{
			CollectedAt: time.Now().Add(-100 * time.Millisecond),
			Status: simple.Status[Scrape]{
				Result: liveScrape(takenAt, true, true),
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

		mgr := NewManager("test")
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
			return t0.Add(time.Second + staleAfter + time.Millisecond)
		})
		defer restore()
		Expect(readIsLive()).To(BeFalse(),
			"past three seconds of failed reads the Benthos must read not live")
	})
})

var _ = Describe("The Failures lookup", func() {
	// Failures returns the failures of the named instance's stored sample,
	// false for an unknown instance, false when the instance has no sample
	// yet, and false when the store read fails.
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
		stored := &benthosmetrics.FailureReport{
			Entries: []benthosmetrics.FailureEntry{{
				Signal: "output_error", Label: "output error", Count: 1,
			}},
		}
		reader.err = nil
		reader.obs = &fsmv2.Observation[simple.Status[Scrape]]{
			CollectedAt: time.Now().Add(-100 * time.Millisecond),
			Status: simple.Status[Scrape]{
				Result: Scrape{Sample: benthosmetrics.Sample{Failures: stored}},
			},
		}

		f, ok = Failures(context.Background(), "benthos-1")
		Expect(ok).To(BeTrue(), "a stored sample answers")
		Expect(f).To(Equal(stored))

		// A minute-old sample still answers. Judging its age is the
		// caller's job.
		reader.obs.CollectedAt = time.Now().Add(-time.Minute)
		f, ok = Failures(context.Background(), "benthos-1")
		Expect(ok).To(BeTrue(), "a sample a minute old still answers")
		Expect(f).To(Equal(stored))

		// An observation whose sample has no Failures — a worker whose first
		// scrape failed — answers nothing.
		reader.obs.Status.Result = Scrape{}
		f, ok = Failures(context.Background(), "benthos-1")
		Expect(ok).To(BeFalse(), "a sample without Failures answers nothing")
		Expect(f).To(BeNil())

		// A store read that fails answers nothing, even after a good read.
		reader.obs.Status.Result = Scrape{Sample: benthosmetrics.Sample{Failures: stored}}
		reader.err = errors.New("store read timed out")
		f, ok = Failures(context.Background(), "benthos-1")
		Expect(ok).To(BeFalse(), "a failed store read answers nothing")
		Expect(f).To(BeNil())
	})
})

var _ = Describe("ErrorsNotClearedCount", func() {
	// sampleWith wraps one Failures, holding one output error whose last
	// error is lastErrorAgo old, in a stored observation.
	sampleWith := func(lastErrorAgo time.Duration) *fsmv2.Observation[simple.Status[Scrape]] {
		now := time.Now()

		return &fsmv2.Observation[simple.Status[Scrape]]{
			CollectedAt: now.Add(-100 * time.Millisecond),
			Status: simple.Status[Scrape]{Result: Scrape{Sample: benthosmetrics.Sample{Failures: &benthosmetrics.FailureReport{
				ObservedAt: now,
				Entries: []benthosmetrics.FailureEntry{{
					Signal: "output_error", Label: "output error", Count: 1,
					Since: now.Add(-lastErrorAgo), LastErrorAt: now.Add(-lastErrorAgo),
				}},
			}}}},
		}
	}

	It("counts every registered Benthos worker holding an error that should have cleared", func() {
		Expect(ErrorsNotClearedCount(context.Background(), time.Minute)).To(BeZero(), "no client")

		// Both sides of a protocol converter are stuck, so it counts twice.
		reader := &stubManagerReader{obs: sampleWith(benthosmetrics.ErrorWindow + 2*time.Minute)}
		c := fsmv2client.NewFSMv2Client(dynamicchildren.NewWriter(), reader)
		fsmv2client.SetClient(c)
		DeferCleanup(func() { fsmv2client.SetClient(nil) })

		for _, name := range []string{
			"benthos-dataflow-read-protocolconverter-pc-1",
			"benthos-dataflow-write-protocolconverter-pc-1",
		} {
			Expect(c.Upsert(dynamicchildren.Ref{WorkerType: WorkerType, Name: name}, map[string]any{})).To(Succeed())
		}

		Expect(ErrorsNotClearedCount(context.Background(), time.Minute)).To(Equal(2))

		reader.obs = sampleWith(time.Minute)
		Expect(ErrorsNotClearedCount(context.Background(), time.Minute)).To(BeZero(),
			"errors still inside the window do not count")

		reader.obs = sampleWith(benthosmetrics.ErrorWindow + 2*time.Minute)
		reader.err = errors.New("store read timed out")

		Expect(ErrorsNotClearedCount(context.Background(), time.Minute)).To(BeZero(),
			"a worker whose read fails does not count")
	})
})
