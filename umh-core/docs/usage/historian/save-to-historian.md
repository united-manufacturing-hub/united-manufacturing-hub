# Save data to the Historian

A Historian bridge reads one data contract from the Unified Namespace, beneath its own location,
and writes it into TimescaleDB. You create one bridge per contract you want to keep.

The bridge is a write flow only: it has no read flow, because its source is the UNS rather than a
device.

## Before you start

Set up the instance's [Historian connection](README.md#set-up-the-connection) first. Without it, the
bridge has nothing to connect to and deployment fails with an error saying so.

## Create the bridge

1. Go to **Data Flows** → **Add Bridge**, and choose **From Scratch**.
2. In the template list, pick **TimescaleDB (PostgreSQL, auto-connect) — Historian**. The **Vendor**
   filter narrows the list to the TimescaleDB templates.
3. Select the instance. The connection host and port are prefilled read-only from that instance's
   Historian connection, so the bridge's health check always targets the configured database.
4. In the write flow's output, set `data_contract_name` to the contract you want to store, written
   **without the version suffix**: `pump`, not `pump_v1`. The source topics must name the same
   contract, see [Which topics the bridge reads](#which-topics-the-bridge-reads).
5. Deploy.

There is no read flow to configure and no address table to map, so the protocol and address steps of
the general [bridge walkthrough](../data-flows/bridges.md) don't apply.

{% hint style="warning" %}
**Drop the version, and no dashes.** `data_contract_name` is the bare contract name: no leading
underscore, no `_vN` suffix. For the UNS contract `_pump_v1`, write `pump`.

A contract whose name contains a dash cannot be stored by the Historian at all. The name becomes a
PostgreSQL table identifier, which allows only lowercase letters, numbers, and underscores.
{% endhint %}

The only values you normally change are `data_contract_name` and the source topics that name the
same contract. Credentials, database, and TLS mode come
from the shared connection through `{{ .historian.timescale.* }}`, so they are never entered twice
and follow the connection if you later change it.

### Naming the data contract

Every version of a contract shares one set of tables, so `pump` stores `_pump_v1` and `_pump_v2`
alike. That is why the version is left off: the tables hold the contract, not one version of it.

The Management Console rejects dashes in new data model names for the table-identifier reason above.
Models created before that check keep working everywhere else, but a Historian bridge cannot store
them under their own name.

The default is `historian`, the generic UNS time-series contract.

### What the bridge writes

For `data_contract_name: pump`, the bridge creates and fills two hypertables in a dedicated `umh`
schema:

| Table | Contents |
|---|---|
| `umh.value_pump` | One row per (tag, timestamp). Numbers and booleans in `value_num`, strings and JSON in `value_text`. |
| `umh.attribute_pump` | The message metadata as a JSON object, rewritten only when the key set changes. |

Tag identity lives in the shared dimension tables `umh.topic`, `umh.tag`, and `umh.location`. See
[Query the Historian](querying.md) for the layout and how to read it back.

### Which topics the bridge reads

The bridge reads only the source topics of its write flow, and umh-core checks them for every write
flow. Each source topic:

- starts with `^umh\.v1\.` followed by the bridge location, so the bridge reads data at or beneath
  its own location only.
- names one data contract: `_historian`, or a versioned one declared under `dataContracts:`, such
  as `_pump_v1`.
- lists the versions of a versioned contract, such as `_pump_v[1-2]`, never `_pump_v\d+`.

For a bridge at `acme.cologne` that stores `_pump_v1` and `_pump_v2`, at any level beneath the
location:

```
^umh\.v1\.acme\.cologne\.(?:[^._][^.]*\.)*_pump_v[1-2]\.
```

You can write `{{ .location_path }}` in place of `acme\.cologne`. umh-core escapes it for you, and
the topic follows the bridge if its location changes.

For the default contract `historian`, the topic ends in `_historian\.` with no version. To store
data from several sites in one bridge, place the bridge at a location above them.

A new bridge reads the latest declared version of the contract. The bridge's tooltip in the data
model view shows which versions it saves. To also save another version, add it to the source topics.

A topic that breaks a rule is, for example, one that covers the whole UNS, another location, or
every contract of a location. Deploying or editing a bridge with such a topic fails with an error
that names it, and so does changing a location in a way that would break an existing bridge's
topics.

A bridge whose topics were saved before these rules existed keeps its config, but its write flow
does not start. The bridge shows degraded, and its status names each rejected topic, why it is
rejected, and an example topic. That example names the first declared versioned contract, which may
not be the one this bridge stores, and matches only data published at the bridge location itself.
Use the pattern above instead.

The source topics must name the contract in `data_contract_name`. The output refuses a batch that
holds any message of another contract and retries it, so the write flow stores nothing until the two
match.

## Connecting to a different database

The **TimescaleDB (PostgreSQL) — Historian** template is the same output without the shared
connection: you enter host and port in the connection step and set the username, password, and SSL
mode in the output config yourself. Use it for a database that isn't the instance's Historian, such
as a second archive or a customer-managed database with its own role.

Everything below applies to both templates; only where the connection details come from differs.

## Advanced options

The output supports retention, compression, metadata filtering, batching, and timeouts. They are
plain YAML fields you add to the output config in the bridge editor. See the
[Historian output reference](https://docs.umh.app/benthos-umh/output/historian) for the full field
list and defaults.

`compress_after` and `retention` are the exceptions. Both are applied once, when the tables are
first created, so editing them later has no effect on an existing database: a config edit must not
silently change how production history is compressed or deleted. To change them, update the
TimescaleDB policies directly on both hypertables, then set the same value in the bridge config so
the drift warning stops.

## Troubleshooting

**The bridge fails immediately after deploying.** The output verifies the connection at startup
rather than failing later on the first write. It fails fast on an unreachable host, a PostgreSQL
older than 16, a missing `timescaledb` or `ltree` extension, and a role that can connect but cannot
`INSERT` into the contract's tables. The error names which one. Check the plugin overview card in
the Plugins tab first: if **Auth** is not valid there, fix the connection before looking at the
bridge.

**The bridge stopped saving after an update to umh-core 0.45.0.** Historian bridges created before
that release read the whole UNS, so their write flow does not start. Change their source topics as
in [Which topics the bridge reads](#which-topics-the-bridge-reads).

**The bridge is running but nothing arrives.** The output logs once when it stores its first
message. If the contract in the source topics differs from `data_contract_name`, the output refuses
every batch and logs an error at most every two minutes that names the contracts that arrived. Set
`data_contract_name` to the contract in the source topics, without the version. The error also
suggests a topic pattern; umh-core rejects that pattern, so use the one in
[Which topics the bridge reads](#which-topics-the-bridge-reads) instead.

If the output logs nothing at all, no data of the versions in the source topics is published beneath
the bridge location. Check the Topic Browser: devices may publish a newer version, such as
`_pump_v3`, that the source topics don't list yet.

**Rows are being dropped.** Two cases can never be written and are dropped rather than retried: a
tag whose datatype flips between numeric and text, and two different values at the same millisecond.
Both are logged with the tag name and counted on the `historian_rows_poisoned` metric. The
[Historian output reference](https://docs.umh.app/benthos-umh/output/historian) has a runbook for
resolving them.

Connection loss, deadlocks, a missing grant, and a full disk are all retried rather than
dropped. A database restart mid-stream loses nothing: held messages replay, and an identical value
at the same timestamp is absorbed.

## Next steps

- [Query the Historian](querying.md)
