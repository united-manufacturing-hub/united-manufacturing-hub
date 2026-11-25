# Topic Browser

The Topic Browser shows your topics as a tree, with the current value and the recent history of the topic you select. Use it to find a data point and to check what it last reported. It runs in the Management Console under **Use** → **Topic browser**. This page is not about the Topic Browser of UMH Classic. To read topics from a program, see [Topic Browser GraphQL API](../../reference/http-api/topic-browser-graphql.md).

If the tree stays empty while your instance is producing data, or a topic you expect never appears, see [Topic does not appear in the Topic Browser](topic-does-not-appear-in-the-topic-browser.md). If a value changes at uneven times or its age grows longer than you expect, see [Values are not updating at regular intervals](values-are-not-updating-at-regular-intervals.md).

## How live updates work

A topic reaches the tree in three steps:

1. A [bridge](../data-flows/bridges.md) or [stream processor](../data-flows/stream-processor.md) publishes a message for the topic to the [Unified Namespace](README.md) of its instance.
2. The Topic Browser of that instance reads the message and keeps the latest value of every topic it has seen.
3. The Topic Browser collects what it reads into batches and sends each batch to the Management Console as an update. Each batch carries at most one value per topic, the latest. While the instance is offline, no updates arrive.

The Topic Browser aims for an interval of 1 to 15 seconds between two batches. It sets the interval from the amount of data it has recently read and the CPU use of the Topic Browser process: more data and low CPU use shorten it, less data and high CPU use lengthen it. A new value can take longer than the interval to show: the Topic Browser checks the interval only when new messages arrive, and the update then still has to reach the Management Console.

While UMH Core runs, a topic stays in the tree even after it stops sending data. After UMH Core restarts, the instance reports each topic again once a message for it arrives, so a topic that sends data rarely can take a while to come back. A Management Console tab that stayed open during the restart can keep showing the older topics, with their age growing. To see only the topics reported since the restart, sign out of the Management Console and sign back in.

## The Topic Browser screen

The screen splits into two panels. The left panel searches and shows your topics as a tree. The right panel shows the topic you select. Both update on their own, without a reload.

![The Topic Browser with a numeric topic selected](../../generated/topic-browser-main.png)

The instance filter in the Management Console header decides which instances the tree draws from. The filter applies to every page and accepts several instances. Its menu is headed **Filtering N of M instances**.

## The topic tree

The tree follows your topic structure, which [Topic Convention](topic-convention.md) explains: the location path comes first (enterprise, site, area, line, machine), then the data contract, then the tag. A data contract is the schema applied to the data, for example `_raw` or a device model such as `_metal_forming_v1`. A virtual path, if the topic has one, shows up as further folders between the contract and the tag.

Each branch shows the number of topics beneath it. Individual topics show no count.

The search box narrows the tree. It matches parts of topic names.

## The selected topic

The right panel opens with the full topic string and a button that copies it. Below it:

| Field | Shows |
|---|---|
| **Current value** | The latest value, with how long ago it arrived beside it. |
| **Instance** | The instance that reported the topic, as a link. When several instances report it, a dropdown lists all of them. An instance is marked `(offline)` when it is not reachable, and `(not in the filter)` when the header filter excludes it. |
| **Bridge** or **Stream** | What produced the topic. The row reads **Stream** when a stream processor produced it and **Bridge** when a protocol converter did. It is absent when neither did. |
| **Data contract** | The contract applied to the data. |

A numeric topic also draws a chart of its recent values, with a note of the window they cover.

Below that are two tabs, **History** and **Details**. A third, **SQL query**, appears when the historian feature is enabled on your instance.

### History

The History tab lists the values the Topic Browser is holding: the last 100 of that topic, newest first.

