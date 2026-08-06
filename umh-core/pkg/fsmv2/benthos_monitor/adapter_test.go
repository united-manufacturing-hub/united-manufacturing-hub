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
