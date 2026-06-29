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
// S6 monitor reconcile. GetInstance and GetLastObservedState return
// not-found/nil so the Remove path proceeds to the window reset.
type fakeBenthosMonitorManager struct {
	reconcileCalls int
}

func (f *fakeBenthosMonitorManager) Reconcile(_ context.Context, _ public_fsm.SystemSnapshot, _ serviceregistry.Provider) (error, bool) {
	f.reconcileCalls++

	return nil, false
}

func (f *fakeBenthosMonitorManager) GetInstance(_ string) (public_fsm.FSMInstance, bool) {
	return nil, false
}

func (f *fakeBenthosMonitorManager) GetLastObservedState(_ string) (public_fsm.ObservedState, error) {
	return nil, nil
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
