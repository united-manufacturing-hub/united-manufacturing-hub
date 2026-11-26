# Write Flow

The write flow is the part of a [bridge](bridges.md) that subscribes to UNS topics and writes to the external system. Typical external systems are:

- MES systems
- HTTP APIs
- Data lakes such as Snowflake
- PLCs on the shop floor
- The [Historian](../historian/README.md)

To configure a write flow in the Management Console, see [Set up a write flow](set-up-a-write-flow.md). When the Management Console rejects your topic patterns, or the bridge shows as degraded, see [Write Flow Topics Rejected](write-flow-topics-rejected.md).

A write flow can lose messages. Before you use one for data that must arrive, such as production orders or quality records, read [What happens when a write fails](#what-happens-when-a-write-fails).

{% hint style="warning" %}
**WARNING: Risk of unexpected machine or process operation**

Writing to an external system can affect the operation of connected machinery or processes. Incorrect writes can result in death, serious injury or equipment damage.

**UMH does not provide safety functions.** Data delivery guarantees do not make UMH suitable for use as part of a safety function. Safety functions must be implemented independently of UMH using appropriate safety-rated controllers and devices, correctly programmed and validated for the application.
{% endhint %}

## How a write flow works

A write flow has three parts. Each part is a card on the **Write Flow** tab of the bridge editor:

1. **UMH Topics**: the topic patterns the write flow subscribes to. A topic pattern is a regular expression that matches one or more UNS topics. Each topic pattern is [validated](#topic-validation).
2. **Processing**: JavaScript that runs on every message before it is written.
3. **Destination**: the settings of the external system the message is written to, such as the URL of an HTTP API.

In this example, a bridge's read flow publishes pump data into the UNS. Its write flow subscribes to that data and writes it to an MES:

```mermaid
flowchart LR
    PLC["<b>Pump PLC</b><br/>shop floor"]
    R["<b>Read flow</b><br/>reads the PLC"]
    UNS["<b>UNS</b><br/>_pump_v1 data"]
    subgraph W["Write flow"]
        W1["<b>UMH Topics</b><br/>subscribes to _pump_v1<br/>at the bridge location"]
        W2["<b>Processing</b><br/>shapes each<br/>message"]
        W3["<b>Destination</b><br/>writes it to<br/>the MES"]
        W1 --> W2 --> W3
    end
    MES["<b>MES</b><br/>test system"]
    PLC --> R --> UNS --> W1
    W3 --> MES
```

The read flow and the write flow of a bridge start, stop and report health separately. A bridge can also have only a write flow. A [Historian bridge](../historian/save-to-historian.md) is a bridge of that kind.

A write flow does not publish anything back into the UNS, and the answer of the external system is not passed on. To call an MES and publish its answer into the UNS, use a [stand-alone flow](stand-alone-flow.md).

Each card has a **Code** mode. **UMH Topics** also has a **Visual** mode, and **Processing** and **Destination** have one for some protocols. In **Code** mode, a card shows the text that is stored in `config.yaml`. [Configuration](#configuration) lists the key of each card.

## What the Management Console shows

The bridge's overview page has a **Write Flow** card. It shows the health of the write flow and its throughput in msg/sec. **Logs** on the card shows the log of the write flow, including errors from the external system.

When a topic pattern breaks the [topic rules](#topic-validation), the write flow does not start and the bridge shows as degraded. Its status starts with "flow degraded:", or with "connection degraded:" while the connection is down. The connection and the read flow keep running. [Write Flow Topics Rejected](write-flow-topics-rejected.md) explains the messages and how to fix them.

## What happens when a write fails

When a write fails for good, the write flow drops the message and goes on with the next one. UMH Core keeps no copy, so you cannot inspect or resend it.

Two terms describe this:

- An **acknowledgement** tells the UNS that the write flow has handled a message, so the message is not delivered again.
- A **dead-letter queue** holds the messages that could not be written, so you can inspect or replay them instead of losing them.

UMH Core adds no retry and no dead-letter queue of its own. What happens on a failed write depends on the output, the part of the write flow that writes to the external system. The **Destination** card links to its documentation.

- Whether and how often the output retries a failed write is described in its documentation.
- When the output gives up, it does not acknowledge the message. The write flow goes on with the next messages. The failed message arrives again only if the write flow restarts before a later message is written.
- A failed write may still have reached the external system, for example when an HTTP request arrived but its response was lost. Check the external system before you write the message again.

Two more cases lose messages:

- When the **Processing** code throws an error, the write flow drops the message and logs the error.
- A `buffer` in `extra.code` acknowledges a message as soon as it stores it, before the output writes it. A `memory` buffer loses the messages it holds when the write flow restarts.

The [Historian](../historian/save-to-historian.md) output retries a failed write until it succeeds. It drops only rows that can never be written, such as a value of the wrong data type.

To keep the messages that could not be written, build a dead-letter queue with the Redpanda Connect [`fallback` output](https://docs.redpanda.com/redpanda-connect/components/outputs/fallback/). Set `destination.protocol` to `fallback`, and list in `destination.code` the output to the external system first and then an output that stores the failed messages, such as `file`.

## Topic validation

From umh-core 0.45.0, UMH Core checks every topic pattern of a write flow. In **Visual** mode the Management Console builds topic patterns that pass the checks. The rules matter only when you write topic patterns in **Code** mode or in `config.yaml`. [Topic Convention](../unified-namespace/topic-convention.md) explains the parts of a UNS topic, and [Topic patterns](../unified-namespace/topic-convention.md#topic-patterns) explains the regular expression syntax.

UMH Core accepts a topic pattern only if it follows three rules:

1. **The topic pattern stays at the bridge location or below it.** The bridge location is the instance location plus the bridge's own levels. You set those levels on the **General** tab. A bridge at `acme.cologne.packaging` may subscribe to topics at `acme.cologne.packaging` and `acme.cologne.packaging.line-1`. It may not subscribe to topics at `acme.cologne.assembly` or `acme.cologne`, unless the [emergency setting](write-flow-topics-rejected.md#emergency-setting-bypass-write-flow-validation) is on.
2. **The topic pattern names one [data contract](../data-modeling/data-contracts.md), in one or more versions.** The built-in contracts are `_raw` and `_historian`. Any other contract must exist on the instance with a version, such as `_pump_v1`. List several versions explicitly, such as `_pump_v[1-2]`. Each listed version must exist. A wildcard for any contract (`_.*`) or any version (`_pump_v\d+`) is rejected.
3. **The topic pattern starts with `^`.** `^` ties the pattern to the start of the topic. Without `^`, the pattern also matches topics that contain it anywhere.

Without the emergency setting, data from another location reaches a bridge only if a [stream processor](stream-processor.md) or another flow publishes that data at the bridge location first.

The diagram shows at which locations a bridge at `acme.cologne.packaging` may subscribe to topics:

```mermaid
flowchart TD
    A["acme"] --> C["acme.cologne ✗"]
    C --> P["acme.cologne.packaging ✓<br/>(bridge location)"]
    C --> S["acme.cologne.assembly ✗"]
    P --> L1["acme.cologne.packaging.line-1 ✓"]
    P --> L2["acme.cologne.packaging.line-2 ✓"]
```

After the data contract, a topic pattern can match every tag or only some of them. `{{ .location_path }}` stands for the bridge location:

| You want | Topic pattern |
|---|---|
| All `_pump_v1` tags at the bridge location | `^umh\.v1\.{{ .location_path }}\._pump_v1\.` |
| All `_pump_v1` tags at the bridge location and every level below it | `^umh\.v1\.{{ .location_path }}\.(?:[^._][^.]*\.)*_pump_v1\.` |
| Only the `pressure` tag | `^umh\.v1\.{{ .location_path }}\._pump_v1\.pressure$` |
| Versions 1 and 2 of `_pump`, both on the instance | `^umh\.v1\.{{ .location_path }}\._pump_v[1-2]\.` |

A new version of a data contract does not reach the write flow on its own. Add the version to the topic pattern, for example by changing `_pump_v1` to `_pump_v[1-2]`.

## Reference

### Configuration

A write flow is stored in `config.yaml` in the section `dataflowcomponent_write`. Where that section sits depends on whether the bridge uses a template:

- A bridge created in the Management Console keeps its whole config in a template, in `templates.protocolConverter.<name>`, where `<name>` is the bridge's name. The write flow is in `templates.protocolConverter.<name>.dataflowcomponent_write`. The bridge references the template with `templateRef: <name>`.
- A bridge without `templateRef` keeps it under the bridge, in `protocolConverterServiceConfig.config.dataflowcomponent_write`.

Each card on the **Write Flow** tab stores its text in one key. UMH Core turns the three cards into one Benthos pipeline: a `uns` input, one processor and one output.

| Card | Code mode shows | `config.yaml` key | Benthos part |
|---|---|---|---|
| **UMH Topics** | The topic patterns, one per line | `source.topics` | The `uns` input, with the topic patterns in `umh_topics` |
| **Processing** | The JavaScript | `processing.code` | One `nodered_js` processor |
| **Destination** | The settings of the output, as YAML | `destination.code` | The output named in `destination.protocol`, such as `http_client` |

The Management Console writes these sections for you and sets `destination.protocol`. Edit them by hand only on the instance's [Config File](../instances/config-file.md) page.

This bridge has only a write flow. It writes every `_pump_v1` tag at its bridge location and below to a test MES over HTTP. A bridge with only a write flow still needs a connection probe:

```yaml
protocolConverter:
  - name: pump-to-mes
    desiredState: active
    protocolConverterServiceConfig:
      location:
        2: packaging
      writeDFCDesiredState: active
      config:
        connection:
          nmap:
            target: "{{ .IP }}"
            port: "{{ .PORT }}"
        dataflowcomponent_write:
          source:
            topics: |-
              ^umh\.v1\.{{ .location_path }}\.(?:[^._][^.]*\.)*_pump_v1\.
          processing:
            type: nodered_js
            code: |-
              msg.payload = {
                topic: msg.meta.umh_topic,
                value: msg.payload.value,
                timestamp_ms: msg.payload.timestamp_ms
              };
              return msg;
          destination:
            protocol: http_client
            code: |-
              url: http://{{ .IP }}:{{ .PORT }}/api/measurements
              verb: POST
      variables:
        IP: "mes-test.example"
        PORT: "8080"
```

Created in the Management Console, the same bridge looks like this. `config` moves to the template, and the bridge keeps `location`, `writeDFCDesiredState` and `variables`:

```yaml
templates:
  protocolConverter:
    pump-to-mes:
      connection:
        nmap:
          target: "{{ .IP }}"
          port: "{{ .PORT }}"
      dataflowcomponent_write:
        source:
          topics: |-
            ^umh\.v1\.{{ .location_path }}\.(?:[^._][^.]*\.)*_pump_v1\.
        # processing and destination as in the example above
protocolConverter:
  - name: pump-to-mes
    desiredState: active
    protocolConverterServiceConfig:
      templateRef: pump-to-mes
      location:
        2: packaging
      writeDFCDesiredState: active
      variables:
        IP: "mes-test.example"
        PORT: "8080"
```

The table lists the keys that control the write flow:

| Key | Default | Description |
|---|---|---|
| `source.topics` | none | The topic patterns, one per line. Without topic patterns, the write flow subscribes to no topics. The Management Console rejects a write flow that has a destination and no topic patterns. A line may start with `- `. Template variables such as `{{ .location_path }}` are inserted as literal text, so a variable cannot add pattern syntax to a topic pattern. |
| `processing.type` | `nodered_js` | The processor that runs `processing.code`. `nodered_js` is the only supported processor. |
| `processing.code` | `return msg;` | The JavaScript that runs on every message. |
| `destination.protocol` | none | The output, such as `http_client` or `historian`. A write flow without it does not start. |
| `destination.code` | none | The settings of the output, as YAML. |
| `extra.code` | none | Advanced settings as YAML. Only `cache_resources`, `rate_limit_resources` and `buffer` are used. Other keys are ignored. See the [Benthos-UMH documentation](https://docs.umh.app/benthos-umh). |
| `BYPASS_WRITE_FLOW_VALIDATION` | off | Environment variable. Lifts the location rule, and the rule that a data contract written out in full must exist with a version, for every bridge on the instance. A version range such as `_pump_v[1-2]` stays rejected while one of its versions is missing. UMH Core reads it once at startup, so a change needs a new container. `docker restart` keeps the old value. See [Emergency setting](write-flow-topics-rejected.md#emergency-setting-bypass-write-flow-validation). |
| `writeDFCDesiredState` | the bridge's `desiredState` | `active` or `stopped`. It sits in `protocolConverterServiceConfig`, next to `location`. `stopped` stops only the write flow. `active` is a request: the write flow starts only when it has a `destination.protocol` and the connection probe is up. A bridge with `desiredState: stopped` always stops its write flow. |

Changes to the `config.yaml` keys take effect without a restart. UMH Core checks the topic patterns each time it loads the configuration. A topic pattern that breaks the rules keeps the write flow from starting, and the bridge shows as degraded. A stopped write flow, or one without a destination, is not checked.

### Metrics

The **Metrics** action on the **Write Flow** card opens the bridge metrics for the write flow. They show messages received and sent, errors, and latency.

For a bridge named `mixing-station`, the write flow's series carry these labels. [Bridges: Metrics](bridges.md#metrics) explains the labels of the other parts of the bridge.

| Part of the bridge | `component` | `instance` |
|---|---|---|
| Its write flow | `dataflow_component_instance` | `write-protocolconverter-mixing-station` |
| The Benthos process of its write flow | `benthos_instance` | `dataflow-write-protocolconverter-mixing-station` |

To alert when a write flow is in any state other than active (3), idle (4) or stopped (0) for 5 minutes, use this rule with `for: 5m`:

```promql
umh_core_service_current_state{component="dataflow_component_instance", exported_instance=~"write-protocolconverter-.*"} != 3 != 4 != 0
```

This rule does not catch a write flow whose topic patterns break the rules, or whose connection is down, because such a write flow is stopped. Use the bridge rule in [Bridges: Metrics](bridges.md#metrics) for those cases: a degraded bridge reads -1.

[Read the metrics](../instances/agent/metrics.md#read-the-metrics) explains the state values and why Prometheus shows `instance` as `exported_instance`. The definitions are in [`pkg/metrics/metrics.go`](https://github.com/united-manufacturing-hub/united-manufacturing-hub/blob/staging/umh-core/pkg/metrics/metrics.go).
