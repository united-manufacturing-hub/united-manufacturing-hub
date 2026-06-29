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
