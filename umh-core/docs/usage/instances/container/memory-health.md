# Memory Health

{% hint style="info" %}
The memory monitor is a preview and off by default. It turns on when the container starts with the environment variable `USE_FSMV2_MEMORY_MONITOR=true`. See [Container: Configuration](README.md#configuration).
{% endhint %}

The memory health check decides whether the instance has the memory it needs. The Management Console shows the result in the Memory row and the badge of the Container card. [Container: Memory thresholds](README.md#memory-and-disk) and [Container: Memory monitor preview thresholds](README.md#memory-monitor-preview-thresholds) give the values, and [Container Shows Degraded: Memory](container-shows-degraded.md#memory) says what to do.

## Without the memory monitor preview

Memory health is judged on one number: the memory the container uses, as a share of its memory limit, or of the machine's memory when the container has no limit. Memory is degraded above 80%.

## With the memory monitor preview

UMH Core measures memory once per second. Memory is degraded while any of these causes holds:

| Cause | Degraded when | Judged only when |
|---|---|---|
| `container-limit-full` | The container is close to its own memory limit. | the container has a memory limit |
| `host-memory-full` | The machine is close to full. | always |
| `memory-pressure` | Processes in the container waited for memory, as the kernel reports in its pressure stall information (PSI). | the container runs with cgroup v2 and PSI, the same as for CPU pressure (see [Enable CPU pressure stats](enable-cpu-pressure-stats.md)) |
| `oom-kill` | The kernel stopped a process in the container because memory ran out (out of memory, OOM). Only the kills after umh-core started count, so a kill that restarted umh-core itself does not. The status message also shows the kernel's count since the container started, which includes those kills. | always |

`container-limit-full` and `host-memory-full` use 60-second means. Each cause degrades at one value and recovers at a lower one. Memory also counts as degraded, and new bridges wait, whenever no reading from the last three seconds is available.

The reason line on the Container card names the cause, for example `Memory degraded: Memory running near its limit` for `container-limit-full`.

## Metrics

The memory monitor preview reports each reading as a Prometheus gauge. See [Container: Memory monitor preview gauges](README.md#memory-monitor-preview-gauges).
