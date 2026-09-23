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

package timescalemetrics

import (
	"context"
	"fmt"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
)

const (
	timescaleImage = "timescale/timescaledb:2.24.0-pg17"
	postgresImage  = "postgres:17-alpine"
)

// historianSchemaDDL mirrors the shape benthos-umh's historian output creates: two
// hypertables per contract, 168h chunks, compression enabled and applied.
const historianSchemaDDL = `
CREATE SCHEMA IF NOT EXISTS umh;
CREATE TABLE umh.value_bench (
  topic_id BIGINT NOT NULL, ts TIMESTAMPTZ NOT NULL, value_num DOUBLE PRECISION,
  PRIMARY KEY (topic_id, ts));
SELECT create_hypertable('umh.value_bench', 'ts', chunk_time_interval => INTERVAL '168h');
CREATE TABLE umh.attribute_bench (
  topic_id BIGINT NOT NULL, ts TIMESTAMPTZ NOT NULL, attribute JSONB NOT NULL,
  PRIMARY KEY (topic_id, ts));
SELECT create_hypertable('umh.attribute_bench', 'ts', chunk_time_interval => INTERVAL '168h');
INSERT INTO umh.value_bench SELECT g, now() - (g * INTERVAL '24h'), random()
  FROM generate_series(1, 200) g;
INSERT INTO umh.attribute_bench SELECT g, now() - (g * INTERVAL '24h'), '{"unit":"degC"}'::jsonb
  FROM generate_series(1, 200) g;
ALTER TABLE umh.value_bench SET (timescaledb.compress);
ALTER TABLE umh.attribute_bench SET (timescaledb.compress);
SELECT add_compression_policy('umh.value_bench', INTERVAL '168h');
SELECT add_compression_policy('umh.attribute_bench', INTERVAL '168h');
SELECT compress_chunk(c) FROM show_chunks('umh.value_bench', older_than => INTERVAL '168h') c;
`

// retentionSchemaDDL adds a retention policy to the value hypertable only, so a
// spec can tell a historian that expires data from one that grows forever.
const retentionSchemaDDL = `
SELECT add_retention_policy('umh.value_bench', INTERVAL '720h');
`

// driftedPolicyDDL gives the two hypertables different compression intervals, the
// drift an operator creates by hand that a single reported interval would hide.
const driftedPolicyDDL = `
SELECT remove_compression_policy('umh.attribute_bench');
SELECT add_compression_policy('umh.attribute_bench', INTERVAL '336h');
`

// startDatabase runs image as a throwaway Postgres and returns a pool pointed at it
// plus the config a worker would dial it with. The container and pool are torn down
// when the spec finishes.
func startDatabase(image string) *pgxpool.Pool {
	_, pool := startDatabaseWithConfig(image)

	return pool
}

func startDatabaseWithConfig(image string) (config.HistorianConfig, *pgxpool.Pool) {
	ctx := context.Background()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		Started: true,
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        image,
			ExposedPorts: []string{"5432/tcp"},
			Env: map[string]string{
				"POSTGRES_USER":     "umh_owner",
				"POSTGRES_PASSWORD": "secret",
				"POSTGRES_DB":       "umh",
			},
			WaitingFor: wait.ForListeningPort("5432/tcp").WithStartupTimeout(2 * time.Minute),
		},
	})
	Expect(err).NotTo(HaveOccurred(), "the database container starts")
	if container == nil {
		Fail("testcontainers.GenericContainer returned a nil container")
	}

	DeferCleanup(func() { _ = container.Terminate(context.Background()) })

	var dialled config.HistorianConfig

	host, err := container.Host(ctx)
	Expect(err).NotTo(HaveOccurred())

	port, err := container.MappedPort(ctx, "5432/tcp")
	Expect(err).NotTo(HaveOccurred())

	dsn := fmt.Sprintf("postgres://umh_owner:secret@%s:%s/umh?sslmode=disable", host, port.Port())

	var pool *pgxpool.Pool

	Eventually(func() error {
		p, err := pgxpool.New(ctx, dsn)
		if err != nil {
			return err
		}

		if err := p.Ping(ctx); err != nil {
			p.Close()

			return err
		}

		pool = p

		return nil
	}, 2*time.Minute, time.Second).Should(Succeed(), "the database accepts connections")

	DeferCleanup(pool.Close)

	portNumber, err := strconv.ParseUint(port.Port(), 10, 16)
	Expect(err).NotTo(HaveOccurred(), "the mapped port is numeric")

	dialled = config.HistorianConfig{Timescale: config.TimescaleConfig{
		Host:     host,
		Port:     uint16(portNumber),
		Database: "umh",
		Username: "umh_owner",
		Password: "secret",
		SSLMode:  config.HistorianSSLModeDisable,
	}}

	return dialled, pool
}

