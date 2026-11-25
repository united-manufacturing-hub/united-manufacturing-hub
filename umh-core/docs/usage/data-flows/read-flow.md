# Read Flow

The read flow is the part of a [bridge](bridges.md) that reads from the external system and publishes what it reads to the Unified Namespace (UNS). It reads values from a PLC, an OPC UA server, a Modbus device or an MQTT broker, for example. It publishes each value as a message on one UNS topic at the bridge location.

A bridge can have only a read flow, only a write flow, or both. The read flow and the write flow start, stop and report their state separately. To configure a read flow in the Management Console, see [Set up a read flow](set-up-a-read-flow.md).

## What the Management Console shows

The bridge's overview page shows the read flow as its own card, next to the connection and the write flow. The card shows the state of the read flow and its throughput in messages per second. **Metrics** opens the message counts of the read flow, and **Logs** opens its log.

| State | Meaning |
|---|---|
| `active` | The read flow reads messages from the external system. |
| `idle` | The read flow runs, but has read no messages recently. |
| `degraded` | The read flow runs, but has hit errors. The log shows them. |
| `starting` | The read flow is starting. If it stays in this state, the log shows why. |
| `stopping` | The read flow is stopping. |
| `stopped` | The read flow is stopped with `readDFCDesiredState: stopped`, the bridge is stopped, or the bridge waits for its connection or for Redpanda. |

## How the read flow works

