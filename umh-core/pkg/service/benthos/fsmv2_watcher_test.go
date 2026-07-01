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

package benthos

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	public_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthos_monitor_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	bmworker "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/benthos_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthosmetrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// fakeBenthosMonitorWatcher is a test double for benthosMonitorWatcher. It
// returns a canned (status, res, err) and records the ref/maxAge it was called
// with so the test can assert the FF-on read path used s6ServiceName (not raw
// benthosName) for the ref Name. Upsert/Delete calls are recorded so the R8/R9
// lifecycle tests can assert the refs and maps they were called with.
type fakeBenthosMonitorWatcher struct {
	status    bmworker.BenthosMonitorStatus
	res       fsmv2client.Freshness
	err       error
	gotRef    dynamicchildren.Ref
	gotMaxAge time.Duration

	upsertCalls []upsertCall
	deleteCalls []dynamicchildren.Ref
}

// upsertCall records one Upsert invocation's ref and config map.
type upsertCall struct {
	ref dynamicchildren.Ref
	cfg map[string]any
}

func (f *fakeBenthosMonitorWatcher) GetFresh(_ context.Context, ref dynamicchildren.Ref, maxAge time.Duration) (bmworker.BenthosMonitorStatus, fsmv2client.Freshness, error) {
	f.gotRef = ref
	f.gotMaxAge = maxAge

	return f.status, f.res, f.err
}

func (f *fakeBenthosMonitorWatcher) Upsert(ref dynamicchildren.Ref, cfg map[string]any) error {
	f.upsertCalls = append(f.upsertCalls, upsertCall{ref: ref, cfg: cfg})

	return nil
}

func (f *fakeBenthosMonitorWatcher) Delete(ref dynamicchildren.Ref) {
	f.deleteCalls = append(f.deleteCalls, ref)
}

// TestFSMv2Watcher_GetHealthCheckAndMetrics_FreshReadPath verifies the FF-on
// (USE_FSMV2_BENTHOS_MONITOR) Fresh branch of GetHealthCheckAndMetrics:
//
//   - The returned HealthCheck is the worker's raw scan.HealthCheck (not
//     synthesized).
//   - The returned Metrics are the worker's raw scan.Metrics.
//   - The returned MetricsState is the BenthosService's own s.window pointer
//     (the live instance, not a copy).
//   - s.window was fed (LastTick advanced, IsActive reflects the non-zero
//     input metrics) — only when MetricsAvailable is true (C2 freeze guard).
//   - err is nil.
//   - The watcher ref uses s6ServiceName ("benthos-"+name), not raw benthosName.
//
// Each assertion is front-loaded so the test FAILS before the FF-on branch
// exists: the FF-off path never calls the fake watcher, so gotRef stays zero
// and the returned status comes from the (nil) v1 monitor manager instead of
// the fake.
func TestFSMv2Watcher_GetHealthCheckAndMetrics_FreshReadPath(t *testing.T) {
	// --- FF-on ---
	prev := fsmv2BenthosMonitorEnabled
	fsmv2BenthosMonitorEnabled = true

	t.Cleanup(func() { fsmv2BenthosMonitorEnabled = prev })

	const (
		benthosName        = "testbridge"
		tick        uint64 = 42
	)

	scan := benthosmetrics.Scan{
		Metrics: benthosmetrics.Metrics{
			Inputs: map[string]benthosmetrics.InputInstance{
				"in1": {Received: 5},
			},
		},
		HealthCheck: benthosmetrics.HealthCheck{
			IsLive:  true,
			IsReady: true,
			Version: "v1.2.3",
		},
		MetricsAvailable: true,
	}
	fake := &fakeBenthosMonitorWatcher{
		status: bmworker.BenthosMonitorStatus{Scan: scan, Stopped: false},
		res:    fsmv2client.Fresh,
	}

	s := NewDefaultBenthosService(benthosName, WithFSMv2BenthosWatcher(fake))

	// The GetInstance guard runs BEFORE the FF-on branch, so the s6 service
	// must be registered for the read path to reach the watcher. The instance
	// value is unused on the FF-on path (only existence is checked).
	s.s6Manager.AddInstanceForTest(s.GetS6ServiceName(benthosName), nil)

	got, err := s.GetHealthCheckAndMetrics(context.Background(), nil, tick, time.Now(), benthosName, nil)

	// (e) err == nil
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	// (a) HealthCheck == fake's scan.HealthCheck (raw, not synthesized).
	if !reflect.DeepEqual(got.HealthCheck, scan.HealthCheck) {
		t.Errorf("HealthCheck = %+v, want %+v", got.HealthCheck, scan.HealthCheck)
	}

	// (b) Metrics == fake's scan.Metrics.
	if !reflect.DeepEqual(got.BenthosMetrics.Metrics, scan.Metrics) {
		t.Errorf("Metrics = %+v, want %+v", got.BenthosMetrics.Metrics, scan.Metrics)
	}

	// (c) MetricsState == s.window (same pointer — the live instance).
	if got.BenthosMetrics.MetricsState != s.window {
		t.Errorf("MetricsState = %p, want s.window %p (must be the live pointer)",
			got.BenthosMetrics.MetricsState, s.window)
	}

	// (d) s.window was fed — LastTick advanced to tick and IsActive reflects
	// the non-zero input metrics (Received=5 > 0).
	if s.window.LastTick != tick {
		t.Errorf("window.LastTick = %d, want %d (window must be fed on Fresh+MetricsAvailable)",
			s.window.LastTick, tick)
	}

	if !s.window.IsActive {
		t.Errorf("window.IsActive = false, want true (non-zero input metrics were fed)")
	}

	// The watcher ref must use s6ServiceName ("benthos-"+name), not raw
	// benthosName — the monitor FSM keys on s6ServiceName; a mismatch makes
	// Contains==false and every read returns Unregistered.
	wantRef := dynamicchildren.Ref{WorkerType: "benthos_monitor", Name: s.GetS6ServiceName(benthosName)}

	if fake.gotRef != wantRef {
		t.Errorf("watcher ref = %+v, want %+v (must use s6ServiceName)", fake.gotRef, wantRef)
	}

	if fake.gotMaxAge != benthosMonitorMaxAge {
		t.Errorf("watcher maxAge = %v, want %v", fake.gotMaxAge, benthosMonitorMaxAge)
	}
}

