# Container

The container is the Docker container that UMH Core runs in. Its CPU, memory and disk are shared by every bridge, flow and Redpanda. When one of them crosses its threshold, the Container card shows Degraded, and new bridges wait to start.

## The Container card

The Container card on the instance details page shows the CPU, memory and disk of the container, its CPU architecture and the Hardware ID of the installation.

| Field | Shows |
|---|---|
| Status badge | The health of the container. See [Container status](#container-status). |
| CPU | The CPU usage. See [The CPU row](#the-cpu-row). |
| Memory | The memory the container uses, the memory it may use, and the percentage. The total is the memory limit of the container. Without a limit, it is the memory of the host. With the [memory monitor preview](#memory-with-the-memory-monitor-preview), the row shows 60-second means. With a memory limit, it shows the container's memory against the limit, for example `670.6 MiB / 680.0 MiB (99% of limit, 60s avg)`. Without a limit, it shows the used memory of the whole machine against the machine's memory. |
| Disk | The used and total space of the `/data` volume, and the percentage. |
| Architecture | The CPU architecture UMH Core runs on, `amd64` or `arm64`. |
| Hardware ID | The ID of this installation. UMH Core creates it on the first start and stores it in `/data/hwid`. It stays the same as long as the `/data` volume is kept. See [Container Layout](../../../reference/container-layout.md#hwid). |

The CPU, Memory and Disk rows turn red when that resource is degraded. The **S6 Logs** button is not available yet.

### Container status

The container shows Degraded when the CPU, the memory or the disk is degraded. The badge reason lists one line per degraded resource, such as `Memory degraded: Memory utilization critical`.

| Resource | Degraded at | Reason line |
|---|---|---|
| Memory | Above 80% of the total. With the memory monitor preview, memory also degrades for other causes, see [Memory with the memory monitor preview](#memory-with-the-memory-monitor-preview). | `Memory degraded: Memory utilization critical` |
| Disk | Above 85% of the `/data` volume | `Disk degraded: Disk utilization critical` |
| CPU | See [CPU status](#cpu-status) | `CPU degraded: …` |

Memory from 70% and disk from 75% count as a warning. The badge stays Active and does not show the warning.

### The CPU row

From v0.44.46, the CPU row shows 60-second mean CPU usage. With a CPU limit, it shows the instance's own usage against the limit, for example `0.2 / 0.3 cores (67% of limit, 60s avg)`. Without a limit, it shows the busy cores of the whole machine, including software outside UMH Core, against the machine's cores, for example `2.1 / 14.0 cores (15% of host, 60s avg)`. When the machine's busy cores cannot be measured, it falls back to the instance's own usage. [How CPU Health Is Judged: With and without a CPU limit](how-cpu-health-is-judged.md#with-and-without-a-cpu-limit) explains the difference. The info button next to it shows the readings of the container and the machine, which the [Glossary](how-cpu-health-is-judged.md#glossary) defines. The row turns red while the [CPU status](#cpu-status) is degraded, and reads `N/A` when umh-core cannot measure the CPU.

<figure><img src="../images/container-cpu-info.png" alt="The CPU info button of the Container card on an instance without a CPU limit" width="317"><figcaption></figcaption></figure>

The [CPU status](#cpu-status) and its [Technical Details](#cpu-technical-details) appear in the badge reason of the Container card. While the CPU is degraded, the reason line starts with `CPU degraded:`, for example `CPU degraded: CPU limited`. While the whole container is Active, the reason is the CPU status itself, for example `CPU healthy. …`.

<figure><img src="../images/container-badge-reason.png" alt="The badge reason of an Active Container card with the CPU status and its Technical Details" width="273"><figcaption></figcaption></figure>

### CPU status

| Status | What it means |
|---|---|
| **CPU healthy** | The instance has the CPU it needs. Usage is shown for context, for example "1.2 of 4 cores". |
| **CPU: starting up.** | UMH Core has not measured the CPU yet. This shows in the first seconds after a start. |
| **CPU limited** | The instance hit its CPU limit and was paused until the next scheduling period, for example in 12% of periods over the last minute. Work is being delayed. |
| **CPU contention** | Tasks inside the instance spent time waiting for a free CPU core, for example 23% of the last minute. |
| **CPU taken by the server** | Other virtual machines on the same physical server took CPU this instance needed. |
| **CPU running near full** | There is no room left for the next burst of work, because either the machine is full or the instance is at its CPU limit. The status message says which. |

Every status except **CPU healthy** means CPU health is degraded, and new bridges wait while it lasts.

When no CPU limit is set and the operating system does not report CPU pressure stats, **CPU healthy** adds a line that starts with `Limited visibility:`. UMH Core then cannot fully tell when work waits for a free core. [Enable CPU pressure stats](enable-cpu-pressure-stats.md) turns on full monitoring.

If UMH Core cannot read a file it needs to measure the CPU, the CPU health is degraded and the message names the file, for example `poll error: CPU not measured: open /sys/fs/cgroup/cpu.stat: no such file or directory`.

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

### Memory with the memory monitor preview

With the [memory monitor preview](how-memory-health-is-judged.md) on, memory degrades for one of four causes. The reason line on the Container card names the cause, for example `Memory degraded: Memory running near its limit` for `container-limit-full` or `Memory degraded: Memory ran out` for `oom-kill`. [How Memory Health Is Judged: Memory thresholds](how-memory-health-is-judged.md#memory-thresholds) lists the causes and their values.

## While the container is degraded

A new bridge waits to start while CPU, memory or disk is degraded. Bridges that already run keep running. The reason of the waiting bridge starts with the same line as the reason on the Container card. See [Bridges do not start](../../data-flows/bridges-do-not-start.md).

[Container Shows Degraded](container-shows-degraded.md) lists what to do for each resource and each CPU status.

## Reference

### Configuration

The environment variable `USE_FSMV2_MEMORY_MONITOR` changes how umh-core judges the memory. umh-core reads it once at startup. To change it, remove the container and start it again with the same `docker run` command and the new value. `docker restart` keeps the old environment.

| Variable | Default | Effect |
|---|---|---|
| `USE_FSMV2_MEMORY_MONITOR` | `false` | `true` turns on the memory monitor preview, which [How Memory Health Is Judged](how-memory-health-is-judged.md) explains. |
| `USE_FSMV2_CPU` | none | Ignored from v0.45.4. The CPU status always comes from the CPU health check. On v0.44.46 to v0.45.3, `USE_FSMV2_CPU=false` turns the check off and applies the 70% CPU usage rule instead. Remove the variable from your `docker run` command. |

The CPU health check also runs on an instance started without `AUTH_TOKEN` and `API_URL`.

### Metrics

The Container card reports on port 8080 as `umh_container_*` gauges for CPU, memory and disk, plus `umh_container_health_status`, which holds the health per resource: 0 = Neutral, 1 = Active, 2 = Degraded. The label `component` is `overall`, `cpu`, `memory` or `disk`. [Read the metrics](../agent/metrics.md#read-the-metrics) explains how to reach the endpoint and why Prometheus renames the `instance` label.

To alert when the container degrades:

```promql
umh_container_health_status{component="overall"} == 2
```

To alert on a single resource, for example the disk:

```promql
umh_container_disk_usage_percent{exported_instance="Core"} > 80
```

From v0.45.4, `umh_container_cpu_usage_mcores` and `umh_container_cpu_load_percent` report the CPU health check's 60-second average of the container's own usage. `umh_container_cpu_core_count` is the number of CPU cores available to the container. If reading the CPU, the memory or the disk fails, these gauges keep their last value. An alert on them then keeps its last state.

The container monitor, the part of the agent that collects these values, also reports the standard [FSMv1 metrics](../agent/metrics.md#fsmv1-metrics) with `component="container_monitor"` and `instance="Core"`. The definitions are in [`pkg/service/container_monitor/metrics.go`](https://github.com/united-manufacturing-hub/united-manufacturing-hub/blob/staging/umh-core/pkg/service/container_monitor/metrics.go). umh-core also exposes the `umh_fsmv2_worker_cpu_*` gauges, which [Metrics: CPU gauges](../agent/metrics.md#cpu-gauges) lists. With the memory monitor preview on, it exposes the `umh_fsmv2_worker_memory_*` gauges, which [Metrics: Memory gauges](../agent/metrics.md#memory-gauges) lists.
