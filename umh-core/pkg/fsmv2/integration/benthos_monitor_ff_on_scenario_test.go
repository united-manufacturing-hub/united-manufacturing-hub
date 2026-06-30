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

package integration_test

import (
	"context"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	bmworker "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/benthos_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
)

// restartBenthosMetrics mirrors a freshly-restarted benthos whose counters have
// reset to zero. Its input_received counter is 0, distinct from the primary
// stub's 18, so R12b can prove a restart (count 0 < window's last 18) trips the
// counter-reset branch and wipes the throughput window — the parity-correct
// behavior the FF-off path also exhibits.
const restartBenthosMetrics = `# HELP input_received Benthos Counter metric
# TYPE input_received counter
input_received{label="",path="root.input"} 0
# HELP input_latency_ns Benthos Timing metric
# TYPE input_latency_ns summary
input_latency_ns_sum{label="",path="root.input"} 0
input_latency_ns_count{label="",path="root.input"} 0
# HELP output_sent Benthos Counter metric
# TYPE output_sent counter
output_sent{label="",path="root.output"} 0
`

// cliWatcher adapts the direct fsmv2client.FSMv2Client to the (unexported)
// benthos.benthosMonitorWatcher seam. It is the test stand-in for the production
// defaultBenthosMonitorWatcher, which delegates to the process-scoped singleton.
// Defining it here (rather than reusing the singleton) keeps the test hermetic:
// the cli writes through a per-test Writer and reads through a per-test store,
// both owned by the in-process application supervisor.
type cliWatcher struct {
	cli *fsmv2client.FSMv2Client
}

func (w cliWatcher) GetFresh(ctx context.Context, ref dynamicchildren.Ref, maxAge time.Duration) (bmworker.BenthosMonitorStatus, fsmv2client.Freshness, error) {
	return fsmv2client.GetFresh[bmworker.BenthosMonitorStatus](ctx, w.cli, ref, maxAge)
}

func (w cliWatcher) Upsert(ref dynamicchildren.Ref, cfg map[string]any) error {
	return w.cli.Upsert(ref, cfg)
}

func (w cliWatcher) Delete(ref dynamicchildren.Ref) {
	w.cli.Delete(ref)
}

// tickSupervisor is the subset of *supervisor.Supervisor the capstone drives:
// TestMarkAsStarted arms the supervisor, TestTick advances the collector one
// tick so the benthos_monitor child scrapes and publishes its observation.
type tickSupervisor interface {
	TestMarkAsStarted()
	TestTick(context.Context) error
}