// newFFOnService builds a BenthosService wired to the FF-on read path with the
// given fake watcher. It registers the s6 instance (the GetInstance guard at
// the top of GetHealthCheckAndMetrics runs before the FF-on branch) and arms
// the feature flag with a cleanup that restores the prior value.
func newFFOnService(t *testing.T, benthosName string, fake *fakeBenthosMonitorWatcher) *BenthosService {
	t.Helper()

	prev := fsmv2BenthosMonitorEnabled
	fsmv2BenthosMonitorEnabled = true

	t.Cleanup(func() { fsmv2BenthosMonitorEnabled = prev })

	s := NewDefaultBenthosService(benthosName, WithFSMv2BenthosWatcher(fake))
	s.s6Manager.AddInstanceForTest(s.GetS6ServiceName(benthosName), nil)

	return s
}

// nonZeroMetrics returns a Metrics value with a non-zero input so deep-equality
// can distinguish "served" from "zero/empty" in the assertions.
func nonZeroMetrics() benthosmetrics.Metrics {
	return benthosmetrics.Metrics{
		Inputs: map[string]benthosmetrics.InputInstance{
			"in1": {Received: 5},
		},
	}
}

// TestFSMv2Watcher_GetHealthCheckAndMetrics_Fresh_NoMetricsAvailable_FreezesWindow
// pins R1's C2 freeze guard: when the scrape is Fresh but MetricsAvailable is
// false (benthos down/unreachable), the status is still served but s.window is
// NOT fed (a counter-reset spike must not freeze the throughput window).
//
// Forcing assertion: s.window.LastTick is unchanged across the call. Without
// the MetricsAvailable gate at benthos.go:680, the feed would advance LastTick
// to tick and this test would fail.
func TestFSMv2Watcher_GetHealthCheckAndMetrics_Fresh_NoMetricsAvailable_FreezesWindow(t *testing.T) {
	const (
		benthosName        = "testbridge"
		tick        uint64 = 42
	)

	metrics := nonZeroMetrics()
	scan := benthosmetrics.Scan{
		Metrics:          metrics,
		HealthCheck:      benthosmetrics.HealthCheck{IsLive: true, IsReady: true, Version: "v1.2.3"},
		MetricsAvailable: false,
	}
	fake := &fakeBenthosMonitorWatcher{
		status: bmworker.BenthosMonitorStatus{Scan: scan, Stopped: false},
		res:    fsmv2client.Fresh,
	}
	s := newFFOnService(t, benthosName, fake)

	// Pre-seed a known LastTick so "unchanged" is a meaningful assertion (not
	// just the zero default).
	const seedTick uint64 = 99

	s.window.LastTick = seedTick

	got, err := s.GetHealthCheckAndMetrics(context.Background(), nil, tick, time.Now(), benthosName, nil)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	// The Fresh branch serves the status regardless of MetricsAvailable.
	if !reflect.DeepEqual(got.BenthosMetrics.Metrics, metrics) {
		t.Errorf("Metrics = %+v, want %+v (Fresh status must be served even when MetricsAvailable=false)", got.BenthosMetrics.Metrics, metrics)
	}

	// C2 freeze: the window must NOT be fed when MetricsAvailable is false.
	if s.window.LastTick != seedTick {
		t.Errorf("window.LastTick = %d, want %d (window must not be fed when MetricsAvailable=false — the C2 freeze)",
			s.window.LastTick, seedTick)
	}
}

// TestFSMv2Watcher_GetHealthCheckAndMetrics_Stale_ServesLastGoodOverridesIsLive
// verifies the Stale branch (R3): a wedged watcher (CollectedAt > maxAge) still
// surfaces the stored last-good Metrics, but overrides IsLive=false so the
// bridge reads as unhealthy. The window is NOT fed (C2: feed only on Fresh).
//
// Forcing assertion: got.BenthosMetrics.Metrics == stale M. Before R3 is
// implemented, the default stub returns BenthosStatus{} (zero Metrics), so this
// assertion fails RED — the stale Metrics are not served.
func TestFSMv2Watcher_GetHealthCheckAndMetrics_Stale_ServesLastGoodOverridesIsLive(t *testing.T) {
	const (
		benthosName        = "testbridge"
		tick        uint64 = 42
	)

	metrics := nonZeroMetrics()
	scan := benthosmetrics.Scan{
		Metrics:     metrics,
		HealthCheck: benthosmetrics.HealthCheck{IsLive: true, IsReady: true, Version: "v1.2.3"},
	}
	fake := &fakeBenthosMonitorWatcher{
		status: bmworker.BenthosMonitorStatus{Scan: scan, Stopped: false},
		res:    fsmv2client.Stale,
	}
	s := newFFOnService(t, benthosName, fake)

	const seedTick uint64 = 99

	s.window.LastTick = seedTick

	got, err := s.GetHealthCheckAndMetrics(context.Background(), nil, tick, time.Now(), benthosName, nil)
	if err != nil {
		t.Fatalf("err = %v, want nil (Stale serves last-good, no error)", err)
	}

	// Stale serves the stored last-good Metrics.
	if !reflect.DeepEqual(got.BenthosMetrics.Metrics, metrics) {
		t.Errorf("Metrics = %+v, want %+v (Stale must serve the stored last-good Metrics)", got.BenthosMetrics.Metrics, metrics)
	}

	// IsLive is OVERRIDDEN to false — a wedged watcher is unhealthy even if the
	// stale scan said IsLive=true.
	if got.HealthCheck.IsLive {
		t.Errorf("HealthCheck.IsLive = true, want false (Stale must override IsLive to false)")
	}

	// C2: the window is NOT fed on Stale (feed only on Fresh+MetricsAvailable).
	if s.window.LastTick != seedTick {
		t.Errorf("window.LastTick = %d, want %d (window must not be fed on Stale)", s.window.LastTick, seedTick)
	}
}

