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
| Bridge | Protocol Converter | `protocolConverter:` | Reads from and writes to an external system, with a connection probe |
| Stand-alone Flow | Data Flow Component (DFC) | `dataFlow:` | Point-to-point data processing pipelines |
| Stream Processor | Stream Processor | `streamProcessor:` | Processes data within UNS |

> **Note:** YAML configuration keys retain legacy names for backward compatibility. The Management Console UI uses the current terminology.

### Agent - Runtime & Device identity

| Field                           | Type               | Default             | Purpose                                                                                        |
| ------------------------------- | ------------------ | ------------------- | ---------------------------------------------------------------------------------------------- |
| `metricsPort`                   | `int`              | **8080**            | Port of the Prometheus metrics. Fixed at 8080: umh-core sets this value on every start, so a different value has no effect. See [Metrics](../usage/instances/agent/metrics.md). |
| `location`                      | map `int → string` | –                   | Hierarchical location path (level0-4+) that identifies this instance. **Level 0 (enterprise) is mandatory**. Can follow ISA-95, KKS, or any organizational naming standard. |
| `communicator.apiUrl`           | `string`           | – (console-managed) | HTTPS endpoint of the Management Console.                                                      |
| `communicator.authToken`        | `string`           | –                   | API Key issued by the console. Can be set via `AUTH_TOKEN` env-var.                            |
| `communicator.allowInsecureTLS` | `bool`             | `false`             | Accepted and ignored. Certificate verification is always enforced. Leaving this key in place is harmless.                            |
| `enableResourceLimitBlocking`   | `bool`             | `true`, also when config.yaml does not contain the setting | Whether bridge admission is on. While it is `true`, a new bridge waits when a resource is degraded, when resource health is not proven yet, or when the bridge limit is reached. `false` is the emergency setting *Turn off bridge admission*: new bridges start anyway, and the instance shows as degraded while it is `false`. See [Bridges Do Not Start](../usage/data-flows/bridges-do-not-start.md). Takes effect without a restart. |

**Location levels**

| Index | Generic Level | ISA-95 Example     | KKS Example      | Other Examples  |
| ----- | ------------- | ------------------ | ---------------- | --------------- |
| `0`   | Enterprise    | `enterprise`       | `powerplant`     | `acme-inc`      |
| `1`   | Site/Region   | `site`/`plant`     | `unit-group`     | `cologne-plant` |
| `2`   | Area/Zone     | `area`/`line`      | `unit`           | `cnc-line`      |
| `3+`  | Work Cell+    | `work-cell`, `plc` | `component-grp`  | `plc123`        |

```yaml
agent:
  metricsPort: 8080
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
| `debug_level` | `bool` | ✗ | Enable verbose logging. When `true`, enables DEBUG log level and detailed operational logging (e.g., input/output operations, processing steps, data transformations). When `false` (default), uses INFO log level. **Use only for troubleshooting** - debug logging generates significantly more log data and may impact performance. |
| `dataFlowComponentConfig.benthos` | object                | ✔        | Inline Benthos config (inputs, pipeline, outputs). |

```yaml
dataFlow:
- name: opcua-to-uns
  desiredState: active
  debug_level: false  # Set to true for detailed logging while troubleshooting
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

umh-core passes only these top-level keys of `benthos` to Benthos: `input`, `pipeline`, `output`, `buffer`, `cache_resources` and `rate_limit_resources`. It drops any other key without an error, for example `processor_resources`. The same holds for the `benthos` of a bridge's read and write flows. Define processors inline under `pipeline.processors` instead.

_For complete input/output syntax see [Benthos-UMH Documentation](https://docs.umh.app/benthos-umh)_

### Bridge

Bridges (YAML: `protocolConverter:`) are configured on the [Bridges](../usage/data-flows/bridges.md#configuration) page.

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
- Use explicit contracts (e.g., `_temperature`, `_pump`) with [data models](../usage/data-modeling/README.md) for structured enterprise data
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
      config:
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

### GraphQL API - Topic Browser 🚧

GraphQL API for querying Unified Namespace topics.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | `bool` | `false` | Enable GraphQL API (Early Access; opt in) |
| `port` | `int` | `8090` | HTTP port for GraphQL endpoint |
| `debug` | `bool` | `false` | Enable GraphiQL playground at `/` |
| `corsOrigins` | `[]string` | `[]` | CORS origins (empty = allow all) |

```yaml
agent:
  graphql:
    enabled: false   # Default: false; set true to opt in (Early Access)
    port: 8090
    debug: false
    corsOrigins: 
      - "http://localhost:3000"
      - "https://your-app.com"
```

**Endpoints:**
- **GraphQL API:** `POST /graphql`
- **GraphiQL Playground:** `GET /` (debug mode only)
- **CORS Preflight:** `OPTIONS /graphql`

For complete API reference, see [Topic Browser GraphQL API](http-api/topic-browser-graphql.md).

### Internal - Built-In Services (expert)

UMH Core injects this section automatically.

```yaml
internal:
  redpanda:
    desiredState: active
    redpandaServiceConfig:
      topic:
        defaultTopicRetentionMs: 604800000   # 7 days
        defaultTopicCleanupPolicy: delete    # keep 7 days of raw records
      resources:
        maxCores: 1
        memoryPerCoreInBytes: 2147483648
  topicbrowser:
    desiredState: active  # Topic Browser service (auto-enabled)
```

> **Do not edit** unless instructed by UMH support; invalid settings can brick the stack.

#### Topic retention on new instances

On its very first boot — when there is no `config.yaml` in `/data` yet — an instance writes
`defaultTopicCleanupPolicy: delete` and `defaultTopicRetentionMs: 604800000` into the file
it creates. Together these keep seven days of raw records in `umh.messages`, so a new
consumer group can replay the full history; the Historian and any other late-joining
consumer depend on this.

Two consequences to be aware of:

* **Disk usage scales with throughput, not tag count.** Seven days of raw records is far
  more than a compacted snapshot. See the [Sizing Guide](../production/sizing-guide.md)
  before running a high-throughput instance, and cap the topic with
  `defaultTopicRetentionBytes` if disk is tighter than the retention window.
* **The Topic Browser reads the retained window only when it has no committed offset,**
  normally on its first run. After that it resumes from the offset it committed, so a
  restart does not replay older records.

Instances that already have a `config.yaml` keep the policy they run on today, which is
`compact` unless it was set explicitly — upgrading never changes their retention. To move
an existing instance to seven-day replay, set `defaultTopicCleanupPolicy: delete` yourself.
Records that compaction already collapsed cannot be recovered.

### Validation & tips

* **YAML anchors** are supported – useful for re-using input or output snippets.
* The Agent logs schema errors and refuses to activate a malformed DFC.

## Related Documentation

- **[Bridges](../usage/data-flows/bridges.md)** - Read from and write to external systems
- **[Stand-alone Flows](../usage/data-flows/stand-alone-flow.md)** - Custom data processing
- **[Data Modeling](../usage/data-modeling/README.md)** - Structure your industrial data
- **[State Machines](state-machines.md)** - Component lifecycle management

## External References

- **[Benthos-UMH Documentation](https://docs.umh.app/benthos-umh)** - Complete protocol and processor reference
- **[Management Console](https://management.umh.app)** - Web-based configuration interface
