# Troubleshoot a Historian bridge

This page lists what can go wrong with a Historian bridge, why, and how to fix it. Most fixes need the
bridge's logs: open the bridge and select **Logs** (see [View Logs](../data-flows/bridges.md#view-logs)).

## The bridge fails right after deploying

**Cause.** The Historian output checks the database when it starts, instead of failing later on the
first write. It stops with an error that names the cause:

- The host is unreachable.
- The server runs PostgreSQL older than 16.
- The `timescaledb` or `ltree` extension is missing.
- The login role can connect but cannot `INSERT` into the data contract's tables.

**Fix.** Open the Historian overview page (**Plugins** → Historian card → **Open plugin**). If
**Reachable** is `No` or **Auth** is not valid, fix the [connection](connection.md) first. Otherwise
fix the cause the error names; the [requirements](connection.md#requirements) list what the database
needs.

## The bridge is degraded or does not deploy because of its topics

**Cause.** umh-core rejected the bridge's source topics. Historian bridges created before umh-core
0.45.0 read the whole Unified Namespace, which umh-core 0.45.0 no longer accepts. Their write flow
does not start, and the bridge shows degraded.

**Fix.** Open the bridge and pick one data contract in the topic table, then save it. For a topic
written in Code mode or in `config.yaml`, follow
[Write Flow Topics Rejected](../data-flows/troubleshooting/write-flow-topics-rejected.md).

## The bridge is running, but the data of its data contract is lost

**Cause.** The data contract in the source topics differs from `data_contract_name`. The output
refuses every batch that holds a message of another data contract, and does not retry it. All
messages in the batch are lost, including those of the right contract. The output logs an error at
most every two minutes that names the data contracts that arrived.

This happens only with a bridge edited outside the Management Console, for example in `config.yaml`,
or saved before the Management Console kept the two equal. Deploying does not check it.

**Fix.** Decide which data contract the bridge stores.

- In the Management Console, pick the data contract in the topic table. `data_contract_name` follows
  it.
- In `config.yaml`, correct the source topics or `data_contract_name`, whichever names the other data
  contract. Write `data_contract_name` without the version.

Do not use the source topic the error suggests: umh-core rejects it. Data refused before the fix
stays lost.

## A new version of the data model is not stored

**Cause.** The source topic lists the versions that were declared when the bridge was saved. It does
not include later versions. Devices that publish a newer version, such as `_pump_v3`, write data the
bridge does not read.

**Fix.** Check in the Topic Browser which versions your devices publish. Hover over the data model's
**Historian** column on the **Data Models** page to see the versions the bridge stores. Add the new
version to the source topic in Code mode, for example `_pump_v[1-3]\.`. See
[Contracts, versions and tables](contracts-versions-and-tables.md) for why a new version is not
stored on its own.

## A query from the SQL query tab returns no rows

**Cause.** The Topic Browser builds the queries from the topic name, so the **SQL query** tab shows
them for every time-series tag, whether or not the Historian stores it. A query returns no rows when:

- the instance has no Historian connection.
- no Historian bridge stores the tag's data contract.
- the bridge has not written any data yet.
- the bridge does not read the tag, for example because the tag's version is not in its source topic.

**Fix.** Check the **Historian** column on the **Data Models** page. If the data model is not stored,
save it as in [Use the Historian](use-the-historian.md#step-3-save-the-data-model-to-the-historian).
If it is stored, open the bridge and check its status and logs against the sections above.

## Rows are dropped

**Cause.** Some rows can never be written. The output drops them and writes the rest of the batch:

- A tag whose datatype changes between number and text. See
  [Precision and datatypes](precision-and-datatypes.md).
- Two different values of one tag at the same millisecond.

The output logs each dropped row with its tag and counts it on `historian_rows_poisoned`. Messages
dropped before the write, such as a payload without `timestamp_ms`, are counted on
`messages_dropped` with a `reason` label. Both counters belong to the bridge's Benthos process, not to
umh-core's port 8080. See [Historian output counters](README.md#historian-output-counters).

**Fix.** Follow
[Runbook: poisoned tags](https://docs.umh.app/benthos-umh/output/historian#runbook-poisoned-tags) in
the Historian output reference.

Connection loss, deadlocks, a missing grant and a full disk do not drop rows. The output retries them
until the cause is fixed.