// TestFSMv2Watcher_GetHealthCheckAndMetrics_Unregistered_ServesEmptyNil pins
// the default stub's behavior for Unregistered: serve empty/not-ready (warming),
// no error, and NOT the ErrServiceNotExist sentinel.
func TestFSMv2Watcher_GetHealthCheckAndMetrics_Unregistered_ServesEmptyNil(t *testing.T) {
	const benthosName = "testbridge"

	fake := &fakeBenthosMonitorWatcher{
		status: bmworker.BenthosMonitorStatus{},
		res:    fsmv2client.Unregistered,
	}
	s := newFFOnService(t, benthosName, fake)

	got, err := s.GetHealthCheckAndMetrics(context.Background(), nil, 1, time.Now(), benthosName, nil)
	if err != nil {
		t.Fatalf("err = %v, want nil (Unregistered serves empty, no error)", err)
	}

	want := BenthosStatus{}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got = %+v, want %+v (Unregistered must serve empty BenthosStatus)", got, want)
	}
}

// TestFSMv2Watcher_GetHealthCheckAndMetrics_NeverObserved_ServesEmptyNil pins
// the default stub's behavior for NeverObserved: serve empty/not-ready
// (warming), no error.
func TestFSMv2Watcher_GetHealthCheckAndMetrics_NeverObserved_ServesEmptyNil(t *testing.T) {
	const benthosName = "testbridge"

	fake := &fakeBenthosMonitorWatcher{
		status: bmworker.BenthosMonitorStatus{},
		res:    fsmv2client.NeverObserved,
	}
	s := newFFOnService(t, benthosName, fake)

	got, err := s.GetHealthCheckAndMetrics(context.Background(), nil, 1, time.Now(), benthosName, nil)
	if err != nil {
		t.Fatalf("err = %v, want nil (NeverObserved serves empty, no error)", err)
	}

	want := BenthosStatus{}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got = %+v, want %+v (NeverObserved must serve empty BenthosStatus)", got, want)
	}
}

// TestFSMv2Watcher_GetHealthCheckAndMetrics_Unknown_ReturnsErrRaw pins R1's
// err-first guard: when GetFresh returns a non-nil err (store read failure),
// the err is returned raw (reconcile.go catches it non-fatally) and the status
// is empty. The Freshness is Unknown (the zero value — only meaningful when err
// is nil).
func TestFSMv2Watcher_GetHealthCheckAndMetrics_Unknown_ReturnsErrRaw(t *testing.T) {
	const benthosName = "testbridge"

	sentinel := errors.New("store read failed")
	fake := &fakeBenthosMonitorWatcher{
		status: bmworker.BenthosMonitorStatus{},
		res:    fsmv2client.Unknown,
		err:    sentinel,
	}
	s := newFFOnService(t, benthosName, fake)

	got, err := s.GetHealthCheckAndMetrics(context.Background(), nil, 1, time.Now(), benthosName, nil)

	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want the sentinel %v (Unknown must return the raw err)", err, sentinel)
	}

	want := BenthosStatus{}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got = %+v, want %+v (Unknown must return empty BenthosStatus)", got, want)
	}
}

// TestFSMv2Watcher_GetHealthCheckAndMetrics_Fresh_Stopped_NotUnhealthy pins
// R1's stopped guard: a Fresh + Stopped=true bridge (admin-paused, scrape
// skipped) returns empty+nil — NOT Degraded. IsLive=false on a stopped bridge
// must not surface as unhealthy.
func TestFSMv2Watcher_GetHealthCheckAndMetrics_Fresh_Stopped_NotUnhealthy(t *testing.T) {
	const benthosName = "testbridge"

	fake := &fakeBenthosMonitorWatcher{
		status: bmworker.BenthosMonitorStatus{Stopped: true, Scan: benthosmetrics.Scan{}},
		res:    fsmv2client.Fresh,
	}
	s := newFFOnService(t, benthosName, fake)

	got, err := s.GetHealthCheckAndMetrics(context.Background(), nil, 1, time.Now(), benthosName, nil)
	if err != nil {
		t.Fatalf("err = %v, want nil (Fresh+Stopped returns empty, no error)", err)
	}

	want := BenthosStatus{}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got = %+v, want %+v (Fresh+Stopped must return empty BenthosStatus, not Degraded)", got, want)
	}
}

// fakeBenthosMonitorManager is a test double for BenthosMonitorReconciler. It
// records Reconcile calls so the R11 test can assert the FF-on path SKIPS the
// S6 monitor reconcile. GetInstance returns not-found so the Remove path
// proceeds to the window reset.
//
// GetLastObservedState drains a queue of scripted observed states
// (observedStates) so the FF-off down→recovery parity test can drive the read
// path through a healthy scan, a retained-last-good outage, and a fresh
// recovery scan. When the queue is empty it returns (nil, nil) — preserving
// the behavior the R10/R11 tests rely on.
type fakeBenthosMonitorManager struct {
	reconcileCalls int
	observedStates []public_fsm.ObservedState
}

func (f *fakeBenthosMonitorManager) Reconcile(_ context.Context, _ public_fsm.SystemSnapshot, _ serviceregistry.Provider) (error, bool) {
	f.reconcileCalls++

	return nil, false
}

func (f *fakeBenthosMonitorManager) GetInstance(_ string) (public_fsm.FSMInstance, bool) {
	return nil, false
}

func (f *fakeBenthosMonitorManager) GetLastObservedState(_ string) (public_fsm.ObservedState, error) {
	if len(f.observedStates) == 0 {
		return nil, nil
	}

	state := f.observedStates[0]
	f.observedStates = f.observedStates[1:]

	return state, nil
}

