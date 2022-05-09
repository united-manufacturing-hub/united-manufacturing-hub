-- Required for docker-compose
    CREATE USER factoryinsight_user;
    CREATE DATABASE factoryinsight_database OWNER factoryinsight_user;
    GRANT ALL PRIVILEGES ON DATABASE factoryinsight_database TO factoryinsight_user;
    ALTER USER factoryinsight_user WITH PASSWORD 'factoryinsight';


    -- Extend the database with TimescaleDB
    CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE;

\c factoryinsight_database

    -------------------- Table with all assets --------------------
CREATE TABLE IF NOT EXISTS assetTable
(
    id SERIAL PRIMARY KEY,
    assetID VARCHAR(20) NOT NULL,
    location VARCHAR(20) NOT NULL,
    customer VARCHAR(20) NOT NULL,
    unique (assetID, location, customer)
    );

-------------------- TimescaleDB table for states --------------------
CREATE TABLE IF NOT EXISTS stateTable
(
    timestamp                TIMESTAMPTZ                         NOT NULL,
    asset_id            SERIAL REFERENCES assetTable (id),
    state INTEGER CHECK (state >= 0),
    UNIQUE(timestamp, asset_id)
    );
-- creating hypertable
SELECT create_hypertable('stateTable', 'timestamp');

-- creating an index to increase performance
CREATE INDEX ON stateTable (asset_id, timestamp DESC);

-------------------- TimescaleDB table for counts --------------------
CREATE TABLE IF NOT EXISTS countTable
(
    timestamp                TIMESTAMPTZ                         NOT NULL,
    asset_id            SERIAL REFERENCES assetTable (id),
    count INTEGER CHECK (count > 0),
    UNIQUE(timestamp, asset_id)
    );
-- creating hypertable
SELECT create_hypertable('countTable', 'timestamp');

-- creating an index to increase performance
CREATE INDEX ON countTable (asset_id, timestamp DESC);

-------------------- TimescaleDB table for process values --------------------
CREATE TABLE IF NOT EXISTS processValueTable
(
    timestamp               TIMESTAMPTZ                         NOT NULL,
    asset_id                SERIAL                              REFERENCES assetTable (id),
    valueName               TEXT                                NOT NULL,
    value                   DOUBLE PRECISION                    NULL,
    UNIQUE(timestamp, asset_id, valueName)
    );
-- creating hypertable
SELECT create_hypertable('processValueTable', 'timestamp');

-- creating an index to increase performance
CREATE INDEX ON processValueTable (asset_id, timestamp DESC);

-- creating an index to increase performance
CREATE INDEX ON processValueTable (valueName);

-------------------- TimescaleDB table for uniqueProduct --------------------
CREATE TABLE IF NOT EXISTS uniqueProductTable
(
    uid TEXT NOT NULL,
    asset_id            SERIAL REFERENCES assetTable (id),
    begin_timestamp_ms                TIMESTAMPTZ NOT NULL,
    end_timestamp_ms                TIMESTAMPTZ                         NOT NULL,
    product_id TEXT NOT NULL,
    is_scrap BOOLEAN NOT NULL,
    quality_class TEXT NOT NULL,
    station_id TEXT NOT NULL,
    UNIQUE(uid, asset_id, station_id),
    CHECK (begin_timestamp_ms < end_timestamp_ms)
    );

-- creating an index to increase performance
CREATE INDEX ON uniqueProductTable (asset_id, uid, station_id);


-------------------- TimescaleDB table for recommendations  --------------------
CREATE TABLE IF NOT EXISTS recommendationTable
(
    uid                     TEXT                                PRIMARY KEY,
    timestamp               TIMESTAMPTZ                         NOT NULL,
    recommendationType      INTEGER                             NOT NULL,
    enabled                 BOOLEAN                             NOT NULL,
    recommendationValues    TEXT,
    diagnoseTextDE          TEXT,
    diagnoseTextEN          TEXT,
    recommendationTextDE    TEXT,
    recommendationTextEN    TEXT
);

