# Historian Bridge Loses Data

A Historian bridge runs, but its data does not arrive in the database, and its logs show an error
that names the data contracts that arrived.

## Cause

The source topic names another data contract than `data_contract_name`. This happens only after
editing the bridge in `config.yaml` or saving it with an older Management Console. The output refuses
whole batches and does not retry them.

## What to do

Pick the data contract in the topic table and save. Refused data stays lost.
