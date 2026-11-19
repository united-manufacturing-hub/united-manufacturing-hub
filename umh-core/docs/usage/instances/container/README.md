# Container

The Container card on the instance details page shows the CPU, memory and disk of the UMH Core container. When one of them crosses its threshold, the card shows Degraded. New bridges then wait to start.

## The Container card

The card shows the CPU, memory and disk usage, the CPU architecture and the Hardware ID of the installation. [Instance Details Page: Container](../instance-details-page.md#container) lists every field and the threshold of each resource.

## When the Container shows Degraded

The badge reason lists one line per degraded resource, such as `Memory degraded: Memory utilization critical`. Memory degrades at 80% and disk at 85%. CPU degrades by the rules in [How CPU Health Is Judged](how-cpu-health-is-judged.md).

### What happens while the container is degraded

A new bridge waits to start while CPU, memory or disk is degraded. Bridges that already run keep running. The reason of the waiting bridge starts with the same line as the reason on the Container card. See [Bridges do not start](../../data-flows/bridges-do-not-start.md).

### What to do

- **Memory**: raise the memory limit of the container, or remove bridges or flows. If the [memory monitor preview](how-memory-health-is-judged.md) names `host-memory-full` as the cause, the machine is out of memory. A higher container limit does not help. Free memory on the machine or add memory to it. The [Sizing Guide](../../../production/sizing-guide.md#memory) lists how much memory UMH Core needs.
- **Disk**: free space on the `/data` volume, or shorten how long Redpanda keeps data. See [Disk](../../../production/sizing-guide.md#disk).
- **CPU**: raise or remove the CPU limit of the container, or reduce the load. [What to do for each CPU status](#what-to-do-for-each-cpu-status) lists the steps.

If a bridge must start before the resource problem is fixed, use the emergency setting. See [Emergency settings: Turn off bridge admission](../../data-flows/bridges-do-not-start.md#emergency-settings-turn-off-bridge-admission).

### What to do for each CPU status

[Instance Details Page: CPU status](../instance-details-page.md#cpu-status) explains each status. [How CPU Health Is Judged](how-cpu-health-is-judged.md) explains the signals behind them.

| Status | What to do |
|---|---|
| **CPU healthy** | Nothing. |
| **CPU healthy, limited visibility** | For full monitoring, set a CPU limit, or [enable CPU pressure stats](enable-cpu-pressure-stats.md). |
| **CPU limited** | Raise the CPU limit, or reduce the load on the instance. |
| **CPU contention** | See [CPU Contention While Cores Are Free](../troubleshooting/cpu-contention-while-cores-are-free.md). |
| **CPU taken by the server** | On your virtualization platform, give this virtual machine more guaranteed CPU, or move the other virtual machines off the server. |
| **CPU running near full** | Add CPU capacity, reduce load, or raise the CPU limit. If the machine is full and this container is not the cause, reserve or pin cores for UMH Core and also set a CPU limit (see [Known limitation](how-cpu-health-is-judged.md#known-limitation)), or reduce what else runs on the machine. A CPU limit only caps UMH Core's usage. It reserves no CPU for UMH Core. |

When the status says `CPU not measured` and names a file, check whether the container can read that file, for example `docker exec umh-core cat /sys/fs/cgroup/cpu.stat`.


## Configuration

Two environment variables of the container change how umh-core judges the CPU and the memory. umh-core reads them once at startup. To change one, remove the container and start it again with the same `docker run` command and the new value. `docker restart` keeps the old environment.

| Variable | Default | Effect |
|---|---|---|
| `USE_FSMV2_CPU` | `true` from v0.44.46. On v0.44.37 to v0.44.45 `false`. | `true` judges the CPU with the CPU health check, which [How CPU Health Is Judged](how-cpu-health-is-judged.md) explains. `false` applies the 70% CPU usage rule instead: the CPU is degraded while the whole machine's CPU usage is at 70% or more, or while the kernel throttles the container in more than 5% of scheduling periods over 60 seconds. Both apply whether or not work is being delayed, and even when the container has a CPU limit. |
| `USE_FSMV2_MEMORY_MONITOR` | `false` | `true` turns on the memory monitor preview, which [How Memory Health Is Judged](how-memory-health-is-judged.md) explains. |

Since v0.44.42, the CPU health check also runs on an instance started without `AUTH_TOKEN` and `API_URL`.

## Metrics

The Container card reports on port 8080 as `umh_container_*` gauges for CPU, memory and disk, plus `umh_container_health_status`, which holds the health per resource: 0 = Neutral, 1 = Active, 2 = Degraded. The label `component` is `overall`, `cpu`, `memory` or `disk`. [Read the metrics](../metrics/README.md#read-the-metrics) explains how to reach the endpoint and why Prometheus renames the `instance` label.

To alert when the container degrades:

```promql
umh_container_health_status{component="overall"} == 2
```

To alert on a single resource, for example the disk:

```promql
umh_container_disk_usage_percent{exported_instance="Core"} > 80
```

The meaning of the three `umh_container_cpu_*` gauges changes with `USE_FSMV2_CPU`, so adjust an alert on them when you change the variable. If reading the CPU, the memory or the disk fails, these gauges keep their last value. An alert on them then keeps its last state.

The container monitor, the part of the agent that collects these values, also reports the standard [FSMv1 metrics](../metrics/README.md#fsmv1-metrics) with `component="container_monitor"` and `instance="Core"`. The definitions are in [`pkg/service/container_monitor/metrics.go`](https://github.com/united-manufacturing-hub/united-manufacturing-hub/blob/staging/umh-core/pkg/service/container_monitor/metrics.go). With the CPU health check on, which is the default, umh-core also exposes the `umh_fsmv2_worker_cpu_*` gauges, which [Metrics: CPU gauges](../metrics/README.md#cpu-gauges) lists. With the memory monitor preview on, it exposes the `umh_fsmv2_worker_memory_*` gauges, which [Metrics: Memory gauges](../metrics/README.md#memory-gauges) lists.