-------------------- TimescaleDB table for shifts --------------------
-- Using btree_gist to avoid overlapping shifts
-- Source: https://gist.github.com/fphilipe/0a2a3d50a9f3834683bf
CREATE EXTENSION btree_gist;
CREATE TABLE IF NOT EXISTS shiftTable
(
    id SERIAL PRIMARY KEY,
    type INTEGER,
    begin_timestamp TIMESTAMPTZ NOT NULL,
    end_timestamp TIMESTAMPTZ,
    asset_id SERIAL REFERENCES assetTable (id),
    unique (begin_timestamp, asset_id),
    CHECK (begin_timestamp < end_timestamp),
    EXCLUDE USING gist (asset_id WITH =, tstzrange(begin_timestamp, end_timestamp) WITH &&)
    );


-------------------- TimescaleDB table for products --------------------
CREATE TABLE IF NOT EXISTS productTable
(
    product_id SERIAL PRIMARY KEY,
    product_name VARCHAR(40) NOT NULL,
    asset_id SERIAL REFERENCES assetTable (id),
    time_per_unit_in_seconds REAL NOT NULL,
    UNIQUE(product_name, asset_id),
    CHECK (time_per_unit_in_seconds > 0)
    );

-------------------- TimescaleDB table for orders --------------------
CREATE TABLE IF NOT EXISTS orderTable
(
    order_id SERIAL PRIMARY KEY,
    order_name VARCHAR(40) NOT NULL,
    product_id SERIAL REFERENCES productTable (product_id),
    begin_timestamp TIMESTAMPTZ,
    end_timestamp TIMESTAMPTZ,
    target_units INTEGER,
    asset_id SERIAL REFERENCES assetTable (id),
    unique (asset_id, order_id),
    CHECK (begin_timestamp < end_timestamp),
    CHECK (target_units > 0),
    EXCLUDE USING gist (asset_id WITH =, tstzrange(begin_timestamp, end_timestamp) WITH &&) WHERE (begin_timestamp IS NOT NULL AND end_timestamp IS NOT NULL)
    );

-------------------- TimescaleDB table for configuration --------------------
CREATE TABLE IF NOT EXISTS configurationTable
(
    customer VARCHAR(20) PRIMARY KEY,
    MicrostopDurationInSeconds INTEGER DEFAULT 60*2,
    IgnoreMicrostopUnderThisDurationInSeconds INTEGER DEFAULT -1, --do not apply
    MinimumRunningTimeInSeconds INTEGER DEFAULT 0, --do not apply
    ThresholdForNoShiftsConsideredBreakInSeconds INTEGER DEFAULT 60*35,
    LowSpeedThresholdInPcsPerHour INTEGER DEFAULT -1, --do not apply
    AutomaticallyIdentifyChangeovers BOOLEAN DEFAULT true,
    LanguageCode INTEGER DEFAULT 1, -- english
    AvailabilityLossStates INTEGER[] DEFAULT '{40000, 180000, 190000, 200000, 210000, 220000}',
    PerformanceLossStates INTEGER[] DEFAULT '{20000, 50000, 60000, 70000, 80000, 90000, 100000, 110000, 120000, 130000, 140000, 150000}'
    );


--- MIGRATIONS
-- Add here your migrations

-- #2
ALTER TABLE configurationTable ADD IF NOT EXISTS AutomaticallyIdentifyChangeovers BOOLEAN DEFAULT true;

-- #74
ALTER TABLE counttable ADD IF NOT EXISTS scrap INTEGER DEFAULT 0;