var _ = Describe("Metrics collection", Label("integration"), func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("reports versions, table counts and compression against a historian schema", func() {
		pool := startDatabase(timescaleImage)
		_, err := pool.Exec(ctx, historianSchemaDDL)
		Expect(err).NotTo(HaveOccurred(), "the historian schema is created")

		metrics, err := Collect(ctx, pool)

		Expect(err).NotTo(HaveOccurred())
		Expect(metrics.PostgresVersion).To(HavePrefix("17."))
		Expect(metrics.TimescaleVersion).To(Equal("2.24.0"))
		Expect(metrics.DatabaseBytes).To(BeNumerically(">", 0))
	})

	It("counts no failed jobs when every umh job is succeeding", func() {
		pool := startDatabase(timescaleImage)
		_, err := pool.Exec(ctx, historianSchemaDDL)
		Expect(err).NotTo(HaveOccurred())

		jobs, err := readJobs(ctx, pool)

		Expect(err).NotTo(HaveOccurred())
		Expect(jobs).To(HaveLen(2), "one compression policy per hypertable")
	})

	It("reads the last write per table", func() {
		pool := startDatabase(timescaleImage)
		_, err := pool.Exec(ctx, historianSchemaDDL)
		Expect(err).NotTo(HaveOccurred())

		tables, err := readTables(ctx, pool)
		Expect(err).NotTo(HaveOccurred())

		readable, err := readTimeColumnTables(ctx, pool)
		Expect(err).NotTo(HaveOccurred())

		writes, err := collectTimestamps(ctx, pool, tables,
			latestRowTimestampQuery, "latest row timestamps", readable)

		Expect(err).NotTo(HaveOccurred())
		Expect(writes).To(HaveKey("value_bench"))
		Expect(writes["value_bench"]).To(BeNumerically(">", 0), "the fixture wrote rows inside the freshness window")
	})

	It("skips regular tables, which have no time column", func() {
		pool := startDatabase(timescaleImage)
		_, err := pool.Exec(ctx, historianSchemaDDL)
		Expect(err).NotTo(HaveOccurred())

		_, err = pool.Exec(ctx, `CREATE TABLE umh.tag (id bigint PRIMARY KEY, name text)`)
		Expect(err).NotTo(HaveOccurred())

		tables, err := readTables(ctx, pool)
		Expect(err).NotTo(HaveOccurred())

		readable, err := readTimeColumnTables(ctx, pool)
		Expect(err).NotTo(HaveOccurred())

		writes, err := collectTimestamps(ctx, pool, tables,
			latestRowTimestampQuery, "latest row timestamps", readable)

		Expect(err).NotTo(HaveOccurred(), "a regular table must not fail the whole read")
		Expect(writes).To(HaveKey("value_bench"))
		Expect(writes).NotTo(HaveKey("tag"))
	})

	It("refuses to build the freshness query from a name it cannot vouch for", func() {
		pool := startDatabase(timescaleImage)
		_, err := pool.Exec(ctx, historianSchemaDDL)
		Expect(err).NotTo(HaveOccurred())

		readable, err := readTimeColumnTables(ctx, pool)
		Expect(err).NotTo(HaveOccurred())

		writes, err := collectTimestamps(ctx, pool, []Table{
			{Name: `value"; DROP TABLE umh.value_bench; --`},
		}, latestRowTimestampQuery, "latest row timestamps", readable)

		Expect(err).NotTo(HaveOccurred())
		Expect(writes).To(BeEmpty())

		var survives int
		Expect(pool.QueryRow(ctx, `SELECT count(*) FROM umh.value_bench`).Scan(&survives)).To(Succeed())
		Expect(survives).To(BeNumerically(">", 0), "the table is untouched")
	})

	It("lists regular tables alongside the hypertables", func() {
		pool := startDatabase(timescaleImage)
		_, err := pool.Exec(ctx, historianSchemaDDL)
		Expect(err).NotTo(HaveOccurred())

		_, err = pool.Exec(ctx, `CREATE TABLE umh.tag (id bigint PRIMARY KEY, name text)`)
		Expect(err).NotTo(HaveOccurred())

		tables, err := readTables(ctx, pool)
		Expect(err).NotTo(HaveOccurred())

		byName := map[string]Table{}
		for _, table := range tables {
			byName[table.Name] = table
		}

		Expect(byName).To(HaveKey("tag"))
		Expect(byName["tag"].IsHypertable).To(BeFalse())
		Expect(byName["tag"].DiskBytes).To(BeNumerically(">", 0))
		Expect(byName["value_bench"].IsHypertable).To(BeTrue())
	})

	It("leaves the migration bookkeeping table out of the listing", func() {
		pool := startDatabase(timescaleImage)
		_, err := pool.Exec(ctx, historianSchemaDDL)
		Expect(err).NotTo(HaveOccurred())

		_, err = pool.Exec(ctx, `CREATE TABLE umh.schema_migrations (version bigint PRIMARY KEY)`)
		Expect(err).NotTo(HaveOccurred())

		_, err = pool.Exec(ctx, `CREATE TABLE umh.tag (id bigint PRIMARY KEY)`)
		Expect(err).NotTo(HaveOccurred())

		tables, err := readTables(ctx, pool)
		Expect(err).NotTo(HaveOccurred())

		names := []string{}
		for _, table := range tables {
			names = append(names, table.Name)
		}

		Expect(names).NotTo(ContainElement("schema_migrations"))
		Expect(names).To(ContainElement("tag"), "other plain tables are still listed")
	})

	It("survives a hypertable whose time column is not ts", func() {
		pool := startDatabase(timescaleImage)
		_, err := pool.Exec(ctx, historianSchemaDDL)
		Expect(err).NotTo(HaveOccurred())

		_, err = pool.Exec(ctx, `CREATE TABLE umh.custom_metric (device_id bigint, event_time timestamptz NOT NULL, v double precision)`)
		Expect(err).NotTo(HaveOccurred())
		_, err = pool.Exec(ctx, `SELECT create_hypertable('umh.custom_metric', 'event_time')`)
		Expect(err).NotTo(HaveOccurred())

		tables, err := readTables(ctx, pool)
		Expect(err).NotTo(HaveOccurred())

		readable, err := readTimeColumnTables(ctx, pool)
		Expect(err).NotTo(HaveOccurred())

		writes, err := collectTimestamps(ctx, pool, tables,
			latestRowTimestampQuery, "latest row timestamps", readable)

		Expect(err).NotTo(HaveOccurred(), "one unreadable table must not fail every table's freshness")
		Expect(writes).To(HaveKey("value_bench"))
		Expect(writes).NotTo(HaveKey("custom_metric"))
	})

	It("lists every job, including the ones that are succeeding", func() {
		pool := startDatabase(timescaleImage)
		_, err := pool.Exec(ctx, historianSchemaDDL)
		Expect(err).NotTo(HaveOccurred())

		jobs, err := readJobs(ctx, pool)

		Expect(err).NotTo(HaveOccurred())
		Expect(jobs).To(HaveLen(2), "a healthy job is still worth reporting")
		for _, job := range jobs {
			Expect(job.Table).To(BeElementOf("value_bench", "attribute_bench"))
			Expect(job.ScheduleSeconds).To(BeNumerically(">", 0))
			Expect(job.LastRunFailed).To(BeFalse())
		}
	})

	It("lists a job that has failed", func() {
		pool := startDatabase(timescaleImage)
		_, err := pool.Exec(ctx, historianSchemaDDL)
		Expect(err).NotTo(HaveOccurred())

		var jobID int
		Expect(pool.QueryRow(ctx,
			`SELECT job_id FROM timescaledb_information.jobs WHERE hypertable_schema = 'umh' ORDER BY job_id LIMIT 1`,
		).Scan(&jobID)).To(Succeed())

		// Every umh job stops before the stat row below is written. A background
		// run landing after it would record its own outcome over the one this spec
		// is asserting on.
		_, err = pool.Exec(ctx, `SELECT alter_job(job_id, scheduled => false, next_start => 'infinity')
			  FROM timescaledb_information.jobs WHERE hypertable_schema = 'umh'`)
		Expect(err).NotTo(HaveOccurred())

		_, err = pool.Exec(ctx, `INSERT INTO _timescaledb_internal.bgw_job_stat
			(job_id, last_start, last_finish, next_start, last_successful_finish, last_run_success,
			 total_runs, total_duration, total_duration_failures, total_successes, total_failures,
			 total_crashes, consecutive_failures, consecutive_crashes, flags)
			VALUES ($1, now(), now(), now() + interval '1 hour', '-infinity', false,
			 3, interval '0', interval '0', 0, 3, 0, 3, 0, 0)
			ON CONFLICT (job_id) DO UPDATE SET last_run_success = false, total_failures = 3`, jobID)
		Expect(err).NotTo(HaveOccurred())

		jobs, err := readJobs(ctx, pool)

		Expect(err).NotTo(HaveOccurred())
		Expect(jobs).To(HaveLen(2), "the healthy job is listed alongside the failing one")
		Expect(jobs[0].LastRunFailed).To(BeTrue(), "the failing job sorts first")
		Expect(jobs[0].Kind).To(Equal("compression"))
		Expect(jobs[0].Table).To(BeElementOf("value_bench", "attribute_bench"))
		Expect(jobs[0].ScheduleSeconds).To(BeNumerically(">", 0))
		Expect(jobs[1].LastRunFailed).To(BeFalse())
	})

	It("clears a job that failed before but whose last run succeeded", func() {
		pool := startDatabase(timescaleImage)
		_, err := pool.Exec(ctx, historianSchemaDDL)
		Expect(err).NotTo(HaveOccurred())

		var jobID int
		Expect(pool.QueryRow(ctx,
			`SELECT job_id FROM timescaledb_information.jobs WHERE hypertable_schema = 'umh' ORDER BY job_id LIMIT 1`,
		).Scan(&jobID)).To(Succeed())

		// Every umh job stops before the stat row below is written. A background
		// run landing after it would record its own outcome over the one this spec
		// is asserting on.
		_, err = pool.Exec(ctx, `SELECT alter_job(job_id, scheduled => false, next_start => 'infinity')
			  FROM timescaledb_information.jobs WHERE hypertable_schema = 'umh'`)
		Expect(err).NotTo(HaveOccurred())

		_, err = pool.Exec(ctx, `INSERT INTO _timescaledb_internal.bgw_job_stat
			(job_id, last_start, last_finish, next_start, last_successful_finish, last_run_success,
			 total_runs, total_duration, total_duration_failures, total_successes, total_failures,
			 total_crashes, consecutive_failures, consecutive_crashes, flags)
			VALUES ($1, now(), now(), now() + interval '1 hour', now(), true,
			 4, interval '0', interval '0', 1, 3, 0, 0, 0, 0)
			ON CONFLICT (job_id) DO UPDATE SET last_run_success = true, total_failures = 3`, jobID)
		Expect(err).NotTo(HaveOccurred())

		jobs, err := readJobs(ctx, pool)

		Expect(err).NotTo(HaveOccurred())
		for _, job := range jobs {
			Expect(job.LastRunFailed).To(BeFalse(),
				"three failures in this job's history are not a reason to flag it while it is succeeding")
		}
	})

	It("ignores the built-in telemetry job, which fails on an air-gapped host", func() {
		pool := startDatabase(timescaleImage)
		_, err := pool.Exec(ctx, historianSchemaDDL)
		Expect(err).NotTo(HaveOccurred())

		var telemetryFailures int
		Expect(pool.QueryRow(ctx,
			`SELECT count(*) FROM timescaledb_information.jobs WHERE proc_name = 'policy_telemetry'`,
		).Scan(&telemetryFailures)).To(Succeed())
		Expect(telemetryFailures).To(Equal(1), "the telemetry job exists and is not counted below")

		jobs, err := readJobs(ctx, pool)

		Expect(err).NotTo(HaveOccurred())
		Expect(jobs).To(HaveLen(2), "only the two umh compression policies")
	})

	It("fails against a plain Postgres with no TimescaleDB extension", func() {
		pool := startDatabase(postgresImage)

		_, err := Collect(ctx, pool)

		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("Policy reporting", Label("integration"), func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("reports a historian that compresses but never expires data", func() {
		pool := startDatabase(timescaleImage)
		_, err := pool.Exec(ctx, historianSchemaDDL)
		Expect(err).NotTo(HaveOccurred())

		tables, err := readTables(ctx, pool)

		Expect(err).NotTo(HaveOccurred())
		Expect(tableNamed(tables, "value_bench").CompressAfterSeconds).To(Equal(int64(604800)), "168h")
		Expect(tableNamed(tables, "value_bench").RetentionSeconds).To(BeZero(),
			"nothing expires, so the database grows forever")
	})

	It("reports the retention interval when one is configured", func() {
		pool := startDatabase(timescaleImage)
		_, err := pool.Exec(ctx, historianSchemaDDL)
		Expect(err).NotTo(HaveOccurred())
		_, err = pool.Exec(ctx, retentionSchemaDDL)
		Expect(err).NotTo(HaveOccurred())

		tables, err := readTables(ctx, pool)

		Expect(err).NotTo(HaveOccurred())
		Expect(tableNamed(tables, "value_bench").RetentionSeconds).To(Equal(int64(2592000)), "720h")
	})

	It("reports each table's own interval when they disagree", func() {
		pool := startDatabase(timescaleImage)
		_, err := pool.Exec(ctx, historianSchemaDDL)
		Expect(err).NotTo(HaveOccurred())
		_, err = pool.Exec(ctx, driftedPolicyDDL)
		Expect(err).NotTo(HaveOccurred())

		tables, err := readTables(ctx, pool)

		Expect(err).NotTo(HaveOccurred())
		Expect(tableNamed(tables, "value_bench").CompressAfterSeconds).To(Equal(int64(604800)))
		Expect(tableNamed(tables, "attribute_bench").CompressAfterSeconds).To(Equal(int64(1209600)),
			"the drift is visible per table, which is where the console reads it")
	})
})

// readTables is what the table listing looks like before Collect filters it: the
// hypertables and the plain tables of the historian schema, foreign ones
// included, which is what the specs below assert against.
func readTables(ctx context.Context, db Querier) ([]Table, error) {
	hypertables, err := readHypertables(ctx, db)
	if err != nil {
		return nil, err
	}

	plainTables, err := readPlainTables(ctx, db)
	if err != nil {
		return nil, err
	}

	return append(hypertables, plainTables...), nil
}

// tableNamed returns the reported entry for one hypertable, failing the spec when
// it is absent so the assertion that follows reads against a real value.
func tableNamed(tables []Table, name string) Table {
	for _, table := range tables {
		if table.Name == name {
			return table
		}
	}

	Fail("no reported table named " + name)

	return Table{}
}

var _ = Describe("Per-table reporting", Label("integration"), func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("reports one entry per hypertable, named", func() {
		pool := startDatabase(timescaleImage)
		_, err := pool.Exec(ctx, historianSchemaDDL)
		Expect(err).NotTo(HaveOccurred())

		tables, err := readTables(ctx, pool)

		Expect(err).NotTo(HaveOccurred())
		Expect(tables).To(HaveLen(2))
		Expect([]string{tables[0].Name, tables[1].Name}).
			To(ConsistOf("attribute_bench", "value_bench"))
	})

	It("reports storage and compression for each table", func() {
		pool := startDatabase(timescaleImage)
		_, err := pool.Exec(ctx, historianSchemaDDL)
		Expect(err).NotTo(HaveOccurred())

		tables, err := readTables(ctx, pool)
		Expect(err).NotTo(HaveOccurred())

		value := tableNamed(tables, "value_bench")
		Expect(value.Chunks).To(BeNumerically(">", 0))
		Expect(value.CompressedChunks).To(BeNumerically(">", 0))
		Expect(value.BytesBeforeCompression).To(BeNumerically(">", 0))
		Expect(value.BytesAfterCompression).To(BeNumerically(">", 0))
		Expect(value.DiskBytes).To(BeNumerically(">=", value.BytesAfterCompression))
	})

	It("counts the chunks no policy has compressed into a table's size", func() {
		pool := startDatabase(timescaleImage)
		_, err := pool.Exec(ctx, historianSchemaDDL)
		Expect(err).NotTo(HaveOccurred())

		_, err = pool.Exec(ctx, `INSERT INTO umh.value_bench
			SELECT n, now() - (n || ' minutes')::interval, random()
			  FROM generate_series(1, 20000) n`)
		Expect(err).NotTo(HaveOccurred())

		tables, err := readTables(ctx, pool)
		Expect(err).NotTo(HaveOccurred())

		value := tableNamed(tables, "value_bench")
		Expect(value.CompressedChunks).To(BeNumerically("<", value.Chunks),
			"the rows just written are in chunks no policy has compressed yet")
		Expect(value.DiskBytes).To(BeNumerically(">", value.BytesAfterCompression),
			"a size that counted only the compressed chunks would miss them")
	})

	It("reports the chunk interval each table was created with", func() {
		pool := startDatabase(timescaleImage)
		_, err := pool.Exec(ctx, historianSchemaDDL)
		Expect(err).NotTo(HaveOccurred())

		tables, err := readTables(ctx, pool)

		Expect(err).NotTo(HaveOccurred())
		Expect(tableNamed(tables, "value_bench").ChunkIntervalSeconds).To(Equal(int64(604800)), "168h")
	})

	It("reports each table's own policies, so a table without retention is visible", func() {
		pool := startDatabase(timescaleImage)
		_, err := pool.Exec(ctx, historianSchemaDDL)
		Expect(err).NotTo(HaveOccurred())
		_, err = pool.Exec(ctx, retentionSchemaDDL)
		Expect(err).NotTo(HaveOccurred())

		tables, err := readTables(ctx, pool)
		Expect(err).NotTo(HaveOccurred())

		Expect(tableNamed(tables, "value_bench").RetentionSeconds).To(Equal(int64(2592000)), "720h")
		Expect(tableNamed(tables, "attribute_bench").RetentionSeconds).To(BeZero(),
			"this table expires nothing, which the aggregate alone would hide")
		Expect(tableNamed(tables, "value_bench").CompressAfterSeconds).To(Equal(int64(604800)))
	})
})

var _ = Describe("Data span reporting", Label("integration"), func() {
	It("reports how much history the hypertables cover", func() {
		ctx := context.Background()
		pool := startDatabase(timescaleImage)
		_, err := pool.Exec(ctx, historianSchemaDDL)
		Expect(err).NotTo(HaveOccurred())

		metrics, err := Collect(ctx, pool)

		Expect(err).NotTo(HaveOccurred())
		// The schema writes 200 daily points, and the span is taken from the rows
		// themselves, so it is the interval between the first and the last.
		Expect(metrics.DataSpanSeconds).To(BeNumerically("~", 199*24*3600, 24*3600))
	})

	It("reports no span when nothing has been written", func() {
		ctx := context.Background()
		pool := startDatabase(timescaleImage)
		_, err := pool.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS umh;`)
		Expect(err).NotTo(HaveOccurred())

		metrics, err := Collect(ctx, pool)

		Expect(err).NotTo(HaveOccurred())
		Expect(metrics.DataSpanSeconds).To(BeZero())
	})
})

var _ = Describe("Per-table rows and timespan", Label("integration"), func() {
	ctx := context.Background()

	It("reports roughly how many rows a table holds", func() {
		pool := startDatabase(timescaleImage)
		_, err := pool.Exec(ctx, historianSchemaDDL)
		Expect(err).NotTo(HaveOccurred())

		metrics, err := Collect(ctx, pool)

		Expect(err).NotTo(HaveOccurred())
		// The fixture inserts 200 rows. The count is approximate by design, read
		// from planner statistics rather than by scanning, so it is asserted as a
		// range: an exact count is unaffordable on a real historian.
		Expect(tableNamed(metrics.Tables, "value_bench").Rows).To(BeNumerically("~", 200, 20))
	})

	It("counts rows written into a chunk nothing has analysed yet", func() {
		pool := startDatabase(timescaleImage)
		_, err := pool.Exec(ctx, historianSchemaDDL)
		Expect(err).NotTo(HaveOccurred())

		// The planner's estimate for a relation it has never analysed is -1, which
		// reads as no rows at all. This is the table an operator is most likely to
		// be looking at: the one that started receiving data a moment ago.
		_, err = pool.Exec(ctx, `INSERT INTO umh.value_bench
			SELECT n, now() - (n || ' seconds')::interval, random()
			  FROM generate_series(1, 50000) n`)
		Expect(err).NotTo(HaveOccurred())

		// The statistics system flushes what a backend has counted at an interval,
		// so the rows appear within a second of the write rather than instantly.
		Eventually(func() int64 {
			metrics, err := Collect(ctx, pool)
			Expect(err).NotTo(HaveOccurred())

			return tableNamed(metrics.Tables, "value_bench").Rows
		}, "10s", "500ms").Should(BeNumerically(">=", 50000),
			"every row just written is counted")
	})

	It("reports how long ago the first entry was, giving each table its own span", func() {
		pool := startDatabase(timescaleImage)
		_, err := pool.Exec(ctx, historianSchemaDDL)
		Expect(err).NotTo(HaveOccurred())

		metrics, err := Collect(ctx, pool)
		Expect(err).NotTo(HaveOccurred())

		table := tableNamed(metrics.Tables, "value_bench")

		oldestRow, err := time.Parse(time.RFC3339, table.EarliestRowTimestamp)
		Expect(err).NotTo(HaveOccurred(), "the first write is a parseable timestamp")
		newestRow, err := time.Parse(time.RFC3339, table.LatestRowTimestamp)
		Expect(err).NotTo(HaveOccurred(), "the last write is a parseable timestamp")

		// The fixture writes one row per day going back 200 days.
		Expect(oldestRow).To(BeTemporally("~", time.Now().Add(-200*24*time.Hour), 24*time.Hour))
		Expect(oldestRow).To(BeTemporally("<", newestRow), "the oldest row precedes the newest")
	})
})

var _ = Describe("Table selection", Label("integration"), func() {
	ctx := context.Background()

	It("counts and sizes the tables the historian did not create, apart from its own", func() {
		pool := startDatabase(timescaleImage)
		_, err := pool.Exec(ctx, historianSchemaDDL)
		Expect(err).NotTo(HaveOccurred())

		_, err = pool.Exec(ctx, `
CREATE TABLE umh.customer_export (id BIGINT, payload TEXT);
INSERT INTO umh.customer_export SELECT g, repeat('x', 500) FROM generate_series(1, 500) g;
CREATE TABLE umh.scratch_notes (id BIGINT);
ANALYZE umh.customer_export;`)
		Expect(err).NotTo(HaveOccurred())

		metrics, err := Collect(ctx, pool)
		Expect(err).NotTo(HaveOccurred())

		names := []string{}
		for _, table := range metrics.Tables {
			names = append(names, table.Name)
		}

		Expect(names).NotTo(ContainElement("customer_export"))
		Expect(names).NotTo(ContainElement("scratch_notes"))
	})

	It("keeps the tables the historian does create", func() {
		pool := startDatabase(timescaleImage)
		_, err := pool.Exec(ctx, historianSchemaDDL)
		Expect(err).NotTo(HaveOccurred())

		metrics, err := Collect(ctx, pool)
		Expect(err).NotTo(HaveOccurred())

		names := []string{}
		for _, table := range metrics.Tables {
			names = append(names, table.Name)
		}

		Expect(names).To(ContainElement("value_bench"))
		Expect(names).To(ContainElement("attribute_bench"))
	})
})

var _ = Describe("Stale and small tables", Label("integration"), func() {
	ctx := context.Background()

	It("reports the last write of a table that stopped receiving data long ago", func() {
		pool := startDatabase(timescaleImage)
		_, err := pool.Exec(ctx, historianSchemaDDL)
		Expect(err).NotTo(HaveOccurred())

		// A table whose writes stopped is the one an operator most needs to see, so
		// the last write must not be hidden behind a recency window.
		_, err = pool.Exec(ctx, `DELETE FROM umh.value_bench WHERE ts > now() - interval '90 days'`)
		Expect(err).NotTo(HaveOccurred())

		metrics, err := Collect(ctx, pool)
		Expect(err).NotTo(HaveOccurred())

		newestRow, err := time.Parse(time.RFC3339, tableNamed(metrics.Tables, "value_bench").LatestRowTimestamp)
		Expect(err).NotTo(HaveOccurred(), "a table silent for months still reports when it last wrote")
		Expect(newestRow).To(BeTemporally("<", time.Now().Add(-89*24*time.Hour)))
	})

	It("counts a lookup table exactly, because its planner estimate is zero until analysed", func() {
		pool := startDatabase(timescaleImage)
		_, err := pool.Exec(ctx, historianSchemaDDL)
		Expect(err).NotTo(HaveOccurred())

		_, err = pool.Exec(ctx, `
CREATE TABLE umh.tag (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL);
INSERT INTO umh.tag (name) SELECT 'tag_' || g FROM generate_series(1, 10) AS g;`)
		Expect(err).NotTo(HaveOccurred())

		metrics, err := Collect(ctx, pool)

		Expect(err).NotTo(HaveOccurred())
		Expect(tableNamed(metrics.Tables, "tag").Rows).To(Equal(int64(10)))
	})
})

var _ = Describe("Summary collection", Label("integration"), func() {
	ctx := context.Background()

	It("reports the tables and the failing job count", func() {
		pool := startDatabase(timescaleImage)
		_, err := pool.Exec(ctx, historianSchemaDDL)
		Expect(err).NotTo(HaveOccurred())

		summary, err := CollectSummary(ctx, pool)

		Expect(err).NotTo(HaveOccurred())
		Expect(summary.FailedJobs).To(BeZero())
		Expect(summary.TableNames).To(ConsistOf("value_bench", "attribute_bench"))
	})

	It("reports nothing rather than failing on a database with no historian", func() {
		pool := startDatabase(timescaleImage)

		summary, err := CollectSummary(ctx, pool)

		Expect(err).NotTo(HaveOccurred())
		Expect(summary.TableNames).To(BeEmpty())
		Expect(summary.FailedJobs).To(BeZero())
	})

	It("lists only the tables the historian created", func() {
		pool := startDatabase(timescaleImage)
		_, err := pool.Exec(ctx, historianSchemaDDL)
		Expect(err).NotTo(HaveOccurred())
		_, err = pool.Exec(ctx, `CREATE TABLE umh.customer_export (id BIGINT)`)
		Expect(err).NotTo(HaveOccurred())

		summary, err := CollectSummary(ctx, pool)

		Expect(err).NotTo(HaveOccurred())
		Expect(summary.TableNames).NotTo(ContainElement("customer_export"))
	})
})
