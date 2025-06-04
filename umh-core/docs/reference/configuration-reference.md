# Configuration Reference

This is the reference for the central config `/data/config.yaml`

> **File location:** The container mounts `/data` as a writable volume.\
> **Hot-reload:** The Agent polls the file every tick; valid changes are applied automatically.

### Agent - Runtime & Device identity

| Field                           | Type               | Default             | Purpose                                                                                        |
| ------------------------------- | ------------------ | ------------------- | ---------------------------------------------------------------------------------------------- |
| `metricsPort`                   | `int`              | **9102**            | Exposes Prometheus metrics for the container.                                                  |
| `location`                      | map `int → string` | –                   | ISA-95 hierarchy that identifies this gateway. **Level 0 must exist**.                         |
| `communicator.apiUrl`           | `string`           | – (console-managed) | HTTPS endpoint of the Management Console.                                                      |
| `communicator.authToken`        | `string`           | –                   | API Key issued by the console. Can be set via `UMH_AUTH_TOKEN` env-var.                        |
| `communicator.allowInsecureTLS` | `bool`             | `false`             | Skip TLS verification [corporate-firewalls.md](../production/corporate-firewalls.md "mention") |

**Location levels**

| Index | Typical meaning                  | Example         |
| ----- | -------------------------------- | --------------- |
| `0`   | Enterprise                       | `acme-inc`      |
| `1`   | Site / Plant                     | `cologne-plant` |
| `2`   | Area / Line                      | `cnc-line`      |
| `3+`  | Free-form (work-cell, PLC id, …) | `plc123`        |

```yaml
agent:
  metricsPort: 9102
  communicator:
    apiUrl: "https://api.management.umh.app"
    authToken: "${UMH_AUTH_TOKEN}"
  location:
    0: acme-inc
    1: plant1
    2: press-shop
    
```

### DataFlow - Stand-alone Flows

A list of all [stand-alone flows](../usage/data-flows/stand-alone-flow.md) (formerly called "Dataflow Components). Each entry spins up one Benthos-UMH instance.

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
                msg.meta.data_contract = "_historian"
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

_For full  syntax see_ [Broken link](broken-reference "mention")

### Bridge – Bridge from Device to UNS

> 🚧 **Coming Soon**: Bridges are under active development.

A [**bridge**](../usage/data-flows/bridges.md) (formerly known as protocol converter) ingests data from a field device (e.g. OPC UA server, Siemens S7, Modbus controller) and pushes it into the Unified Namespace (UNS), and vice versa.\
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
          target: "{{ .HOST }}"
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
                    msg.meta.data_contract = "_historian"
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
| `variables`                                | Optional Go-template variables, referenced via `{{ .VARNAME }}` in the template.            |

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