-- #297
ALTER TABLE uniqueProductTable ADD COLUMN IF NOT EXISTS uniqueProductAlternativeID TEXT NOT NULL DEFAULT 'beforeMigration';
ALTER TABLE uniqueProductTable DROP COLUMN IF EXISTS quality_class RESTRICT;
ALTER TABLE uniqueProductTable DROP COLUMN IF EXISTS station_id RESTRICT;
ALTER TABLE uniqueProductTable DROP COLUMN IF EXISTS uid CASCADE;
ALTER TABLE uniqueProductTable ADD COLUMN IF NOT EXISTS uniqueProductID SERIAL PRIMARY KEY;
ALTER TABLE uniqueProductTable DROP CONSTRAINT IF EXISTS uniqueproducttable_check;
ALTER TABLE uniqueProductTable ALTER COLUMN end_timestamp_ms DROP NOT NULL;
ALTER TABLE uniqueProductTable ALTER COLUMN product_id SET DATA TYPE INTEGER USING (product_id::integer);
ALTER TABLE uniqueProductTable ADD CONSTRAINT productID_reference FOREIGN KEY (product_id) REFERENCES productTable (product_id);
ALTER TABLE uniqueProductTable ALTER COLUMN  end_timestamp_ms DROP NOT NULL;
ALTER TABLE uniqueProductTable ADD CONSTRAINT checkTimestampOrder CHECK ((begin_timestamp_ms < end_timestamp_ms) OR (end_timestamp_ms IS NULL));

-------------------- TimescaleDB table for product tags --------------------
CREATE TABLE IF NOT EXISTS productTagTable
(
    timestamp               TIMESTAMPTZ                         NOT NULL,
    product_uid             SERIAL                              REFERENCES uniqueProductTable (uniqueProductID),
    valueName               TEXT                                NOT NULL,
    value                   DOUBLE PRECISION                    NOT NULL,
    UNIQUE(product_uid, valueName)
    );
-- creating hypertable
SELECT create_hypertable('productTagTable', 'product_uid', chunk_time_interval => 100000);

-------------------- TimescaleDB table for product tags strings --------------------
CREATE TABLE IF NOT EXISTS productTagStringTable
(
    timestamp               TIMESTAMPTZ                         NOT NULL,
    product_uid             SERIAL                              REFERENCES uniqueProductTable (uniqueProductID),
    valueName               TEXT                                NOT NULL,
    value                   TEXT                                NOT NULL,
    UNIQUE(product_uid, valueName)
    );
-- creating hypertable
SELECT create_hypertable('productTagStringTable', 'product_uid', chunk_time_interval => 100000);

-------------------- TimescaleDB table for product inheritance info --------------------
CREATE TABLE IF NOT EXISTS productInheritanceTable
(
    timestamp               TIMESTAMPTZ                         NOT NULL,
    parent_uid              SERIAL                              REFERENCES uniqueProductTable (uniqueProductID),
    child_uid               SERIAL                              REFERENCES uniqueProductTable (uniqueProductID),
    UNIQUE(parent_uid, child_uid)
    );
-- creating hypertable
CREATE INDEX ON productInheritanceTable (parent_uid, timestamp DESC);
CREATE INDEX ON productInheritanceTable (child_uid, timestamp DESC);

-------------------- TimescaleDB table for process values string --------------------
CREATE TABLE IF NOT EXISTS processValueStringTable
(
    timestamp               TIMESTAMPTZ                         NOT NULL,
    asset_id                SERIAL                              REFERENCES assetTable (id),
    valueName               TEXT                                NOT NULL,
    value                   TEXT                                NOT NULL,
    UNIQUE(timestamp, asset_id, valueName)
    );
-- creating hypertable
SELECT create_hypertable('processValueStringTable', 'timestamp');
-- creating an index to increase performance
CREATE INDEX ON processValueStringTable (asset_id, timestamp DESC);

-- creating an index to increase performance
CREATE INDEX ON processValueStringTable (valueName);

-------------------- TimescaleDB table for components --------------------