// TestFSMv2Watcher_ReconcileManager_FFOn_UpsertsEachConfig_SkipsS6MonitorReconcile
// verifies R8 (Upsert each config + mapFrom) and R11 (skip the S6 monitor
// reconcile) under FF-on:
//
//   - R8: for each cfg in s.benthosMonitorConfigs, the watcher's Upsert is
//     called with ref{WorkerType:"benthos_monitor", Name:cfg.Name} and a map
//     {state: mapFrom(cfg.DesiredFSMState), metricsPort: cfg.MetricsPort}.
//     Active→"running", Stopped→"stopped".
//   - R11: benthosMonitorManager.Reconcile is NOT called (the S6 fork tree
//     never spawns — the CPU win).
//
// Forcing assertion: fake.upsertCalls has exactly 2 entries with the right
// refs/maps AND fakeBenthosMonitorManager.reconcileCalls == 0. Before the FF-on
// branch exists, no Upsert is called (upsertCalls is empty) and Reconcile IS
// called (reconcileCalls == 1), so both halves fail RED.
func TestFSMv2Watcher_ReconcileManager_FFOn_UpsertsEachConfig_SkipsS6MonitorReconcile(t *testing.T) {
	prev := fsmv2BenthosMonitorEnabled
	fsmv2BenthosMonitorEnabled = true

	t.Cleanup(func() { fsmv2BenthosMonitorEnabled = prev })

	fake := &fakeBenthosMonitorWatcher{}
	monMgr := &fakeBenthosMonitorManager{}
	s := NewDefaultBenthosService("test",
		WithFSMv2BenthosWatcher(fake),
		WithMonitorManager(monMgr),
	)

	// cfg.Name IS s6ServiceName (set at AddBenthosToS6Manager:706/:765 — NOT raw
	// benthosName). Directly set two monitor configs with Active + Stopped.
	s.benthosMonitorConfigs = []config.BenthosMonitorConfig{
		{
			FSMInstanceConfig: config.FSMInstanceConfig{
				Name:            "benthos-a",
				DesiredFSMState: benthos_monitor_fsm.OperationalStateActive,
			},
			MetricsPort: 4195,
		},
		{
			FSMInstanceConfig: config.FSMInstanceConfig{
				Name:            "benthos-b",
				DesiredFSMState: benthos_monitor_fsm.OperationalStateStopped,
			},
			MetricsPort: 4196,
		},
	}

	// The s6 manager's Reconcile requires a context with a deadline (it enforces
	// a per-tick time budget). Use a generous deadline so the empty-config
	// reconcile completes instantly.
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(100*time.Second))
	defer cancel()

	_, _ = s.ReconcileManager(
		ctx,
		serviceregistry.NewMockRegistry(),
		public_fsm.SystemSnapshot{Tick: 1, SnapshotTime: time.Now()},
	)

	// --- R8: Upsert called twice with correct refs + maps ---

	if len(fake.upsertCalls) != 2 {
		t.Fatalf("upsertCalls = %d, want 2 (one per config)", len(fake.upsertCalls))
	}

	wantRefA := dynamicchildren.Ref{WorkerType: "benthos_monitor", Name: "benthos-a"}
	if fake.upsertCalls[0].ref != wantRefA {
		t.Errorf("upsertCalls[0].ref = %+v, want %+v", fake.upsertCalls[0].ref, wantRefA)
	}

	if state, ok := fake.upsertCalls[0].cfg["state"].(string); !ok || state != "running" {
		t.Errorf("upsertCalls[0].cfg[\"state\"] = %v, want \"running\" (Active→running)", fake.upsertCalls[0].cfg["state"])
	}

	if port, ok := fake.upsertCalls[0].cfg["metricsPort"].(uint16); !ok || port != 4195 {
		t.Errorf("upsertCalls[0].cfg[\"metricsPort\"] = %v, want uint16(4195)", fake.upsertCalls[0].cfg["metricsPort"])
	}

	wantRefB := dynamicchildren.Ref{WorkerType: "benthos_monitor", Name: "benthos-b"}
	if fake.upsertCalls[1].ref != wantRefB {
		t.Errorf("upsertCalls[1].ref = %+v, want %+v", fake.upsertCalls[1].ref, wantRefB)
	}

	if state, ok := fake.upsertCalls[1].cfg["state"].(string); !ok || state != "stopped" {
		t.Errorf("upsertCalls[1].cfg[\"state\"] = %v, want \"stopped\" (Stopped→stopped)", fake.upsertCalls[1].cfg["state"])
	}

	if port, ok := fake.upsertCalls[1].cfg["metricsPort"].(uint16); !ok || port != 4196 {
		t.Errorf("upsertCalls[1].cfg[\"metricsPort\"] = %v, want uint16(4196)", fake.upsertCalls[1].cfg["metricsPort"])
	}

	// --- R11: benthosMonitorManager.Reconcile NOT called (the skip) ---

	if monMgr.reconcileCalls != 0 {
		t.Errorf("monitor manager.Reconcile called %d time(s), want 0 (FF-on must SKIP the S6 monitor reconcile)", monMgr.reconcileCalls)
	}
}

