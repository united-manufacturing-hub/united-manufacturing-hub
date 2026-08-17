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

package fsmv2benthosmonitor

import (
	"testing"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	benthosmonitorfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
)

func testConfig() config.BenthosMonitorConfig {
	return config.BenthosMonitorConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{Name: "benthos-1", DesiredFSMState: "active"},
		MetricsPort:       4195,
	}
}

// TestMapObservedBuildsFullNestedStructure pins the control-loop-panic trap: the
// adapter dereferences ServiceInfo.BenthosStatus.LastScan.BenthosMetrics.MetricsState
// unguarded, so mapObserved must return every inner pointer non-nil. It also pins
// the single conversion: the worker's tick-free MessagesPerSecond becomes FSMv1's
// MessagesPerTick via MessagesPerSecond x DefaultTickerTime, and IsActive is copied
// through unchanged (never scaled).
func TestMapObservedBuildsFullNestedStructure(t *testing.T) {
	status := simple.Status[BenthosMonitorStatus]{
		Result: BenthosMonitorStatus{
			ScrapedAt: time.Now().Truncate(time.Second),
			Input:     ComponentThroughput{MessagesPerSecond: 20, LastCount: 5},
			Output:    ComponentThroughput{MessagesPerSecond: 10, LastCount: 3},
			IsActive:  true,
			PingAlive: true,
			Ready:     true,
			Version:   "1.2.3",
		},
	}

	observed := mapObserved(testConfig(), status)
	bmState, ok := observed.(benthosmonitorfsm.BenthosMonitorObservedState)
	if !ok {
		t.Fatalf("mapObserved returned %T, want benthosmonitorfsm.BenthosMonitorObservedState", observed)
	}

	if bmState.ServiceInfo == nil {
		t.Fatal("ServiceInfo is nil; benthos.go dereferences it")
	}
	if bmState.ServiceInfo.BenthosStatus.LastScan == nil {
		t.Fatal("LastScan is nil; benthos.go dereferences it")
	}
	if bmState.ServiceInfo.BenthosStatus.LastScan.BenthosMetrics == nil {
		t.Fatal("LastScan.BenthosMetrics is nil; benthos.go dereferences it")
	}
	ms := bmState.ServiceInfo.BenthosStatus.LastScan.BenthosMetrics.MetricsState
	if ms == nil {
		t.Fatal("MetricsState is nil; HasProcessingActivity dereferences it")
	}

	// MessagesPerTick = MessagesPerSecond x DefaultTickerTime (0.1): 20 -> 2, 10 -> 1.
	wantIn := 20 * tickSeconds()
	if ms.Input.MessagesPerTick != wantIn {
		t.Errorf("Input.MessagesPerTick = %v, want %v (20 msg/s x 0.1s tick)", ms.Input.MessagesPerTick, wantIn)
	}
	wantOut := 10 * tickSeconds()
	if ms.Output.MessagesPerTick != wantOut {
		t.Errorf("Output.MessagesPerTick = %v, want %v (10 msg/s x 0.1s tick)", ms.Output.MessagesPerTick, wantOut)
	}
	if ms.Input.LastCount != int64(status.Result.Input.LastCount) {
		t.Errorf("Input.LastCount = %d, want %d (never left zero; it gates every MC panel)", ms.Input.LastCount, status.Result.Input.LastCount)
	}
	if ms.Output.LastCount != int64(status.Result.Output.LastCount) {
		t.Errorf("Output.LastCount = %d, want %d", ms.Output.LastCount, status.Result.Output.LastCount)
	}
	if ms.IsActive != status.Result.IsActive {
		t.Errorf("MetricsState.IsActive = %v, want %v (copied through, never scaled)", ms.IsActive, status.Result.IsActive)
	}
	if ms.Output.MessagesPerTick == ms.Input.MessagesPerTick {
		t.Errorf("input and output ticks must differ (20 vs 10 msg/s)")
	}

	// HealthCheck mirrors the scrape's liveness/readiness/version.
	hc := bmState.ServiceInfo.BenthosStatus.LastScan.HealthCheck
	if !hc.IsLive || !hc.IsReady || hc.Version != "1.2.3" {
		t.Errorf("HealthCheck = %+v, want IsLive+IsReady=true and Version 1.2.3", hc)
	}
}

