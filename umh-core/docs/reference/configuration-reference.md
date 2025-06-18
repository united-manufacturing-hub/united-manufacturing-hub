# Configuration Reference

This is the reference for the central config `/data/config.yaml`

> **File location:** The container mounts `/data` as a writable volume.\
> **Hot-reload:** The Agent polls the file every tick; valid changes are applied automatically.\
> **UI Terminology:** In the Management Console UI, `protocolConverter:` is shown as "Bridges" and `dataFlow:` is shown as "Stand-alone Flows"

## YAML vs UI: Choose Your Workflow

UMH Core supports both **direct YAML editing** and **Management Console UI** for configuration. Each approach has distinct advantages:

### YAML Configuration Benefits

* **Version Control**: Every change is tracked with Git — see who changed what, when, and why
* **GitOps Integration**: Automate deployments, rollbacks, and multi-environment promotion
* **AI/LLM Integration**: Tools like Cursor and ChatGPT can generate, modify, and optimize configurations
* **Templating Power**: Create and connect hundreds of machines with a few keystrokes using YAML anchors
* **IDE Support**: Syntax highlighting, validation, autocomplete, and refactoring tools

### UI Configuration Benefits

* **Zero Code**: Any OT person or business user can click and configure without YAML knowledge
* **Input Validation**: Real-time validation prevents configuration errors before deployment
* **Visual Guidance**: Wizards and forms guide users through complex protocol configurations
* **Live Preview**: See the impact of changes before applying them

### The Best of Both Worlds

**UMH Core provides two-way synchronization:** changes made in the UI are reflected in the YAML file, and YAML edits are shown in the UI. This means:

* **OT teams** can use the UI for day-to-day operations
* **IT teams** can use YAML for infrastructure-as-code workflows  
* **Both approaches** maintain full version control and auditability
* **Scaling operations** benefit from templating while **individual adjustments** benefit from UI convenience

> **Pro Tip**: Start with the UI to understand the configuration structure, then switch to YAML for advanced templating and automation.

## Terminology Reference

| Current Term (UMH Core) | Legacy Term (UMH Classic) | YAML Key | Description |
|------------------------|---------------------------|-----------|-------------|
| Bridge | Protocol Converter | `protocolConverter:` | Connects external devices to UNS with health monitoring |
| Stand-alone Flow | Data Flow Component (DFC) | `dataFlow:` | Point-to-point data processing pipelines |
| Stream Processor | Stream Processor | `dataFlow:` | Processes data within UNS (upcoming feature) |

> **Note:** YAML configuration keys retain legacy names for backward compatibility. The Management Console UI uses the current terminology.

### Agent - Runtime & Device identity

| Field                           | Type               | Default             | Purpose                                                                                        |
| ------------------------------- | ------------------ | ------------------- | ---------------------------------------------------------------------------------------------- |
| `metricsPort`                   | `int`              | **9102**            | Exposes Prometheus metrics for the container.                                                  |
| `location`                      | map `int → string` | –                   | Hierarchical location path (level0-4+) that identifies this gateway. **Level 0 (enterprise) is mandatory**. Can follow ISA-95, KKS, or any organizational naming standard. |
| `communicator.apiUrl`           | `string`           | – (console-managed) | HTTPS endpoint of the Management Console.                                                      |
| `communicator.authToken`        | `string`           | –                   | API Key issued by the console. Can be set via `AUTH_TOKEN` env-var.                        |
| `communicator.allowInsecureTLS` | `bool`             | `false`             | Skip TLS verification [corporate-firewalls.md](../production/corporate-firewalls.md "mention") |

**Location levels**

| Index | Generic Level | ISA-95 Example     | KKS Example      | Other Examples  |
| ----- | ------------- | ------------------ | ---------------- | --------------- |
| `0`   | Enterprise    | `enterprise`       | `powerplant`     | `acme-inc`      |
| `1`   | Site/Region   | `site`/`plant`     | `unit-group`     | `cologne-plant` |
| `2`   | Area/Zone     | `area`/`line`      | `unit`           | `cnc-line`      |
| `3+`  | Work Cell+    | `work-cell`, `plc` | `component-grp`  | `plc123`        |