CREATE TABLE IF NOT EXISTS componenttable
(
    id                          SERIAL                              PRIMARY KEY ,
    asset_id                    SERIAL,
    componentname               varchar(20)                         NOT NULL,
    CONSTRAINT componenttable_asset_id_componentname_key UNIQUE (asset_id, componentname),
    CONSTRAINT componenttable_asset_id_fkey FOREIGN KEY (asset_id)
    REFERENCES assettable (id) MATCH SIMPLE
    ON UPDATE CASCADE
    ON DELETE CASCADE
    );

-------------------- TimescaleDB table for maintenance activities --------------------
CREATE TABLE IF NOT EXISTS maintenanceactivities
(
    id                        SERIAL                              PRIMARY KEY,
    component_id              SERIAL,
    activitytype              integer,
    "timestamp"               TIMESTAMPTZ                         NOT NULL,
    CONSTRAINT maintenanceactivities_component_id_activitytype_timestamp_key UNIQUE (component_id, activitytype, "timestamp"),
    CONSTRAINT maintenanceactivities_component_id_fkey FOREIGN KEY (component_id)
    REFERENCES componenttable (id) MATCH SIMPLE
    ON UPDATE CASCADE
    ON DELETE CASCADE
    );

-------------------- TimescaleDB table for time based maintenance --------------------
CREATE TABLE IF NOT EXISTS timebasedmaintenance
(
    id                          SERIAL                              PRIMARY KEY ,
    component_id                SERIAL,
    activitytype integer,
    intervallinhours integer,
    CONSTRAINT timebasedmaintenance_component_id_activitytype_key UNIQUE (component_id, activitytype),
    CONSTRAINT timebasedmaintenance_component_id_fkey FOREIGN KEY (component_id)
    REFERENCES componenttable (id) MATCH SIMPLE
    ON UPDATE CASCADE
    ON DELETE CASCADE
    );


GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO factoryinsight_user;
    GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO factoryinsight_user;

    -- TEST DATA
    -- assetTable
INSERT INTO assetTable(assetID,location,customer) VALUES ('testMachine', 'testLocation', 'factoryinsight');

-- productTable
INSERT INTO productTable(product_id,product_name,asset_id,time_per_unit_in_seconds) VALUES (1,'product107782',1,600);
INSERT INTO productTable(product_id,product_name,asset_id,time_per_unit_in_seconds) VALUES (2,'product107118',1,2946);
INSERT INTO productTable(product_id,product_name,asset_id,time_per_unit_in_seconds) VALUES (3,'product107793',1,7290);
INSERT INTO productTable(product_id,product_name,asset_id,time_per_unit_in_seconds) VALUES (4,'product107791',1,16872);
INSERT INTO productTable(product_id,product_name,asset_id,time_per_unit_in_seconds) VALUES (5,'product107119',1,1908);
INSERT INTO productTable(product_id,product_name,asset_id,time_per_unit_in_seconds) VALUES (6,'product107117',1,2946);
INSERT INTO productTable(product_id,product_name,asset_id,time_per_unit_in_seconds) VALUES (7,'product107829',1,478);
INSERT INTO productTable(product_id,product_name,asset_id,time_per_unit_in_seconds) VALUES (8,'product107823',1,2544);
INSERT INTO productTable(product_id,product_name,asset_id,time_per_unit_in_seconds) VALUES (9,'product107765',1,4098);
INSERT INTO productTable(product_id,product_name,asset_id,time_per_unit_in_seconds) VALUES (10,'product107796',1,6576);
INSERT INTO productTable(product_id,product_name,asset_id,time_per_unit_in_seconds) VALUES (11,'product107792',1,5112);
INSERT INTO productTable(product_id,product_name,asset_id,time_per_unit_in_seconds) VALUES (12,'product107799',1,4008);

