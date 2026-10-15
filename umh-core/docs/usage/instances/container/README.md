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
| Memory | 80% of the total | `Memory degraded: Memory utilization critical` |
| Disk | 85% of the `/data` volume | `Disk degraded: Disk utilization critical` |
| CPU | See [The CPU row](#the-cpu-row) | `CPU degraded: …` |

Memory from 70% and disk from 75% count as a warning. The badge stays Active and does not show the warning.

### What happens while the container is degraded

A new bridge waits to start while CPU, memory or disk is degraded. Bridges that already run keep running. The reason of the waiting bridge starts with the same line as the reason on the Container card. See [Bridges do not start](../../data-flows/bridges-do-not-start.md).

### What to do

- **Memory**: raise the memory limit of the container, or remove bridges or flows. The [Sizing Guide](../../../production/sizing-guide.md#memory) lists how much memory UMH Core needs.
- **Disk**: free space on the `/data` volume, or shorten how long Redpanda keeps data. See [Disk usage in practice](../../../production/sizing-guide.md#disk-usage-in-practice).
- **CPU**: raise or remove the CPU limit of the container, or reduce the load. [CPU Health](cpu-health/README.md) lists what to do for each CPU status.

If a bridge must start before the resource problem is fixed, use the emergency setting. See [Emergency settings: Turn off bridge admission](../../data-flows/bridges-do-not-start.md#emergency-settings-turn-off-bridge-admission).

## The CPU row

The CPU row depends on the `USE_FSMV2_CPU` environment variable. It chooses which of umh-core's two frameworks measures the CPU: FSMv1, the older one, or FSMv2, the newer one. See [Which framework runs a component](../agent/metrics.md#which-framework-runs-a-component). umh-core reads the variable once at startup. To change it, remove the container and start it again with the same `docker run` command plus `-e USE_FSMV2_CPU=true`. `docker restart` keeps the old environment.

Without `USE_FSMV2_CPU`, which is the default, the row shows the CPU load of the whole host. The load includes other processes on the same machine. Without a CPU limit, the row shows the load as a percentage of the host cores, followed by `(no limit set)`. With a CPU limit, the row shows the host load scaled to the limit, in millicores (mCPU). It does not show the usage of the container alone. The CPU is degraded when the host load reaches 70%. It is also degraded when the container was throttled in more than 5% of the scheduling periods of the last 60 seconds. Throttling means the container hit its CPU limit and had to wait. The reason line is then `CPU degraded: CPU utilization critical` or `CPU degraded: CPU throttled (…% periods throttled)`.

With `USE_FSMV2_CPU=true`, the row shows the CPU Health of the instance, which is explained on [CPU Health](cpu-health/README.md). When umh-core cannot measure the CPU, the row reads `N/A` in red. The container is then degraded, and the reason line says why the measurement failed.

## Metrics

umh-core exposes the metrics of the container on its Prometheus endpoint, port 8080 at `/metrics`. [Read the metrics](../agent/metrics.md#read-the-metrics) explains how to reach the endpoint.

### Resource gauges

These gauges carry the values of the Container card. Each has the label `instance="Core"`. In Prometheus, this label usually arrives as `exported_instance`. [Read the metrics](../agent/metrics.md#read-the-metrics) explains why.

| Name | Type | Reports |
|---|---|---|
| `umh_container_cpu_usage_mcores` | gauge | CPU usage, in millicores (1000 = one core). See [CPU gauges](#cpu-gauges). |
| `umh_container_cpu_core_count` | gauge | Number of CPU cores. See [CPU gauges](#cpu-gauges). |
| `umh_container_cpu_load_percent` | gauge | CPU usage as a percentage of the cores. See [CPU gauges](#cpu-gauges). |
| `umh_container_memory_used_bytes` | gauge | Memory the container uses, in bytes |
| `umh_container_memory_total_bytes` | gauge | Memory the container may use, in bytes |
| `umh_container_memory_usage_percent` | gauge | Memory used as a percentage of the total |
| `umh_container_disk_used_bytes` | gauge | Used space of the `/data` volume, in bytes |
| `umh_container_disk_total_bytes` | gauge | Total space of the `/data` volume, in bytes |
| `umh_container_disk_usage_percent` | gauge | Used space of the `/data` volume as a percentage of the total |
| `umh_container_health_status` | gauge | Health per resource: 0 = Neutral, 1 = Active, 2 = Degraded. The `component` label is `overall`, `cpu`, `memory` or `disk`. These are the values behind the badge and the row colors of the card. |

To alert when the container degrades, alert on `umh_container_health_status{component="overall"} == 2`.

When umh-core has no CPU reading for a cycle, the three CPU gauges keep their last value, and `umh_container_health_status` still updates. When reading the CPU, the memory or the disk fails with an error, umh-core updates none of these gauges. Each gauge, `umh_container_health_status` included, then keeps its last value. An alert on these gauges keeps its last state.

### CPU gauges

The three `umh_container_cpu_*` gauges change their meaning with `USE_FSMV2_CPU`. An alert on them has to be adjusted when you change the variable.

#### FSMv1 (default)

| Name | Type | Reports |
|---|---|---|
| `umh_container_cpu_usage_mcores` | gauge | Host CPU load scaled to the CPU limit, or to the host cores without a limit. One sample, in millicores. |
| `umh_container_cpu_core_count` | gauge | Number of CPUs the umh-core process may run on. This is the number of host cores, unless the container is pinned to some of them with `--cpuset-cpus`. |
| `umh_container_cpu_load_percent` | gauge | `umh_container_cpu_usage_mcores` as a percentage of the host cores |

#### FSMv2 (with `USE_FSMV2_CPU=true`)

| Name | Type | Reports |
|---|---|---|
| `umh_container_cpu_usage_mcores` | gauge | CPU usage of the container alone, averaged over 60 seconds, in millicores |
| `umh_container_cpu_core_count` | gauge | Number of CPUs the container may run on. A CPU limit does not change this number. |
| `umh_container_cpu_load_percent` | gauge | `umh_container_cpu_usage_mcores` as a percentage of those CPUs. With a CPU limit, it is not a percentage of the limit. |

With `USE_FSMV2_CPU=true`, umh-core also exposes the `umh_fsmv2_worker_cpu_*` gauges. [CPU Health](cpu-health/README.md) lists them.

### Container monitor

The container monitor is the part of the agent that collects these values. It uses the standard [FSMv1 metrics](../agent/metrics.md#fsmv1-metrics) with `component="container_monitor"` and `instance="Core"`. In Prometheus, `instance` usually arrives as `exported_instance`, see [Read the metrics](../agent/metrics.md#read-the-metrics).

| Name | Type | Reports |
|---|---|---|
| `umh_core_service_current_state{component="container_monitor",instance="Core"}` | gauge | State of the container monitor: 3 = active, 5 = degraded. -1 means the container monitor is in a sub-state, such as its starting or stopping steps. See [State](../agent/metrics.md#state). |
| `umh_core_service_desired_state{component="container_monitor",instance="Core"}` | gauge | State the container monitor should reach, with the same values |
| `umh_core_errors_total{component="container_monitor",instance="Core"}` | counter | Errors while the container monitor collected the values |
| `umh_core_reconcile_duration_milliseconds{component="container_monitor",instance="Core"}` | histogram | Duration of one update of the container monitor, in milliseconds |
