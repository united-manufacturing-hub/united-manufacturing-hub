# Producing Data

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
      template:
        connection:
          nmap:
            target: "{{ .HOST }}"
            port: "{{ .PORT }}"
        dataflowcomponent_read:
          benthos:
            input:
              opcua:
                endpoint: "opc.tcp://{{ .HOST }}:{{ .PORT }}"
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
        HOST: "192.168.1.100"
        PORT: "4840"
```

## Message Flow

1. **Input**: Benthos reads from your device using industrial protocols
2. **Processing**: `tag_processor` adds UNS metadata (location, contract, tag name)
3. **Output**: Messages are published to the embedded Redpanda broker following [Topic Convention](topic-convention.md)

## Supported Industrial Protocols

UMH Core supports 50+ industrial connectors via Benthos-UMH. Key protocols include:

- **OPC UA** - Industry standard for industrial automation
- **Modbus TCP/RTU** - Serial and Ethernet Modbus devices  
- **Siemens S7** - Direct PLC communication
- **Ethernet/IP** - Allen-Bradley and other CIP devices
- **ifm IO-Link Master** - Sensor connectivity via IO-Link

For complete protocol documentation, see [Benthos-UMH Input Documentation](https://docs.umh.app/benthos-umh).

## Tag Processor

The `tag_processor` is crucial for UNS compliance. It adds required metadata:

```yaml
pipeline:
  processors:
    - tag_processor:
        defaults: |
          msg.meta.location_path = "acme.plant1.line4.machine7";
          msg.meta.data_contract = "_raw";  
          msg.meta.tag_name = msg.meta.opcua_tag_name;
          return msg;
```

**Required fields:**
- `location_path`: ISA-95 hierarchy (dot-separated)
- `data_contract`: Schema identifier (starts with `_`)
- `tag_name`: Final topic segment

## Data Contracts

Choose the appropriate data contract for your use case:

| Contract | Usage | Payload Format | When to Use |
|----------|--------|----------------|-------------|
| `_raw` | Unprocessed device data | `{"value": 42.5, "timestamp_ms": 1733904005123}` | Initial data ingestion, simple sensors |
| Custom contracts | Structured business data | Model-defined schema | Enterprise data modeling |

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
*Results in:* `umh.v1.acme.plant1.line4.sensor1._raw.temperature`

**Evolve to Structured (Data Models):** 🚧
```yaml
# Define data model (part of data modeling system)
datamodels:
  - name: Temperature
    version: v1
    structure:
      temperature_in_c:
        type: timeseries
        constraints:
          unit: "°C"

# Create data contract
datacontracts:
  - name: _temperature
    version: v1
    model: Temperature:v1

# Stream processor transforms raw to structured
streamprocessors:
  - name: temp_structured
    contract: _temperature:v1
    sources:
      raw_temp: "umh.v1.acme.plant1.line4.sensor1._raw.temperature"
    mapping: |
      temperature_in_c: raw_temp
```
*Results in:* `umh.v1.acme.plant1.line4.sensor1._temperature.temperature_in_c`

## Stand-alone Flows vs Bridges

**Use Bridges when:**
- Connecting to field devices (PLCs, sensors)
- Need connection monitoring
- Want location-based organization

**Use Stand-alone Flows when:**
- Point-to-point data movement
- No connection monitoring needed
- Custom processing pipelines

```yaml
# Stand-alone Flow example
dataFlow:
  - name: mqtt-to-uns
    desiredState: active
    dataFlowComponentConfig:
      benthos:
        input:
          mqtt:
            urls: ["tcp://mqtt-broker:1883"]
            topics: ["sensors/+/temperature"]
        pipeline:
          processors:
            - tag_processor:
                defaults: |
                  msg.meta.location_path = "acme.plant1.line4.sensor1";
                  msg.meta.data_contract = "_raw";
                  msg.meta.tag_name = "temperature";
                  return msg;
        output:
          uns: {}
```

## Legacy Note

> **UMH Classic Users:** The previous `_historian` data contract is deprecated. Use `_raw` for simple sensor data and explicit data contracts (like `_temperature`, `_pump`) with the new [data modeling system](../data-modeling/README.md) 🚧 for structured industrial data.

## Next Steps

- **[Consuming Data](consuming-data.md)** 🚧 - Process UNS messages
- **[Data Modeling](../data-modeling/README.md)** 🚧 - Structure your data with explicit models
- **[Configuration Reference](../../reference/configuration-reference.md)** - Complete YAML syntax

## Learn More

- [Connect ifm IO-Link Masters with the UNS](https://learn.umh.app/blog/connect-ifm-io-link-masters-with-the-uns/) - Real-world sensor connectivity
- [Node-RED meets Benthos!](https://learn.umh.app/blog/node-red-meets-benthos/) - Custom processing with Node-RED JavaScript

