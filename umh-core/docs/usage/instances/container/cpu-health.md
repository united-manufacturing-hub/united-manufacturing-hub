# CPU Health

{% hint style="info" %}
The CPU health check is the only CPU status. From UMH Core v0.45.4, it always runs and `USE_FSMV2_CPU` is ignored. On v0.44.46 to v0.45.3, it is on by default and `USE_FSMV2_CPU=false` turns it off. See [Container: Configuration](README.md#configuration).
{% endhint %}

The CPU health check decides whether the container has the CPU it needs. The Management Console shows the result as the [CPU status](README.md#cpu-status) on the Container card.

## When CPU health is degraded

CPU health is degraded in either of these cases:

- The headroom falls below its threshold. Headroom is the CPU the container can still use.
- Throttling, CPU pressure or steal is above its threshold. Each of them shows that work of the container is delayed.

High usage alone degrades CPU health only when the check has no other signal: no CPU limit is set, the container cannot read CPU pressure, and the machine's free cores cannot be measured. **Instance usage** in [Container: CPU thresholds](README.md#cpu-thresholds) gives the values.

[Container Shows Degraded: CPU](container-shows-degraded.md#cpu) says what to do for each status.

## With and without a CPU limit

The headroom depends on whether the container has a CPU limit, such as a Docker `--cpus` or a Kubernetes CPU limit. [Container: CPU thresholds](README.md#cpu-thresholds) gives the value of each check.

| | Without a CPU limit | With a CPU limit |
|---|---|---|
| Headroom | Machine headroom: the free cores of the whole machine, as a 60-second mean. | Instance headroom: the cores of the limit that the container does not use. |
| Throttling | Not measured. | Measured. |
| Machine headroom | Counts, as above. | Also counts, so a full machine degrades the container below its limit. |

[Container: The CPU row](README.md#the-cpu-row) explains what the CPU row on the Container card shows with and without a limit.

## Metrics

The CPU health check reports each reading as a Prometheus gauge. See [Container: CPU health gauges](README.md#cpu-health-gauges).