```yaml
agent:
  metricsPort: 9102
  communicator:
    apiUrl: "https://api.management.umh.app"
    authToken: "${AUTH_TOKEN}"
  location:
    0: acme-inc
    1: plant1
    2: press-shop
    
```

### DataFlow - Stand-alone Flows

A list of all [stand-alone flows](../usage/data-flows/stand-alone-flow.md) (UI: "Stand-alone Flows", YAML: `dataFlow:`). Each entry spins up one Benthos-UMH instance.

| Field                             | Type                  | Required | Description                                        |
| --------------------------------- | --------------------- | -------- | -------------------------------------------------- |
| `name`                            | `string`              | ✔        | Unique within the file.                            |
| `desiredState`                    | `active` \| `stopped` | ✔        | Agent will converge to this state.                 |
| `dataFlowComponentConfig.benthos` | object                | ✔        | Inline Benthos config (inputs, pipeline, outputs). |

```yaml
dataFlow:
- name: opcua-to-uns
  desiredState: active
  dataFlowComponentConfig:
    benthos:
      input:
        opcua:
          endpoint:  "opc.tcp://192.168.0.50:4840"
          nodeIDs:   ["ns=2;s=FolderNode"]
      pipeline:
        processors:
          - tag_processor:
              defaults: |
                msg.meta.location_path = "acme.plant1.press-shop.plc1"
                msg.meta.data_contract = "_raw"
                msg.meta.tag_name      = "value"
                return msg;
      output:
        uns: {}           # write into embedded Redpanda
```

**Quick field map**

| Path                  | Notes                                                                          |
| --------------------- | ------------------------------------------------------------------------------ |
| `input.*`             | Any supported Benthos-UMH input (OPC UA, S7comm, Modbus, MQTT, Kafka, …).      |
| `pipeline.processors` | Standard Benthos processors **plus** `tag_processor` & `nodered_js`.           |
| `output.*`            | Usually `uns: {}` for Unified Namespace. MQTT, HTTP, SQL, etc. also available. |

