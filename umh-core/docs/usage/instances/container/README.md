# Container

The Container card on the instance details page shows the CPU, memory and disk of the UMH Core container. When one of them crosses its threshold, the card shows Degraded. New bridges then wait to start.

## The Container card

The CPU, Memory and Disk rows turn red when that resource is degraded.

| Field | Shows |
|---|---|
| Status badge | The health of the container. Hover over it to see the reason. See [When the Container shows Degraded](#when-the-container-shows-degraded). |
| CPU | The CPU usage. See [The CPU row](#the-cpu-row). |
| Memory | The memory the container uses, the memory it may use, and the percentage. The total is the memory limit of the container. Without a limit, it is the memory of the host. |
| Disk | The used and total space of the `/data` volume, and the percentage. |
| Architecture | The CPU architecture UMH Core runs on, `amd64` or `arm64`. |
| Hardware ID | The ID of this installation. UMH Core creates it on the first start and stores it in `/data/hwid`. It stays the same as long as the `/data` volume is kept. See [Container Layout](../../../reference/container-layout.md#hwid). |

## When the Container shows Degraded

The container shows Degraded when the CPU, the memory or the disk is degraded. The badge reason lists one line per degraded resource, such as `Memory degraded: Memory utilization critical`.

| Resource | Degraded at | Reason line |
|---|---|---|
| Memory | 80% of the total. With the [memory monitor preview](#memory-monitor-preview), memory also degrades for other causes. | `Memory degraded: Memory utilization critical` |
| Disk | 85% of the `/data` volume | `Disk degraded: Disk utilization critical` |
| CPU | See [The CPU row](#the-cpu-row) | `CPU degraded: …` |

Memory from 70% and disk from 75% count as a warning. The badge stays Active and does not show the warning.

### What happens while the container is degraded

A new bridge waits to start while CPU, memory or disk is degraded. Bridges that already run keep running. The reason of the waiting bridge starts with the same line as the reason on the Container card. See [Bridges do not start](../../data-flows/bridges-do-not-start.md).

### What to do

- **Memory**: raise the memory limit of the container, or remove bridges or flows. If the memory preview names `host-memory-full` as the cause (see [Memory monitor](#memory-monitor-preview)), the machine is out of memory. A higher container limit does not help. Free memory on the machine or add memory to it. The [Sizing Guide](../../../production/sizing-guide.md#memory) lists how much memory UMH Core needs.
- **Disk**: free space on the `/data` volume, or shorten how long Redpanda keeps data. See [Disk usage in practice](../../../production/sizing-guide.md#disk-usage-in-practice).
- **CPU**: raise or remove the CPU limit of the container, or reduce the load. [CPU Health](cpu-health/README.md) lists what to do for each CPU status.

If a bridge must start before the resource problem is fixed, use the emergency setting. See [Emergency settings: Turn off bridge admission](../../data-flows/bridges-do-not-start.md#emergency-settings-turn-off-bridge-admission).

## The CPU row

The CPU row depends on the `USE_FSMV2_CPU` environment variable. It chooses which of umh-core's two frameworks measures the CPU: FSMv2, the newer one, or FSMv1, the older one. See [Which framework runs a component](../agent/metrics.md#which-framework-runs-a-component). umh-core reads the variable once at startup. To change it, remove the container and start it again with the same `docker run` command plus `-e USE_FSMV2_CPU=false`, or without the variable to go back to the default. `docker restart` keeps the old environment.

From v0.44.46, the row shows the CPU Health of the instance by default, which [CPU Health](cpu-health/README.md) explains. When umh-core cannot measure the CPU, the row reads `N/A` in red. The container is then degraded, and the reason line says why the measurement failed.

With `USE_FSMV2_CPU=false`, the row shows the CPU load of the whole host, and the CPU is degraded from 70% host load or from throttling. [CPU Health](cpu-health/README.md) describes this fallback rule.

## Memory monitor (preview)

The memory monitor is a preview. It is off by default. It turns on when the container is started with `-e USE_FSMV2_MEMORY_MONITOR=true`, and the gauges below then appear. umh-core reads the variable once at startup, and `docker restart` keeps the old environment, so changing it needs a new container (`docker rm`, then `docker run` with the volume). They use the same `umh_fsmv2_worker_<gauge name>` series and `hierarchy_path` label as the [CPU Health gauges](cpu-health/README.md#metrics), and they also keep their previous value on a tick that could not measure. On a tick that could not measure, the age of `memory_last_sample_unix` is the only thing that reveals the freeze.

With the preview on, the instance's memory status comes from this reading. Memory counts as degraded while any of these causes holds:

| Cause | Degraded from | Healthy again below | Gauge |
|---|---|---|---|
| `container-limit-full` | 0.8 (80%) of the container's own memory limit, as a 60s mean. Judged only when the container has a limit. Without one, `memory_avg_used_fraction` still shows the container's share of the machine's memory, but only `host-memory-full` judges | 0.75 | `memory_avg_used_fraction` while `memory_total_is_cgroup_limit` is 1 |
| `host-memory-full` | 0.8 (80%) of the machine's memory, as a 60s mean | 0.75 | `memory_avg_host_used_bytes` / `memory_host_total_bytes` |
| `memory-pressure` | 0.20: processes in the container waited for memory for 20% of the last minute | 0.12 | `memory_pressure_avg60_ratio` |
| `oom-kill` | 1 OOM kill in the container within the last hour | 0 for a whole hour | `memory_oom_kills_last_hour` |

Memory also counts as degraded, and bridges are held back, whenever no reading from the last three seconds is available.

`memory-pressure` needs cgroup v2 with pressure stall information (PSI) enabled, the same as CPU pressure (see [CPU Health](cpu-health/README.md#enabling-cpu-pressure-stats)). `oom-kill` counts only the kills after umh-core started, so it does not report a kill that restarted umh-core itself. The status message also shows the kernel's OOM kill count since the container started, which includes those kills.

### Memory measurements

| Gauge | Reports |
|---|---|
| `memory_avg_used_bytes` | the 60s mean of the container's memory in use, in bytes |
| `memory_total_bytes` | the limit that usage is judged against: the container's memory limit, or the machine's total memory when the container has none |
| `memory_avg_used_fraction` | the 60s mean of the memory in use as a 0..1 fraction of `memory_total_bytes` |
| `memory_avg_host_used_bytes` | the whole machine's 60s mean used memory, in bytes. It works like `cpu_avg_host_busy_cores` in [CPU Health](cpu-health/README.md#measurements) |
| `memory_host_total_bytes` | the machine's total memory, in bytes |
| `memory_pressure_avg60_ratio` | the kernel's PSI memory-some avg60 as a 0..1 fraction |
| `memory_oom_kills_last_hour` | the OOM kills in the container within the last hour |
| `memory_last_sample_unix` | the unix seconds of the last tick that measured |

### Memory flags

| Flag | Applies to |
|---|---|
| `memory_used_ring_active` | `memory_avg_used_bytes` and `memory_avg_used_fraction` |
| `memory_host_ring_active` | `memory_avg_host_used_bytes` |
| `memory_pressure_signal_ready` | `memory_pressure_avg60_ratio` |
| `memory_oom_signal_ready` | `memory_oom_kills_last_hour` |
| `memory_used_from_cgroup` | `memory_avg_used_bytes` |
| `memory_total_is_cgroup_limit` | `memory_total_bytes` and `memory_avg_used_fraction` |

The `_ring_active` and `_signal_ready` flags work like the [CPU flags](cpu-health/README.md#flags). `memory_host_ring_active` also reads 0 while the machine's memory is unreadable. `memory_pressure_signal_ready` stays 0 on cgroup v1 and without PSI. `memory_oom_signal_ready` reads 0 before the second read of the OOM counter and while that counter is unreadable.

The two source flags say where a number came from, not whether to trust it. `memory_used_from_cgroup` reads 0 when the container's cgroup was unreadable. `memory_avg_used_bytes` and `memory_avg_used_fraction` then have no value, `memory_used_ring_active` reads 0, and the machine's gauges carry the reading. `memory_total_is_cgroup_limit` reads 0 when the container has no memory limit and usage is judged against the machine's total memory.

## Metrics

The Container card reports on port 8080 as `umh_container_*` gauges for CPU, memory and disk, plus `umh_container_health_status`, which holds the health per resource: 0 = Neutral, 1 = Active, 2 = Degraded. The label `component` is `overall`, `cpu`, `memory` or `disk`. [Read the metrics](../agent/metrics.md#read-the-metrics) explains how to reach the endpoint and why Prometheus renames the `instance` label.

To alert when the container degrades:

```promql
umh_container_health_status{component="overall"} == 2
```

To alert on a single resource, for example the disk:

```promql
umh_container_disk_usage_percent{exported_instance="Core"} > 80
```

The meaning of the three `umh_container_cpu_*` gauges changes with `USE_FSMV2_CPU`, so adjust an alert on them when you change the variable. If reading the CPU, the memory or the disk fails, these gauges keep their last value. An alert on them then keeps its last state.

The container monitor, the part of the agent that collects these values, also reports the standard [FSMv1 metrics](../agent/metrics.md#fsmv1-metrics) with `component="container_monitor"` and `instance="Core"`. The definitions are in [`pkg/service/container_monitor/metrics.go`](https://github.com/united-manufacturing-hub/united-manufacturing-hub/blob/staging/umh-core/pkg/service/container_monitor/metrics.go). With the CPU health check on, which is the default, umh-core also exposes the `umh_fsmv2_worker_cpu_*` gauges, which [CPU Health](cpu-health/README.md#metrics) lists.
