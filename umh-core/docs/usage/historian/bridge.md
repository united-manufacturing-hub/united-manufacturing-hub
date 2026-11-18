# Historian bridge

This page describes a Historian bridge: the topics it reads, the fields of its Destination, and the
tables it writes. A Historian bridge reads one data contract from the Unified Namespace and writes it
into TimescaleDB. It has a write flow only, with the Historian as its destination, and no read flow.
The output plugin behind it is described in the
[Historian output reference](https://docs.umh.app/benthos-umh/output/historian).

## Which topics the bridge reads

A Historian bridge reads one data contract at its own location and the levels below it. In the
Management Console, the topic table builds its source topic and picks the latest declared version
of the data contract. The source topic does not follow later versions; see
[Contracts, versions and tables](contracts-versions-and-tables.md).

The source topic follows the topic rules of every write flow, described in
[Topic validation](../data-flows/write-flow.md#topic-validation). A deploy that breaks them fails
with a message that names the topic; see
[Write Flow Topics Rejected](../data-flows/troubleshooting/write-flow-topics-rejected.md).

## Data models with a dash in the name

A Historian bridge stores the data contract picked in its topic table. A data model created before
the Management Console rejected dashes in data model names, such as `pump-a`, cannot be stored: its
name becomes part of the PostgreSQL table names, which allow no dashes. The **Historian** column on
the **Data Models** page shows a dash (—) for such a data model, and its tooltip names the reason.

## Destination fields

The **Destination** of the write flow shows these fields in Visual mode. Defaults and accepted
values of each field are in the output reference's
[Configuration](https://docs.umh.app/benthos-umh/output/historian#configuration) table.

**Standard:**

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

Durations are written in hours, minutes or seconds, such as `168h`. Days are not a unit. See
[Duration values](https://docs.umh.app/benthos-umh/output/historian#duration-values).

### Settings applied once

`compress_after`, `retention`, `value_chunk_interval` and `attribute_chunk_interval` apply when the
bridge creates the tables of its data contract. `retention` applies only before the first value is
written. Advanced Settings shows a notice about this:

| Notice | When |
|---|---|
| **Table settings apply once** | Before the first deployment. The values you set apply when the tables are created. |
| **Table settings are fixed** | After the first deployment, or once the tables exist. The four fields are read-only. |

The [Historian parameters](historian-parameters.md) page explains why.

## What the bridge writes

For `data_contract_name: pump`, the bridge creates and fills two hypertables in the `umh` schema:

| Table | Contents |
|---|---|
| `umh.value_pump` | One row per tag and timestamp. Numbers and booleans in `value_num`, strings and JSON in `value_text`. |
| `umh.attribute_pump` | The message metadata as a JSON object. A new row is written only when the set of metadata keys of a tag changes. |

Tag identity is in the shared tables `umh.topic`, `umh.tag` and `umh.location`. All versions of the
data contract write to the same two tables. See [Database schema](database-schema.md).
