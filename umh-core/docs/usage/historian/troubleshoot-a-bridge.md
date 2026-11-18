# Troubleshoot a Historian bridge

Most fixes need the bridge's logs: open the bridge and select **Logs**.

## The bridge fails right after deploying

**Cause.** The output checks the database on start and stops with an error that names the cause:

- The host is unreachable.
- The server runs PostgreSQL older than 16.
- The `timescaledb` or `ltree` extension is missing.
- The login role can connect but cannot `INSERT` into the data contract's tables.

**Fix.** If the Historian overview page shows **Reachable** `No` or an invalid **Auth**, fix the
[connection](connection.md). Otherwise fix the cause the error names; see
[Requirements](connection.md#requirements).

## The bridge is degraded or does not deploy because of its topics

**Cause.** umh-core rejected the source topic. Historian bridges created before umh-core 0.45.0 read
the whole Unified Namespace, which 0.45.0 rejects.

**Fix.** Open the bridge, pick one data contract in the topic table and save. For a topic written by
hand, see [Write Flow Topics Rejected](../data-flows/troubleshooting/write-flow-topics-rejected.md).

## The bridge is running, but the data of its data contract is lost

**Cause.** The source topic names another data contract than `data_contract_name`, which only happens
after editing the bridge in `config.yaml` or saving it with an older Management Console. The output then refuses whole batches and does not retry
them, and logs an error naming the contracts that arrived.

**Fix.** Pick the data contract in the topic table and save. Refused data stays lost.

## A new version of the data model is not stored

**Cause.** The source topic lists the versions that existed when the bridge was saved.

**Fix.** Hover over the **Historian** column on **Data Models** to see the stored versions. Add the new
version to the source topic in Code mode, for example `_pump_v[1-3]\.`.

## A query from the SQL query tab returns no rows

**Cause.** The **SQL query** tab shows queries for every time-series tag, stored or not. A query
returns no rows when:

- the instance has no Historian connection.
- no Historian bridge stores the tag's data contract.
- the bridge has not written any data yet.
- the bridge does not read the tag, for example because the tag's version is not in its source topic.

**Fix.** Check the **Historian** column on **Data Models**. Save the data model if it is not stored
([Step 3](use-the-historian.md#step-3-save-the-data-model-to-the-historian)); otherwise check the
bridge against the sections above.

## Rows are dropped

**Cause.** The output drops rows it can never write and writes the rest of the batch:

- A tag whose datatype changes between number and text; see
  [Precision and datatypes](precision-and-datatypes.md).
- Two different values of one tag at the same millisecond.

It logs each dropped row and counts it on `historian_rows_poisoned`. Messages dropped earlier, such as
a payload without `timestamp_ms`, count on `messages_dropped`. Connection loss and a full disk drop
nothing; the output retries.

**Fix.** Follow
[Runbook: poisoned tags](https://docs.umh.app/benthos-umh/output/historian#runbook-poisoned-tags).
