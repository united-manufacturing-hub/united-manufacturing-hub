# Contracts, versions and tables

Each version of a data model declares its own data contract: version 1 of `pump` declares `_pump_v1`,
version 2 declares `_pump_v2`. The Historian stores all versions in one set of tables,
`umh.value_pump` and `umh.attribute_pump`, so a tag keeps one history across versions and a Grafana
panel does not change when the data model gets a new version. The version is stored with each value
in `attribute->>'data_contract_version'`.

A Historian bridge stores only the versions listed in its source topic, so a new version is stored
once you add it to the bridge; see
[Troubleshoot a Historian bridge](troubleshoot-a-bridge.md#a-new-version-of-the-data-model-is-not-stored).