var _ = Describe("FF-on benthos_monitor read path through the real supervisor", func() {
	const (
		configWorkerKey = "configworker"
		benthosName     = "cap1"
		// s6ServiceName is the ref Name GetHealthCheckAndMetrics builds
		// (benthos.go:658) and the S6 guard keys on (benthos.go:645). The
		// cli-watcher's ref MUST match.
		s6ServiceName = "benthos-cap1"
	)

	var (
		ctx      context.Context
		cancel   context.CancelFunc
		cli      *fsmv2client.FSMv2Client
		sup      tickSupervisor
		ref      dynamicchildren.Ref
		bSvc     *benthos.BenthosService
		stubSrvs []*httptest.Server
		readTick uint64
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 60*time.Second)

		// Arm the FF-on read/lifecycle paths via the cross-package seam.
		*benthos.EnableFSMv2BenthosMonitorForTest = true

		logger := deps.NewNopFSMLogger()
		reg := dynamicchildren.NewWriter()
		register.SetDeps[*dynamicchildren.Registry](configWorkerKey, reg.Registry())

		s, store, _ := newAppSupervisorWithStore(logger)
		s.TestMarkAsStarted()
		sup = s
		cli = fsmv2client.NewFSMv2Client(reg, store)

		// A mocked S6 manager whose GetInstance returns exists=true for
		// s6ServiceName, satisfying the :610 guard that gates the FF-on branch.
		s6Mgr := s6.NewS6ManagerWithMockedServices("cap")
		s6Mgr.AddInstanceForTest(s6ServiceName, nil)

		bSvc = benthos.NewDefaultBenthosService(benthosName,
			benthos.WithFSMv2BenthosWatcher(cliWatcher{cli: cli}),
			benthos.WithS6Manager(s6Mgr),
		)

		ref = dynamicchildren.Ref{WorkerType: bmworker.WorkerTypeName, Name: s6ServiceName}
		readTick = 0
	})

	AfterEach(func() {
		if cancel != nil {
			cancel()
		}
		register.ClearDeps(configWorkerKey)
		// Reset the FF so it cannot leak into a sibling spec in this suite.
		*benthos.EnableFSMv2BenthosMonitorForTest = false
		for _, srv := range stubSrvs {
			srv.Close()
		}
		stubSrvs = nil
	})

	// tickRead advances the supervisor one tick and calls the FF-on read path.
	// The tick is monotonic across the scenario so the throughput window sees
	// advancing ticks (the counter-reset branch keys off tick diffs).
	tickRead := func() (benthos.BenthosStatus, error) {
		Expect(sup.TestTick(ctx)).To(Succeed())
		readTick++

		return bSvc.GetHealthCheckAndMetrics(ctx, nil, readTick, time.Now(), benthosName, nil)
	}

	// upsert upserts a child-spec WITHOUT ticking (the caller ticks via
	// tickRead/Eventually). Splitting upsert from tick lets a scenario change
	// the stub state between the upsert and the ticks that observe it.
	upsert := func(state string, port uint16) {
		Expect(cli.Upsert(ref, map[string]any{"state": state, "metricsPort": port})).To(Succeed())
	}

	// waitUntil ticks+reads until pred holds on the returned status, returning
	// the last status. Mirrors the PR2 harness's tickLoad/Eventually pattern:
	// the benthos_monitor worker needs several ticks to spawn, scrape, and
	// publish, so single-tick assertions race the collector.
	waitUntil := func(desc string, pred func(got benthos.BenthosStatus) bool) benthos.BenthosStatus {
		var last benthos.BenthosStatus
		Eventually(func(g Gomega) bool {
			got, err := tickRead()
			if err != nil {
				return false
			}
			last = got

			return pred(got)
		}, "10s", "50ms").Should(BeTrue(), desc)

		return last
	}

	// R12a: a transient benthos down→up (NOT a restart — the SAME counters come
	// back) must NOT spike or wipe the throughput window. The C2 freeze (feed
	// only when MetricsAvailable) prevents the zero-feed that would trip the
	// counter-reset branch, so LastCount survives the outage and the resumed
	// feed sees count==LastCount (no reset, no spike).
	It("R12a: a transient down-up does not spike or wipe the throughput window", func() {
		stubSrv, stub := newBenthosStub(sampleBenthosMetrics)
		stubSrvs = append(stubSrvs, stubSrv)
		port := stubPort(stubSrv)

		// Step 1: up, counters=18. Window fed (LastCount=18, active).
		upsert("running", port)
		got1 := waitUntil("step 1: worker must reach Running with a fresh, healthy scan (IsLive, 18)",
			func(g benthos.BenthosStatus) bool {
				return g.HealthCheck.IsLive && g.BenthosMetrics.Metrics.InputReceivedTotal() == 18
			})
		Expect(got1.BenthosMetrics.MetricsState).ToNot(BeNil())
		Expect(got1.BenthosMetrics.MetricsState.Input.LastCount).To(Equal(int64(18)),
			"step 1: the window must be fed the stub's input_received counter (18)")
		Expect(got1.BenthosMetrics.MetricsState.IsActive).To(BeTrue(), "step 1: the window must be active")
		windowLenBeforeOutage := len(got1.BenthosMetrics.MetricsState.Input.Window)

		// Step 2: stub DOWN (transient outage, NOT a restart). IsLive=false,
		// MetricsAvailable=false → C2 freeze: the down read must NOT feed the
		// window. (During the lag before the worker's down scrape lands, the
		// read serves the pre-outage Fresh observation with MetricsAvailable=true
		// and correctly feeds — that is not the freeze; the freeze is that the
		// down observation itself, once served, does not advance LastTick.)
		stub.down.Store(true)
		got2 := waitUntil("step 2: a down stub must yield IsLive=false",
			func(g benthos.BenthosStatus) bool { return !g.HealthCheck.IsLive })
		Expect(got2.HealthCheck.IsLive).To(BeFalse(), "step 2: IsLive must be false when the stub is down")
		Expect(got2.BenthosMetrics.MetricsState.LastTick).To(BeNumerically("<", readTick),
			"step 2 (C2 freeze): the down read (MetricsAvailable=false) must NOT feed the window — LastTick stays below the current readTick")
		Expect(got2.BenthosMetrics.MetricsState.Input.LastCount).To(Equal(int64(18)),
			"step 2 (C2 freeze): LastCount must survive the outage (no zero-feed → no counter-reset)")

		// Step 3: stub UP again with the SAME counters (18 — transient, not a
		// restart). The C2 freeze kept LastCount=18, so the resumed feed sees
		// count=18 == LastCount=18 → NO counter-reset → no wipe, no spike.
		stub.down.Store(false)
		got3 := waitUntil("step 3: the stub is up again, IsLive must recover",
			func(g benthos.BenthosStatus) bool { return g.HealthCheck.IsLive })
		Expect(got3.HealthCheck.IsLive).To(BeTrue(), "step 3: IsLive must recover when the stub is up again")
		Expect(got3.BenthosMetrics.MetricsState.Input.LastCount).To(Equal(int64(18)),
			"step 3: LastCount must still be 18 (the counter-reset branch must NOT fire on a transient outage)")
		Expect(len(got3.BenthosMetrics.MetricsState.Input.Window)).To(BeNumerically(">=", windowLenBeforeOutage),
			"step 3: the window must NOT be wiped across a transient outage (pre-outage entries survive)")
		// The spike would be ~18 msg/tick (the full counter as a per-tick rate)
		// if the zero-feed had wiped the window. With the C2 freeze, the resumed
		// feed sees count==LastCount so countDiff=0 → rate 0 (continuous, no spike).
		Expect(got3.BenthosMetrics.MetricsState.Input.MessagesPerTick).To(BeNumerically("<", 18.0),
			"step 3: the rate must NOT spike to ~18 (the C2 freeze prevented the counter-reset a zero-feed would trip)")
	})

	// R12b: a crash+restart (the benthos comes back with counters reset to 0) is
	// a LEGITIMATE wipe: count 0 < window's last 18 trips the counter-reset
	// branch, wiping the window. This is parity-correct — the FF-off path wipes
	// the same way via its errgroup-freeze-then-feed — so it is NOT a regression.
	It("R12b: a crash-restart legitimately wipes the throughput window (parity-correct)", func() {
		stubSrv1, _ := newBenthosStub(sampleBenthosMetrics)
		stubSrvs = append(stubSrvs, stubSrv1)
		port1 := stubPort(stubSrv1)

		// Step 1: up, counters=18. Window fed (LastCount=18).
		upsert("running", port1)
		got1 := waitUntil("step 1: worker must reach Running with counters=18",
			func(g benthos.BenthosStatus) bool {
				return g.HealthCheck.IsLive && g.BenthosMetrics.Metrics.InputReceivedTotal() == 18
			})
		Expect(got1.BenthosMetrics.MetricsState.Input.LastCount).To(Equal(int64(18)),
			"step 1: the window must be seeded with the stub's counter (18)")

		// Step 2: the benthos crashed and restarted on a fresh port with counters
		// reset to 0 (a restarted benthos resets its Prometheus counters). The
		// scrape is Fresh + MetricsAvailable=true (stub2 is up), so the C2 guard
		// does NOT freeze — the feed happens with count=0.
		stubSrv2, _ := newBenthosStub(restartBenthosMetrics)
		stubSrvs = append(stubSrvs, stubSrv2)
		port2 := stubPort(stubSrv2)
		upsert("running", port2)
		got2 := waitUntil("step 2: the restarted stub (counters=0) must be scraped and served",
			func(g benthos.BenthosStatus) bool {
				return g.HealthCheck.IsLive && g.BenthosMetrics.Metrics.InputReceivedTotal() == 0
			})

		// count 0 < LastCount 18 → counter-reset branch fired → window wiped.
		Expect(got2.BenthosMetrics.MetricsState.Input.LastCount).To(Equal(int64(0)),
			"step 2: the counter-reset branch must fire (count 0 < last 18) and reset LastCount to 0")
		Expect(got2.BenthosMetrics.MetricsState.Input.Window).To(HaveLen(1),
			"step 2: the window must be wiped to a single reset entry (the counter-reset branch clears the window)")
		Expect(got2.BenthosMetrics.MetricsState.Input.MessagesPerTick).To(Equal(float64(0)),
			"step 2: the wiped window's rate is 0 (no fake throughput from a restarted counter)")
	})

	// R13: a stopped bridge (admin-paused) skips the scrape entirely. The
	// worker's COS guard leaves /metrics un-hit, and the read path's Stopped
	// guard returns empty+nil (Fresh + Stopped → NOT unhealthy, per v6.1c).
	It("R13: a stopped bridge skips the scrape and reads Fresh-stopped (not unhealthy)", func() {
		stubSrv, stub := newBenthosStub(sampleBenthosMetrics)
		stubSrvs = append(stubSrvs, stubSrv)
		port := stubPort(stubSrv)

		// Drive to running first so the worker has a live observation and the
		// /metrics hit counter is non-zero; the stop must NOT advance it.
		upsert("running", port)
		waitUntil("precondition: the running worker must scrape /metrics and read IsLive",
			func(g benthos.BenthosStatus) bool { return g.HealthCheck.IsLive })
		hitsBeforeStop := stub.metricsHits.Load()
		Expect(hitsBeforeStop).To(BeNumerically(">", 0),
			"precondition: the running worker must have scraped /metrics at least once")

		// Stop: the worker's COS scrape-skip must leave /metrics un-hit, and the
		// read path's Stopped guard returns empty+nil (Fresh+Stopped → not
		// unhealthy). The ref stays registered (no Delete), so the empty read is
		// Fresh+Stopped, NOT Unregistered. waitUntil ticks until the read returns
		// the empty status (nil MetricsState — the zero BenthosStatus carries no
		// window pointer, distinguishing it from a Fresh+MetricsAvailable read).
		upsert("stopped", port)
		waitUntil("R13: a stopped bridge must read Fresh+Stopped (empty BenthosStatus, nil MetricsState)",
			func(g benthos.BenthosStatus) bool {
				return g.BenthosMetrics.MetricsState == nil
			})
		got, err := tickRead()
		Expect(err).ToNot(HaveOccurred(),
			"R13: a stopped bridge must return empty+nil (Fresh+Stopped → not unhealthy, v6.1c)")
		Expect(got.BenthosMetrics.MetricsState).To(BeNil(),
			"R13: a stopped bridge must serve an empty BenthosStatus (nil window — not Degraded)")
		Expect(got.BenthosMetrics.Metrics.InputReceivedTotal()).To(Equal(int64(0)),
			"R13: a stopped bridge must serve zero metrics (the scrape was skipped)")
		Expect(stub.metricsHits.Load()).To(Equal(hitsBeforeStop),
			"R13: the /metrics endpoint must not be hit while the worker is Stopped (COS scrape-skip)")
	})

	// R14: a port change on the SAME ref (no Delete+re-add) keeps the ref
	// registered, so the read never returns Unregistered. After the Upsert of
	// the new port, ~one tick may still serve the stale old-port observation as
	// Fresh (the documented deviation — the CSE store does not clear a despawned
	// child's observation until ENG-5107); once the worker scrapes the new stub,
	// the new counters appear.
	It("R14: a port change on the same ref never reads Unregistered", func() {
		stubSrv1, stub1 := newBenthosStub(sampleBenthosMetrics)
		stubSrvs = append(stubSrvs, stubSrv1)
		port1 := stubPort(stubSrv1)

		// Step 1: stub1 scraped, counters=18 served as Fresh.
		upsert("running", port1)
		waitUntil("step 1: stub1 must be scraped and serve counters=18",
			func(g benthos.BenthosStatus) bool {
				return g.HealthCheck.IsLive && g.BenthosMetrics.Metrics.InputReceivedTotal() == 18
			})
		Expect(stub1.metricsHits.Load()).To(BeNumerically(">", 0),
			"step 1: stub1 must have been scraped")

		// Step 2: Upsert stub2 on the SAME ref (no Delete) + ONE tick. The ref
		// stays registered (Contains==true), so the read must NOT return an
		// error/Unregistered — it serves Fresh metrics (stale-old-port or new).
		stubSrv2, stub2 := newBenthosStub(altBenthosMetrics)
		stubSrvs = append(stubSrvs, stubSrv2)
		port2 := stubPort(stubSrv2)
		upsert("running", port2)
		// One tick after the port-change upsert.
		got2, err := tickRead()
		Expect(err).ToNot(HaveOccurred(),
			"R14: a port change on the same ref must NOT read Unregistered (no Delete → ref stays registered)")
		// Whatever the one-tick lag serves, it must be live metrics (not the
		// empty BenthosStatus an Unregistered read returns).
		Expect(got2.HealthCheck.IsLive).To(BeTrue(),
			"R14: the read after a port change must serve live metrics (stale-old-port or fresh-new-port)")

		// Step 3: eventually the worker scrapes stub2 and its counters (42)
		// appear. The KEY assertion is that the new stub IS scraped and served
		// without an Unregistered gap in between.
		waitUntil("R14: after the port change stub2's counters (42) must eventually be served",
			func(g benthos.BenthosStatus) bool {
				return g.BenthosMetrics.Metrics.InputReceivedTotal() == 42
			})
		Expect(stub2.metricsHits.Load()).To(BeNumerically(">", 0),
			"R14: stub2 must actually have been scraped")
	})
})