// TestFSMv2Watcher_ReconcileManager_FFOff_CallsS6MonitorReconcile is the FF-off
// characterization complement to the R11 skip test: with the feature flag OFF,
// ReconcileManager calls benthosMonitorManager.Reconcile (the existing path).
// This pins that the FF-on early-return did not accidentally swallow the FF-off
// branch — the two paths are mutually exclusive on the flag.
func TestFSMv2Watcher_ReconcileManager_FFOff_CallsS6MonitorReconcile(t *testing.T) {
	prev := fsmv2BenthosMonitorEnabled
	fsmv2BenthosMonitorEnabled = false

	t.Cleanup(func() { fsmv2BenthosMonitorEnabled = prev })

	fake := &fakeBenthosMonitorWatcher{}
	monMgr := &fakeBenthosMonitorManager{}
	s := NewDefaultBenthosService("test",
		WithFSMv2BenthosWatcher(fake),
		WithMonitorManager(monMgr),
	)

	s.benthosMonitorConfigs = []config.BenthosMonitorConfig{
		{
			FSMInstanceConfig: config.FSMInstanceConfig{
				Name:            "benthos-a",
				DesiredFSMState: benthos_monitor_fsm.OperationalStateActive,
			},
			MetricsPort: 4195,
		},
	}

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(100*time.Second))
	defer cancel()

	_, _ = s.ReconcileManager(
		ctx,
		serviceregistry.NewMockRegistry(),
		public_fsm.SystemSnapshot{Tick: 1, SnapshotTime: time.Now()},
	)

	// FF-off: benthosMonitorManager.Reconcile IS called.
	if monMgr.reconcileCalls != 1 {
		t.Errorf("monitor manager.Reconcile called %d time(s), want 1 (FF-off must call the S6 monitor reconcile)", monMgr.reconcileCalls)
	}

	// FF-off: the watcher's Upsert is NOT called (the FF-on lifecycle is off).
	if len(fake.upsertCalls) != 0 {
		t.Errorf("upsertCalls = %d, want 0 (FF-off must not use the fsmv2 watcher lifecycle)", len(fake.upsertCalls))
	}
}

// TestFSMv2Watcher_Remove_FFOn_DeletesRef verifies R9: when a config leaves the
// slice during RemoveBenthosFromS6Manager and FF-on is active, the watcher's
// Delete is called with ref{WorkerType:"benthos_monitor", Name:s6ServiceName}.
//
// Forcing assertion: fake.deleteCalls has exactly one entry with the right ref.
// Before the FF-on Delete branch exists, deleteCalls is empty → fails RED.
func TestFSMv2Watcher_Remove_FFOn_DeletesRef(t *testing.T) {
	prev := fsmv2BenthosMonitorEnabled
	fsmv2BenthosMonitorEnabled = true

	t.Cleanup(func() { fsmv2BenthosMonitorEnabled = prev })

	fake := &fakeBenthosMonitorWatcher{}
	monMgr := &fakeBenthosMonitorManager{}
	s := NewDefaultBenthosService("test",
		WithFSMv2BenthosWatcher(fake),
		WithMonitorManager(monMgr),
	)

	// cfg.Name = s6ServiceName = "benthos-" + benthosName. For benthosName "a",
	// the s6ServiceName is "benthos-a".
	s.benthosMonitorConfigs = []config.BenthosMonitorConfig{
		{
			FSMInstanceConfig: config.FSMInstanceConfig{
				Name:            "benthos-a",
				DesiredFSMState: benthos_monitor_fsm.OperationalStateActive,
			},
			MetricsPort: 4195,
		},
	}

	err := s.RemoveBenthosFromS6Manager(context.Background(), nil, "a")
	if err != nil {
		t.Fatalf("err = %v, want nil (Remove is idempotent, no instances to block)", err)
	}

	wantRef := dynamicchildren.Ref{WorkerType: "benthos_monitor", Name: "benthos-a"}

	if len(fake.deleteCalls) != 1 {
		t.Fatalf("deleteCalls = %d, want 1 (the removed config's ref)", len(fake.deleteCalls))
	}

	if fake.deleteCalls[0] != wantRef {
		t.Errorf("deleteCalls[0] = %+v, want %+v", fake.deleteCalls[0], wantRef)
	}
}

// TestFSMv2Watcher_Remove_FFOn_ResetsWindow verifies R10: A1's window reset on
// Remove — after RemoveBenthosFromS6Manager, s.window is a FRESH instance
// (LastTick==0, IsActive==false). A1 already implements this reset; this test
// PINS it so a future refactor cannot silently drop the reset.
//
// Forcing assertion: s.window.LastTick == 0 after the call. A1's reset at
// benthos.go:967 assigns NewBenthosMetricsState(), so this passes GREEN from
// the start — it's a pin, not a RED→GREEN rung.
func TestFSMv2Watcher_Remove_FFOn_ResetsWindow(t *testing.T) {
	prev := fsmv2BenthosMonitorEnabled
	fsmv2BenthosMonitorEnabled = true

	t.Cleanup(func() { fsmv2BenthosMonitorEnabled = prev })

	fake := &fakeBenthosMonitorWatcher{}
	monMgr := &fakeBenthosMonitorManager{}
	s := NewDefaultBenthosService("test",
		WithFSMv2BenthosWatcher(fake),
		WithMonitorManager(monMgr),
	)

	// Pre-seed the window so "fresh" is a meaningful assertion (not just the
	// zero default). Feed it non-zero metrics + a known LastTick.
	s.window.UpdateFromMetrics(benthosmetrics.Metrics{
		Inputs: map[string]benthosmetrics.InputInstance{
			"in1": {Received: 5},
		},
	}, 99)

	if s.window.LastTick != 99 || !s.window.IsActive {
		t.Fatalf("pre-condition: window not seeded (LastTick=%d, IsActive=%v)", s.window.LastTick, s.window.IsActive)
	}

	// Add a config so Remove has something to remove.
	s.benthosMonitorConfigs = []config.BenthosMonitorConfig{
		{
			FSMInstanceConfig: config.FSMInstanceConfig{
				Name:            "benthos-a",
				DesiredFSMState: benthos_monitor_fsm.OperationalStateActive,
			},
			MetricsPort: 4195,
		},
	}

	err := s.RemoveBenthosFromS6Manager(context.Background(), nil, "a")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	// R10: window is a FRESH instance.
	if s.window.LastTick != 0 {
		t.Errorf("window.LastTick = %d, want 0 (window must be reset on Remove)", s.window.LastTick)
	}

	if s.window.IsActive {
		t.Errorf("window.IsActive = true, want false (fresh window must be inactive)")
	}
}

