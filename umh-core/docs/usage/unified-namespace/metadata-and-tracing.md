# Metadata and Tracing

Every message in the UNS carries metadata that preserves its origin and transformation history. This enables troubleshooting and data lineage tracking.

## Understanding Metadata

When data flows through the UNS, metadata accumulates at each step:

```json
{
  "_incomingKeys": "s7_address",
  "_initialMetadata": "{\"s7_address\":\"DB1.DW20\"}",
  "bridged_by": "protocol-converter_pump-bridge",
  "data_contract": "_pump_v1",
  "data_contract_name": "_pump",
  "data_contract_version": "1",
  "kafka_msg_key": "umh.v1.enterprise.chicago.packaging.line-1.pump-01._pump_v1.inlet_temperature",
  "kafka_timestamp_ms": "1758290100065",
  "kafka_topic": "umh.messages",
  "location_path": "enterprise.chicago.packaging.line-1.pump-01",
  "s7_address": "DB1.DW20",
  "name": "inlet_temperature",
  "topic": "umh.v1.enterprise.chicago.packaging.line-1.pump-01._pump_v1.inlet_temperature",
  "umh_topic": "umh.v1.enterprise.chicago.packaging.line-1.pump-01._pump_v1.inlet_temperature"
}
```
## Tracing Data Flow

### Bridge Metadata
Every bridge adds metadata to identify the source:

```yaml
# In Topic Browser or when consuming:
bridged_by: "protocol-converter_pump-bridge"    # Which bridge created this
location_path: "enterprise.chicago.packaging"    # Where it came from
data_contract: "_pump_v1"                       # What model was applied
```

Note: The exact format of bridge metadata may vary by implementation.

## Accessing Metadata

### In Topic Browser
The Management Console shows metadata in the details panel:
1. Select any topic in the tree
2. View "Metadata" section in the right panel
3. See all headers including original tags

For details, see [Topic Browser documentation](topic-browser.md).

**Further metadata**:
Each input or processing step adds its own metadata. For complete field documentation, see:
- [Benthos-UMH Input Plugins](https://docs.umh.app/benthos-umh/input)

## Next Steps

- [**Topic Browser**](topic-browser.md) - View metadata interactively
- [**Bridges**](../data-flows/bridges.md) - How data enters and exits the UNS
