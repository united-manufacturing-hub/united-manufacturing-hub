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
	fsmv2config "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	nmapservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/nmap"
)

const (
	// WorkerType is the canonical worker-type name used in config and CSE storage.
	WorkerType = "nmap"

	// pollInterval is the cadence at which the framework calls Poll.
	pollInterval = 1 * time.Second
)

// NmapStatus is the result of one TCP-dial observation of the target port.
type NmapStatus struct {
	// Target is the hostname or IP address the scan dialed.
	Target string `json:"target"`
	// PortState is one of nmapservice.PortStateOpen or nmapservice.PortStateClosed.
	PortState string `json:"port_state"`
	// LatencyMs is the dial round-trip time in milliseconds. Zero unless the
	// port is open.
	LatencyMs float64 `json:"latency_ms"`
	// Port is the scanned target port.
	Port uint16 `json:"port"`
	// IsRunning is true when the target port accepted the connection.
	IsRunning bool `json:"is_running"`
	// ScannedAt is when the dial started. A consumer comparing it against the
	// time a config edit was persisted can tell a scan of the new target from a
	// leftover scan of the previous one, which no other field distinguishes.
	ScannedAt time.Time `json:"scanned_at"`
}

// Dialer is what Poll dials the target through.
type Dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// DialerKey holds a Dialer that Poll uses instead of a *net.Dialer.
var DialerKey = fsmv2config.NewDependencyKey[Dialer]("nmap.dialer")

// Deps is the per-instance value Poll receives.
type Deps struct {
	dialer Dialer
}

func newDeps(_ deps.Identity, _ *deps.BaseDependencies, m map[string]any) Deps {
	// No timeout: the collector bounds every Poll with its ObservationTimeout,
	// which cancels the dial context.
	var dialer Dialer = &net.Dialer{}

	if injected, ok := fsmv2config.LookupDependency(m, DialerKey); ok {
		dialer = injected
	}

	return Deps{
		dialer: dialer,
	}
}

// Poll dials the configured target once and reports the port state. A
// failed dial reports the port closed with a nil error, because a closed port
// is a scan result, not a poll failure. A cancelled context (worker shutdown)
// is the exception: Poll returns an error and no port state.
func Poll(ctx context.Context, d Deps, cfg config.NmapConfig) (NmapStatus, error) {
	target := net.JoinHostPort(cfg.NmapServiceConfig.Target, strconv.Itoa(int(cfg.NmapServiceConfig.Port)))

	start := time.Now()

	conn, err := d.dialer.DialContext(ctx, "tcp", target)
	if err != nil {
		// Worker shutdown cancels the context; surface it as an error so the
		// framework does not misreport a shutdown as a port state. This
		// deliberately masks the port result: a dial that fails at the same tick
		// as shutdown reports cancelled, not closed. A deadline
		// (ObservationTimeout) is not a shutdown: it falls through to closed.
		if errors.Is(ctx.Err(), context.Canceled) {
			return NmapStatus{
				Target:    cfg.NmapServiceConfig.Target,
				Port:      cfg.NmapServiceConfig.Port,
				ScannedAt: start,
			}, fmt.Errorf("scan cancelled: %w", ctx.Err())
		}

		return NmapStatus{
			Target:    cfg.NmapServiceConfig.Target,
			PortState: string(nmapservice.PortStateClosed),
			Port:      cfg.NmapServiceConfig.Port,
			ScannedAt: start,
		}, nil
	}

	elapsedMs := float64(time.Since(start).Microseconds()) / 1000.0
	_ = conn.Close()

	return NmapStatus{
		Target:    cfg.NmapServiceConfig.Target,
		PortState: string(nmapservice.PortStateOpen),
		LatencyMs: elapsedMs,
		Port:      cfg.NmapServiceConfig.Port,
		IsRunning: true,
		ScannedAt: start,
	}, nil
}

func init() {
	simple.Register(simple.MonitorSpec[config.NmapConfig, NmapStatus, Deps]{
		WorkerType: WorkerType,
		Interval:   pollInterval,
		NewDeps:    newDeps,
		Poll:       Poll,
	})
}