The read flow runs one [Benthos](https://docs.umh.app/benthos-umh) pipeline, a process that reads, changes and writes messages. Its input reads from the external system, its processors build the UNS topic of each message, and its output publishes the message to the UNS. You set the input and the processors. umh-core sets the output.

### The read flow starts only once the connection is up

umh-core starts the read flow once the [connection](connection-probe.md) is `up` and Redpanda is healthy. When the connection goes down, the read flow keeps running only while it still reads from the external system. Then umh-core stops it until the connection is `up` again. [How the connection affects the bridge](connection-probe.md#how-the-connection-affects-the-bridge) describes the bridge states.

### Every message needs a UNS topic

Each message the read flow publishes goes to one UNS topic. The topic is built from the bridge location, the data contract, an optional virtual path and the tag name, as described in [Topic Convention](../unified-namespace/topic-convention.md). The Management Console's protocol templates set these parts with a [`tag_processor`](https://docs.umh.app/benthos-umh/processing/tag-processor):

- `msg.meta.location_path`: the bridge location. umh-core replaces `{{ .location_path }}` with it.
- `msg.meta.data_contract`: the data contract, such as `_raw`, `_historian` or a contract of your own.
- `msg.meta.virtual_path`: optional levels between the data contract and the tag name.
- `msg.meta.tag_name`: the name of the value, such as the PLC address.

In the Management Console, each row of the address table sets these parts for one protocol address, such as `DB1.DW20`.

The `uns` output checks each message against the data model of its data contract and does not publish a message that breaks it. The read flow's log shows the error, and **Metrics** counts it as an output error. The output checks only data contracts that exist on the instance. The Management Console creates a data contract, such as `_pump_v1`, when you create a data model. In `config.yaml`, a data contract is an entry in `dataContracts:`. [Data Contracts](../data-modeling/data-contracts.md) explains the check. `_raw` and `_historian` are built in and have no data model.

### umh-core sets the output

The output of a read flow is always the `uns` output, which publishes to the UNS. umh-core replaces any `output` you set in `dataflowcomponent_read` with it. To write data to a system other than the UNS, use a [write flow](bridges.md) or a [stand-alone flow](stand-alone-flow.md).

## Reference

### Configuration

The read flow is the `dataflowcomponent_read` key of a bridge. Where that key sits depends on whether the bridge uses a template:

- A bridge created in the Management Console keeps its config in a template, in `templates.protocolConverter.<name>`. The read flow is in `templates.protocolConverter.<name>.dataflowcomponent_read`. The bridge references the template with `templateRef: <name>`.
- A bridge without `templateRef` keeps it under the bridge, in `protocolConverterServiceConfig.config.dataflowcomponent_read`.

With `templateRef`, umh-core uses the template and ignores a `config` key under the bridge. Edit the read flow in the template. Keep `readDFCDesiredState`, `location` and `variables` in `protocolConverterServiceConfig`, because umh-core ignores them in a template without an error.

This bridge reads two addresses from a Siemens S7 PLC and publishes them to the `_raw` data contract. The full bridge configuration is in [Bridges](bridges.md#configuration).

```yaml
protocolConverter:
  - name: my-plc-bridge
    desiredState: active
    protocolConverterServiceConfig:
      location:
        2: "production-line"
      config:
        connection:
          nmap:
            target: "{{ .IP }}"
            port: "{{ .PORT }}"
        dataflowcomponent_read:
          benthos:
            input:
              s7comm:
                addresses:
                  - DB1.DW20
                  - DB3.I270
                tcpDevice: "{{ .IP }}"
                rack: 0
                slot: 1
            pipeline:
              processors:
                - tag_processor:
                    defaults: |
                      msg.meta.location_path = "{{ .location_path }}";
                      msg.meta.data_contract = "_raw";
                      msg.meta.tag_name = msg.meta.s7_address;
                      return msg;
      variables:
        IP: "192.168.1.100"
        PORT: "102"
```

| Key | Default | Description |
|---|---|---|
| `dataflowcomponent_read.benthos.input` | none, required | The Benthos input that reads from the external system, such as `s7comm` or `opcua`. The [Benthos-UMH input documentation](https://docs.umh.app/benthos-umh/input) lists the inputs. Without an input, the read flow does not start. |
| `dataflowcomponent_read.benthos.pipeline` | none | The processors that run on every message, usually one `tag_processor`. |
| `dataflowcomponent_read.benthos.buffer`, `cache_resources`, `rate_limit_resources` | none | Benthos settings for buffering, caching and rate limits. See the [Benthos-UMH documentation](https://docs.umh.app/benthos-umh). |
| `dataflowcomponent_read.benthos.output` | the `uns` output | Ignored. umh-core always sets the `uns` output. |
| `readDFCDesiredState` | the bridge's `desiredState` | `active` or `stopped`. It sits in `protocolConverterServiceConfig`, next to `location`. `stopped` stops only the read flow, and the write flow keeps running. A bridge with `desiredState: stopped` always stops its read flow. |

Changes to these keys take effect without a restart of the container.

### Metrics

The **Metrics** action on the **Read Flow** card shows the message counts of the read flow. They are messages received and sent, errors and latency, for its input, processors and output. umh-core's Prometheus endpoint on port 8080 does not carry these counts. It carries the state and the errors of the read flow. [Read the metrics](../instances/agent/metrics.md#read-the-metrics) explains the state values and the label names in Prometheus.

On the Prometheus endpoint, the read flow of a bridge named `mixing-station` reports as `component="dataflow_component_instance"`, `exported_instance="read-protocolconverter-mixing-station"`. Its Benthos process reports as `component="benthos_instance"`, `exported_instance="dataflow-read-protocolconverter-mixing-station"`. This rule fires when a read flow is not active (3), idle (4) or stopped (0). Use it with `for: 5m`:

```promql
umh_core_service_current_state{component="dataflow_component_instance", exported_instance=~"read-protocolconverter-.*"} != 3 != 4 != 0
```

A read flow that has not started because its connection is not up reads 0, so this rule does not fire for it. To alert on an unreachable external system, use the bridge rule in the [Bridges metrics section](bridges.md#metrics).

The [Bridges metrics section](bridges.md#metrics) lists the labels of every part of a bridge. The definitions are in [`pkg/metrics/metrics.go`](https://github.com/united-manufacturing-hub/united-manufacturing-hub/blob/staging/umh-core/pkg/metrics/metrics.go).
