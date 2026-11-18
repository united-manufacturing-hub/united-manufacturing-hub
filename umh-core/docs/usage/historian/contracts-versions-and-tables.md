# Contracts, versions and tables

This page explains how the Historian maps data contracts and their versions to database tables, and
what that means for a Historian bridge when a data model gets a new version.

## One set of tables per data contract

A data model has versions. Each version declares its own data contract in the Unified Namespace:
version 1 of the `pump` data model declares `_pump_v1`, version 2 declares `_pump_v2`. The Historian
stores all versions of a data contract in one set of tables, `umh.value_pump` and
`umh.attribute_pump`. A tag that exists in both versions keeps one identity and one history.

The version is metadata of each stored value. It is in `attribute->>'data_contract_version'`, not in
a column or a table name; see [Database schema](database-schema.md#contract-version).

This design keeps a tag's history in one place across versions. A query for the pump `temperature`
returns its values from before and after the new version, and a Grafana panel does not change when the
data model gets a new version.

## A bridge reads chosen versions

A Historian bridge stores only the versions listed in its source topic, so a new version of the data
model is stored once you add it to the bridge; see
[Troubleshoot a Historian bridge](troubleshoot-a-bridge.md#a-new-version-of-the-data-model-is-not-stored).
