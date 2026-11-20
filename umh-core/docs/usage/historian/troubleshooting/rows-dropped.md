# Historian Rows Dropped

The bridge logs dropped rows, and `historian_rows_poisoned` or `messages_dropped` rises.

## Cause

The output drops rows it can never write and writes the rest of the batch:

- A tag whose datatype changes between number and text; see
  [Precision and datatypes](../how-the-historian-stores-data.md#precision-and-datatypes).
- Two different values of one tag at the same millisecond.
- A message without `timestamp_ms` or with another payload shape, counted on `messages_dropped`.

Connection loss and a full disk drop nothing; the output retries.

A drop logged as an error marks the bridge as degraded for 10 minutes after the log line, even when
the rest of the batch is stored.

## What to do

Follow
[Runbook: poisoned tags](https://docs.umh.app/benthos-umh/output/historian#runbook-poisoned-tags).
