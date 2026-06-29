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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	bmworker "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/benthos_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthosmetrics"
)

// fakeBenthosMonitorWatcher is a test double for benthosMonitorWatcher. It
// returns a canned (status, res, err) and records the ref/maxAge it was called
// with so the test can assert the FF-on read path used s6ServiceName (not raw
// benthosName) for the ref Name.
type fakeBenthosMonitorWatcher struct {
	status    bmworker.BenthosMonitorStatus
	res       fsmv2client.Freshness
	err       error
	gotRef    dynamicchildren.Ref
	gotMaxAge time.Duration
}

func (f *fakeBenthosMonitorWatcher) GetFresh(_ context.Context, ref dynamicchildren.Ref, maxAge time.Duration) (bmworker.BenthosMonitorStatus, fsmv2client.Freshness, error) {
	f.gotRef = ref
	f.gotMaxAge = maxAge

	return f.status, f.res, f.err
}

func (f *fakeBenthosMonitorWatcher) Upsert(_ dynamicchildren.Ref, _ map[string]any) error { return nil }
func (f *fakeBenthosMonitorWatcher) Delete(_ dynamicchildren.Ref)                         {}

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
