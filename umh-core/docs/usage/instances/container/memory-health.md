# Memory Health

{% hint style="info" %}
**Early access.** The memory monitor is off by default. It turns on when the container starts with the environment variable `USE_FSMV2_MEMORY_MONITOR=true`. See [Container: Configuration](README.md#configuration).
{% endhint %}

The memory health check decides whether the container has the memory it needs. The Management Console shows the result in the Memory row of the Container card and, while the memory is degraded, as the [Memory status](README.md#memory-status) in its badge reason.

## Without the memory monitor

Memory health is judged on one number: the memory the container uses, as a share of its memory limit, or of the machine's memory when the container has no limit. [Container: Memory and disk](README.md#memory-and-disk) gives the threshold.

## When memory health is degraded

With the memory monitor, umh-core measures memory once per second. Memory health is degraded while any of these readings is past its threshold:

- Instance memory or Machine memory: the container is close to its memory limit, or the whole machine is close to full.
- Memory pressure: processes in the container wait for memory.
- OOM kills: the kernel stopped a process in the container because memory ran out.

[Container: Memory monitor thresholds](README.md#memory-monitor-thresholds) gives the value of each reading and when it is measured.

[Container Shows Degraded: Memory](container-shows-degraded.md#memory) says what to do for each status.

## Metrics

The memory monitor reports each reading as a Prometheus gauge. See [Container: Memory monitor gauges](README.md#memory-monitor-gauges).
