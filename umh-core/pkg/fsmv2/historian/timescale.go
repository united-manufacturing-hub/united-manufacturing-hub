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
// configuration faults (Auth=TimescaleAuthInvalid) rather than transient network
// faults, which leave authentication unverified (Auth=TimescaleAuthUnknown).
//
// # Scope: connection health, plus a summary
//
// Per tick this worker checks the connection and nothing else: one `SELECT 1`
// over a pooled connection for reachability, latency, and whether the
// credentials and database name are accepted. On a slower schedule it also reads
// a summary of the database: versions, disk usage, table names, the span of the
// stored rows, and the job counts.
//
// Per-table detail and the job list are read on request by the
// get-historian-metrics action.
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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/historian/timescalemetrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
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
	// Auth reports whether the endpoint accepted the credentials and database
	// name. It is TimescaleAuthUnknown when nothing answered (a network or timeout
	// fault), TimescaleAuthInvalid when the server answered but rejected the
	// config, and TimescaleAuthValid on a successful query.
	Auth models.TimescaleAuthState `json:"auth"`
	// LatencyMs is the query round-trip time in milliseconds.
	LatencyMs float64 `json:"latency_ms"`
	// Port is the observed timescale port.
	Port uint16 `json:"port"`
	// Reachable is true when the endpoint answered, whether the query succeeded
	// or the server rejected the credentials/database (an auth fault). It is
	// false only for network or timeout faults, where nothing answered.
	Reachable bool `json:"reachable"`
	// Read on a slower schedule than the connection check, keeping its last value
	// between reads, so it is empty only until the first one completes.
	timescalemetrics.Summary
	DatabaseOccupiedDiskBytes int64 `json:"databaseOccupiedDiskBytes"`
}

// sharedPool is the one holder every worker instance polls through. The
// framework never closes a pool a worker owns: fsmv2.GracefulShutdowner is
// declared but never invoked, so a worker is never called at a point where it
// could close one (ENG-5375). One holder bounds the process to a single pool,
// because get closes the previous one when the DSN changes; a per-instance
// holder would orphan a pgxpool health-check goroutine on every despawn that
// followed a poll, with nothing able to close it. A child despawned before its
// first poll orphans nothing, because only Poll reaches poolHolder.get.
//
// Historian is a singleton today, one Ref under one writer. Every instance polls
// through this holder and it caches a single pool, so two instances on different
// DSNs would rebuild each other's pool on every poll; going multi-instance needs
// a holder per instance, and that needs a teardown path first (ENG-5375).
//
// Removing the historian config block despawns the child without closing the
// pool, so the health-check goroutine runs for the life of the process and up to
// maxConns server sessions stay open until connMaxLifetime recycles them. A
// respawn with an identical DSN then gets the cached pool.
var sharedPool = &poolHolder{}

// Deps carries what Poll needs: this instance's BaseDependencies, whose logger
// Poll writes to, and the holder it queries through.
type Deps struct {
	*deps.BaseDependencies

	pool         *poolHolder
	summary      *readCache[timescalemetrics.Summary]
	databaseSize *readCache[int64]
}

// newDeps builds one worker instance's poll dependencies. It keeps the
// BaseDependencies the framework built for this instance, whose logger already
// names the worker, and hands out sharedPool rather than building a holder,
// because the framework never releases what a worker holds. The identity is
// unused: nothing else here varies per instance.
func newDeps(_ deps.Identity, bd *deps.BaseDependencies) Deps {
	return Deps{
		BaseDependencies: bd,
		pool:             sharedPool,
		summary:          sharedSummary,
		databaseSize:     sharedDatabaseSize,
	}
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
// authentication happens on first acquire (in Poll). The DSN is parsed only when
// a pool is built, so TLS material at the sslrootcert and sslcert paths is
// re-read only on a DSN change (ENG-5593).
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

	// pgBouncerCodeInvalidCatalogName is the SQLSTATE PgBouncer returns (08P01,
	// protocol violation) when the requested database does not exist in its pool.
	// The bare code is also used for unrelated protocol violations, so a match
	// requires pgBouncerMissingDBPrefix in the message as well.
	pgBouncerCodeInvalidCatalogName = "08P01"

	// pgBouncerMissingDBPrefix is the message prefix PgBouncer pairs with 08P01
	// when the requested database is not in its pool. Requiring it prevents
	// unrelated 08P01 protocol violations from being misread as a missing database.
	pgBouncerMissingDBPrefix = "no such database"
)

