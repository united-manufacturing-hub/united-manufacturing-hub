# Bridges

> 🚧 **Roadmap Item** - Bridges are under active development. Current functionality includes connection monitoring and basic read/write flows.

Bridges (shown as `protocolConverter:` in YAML configuration) connect external devices and systems to the Unified Namespace. They combine connection health monitoring with bidirectional data flows, making them ideal for industrial device connectivity.

## Key Features

* **Connection Monitoring**: Continuous health checks via nmap, ping, or custom probes
* **Location Hierarchy**: Automatic hierarchical path construction from agent and bridge locations (supports ISA-95, KKS, or custom naming)
* **Bidirectional Data Flow**: Separate read and write pipelines for full device interaction
* **Variable Templating**: Go template support for flexible configuration
* **State Management**: Advanced finite state machine for operational visibility

## When to Use Bridges

**Choose Bridges for:**

* Connecting to field devices (PLCs, HMIs, sensors, actuators)
* Industrial protocol communication (OPC UA, Modbus, S7, Ethernet/IP)
* Systems requiring connection health monitoring
* Publishing data to the Unified Namespace with automatic location context

**Use** [**Stand-alone Flows**](stand-alone-flow.md) **for:**

* Point-to-point data transformation without UNS integration
* Custom processing that doesn't require device connectivity patterns
* High-throughput scenarios where monitoring overhead isn't needed

## Benthos vs Node-RED Decision Matrix

**Use Benthos/Bridges** for data pipelines with verified industrial protocols (OPC UA, Modbus, S7, Ethernet/IP), production throughput requirements, or when you need enterprise reliability and structured configurations that scale across teams. The Management Console provides GUI configuration with input validation 🚧, making Benthos accessible to OT teams while maintaining structured, maintainable configs. **Use Node-RED** for rapid prototyping with visual programming, complex site-specific business logic requiring full JavaScript, protocols not yet available in Benthos-UMH, or when building custom applications and dashboards beyond data movement. Many deployments use both: Benthos for high-volume standard protocols, Node-RED for specialized integration and custom applications.

## Configuration Structure

```yaml
protocolConverter:
  - name: device-bridge
    desiredState: active
    protocolConverterServiceConfig:
      location:
        2: "area-name"    # Appended to agent.location
        3: "device-id"
      config:
        connection:
          # Health monitoring configuration
        dataflowcomponent_read:
          # Data ingestion pipeline (optional)
        dataflowcomponent_write:
          # Data output pipeline (optional)
      variables:
        # Template variables
```

## Industrial Protocol Examples

### Basic Bridge Pattern

