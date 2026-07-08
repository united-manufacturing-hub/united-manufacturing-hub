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

// Package fsmv2timescale is a standalone monitor built on the fsmv2 simple
// framework. It watches the timescale's TimescaleDB/Postgres endpoint by
// dialing host:port once per tick: a successful dial reports the endpoint
// reachable, a failed dial drives the worker degraded. Each check is logged at
// INFO level. Unlike the nmap worker, it is not patched into an fsmv1 worker —
// it runs purely on the fsmv2 runtime.
package fsmv2timescale

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"go.uber.org/zap"
)

const (
	// WorkerType is the canonical worker-type name used in config and CSE storage.
	WorkerType = "historian-timescale"

	// InstanceName is the fixed dynamic-child name for the single timescale
	// monitor configured per instance. Shared so writers (the config watcher)
	// and readers (the status generator) address the same child without
	// re-typing the literal.
	InstanceName = "timescale"

	// pollInterval is the cadence at which the framework calls Poll.
	pollInterval = 1 * time.Second
)

// Ref identifies the timescale monitor child in the fsmv2 runtime. It is the
// single source of truth for the child's (WorkerType, Name) pair, shared by the
// config watcher that upserts it and the status generator that reads it.
var Ref = dynamicchildren.Ref{WorkerType: WorkerType, Name: InstanceName}

// TimescaleStatus is the result of one TCP-dial observation of the timescale
// endpoint.
type TimescaleStatus struct {
	// Host is the dialed timescale host.
	Host string `json:"host"`
	// LatencyMs is the dial round-trip time in milliseconds.
	LatencyMs float64 `json:"latency_ms"`
	// Port is the dialed timescale port.
	Port uint16 `json:"port"`
	// Reachable is true when the endpoint accepted the connection.
	Reachable bool `json:"reachable"`
}

// Deps carries the poll dependencies. The logger is shared across ticks and
// instances; it is stateless, so it satisfies the MonitorSpec.Deps contract.
type Deps struct {
	Logger deps.FSMLogger
}

// Poll dials the configured timescale endpoint once and logs the outcome at
// INFO level. On a successful dial it returns a reachable status with the
// measured latency; on a dial error it returns an unreachable status carrying
// host/port and wraps the error, which the framework persists as a degraded
// verdict ("poll error: ...").
func Poll(ctx context.Context, d Deps, cfg config.HistorianConfig) (TimescaleStatus, error) {
	cfg = cfg.WithDefaults()
	target := net.JoinHostPort(cfg.Timescale.Host, strconv.Itoa(int(cfg.Timescale.Port)))

	start := time.Now()

	var dialer net.Dialer

	conn, err := dialer.DialContext(ctx, "tcp", target)
	if err != nil {
		zap.S().Errorw("timescale bad", "target", target, "error", err)
		d.Logger.Info("timescale connection check",
			deps.String("target", target),
			deps.Bool("reachable", false),
			deps.Err(err))

		return TimescaleStatus{Host: cfg.Timescale.Host, Port: cfg.Timescale.Port}, fmt.Errorf("timescale dial %s: %w", target, err)
	}

	elapsedMs := float64(time.Since(start).Microseconds()) / 1000.0
	_ = conn.Close()

	zap.S().Errorw("timescale good", "target", target, "error", err)
	d.Logger.Info("timescale connection check",
		deps.String("target", target),
		deps.Bool("reachable", true),
		deps.Float64("latency_ms", elapsedMs))

	return TimescaleStatus{
		Host:      cfg.Timescale.Host,
		LatencyMs: elapsedMs,
		Port:      cfg.Timescale.Port,
		Reachable: true,
	}, nil
}

func init() {
	simple.Register(simple.MonitorSpec[config.HistorianConfig, TimescaleStatus, Deps]{
		WorkerType: WorkerType,
		Interval:   pollInterval,
		Poll:       Poll,
		Deps:       Deps{Logger: deps.NewFSMLogger(logger.For(WorkerType))},
	})
}