The instance sends at most one value per topic per batch, as [How live updates work](#how-live-updates-work) describes. Other values measured between two batches do not reach the Topic Browser, so the History tab holds a sample of the signal. [Values are not updating at regular intervals](values-are-not-updating-at-regular-intervals.md) explains which gaps are normal. To keep every reading, save the data contract to the historian database with a Historian bridge: see [Save data to the Historian](../historian/save-to-historian.md).

A text or boolean topic has no chart. Its History tab has two views instead. **Changes** collapses runs of the same value into one row, with the time it held and how many values it stored. **All values** lists them individually.

![A text topic, showing the Changes view of its history](../../generated/topic-browser-text-topic.png)

### Details

The Details tab describes the last message of the topic.

| Field | Shows |
|---|---|
| **Virtual path** | The folders between the data contract and the tag, if the topic has any. |
| **Payload type** | `Time-Series` for a single value, or `Relational` for a JSON object. [Payload Formats](payload-formats.md) explains both. |
| **Data type** | `number`, `string`, `boolean`, `JSON` for a relational topic, or `unknown`. |
| **Value** | The value itself. A relational topic shows no **Value** or **Measured** row. |
| **Measured** | When the device recorded it. |
| **Arrived in UMH** | When UMH received it. The gap between the two is your pipeline's latency. |
| **Headers** | The Kafka headers of the message, including what produced it, the data contract, the location path and the original tag name. The section is absent when the message carries no headers. [Metadata and Tracing](metadata-and-tracing.md) explains each field. |

![The Details tab, showing the fields and the message headers](../../generated/topic-browser-details-panel.png)

### SQL query

The SQL query tab writes the query that reads this tag out of the historian database, for **Grafana** or for **TimescaleDB** directly. The tab appears only when the historian feature is enabled on your instance.

**Aggregate** is a switch. Turn it on and choose an **Interval**, and the query groups the readings into buckets of that length, returning the average, minimum and maximum of each.

![The SQL query tab, with a Grafana query for the selected tag](../../generated/topic-browser-sql-query.png)

The query returns rows only once the tag has been saved to the historian database. Until then it returns nothing, even when the Topic Browser shows values for the tag.

## Relational topics

A relational topic, for example `_erp.orders`, carries a JSON document. It shows **Latest payload** in place of a current value, and has two tabs, **Payload** and **Details**. The Topic Browser keeps only the most recent payload, so there is no history and no chart.

![A relational topic, showing its latest JSON payload](../../generated/topic-browser-relational-topic.png)

## Topics from several instances

Each instance runs its own Topic Browser and reports its own topics, and the Management Console merges them into one tree. When more than one instance reports the same topic, the **Instance** dropdown lists all of them.

![A topic reported by three instances, one of them offline](../../generated/topic-browser-several-instances.png)

## The Topic Browser card

The **Topic Browser** card on the instance's detail page shows one value. **Topics** counts the different topics the instance has seen since umh-core started. Once a topic stops receiving messages it stays in the count. When umh-core restarts, the count can start from zero again. The **Metrics** button on the card opens the counters of the [Benthos](../data-flows/bridges.md) process that consumes the Kafka topic `umh.messages` from [Redpanda](redpanda.md). The **Logs** button opens the log of the same process.

While that process runs, the card's badge reads **Active**. The information icon beside the badge gives the state of the Topic Browser itself, for example `benthos degraded` when the Benthos process fails or `redpanda degraded` when Redpanda is degraded. The badge can read **Active** while Redpanda is degraded, so read the information icon too. The Redpanda card shows the cause of a Redpanda failure; see [When Redpanda is degraded](redpanda.md#when-redpanda-is-degraded).

## Reference

### Configuration

One key controls the Topic Browser, in the `config.yaml` of the instance. [Configuration reference](../../reference/configuration-reference.md) lists it with the rest of `internal` and warns not to edit that section without guidance from UMH support.

| Key | Default | Effect |
|---|---|---|
| `internal.topicbrowser.desiredState` | `active` | `stopped` turns off the Topic Browser on this instance: it collects no topics, and both the tree and the [GraphQL API](#graphql-api) stop updating. The change takes effect without a restart. |

### Metrics

The metrics show the state of the Topic Browser and of its Benthos process, so you can alert when either is degraded. The Topic Browser reports on port 8080 and runs on FSMv1, so it uses the standard [FSMv1 metrics](../instances/agent/metrics.md#fsmv1-metrics). [Read the metrics](../instances/agent/metrics.md#read-the-metrics) explains how to reach the endpoint. Its series carry these labels:

| Part of the Topic Browser | `component` | `instance` |
|---|---|---|
| The Topic Browser | `topic_browser_instance` | `topic-browser` |
| Its Benthos process | `benthos_instance` | `topicbrowser-topic-browser` |

In Prometheus queries, the `instance` label becomes `exported_instance`, as [Read the metrics](../instances/agent/metrics.md#read-the-metrics) explains.

To raise an alert while the Benthos process of the Topic Browser is degraded, use:

```promql
umh_core_service_current_state{component="benthos_instance", exported_instance="topicbrowser-topic-browser"} == 5
```

The definitions are in [`pkg/metrics/metrics.go`](https://github.com/united-manufacturing-hub/united-manufacturing-hub/blob/staging/umh-core/pkg/metrics/metrics.go).

### GraphQL API

For the endpoint, the schema, example queries and every configuration option, see [Topic Browser GraphQL API](../../reference/http-api/topic-browser-graphql.md).
