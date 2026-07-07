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

// Package fsmv2nmap is the minimal proof-of-concept nmap monitor built on the
// fsmv2 simple framework. Behind the fsmv2 flag, it observes a single TCP
// target by dialing it once per tick: a successful dial reports the port open,
// a failed dial drives the worker degraded. Richer scanning (refused->closed,
// timeout->filtered, DNS failure->degraded classification) is deferred to PR2.
package fsmv2nmap

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
)

const (
	// WorkerType is the canonical worker-type name used in config and CSE storage.
	WorkerType = "nmap"

	// pollInterval is the cadence at which the framework calls Poll.
	pollInterval = 1 * time.Second
)

// NmapStatus is the result of one TCP-dial observation of the target port.
type NmapStatus struct {
	// PortState is "open" on a successful dial; empty otherwise (PR2 adds
	// closed/filtered/degraded classification).
	PortState string `json:"port_state"`
	// LatencyMs is the dial round-trip time in milliseconds.
	LatencyMs float64 `json:"latency_ms"`
	// Port is the scanned target port.
	Port uint16 `json:"port"`
	// IsRunning is true when the target port accepted the connection.
	IsRunning bool `json:"is_running"`
}

// Poll dials the configured target once and reports the port state. On a
// successful dial it returns an "open"/running status with the measured
// latency; on a dial error it returns a non-open status carrying the port and
// wraps the error, which the framework persists as a degraded verdict
// ("poll error: ..."). Error->state classification (refused->closed,
// timeout->filtered, DNS->degraded) is deferred to PR2.
func Poll(ctx context.Context, _ struct{}, cfg config.NmapConfig) (NmapStatus, error) {
	target := net.JoinHostPort(cfg.NmapServiceConfig.Target, strconv.Itoa(int(cfg.NmapServiceConfig.Port)))

	start := time.Now()

	var dialer net.Dialer

	conn, err := dialer.DialContext(ctx, "tcp", target)
	if err != nil {
		return NmapStatus{Port: cfg.NmapServiceConfig.Port}, fmt.Errorf("nmap dial %s: %w", target, err)
	}

	elapsedMs := float64(time.Since(start).Microseconds()) / 1000.0
	_ = conn.Close()

	return NmapStatus{
		PortState: "open",
		LatencyMs: elapsedMs,
		Port:      cfg.NmapServiceConfig.Port,
		IsRunning: true,
	}, nil
}

func init() {
	simple.Register(simple.MonitorSpec[config.NmapConfig, NmapStatus, struct{}]{
		WorkerType: WorkerType,
		Interval:   pollInterval,
		Poll:       Poll,
	})
}
