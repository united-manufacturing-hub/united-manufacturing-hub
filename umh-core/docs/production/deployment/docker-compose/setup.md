# Setup Guide

This guide walks you through setting up umh-core with Docker Compose, starting with a minimal configuration and building up to a full production-ready stack.

## Prerequisites

- Docker and Docker Compose installed on your system
- Basic familiarity with the command line

## Minimal Setup

1. Find the latest version on the [Releases](https://github.com/united-manufacturing-hub/united-manufacturing-hub/releases) page and replace `<VERSION>` with your selected version.
2. Save this in `docker-compose.yaml`.

```yaml
services:
  umh:
    image: management.umh.app/oci/united-manufacturing-hub/umh-core:<VERSION> # TODO: change this
    restart: unless-stopped
    volumes:
      - umh-data:/data
    environment:
      - AUTH_TOKEN=your-auth-token # TODO: change this
      - LOCATION_0=your-location # TODO: change this
      - RELEASE_CHANNEL=stable
      - API_URL=https://management.umh.app/api

volumes:
  umh-data: {}
```

Then run:

```bash
docker compose up -d
```

This achieves the same result as the docker cli commands, but the configuration is now documented in a file that you can version control and extend.

## Adding TimescaleDB and Grafana

While umh-core can operate standalone, most deployments benefit from persistent storage and visualization capabilities.

### TimescaleDB

[TimescaleDB](https://www.timescale.com/) is PostgreSQL with an extension optimized for time-series data. Manufacturing data is inherently time-series: sensor readings, production counts, and machine states all have timestamps. It is an optimal solution to store this kind of data.

#### PgBouncer

PgBouncer is a connection pooler that manages database connections efficiently.

umh-core bridges can create many simultaneous connections to the database. PostgreSQL has a limited number of connections it can handle (typically 100 by default). Without connection pooling, a busy system can exhaust this limit, causing connection failures. PgBouncer multiplexes many client connections onto a pool of database connections. This allows hundreds of application connections while using only a few actual PostgreSQL connections.

In this configuration applications connect to PgBouncer on port 5432 and PgBouncer connects to TimescaleDB internally.

#### Adding TimescaleDB

Below are the changes to be made to the base configuration to deploy TimescaleDB and PgBouncer together with umh-core.

```diff
services:
  umh:
    image: management.umh.app/oci/united-manufacturing-hub/umh-core:<VERSION> # TODO: change this
    restart: unless-stopped
    volumes:
      - umh-data:/data
    environment:
      - AUTH_TOKEN=your-auth-token # TODO: change this
      - RELEASE_CHANNEL=stable
      - API_URL=https://management.umh.app/api
      - LOCATION_0=your-location # TODO: change this

+   pgbouncer:
+     image: docker.io/edoburu/pgbouncer:v1.24.1-p1
+     restart: unless-stopped
+     environment:
+       - DB_NAME=umh
+       - DB_USER=postgres     # TODO: change this, it has to be the same as `POSTGRES_USER` for timescaledb below
+       - DB_PASSWORD=postgres # TODO: change this, it has to be the same as `POSTGRES_PASSWORD` for timescaledb below
+       - DB_HOST=timescaledb
+       - AUTH_TYPE=scram-sha-256
+     ports:
+       - 5432:5432
+     healthcheck:
+       test: ["CMD-SHELL", "pg_isready -h pgbouncer"]
+       interval: 10s
+       timeout: 5s
+       retries: 5
+     depends_on:
+       timescaledb:
+         condition: service_healthy
+     networks:
+       - default
+       - timescaledb-network

+   timescaledb:
+     image: management.umh.app/oci/timescale/timescaledb:2.24.0-pg17
+     restart: unless-stopped
+     environment:
+       - POSTGRES_DB=umh
+       - POSTGRES_USER=postgres     # TODO: change this
+       - POSTGRES_PASSWORD=postgres # TODO: change this
+     volumes:
+       - timescaledb-data:/var/lib/postgresql/data
+     healthcheck:
+       test: ["CMD-SHELL", "pg_isready -h timescaledb"]
+       interval: 10s
+       timeout: 5s
+       retries: 5
+     networks:
+       - timescaledb-network

+ networks:
+   default:
+   timescaledb-network:
+     internal: true

  volumes:
    umh-data: {}
+   timescaledb-data: {}
```

This declares:

- 2 Services called `pgbouncer` and `timescaledb`: PgBouncer acts as a proxy to TimescaleDB. This means the Credentials have to match between these two Services. Healthchecks ensure that the Services are started in order. `timescaledb` is isolated in the Network `timescaledb-network`. `pgbouncer` is both in the `default` and the `timescaledb-network`. This makes PgBouncer the only Service which can talk to TimescaleDB directly.
- 2 Networks called `timescaledb-network` and `default`: Without the Network section the Network `default` is always created by default. Services use the Network `default` if no explicit Network configuration is provided. The `timescaledb-network` is configured to be internal which means Services can't reach the internet or any Service outside this Network if they are only connected through this Network.
- 1 Volume called `timescaledb-data`: This is where TimescaleDB stores its data.

### Grafana

[Grafana](https://grafana.com/) is an open-source visualization platform. It can for example connect to TimescaleDB and allows you to build dashboards showing real-time and historical data.

#### Adding Grafana

Below are the changes to be made to the base configuration to deploy Grafana together with umh-core.

```diff
  services:
    umh:
      image: management.umh.app/oci/united-manufacturing-hub/umh-core:<VERSION> # TODO: change this
      restart: unless-stopped
      volumes:
        - umh-data:/data
      environment:
        - AUTH_TOKEN=your-auth-token # TODO: change this
        - RELEASE_CHANNEL=stable
        - API_URL=https://management.umh.app/api
        - LOCATION_0=your-location # TODO: change this

+   grafana:
+     image: management.umh.app/oci/grafana/grafana:12.3.0
+     restart: unless-stopped
+     ports:
+       - 3000:3000
+     volumes:
+       - grafana-data:/var/lib/grafana
+     healthcheck:
+       test: ["CMD-SHELL", "curl --fail http://grafana:3000/api/health"]
+       interval: 10s
+       timeout: 5s
+       retries: 3

  volumes:
    umh-data: {}
+   grafana-data: {}
```

## umh-core + Grafana + PgBouncer + TimescaleDB

To get the most out of Grafana it should be deployed together with TimescaleDB to have a good and persistent data source.

> IMPORTANT: Do **not** forget to change Database credentials before using this in production!

```yaml
services:
  umh:
    image: management.umh.app/oci/united-manufacturing-hub/umh-core:<VERSION> # TODO: change this
    restart: unless-stopped
    volumes:
      - umh-data:/data
    environment:
      - AUTH_TOKEN=your-auth-token # TODO: change this
      - RELEASE_CHANNEL=stable
      - API_URL=https://management.umh.app/api
      - LOCATION_0=your-location # TODO: change this

  grafana:
    image: management.umh.app/oci/grafana/grafana:12.3.0
    restart: unless-stopped
    ports:
      - 3000:3000
    volumes:
      - grafana-data:/var/lib/grafana
    healthcheck:
      test: ["CMD-SHELL", "curl --fail http://grafana:3000/api/health"]
      interval: 10s
      timeout: 5s
      retries: 3

  pgbouncer:
    image: docker.io/edoburu/pgbouncer:v1.24.1-p1
    restart: unless-stopped
    environment:
      - DB_NAME=umh
      - DB_USER=postgres     # TODO: change this, it has to be the same as `POSTGRES_USER` for timescaledb below
      - DB_PASSWORD=postgres # TODO: change this, it has to be the same as `POSTGRES_PASSWORD` for timescaledb below
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

  timescaledb:
    image: management.umh.app/oci/timescale/timescaledb:2.24.0-pg17
    restart: unless-stopped
    environment:
      - POSTGRES_DB=umh
      - POSTGRES_USER=postgres     # TODO: change this
      - POSTGRES_PASSWORD=postgres # TODO: change this
    volumes:
      - timescaledb-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -h timescaledb"]
      interval: 10s
      timeout: 5s
      retries: 5
    networks:
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

## Starting the Stack

Once the configuration is done you can start the full application stack by using `docker compose up -d`. There are a lot of useful commands for docker compose for example:

- `docker compose ps`: check the running state of all services
- `docker compose stats`: check resource usage of all services
- `docker compose pull`: attempt to pull new versions of all images used in the `docker-compose.yaml`

For more refer to the [official Docker Compose documentation](https://docs.docker.com/compose/).

Once started, you can access:

- Grafana at http://localhost:3000 (default login: admin/admin)
- PostgreSQL through PgBouncer at localhost:5432
