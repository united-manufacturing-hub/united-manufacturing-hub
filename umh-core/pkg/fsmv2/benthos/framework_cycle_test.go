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

// Scenario specs drive the REAL worker through the three framework entry
// points the supervisor uses on every tick — factory.NewWorkerByType, then
// fsmv2.Worker's DeriveDesiredState and CollectObservedState — against a local
// HTTP server on the four endpoints. A wiring regression surfaces here: a
// worker the registry cannot build, a child-spec config that does not
// round-trip through YAML, or a Poll the framework never reaches.

package fsmv2benthos

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2"
	fsmv2config "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/benthosmetrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	publicfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthosmonitorfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos_monitor"

	"gopkg.in/yaml.v3"
)

// userSpecFor renders the child spec cfgFor would produce: the config as YAML
// with the yaml-tag keys, which is what the worker unmarshals back into a
// config.BenthosMonitorConfig.
func userSpecFor(name string, port uint16) fsmv2config.UserSpec {
	raw, err := yaml.Marshal(config.BenthosMonitorConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{Name: name, DesiredFSMState: "active"},
		MetricsPort:       port,
	})
	Expect(err).NotTo(HaveOccurred(), "marshal child config")

	return fsmv2config.UserSpec{Config: string(raw)}
}

// observeOneCycle runs one full framework observation cycle — the Poll →
// Health cycle the supervisor's collector drives on every tick — and returns
// the stored status.
func observeOneCycle(w fsmv2.Worker, spec fsmv2config.UserSpec) simple.Status[benthosmetrics.Sample] {
	desired, err := w.DeriveDesiredState(spec)
	Expect(err).NotTo(HaveOccurred(), "DeriveDesiredState")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	observed, err := w.CollectObservedState(ctx, desired)
	Expect(err).NotTo(HaveOccurred(), "CollectObservedState")

	// The observation is generic to the framework; round-trip its JSON into
	// the worker's stored status type, which is what the fsmv1 adapter reads
	// back out of the store. A field the worker fails to publish cannot survive
	// this.
	blob, err := json.Marshal(observed)
	Expect(err).NotTo(HaveOccurred(), "marshal observation")

	var status simple.Status[benthosmetrics.Sample]
	err = json.Unmarshal(blob, &status)
	Expect(err).NotTo(HaveOccurred(), "unmarshal observation into the worker status")

	return status
}

// swapScrapeClock replaces the clock NewDeps hands each worker's Tracker and
// returns a restore func, so a spec can step scrape time without sleeping.
func swapScrapeClock(clock func() time.Time) func() {
	prior := scrapeClock
	scrapeClock = clock

	return func() { scrapeClock = prior }
}

