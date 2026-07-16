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
// framework. Once per tick it runs a lightweight query over a shared connection
// pool against the TimescaleDB/Postgres endpoint: a successful query reports the
// endpoint reachable and credentials valid, a failure drives the worker
// degraded. Authentication and missing-database errors are classified as
// configuration faults (AuthValid=false) rather than transient network faults.
package fsmv2timescale

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
)

const (
	// WorkerType is the canonical worker-type name used in config and CSE storage.
	WorkerType = "historian-timescale"

	// InstanceName is the fixed dynamic-child name for the single per-instance
	// timescale monitor.
	InstanceName = "timescale"

	// pollInterval is the cadence at which the framework calls Poll.
	pollInterval = 1 * time.Second
)

// Ref is the (WorkerType, Name) pair identifying the timescale monitor child,
// shared by the config watcher that upserts it and the status generator that reads it.
var Ref = dynamicchildren.Ref{WorkerType: WorkerType, Name: InstanceName}

const (
	// maxConns caps the pool. The monitor needs a single connection per tick;
	// the small headroom lets a recycled connection be established while the old
	// one drains.
	maxConns = 2

	// connMaxLifetime forces connections to be recycled periodically. A fresh
	// connection re-runs the authentication handshake, so a password rotated on
	// the server (with config unchanged) is caught within this window rather than
	// masked forever by a long-lived authenticated connection.
	connMaxLifetime = 5 * time.Minute
)

// TimescaleStatus is the result of one query observation of the timescale
// endpoint.
type TimescaleStatus struct {
	// Host is the observed timescale host.
	Host string `json:"host"`
	// LatencyMs is the query round-trip time in milliseconds.
	LatencyMs float64 `json:"latency_ms"`
	// Port is the observed timescale port.
	Port uint16 `json:"port"`
	// Reachable is true when the endpoint answered, whether the query succeeded
	// or the server rejected the credentials/database (an auth fault). It is
	// false only for network or timeout faults, where nothing answered.
	Reachable bool `json:"reachable"`
	// AuthValid is false when the endpoint answered but rejected the supplied
	// credentials or database name (a configuration fault, not a network fault).
	// It is true on a successful query.
	AuthValid bool `json:"auth_valid"`
}

// Deps carries the poll dependencies, shared across ticks. The pool holder
// lazily builds and caches a pgx pool keyed by DSN, so Poll reuses pooled
// connections instead of dialing every tick.
type Deps struct {
	Logger deps.FSMLogger
	pool   *poolHolder
}

// poolHolder caches a single pgx pool, rebuilding it when the DSN changes (for
// example after a historian config edit). It is safe for concurrent use.
type poolHolder struct {
	pool *pgxpool.Pool
	dsn  string
	mu   sync.Mutex
}

// get returns a pool for dsn, creating it on first use and rebuilding it if the
// DSN changed since the last call. Pool creation does not open a connection;
// authentication happens on first acquire (in Poll).
func (h *poolHolder) get(dsn string) (*pgxpool.Pool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.pool != nil && h.dsn == dsn {
		return h.pool, nil
	}

	if h.pool != nil {
		h.pool.Close()
		h.pool = nil
	}

	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse timescale dsn: %w", err)
	}

	poolCfg.MaxConns = maxConns
	poolCfg.MaxConnLifetime = connMaxLifetime

	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create timescale pool: %w", err)
	}

	h.dsn = dsn
	h.pool = pool

	return pool, nil
}

const (
	// pgClassInvalidAuthorization is Postgres SQLSTATE class 28: the server
	// rejected the supplied credentials (bad password, invalid authorization).
	pgClassInvalidAuthorization = "28"

	// pgCodeInvalidCatalogName is Postgres SQLSTATE 3D000: the server rejected
	// the connection because the requested database does not exist.
	pgCodeInvalidCatalogName = "3D000"
)

// serverRejected reports whether err is the server rejecting the supplied
// credentials or database name, rather than a network or timeout fault where
// nothing answered. A rejection means the endpoint is reachable but the config
// is wrong.
func serverRejected(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	badCredentials := strings.HasPrefix(pgErr.Code, pgClassInvalidAuthorization)
	unknownDatabase := pgErr.Code == pgCodeInvalidCatalogName

	return badCredentials || unknownDatabase
}

// Poll runs a `SELECT 1` over the shared pool once and logs the outcome. A
// successful query returns a reachable, auth-valid status with the measured
// latency. On error it returns an unreachable status and wraps the error, which
// the framework persists as a degraded verdict; an authentication or
// unknown-database error additionally sets AuthValid=false to flag a
// configuration fault rather than a transient network problem.
func Poll(ctx context.Context, d Deps, cfg config.HistorianConfig) (TimescaleStatus, error) {
	cfg = cfg.WithDefaults()
	host, port := cfg.Timescale.Host, cfg.Timescale.Port

	pool, err := d.pool.get(cfg.Timescale.ToDSN())
	if err != nil {
		d.Logger.Info("timescale connection check",
			deps.String("host", host),
			deps.Bool("reachable", false),
			deps.Err(err))

		return TimescaleStatus{Host: host, Port: port}, fmt.Errorf("timescale pool: %w", err)
	}

	start := time.Now()

	var one int
	if err := pool.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
		// An auth fault means the server answered but rejected the credentials or
		// database name: the endpoint is reachable, only the config is wrong.
		reachable := serverRejected(err)
		d.Logger.Info("timescale connection check",
			deps.String("host", host),
			deps.Bool("reachable", reachable),
			deps.Bool("auth_valid", false),
			deps.Err(err))

		return TimescaleStatus{Host: host, Port: port, Reachable: reachable, AuthValid: false}, fmt.Errorf("timescale query %s: %w", host, err)
	}

	elapsedMs := float64(time.Since(start).Microseconds()) / 1000.0
	d.Logger.Info("timescale connection check",
		deps.String("host", host),
		deps.Bool("reachable", true),
		deps.Bool("auth_valid", true),
		deps.Float64("latency_ms", elapsedMs))

	return TimescaleStatus{
		Host:      host,
		LatencyMs: elapsedMs,
		Port:      port,
		Reachable: true,
		AuthValid: true,
	}, nil
}

func init() {
	simple.Register(simple.MonitorSpec[config.HistorianConfig, TimescaleStatus, Deps]{
		WorkerType: WorkerType,
		Interval:   pollInterval,
		Poll:       Poll,
		Deps: Deps{
			Logger: deps.NewFSMLogger(logger.For(WorkerType)),
			pool:   &poolHolder{},
		},
	})
}