// ffOffObservedState builds a BenthosMonitorObservedState carrying a single
// input scan with the given LastUpdatedAt, received-count, and liveness. The
// monitor FSM is marked Running (the FF-off read path's IsRunning guard at
// benthos.go:752 must pass for the feed site at :762 to be reached). Used by
// the FF-off down→recovery parity test to script the monitor's
// GetLastObservedState return values across the healthy → outage → recovery
// sequence.
func ffOffObservedState(lastUpdatedAt time.Time, count int64, isLive bool) benthos_monitor_fsm.BenthosMonitorObservedState {
	return benthos_monitor_fsm.BenthosMonitorObservedState{
		ServiceInfo: &benthos_monitor.ServiceInfo{
			BenthosStatus: benthos_monitor.BenthosMonitorStatus{
				IsRunning: true,
				LastScan: &benthos_monitor.BenthosMetricsScan{
					LastUpdatedAt: lastUpdatedAt,
					HealthCheck: benthosmetrics.HealthCheck{
						IsLive:  isLive,
						IsReady: true,
					},
					BenthosMetrics: &benthos_monitor.BenthosMetrics{
						Metrics: benthosmetrics.Metrics{
							Inputs: map[string]benthosmetrics.InputInstance{
								"in1": {Received: count},
							},
						},
					},
				},
			},
		},
	}
}

// TestFSMv2Watcher_GetHealthCheckAndMetrics_FFOff_OutageFreezesWindow is the
// FF-off mirror of the R12a capstone. It pins the M1 parity fix: the A1
// window-move relocated the throughput feed from monitor-side
// ProcessMetricsData (fired only on a successful parse) to
// GetHealthCheckAndMetrics (benthos.go:762, every read tick). On a benthos-down
// outage the monitor retains last-good (UpdateObservedStateOfInstance returns
// err without updating ObservedState.ServiceInfo), so LastScan.LastUpdatedAt
// stops advancing. The feed must therefore be gated on scan-freshness — feeding
// only when LastUpdatedAt advances since the last feed — so the window FREEZES
// on outage (MessagesPerTick retains its pre-outage value) instead of re-feeding
// last-good every tick (which would drive MessagesPerTick to 0).
//
// Sequence:
//  1. Healthy tick (count=18, t1): window fed, MetricsState non-nil, IsLive.
//  2. Outage ticks 2-5: monitor RETAINS the same last-good scan (same t1, same
//     count=18, IsRunning=true). The window must FREEZE — MessagesPerTick
//     retains the pre-outage value and LastTick does not advance past tick 1.
//     THIS IS THE RED ASSERTION: on the ungated code the re-feed drives
//     MessagesPerTick to 0 and advances LastTick, so these fail.
//  3. Recovery tick (count=20, t2 advanced): window fed the new scan, no
//     counter-reset wipe (20 > 18 → LastCount advances to 20), IsLive.
//
// Forcing assertion: MessagesPerTick after the outage == the pre-outage value
// (NOT 0) AND s.window.LastTick == the healthy tick. Without the
// LastUpdatedAt gate at benthos.go:762, the outage re-feeds last-good every
// tick → count==LastCount → MessagesPerTick drops to 0 → fails RED.
func TestFSMv2Watcher_GetHealthCheckAndMetrics_FFOff_OutageFreezesWindow(t *testing.T) {
	prev := fsmv2BenthosMonitorEnabled
	fsmv2BenthosMonitorEnabled = false

	t.Cleanup(func() { fsmv2BenthosMonitorEnabled = prev })

	const benthosName = "testbridge"

	t1 := time.Now()
	t2 := t1.Add(time.Second)

	monMgr := &fakeBenthosMonitorManager{
		observedStates: []public_fsm.ObservedState{
			ffOffObservedState(t1, 18, true), // tick 1: healthy
			ffOffObservedState(t1, 18, true), // ticks 2-5: retained last-good
			ffOffObservedState(t1, 18, true),
			ffOffObservedState(t1, 18, true),
			ffOffObservedState(t1, 18, true),
			ffOffObservedState(t2, 20, true), // tick 6: recovery (fresh scan)
		},
	}
	s := NewDefaultBenthosService(benthosName, WithMonitorManager(monMgr))

	// The GetInstance guard runs before the FF-off branch, so the s6 service
	// must be registered for the read path to reach the monitor manager.
	s.s6Manager.AddInstanceForTest(s.GetS6ServiceName(benthosName), nil)

	// --- Step 1: healthy tick (count=18, t1) — window fed ---
	const healthyTick uint64 = 1

	got1, err := s.GetHealthCheckAndMetrics(context.Background(), nil, healthyTick, time.Now(), benthosName, nil)
	if err != nil {
		t.Fatalf("step 1: err = %v, want nil", err)
	}

	if !got1.HealthCheck.IsLive {
		t.Fatalf("step 1: IsLive = false, want true (healthy scan)")
	}

	if got1.BenthosMetrics.MetricsState == nil {
		t.Fatalf("step 1: MetricsState = nil, want the live window (fed on a healthy scan)")
	}

	if s.window.Input.LastCount != 18 {
		t.Fatalf("step 1: window.Input.LastCount = %d, want 18 (window must be fed the healthy scan)", s.window.Input.LastCount)
	}

	preOutageRate := s.window.Input.MessagesPerTick
	if preOutageRate == 0 {
		t.Fatalf("step 1: pre-outage MessagesPerTick = 0, want non-zero (healthy scan with count=18 must seed a non-zero rate)")
	}

	// --- Step 2: outage ticks 2-5 — monitor retains last-good (same t1) ---
	// The feed must NOT re-feed last-good: the window FREEZES (MessagesPerTick
	// retains the pre-outage value, LastTick stays at the healthy tick). On the
	// ungated code the re-feed drives MessagesPerTick to 0 and advances LastTick.
	for outageTick := uint64(2); outageTick <= 5; outageTick++ {
		got, err := s.GetHealthCheckAndMetrics(context.Background(), nil, outageTick, time.Now(), benthosName, nil)
		if err != nil {
			t.Fatalf("step 2 (tick %d): err = %v, want nil (retained last-good is served, not an error)", outageTick, err)
		}

		if !got.HealthCheck.IsLive {
			t.Errorf("step 2 (tick %d): IsLive = false, want true (retained last-good is healthy)", outageTick)
		}
	}

	if s.window.LastTick != healthyTick {
		t.Errorf("step 2: window.LastTick = %d, want %d (the outage must NOT re-feed the window — LastUpdatedAt did not advance)",
			s.window.LastTick, healthyTick)
	}

	if s.window.Input.MessagesPerTick != preOutageRate {
		t.Errorf("step 2: window.Input.MessagesPerTick = %v, want %v (the pre-outage rate must be RETAINED — re-feeding last-good drives it to 0)",
			s.window.Input.MessagesPerTick, preOutageRate)
	}

	if s.window.Input.LastCount != 18 {
		t.Errorf("step 2: window.Input.LastCount = %d, want 18 (LastCount must survive the outage — no counter-reset wipe)", s.window.Input.LastCount)
	}

	// --- Step 3: recovery tick (count=20, t2 advanced) — window fed, no wipe ---
	const recoveryTick uint64 = 6

	got3, err := s.GetHealthCheckAndMetrics(context.Background(), nil, recoveryTick, time.Now(), benthosName, nil)
	if err != nil {
		t.Fatalf("step 3: err = %v, want nil", err)
	}

	if !got3.HealthCheck.IsLive {
		t.Errorf("step 3: IsLive = false, want true (recovery scan is healthy)")
	}

	if s.window.Input.LastCount != 20 {
		t.Errorf("step 3: window.Input.LastCount = %d, want 20 (the fresh recovery scan must feed the window)", s.window.Input.LastCount)
	}

	if s.window.LastTick != recoveryTick {
		t.Errorf("step 3: window.LastTick = %d, want %d (the fresh recovery scan must advance LastTick)", s.window.LastTick, recoveryTick)
	}

	// 20 > 18 → counter-reset branch must NOT fire; the pre-outage window
	// entry survives (the window is not wiped to a single reset entry).
	if len(s.window.Input.Window) < 2 {
		t.Errorf("step 3: len(window.Input.Window) = %d, want >= 2 (count 20 > last 18 must NOT trip the counter-reset wipe)", len(s.window.Input.Window))
	}
}