-- orderTable
INSERT INTO orderTable(order_name,product_id,target_units,asset_id,begin_timestamp,end_timestamp) VALUES (107117,6,1,1,'2020-11-23 00:05:43+00','2020-11-23 01:25:11+00');
INSERT INTO orderTable(order_name,product_id,target_units,asset_id,begin_timestamp,end_timestamp) VALUES (107118,2,1,1,'2020-11-23 01:25:30+00','2020-11-23 03:06:11+00');
INSERT INTO orderTable(order_name,product_id,target_units,asset_id,begin_timestamp,end_timestamp) VALUES (107119,5,1,1,'2020-11-25 00:52:59+00','2020-11-25 01:22:29+00');
INSERT INTO orderTable(order_name,product_id,target_units,asset_id,begin_timestamp,end_timestamp) VALUES (107765,9,1,1,'2020-11-26 00:55:42+00','2020-11-26 02:31:36+00');
INSERT INTO orderTable(order_name,product_id,target_units,asset_id,begin_timestamp,end_timestamp) VALUES (107782,1,1,1,'2020-11-25 21:43:12+00','2020-11-26 00:02:24+00');
INSERT INTO orderTable(order_name,product_id,target_units,asset_id,begin_timestamp,end_timestamp) VALUES (107791,4,1,1,'2020-11-26 16:23:47+00','2020-11-26 18:51:02+00');
INSERT INTO orderTable(order_name,product_id,target_units,asset_id,begin_timestamp,end_timestamp) VALUES (107792,11,1,1,'2020-11-24 18:42:01+00','2020-11-24 21:26:21+00');
INSERT INTO orderTable(order_name,product_id,target_units,asset_id,begin_timestamp,end_timestamp) VALUES (107793,3,1,1,'2020-11-24 22:11:28+00','2020-11-25 00:52:45+00');
INSERT INTO orderTable(order_name,product_id,target_units,asset_id,begin_timestamp,end_timestamp) VALUES (107796,10,1,1,'2020-11-25 01:53:09+00','2020-11-25 04:37:09+00');
INSERT INTO orderTable(order_name,product_id,target_units,asset_id,begin_timestamp,end_timestamp) VALUES (107799,12,1,1,'2020-11-26 03:40:37+00','2020-11-26 05:06:48+00');
INSERT INTO orderTable(order_name,product_id,target_units,asset_id,begin_timestamp,end_timestamp) VALUES (107823,8,1,1,'2020-11-26 02:31:52+00','2020-11-26 03:11:37+00');
INSERT INTO orderTable(order_name,product_id,target_units,asset_id,begin_timestamp,end_timestamp) VALUES (107829,7,1,1,'2020-11-25 18:51:15+00','2020-11-25 19:24:43+00');

-- productTable
INSERT INTO shiftTable(type, begin_timestamp, end_timestamp, asset_id) VALUES (1,'2020-11-25 05:00:00+00','2020-11-26 05:00:00+00',1);
INSERT INTO shiftTable(type, begin_timestamp, end_timestamp, asset_id) VALUES (1,'2020-11-27 05:00:00+00','2020-11-27 21:00:00+00',1);
INSERT INTO shiftTable(type, begin_timestamp, end_timestamp, asset_id) VALUES (1,'2020-11-28 05:00:00+00','2020-11-28 12:00:00+00',1);
INSERT INTO shiftTable(type, begin_timestamp, end_timestamp, asset_id) VALUES (1,'2020-11-26 05:00:00+00','2020-11-27 05:00:00+00',1);
INSERT INTO shiftTable(type, begin_timestamp, end_timestamp, asset_id) VALUES (1,'2020-11-24 05:00:00+00','2020-11-25 05:00:00+00',1);

-- stateTable
INSERT INTO stateTable(timestamp,asset_id,state) VALUES ('2020-11-25 23:59:01+00',1,40000);
INSERT INTO stateTable(timestamp,asset_id,state) VALUES ('2020-11-25 23:59:00+00',1,10000);
INSERT INTO stateTable(timestamp,asset_id,state) VALUES ('2020-11-25 23:58:59+00',1,40000);
INSERT INTO stateTable(timestamp,asset_id,state) VALUES ('2020-11-25 23:58:58+00',1,10000);

GRANT ALL ON ALL TABLES IN SCHEMA public TO factoryinsight_user;