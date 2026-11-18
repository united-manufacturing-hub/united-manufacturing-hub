# Historian bridge

This page describes a Historian bridge: the topics it reads, the fields of its Destination, and the
tables it writes. A Historian bridge reads one data contract from the Unified Namespace and writes it
into TimescaleDB. It has a write flow only, with the Historian as its destination, and no read flow.
The output plugin behind it is described in the
[Historian output reference](https://docs.umh.app/benthos-umh/output/historian).

## Which topics the bridge reads

The bridge reads only the topics listed under `topics` in its write flow's source. In the Management
Console, a Historian bridge has one source topic, which the topic table builds for you. A topic the
table cannot build, such as a version range, opens in Code mode, where you write it yourself. A
bridge edited outside the Management Console, for example in `config.yaml`, may have several source
topics.

Each source topic is a regular expression. It must start with `^`, so it matches from the beginning
of the UNS topic name. Anything may follow the data contract, so a source topic selects every topic
that begins with what it matches. umh-core checks the source topics of every write flow. For a
Historian bridge, each source topic:

- starts with `^umh\.v1\.` followed by the bridge location, so the bridge reads data at or beneath
  its own location only.
- names one data contract: `_historian`, or a versioned contract such as `_pump_v1`. Each data model
  you add in the Management Console declares a versioned contract, and each new version of the model
  declares the next one.
- matches only declared versions of that data contract. `_pump_v[1-2]` and `_pump_v(?:1|2)` are
  accepted while `_pump_v1` and `_pump_v2` are declared. `_pump_v\d+` and `_pump_v[0-9]` are
  rejected, because they also match versions that are not declared.

For a bridge at `acme.cologne` that stores `_pump_v1` and `_pump_v2`, at the location or any level
beneath it:

```
^umh\.v1\.acme\.cologne\.(?:[^._][^.]*\.)*_pump_v[1-2]\.
```

You can write `{{ .location_path }}` in place of `acme\.cologne`. umh-core escapes it for you, and
the topic follows the bridge if its location changes. The variable does not end in a dot, so you
still write the `\.` after it:

```
^umh\.v1\.{{ .location_path }}\.(?:[^._][^.]*\.)*_pump_v[1-2]\.
```

For the data contract `historian`, the topic ends in `_historian\.` with no version. To store data
from several sites in one bridge, place the bridge at a location above them, up to the enterprise
itself, such as `acme`.

When you pick a data contract in the topic table, the source topic lists its latest declared
version. It does not follow later versions; see
[Contracts, versions and tables](contracts-versions-and-tables.md).

Deploying or editing a bridge fails with an error that names the topic if a source topic breaks one
of these rules. Examples are a topic outside the bridge location, such as `^umh\.v1\..*`, and one
that names no single data contract, such as `^umh\.v1\.acme\.cologne\..*`. Changing a location in a
way that would break an existing bridge's topics fails the same way.

A bridge whose topics were saved before umh-core 0.45.0 keeps its config, but its write flow does
not start. See [Troubleshoot a Historian bridge](troubleshoot-a-bridge.md#the-bridge-stopped-saving-after-updating-umh-core).

## Data contract name

`data_contract_name` is the bare name of the data contract the bridge stores:

- no leading underscore and no `_vN` suffix: for the UNS contract `_pump_v1`, it is `pump`.
- lowercase letters, digits and underscores only, so no dashes.
- 53 characters or fewer.

The name becomes part of the PostgreSQL table names, which allow no dashes. A data model created
before the Management Console rejected dashes in data model names still declares a contract such as
`_pump-a_v1`. That contract works everywhere else, but a Historian bridge cannot store it.

The Management Console keeps `data_contract_name` equal to the data contract in the source topic,
and shows it read-only. A bridge edited outside the Management Console can name another data
contract in its source topics. Deploying does not check this, and the output then refuses the data;
see [Troubleshoot a Historian bridge](troubleshoot-a-bridge.md#the-bridge-is-running-but-the-data-of-its-data-contract-is-lost).

The [Contracts, versions and tables](contracts-versions-and-tables.md) page explains why the name has
no version and no dashes.

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

The [Settings applied once](settings-applied-once.md) page explains why.

## What the bridge writes

For `data_contract_name: pump`, the bridge creates and fills two hypertables in the `umh` schema:

| Table | Contents |
|---|---|
| `umh.value_pump` | One row per tag and timestamp. Numbers and booleans in `value_num`, strings and JSON in `value_text`. |
| `umh.attribute_pump` | The message metadata as a JSON object. A new row is written only when the set of metadata keys of a tag changes. |

Tag identity is in the shared tables `umh.topic`, `umh.tag` and `umh.location`. All versions of the
data contract write to the same two tables. See [Database schema](database-schema.md).
