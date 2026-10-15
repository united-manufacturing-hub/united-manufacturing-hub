# Redpanda

Redpanda is the message broker inside the UMH Core container, and it uses the Kafka protocol. It stores the messages of the [Unified Namespace](README.md) in the Kafka topic `umh.messages`. The [Topic Browser](topic-browser.md) reads the Unified Namespace topics from the messages in `umh.messages`. Redpanda also runs the schema registry on port 8081, which holds the schemas of your data models and data contracts.

umh-core starts one Redpanda inside the container and restarts it when it fails. Redpanda keeps its data in `/data/redpanda`.

## Redpanda states

Redpanda is always in one of the states below. The **Redpanda** card on the instance details page in the Management Console shows the state as a badge. The badge tooltip shows the reason for the state.

| State | Meaning | Badge |
|---|---|---|
| `starting` | Redpanda is starting. umh-core waits for the log line `Successfully started Redpanda!` and for the schema registry to answer. | Neutral |
| `idle` | Redpanda runs. No messages were written to it or read from it recently. | Active |
| `active` | Redpanda runs, and messages are written to it or read from it. | Active |
| `degraded` | One of the checks in [When Redpanda is degraded](#when-redpanda-is-degraded) failed. | Degraded |
| `stopping` | Redpanda is shutting down. | Neutral |
| `stopped` | Redpanda does not run. | Stopped when Redpanda is set to stop, otherwise Neutral |

An idle Redpanda shows the Active badge. Its tooltip starts with `idle: no input bytes activity`. This is normal for an instance that has no data flows yet.

## When Redpanda is degraded

The badge tooltip starts with `degraded: ` and continues with the reason. umh-core runs the checks in the order of this table and shows the reason of the first check that fails. The two `found error` rows are one check, which shows the earliest matching log line. Redpanda returns to `idle` once no check fails.

| Reason | What it means | What to do |
|---|---|---|
| `s6 is not running, current state: <state>` | The Redpanda process is not running. umh-core restarts it, and the reason changes to `degraded: restarting service`. | Select **Logs** on the Redpanda card to see why the process stopped. |
| `healthchecks did not pass, live: false, ready: false` | Redpanda does not report itself as ready. Both values come from the same readiness check. They are therefore always equal. | Read the Redpanda logs. |
| `found error (clears in <time>): <log line>` with `Address already in use` | Another process holds a port that Redpanda needs, such as 9092 or 9644. | Stop the other process. If the container shares the host network, check which host process uses the port. |
| `found error (clears in <time>): <log line>` with `Reactor stalled for <n> ms` | Redpanda was blocked for more than 500 milliseconds and processed nothing in that time. | Check [CPU Health](../instances/container/cpu-health/README.md). Give the container more CPU or a faster disk. |
| `Redpanda storage free space alert: <free> free bytes (total: <total> bytes)` | Redpanda reports that its disk is nearly full. | Free disk space, enlarge the disk, or shorten the retention. See [Disk usage in practice](../../production/sizing-guide.md#disk-usage-in-practice). |
| `Redpanda unavailable topics: <n>` | Redpanda cannot serve `<n>` partitions. A partition is one part of a Kafka topic. The number counts partitions, although the text says topics. | Read the Redpanda logs to find the cause. |
| `unexpected restarts within the last hour: [<exit codes>]` | The Redpanda process exited with an error at least once in the last hour. The reason stays for one hour after the last exit. | Read the Redpanda logs around the time of the exit. |

A `found error` reason stays for 10 minutes after the log line. The `clears in` value counts down that time. Only the two log lines in the table cause this reason. A `Reactor stalled` line counts only after Redpanda has started.

## Redpanda card

The Redpanda card on the instance details page shows the state badge, the throughput, and two buttons.

| Field | Shows |
|---|---|
| **Incoming Throughput** | Bytes per second written to Redpanda, averaged over a time window. The unit scales from B/s to KiB/s and MiB/s. |
| **Outgoing Throughput** | Bytes per second read from Redpanda, averaged over the same window. |
| **Logs** | Opens the Redpanda log. |
| **Metrics** | Opens the Redpanda Metrics page. |

The Redpanda Metrics page shows a table with the columns Path, Name and Value. Select **Refresh** to load the current values.

| Path | Name | Value |
|---|---|---|
| `redpanda.storage` | `disk_free_bytes` | Free bytes on the disk that holds Redpanda's data. |
| `redpanda.storage` | `disk_total_bytes` | Size of that disk in bytes. |
| `redpanda.storage` | `disk_free_space_alert` | `true` while Redpanda's low-disk alert is raised, otherwise `false`. |
| `redpanda.cluster` | `topics` | Number of Kafka topics. |
| `redpanda.cluster` | `unavailable_partitions` | Number of partitions Redpanda cannot serve. |
| `redpanda.kafka` | `request_bytes_in` | Total bytes written to Redpanda since Redpanda started. |
| `redpanda.kafka` | `request_bytes_out` | Total bytes read from Redpanda since Redpanda started. |
| `redpanda.topic.<topic>` | `partitions` | Number of partitions of that topic. |

`request_bytes_in` and `request_bytes_out` are totals. umh-core computes the throughput on the card from the same totals.

## Settings

Redpanda's settings are under `internal.redpanda` in the [config file](../instances/config-file.md). The [configuration reference](../../reference/configuration-reference.md) lists them. To change how long Redpanda keeps messages, follow [Disk usage in practice](../../production/sizing-guide.md#disk-usage-in-practice) in the sizing guide.

When umh-core starts without a config file, it creates one with the two settings in this table. Both are under `internal.redpanda.redpandaServiceConfig.topic`.

| Setting | Value | Effect |
|---|---|---|
| `defaultTopicRetentionMs` | `604800000` | Redpanda keeps messages for seven days. |
| `defaultTopicCleanupPolicy` | `delete` | Redpanda deletes messages older than the retention time. |

If your config file was created before umh-core started to write `defaultTopicCleanupPolicy`, it has no such setting, and Redpanda uses `compact`. Under `compact`, Redpanda keeps only the latest message per key once a segment of the topic closes. A segment closes after one hour by default. You cannot replay the older history of a topic then.

## Metrics

Port 8080 has no series that Redpanda reports itself. umh-core reports its own view of Redpanda in the FSMv1 series. FSMv1 is the older of umh-core's two frameworks, the parts of umh-core that start and watch components (see [Which framework runs a component](../instances/agent/metrics.md#which-framework-runs-a-component)). The [Metrics](../instances/agent/metrics.md) page explains how to read the series and lists their names under [FSMv1 metrics](../instances/agent/metrics.md#fsmv1-metrics).

| `component` value | `instance` value ([how Prometheus labels it](../instances/agent/metrics.md#read-the-metrics)) | Series |
|---|---|---|
| `redpanda_instance` | `redpanda` | `umh_core_service_current_state`, `umh_core_service_desired_state`, `umh_core_errors_total`, `umh_core_reconcile_duration_milliseconds` |
| `redpanda_monitor` | `redpanda-redpanda` | `umh_core_errors_total`, `umh_core_reconcile_duration_milliseconds` |

`umh_core_reconcile_duration_milliseconds` has further `instance` values of the form `redpanda.<step>` and `redpanda-redpanda.<step>`. Filter on the exact values in the table to get one series per component. `umh_core_errors_total` with `component="redpanda_monitor"` appears only after the first error.

The Redpanda monitor is a separate process in the container. The [agent monitor](../instances/agent/README.md), by contrast, is part of the agent. The Redpanda monitor reads Redpanda's status and metrics once per second, and umh-core builds the card from these values. The Redpanda monitor reports no state series.

`umh_core_service_current_state` with `component="redpanda_instance"` reads 0 for `stopped`, 1 for `starting`, 3 for `active`, 4 for `idle` and 5 for `degraded`. It reads -1 while Redpanda is `stopping`.

Redpanda serves its own metrics on its admin port 9644 at `/public_metrics`. The Redpanda monitor reads them there. The port is inside the container and is not published by default.
