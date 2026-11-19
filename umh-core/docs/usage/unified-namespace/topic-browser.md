# Topic Browser

The Topic Browser shows every topic in your Unified Namespace as a tree, with the current value and the recent history of the topic you select. Use it to find a data point, to see where it sits in your hierarchy, and to check what it last reported. It runs in the Management Console at `management.umh.app`, under **Topic Browser**; this page is not about the Topic Browser of UMH Classic. To read topics from a program, use the [GraphQL API](#graphql-api).

If the tree shows `No topics found. Ensure your UMH instance is connected and generating topics.` while your instance is producing data, or a topic you expect never appears, see [Topic does not appear in the Topic Browser](topic-does-not-appear-in-the-topic-browser.md).

The Topic Browser service runs on every instance by default. Setting `internal.topicbrowser.desiredState` to `stopped` in the instance configuration turns the service off: the instance then collects no topics, and both the tree and the [GraphQL API](#graphql-api) stop updating. [Configuration reference](../../reference/configuration-reference.md) lists the setting.

## The main view

The screen splits into two panels. The left panel shows your topics as a tree. The right panel shows the topic you select: its metadata, its current value and its history. Both panels update on their own, without a reload.

![The Topic Browser, with a numeric topic selected](../../getting-started/images/2-topic-browser-DB1.DW20.png)

The selected topic in the example is a number, so its history is a line chart.

## The topic tree

The tree follows your topic structure, which [Topic Convention](topic-convention.md) explains: the location path comes first — enterprise, site, area, line, machine — then the data contract, then the tag. A virtual path shows up as further folders between the contract and the tag. Open each level to drill down from the enterprise to a single tag.

The tree combines the topics of all your instances: each instance runs its own Topic Browser service and reports its own topics, and the Management Console merges them into one tree.

The search bar narrows the tree: it matches parts of topic names and of metadata, so `temp` finds every topic with "temp" in its name or metadata.

A namespace with more than 10,000 topics can load progressively instead of all at once.

## The details panel

Selecting a topic shows its details on the right:

| Field | Shows |
|---|---|
| **Location** | The topic's path through your hierarchy, from the enterprise down to the machine. |
| **Data contract** | The contract applied to the data, for example `_raw` or a device model such as `_pump_v1`. |
| **Instance** | The instance that reported the topic. |
| **Bridge** | The bridge or stream processor that sends the topic. |
| **Current value** | The latest value, with the time it was measured and the time it reached UMH. |
| **Metadata** | The headers of the last message, including the original device tag and the bridge that sent it. [Metadata and Tracing](metadata-and-tracing.md) explains each field. |
| **History** | The last 100 values of the topic, as a line chart for numbers and as a table for booleans and strings. |

When values arrive faster than the display can draw them, they are batched. A relational topic, for example `_erp.orders`, shows the payload of its last message and no history.

## Metrics

### The Topic Browser card

The **Topic Browser** card on the instance's detail page shows one value. **Topics** counts the different topics the instance has seen since umh-core started. Once a topic stops receiving messages it stays in the count, and when umh-core restarts the count starts from zero again. The **Metrics** button on the card opens the counters of the Benthos process that consumes the Kafka topic `umh.messages` from [Redpanda](redpanda.md).

While that process runs, the card's badge reads **Active**. Hover the badge to see the reason for a failure: `benthos degraded` when the Benthos process fails, `redpanda degraded` when Redpanda is degraded. The Redpanda card shows the cause of a Redpanda failure; see [When Redpanda is degraded](redpanda.md#when-redpanda-is-degraded).

### The Prometheus endpoint

The Topic Browser reports on port 8080 and runs on FSMv1, so it uses the standard [FSMv1 metrics](../instances/agent/metrics.md#fsmv1-metrics). To read them, see [Metrics](../instances/agent/metrics.md). Its series carry these labels:

| Part of the Topic Browser | `component` | `instance` |
|---|---|---|
| The Topic Browser | `topic_browser_instance` | `topic-browser` |
| Its Benthos process | `benthos_instance` | `topicbrowser-topic-browser` |

In Prometheus, the `instance` label appears as `exported_instance` unless the scrape job sets `honor_labels: true`.

To raise an alert while the Benthos process of the Topic Browser is degraded, use:

```promql
umh_core_service_current_state{component="benthos_instance", exported_instance="topicbrowser-topic-browser"} == 5
```

The state definitions live in [`pkg/metrics/metrics.go`](https://github.com/united-manufacturing-hub/united-manufacturing-hub/blob/staging/umh-core/pkg/metrics/metrics.go).

## GraphQL API

The GraphQL API is documented on its own reference page. For the endpoint, the schema, example queries and every configuration option, see [Topic Browser GraphQL API](../../reference/http-api/topic-browser-graphql.md). Old links to `#graphql-api` land here.
