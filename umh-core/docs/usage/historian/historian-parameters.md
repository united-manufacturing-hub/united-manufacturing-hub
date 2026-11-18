# Historian parameters

Four parameters decide how the Historian keeps data: **Value Chunk Interval**, **Attribute Chunk
Interval**, **Compress After** and **Retention**, in the Advanced Settings of a Historian bridge. All
four are applied once, when the bridge creates its tables.

## What the parameters control

TimescaleDB stores a table in chunks, each covering one time range; the chunk interval sets its
width. TimescaleDB compresses a chunk once all its rows are older than **Compress After**, and deletes
it once all its rows are older than **Retention**.

## Applied once, when the tables are created

The parameters belong to the tables, not to the bridge. If every config edit rewrote them, an
unrelated edit could change how production history is compressed or delete it. The Historian
therefore sets them when it creates a data contract's tables and leaves them alone afterwards. The
first bridge of a data contract sets them for every later one.

If the configured value differs from the database, the bridge logs a warning and keeps the database
value. The [Monitor page](README.md#monitor-page) shows the values in use.

## Retention applies only to an empty table

Applied to a table with data, a retention would delete every older chunk at once. The Historian
therefore applies it only to a table that holds no data yet. To add a retention later, run the
statement from the bridge's warning; see
[Changing compression or retention](https://docs.umh.app/benthos-umh/output/historian#changing-compression-or-retention).
