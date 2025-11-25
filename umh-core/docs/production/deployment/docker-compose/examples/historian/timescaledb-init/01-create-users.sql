-- ==============================================================================
-- UMH TimescaleDB Initialization Script
-- ==============================================================================
-- This script creates the database schema and users for UMH Core historian
-- Executed automatically on first container startup
-- Reference: Linear tickets IT-276, ENG-3107, INS-17

-- ==============================================================================
-- Create Users
-- ==============================================================================
-- kafkatopostgresqlv2: Write access for UNS-to-TimescaleDB bridge
-- grafanareader: Read-only access for Grafana dashboards

-- Generate secure passwords in production:
-- openssl rand -base64 24

-- Writer user (used by UNS-to-TimescaleDB bridge)
CREATE USER kafkatopostgresqlv2 WITH PASSWORD 'changeme';

-- Reader user (used by Grafana)
CREATE USER grafanareader WITH PASSWORD 'changeme';

-- ==============================================================================
-- Enable TimescaleDB Extension
-- ==============================================================================
-- Must be enabled before creating hypertables

CREATE EXTENSION IF NOT EXISTS timescaledb;

-- ==============================================================================
-- Create Schema
-- ==============================================================================

-- Asset table: Metadata about devices/equipment
-- Maps asset_name to asset_id for efficient time-series queries
CREATE TABLE IF NOT EXISTS asset (
    id SERIAL PRIMARY KEY,
    asset_name VARCHAR(255) NOT NULL UNIQUE,
    location VARCHAR(500),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Tag hypertable: Numeric time-series data
-- Stores temperature, pressure, speed, count, and other numeric values
CREATE TABLE IF NOT EXISTS tag (
    time TIMESTAMPTZ NOT NULL,
    asset_id INTEGER NOT NULL REFERENCES asset(id) ON DELETE CASCADE,
    tag_name VARCHAR(255) NOT NULL,
    value DOUBLE PRECISION,
    origin VARCHAR(255)
);

-- Convert tag to hypertable (TimescaleDB extension)
-- Chunks data by time for efficient time-series queries
SELECT create_hypertable('tag', 'time', if_not_exists => TRUE);

-- Tag_string hypertable: Text/string time-series data
-- Stores status messages, error codes, operation modes, etc.
CREATE TABLE IF NOT EXISTS tag_string (
    time TIMESTAMPTZ NOT NULL,
    asset_id INTEGER NOT NULL REFERENCES asset(id) ON DELETE CASCADE,
    tag_name VARCHAR(255) NOT NULL,
    value TEXT,
    origin VARCHAR(255)
);

-- Convert tag_string to hypertable
SELECT create_hypertable('tag_string', 'time', if_not_exists => TRUE);

-- ==============================================================================
-- Create Indexes
-- ==============================================================================
-- Optimize common query patterns:
-- 1. Query by asset_id + tag_name + time range (most common)
-- 2. Query by asset_id + time range (all tags for an asset)
-- 3. Query by tag_name + time range (same tag across assets)

-- Indexes for tag table
CREATE INDEX IF NOT EXISTS idx_tag_asset_id ON tag (asset_id, time DESC);
CREATE INDEX IF NOT EXISTS idx_tag_asset_tag_time ON tag (asset_id, tag_name, time DESC);
CREATE INDEX IF NOT EXISTS idx_tag_tag_name_time ON tag (tag_name, time DESC);

-- Indexes for tag_string table
CREATE INDEX IF NOT EXISTS idx_tag_string_asset_id ON tag_string (asset_id, time DESC);
CREATE INDEX IF NOT EXISTS idx_tag_string_asset_tag_time ON tag_string (asset_id, tag_name, time DESC);
CREATE INDEX IF NOT EXISTS idx_tag_string_tag_name_time ON tag_string (tag_name, time DESC);

-- ==============================================================================
-- Grant Permissions
-- ==============================================================================

-- Writer permissions (kafkatopostgresqlv2)
-- Needs to INSERT data and SELECT to resolve asset_id
GRANT CONNECT ON DATABASE umh_v2 TO kafkatopostgresqlv2;
GRANT USAGE ON SCHEMA public TO kafkatopostgresqlv2;
GRANT SELECT, INSERT, UPDATE ON asset TO kafkatopostgresqlv2;
GRANT SELECT, INSERT ON tag TO kafkatopostgresqlv2;
GRANT SELECT, INSERT ON tag_string TO kafkatopostgresqlv2;
GRANT USAGE, SELECT ON SEQUENCE asset_id_seq TO kafkatopostgresqlv2;

-- Reader permissions (grafanareader)
-- Read-only access for dashboards and queries
GRANT CONNECT ON DATABASE umh_v2 TO grafanareader;
GRANT USAGE ON SCHEMA public TO grafanareader;
GRANT SELECT ON asset TO grafanareader;
GRANT SELECT ON tag TO grafanareader;
GRANT SELECT ON tag_string TO grafanareader;

-- ==============================================================================
-- Compression Policy
-- ==============================================================================
-- Automatically compress old data to save disk space
-- Compression happens after 7 days (data older than 7 days is compressed)
-- Compressed data is still queryable, just stored more efficiently

-- Enable compression on tag hypertable
ALTER TABLE tag SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'asset_id,tag_name',
    timescaledb.compress_orderby = 'time DESC'
);

