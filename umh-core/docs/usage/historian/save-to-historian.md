# Save data to the Historian

A Historian bridge subscribes to the Unified Namespace and writes one data contract into
TimescaleDB. You create one bridge per contract you want to keep.

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
   **without the version suffix**: `pump`, not `pump_v1`.
5. Deploy.

There is no read flow to configure and no address table to map, so the protocol and address steps of
the general [bridge walkthrough](../data-flows/bridges.md) don't apply.

{% hint style="warning" %}
**Drop the version, and no dashes.** `data_contract_name` is the bare contract name: no leading
underscore, no `_vN` suffix. For the UNS contract `_pump_v1`, write `pump`.

A contract whose name contains a dash cannot be stored by the Historian at all. The name becomes a
PostgreSQL table identifier, which allows only lowercase letters, numbers, and underscores.
{% endhint %}

The only value you normally change is `data_contract_name`. Credentials, database, and TLS mode come
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

The bridge subscribes to the whole UNS by default and drops messages whose contract doesn't match
`data_contract_name`, so there is no topic regex to keep in sync. Narrow the source topics only if
you want to stop unrelated messages from reaching the output at all.

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

Two of them behave differently from the rest:

- `compress_after` and `retention` are applied **once, when the tables are first created**. Editing
  them afterwards has no effect on an existing database. A config edit must not silently change how
  production history is compressed or deleted. To change them, update the TimescaleDB policies
  directly on both hypertables, then set the same value in the bridge config so the drift warning
  stops.

## Troubleshooting

**The bridge fails immediately after deploying.** The output verifies the connection at startup
rather than failing later on the first write. It fails fast on an unreachable host, a PostgreSQL
older than 16, a missing `timescaledb` or `ltree` extension, and a role that can connect but cannot
`INSERT` into the contract's tables. The error names which one. Check the plugin overview card in
the Plugins tab first: if **Auth** is not valid there, fix the connection before looking at the
bridge.

**The bridge is running but nothing arrives.** The output logs once when it stores its first
message, and once if data is flowing but none of it matches `data_contract_name`. A mismatch between
the contract in your topics and the configured name is the usual cause.

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
