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

// Package fsmv2nmap is the nmap monitor built on the fsmv2 simple framework.
// Behind the fsmv2 flag, it observes a single TCP target by dialing it once per
// tick: a successful dial reports the port open, a failed dial reports it
// closed. A single TCP connect cannot tell a refused port apart from a dropped
// probe, so any non-open outcome reports closed.
package fsmv2nmap

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	nmapfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/nmap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
)

const (
	// WorkerType is the canonical worker-type name used in config and CSE storage.
	WorkerType = "nmap"

	// pollInterval is the cadence at which the framework calls Poll.
	pollInterval = 1 * time.Second

	// The dialer sets no timeout: the collector already bounds every Poll with
	// its ObservationTimeout, which cancels the dial context.
)

// NmapStatus is the result of one TCP-dial observation of the target port.
type NmapStatus struct {
	// PortState is one of nmapfsm.PortStateOpen or nmapfsm.PortStateClosed.
	PortState string `json:"port_state"`
	// LatencyMs is the dial round-trip time in milliseconds. Zero unless the
	// port is open.
	LatencyMs float64 `json:"latency_ms"`
	// Port is the scanned target port.
	Port uint16 `json:"port"`
	// IsRunning is true when the target port accepted the connection.
	IsRunning bool `json:"is_running"`
}

// Poll dials the configured target once and reports the port state. A
// successful dial yields an open/running status with the measured latency; any
// dial failure yields a closed port. A closed port is a legitimate scan
// outcome, not a poll failure, so Poll returns it with a nil error. Poll
// returns an error only when the context is cancelled (worker shutdown), so the
// framework does not misreport a shutdown as a port state.
func Poll(ctx context.Context, _ struct{}, cfg config.NmapConfig) (NmapStatus, error) {
	target := net.JoinHostPort(cfg.NmapServiceConfig.Target, strconv.Itoa(int(cfg.NmapServiceConfig.Port)))

	start := time.Now()

	dialer := net.Dialer{}

	conn, err := dialer.DialContext(ctx, "tcp", target)
	if err != nil {
		// Worker shutdown cancels the context; surface it as an error so the
		// framework does not misreport a shutdown as a port state. This
		// deliberately masks the port result: a dial that fails at the same tick
		// as shutdown reports cancelled, not closed. A deadline
		// (ObservationTimeout) is not a shutdown: it falls through to closed.
		if errors.Is(ctx.Err(), context.Canceled) {
			return NmapStatus{Port: cfg.NmapServiceConfig.Port}, fmt.Errorf("scan cancelled: %w", ctx.Err())
		}

		return NmapStatus{
			PortState: string(nmapfsm.PortStateClosed),
			Port:      cfg.NmapServiceConfig.Port,
		}, nil
	}

	elapsedMs := float64(time.Since(start).Microseconds()) / 1000.0
	_ = conn.Close()

	return NmapStatus{
		PortState: string(nmapfsm.PortStateOpen),
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
