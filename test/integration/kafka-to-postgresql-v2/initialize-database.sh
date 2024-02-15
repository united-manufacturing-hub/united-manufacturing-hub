#!/bin/bash

echo "Initializing the TimescaleDB database..."

# Run your database initialization commands here
# Example: Create a database and execute SQL commands
docker exec -u postgres timescaledb createdb umh_v2
docker exec -i timescaledb psql -U postgres -d umh_v2 <<-EOSQL
    CREATE TABLE asset (
      id SERIAL PRIMARY KEY,
      enterprise TEXT NOT NULL,
      site TEXT DEFAULT '' NOT NULL,
      area TEXT DEFAULT '' NOT NULL,
      line TEXT DEFAULT '' NOT NULL,
      workcell TEXT DEFAULT '' NOT NULL,
      origin_id TEXT DEFAULT '' NOT NULL,
      UNIQUE (enterprise, site, area, line, workcell, origin_id)
    );

    CREATE TABLE tag (
      timestamp TIMESTAMPTZ NOT NULL,
      name TEXT NOT NULL,
      origin TEXT NOT NULL,
      asset_id INT REFERENCES asset(id) NOT NULL,
      value REAL,
      UNIQUE (name, asset_id, timestamp)
    );
    SELECT create_hypertable('tag', 'timestamp');
    CREATE INDEX ON tag (asset_id, timestamp DESC);
    CREATE INDEX ON tag (name);

    CREATE TABLE tag_string (
      timestamp TIMESTAMPTZ NOT NULL,
      name TEXT NOT NULL,
      origin TEXT NOT NULL,
      asset_id INT REFERENCES asset(id) NOT NULL,
      value TEXT,
      UNIQUE (name, asset_id, timestamp)
    );
    SELECT create_hypertable('tag_string', 'timestamp');
    CREATE INDEX ON tag_string (asset_id, timestamp DESC);
    CREATE INDEX ON tag_string (name);
EOSQL

echo "Database initialized successfully."
