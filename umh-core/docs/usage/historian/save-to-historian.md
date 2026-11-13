# Save data to the Historian

A Historian bridge reads one data contract from the Unified Namespace, at or beneath its own
location, and writes it into TimescaleDB. You create one bridge per contract you want to keep.

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
4. In the write flow's source, pick the contract you want to store from the topic table. The
   Management Console selects its latest declared version and writes the contract into the output's
   `data_contract_name`, which is read-only. Raw data can't be picked: the Historian stores only
   data contracts.
5. Deploy.

**Save to Historian** on the data models page creates the same bridge for a data model, with its
latest version.

There is no read flow to configure and no address table to map, so the protocol and address steps of
the general [bridge walkthrough](../data-flows/bridges.md) don't apply.

{% hint style="warning" %}
**No dashes in the contract name.** `data_contract_name` is the bare contract name: no leading
underscore, no `_vN` suffix. For the UNS contract `_pump_v1`, it is `pump`.

A contract whose name contains a dash cannot be stored by the Historian at all. The name becomes a
PostgreSQL table identifier, which allows only lowercase letters, numbers, and underscores.
{% endhint %}

The only value you normally change is the source topic; `data_contract_name` follows it.
Credentials, database, and TLS mode come from the Historian connection through
`{{ .historian.timescale.* }}`, so they are never entered twice and follow the connection if you
later change it.

### Naming the data contract

Every version of a contract shares one set of tables, so `pump` stores `_pump_v1` and `_pump_v2`
alike. That is why the version is left off: the tables hold the contract, not one version of it.

The Management Console rejects dashes in new data model names for the table-identifier reason above.
A data model created before that check still declares a contract with a dash in its name, such as
`_pump-a_v1`. That contract works everywhere else, but a Historian bridge cannot store it.

`historian` is the generic UNS time-series contract.

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

The bridge reads only the topics listed under `topics` in its write flow's source. In the Management
Console, a Historian bridge has one source topic, which the topic table builds for you. A topic the
table can't build, such as the version range below, opens in Code mode, where you write it yourself.
A bridge edited outside the Management Console, such as in `config.yaml`, may have several source
topics. Each source topic is a regular expression. It must start with `^`, so it matches from the
beginning of the UNS topic name. Anything may follow the contract, so a source topic selects every
topic that begins with what it matches. umh-core checks the source topics of every write flow. For a
Historian bridge, each source topic:

- starts with `^umh\.v1\.` followed by the bridge location, so the bridge reads data at or beneath
  its own location only.
- names one data contract: `_historian`, or a versioned contract such as `_pump_v1`. Each data
  model you add in the Management Console declares a versioned contract, and each new version of
  the model declares the next one.
- can match only declared versions of that contract. `_pump_v[1-2]` and `_pump_v(?:1|2)` are
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

For the contract `historian`, the topic ends in `_historian\.` with no version. To store
data from several sites in one bridge, place the bridge at a location above them, up to the
enterprise itself, such as `acme`.

When you pick a contract in the topic table, the source topic lists its latest declared version. It
doesn't follow later versions on its own, so data of a version you declare later isn't stored
(see [Troubleshooting](#troubleshooting)). Hovering over a data model's historian status in the
data model view lists the versions its bridge saves. To also save another version, rewrite the
source topic in Code mode with a version range, such as `_pump_v[1-2]\.`.

Deploying or editing a bridge fails with an error that names the topic if a source topic breaks one
of these rules. Examples are a topic outside the bridge location, such as `^umh\.v1\..*`, and one
that names no single contract, such as `^umh\.v1\.acme\.cologne\..*`. Changing a location in a
way that would break an existing bridge's topics fails the same way.

A bridge whose topics were saved before umh-core 0.45.0 keeps its config, but its write flow does
not start. The bridge shows degraded, and its status names each rejected topic and why it is
rejected.

{% hint style="warning" %}
**Don't copy the topics umh-core suggests.** The degraded status and the contract error in the logs
(see [Troubleshooting](#troubleshooting)) each suggest a topic. The topic in the status may name a
different contract than this bridge stores, and matches only data published at the bridge location
itself. The topic in the logs is rejected when you deploy. Write the source topic from the pattern
above, with this bridge's location, contract and versions.
{% endhint %}

`BYPASS_WRITE_FLOW_VALIDATION=true` is an environment variable of the umh-core container, set for
example with `docker run -e`. With it, a write flow whose topics only name an undeclared or
unversioned contract, such as `_mes_order`, keeps running and writing. Its degraded status then
contains "BYPASS_WRITE_FLOW_VALIDATION is set, so the write flow runs although its topics break the
data contract rules". A bridge degraded because of its source topics, without that text, has
stopped writing.

The Management Console keeps `data_contract_name` equal to the contract in the source topic. A
bridge edited outside it, such as in `config.yaml`, or saved before the Console did this, can name
another contract in its source topics. Deploying does not check this. If any message the bridge
receives names another contract, the output refuses it together with the messages received with it,
and does not retry them. All of them are lost.

## Connecting to a different database

The **TimescaleDB (PostgreSQL) — Historian** template is the same output without the Historian
connection: you enter host and port in the connection step and set the username, password, and SSL
mode in the output config yourself. Use it for a database that isn't the instance's Historian, such
as a second archive or a customer-managed database with its own role.

Everything else on this page applies to both templates; only where the connection details come from
differs.

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
umh-core 0.45.0 read the whole UNS, so their write flow does not start. Change their source topics
as in [Which topics the bridge reads](#which-topics-the-bridge-reads). In the Management Console,
such a bridge's topic opens in Code mode; pick one contract in the topic table to save the bridge.

**The bridge is running but nothing arrives.** Open the bridge and select **Logs** (see
[View logs](../data-flows/bridges.md#view-logs)). The output logs once when it stores its first
message. If the contract in the source topics differs from `data_contract_name`, the output refuses
all data and logs an error at most every two minutes that names the contracts that arrived. This
happens only with a bridge edited outside the Management Console, or saved before the Console kept
the two equal. Decide which contract the bridge should store. In the Management Console, pick it in
the topic table, and `data_contract_name` follows. In `config.yaml`, correct the source topics or
`data_contract_name`, whichever names another contract, and write `data_contract_name` without the
version. Data refused until then is lost. Don't use the source topic the error suggests: umh-core
rejects it. Write one as in [Which topics the bridge reads](#which-topics-the-bridge-reads) instead.

If the output logs nothing at all, first check whether the bridge shows degraded. A bridge whose
source topics are rejected stays connected, but its write flow doesn't start, so the output logs
nothing. Its status names the rejected topics; correct them as in
[Which topics the bridge reads](#which-topics-the-bridge-reads). If the bridge isn't degraded, no
data of the versions in the source topics is published beneath the bridge location. Check the Topic
Browser: devices may publish a newer version, such as `_pump_v3`, that the source topics don't list
yet. To store it too, add it to the source topic in Code mode.

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
