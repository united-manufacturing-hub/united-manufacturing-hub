# Instance Details Page

This page lists what the instance details page in the Management Console shows: every card, its fields, its status badge and the reasons behind each status. The usage page of each component explains what to do about a status.

The page has two tabs. **Core** shows one card per component of UMH Core. **Plugins** shows the plugins added to the instance.

Each card has a status badge: **Active**, **Neutral** or **Degraded**. When the instance sent a reason, an info button next to the badge shows it. The cards show the status that the instance sent last. After 15 seconds without a message from the instance, the Management Console shows a red alert, "Instance Offline", above the tabs, and the cards keep showing the last status.

## Agent

The Agent card shows the identity and the version of the instance. Usage page: [Agent](agent/README.md).

| Field | Shows |
|---|---|
| Status badge | The health of the agent. See [Agent: When the Agent shows Degraded](agent/README.md#when-the-agent-shows-degraded). |
| Name | The name of the instance. The **Rename** button changes it. |
| Location | The location path of the instance, one level per line. It is set when you create the instance and cannot be changed on the card. |
| Core Version | The version of UMH Core. Hover over it to see the versions of S6 Overlay, Benthos and Redpanda. A marker next to the version shows when the release channel has a newer version. |
| Release Channel | The release channel the instance follows, from `agent.releaseChannel` in the config file: `stable`, `nightly` or `enterprise`. It shows `n/a` when `agent.releaseChannel` is missing or has another value. The `RELEASE_CHANNEL` environment variable overwrites the key on every start, so to change the channel, start a new container with the new value. |

The **Logs** button opens the logs of the agent. The agent writes them to `/data/logs/umh-core/`. See [Container Layout](../../reference/container-layout.md#logs). The **Metrics** button is not available yet.

## Container

The Container card shows the CPU, memory and disk of the UMH Core container. Usage page: [Container](container/README.md).

| Field | Shows |
|---|---|
| Status badge | The health of the container. See [Container status](#container-status). |
| CPU | The CPU usage. See [The CPU row](#the-cpu-row). |
| Memory | The memory the container uses, the memory it may use, and the percentage. The total is the memory limit of the container. Without a limit, it is the memory of the host. With the [memory monitor preview](#memory-with-the-memory-monitor-preview), the row shows 60-second means. With a memory limit, it shows the container's memory against the limit, for example `670.6 MiB / 680.0 MiB (99% of limit, 60s avg)`. Without a limit, it shows the used memory of the whole machine against the machine's memory. |
| Disk | The used and total space of the `/data` volume, and the percentage. |
| Architecture | The CPU architecture UMH Core runs on, `amd64` or `arm64`. |
| Hardware ID | The ID of this installation. UMH Core creates it on the first start and stores it in `/data/hwid`. It stays the same as long as the `/data` volume is kept. See [Container Layout](../../reference/container-layout.md#hwid). |

The CPU, Memory and Disk rows turn red when that resource is degraded. The **S6 Logs** button is not available yet.

### Container status

The container shows Degraded when the CPU, the memory or the disk is degraded. The badge reason lists one line per degraded resource, such as `Memory degraded: Memory utilization critical`.

| Resource | Degraded at | Reason line |
|---|---|---|
| Memory | 80% of the total. With the memory monitor preview, memory also degrades for other causes, see [Memory with the memory monitor preview](#memory-with-the-memory-monitor-preview). | `Memory degraded: Memory utilization critical` |
| Disk | 85% of the `/data` volume | `Disk degraded: Disk utilization critical` |
| CPU | See [CPU status](#cpu-status) | `CPU degraded: …` |

Memory from 70% and disk from 75% count as a warning. The badge stays Active and does not show the warning.

### The CPU row

From v0.44.46, the CPU row shows 60-second mean CPU usage. With a CPU limit, it shows the instance's own usage against the limit, for example `0.2 / 0.3 cores (67% of limit, 60s avg)`. Without a limit, it shows the busy cores of the whole machine, including software outside UMH Core, against the machine's cores, for example `2.1 / 14.0 cores (15% of host, 60s avg)`. When the machine's busy cores cannot be measured, it falls back to the instance's own usage. The info button next to it shows the usage, its 95th percentile, throttling and pressure of the container, and the busy cores and steal of the machine. The row turns red while the CPU is degraded. When umh-core cannot measure the CPU, the row reads `N/A`.

The [CPU status](#cpu-status) and its [Technical Details](#cpu-technical-details) appear in the badge reason of the Container card. While the CPU is degraded, the reason line starts with `CPU degraded:`, for example `CPU degraded: CPU limited`. While the whole container is Active, the reason is the CPU status itself, for example `CPU healthy. …`.

With `USE_FSMV2_CPU=false`, the row shows the CPU load of the whole host instead, and the CPU is degraded from 70% host load or from throttling. See [Container: Configuration](container/README.md#configuration).

### CPU status

| Status | What it means |
|---|---|
| **CPU healthy** | The instance has the CPU it needs. Usage is shown for context, for example "1.2 of 4 cores". |
| **CPU healthy, limited visibility** | The instance looks fine, but UMH Core cannot fully measure CPU health here: no CPU limit is set and the operating system is not reporting CPU-pressure statistics. |
| **CPU limited** | The instance hit its CPU limit and was paused until the next scheduling period, for example in 12% of periods over the last minute. Work is being delayed. |
| **CPU contention** | Tasks inside the instance spent time waiting for a free CPU core, for example 23% of the last minute. |
| **CPU taken by the server** | Other virtual machines on the same physical server took CPU this instance needed. |
| **CPU running near full** | There is no room left for the next burst of work, because either the machine is full or the instance is at its CPU limit. The status message says which. |

Every status except **CPU healthy** and **CPU healthy, limited visibility** means CPU health is degraded, and new bridges wait while it lasts.

From v0.44.43, if UMH Core cannot read a file it needs to measure the CPU, the CPU health is degraded and the message names the file, for example `CPU not measured: open /sys/fs/cgroup/cpu.stat: no such file or directory`.

### CPU Technical Details

Below the status, in the same badge reason, a Technical Details section lists every signal UMH Core can measure on that machine. Both the status and Technical Details show the thresholds. A healthy instance states its remaining headroom. Each Technical Details line gives the reading next to the threshold that would change the status:

```text
CPU healthy. This instance is using 0.3 of 2 cores (15% of its limit) and can use 1.5 more before it is marked degraded.

Technical Details:
Instance headroom 1.5 cores = 2 total - 0.3 used - 0.2 reserved (degrades below 0).
Instance usage not measured (throttling already shows whether this instance is hitting its CPU limit, so this rough estimate is not needed).
Throttling 2% (degrades above 5%).
Pressure 4% (degrades above 20%).
Steal not measured (this instance is not running in a virtual machine, so no other virtual machine can take its CPU).
```

The headroom lines depend on the machine. A machine whose core count is readable also gets a Machine headroom line, which this example leaves out. An instance with no CPU limit gets no Instance headroom line. Every other signal always has a line, even when this machine cannot measure it.

A signal that is already past its threshold shows what would clear it instead, for example `Throttling 12% (recovers below 3%)`. A signal this machine cannot measure reads `not measured` with its reason, as the Steal line above does.

### CPU thresholds

Each signal degrades at one value and recovers at another. The two are held apart so the status does not flicker. Throttling, pressure, steal and usage are worse the higher they read, headroom the lower. All readings are 60-second figures. A reading must exceed a threshold to count. Where the table says "at", reaching it is enough.

| Signal | Degrades | Recovers | Measured only when |
|--------|----------|----------|--------------------|
| **Throttling** | above 5% of scheduling periods | below 3% | a CPU limit is set |
| **Pressure** | above 20% (Linux Pressure Stall Information, PSI: `some avg60`) | below 12% | the container can read `cpu.pressure` (see [Enable CPU pressure stats](container/enable-cpu-pressure-stats.md)) |
| **Steal** | above 10% | below 6% | the machine is a virtual machine |
| **Machine headroom** | less than 1 core free | more than 1.5 cores free | the machine's core count is readable |
| **Instance headroom** | usage past 90% of the limit | below 85% of the limit | a CPU limit is set |
| **Instance usage** | at 70% | below 60% | no CPU limit is set, the kernel publishes no PSI, and the machine's free cores cannot be measured (last-resort fallback for machine headroom) |

Steal uses the 95th percentile once 20 samples are in, and the mean before that, so a newly started instance gets a steal reading within seconds. Bare metal reports no steal, so on a physical machine the Steal line reads `not measured` with its reason.

### Memory with the memory monitor preview

With the [memory monitor preview](container/how-memory-health-is-judged.md) on, memory counts as degraded while any of these causes holds:

| Cause | Degraded from | Healthy again below | Gauge |
|---|---|---|---|
| `container-limit-full` | 0.8 (80%) of the container's own memory limit, as a 60s mean. Judged only when the container has a limit. Without one, `memory_avg_used_fraction` still shows the container's share of the machine's memory, but only `host-memory-full` judges | 0.75 | `memory_avg_used_fraction` while `memory_total_is_cgroup_limit` is 1 |
| `host-memory-full` | 0.8 (80%) of the machine's memory, as a 60s mean | 0.75 | `memory_avg_host_used_bytes` / `memory_host_total_bytes` |
| `memory-pressure` | 0.20: processes in the container waited for memory for 20% of the last minute | 0.12 | `memory_pressure_avg60_ratio` |
| `oom-kill` | 1 OOM kill in the container within the last hour | 0 for a whole hour | `memory_oom_kills_last_hour` |

The reason line on the Container card names the cause, for example `Memory degraded: Memory running near its limit` for `container-limit-full` or `Memory degraded: Memory ran out` for `oom-kill`.

Memory also counts as degraded whenever no reading from the last three seconds is available. [Metrics](metrics/README.md#memory-gauges) lists the gauges.

## Communicator

The Communicator card shows the connection to the Management Console. It does not show why a connection failed. The [Communicator metrics](communicator/README.md#metrics) show the cause. Usage page: [Communicator](communicator/README.md).

| Field | Shows |
|---|---|
| Status badge | The health of the connection. See [Communicator status](#communicator-status). |
| Usage | How full the outbound queue was over the last 30 seconds, as the 95th percentile. The outbound queue holds the messages that wait to be sent to the Management Console. |
| Peak Usage | The highest fill level of the outbound queue in the last 30 seconds. |
| Active Viewers | The number of Management Console sessions that receive updates from this instance. A session stays counted for a few minutes after you close it. |

The Usage and Peak Usage rows appear while the badge is Active or Degraded. Each viewer adds a copy of every status message to the queue, so many open tabs can fill it.

### Communicator status

| Badge | Reason | What it means |
|---|---|---|
| Active | `Outbound queue …% full …, peak …% …` | The connection works and the queue has room. |
| Degraded | `Messages are held because the last login could not be verified.` | umh-core holds every message to and from the Management Console, because it could not verify the last login. While this lasts, the badge shows this reason instead of any other. |
| Degraded | `Outbound queue … Status updates and action replies may be lost` | The queue is close to full. This happens when Usage rises above 50% or Peak Usage above 90%. Each figure that rose above its mark must fall below its own lower mark before the badge clears: 30% for Usage, 70% for Peak Usage. |
| Degraded | `Authentication with the Management Console failed` | The login failed in a way that another attempt cannot fix. See [Communicator](communicator/README.md#when-the-communicator-shows-degraded). |
| Degraded | `Connection to the Management Console is degraded: <n> consecutive <type> errors` | Requests failed since the last successful login. `<n>` counts the failed logins, sends and fetches since that login. `<type>` names the last error, and each type has an [error counter](metrics/README.md#communicator-error-counters). `invalid_token` counts in `umh_fsmv2_worker_auth_failures_total`, and `cloudflare_challenge` in `umh_fsmv2_worker_cloudflare_errors_total`. `unknown`, `network` and `channel_full` count in `umh_fsmv2_worker_network_errors_total`. Every other type matches the start of its counter name, for example `server_error` and `umh_fsmv2_worker_server_errors_total`. |
| Degraded | `Communicator status is stale` | umh-core has not updated the communicator status for more than 10 seconds. |
| Neutral | `Connecting to the Management Console` | The communicator is starting. |
| Neutral | `Measuring outbound queue usage` | The connection works, and umh-core is still collecting samples of the queue. |
| Neutral | `Communicator not running` | The transport worker is stopped or stopping. |
| Neutral | `Communicator status unknown` | umh-core could not read the communicator status. |

## Data Flows

The Data Flows card counts the data flows of the instance. Usage page: [Data Flows](../data-flows/README.md).

| Field | Shows |
|---|---|
| Status badge | The health of the data flows as a whole. |
| Data Flows | The number of bridges, stand-alone flows and stream processors. It links to the Data Flows list, filtered to this instance. |
| Bridges | The number of bridges. It links to the list of this instance's bridges. |
| Status Breakdown | How many data flows are **Active**, **Degraded**, **Other** (Neutral) and **Unknown** (no health reported). It appears once the instance has a data flow. |

## Redpanda

The Redpanda card shows the message broker that stores the instance's [Unified Namespace](../unified-namespace/README.md). Usage page: [Redpanda](../unified-namespace/redpanda.md).

| Field | Shows |
|---|---|
| Status badge | The health of Redpanda. |
| Incoming Throughput | The data written to Redpanda per second, as a one-minute average, in B, KiB, MiB or a larger binary unit. |
| Outgoing Throughput | The data read from Redpanda per second, as a one-minute average, in the same units. |

The **Logs** and **Metrics** buttons open the logs and the metrics of Redpanda. They are unavailable when the instance's UMH Core version does not support them.

## Topic Browser

The Topic Browser card shows the topics in the instance's Unified Namespace. Usage page: [Topic Browser](../unified-namespace/topic-browser.md).

| Field | Shows |
|---|---|
| Status badge | The health of the Topic Browser. |
| Topics | The number of topics. It links to the Topic Browser. |

The **Logs** and **Metrics** buttons open the logs and the metrics of the Topic Browser. They are unavailable when the instance's UMH Core version does not support them.

## Plugins

The **Plugins** tab shows one card per plugin added to the instance. The tab is unavailable when plugins are not enabled for your organization, or when the instance's UMH Core version does not support them. In the second case, hovering over the tab shows `This instance does not support plugins yet — update umh-core.`

| Plugin | Page |
|---|---|
| Historian | [Historian](../historian/README.md) |
