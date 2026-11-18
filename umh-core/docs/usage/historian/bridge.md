# Historian bridge

A Historian bridge has only a write flow. It reads one data contract from the Unified Namespace and
writes it into TimescaleDB. Its output is described in the
[Historian output reference](https://docs.umh.app/benthos-umh/output/historian).

## Which topics the bridge reads

The bridge reads one data contract at its location and the levels below it. The topic table picks the
latest version of the data contract; later versions are not added on their own. The topic follows
the rules in [Topic validation](../data-flows/write-flow.md#topic-validation).

## Data models with a dash in the name

A data model with a dash in its name, such as `pump-a`, cannot be stored, because its name becomes part
of the PostgreSQL table names. Older data models can have one; new names reject dashes.

## Destination fields

Defaults and accepted values are in the output reference's
[Configuration](https://docs.umh.app/benthos-umh/output/historian#configuration) table.

**Standard Settings:**

| Field | Key | Notes |
|---|---|---|
| **Historian Connection** | `host`, `port`, `database`, `username`, `password`, `sslmode` | Read-only. Taken from the instance's [connection](connection.md) through `{{ .historian.timescale.* }}`. |
| **Data Contract** | `data_contract_name` | Read-only. Follows the source topic. |

**Advanced Settings:**

| Field | Key |
|---|---|
| **Allow Datatype Changes** | `allow_datatype_changes` |
| **Store All Metadata Keys** | `metadata_keys_all` |
| **Metadata Keys to Store** | `metadata_keys`, shown when **Store All Metadata Keys** is off |
| **Metadata Keys to Drop** | `metadata_keys_exclude`, shown when **Store All Metadata Keys** is on |
| **Compress After** | `compress_after` |
| **Retention** | `retention` |
| **Value Chunk Interval** | `value_chunk_interval` |
| **Attribute Chunk Interval** | `attribute_chunk_interval` |
| **Max In Flight** | `max_in_flight` |
| **Write Timeout** | `write_timeout` |

Durations take hours, minutes or seconds, such as `168h`, not days.

### Settings applied once

**Compress After**, **Retention** and the two chunk intervals apply only when the bridge creates its
tables, and are read-only after that. Before the first deployment, Advanced Settings shows **Table
settings apply once**; afterwards it shows **Table settings are fixed**. See
[Historian parameters](historian-parameters.md).

## What the bridge writes

For the data contract `pump`, the bridge writes two tables in the `umh` schema:

| Table | Contents |
|---|---|
| `umh.value_pump` | One row per tag and timestamp. Numbers and booleans in `value_num`, strings and JSON in `value_text`. |
| `umh.attribute_pump` | The message metadata as JSON, written when a tag's metadata keys change. |

All versions of the data contract write to these two tables. See [Database schema](database-schema.md).