_For complete input/output syntax see [Benthos-UMH Documentation](https://docs.umh.app/benthos-umh)_

### Bridge – Bridge from Device to UNS

> 🚧 **Roadmap Item**: Bridges are under active development. Current functionality includes connection monitoring and basic read/write flows.

A [**bridge**](../usage/data-flows/bridges.md) (UI: "Bridges", YAML: `protocolConverter:`) ingests data from a field device (e.g. OPC UA server, Siemens S7, Modbus controller) and pushes it into the Unified Namespace (UNS), and vice versa.\

It combines a **connection probe** and two **stand-alone data flows (one for reading and one for writing)** under a single name and lifecycle.

```yaml
protocolConverter:
- name: press-opcua
  desiredState: active
  protocolConverterServiceConfig:
    location:
      2: press1 
    template:
      connection:
        nmap:
          target: "{{ .IP }}"
          port: "{{ .PORT }}"
      dataflowcomponent_read:
        benthos:
          input:
            opcua:
              endpoint: "opc.tcp://192.168.0.50:4840"
              nodeIDs: ["ns=2;s=MachineFolder"]
          pipeline:
            processors:
              - tag_processor:
                  defaults: |
                    msg.meta.location_path = "{{ .location_path }}"
                    msg.meta.data_contract = "_raw"
                    msg.meta.tag_name      = msg.meta.opcua_tag_name
                    return msg;
          output:
            uns: {}
    variables:
      # Optional templating support
      HOST: "192.168.0.50"
      PORT: "4840"
```

#### Key fields

| Field                                      | Description                                                                                 |
| ------------------------------------------ | ------------------------------------------------------------------------------------------- |
| `name`                                     | Unique ID for this converter instance                                                       |
| `desiredState`                             | `active` to run immediately, or `stopped` to keep it defined but off.                       |
| `protocolConverterServiceConfig.location`  | Appended to the global `agent.location`. Optional, but useful to identify per-machine data. |
| `template.connection.nmap`                 | TCP liveness check to decide if the device is reachable.                                    |
| `template.dataflowcomponent_read.benthos`  | Benthos pipeline to pull and forward data.                                                  |
| `template.dataflowcomponent_write.benthos` | Benthos pipeline to push and forward data.                                                  |
| `variables`                                | Optional Go-template variables, referenced via `{{ .VARNAME }}` in the template. See [Template Variables Reference](variables.md#variables) for details. |

### Benthos-UMH Processors

UMH Core includes specialized processors for industrial data:

#### tag_processor 🚧

> **🚧 Roadmap Item**: The current `tag_processor` implementation follows the benthos-umh pattern with tag names in payloads. With the next UMH Core release, `tag_processor` will be updated to align with the new data model where:
> - Tag names are only in topics (not in payloads)
> - Metadata is not included in message payloads
> - Output follows standard [timeseries payload format](../usage/unified-namespace/payload-formats.md)

For current implementation details, see [Benthos-UMH Tag Processor Documentation](https://docs.umh.app/benthos-umh/processing/tag-processor).

```yaml
pipeline:
  processors:
    - tag_processor:
        defaults: |
          msg.meta.location_path = "{{ .location_path }}";
          msg.meta.data_contract = "_raw";
          msg.meta.tag_name = "temperature";
          return msg;
```

**Data Contract Guidelines:**
- Use `_raw` for simple sensor data and initial device integration
- Use explicit contracts (e.g., `_temperature`, `_pump`) with [data models](../usage/data-modeling/README.md) 🚧 for structured enterprise data
- **Migration from UMH Classic:** See [Migration from UMH Classic to UMH Core](../production/migration-from-classic.md) for `_historian` contract migration instructions

For detailed documentation, see [Benthos-UMH Tag Processor](https://docs.umh.app/benthos-umh/processing/tag-processor).

#### nodered_js 🚧

Process data using Node-RED-style JavaScript:

```yaml
processors:
  - nodered_js:
      code: |
        msg.payload = msg.payload * 1.8 + 32;  // Convert C to F
        msg.topic = "temperature_fahrenheit";
        return msg;
```

For complete documentation, see [Benthos-UMH Node-RED JavaScript Processor](https://docs.umh.app/benthos-umh/processing/node-red-javascript-processor).

### Industrial Input Protocols

UMH Core supports 50+ industrial protocols via Benthos-UMH. For complete, up-to-date configuration examples, see [Benthos-UMH Input Documentation](https://docs.umh.app/benthos-umh/input/).

**Popular protocols include:**
- [OPC UA](https://docs.umh.app/benthos-umh/input/opc-ua-input) - Industry standard automation
- [Modbus](https://docs.umh.app/benthos-umh/input/modbus) - TCP/RTU serial communication  
- [Siemens S7](https://docs.umh.app/benthos-umh/input/siemens-s7) - Direct PLC access
- [Ethernet/IP](https://docs.umh.app/benthos-umh/input/ethernet-ip) - Allen-Bradley devices

**UMH Core Integration Pattern:**
```yaml
protocolConverter:
  - name: device-bridge
    protocolConverterServiceConfig:
      template:
        dataflowcomponent_read:
          benthos:
            input:
              # ANY benthos-umh protocol input
            pipeline:
              processors:
                - tag_processor: { /* UNS metadata setup */ }
            output:
              uns: {}
```

### Internal - Built-In Services (expert)

UMH-Core injects this section automatically.

```yaml
internal:
  redpanda:
    desiredState: active
    redpandaServiceConfig:
      defaultTopicRetentionMs: 604800000   # 7 days
      maxCores: 1
      memoryPerCoreInBytes: 2147483648
```

> **Do not edit** unless instructed by UMH support; invalid settings can brick the stack.

### Validation & tips

* **YAML anchors** are supported – useful for re-using input or output snippets.
* The Agent logs schema errors and refuses to activate a malformed DFC.

## Related Documentation

- **[Bridges](../usage/data-flows/bridges.md)** - Device connectivity patterns
- **[Stand-alone Flows](../usage/data-flows/stand-alone-flow.md)** - Custom data processing
- **[Data Modeling](../usage/data-modeling/README.md)** 🚧 - Structure your industrial data
- **[State Machines](state-machines.md)** - Component lifecycle management

## External References

- **[Benthos-UMH Documentation](https://docs.umh.app/benthos-umh)** - Complete protocol and processor reference
- **[Management Console](https://management.umh.app)** - Web-based configuration interface
