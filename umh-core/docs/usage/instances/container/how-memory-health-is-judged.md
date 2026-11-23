# How Memory Health Is Judged

{% hint style="info" %}
The memory monitor is a preview and off by default. It turns on when the container starts with `-e USE_FSMV2_MEMORY_MONITOR=true`. See [Container: Configuration](README.md#configuration).
{% endhint %}

Without the preview, UMH Core judges memory by one number: the memory the container uses, as a share of its memory limit, or of the machine's memory when the container has no limit. Memory is degraded from 80%.

With the preview on, UMH Core measures memory once per second and judges it on four causes. Memory is degraded while any of them holds:

- **The container is close to its own limit** (`container-limit-full`). This is judged only when the container has a memory limit.
- **The machine is close to full** (`host-memory-full`). A higher container limit does not help then. Free memory on the machine, or add memory to it.
- **Processes wait for memory** (`memory-pressure`). The kernel reports how long processes in the container waited for memory, as pressure stall information (PSI). This needs cgroup v2 with PSI turned on, the same as CPU pressure (see [Enable CPU pressure stats](enable-cpu-pressure-stats.md)).
- **The kernel stopped a process because memory ran out** (`oom-kill`, out of memory). UMH Core counts only the kills after umh-core started, so it does not report a kill that restarted umh-core itself. The status message also shows the kernel's count since the container started, which includes those kills.

The size and usage causes use 60-second means, so a short peak does not degrade the instance. Each cause degrades at one value and recovers at a lower one, so the status does not flicker. Memory also counts as degraded, and new bridges wait, whenever no reading from the last three seconds is available.

## Memory thresholds

| Cause | Degraded from | Healthy again below | Gauge |
|---|---|---|---|
| `container-limit-full` | 0.8 (80%) of the container's own memory limit, as a 60s mean. Judged only when the container has a limit. Without one, `memory_avg_used_fraction` still shows the container's share of the machine's memory, but only `host-memory-full` judges | 0.75 | `memory_avg_used_fraction` while `memory_total_is_cgroup_limit` is 1 |
| `host-memory-full` | 0.8 (80%) of the machine's memory, as a 60s mean | 0.75 | `memory_avg_host_used_bytes` / `memory_host_total_bytes` |
| `memory-pressure` | 0.20: processes in the container waited for memory for 20% of the last minute | 0.12 | `memory_pressure_avg60_ratio` |
| `oom-kill` | 1 OOM kill in the container within the last hour | 0 for a whole hour | `memory_oom_kills_last_hour` |

The preview reports its readings as Prometheus gauges. When a reading fails, the gauges keep their previous value. Only the age of `memory_last_sample_unix` shows the gap. [Metrics: Memory gauges](../agent/metrics.md#memory-gauges) lists them.
