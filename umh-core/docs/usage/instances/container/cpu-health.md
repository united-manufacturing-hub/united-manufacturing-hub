# CPU Health

{% hint style="info" %}
The CPU health check is the only CPU status. From UMH Core v0.45.4, it always runs and `USE_FSMV2_CPU` is ignored. On v0.44.46 to v0.45.3, it is on by default and `USE_FSMV2_CPU=false` turns it off. See [Container: Configuration](README.md#configuration).
{% endhint %}

The CPU health check decides whether an instance has the CPU it needs. The Management Console shows the result as the CPU status on the Container card. [Container: CPU status](README.md#cpu-status) lists the statuses, [Container: CPU thresholds](README.md#cpu-thresholds) gives the values of each signal, and [Container Shows Degraded](container-shows-degraded.md#cpu) says what to do for each status.

## When CPU health is degraded

CPU health is degraded in either of these cases:

- The headroom falls below its threshold. Headroom is the CPU the instance can still use.
- Throttling, CPU pressure or steal is above its threshold. Each of them shows that work of the instance is delayed.

High usage alone degrades CPU health only when the check has no other signal: no CPU limit is set, the container cannot read CPU pressure, and the machine's free cores cannot be measured. **Instance usage** in [Container: CPU thresholds](README.md#cpu-thresholds) gives the values.

## With and without a CPU limit

The headroom depends on whether the container has a CPU limit, such as a Docker `--cpus` or a Kubernetes CPU limit.

| | Without a CPU limit | With a CPU limit |
|---|---|---|
| Headroom | The free cores of the whole machine, as a 60-second mean. Degraded when less than about one core is free. | The cores of the limit that the instance does not use. Degraded once usage passes 90% of the limit. |
| Throttling | Not measured. | Degraded when the kernel paused the container in more than 5% of scheduling periods. |
| Machine headroom | Counts, as above. | Also counts, so a full machine degrades the instance below its limit. |
| CPU row on the Container card | The busy cores of the whole machine against the machine's cores. | The instance's own usage against the limit. |

## New bridges while CPU health is degraded

While CPU health is degraded, UMH Core starts no additional bridge on the instance. Bridges that already run keep running. A bridge can wait for CPU health before the instance reaches its bridge limit. [Bridges Do Not Start](../../data-flows/bridges-do-not-start.md) lists every reason a bridge waits and the emergency setting that turns the checks off.

## Metrics

The CPU health check reports each reading as a Prometheus gauge. See [Container: CPU health gauges](README.md#cpu-health-gauges).