// TestFSMv2Watcher_GetHealthCheckAndMetrics_FFOff_FreshScanSameCountFeeds is the
// triangulation companion to OutageFreezesWindow. It discriminates the
// LastUpdatedAt-advancement gate (the shipped M1 fix) from a wrong
// count-change gate ("feed only when count != LastCount"): a fresh, healthy
// scan whose count is UNCHANGED (benthos up, producing fresh scans, but no new
// messages arrived) must STILL feed the window — advancing LastTick and
// recording MessagesPerTick=0 for the idle interval (the truthful rate).
//
// The OutageFreezesWindow test cannot tell the two gates apart because its
// outage holds BOTH LastUpdatedAt and count fixed, and its recovery advances
// BOTH. A count-change gate would green OutageFreezesWindow (count unchanged →
// no feed → freeze) while REGRESSING this healthy-idle case (count unchanged →
// no feed → LastTick stalls → throughput window freezes at a stale rate even
// though benthos is healthy and publishing fresh scans).
//
// Forcing assertion: after a fresh scan with LastUpdatedAt advanced (t2) but
// count still 18, s.window.LastTick MUST advance past the prior feed tick AND
// MessagesPerTick MUST be 0 (the truthful idle rate), NOT the retained
// pre-idle rate. A count-change gate would leave LastTick frozen + the rate
// retained → fails RED.
func TestFSMv2Watcher_GetHealthCheckAndMetrics_FFOff_FreshScanSameCountFeeds(t *testing.T) {
	prev := fsmv2BenthosMonitorEnabled
	fsmv2BenthosMonitorEnabled = false

	t.Cleanup(func() { fsmv2BenthosMonitorEnabled = prev })

	const benthosName = "testbridge"

	t1 := time.Now()
	t2 := t1.Add(time.Second) // fresh scan: LastUpdatedAt advanced, count UNCHANGED

	monMgr := &fakeBenthosMonitorManager{
		observedStates: []public_fsm.ObservedState{
			ffOffObservedState(t1, 18, true), // tick 1: healthy, count=18
			ffOffObservedState(t2, 18, true), // tick 2: fresh scan (t2 advanced), count STILL 18 (idle)
		},
	}
	s := NewDefaultBenthosService(benthosName, WithMonitorManager(monMgr))
	s.s6Manager.AddInstanceForTest(s.GetS6ServiceName(benthosName), nil)

	// Tick 1: healthy scan seeds the window (count=18, non-zero rate).
	const tick1 uint64 = 1
	if _, err := s.GetHealthCheckAndMetrics(context.Background(), nil, tick1, time.Now(), benthosName, nil); err != nil {
		t.Fatalf("tick 1: err = %v, want nil", err)
	}

	if s.window.Input.LastCount != 18 {
		t.Fatalf("tick 1: LastCount = %d, want 18 (healthy scan must seed the window)", s.window.Input.LastCount)
	}

	// Tick 2: fresh scan (LastUpdatedAt advanced to t2) with count UNCHANGED.
	// The LastUpdatedAt-advancement gate MUST feed — the scan is fresh even
	// though idle. LastTick advances; MessagesPerTick is 0 (countDiff=0 over
	// the interval — the truthful idle rate). A count-change gate would NOT
	// feed (count unchanged) → LastTick stays at tick1 + the rate stays
	// non-zero → this assertion fails.
	const tick2 uint64 = 2
	if _, err := s.GetHealthCheckAndMetrics(context.Background(), nil, tick2, time.Now(), benthosName, nil); err != nil {
		t.Fatalf("tick 2: err = %v, want nil (fresh idle scan is served, not an error)", err)
	}

	if s.window.LastTick != tick2 {
		t.Errorf("tick 2: window.LastTick = %d, want %d (a fresh scan MUST feed even when count is unchanged — the LastUpdatedAt-advancement gate fires; a count-change gate would leave it frozen at tick1)",
			s.window.LastTick, tick2)
	}

	if s.window.Input.MessagesPerTick != 0 {
		t.Errorf("tick 2: MessagesPerTick = %v, want 0 (fresh idle scan: count unchanged over the interval → the truthful rate is 0; a count-change gate would retain the stale non-zero rate)",
			s.window.Input.MessagesPerTick)
	}

	// LastCount must still be 18 (no counter-reset: 18 is not < 18).
	if s.window.Input.LastCount != 18 {
		t.Errorf("tick 2: LastCount = %d, want 18 (unchanged count must not trip counter-reset)", s.window.Input.LastCount)
	}
}

