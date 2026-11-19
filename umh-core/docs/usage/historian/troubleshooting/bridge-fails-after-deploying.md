# Historian Bridge Fails After Deploying

A Historian bridge stops right after it is deployed, and its logs show an error from the Historian
output. The output checks the database on start.

## Cause

The error names one of these causes:

- The host is unreachable.
- The server runs PostgreSQL older than 16.
- The `timescaledb` or `ltree` extension is missing.
- The login role can connect but cannot `INSERT` into the data contract's tables.

## What to do

If the Historian overview page shows **Reachable** `No` or an invalid **Auth**, fix the connection;
see [Setup](../setup.md). Otherwise fix the cause the error names; see
[Requirements](../setup.md#requirements).
