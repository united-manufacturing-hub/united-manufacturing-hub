# Producing Data

> **UMH Classic Users:** The previous `_historian` data contract is not used anymore in UMH core. Use `_raw` for simple sensor data and explicit data contracts (like `_temperature`, `_pump`) with the new [data modeling system](../data-modeling/README.md) 🚧 for structured industrial data. See [Migration from UMH Classic to UMH Core](../../production/migration-from-classic.md) for complete migration instructions.
> 
> 🚧 **Roadmap Item** - Enhanced producer tooling and simplified configuration are under development.

Publishing data to the Unified Namespace involves creating Bridges (shown as `protocolConverter:` in YAML) or Stand-alone Flows (shown as `dataFlow:` in YAML) that read from your devices and format messages according to UNS conventions.

## Quick Start - OPC UA to UNS

Create a Bridge to read from an OPC UA server:

```yaml
protocolConverter:
  - name: opcua-bridge
    desiredState: active
    protocolConverterServiceConfig:
      location:
        2: "line-1"
        3: "pump-01"
      config:
        connection:
          nmap:
            target: "{{ .IP }}"
            port: "{{ .PORT }}"
        dataflowcomponent_read:
          benthos:
            input:
              opcua:
                endpoint: "opc.tcp://{{ .IP }}:{{ .PORT }}"
                nodeIDs: ["ns=2;s=Temperature", "ns=2;s=Pressure"]
            pipeline:
              processors:
                - tag_processor:
                    defaults: |
                      msg.meta.location_path = "{{ .location_path }}";
                      msg.meta.data_contract = "_raw";
                      msg.meta.tag_name = msg.meta.opcua_tag_name;
                      return msg;
            output:
              uns: {}
      variables:
        IP: "192.168.1.100"
        PORT: "4840"
```

## Message Flow

1. **Input**: Benthos reads from your device using industrial protocols
2. **Processing**: `tag_processor` adds UNS metadata (location, contract, tag name)
3. **Output**: Messages are published to the embedded Redpanda broker following [Topic Convention](topic-convention.md)

## Supported Industrial Protocols

UMH Core supports 50+ industrial connectors via Benthos-UMH. For complete protocol documentation and examples, see [Bridges - Supported Protocols](../data-flows/bridges.md#supported-protocols).

## Tag Processor 🚧

> **🚧 Roadmap Item**: The current `tag_processor` implementation follows the benthos-umh pattern with tag names in payloads. With the next UMH Core release, `tag_processor` will be updated to align with the new data model where tag names are only in topics (not in payloads) and metadata is not included in message payloads.

The `tag_processor` is crucial for UNS compliance. It adds required metadata for topic construction. For complete syntax and examples, see [Configuration Reference - Tag Processor](../../reference/configuration-reference.md#tag_processor).

**Essential pattern:**

```yaml
pipeline:
  processors:
    - tag_processor:
        defaults: |
          msg.meta.location_path = "{{ .location_path }}";
          msg.meta.data_contract = "_raw";  
          msg.meta.tag_name = msg.meta.opcua_tag_name;
          return msg;
```

## Data Contracts

Choose the appropriate data contract for your use case:

| Contract         | Usage                    | Payload Format                                   | When to Use                            |
| ---------------- | ------------------------ | ------------------------------------------------ | -------------------------------------- |
| `_raw`           | Unprocessed device data  | `{"value": 42.5, "timestamp_ms": 1733904005123}` | Initial data ingestion, simple sensors |
| Custom contracts | Structured business data | Model-defined schema                             | Enterprise data modeling               |

### Evolution from Raw to Structured

**Start Simple (Raw Data):**

```yaml
pipeline:
  processors:
    - tag_processor:
        defaults: |
          msg.meta.data_contract = "_raw";
          msg.meta.tag_name = "temperature";
```

_Results in:_ `umh.v1.acme.plant1.line4.sensor1._raw.temperature`

**Evolve to Structured (Data Models):** 🚧\
For structured data evolution using data models, data contracts, and stream processors, see [Data Modeling Documentation](../data-modeling/).

Example result: `umh.v1.acme.plant1.line4.sensor1._temperature.temperatureInC`

## Stand-alone Flows vs Bridges

For choosing between Bridges and Stand-alone Flows, see the [complete comparison in Bridges documentation](../data-flows/bridges.md#when-to-use-bridges).

## Next Steps

* [**Consuming Data**](consuming-data.md) 🚧 - Process UNS messages
* [**Data Modeling**](../data-modeling/) 🚧 - Structure your data with explicit models
* [**Configuration Reference**](../../reference/configuration-reference.md) - Complete YAML syntax

## Learn More

* [Connect ifm IO-Link Masters with the UNS](https://learn.umh.app/blog/connect-ifm-io-link-masters-with-the-uns/) - Real-world sensor connectivity
* [Node-RED meets Benthos!](https://learn.umh.app/blog/node-red-meets-benthos/) - Custom processing with Node-RED JavaScript
