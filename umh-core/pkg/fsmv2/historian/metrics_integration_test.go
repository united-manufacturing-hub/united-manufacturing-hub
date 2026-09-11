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

package fsmv2timescale

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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
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

		metrics, err := collectMetrics(ctx, pool)

		Expect(err).NotTo(HaveOccurred())
		Expect(metrics.ServerVersion).To(HavePrefix("17."))
		Expect(metrics.TimescaleVersion).To(Equal("2.24.0"))
		Expect(metrics.Hypertables).To(Equal(2))
		Expect(metrics.Chunks).To(BeNumerically(">", 0))
		Expect(metrics.CompressedChunks).To(BeNumerically(">", 0))
		Expect(metrics.UncompressedBytes).To(BeNumerically(">", 0))
		Expect(metrics.CompressedBytes).To(BeNumerically(">", 0))
		Expect(metrics.DatabaseBytes).To(BeNumerically(">", 0))
	})

	It("counts no failed jobs when every umh job is succeeding", func() {
		pool := startDatabase(timescaleImage)
		_, err := pool.Exec(ctx, historianSchemaDDL)
		Expect(err).NotTo(HaveOccurred())

		metrics, err := collectMetrics(ctx, pool)

		Expect(err).NotTo(HaveOccurred())
		Expect(metrics.Jobs).To(Equal(2), "one compression policy per hypertable")
		Expect(metrics.FailedJobs).To(BeZero())
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

		metrics, err := collectMetrics(ctx, pool)

		Expect(err).NotTo(HaveOccurred())
		Expect(metrics.Jobs).To(Equal(2), "only the two umh compression policies")
	})

	It("fails against a plain Postgres with no TimescaleDB extension", func() {
		pool := startDatabase(postgresImage)

		_, err := collectMetrics(ctx, pool)

		Expect(err).To(HaveOccurred())
	})
})

// pollDeps builds one worker instance's Poll dependencies the way the framework
// would, so a spec exercises the same wiring production gets.
func pollDeps() Deps {
	return newDeps(idUnder("poll-metrics"), baseUnder("poll-metrics"))
}

var _ = Describe("Poll reporting metrics", Label("integration"), func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("reports metrics alongside the connection check", func() {
		cfg, pool := startDatabaseWithConfig(timescaleImage)
		_, err := pool.Exec(ctx, historianSchemaDDL)
		Expect(err).NotTo(HaveOccurred())

		status, err := Poll(ctx, pollDeps(), cfg)

		Expect(err).NotTo(HaveOccurred())
		Expect(status.Reachable).To(BeTrue())
		Expect(status.Auth).To(Equal(models.TimescaleAuthValid))
		Expect(status.Hypertables).To(Equal(2))
		Expect(status.MetricsError).To(BeEmpty())
	})

	It("stays healthy when the database is reachable but carries no TimescaleDB", func() {
		cfg, _ := startDatabaseWithConfig(postgresImage)

		status, err := Poll(ctx, pollDeps(), cfg)

		Expect(err).NotTo(HaveOccurred(), "a metrics failure must not degrade the worker")
		Expect(status.Reachable).To(BeTrue())
		Expect(status.Auth).To(Equal(models.TimescaleAuthValid))
		Expect(status.MetricsError).NotTo(BeEmpty())
		Expect(status.Hypertables).To(BeZero())
	})

	It("keeps the collected metrics on a tick that does not re-collect", func() {
		cfg, pool := startDatabaseWithConfig(timescaleImage)
		_, err := pool.Exec(ctx, historianSchemaDDL)
		Expect(err).NotTo(HaveOccurred())

		deps := pollDeps()
		first, err := Poll(ctx, deps, cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(first.Hypertables).To(Equal(2))

		second, err := Poll(ctx, deps, cfg)

		Expect(err).NotTo(HaveOccurred())
		Expect(second.Hypertables).To(Equal(2), "the second tick must not blank the metrics")
	})
})

var _ = Describe("Metrics surviving a connection failure", Label("integration"), func() {
	It("still reports the last metrics when the connection later fails", func() {
		ctx := context.Background()
		cfg, pool := startDatabaseWithConfig(timescaleImage)
		_, err := pool.Exec(ctx, historianSchemaDDL)
		Expect(err).NotTo(HaveOccurred())

		deps := pollDeps()
		healthy, err := Poll(ctx, deps, cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(healthy.Hypertables).To(Equal(2))

		unreachable := cfg
		unreachable.Timescale.Host = "127.0.0.1"
		unreachable.Timescale.Port = closedPort()

		broken, err := Poll(ctx, deps, unreachable)

		Expect(err).To(HaveOccurred(), "the connection check fails")
		Expect(broken.Reachable).To(BeFalse())
		Expect(broken.Hypertables).To(Equal(2),
			"a connection blip must not blank the last known database metrics")
	})
})
