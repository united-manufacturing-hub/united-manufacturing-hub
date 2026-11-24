# How Memory Health Is Judged

{% hint style="info" %}
The memory monitor is a preview and off by default. It turns on when the container starts with `-e USE_FSMV2_MEMORY_MONITOR=true`. See [Container: Configuration](README.md#configuration).
{% endhint %}

Without the preview, UMH Core judges memory by one number: the memory the container uses, as a share of its memory limit, or of the machine's memory when the container has no limit. Memory is degraded above 80%.

With the preview on, UMH Core measures memory once per second and judges it on four causes. Memory is degraded while any of them holds:

- **The container is close to its own limit** (`container-limit-full`). This is judged only when the container has a memory limit.
- **The machine is close to full** (`host-memory-full`).
- **Processes wait for memory** (`memory-pressure`). The kernel reports how long processes in the container waited for memory, as pressure stall information (PSI). This needs cgroup v2 with PSI turned on, the same as CPU pressure (see [Enable CPU pressure stats](enable-cpu-pressure-stats.md)).
- **The kernel stopped a process because memory ran out** (`oom-kill`, out of memory). UMH Core counts only the kills after umh-core started, so it does not report a kill that restarted umh-core itself. The status message also shows the kernel's count since the container started, which includes those kills.

The size and usage causes use 60-second means, so a short peak does not degrade the instance. Each cause degrades at one value and recovers at a lower one, so the status does not flicker. [Container: Memory monitor preview thresholds](README.md#memory-monitor-preview-thresholds) gives the values, and [Container Shows Degraded: Memory](container-shows-degraded.md#memory) says what to do. Memory also counts as degraded, and new bridges wait, whenever no reading from the last three seconds is available.

The preview reports its readings as Prometheus gauges. See [Container: Metrics](README.md#metrics).