var _ = Describe("The framework cycle", func() {
	// One Benthos runs through the real framework — registration, upsert,
	// poll, snapshot — and the observed state that reaches FSMv1 is the
	// sample the library produced.
	It("observes the worker through the framework", func() {
		s := newFakeBenthos()
		defer s.close()
		s.version = "4.95.0"
		s.setCounters(100, 80)

		spec := userSpecFor("scenario-benthos", s.port())

		// The worker's Tracker stamps scrapes with scrapeClock, captured when
		// the factory builds the worker. A stepping fake clock makes the rate
		// between the two observations exact, without a sleep.
		t0 := time.Now().Truncate(time.Second)
		clockNow := t0
		restore := swapScrapeClock(func() time.Time { return clockNow })
		defer restore()

		// Production instantiation: the registry must know this worker type,
		// and its factory must build a usable worker.
		worker, err := factory.NewWorkerByType(
			WorkerType,
			deps.Identity{ID: "scenario-benthos", Name: "scenario-benthos", WorkerType: WorkerType},
			deps.NewNopFSMLogger(),
			nil,
			nil,
		)
		Expect(err).NotTo(HaveOccurred(), "factory.NewWorkerByType(%q)", WorkerType)
		Expect(worker).NotTo(BeNil())

		// --- observation 1: the cold tick ---------------------------------
		clockNow = t0
		first := observeOneCycle(worker, spec)

		Expect(first.Degraded).To(BeFalse(), "a reachable Benthos must be healthy, got %q", first.Reason)
		Expect(first.Result.At.IsZero()).To(BeFalse(), "Poll did not record its start time through the framework")
		Expect(first.Result.PingAlive).To(BeTrue(), "/ping was not scraped through the framework")
		Expect(first.Result.Ready).To(BeTrue(), "/ready was not scraped or its empty error field was misread")
		Expect(first.Result.Version).To(Equal("4.95.0"))
		Expect(first.Result.Metrics.InputReceivedTotal()).To(Equal(int64(100)))
		Expect(first.Result.Metrics.OutputSentTotal()).To(Equal(int64(80)),
			"summed across both broker legs, not last-wins")
		// The rate window holds a single sample, so the rate is not yet
		// computable and the worker reports no throughput.
		Expect(first.Result.Input.MessagesPerSecond).To(Equal(0.0),
			"a rate needs two samples")
		Expect(first.Result.IsActive).To(BeFalse(), "a single sample must read inactive")

		// --- observation 2: traffic arrives ------------------------------
		// The window lives in the worker's deps and must survive across
		// observations. Raising the counters and stepping the clock by one
		// second must produce a rate and flip IsActive on.
		s.setCounters(140, 120)
		clockNow = t0.Add(time.Second)

		second := observeOneCycle(worker, spec)
		restore()

		Expect(second.Degraded).To(BeFalse(), "second observation, reason %q", second.Reason)
		Expect(second.Result.Metrics.InputReceivedTotal()).To(Equal(int64(140)),
			"the window did not survive the previous observation")
		Expect(second.Result.Metrics.OutputSentTotal()).To(Equal(int64(120)))
		Expect(second.Result.Input.MessagesPerSecond).To(Equal(40.0),
			"40 messages over the one second between the observations")
		Expect(second.Result.IsActive).To(BeTrue(), "input rose: the sample must read active")
	})

	// A Benthos whose endpoints answer nothing drives its worker degraded,
	// and the degraded verdict reaches FSMv1's observed state.
	It("reports an unreachable Benthos degraded", func() {
		s := newFakeBenthos()
		port := s.port()
		s.close() // nothing is listening now

		spec := userSpecFor("scenario-dead", port)

		worker, err := factory.NewWorkerByType(
			WorkerType,
			deps.Identity{ID: "scenario-dead", Name: "scenario-dead", WorkerType: WorkerType},
			deps.NewNopFSMLogger(),
			nil,
			nil,
		)
		Expect(err).NotTo(HaveOccurred(), "factory.NewWorkerByType")

		status := observeOneCycle(worker, spec)

		// The verdict comes from the framework, not the worker: a failed
		// scrape makes Poll return an error, and the poll-error path degrades
		// the observation without consulting Health.
		Expect(status.Degraded).To(BeTrue(), "a dead scrape must not read healthy")
		Expect(status.Result.IsActive).To(BeFalse(), "an unreachable Benthos reported IsActive")
	})

	// Two Benthos instances polled in turn, as the supervisor does every
	// tick: each worker keeps its own rate window, so a scrape of one does
	// not reset the other's.
	It("keeps a separate rate window per Benthos instance", func() {
		a, b := newFakeBenthos(), newFakeBenthos()
		defer a.close()
		defer b.close()
		a.setCounters(100, 0)
		b.setCounters(500, 0)

		t0 := time.Now().Truncate(time.Second)
		clockNow := t0
		defer swapScrapeClock(func() time.Time { return clockNow })()

		newWorker := func(name string) fsmv2.Worker {
			w, err := factory.NewWorkerByType(WorkerType,
				deps.Identity{ID: name, Name: name, WorkerType: WorkerType}, deps.NewNopFSMLogger(), nil, nil)
			Expect(err).NotTo(HaveOccurred(), "factory.NewWorkerByType(%q)", WorkerType)

			return w
		}
		workerA, workerB := newWorker("scenario-a"), newWorker("scenario-b")
		specA, specB := userSpecFor("scenario-a", a.port()), userSpecFor("scenario-b", b.port())

		_ = observeOneCycle(workerA, specA) // A at t0: 100
		clockNow = t0.Add(time.Second)
		_ = observeOneCycle(workerB, specB) // B in between
		a.setCounters(120, 0)
		clockNow = t0.Add(2 * time.Second)
		second := observeOneCycle(workerA, specA)

		Expect(second.Result.Input.MessagesPerSecond).To(Equal(10.0),
			"A's rate is 20 messages over its own 2 s, untouched by B's scrape")
	})

	// The worker's per-instance window state survives a Benthos process
	// restart, an error entry from before the restart clears at the first
	// scrape where the counter is back to 0, and errors counted between the
	// restart and that scrape still degrade the flow.
	It("keeps the window across a Benthos restart and clears pre-restart errors", func() {
		s := newFakeBenthos()
		defer s.close()

		spec := userSpecFor("scenario-restart", s.port())

		t0 := time.Now().Truncate(time.Second)
		clockNow := t0
		defer swapScrapeClock(func() time.Time { return clockNow })()

		worker, err := factory.NewWorkerByType(
			WorkerType,
			deps.Identity{ID: "scenario-restart", Name: "scenario-restart", WorkerType: WorkerType},
			deps.NewNopFSMLogger(),
			nil,
			nil,
		)
		Expect(err).NotTo(HaveOccurred(), "factory.NewWorkerByType")

		// No error yet.
		s.metricsBody = metricsBodyWith(100, 100, 0)
		clockNow = t0
		first := observeOneCycle(worker, spec)
		ok, _ := first.Result.Failures.ErrorFree()
		Expect(ok).To(BeTrue(), "errors before the first scrape are not backfilled")

		// One output error fires the entry.
		s.metricsBody = metricsBodyWith(100, 100, 1)
		clockNow = t0.Add(time.Second)
		fired := observeOneCycle(worker, spec)
		ok, _ = fired.Result.Failures.ErrorFree()
		Expect(ok).To(BeFalse(), "the error entry degrades the flow")

		// Benthos restarts and counts no error since: the counters are back
		// at 0, and the pre-restart entry clears at once.
		s.metricsBody = metricsBodyWith(0, 0, 0)
		clockNow = t0.Add(2 * time.Second)
		restarted := observeOneCycle(worker, spec)
		Expect(restarted.Result.Metrics.InputReceivedTotal()).To(Equal(int64(0)),
			"the restart reset the counters")
		ok, _ = restarted.Result.Failures.ErrorFree()
		Expect(ok).To(BeTrue(), "a restart clears the errors from before it")

		// Benthos runs error-free for a while.
		s.metricsBody = metricsBodyWith(40, 40, 0)
		clockNow = t0.Add(3 * time.Second)
		_ = observeOneCycle(worker, spec)

		// Benthos restarts again and counts three errors before the first
		// scrape after the restart. They still degrade the flow, and the
		// entry carries their count: the worker kept its window state across
		// the restart and compares the new counters with 0.
		s.metricsBody = metricsBodyWith(0, 0, 3)
		clockNow = t0.Add(4 * time.Second)
		degraded := observeOneCycle(worker, spec)
		ok, _ = degraded.Result.Failures.ErrorFree()
		Expect(ok).To(BeFalse(),
			"errors counted between the restart and the first scrape after it still degrade")
		Expect(degraded.Result.Failures.Entries).To(HaveLen(1))
		Expect(degraded.Result.Failures.Entries[0].Count).To(Equal(int64(3)))
	})

	// A Benthos whose metrics stop arriving counts as stale: the worker
	// reports degraded after three missed scrapes (3 s).
	It("degrades on stale metrics after three missed scrapes", func() {
		s := newFakeBenthos()
		defer s.close()
		s.setCounters(100, 80)

		spec := userSpecFor("scenario-stale", s.port())

		worker, err := factory.NewWorkerByType(
			WorkerType,
			deps.Identity{ID: "scenario-stale", Name: "scenario-stale", WorkerType: WorkerType},
			deps.NewNopFSMLogger(),
			nil,
			nil,
		)
		Expect(err).NotTo(HaveOccurred(), "factory.NewWorkerByType")

		// One fresh scrape; its observation is what the store holds from now
		// on, because the scrapes after it stop arriving.
		status := observeOneCycle(worker, spec)

		reader := &stubManagerReader{obs: &fsmv2.Observation[simple.Status[benthosmetrics.Sample]]{
			CollectedAt: time.Now(),
			Status:      status,
		}}
		fsmv2client.SetClient(fsmv2client.NewFSMv2Client(dynamicchildren.NewWriter(), reader))
		DeferCleanup(func() { fsmv2client.SetClient(nil) })

		mgr := NewFsmv2BenthosMetricsManager("scenario")
		snapshot := fsmv2testSnapshot("scenario-stale", s.port())
		rerr, _ := mgr.Reconcile(context.Background(), snapshot, nil)
		Expect(rerr).NotTo(HaveOccurred(), "Reconcile")

		stateAt := func(age time.Duration) string {
			reader.obs.CollectedAt = time.Now().Add(-age)
			state, gerr := mgr.GetCurrentFSMState("scenario-stale")
			Expect(gerr).NotTo(HaveOccurred(), "GetCurrentFSMState at age %v", age)

			return state
		}

		Expect(stateAt(2*time.Second)).To(Equal(benthosmonitorfsm.OperationalStateActive),
			"two missed scrapes must not degrade")
		Expect(stateAt(3500*time.Millisecond)).To(Equal(benthosmonitorfsm.OperationalStateDegraded),
			"three missed scrapes must degrade")
	})

	// With the flag on exactly one live copy of the failure window state
	// exists per Benthos instance, in the worker's deps, that
	// IsMetricsErrorFree and the not-cleared count read through the package's
	// Failures lookup, and the state is dropped when the instance is
	// removed.
	It("gives the flagged-on worker the only live copy of the failure window state", func() {
		s := newFakeBenthos()
		defer s.close()
		s.metricsBody = metricsBodyWith(100, 80, 0)

		spec := userSpecFor("scenario-window", s.port())

		t0 := time.Now().Truncate(time.Second)
		clockNow := t0
		defer swapScrapeClock(func() time.Time { return clockNow })()

		worker, err := factory.NewWorkerByType(
			WorkerType,
			deps.Identity{ID: "scenario-window", Name: "scenario-window", WorkerType: WorkerType},
			deps.NewNopFSMLogger(),
			nil,
			nil,
		)
		Expect(err).NotTo(HaveOccurred(), "factory.NewWorkerByType")

		// The window state the worker's deps hold: seeded by the first scrape,
		// degraded by the error the second finds.
		clockNow = t0
		_ = observeOneCycle(worker, spec)

		s.metricsBody = metricsBodyWith(100, 80, 2)
		clockNow = t0.Add(time.Second)
		fired := observeOneCycle(worker, spec)
		Expect(fired.Result.Failures).NotTo(BeNil())
		Expect(fired.Result.Failures.Entries).To(HaveLen(1),
			"the worker's deps hold the failure window state")

		// The readers reach that one copy through the package's Failures
		// lookup: the stored observation answers with the worker's own
		// failures.
		client := fsmv2client.NewFSMv2Client(dynamicchildren.NewWriter(), &stubManagerReader{
			obs: &fsmv2.Observation[simple.Status[benthosmetrics.Sample]]{
				CollectedAt: time.Now(),
				Status:      fired,
			},
		})
		fsmv2client.SetClient(client)
		DeferCleanup(func() { fsmv2client.SetClient(nil) })

		f, ok := Failures(context.Background(), "scenario-window")
		Expect(ok).To(BeTrue(), "the lookup answers from the worker's stored state")
		Expect(f).To(Equal(fired.Result.Failures),
			"the lookup returns the one live copy the worker's deps hold")

		// The state is dropped when the instance is removed.
		client.Delete(dynamicchildren.Ref{WorkerType: WorkerType, Name: "scenario-window"})
		f, ok = Failures(context.Background(), "scenario-window")
		Expect(ok).To(BeFalse(), "a removed instance has no state left")
		Expect(f).To(BeNil())
	})
})

// metricsBodyWith renders the /metrics exposition the scenario specs scrape:
// one input and output path, and an output error total of err.
func metricsBodyWith(input, output, err int64) string {
	return fmt.Sprintf(
		"input_received{label=\"\",path=\"root.input\"} %d\n"+
			"output_sent{label=\"\",path=\"root.output\"} %d\n"+
			"output_error{label=\"\",path=\"root.output\"} %d\n",
		input, output, err)
}

// fsmv2testSnapshot puts one enabled BenthosMonitorConfig at the real
// snapshot path the manager's ExtractConfigs reads.
func fsmv2testSnapshot(name string, port uint16) publicfsm.SystemSnapshot {
	return publicfsm.SystemSnapshot{
		CurrentConfig: config.FullConfig{
			Internal: config.InternalConfig{BenthosMonitor: []config.BenthosMonitorConfig{{
				FSMInstanceConfig: config.FSMInstanceConfig{
					Name:            name,
					DesiredFSMState: "active",
				},
				MetricsPort: port,
			}}},
		},
	}
}