// TestFSMv2Watcher_GetHealthCheckAndMetrics_FFOff_BackwardsScanFeeds pins the
// ff-off-feed-backwards-parity rung: a FRESH FF-off benthos_monitor scan whose
// LastUpdatedAt went BACKWARDS (e.g. an NTP step correction while benthos is
// running) must STILL feed the BenthosService throughput window — advancing
// s.window.LastTick and updating s.window.Input.LastCount to the scan's count —
// matching pre-A1 behavior, where the feed lived inside ProcessMetricsData and
// fired on every successful scrape regardless of timestamp direction.
//
// The shipped gate at benthos.go:785 (`if lastScan.LastUpdatedAt.After(
// s.lastFedScanAt)`) wrongly skips a backwards timestamp (t0 is not After t1),
// causing a persistent freeze until the clock re-climbs above the prior peak.
// The throughput math (metrics_state.go updateComponentThroughput) takes only
// count + the monotonic read tick, NOT LastUpdatedAt, so feeding a backwards
// timestamp is safe (no corruption). The fix gates on LastUpdatedAt CHANGING
// (!Equal) instead of strictly advancing (After).
//
// Forcing assertions (RED on the shipped After() gate): tick2's backwards-t0
// scan must (a) advance s.window.LastTick to 2 (on After it stays 1 — RED),
// (b) update s.window.Input.LastCount to 20 (on After it stays 18 — RED), and
// (c) leave >= 2 window entries (20 > 18 → no counter-reset wipe). The existing
// OutageFreezesWindow (same LastUpdatedAt → Equal → no feed → freeze) and
// FreshScanSameCountFeeds (forward change → !Equal → feed) regression guards
// MUST stay green: !Equal preserves both (same → Equal → no feed; forward →
// !Equal → feed).
func TestFSMv2Watcher_GetHealthCheckAndMetrics_FFOff_BackwardsScanFeeds(t *testing.T) {
	prev := fsmv2BenthosMonitorEnabled
	fsmv2BenthosMonitorEnabled = false

	t.Cleanup(func() { fsmv2BenthosMonitorEnabled = prev })

	const benthosName = "testbridge"

	t1 := time.Now()
	t0 := t1.Add(-1 * time.Second) // BEFORE t1 — an NTP backward step

	monMgr := &fakeBenthosMonitorManager{
		observedStates: []public_fsm.ObservedState{
			ffOffObservedState(t1, 18, true), // tick 1: healthy scan (feeds + seeds)
			ffOffObservedState(t0, 20, true), // tick 2: FRESH scan, LastUpdatedAt went BACKWARDS, count advanced to 20
		},
	}
	s := NewDefaultBenthosService(benthosName, WithMonitorManager(monMgr))
	s.s6Manager.AddInstanceForTest(s.GetS6ServiceName(benthosName), nil)

	// --- Tick 1: healthy scan (t1, count=18) — feeds + seeds ---
	const tick1 uint64 = 1
	got1, err := s.GetHealthCheckAndMetrics(context.Background(), nil, tick1, time.Now(), benthosName, nil)
	if err != nil {
		t.Fatalf("tick 1: err = %v, want nil", err)
	}

	if !got1.HealthCheck.IsLive {
		t.Fatalf("tick 1: IsLive = false, want true (healthy scan)")
	}

	if s.window.Input.LastCount != 18 {
		t.Fatalf("tick 1: LastCount = %d, want 18 (healthy scan must seed the window)", s.window.Input.LastCount)
	}

	if !s.lastFedScanAt.Equal(t1) {
		t.Fatalf("tick 1: lastFedScanAt = %v, want %v (must be seeded to the scan's LastUpdatedAt)", s.lastFedScanAt, t1)
	}

	// --- Tick 2: FRESH scan with LastUpdatedAt BACKWARDS (t0 < t1), count=20 ---
	// The shipped After() gate skips t0 (t0 is not After t1) → LastTick stays 1
	// and LastCount stays 18 — RED. The !Equal fix feeds (t0 != t1) → advances.
	const tick2 uint64 = 2
	got2, err := s.GetHealthCheckAndMetrics(context.Background(), nil, tick2, time.Now(), benthosName, nil)
	if err != nil {
		t.Fatalf("tick 2: err = %v, want nil (fresh backwards scan is served, not an error)", err)
	}

	if !got2.HealthCheck.IsLive {
		t.Errorf("tick 2: IsLive = false, want true (the backwards scan is healthy)")
	}

	if s.window.LastTick != tick2 {
		t.Errorf("tick 2: window.LastTick = %d, want %d (a fresh backwards-timestamp scan MUST feed — LastUpdatedAt CHANGED; the shipped After() gate wrongly skips it because t0 is not After t1, leaving LastTick frozen at 1)",
			s.window.LastTick, tick2)
	}

	if s.window.Input.LastCount != 20 {
		t.Errorf("tick 2: window.Input.LastCount = %d, want 20 (the backwards scan's count must be fed; the shipped After() gate skips the feed so LastCount stays 18 — RED)",
			s.window.Input.LastCount)
	}

	// 20 > 18 → counter-reset branch must NOT fire; the prior window entry
	// survives (the window is not wiped to a single reset entry).
	if len(s.window.Input.Window) < 2 {
		t.Errorf("tick 2: len(window.Input.Window) = %d, want >= 2 (count 20 > last 18 must NOT trip the counter-reset wipe)", len(s.window.Input.Window))
	}
}
