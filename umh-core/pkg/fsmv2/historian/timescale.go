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
	fsmv2config "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
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
	// name. It is TimescaleAuthValid on a successful query, TimescaleAuthInvalid
	// when the server rejected the credentials or database name, and
	// TimescaleAuthUnknown on any other failure, including a server error that
	// is not an auth fault.
	Auth models.TimescaleAuthState `json:"auth"`
	// LatencyMs is the query round-trip time in milliseconds.
	LatencyMs float64 `json:"latency_ms"`
	// Port is the observed timescale port.
	Port uint16 `json:"port"`
	// Reachable is true when the server answered: the query succeeded or the
	// server returned an error. It is false when nothing answered (a network or
	// timeout fault) and when the connection pool could not be built.
	Reachable bool `json:"reachable"`
	// Summary and DatabaseOccupiedDiskBytes are read on a slower schedule than the
	// connection check. Each is nil until a read succeeds, and nil again after a
	// failed connection check.
	Summary                   *timescalemetrics.Summary `json:"summary,omitempty"`
	DatabaseOccupiedDiskBytes *int64                    `json:"databaseOccupiedDiskBytes,omitempty"`
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

// DatabaseKey holds a database Poll queries instead of the shared pool.
var DatabaseKey = fsmv2config.NewDependencyKey[timescalemetrics.Database]("historian-timescale.database")

// Deps carries what Poll needs: this instance's BaseDependencies, whose logger
// Poll writes to, and what it queries. That is the database stored under
// DatabaseKey, or the shared pool when none is stored.
type Deps struct {
	*deps.BaseDependencies

	pool         *poolHolder
	database     timescalemetrics.Database // nil in production
	summary      *readCache[timescalemetrics.Summary]
	databaseSize *readCache[int64]
}

// newDeps builds one worker instance's poll dependencies. It keeps the
// BaseDependencies the framework built for this instance, whose logger already
// names the worker, and hands out sharedPool rather than building a holder,
// because the framework never releases what a worker holds. The identity is
// unused: nothing else here varies per instance.
func newDeps(_ deps.Identity, bd *deps.BaseDependencies, m map[string]any) Deps {
	database, _ := fsmv2config.LookupDependency(m, DatabaseKey)

	return Deps{
		BaseDependencies: bd,
		pool:             sharedPool,
		database:         database,
		summary:          sharedSummary,
		databaseSize:     sharedDatabaseSize,
	}
}

func (d Deps) databaseFor(dsn string) (timescalemetrics.Database, error) {
	if d.database != nil {
		return d.database, nil
	}

	pool, err := d.pool.get(dsn)
	if err != nil {
		return nil, err
	}

	return pool, nil
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

func unexpectedQueryFailure(err error) bool {
	return serverAnswered(err) && !authRejected(err)
}

type failureTag string

const (
	tagPoolFailed             failureTag = "historian::pool_failed"
	tagQueryFailed            failureTag = "historian::query_failed"
	tagSummaryReadFailed      failureTag = "historian::summary_read_failed"
	tagDatabaseSizeReadFailed failureTag = "historian::database_size_read_failed"
)

func (d Deps) report(ctx context.Context, tag failureTag, host string, err error) {
	if ctx.Err() != nil {
		return
	}

	d.GetLogger().SentryWarn(deps.FeatureSupportHistorian, d.GetHierarchyPath(), string(tag),
		deps.String("host", host),
		deps.Err(err))
}

// Poll runs a `SELECT 1` once and logs the outcome. It queries the database
// stored under DatabaseKey, or the shared pool when none is stored. A
// successful query returns a reachable, auth-valid status with the measured
// latency. A failed query is an observation, returned with a nil error for
// health to judge: an authentication or unknown-database error sets
// Auth=TimescaleAuthInvalid, and a network or timeout error leaves
// Reachable=false and Auth=TimescaleAuthUnknown. Poll returns an error only when
// it could not observe: the pool could not be built, or the worker is shutting
// down.
func Poll(ctx context.Context, d Deps, cfg config.HistorianConfig) (TimescaleStatus, error) {
	cfg = cfg.WithDefaults()
	host, port := cfg.Timescale.Host, cfg.Timescale.Port

	dsn := cfg.Timescale.ToDSN()

	db, err := d.databaseFor(dsn)
	if err != nil {
		d.GetLogger().Debug("timescale connection check",
			deps.String("host", host),
			deps.Bool("reachable", false),
			deps.Err(err))
		d.report(ctx, tagPoolFailed, host, err)
		d.forgetDatabaseFigures()

		return TimescaleStatus{
			Host: host,
			Port: port,
			Auth: models.TimescaleAuthUnknown,
		}, fmt.Errorf("timescale pool: %w", err)
	}

	start := time.Now()

	var one int
	if err := db.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return TimescaleStatus{
				Host: host,
				Port: port,
				Auth: models.TimescaleAuthUnknown,
			}, fmt.Errorf("timescale query %s cancelled: %w", host, ctx.Err())
		}

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

		if unexpectedQueryFailure(err) {
			d.report(ctx, tagQueryFailed, host, err)
		}

		d.forgetDatabaseFigures()

		return TimescaleStatus{
			Host:      host,
			Port:      port,
			Reachable: reachable,
			Auth:      auth,
		}, nil
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
		Summary:                   d.summary.refresh(now, dsn, summaryReader(ctx, d, db, host)),
		DatabaseOccupiedDiskBytes: d.databaseSize.refresh(now, dsn, databaseSizeReader(ctx, d, db, host)),
	}, nil
}

