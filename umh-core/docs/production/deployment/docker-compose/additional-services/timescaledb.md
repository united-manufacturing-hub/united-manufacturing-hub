# TimescaleDB

In this example you'll learn how to add TimescaleDB and PgBouncer to a umh-core Docker Compose stack. If you are new to running umh-core with Docker Compose, read [Setup](../setup.md) first.

[TimescaleDB](https://www.timescale.com/) is PostgreSQL with an extension optimized for time-series data. Manufacturing data is inherently time-series: sensor readings, production counts, and machine states all have timestamps.

## PgBouncer

PgBouncer is a connection pooler that manages database connections efficiently.

umh-core bridges can create many simultaneous connections to the database. PostgreSQL has a limited number of connections it can handle (typically 100 by default). Without connection pooling, a busy system can exhaust this limit, causing connection failures.

In this configuration applications connect to PgBouncer on port 5432 and PgBouncer connects to TimescaleDB internally.

## Adding TimescaleDB

Below are the changes to be made to the minimal configuration to deploy TimescaleDB and PgBouncer together with umh-core.

```diff
  services:
    umh:
      image: management.umh.app/oci/united-manufacturing-hub/umh-core:v0.40.30
      restart: unless-stopped
      volumes:
        - umh-data:/data
      environment:
        - AUTH_TOKEN=your-auth-token
        - LOCATION_0=your-location
        - RELEASE_CHANNEL=stable
        - API_URL=https://management.umh.app/api

+   pgbouncer:
+     image: management.umh.app/oci/edoburu/pgbouncer:v1.24.1-p1
+     restart: unless-stopped
+     environment:
+       - DB_NAME=umh
+       - DB_USER=postgres
+       - DB_PASSWORD=postgres
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
+       - POSTGRES_USER=postgres
+       - POSTGRES_PASSWORD=postgres
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

## Complete docker-compose.yaml

umh-core with TimescaleDB and PgBouncer. Copy this into `docker-compose.yaml` and fill in the fields marked `TODO`.

> **IMPORTANT**: Change the template's database credentials before using this in production!

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
      # TODO: Enter your instance's Auth Token. You'll find it in your instance's configuration file on management.umh.app.
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

volumes:
  umh-data: {}

  timescaledb:
    image: management.umh.app/oci/timescale/timescaledb:2.24.0-pg17
    restart: unless-stopped
    environment:
      - POSTGRES_DB=umh
      # TODO: Set your postgresDB Password and Username here
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
      # This has to be the same value as timescaledb.environment.POSTGRES_USER
      - DB_USER=postgres
      # This has to be the same value as timescaledb.environment.POSTGRES_PASSWORD
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
```

Once the stack is running, PostgreSQL is reachable through PgBouncer at `localhost:5432`.

A database on its own has nothing to show you. Most deployments add [Grafana](grafana.md) on top of it, which is why both are part of the **[Recommended UMH Stack](recommended-umh-stack.md)**.
