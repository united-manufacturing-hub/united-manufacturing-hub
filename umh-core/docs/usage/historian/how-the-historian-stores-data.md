# How the Historian stores data

This page explains how data contract versions map to tables, why the Historian parameters are applied
once, and which values lose precision.

## Versions share one set of tables

Each version of a data model declares its own data contract: version 1 of `pump` declares `_pump_v1`,
version 2 declares `_pump_v2`. The Historian stores all versions in one set of tables,
`umh.value_pump` and `umh.attribute_pump`, so a tag keeps one history across versions and a Grafana
panel does not change when the data model gets a new version. The version is stored with each value
in `attribute->>'data_contract_version'`.

A Historian bridge stores only the versions listed in its source topic, so a new version is stored
once you add it to the bridge; see
[New version not stored](troubleshooting/new-version-not-stored.md).

## Historian parameters

Four parameters decide how the Historian keeps data: **Value Chunk Interval**, **Attribute Chunk
Interval**, **Compress After** and **Retention**, in the Advanced Settings of a Historian bridge. All
four are applied once, when the bridge creates its tables.

### What the parameters control

TimescaleDB stores a table in chunks, each covering one time range; the chunk interval sets its
width. TimescaleDB compresses a chunk once all its rows are older than **Compress After**, and deletes
it once all its rows are older than **Retention**.

### Applied once, when the tables are created

The parameters belong to the tables, not to the bridge. If every config edit rewrote them, an
unrelated edit could change how production history is compressed or delete it. The Historian
therefore sets them when it creates a data contract's tables and leaves them alone afterwards. The
first bridge of a data contract sets them for every later one.

If the configured value differs from the database, the bridge logs a warning and keeps the database
value. The [Monitor page](README.md#monitor-page) shows the values in use. To change a parameter
after the tables exist, change it in the database; see
[Changing compression or retention](https://docs.umh.app/benthos-umh/output/historian#changing-compression-or-retention).

### Retention applies only to an empty table

Applied to a table with data, a retention would delete every older chunk at once. The Historian
therefore applies it only to a table that holds no data yet. To add a retention later, run the
statement from the bridge's warning in the database.

## Precision and datatypes

### Numbers lose precision

Numbers and booleans are stored in `value_num`, a `DOUBLE PRECISION` column, which is exact enough for
sensor readings. Integer counters above 2^53 lose their lowest digits, and decimals such as `0.1` are
stored as the nearest binary fraction. Store such values in a text field of the data contract; they
are then kept as they arrived in `value_text`.

### A tag keeps its first datatype

The first value of a tag sets its datatype, number or text. A later value of the other datatype is
dropped and counted on `historian_rows_poisoned`, so each tag's history stays in one column.

`allow_datatype_changes` lifts this rule for a bridge. The tag then stores both datatypes, and you
read it with `coalesce(value_num::text, value_text)`. See
[Runbook: poisoned tags](https://docs.umh.app/benthos-umh/output/historian#runbook-poisoned-tags).
