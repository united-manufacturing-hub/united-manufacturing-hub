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



## Common Metadata Fields

### Core UNS Metadata
| Field | Description | Example |
|-------|-------------|---------|
| `umh_topic` | Full UNS topic path | `umh.v1.enterprise.chicago._pump_v1.temperature` |
| `location_path` | Location path in hierarchy | `enterprise.chicago.packaging.line-1` |
| `data_contract` | Applied data contract | `_pump_v1` |
| `name` | Data point name | `inlet_temperature` |
| `virtual_path` | Optional grouping | `motor.electrical` |

### Kafka/Redpanda Metadata
| Field | Description | Example |
|-------|-------------|---------|
| `kafka_topic` | Internal topic | `umh.messages` |
| `kafka_msg_key` | Message key | Same as umh_topic |
| `kafka_timestamp_ms` | Broker timestamp | `1758290100065` |
| `kafka_offset` | Message offset | `12345` |
| `kafka_partition` | Topic partition | `0` |

### Protocol-Specific Metadata

**OPC UA** (documented fields):
- `opcua_tag_name` - The sanitized Node ID
- `opcua_tag_path` - Dot-separated path to the tag
- `opcua_tag_type` - The data type
- `opcua_source_timestamp` - OPC UA SourceTimestamp
- `opcua_server_timestamp` - OPC UA ServerTimestamp
- Additional attributes like `opcua_attr_nodeid`, `opcua_attr_browsename`, etc.

**Other Protocols**:
Each protocol input adds its own metadata. For complete field documentation, see:
- [Benthos-UMH Input Plugins](https://docs.umh.app/benthos-umh/input)

## Next Steps

- [**Topic Browser**](topic-browser.md) - View metadata interactively
- [**Bridges**](../data-flows/bridges.md) - How data enters and exits the UNS