// forgetDatabaseFigures drops the cached figures after a failed connection
// check, so the first successful check after it reads them again instead of
// sending figures from before the fault.
func (d Deps) forgetDatabaseFigures() {
	d.summary.clear()
	d.databaseSize.clear()
}

// errReadNotStarted means the poll's deadline passed before the read could query the database.
var errReadNotStarted = errors.New("the poll's deadline passed before the read started")

// readNotPossible returns why a read cannot start, or nil when it can.
func readNotPossible(ctx context.Context) error {
	if errors.Is(ctx.Err(), context.Canceled) {
		return ctx.Err()
	}

	if ctx.Err() != nil {
		return errReadNotStarted
	}

	return nil
}

// the error reaches the cache only, so an unreadable summary does not mark the worker degraded
func summaryReader(ctx context.Context, d Deps, db timescalemetrics.Database, host string) func() (timescalemetrics.Summary, error) {
	return func() (timescalemetrics.Summary, error) {
		if err := readNotPossible(ctx); err != nil {
			return timescalemetrics.Summary{}, err
		}

		summary, err := timescalemetrics.CollectSummary(ctx, db)
		if err != nil && errors.Is(ctx.Err(), context.Canceled) {
			return timescalemetrics.Summary{}, ctx.Err()
		}

		if err != nil {
			d.GetLogger().Debug("timescale summary",
				deps.String("host", host),
				deps.Err(err))
			d.report(ctx, tagSummaryReadFailed, host, err)

			return timescalemetrics.Summary{}, err
		}

		return summary, nil
	}
}

func databaseSizeReader(ctx context.Context, d Deps, db timescalemetrics.Database, host string) func() (int64, error) {
	return func() (int64, error) {
		if err := readNotPossible(ctx); err != nil {
			return 0, err
		}

		databaseOccupiedDiskBytes, err := timescalemetrics.ReadDatabaseOccupiedDiskBytes(ctx, db)
		if err != nil && errors.Is(ctx.Err(), context.Canceled) {
			return 0, ctx.Err()
		}

		if err != nil {
			d.GetLogger().Debug("timescale database size",
				deps.String("host", host),
				deps.Err(err))
			d.report(ctx, tagDatabaseSizeReadFailed, host, err)

			return 0, err
		}

		return databaseOccupiedDiskBytes, nil
	}
}

func health(_ config.HistorianConfig, status TimescaleStatus) simple.Health {
	if status.Auth == models.TimescaleAuthValid {
		return simple.Healthy("connected to TimescaleDB")
	}

	if status.Auth == models.TimescaleAuthInvalid {
		return simple.Degraded("TimescaleDB rejected the credentials or the database name")
	}

	if !status.Reachable {
		return simple.Degraded("TimescaleDB is unreachable")
	}

	return simple.Degraded("TimescaleDB answered, but the connection check failed")
}

// pollAndRecord records nothing for a cancelled poll, because it observed nothing.
func pollAndRecord(ctx context.Context, d Deps, cfg config.HistorianConfig) (TimescaleStatus, error) {
	status, err := Poll(ctx, d, cfg)
	if errors.Is(ctx.Err(), context.Canceled) {
		return status, err
	}

	recordStatus(d.MetricsRecorder(), status)

	return status, err
}

func init() {
	simple.Register(simple.MonitorSpec[config.HistorianConfig, TimescaleStatus, Deps]{
		WorkerType: WorkerType,
		Interval:   pollInterval,
		Poll:       pollAndRecord,
		Health:     health,
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
	value  *T
	// A config edit repoints the pool at another database; holding the value across
	// that would report one database's figures beside the other's host.
	dsn      string
	interval time.Duration
	mu       sync.Mutex
}

// refresh drops the value on a failed read; a cancelled read or one that never started leaves the cache for the next poll.
func (c *readCache[T]) refresh(
	now time.Time,
	dsn string,
	read func() (T, error),
) *T {
	c.mu.Lock()
	defer c.mu.Unlock()

	if dsn != c.dsn {
		c.dsn = dsn
		c.value = nil
		c.readAt = time.Time{}
	} else if !c.readAt.IsZero() && now.Sub(c.readAt) < c.interval {
		return c.value
	}

	value, err := read()
	if errors.Is(err, context.Canceled) {
		return c.value
	}

	if errors.Is(err, errReadNotStarted) {
		return nil
	}

	c.readAt = now

	if err != nil {
		c.value = nil

		return nil
	}

	c.value = &value

	return c.value
}

func (c *readCache[T]) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.readAt = time.Time{}
	c.value = nil
}
