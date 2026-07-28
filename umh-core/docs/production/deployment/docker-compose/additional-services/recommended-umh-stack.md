# Recommended UMH Stack

This is the Docker Compose configuration we recommend for most deployments: umh-core, Grafana, PgBouncer, and TimescaleDB in one file. If you are new to running umh-core with Docker Compose, read [Setup](../setup.md) first.

## Why these four services

umh-core collects and standardizes your data, but it does not keep a long history and it does not draw charts. The other three cover that.

- **TimescaleDB** is PostgreSQL with a time-series extension. Sensor readings, production counts, and machine states all arrive with timestamps, which is exactly what it is built to store and query.
- **Grafana** reads from TimescaleDB and turns it into dashboards. On its own it has nothing to query.
- **PgBouncer** sits in front of TimescaleDB as a connection pooler. umh-core bridges can open many simultaneous connections, and PostgreSQL handles a limited number of them (typically 100 by default). PgBouncer pools hundreds of client connections onto a handful of connections.

TimescaleDB is placed in an internal network that only PgBouncer can reach, so nothing else talks to the database directly.

## The configuration

Copy this into `docker-compose.yaml` and fill in the fields marked `TODO`. Find the latest umh-core version on the [Releases](https://github.com/united-manufacturing-hub/united-manufacturing-hub/releases) page and replace `<VERSION>`.

> **IMPORTANT:** Change the database and Grafana credentials before using this in production!

```yaml
services:
  umh:
    # TODO: set your desired container version here 
    # e.g. `umh-core:v0.44.31`
    image: management.umh.app/oci/united-manufacturing-hub/umh-core:ENTER_VERSION_HERE
    restart: unless-stopped
    volumes:
      - umh-data:/data
    environment:
      # TODO: Enter your instance's Auth Token. 
      # You'll find it in your instance's configuration
      # file on management.umh.app.
      - AUTH_TOKEN=your_auth_token
      # TODO: Change the LOCATION_0 parameter
      # to your desired Level 0 Location name
      - LOCATION_0=your_level_0_location
      # Optional: Define more levels
      # - LOCATION_1=your_level_1
      # - LOCATION_2=your_level_2
      # - LOCATION_3=your_level_3
      # - LOCATION_4=your_level_4
      - RELEASE_CHANNEL=stable
      - API_URL=https://management.umh.app/api

  grafana:
    image: management.umh.app/oci/grafana/grafana:12.3.0
    restart: unless-stopped
    ports:
      - 3000:3000
    environment:
      # TODO: Set your desired username and password here
      # You'll need these credentials to
      # access your local Grafana instance
      - GF_SECURITY_ADMIN_USER=admin
      - GF_SECURITY_ADMIN_PASSWORD=admin
    volumes:
      - grafana-data:/var/lib/grafana
    healthcheck:
      test: ["CMD-SHELL", "curl --fail http://grafana:3000/api/health"]
      interval: 10s
      timeout: 5s
      retries: 3

  timescaledb:
    image: management.umh.app/oci/timescale/timescaledb:2.24.0-pg17
    restart: unless-stopped
    environment:
      - POSTGRES_DB=umh
      # TODO: Set your postgresDB
      # Password and Username here
      - POSTGRES_USER=postgres
      - POSTGRES_PASSWORD=postgres
    volumes:
      - timescaledb-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -h timescaledb"]
      interval: 10s
      timeout: 5s
      retries: 5
    networks:
      - timescaledb-network

  pgbouncer:
    image: management.umh.app/oci/edoburu/pgbouncer:v1.24.1-p1
    restart: unless-stopped
    environment:
      - DB_NAME=umh
      # This has to be the same value as
      # timescaledb.environment.POSTGRES_USER
      - DB_USER=postgres
      # This has to be the same value as
      # timescaledb.environment.POSTGRES_PASSWORD
      - DB_PASSWORD=postgres
      - DB_HOST=timescaledb
      - AUTH_TYPE=scram-sha-256
    ports:
      - 5432:5432
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -h pgbouncer"]
      interval: 10s
      timeout: 5s
      retries: 5
    depends_on:
      timescaledb:
        condition: service_healthy
    networks:
      - default
      - timescaledb-network

networks:
  default:
  timescaledb-network:
    internal: true

volumes:
  umh-data: {}
  timescaledb-data: {}
  grafana-data: {}
```

Prefer to add one service at a time? Each page carries the same configuration in smaller pieces: [TimescaleDB](timescaledb.md), [Grafana](grafana.md).

To start the stack, see [Starting the Stack](../setup.md#starting-the-stack).

## VM Sizing

Our **[Sizing Guide](../../../sizing-guide.md)** covers umh-core on its own, and its recommended box is sized for that. Redpanda inside umh-core already budgets roughly 2 GB per core plus headroom, so running a database and Grafana next to it on the minimum box leaves little room. Give the host more memory and disk than the baseline, and watch actual usage before settling on a size.

## What's next

Starting the stack gives you four running containers. Two things still need configuring before a dashboard shows anything:

1. **Get data into TimescaleDB.** umh-core does not write to the database by itself. You configure a flow that reads from the Unified Namespace and writes to PostgreSQL. See [Stand-alone Flow](../../../../usage/data-flows/stand-alone-flow.md), which covers the `kafka_to_postgresql_historian_bridge`.
2. **Point Grafana at the database.** Add a PostgreSQL data source in Grafana using host `pgbouncer:5432`, database `umh`, and the credentials you set above. See Grafana's [PostgreSQL data source documentation](https://grafana.com/docs/grafana/latest/datasources/postgres/).
