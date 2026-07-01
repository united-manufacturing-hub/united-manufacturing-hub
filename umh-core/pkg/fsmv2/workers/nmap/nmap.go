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

// Package nmap_worker provides an FSMv2 worker that periodically performs TCP
// connect scans against a target:port and reports the result
// (open/closed/filtered/degraded).
//
// NAMING CONVENTION: The package name uses underscore (nmap_worker) but the
// folder name is "nmap". The type prefix must be "Nmap" (one capital) to
// derive correctly as worker type "nmap".
package nmap_worker

import (
	"context"
	"errors"
	"net"
	"strconv"
	"time"

	fsmv2config "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
)

const (
	defaultScanInterval = 1 * time.Second
	scanTimeout         = 3 * time.Second
)

// NmapConfig holds the user-provided configuration for the nmap worker.
type NmapConfig struct {
	fsmv2config.BaseUserSpec `yaml:",inline"`

	// Target is the hostname or IP address to scan.
	Target string `json:"target" yaml:"target"`
	// Port is the TCP port to scan.
	Port uint16 `json:"port" yaml:"port"`
}

// NmapStatus holds the runtime observation data for the nmap worker.
type NmapStatus struct {
	// ScanError contains the last scan error message, if any.
	ScanError string `json:"scanError,omitempty"`
	// PortState is the last observed port state: "open", "closed", "filtered", or "degraded".
	PortState string `json:"portState,omitempty"`
	// Target is the scan target (copied from config for observability).
	Target string `json:"target,omitempty"`
	// LatencyMs is the last measured TCP connect latency in milliseconds.
	LatencyMs float64 `json:"latencyMs"`
	// Port is the scanned port (copied from config for observability).
	Port uint16 `json:"port,omitempty"`
	// IsRunning indicates whether the port is currently open.
	IsRunning bool `json:"isRunning"`
}

// scan performs a TCP connect scan against cfg.Target:cfg.Port and returns the
// port state. Network failures are represented as "closed" or "filtered" states,
// never as errors, so the caller only sees an error on context cancellation.
func scan(ctx context.Context, cfg NmapConfig) (NmapStatus, error) {
	if cfg.Target == "" || cfg.Port == 0 {
		return NmapStatus{Target: cfg.Target, Port: cfg.Port, PortState: "degraded"}, nil
	}

	start := time.Now()
	addr := net.JoinHostPort(cfg.Target, strconv.Itoa(int(cfg.Port)))

	dialCtx, cancel := context.WithTimeout(ctx, scanTimeout)
	defer cancel()

	conn, dialErr := (&net.Dialer{}).DialContext(dialCtx, "tcp", addr)
	latencyMs := float64(time.Since(start).Milliseconds())

	if dialErr != nil {
		if ctx.Err() != nil {
			return NmapStatus{}, ctx.Err()
		}

		portState := "closed"

		var netErr net.Error
		if errors.As(dialErr, &netErr) && netErr.Timeout() {
			portState = "filtered"
		}

		return NmapStatus{
			Target:    cfg.Target,
			Port:      cfg.Port,
			PortState: portState,
			LatencyMs: latencyMs,
			ScanError: dialErr.Error(),
		}, nil
	}

	_ = conn.Close()

	return NmapStatus{
		Target:    cfg.Target,
		Port:      cfg.Port,
		PortState: "open",
		LatencyMs: latencyMs,
		IsRunning: true,
	}, nil
}

func init() {
	simple.Register("nmap", defaultScanInterval, scan)
}