All industrial protocol bridges follow the same pattern - only the input configuration changes. This example uses connection variables ([{{ .IP }}](../../reference/variables.md#variables) and [{{ .PORT }}](../../reference/variables.md#variables)) and the location variable ([{{ .location_path }}](../../reference/variables.md#variables)):

```yaml
protocolConverter:
  - name: industrial-device
    desiredState: active
    protocolConverterServiceConfig:
      location:
        2: "production-line"  # Area
        3: "device-name"      # Work cell
      config:
        connection:
          nmap:
            target: "{{ .IP }}"
            port: "{{ .PORT }}"
        dataflowcomponent_read:
          benthos:
            input:
              # ANY benthos-umh industrial protocol input
              opcua: { /* ... */ }     # or modbus, s7, etc.
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
        PORT: "502"
```

### Supported Protocols

UMH Core supports 50+ industrial protocols via [Benthos-UMH](https://docs.umh.app/benthos-umh). For complete, up-to-date configuration examples:

**Industrial Protocols:**

* [**OPC UA**](https://docs.umh.app/benthos-umh/input/opc-ua-input) - Industry standard automation protocol
* [**Modbus**](https://docs.umh.app/benthos-umh/input/modbus) - Serial and TCP Modbus devices
* [**Siemens S7**](https://docs.umh.app/benthos-umh/input/siemens-s7) - Direct PLC communication
* [**Ethernet/IP**](https://docs.umh.app/benthos-umh/input/ethernet-ip) - Allen-Bradley and CIP devices
* [**ifm IO-Link Master**](https://docs.umh.app/benthos-umh/input/ifm-io-link-master-sensorconnect) - Sensor connectivity

**IT Protocols:**

* MQTT, Kafka, HTTP/REST, SQL databases, File systems

> **Note:** Always reference the [Benthos-UMH documentation](https://docs.umh.app/benthos-umh) for the most current protocol configurations and features.

## Bidirectional Communication

Bridges support both reading from and writing to devices using separate pipelines. This example uses connection variables ([{{ .IP }}](../../reference/variables.md#variables) and [{{ .PORT }}](../../reference/variables.md#variables)) for device connectivity:

```yaml
protocolConverter:
  - name: bidirectional-device
    desiredState: active
    protocolConverterServiceConfig:
      location:
        2: "assembly-line"
        3: "controller"
      config:
        connection:
          nmap:
            target: "{{ .IP }}"
            port: "{{ .PORT }}"
        dataflowcomponent_read:
          benthos:
            input:
              opcua: { /* read configuration */ }
            pipeline:
              processors:
                - tag_processor: { /* UNS metadata */ }
            output:
              uns: {}
        dataflowcomponent_write:
          benthos:
            input:
              uns:
                topics: ["umh.v1.+.+.assembly-line.controller._commands.+"]
            pipeline:
              processors:
                - mapping: |
                    # Transform UNS commands to device writes
                    root.command = metadata("umh_topic").split(".").7
                    root.value = this.value
            output:
              opcua_write: { /* write configuration */ }
      variables:
        IP: "192.168.1.100"
        PORT: "4840"
```

**Pattern:**

* **Read pipeline**: Device → tag\_processor → UNS output
* **Write pipeline**: UNS input → command mapping → Device output

## Connection Health Monitoring

UMH Core currently supports network connectivity monitoring via nmap:

### Network Connectivity (nmap)

The nmap health check uses connection variables ([{{ .IP }}](../../reference/variables.md#variables) and [{{ .PORT }}](../../reference/variables.md#variables)) to verify device connectivity:

```yaml
connection:
  nmap:
    target: "{{ .IP }}"
    port: "{{ .PORT }}"
```

The nmap health check verifies that the target device is reachable on the specified port, ensuring the bridge can establish connections before attempting data operations.

## State Management

Bridges use finite state machines to track operational status. For complete state definitions, transitions, and monitoring details, see [State Machines Reference](../../reference/state-machines.md).

## Template Variables

Use Go template syntax for flexible configuration. For a complete list of all available variables, see the [Template Variables Reference](../../reference/variables.md#variables).

```yaml
variables:
  IP: "192.168.1.100"
  PORT: "4840"
  SCAN_RATE: "1s"
  TAG_PREFIX: "line4_pump"

template:
  dataflowcomponent_read:
    benthos:
      input:
        opcua:
          endpoint: "opc.tcp://{{ .IP }}:{{ .PORT }}"
          subscription_interval: "{{ .SCAN_RATE }}"
      pipeline:
        processors:
          - tag_processor:
              defaults: |
                msg.meta.tag_name = "{{ .TAG_PREFIX }}_" + msg.meta.opcua_tag_name;
```

## Integration with Data Modeling

Bridges work seamlessly with UMH's [data modeling system](../data-modeling/):

```yaml
# Bridge publishes raw data
protocolConverter:
  - name: pump-raw-data
    # ... bridge configuration ...
    pipeline:
      processors:
        - tag_processor:
            defaults: |
              msg.meta.data_contract = "_raw";  # Raw data contract

# Stream Processor transforms to structured model 🚧
streamprocessors:
  - name: pump_structured
    contract: _pump:v1  # References pump data model
    sources:
      pressure: "umh.v1.acme.plant1.line4.pump01._raw.pressure"
      temperature: "umh.v1.acme.plant1.line4.pump01._raw.temperature"
    # ... transformation logic ...
```

## Data Contract Evolution 🚧

> **🚧 Roadmap Item**: The current `tag_processor` implementation follows the benthos-umh pattern with tag names in payloads. With the next UMH Core release, `tag_processor` will be updated to align with the new data model where tag names are only in topics (not in payloads) and metadata is not included in message payloads.

Bridges support evolution from simple raw data to structured data contracts. See [Configuration Reference - Data Contract Guidelines](../../reference/configuration-reference.md#tag_processor) for complete tag\_processor syntax.

### Start Simple (Raw Data)

```yaml
pipeline:
  processors:
    - tag_processor:
        defaults: |
          msg.meta.data_contract = "_raw";
          msg.meta.tag_name = "temperature";
```

_Results in:_ `umh.v1.acme.plant1.line4.sensor1._raw.temperature`

Raw data uses the standard [timeseries payload format](../unified-namespace/payload-formats.md).

### Evolve to Structured (Data Models) 🚧

```yaml
# Stream Processor creates structured data from raw inputs
streamprocessors:
  - name: temperature_structured
    contract: _temperature:v1  # References Temperature data model
    sources:
      raw_temp: "umh.v1.acme.plant1.line4.sensor1._raw.temperature"
    # ... transformation logic ...
```

_Results in:_ `umh.v1.acme.plant1.line4.sensor1._temperature.temperatureInC`

For payload format details, see [Payload Formats](../unified-namespace/payload-formats.md).

## UNS Input/Output Usage

Bridges exclusively use UNS input/output for UNS integration:

**For reading device data:**

```yaml
input:
  opcua: {}           # Industrial protocol input
output:
  uns: {}             # Always use UNS output for publishing to UNS
```

**For writing to devices:**

```yaml
input:
  uns:                # Use UNS input to consume commands from UNS
    topics: ["umh.v1.+.+.+.+._commands.+"]
output:
  opcua_write: {}     # Industrial protocol output
```

This approach abstracts away Kafka/Redpanda complexity and aligns with UMH Core's embedded architecture philosophy. See [UNS Output Documentation](https://docs.umh.app/benthos-umh/output/uns-output) for complete configuration options.

## Migration from UMH Classic

> **UMH Classic Users:** See [Migration from UMH Classic to UMH Core](../../production/migration-from-classic.md) for complete migration instructions including `_historian` → `_raw` data contract changes and configuration updates.

## Related Documentation

* [**Stand-alone Flows**](stand-alone-flow.md) - Alternative for custom processing
* [**Data Modeling**](../data-modeling/) - Structure bridge data with models
* [**Configuration Reference**](../../reference/configuration-reference.md) - Complete YAML syntax
* [**State Machines**](../../reference/state-machines.md) - Bridge state management details

## Learn More

* [**Connect ifm IO-Link Masters with the UNS**](https://learn.umh.app/blog/connect-ifm-io-link-masters-with-the-uns/) - Real-world sensor connectivity example
* [**New Bridges and Automatic Helm Upgrade in UMH**](https://learn.umh.app/blog/new-bridges-and-automatic-helm-upgrade-in-umh/) - Bridge architecture evolution
* [**Benthos-UMH UNS Output Documentation**](https://docs.umh.app/benthos-umh/output/uns-output) - Complete UNS output reference
