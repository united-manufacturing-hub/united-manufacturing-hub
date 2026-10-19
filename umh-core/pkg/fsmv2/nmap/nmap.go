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
// Behind the fsmv2 flag, it observes a single TCP target with a probe that dials
// it: a successful dial reports the port open, a failed dial reports it closed.
// A single TCP connect cannot tell a refused port apart from a dropped probe, so
// any non-open outcome reports closed. The probe runs outside the observation,
// so a DNS lookup slower than the observation timeout still completes, and Poll
// reports the last probe's result.
package fsmv2nmap

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	nmapservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/nmap"
)

const (
	// WorkerType is the canonical worker-type name used in config and CSE storage.
	WorkerType = "nmap"

	// pollInterval is the cadence at which the framework calls Poll.
	pollInterval = 1 * time.Second

	// probeTimeout bounds one lookup plus connect. An unresponsive DNS server
	// costs 4 s in Docker's embedded DNS and 5 s by the resolv.conf default
	// before the next server is asked.
	// https://github.com/moby/moby/blob/master/daemon/libnetwork/resolver.go
	// https://man7.org/linux/man-pages/man5/resolv.conf.5.html
	probeTimeout = 10 * time.Second

	// resultMaxAge is how old the last probe result may be before Poll reports
	// it stale. It exceeds probeTimeout plus the time until the next probe starts.
	resultMaxAge = 15 * time.Second
)

// Deps holds one nmap worker's last probe result and the resolver its probes use.
type Deps struct {
	lastCheck *lastCheck
	resolver  *net.Resolver
}

// NewDeps builds the deps for one nmap worker instance.
func NewDeps(_ deps.Identity, _ *deps.BaseDependencies) Deps {
	return Deps{lastCheck: &lastCheck{}, resolver: net.DefaultResolver}
}

type checkResult struct {
	finishedAt time.Time
	status     NmapStatus
}

type lastCheck struct {
	result *checkResult
	mu     sync.Mutex
}

func (c *lastCheck) record(result checkResult) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.result = &result
}

func (c *lastCheck) load() (checkResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.result == nil {
		return checkResult{}, false
	}

	return *c.result, true
}

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

// Probe dials the configured target once, with its own timeout of probeTimeout,
// and records the port state for Poll. A probe cancelled by a stopping worker
// records nothing.
func Probe(ctx context.Context, d Deps, cfg config.NmapConfig) {
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	status := dial(probeCtx, d.resolver, cfg)

	if errors.Is(ctx.Err(), context.Canceled) {
		return
	}

	d.lastCheck.record(checkResult{status: status, finishedAt: time.Now()})
}

// Poll reports the last probe's result. It returns an error while no probe of
// the configured target has finished, and when the last result is older than
// resultMaxAge, so the worker shows degraded until a probe proves the state.
func Poll(_ context.Context, d Deps, cfg config.NmapConfig) (NmapStatus, error) {
	result, recorded := d.lastCheck.load()
	if !recorded || !probedTarget(result.status, cfg) {
		return NmapStatus{
			Target: cfg.NmapServiceConfig.Target,
			Port:   cfg.NmapServiceConfig.Port,
		}, fmt.Errorf("no connection check of %s has finished yet", targetAddress(cfg))
	}

	age := time.Since(result.finishedAt)
	if age > resultMaxAge {
		return result.status, fmt.Errorf("the last connection check of %s finished %s ago", targetAddress(cfg), age.Round(time.Second))
	}

	return result.status, nil
}

func probedTarget(status NmapStatus, cfg config.NmapConfig) bool {
	return status.Target == cfg.NmapServiceConfig.Target && status.Port == cfg.NmapServiceConfig.Port
}

func targetAddress(cfg config.NmapConfig) string {
	return net.JoinHostPort(cfg.NmapServiceConfig.Target, strconv.Itoa(int(cfg.NmapServiceConfig.Port)))
}

func dial(ctx context.Context, resolver *net.Resolver, cfg config.NmapConfig) NmapStatus {
	start := time.Now()

	dialer := net.Dialer{Resolver: resolver}

	conn, err := dialer.DialContext(ctx, "tcp", targetAddress(cfg))
	if err != nil {
		return NmapStatus{
			Target:    cfg.NmapServiceConfig.Target,
			PortState: string(nmapservice.PortStateClosed),
			Port:      cfg.NmapServiceConfig.Port,
			ScannedAt: start,
		}
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
	}
}

func init() {
	simple.Register(simple.MonitorSpec[config.NmapConfig, NmapStatus, Deps]{
		WorkerType: WorkerType,
		Interval:   pollInterval,
		NewDeps:    NewDeps,
		Probe:      Probe,
		Poll:       Poll,
	})
}