// TestMapObservedSetsIsRunning pins the field that gates every consumer.
// GetHealthCheckAndMetrics (pkg/service/benthos/benthos.go) copies HealthCheck out
// of LastScan and then returns a ZERO BenthosStatus when
// ServiceInfo.BenthosStatus.IsRunning is false — throwing away the health it just
// read. So a scan with correct IsLive/IsReady but IsRunning=false reads as
// live=false, ready=false at the FSM, holding every bridge in starting forever.
// That is what happened in a live container: the worker polled correctly and every
// DataFlowComponent still reported "healthchecks did not pass".
//
// FSMv1 set IsRunning from the S6 FSM state of the monitor service
// (service/benthos_monitor/benthos_monitor.go:1486). Under this flag there is no S6
// monitor service — the worker is the monitor — so a scan carrying a real timestamp
// is the evidence that it ran.
func TestMapObservedSetsIsRunning(t *testing.T) {
	// A real scrape: ScrapedAt set, both probes true.
	live := mapObserved(testConfig(), simple.Status[BenthosMonitorStatus]{
		Result: BenthosMonitorStatus{
			ScrapedAt: time.Now(),
			PingAlive: true,
			Ready:     true,
		},
	})
	bm, ok := live.(benthosmonitorfsm.BenthosMonitorObservedState)
	if !ok {
		t.Fatalf("mapObserved returned %T", live)
	}
	if !bm.ServiceInfo.BenthosStatus.IsRunning {
		t.Error("IsRunning is false after a real scrape: GetHealthCheckAndMetrics will discard the HealthCheck and every bridge stays in starting")
	}

	// A zero status (the adapter passes one on a non-Fresh read) has no scrape
	// behind it, so it must NOT claim the monitor is running.
	empty := mapObserved(testConfig(), simple.Status[BenthosMonitorStatus]{})
	bmEmpty, ok := empty.(benthosmonitorfsm.BenthosMonitorObservedState)
	if !ok {
		t.Fatalf("mapObserved returned %T", empty)
	}
	if bmEmpty.ServiceInfo.BenthosStatus.IsRunning {
		t.Error("IsRunning is true for a zero status: an unobserved worker must not report itself running")
	}

	// A FAILED poll. This is the case the timestamp alone cannot express, and the
	// one that made IsRunning permanently true: Poll stamps ScrapedAt before its
	// first request and returns that partial status on every error path, and
	// simple.CollectObservedState deliberately persists it (with Degraded set and
	// the error in Reason) so partial detail survives. So a dead monitor arrives
	// here with a FRESH ScrapedAt and must still map to not-running, otherwise
	// GetHealthCheckAndMetrics copies a dead monitor's HealthCheck out as if live
	// and its ErrBenthosMonitorNotRunning gate can never fire.
	failed := mapObserved(testConfig(), simple.Status[BenthosMonitorStatus]{
		Result: BenthosMonitorStatus{
			ScrapedAt: time.Now(), // fresh, exactly as a failed poll leaves it
			PingAlive: true,       // /ping answered before /metrics failed
		},
		Degraded: true,
		Reason:   "poll error: scrape /metrics: connection refused",
	})
	bmFailed, ok := failed.(benthosmonitorfsm.BenthosMonitorObservedState)
	if !ok {
		t.Fatalf("mapObserved returned %T", failed)
	}
	if bmFailed.ServiceInfo.BenthosStatus.IsRunning {
		t.Error("IsRunning is true for a failed poll: a dead monitor reports itself running and the not-running gate in GetHealthCheckAndMetrics can never fire")
	}

	// A stale-but-successful scan. health() drives this degraded, and under this
	// flag there is no separate process that could be "running but unhealthy", so
	// it maps to not-running too. Asserted so the collapse is a decision on the
	// record rather than an accident.
	stale := mapObserved(testConfig(), simple.Status[BenthosMonitorStatus]{
		Result:   BenthosMonitorStatus{ScrapedAt: time.Now()},
		Degraded: true,
		Reason:   "benthos monitor scan is stale",
	})
	bmStale, ok := stale.(benthosmonitorfsm.BenthosMonitorObservedState)
	if !ok {
		t.Fatalf("mapObserved returned %T", stale)
	}
	if bmStale.ServiceInfo.BenthosStatus.IsRunning {
		t.Error("IsRunning is true for a stale scan: consumers would treat untrustworthy data as live")
	}
}

// TestMapObservedToleratesZeroStatus pins the other control-loop hazard: on a
// non-Fresh read the adapter passes a zero status to mapObserved, which must
// still produce the full non-nil nested structure (empty content) rather than
// panicking on nil pointers.
func TestMapObservedToleratesZeroStatus(t *testing.T) {
	observed := mapObserved(testConfig(), simple.Status[BenthosMonitorStatus]{})
	bmState, ok := observed.(benthosmonitorfsm.BenthosMonitorObservedState)
	if !ok {
		t.Fatalf("mapObserved returned %T, want BenthosMonitorObservedState", observed)
	}
	if bmState.ServiceInfo == nil || bmState.ServiceInfo.BenthosStatus.LastScan == nil ||
		bmState.ServiceInfo.BenthosStatus.LastScan.BenthosMetrics == nil ||
		bmState.ServiceInfo.BenthosStatus.LastScan.BenthosMetrics.MetricsState == nil {
		t.Fatal("zero-status mapObserved produced a nil inner pointer (would panic in the control loop)")
	}
}

// TestHealthReproducesIsMonitorHealthy pins D9: healthy iff the scan is fresh
// (within BenthosMaxMetricsAndConfigAge). A stale scan is degraded.
func TestHealthReproducesIsMonitorHealthy(t *testing.T) {
	if h := health(testConfig(), BenthosMonitorStatus{ScrapedAt: time.Now()}); h.Degraded {
		t.Errorf("fresh scan reported degraded: %+v", h)
	}

	// Past the 10s age -> degraded (never healthy).
	if h := health(testConfig(), BenthosMonitorStatus{ScrapedAt: time.Now().Add(-(constants.BenthosMaxMetricsAndConfigAge + time.Second))}); !h.Degraded {
		t.Errorf("stale scan reported healthy: %+v", h)
	}

	// Zero ScrapedAt (never scanned) -> degraded.
	if h := health(testConfig(), BenthosMonitorStatus{}); !h.Degraded {
		t.Errorf("empty ScrapedAt reported healthy: %+v", h)
	}
}
