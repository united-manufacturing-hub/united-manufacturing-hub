# Historian Bridge Writes No Data

A Historian bridge runs, but its data does not arrive in the database, and its logs show an error
that starts with "TimescaleDB historian:". The error says how many messages carry another data
contract and names those data contracts.

## Cause

The source topic names another data contract than `data_contract_name`. This happens only after
editing the bridge in `config.yaml` or saving it with an older Management Console. The output refuses
these messages and does not write them to the database.

## What to do

Pick the data contract in the topic table and save. Messages refused before the fix are not written
later.
