# Settings applied once

This page explains why four settings of a Historian bridge take effect only when the bridge creates
its tables, and why retention is stricter than the other three. The settings are `compress_after`,
`retention`, `value_chunk_interval` and `attribute_chunk_interval`, shown as **Compress After**,
**Retention**, **Value Chunk Interval** and **Attribute Chunk Interval** in the bridge's Advanced
Settings.

## What the settings control

TimescaleDB stores a table as chunks. Each chunk holds the rows of one time range. The chunk interval
sets how wide that range is. Compression and retention work on whole chunks: TimescaleDB compresses
a chunk once all its rows are older than `compress_after`, and deletes it once all its rows are older
than `retention`.

All three are properties of the tables in the database, not of the bridge. Compression and retention
are TimescaleDB policies on each table. The chunk interval is set when the table is created.

## Why a config edit does not change them

A Historian bridge's config can change at any time: someone edits it, the bridge is redeployed, a
template update rewrites it. If each change rewrote the table policies, an edit made for an unrelated
reason could change how production history is compressed, or delete it. The Historian therefore
applies the four settings when it creates the tables of a data contract, and leaves them alone after
that. The Management Console shows them read-only from then on.

When the configured value differs from the one in the database, the bridge logs a warning on start
and keeps the value in the database. The Monitor page shows the values the database holds; see
[Historian: Monitor page](README.md#monitor-page).

## Why retention applies only to an empty table

Retention deletes data. A retention applied to a table that already holds data deletes every chunk
older than the retention at once. A bridge that restarts with a retention set months ago could then
delete history that someone kept on purpose.

The Historian therefore applies a retention only to a table that holds no data yet. In practice,
retention is set before the first value of the data contract is written, or not at all. On a table
that already holds data, the bridge logs a warning that names the statement to apply the retention by
hand. The output reference describes this in
[Changing compression or retention](https://docs.umh.app/benthos-umh/output/historian#changing-compression-or-retention).

## Shared by all bridges of a data contract

The settings belong to the tables, and all versions of a data contract share one set of tables; see
[Contracts, versions and tables](contracts-versions-and-tables.md). The first bridge that writes a data
contract sets them for every later bridge that writes the same data contract.