// serverAnswered reports whether err carries a server-side PgError, meaning the
// endpoint is reachable, rather than a network or timeout fault where nothing
// answered.
func serverAnswered(err error) bool {
	var pgErr *pgconn.PgError

	return errors.As(err, &pgErr)
}

// authRejected reports whether err is the server rejecting the supplied
// credentials or database name. It is a narrower condition than serverAnswered:
// a server-side error outside the supported auth (class 28), missing-database
// (3D000), and PgBouncer missing-database (08P01 with pgBouncerMissingDBPrefix)
// classes leaves the endpoint reachable but the credentials unproven.
func authRejected(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	badCredentials := strings.HasPrefix(pgErr.Code, pgClassInvalidAuthorization)
	pgBouncerMissingDB := pgErr.Code == pgBouncerCodeInvalidCatalogName &&
		strings.Contains(strings.ToLower(pgErr.Message), pgBouncerMissingDBPrefix)
	unknownDatabase := pgErr.Code == pgCodeInvalidCatalogName || pgBouncerMissingDB

	return badCredentials || unknownDatabase
}

// Poll runs a `SELECT 1` over the shared pool once and logs the outcome. A
// successful query returns a reachable, auth-valid status with the measured
// latency. On error it returns an unreachable status and wraps the error, which
// the framework persists as a degraded verdict; an authentication or
// unknown-database error additionally sets Auth=TimescaleAuthInvalid to flag a
// configuration fault, while a network or timeout error leaves
// Auth=TimescaleAuthUnknown.
func Poll(ctx context.Context, d Deps, cfg config.HistorianConfig) (TimescaleStatus, error) {
	cfg = cfg.WithDefaults()
	host, port := cfg.Timescale.Host, cfg.Timescale.Port

	dsn := cfg.Timescale.ToDSN()

	pool, err := d.pool.get(dsn)
	if err != nil {
		d.GetLogger().Debug("timescale connection check",
			deps.String("host", host),
			deps.Bool("reachable", false),
			deps.Err(err))

		return TimescaleStatus{
			Host:                      host,
			Port:                      port,
			Auth:                      models.TimescaleAuthUnknown,
			Summary:                   d.summary.last(dsn),
			DatabaseOccupiedDiskBytes: d.databaseSize.last(dsn),
		}, fmt.Errorf("timescale pool: %w", err)
	}

	start := time.Now()

	var one int
	if err := pool.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
		// Any server-side PgError means the endpoint is reachable. Auth is proven
		// invalid only when the server rejected the credentials or database name;
		// other server errors leave the config unverified. A non-PgError (network,
		// timeout) means nothing answered, so both stay false/unknown.
		reachable := serverAnswered(err)
		auth := models.TimescaleAuthUnknown
		if authRejected(err) {
			auth = models.TimescaleAuthInvalid
		}
		d.GetLogger().Debug("timescale connection check",
			deps.String("host", host),
			deps.Bool("reachable", reachable),
			deps.String("auth", string(auth)),
			deps.Err(err))

		return TimescaleStatus{
			Host:                      host,
			Port:                      port,
			Reachable:                 reachable,
			Auth:                      auth,
			Summary:                   d.summary.last(dsn),
			DatabaseOccupiedDiskBytes: d.databaseSize.last(dsn),
		}, fmt.Errorf("timescale query %s: %w", host, err)
	}

	elapsedMs := float64(time.Since(start).Microseconds()) / 1000.0
	d.GetLogger().Debug("timescale connection check",
		deps.String("host", host),
		deps.Bool("reachable", true),
		deps.String("auth", string(models.TimescaleAuthValid)),
		deps.Float64("latency_ms", elapsedMs))

	now := time.Now()

	return TimescaleStatus{
		Host:                      host,
		Auth:                      models.TimescaleAuthValid,
		LatencyMs:                 elapsedMs,
		Port:                      port,
		Reachable:                 true,
		Summary:                   d.summary.refresh(now, dsn, summaryReader(ctx, d, pool, host)),
		DatabaseOccupiedDiskBytes: d.databaseSize.refresh(now, dsn, databaseSizeReader(ctx, d, pool, host)),
	}, nil
}