-- Enable compression on tag_string hypertable
ALTER TABLE tag_string SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'asset_id,tag_name',
    timescaledb.compress_orderby = 'time DESC'
);

-- Add compression policy (compress chunks older than 7 days)
SELECT add_compression_policy('tag', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_compression_policy('tag_string', INTERVAL '7 days', if_not_exists => TRUE);

-- ==============================================================================
-- Retention Policy (Optional)
-- ==============================================================================
-- Uncomment to automatically delete data older than specified interval
-- WARNING: This permanently deletes data. Ensure backups are in place.

-- Delete data older than 1 year:
-- SELECT add_retention_policy('tag', INTERVAL '1 year', if_not_exists => TRUE);
-- SELECT add_retention_policy('tag_string', INTERVAL '1 year', if_not_exists => TRUE);

-- ==============================================================================
-- Continuous Aggregates (Optional)
-- ==============================================================================
-- Pre-compute aggregations for faster dashboards
-- Uncomment to enable hourly aggregates (useful for long-term trending)

-- Example: Hourly average for numeric tags
-- CREATE MATERIALIZED VIEW IF NOT EXISTS tag_hourly
-- WITH (timescaledb.continuous) AS
-- SELECT
--     time_bucket('1 hour', time) AS bucket,
--     asset_id,
--     tag_name,
--     AVG(value) AS avg_value,
--     MAX(value) AS max_value,
--     MIN(value) AS min_value,
--     COUNT(*) AS sample_count
-- FROM tag
-- GROUP BY bucket, asset_id, tag_name
-- WITH NO DATA;

-- Refresh policy: Update hourly aggregates automatically
-- SELECT add_continuous_aggregate_policy('tag_hourly',
--     start_offset => INTERVAL '3 hours',
--     end_offset => INTERVAL '1 hour',
--     schedule_interval => INTERVAL '1 hour',
--     if_not_exists => TRUE
-- );

-- ==============================================================================
-- Maintenance Notes
-- ==============================================================================
-- 1. Change default passwords for kafkatopostgresqlv2 and grafanareader
-- 2. Adjust compression policy interval based on query patterns
-- 3. Enable retention policy if data doesn't need to be kept indefinitely
-- 4. Create continuous aggregates for frequently queried time ranges
-- 5. Monitor disk usage: SELECT * FROM timescaledb_information.hypertable;
-- 6. Backup regularly: pg_dump -U postgres umh_v2 | gzip > backup.sql.gz