// The error is logged and discarded: a summary that cannot be read is not a
// connection fault, and returning it would drive the worker degraded for a
// database that is answering.
func summaryReader(ctx context.Context, d Deps, pool *pgxpool.Pool, host string) func() (timescalemetrics.Summary, bool) {
	return func() (timescalemetrics.Summary, bool) {
		summary, err := timescalemetrics.CollectSummary(ctx, pool)
		if err != nil {
			d.GetLogger().Debug("timescale summary",
				deps.String("host", host),
				deps.Err(err))

			return timescalemetrics.Summary{}, false
		}

		return summary, true
	}
}

func databaseSizeReader(ctx context.Context, d Deps, pool *pgxpool.Pool, host string) func() (int64, bool) {
	return func() (int64, bool) {
		databaseOccupiedDiskBytes, err := timescalemetrics.ReadDatabaseOccupiedDiskBytes(ctx, pool)
		if err != nil {
			d.GetLogger().Debug("timescale database size",
				deps.String("host", host),
				deps.Err(err))

			return 0, false
		}

		return databaseOccupiedDiskBytes, true
	}
}

func init() {
	simple.Register(simple.MonitorSpec[config.HistorianConfig, TimescaleStatus, Deps]{
		WorkerType: WorkerType,
		Interval:   pollInterval,
		Poll:       Poll,
		NewDeps:    newDeps,
	})
}

// None of the figures moves faster than this: a table appears when a contract is
// deployed, and disk usage and a failing job are not urgent to the second.
const summaryInterval = 60 * time.Second

// pg_database_size stats every file of the database, and calling it on a short
// interval is a reported cause of timeouts and inode cache pressure.
// https://www.postgresql.org/message-id/CAGRY4nz94%2Bq_zVxj%2Bdnk7zqm-McBz4mSza_wALKiw2%3D%3D23MiGQ%40mail.gmail.com
const databaseSizeInterval = 15 * time.Minute

// The caches match sharedPool: they have to outlive a single Poll.
var (
	sharedSummary      = &readCache[timescalemetrics.Summary]{interval: summaryInterval}
	sharedDatabaseSize = &readCache[int64]{interval: databaseSizeInterval}
)

type readCache[T any] struct {
	readAt time.Time
	value  T
	// A config edit repoints the pool at another database; holding the value across
	// that would report one database's figures beside the other's host.
	dsn      string
	interval time.Duration
	mu       sync.Mutex
}

// refresh reads again once the interval has elapsed or the database changed. A
// failed read keeps the previous value and waits its turn rather than retrying
// every poll.
func (c *readCache[T]) refresh(
	now time.Time,
	dsn string,
	read func() (T, bool),
) T {
	c.mu.Lock()
	defer c.mu.Unlock()

	if dsn != c.dsn {
		var empty T

		c.dsn = dsn
		c.value = empty
	} else if !c.readAt.IsZero() && now.Sub(c.readAt) < c.interval {
		return c.value
	}

	c.readAt = now

	if value, ok := read(); ok {
		c.value = value
	}

	return c.value
}

// The framework persists a failed Poll's status too, so it carries the last value.
func (c *readCache[T]) last(dsn string) T {
	c.mu.Lock()
	defer c.mu.Unlock()

	if dsn != c.dsn {
		var empty T

		return empty
	}

	return c.value
}
