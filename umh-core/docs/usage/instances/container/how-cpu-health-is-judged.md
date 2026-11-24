# How CPU Health Is Judged

{% hint style="info" %}
The CPU health check is the only CPU status. From UMH Core v0.45.4, it always runs and `USE_FSMV2_CPU` is ignored. On v0.44.46 to v0.45.3, it is on by default and `USE_FSMV2_CPU=false` turns it off. See [Container: Configuration](README.md#configuration).
{% endhint %}

UMH Core checks whether an instance has the CPU it needs. The Management Console shows the result as a CPU status on the Container card. While CPU health is degraded, new bridges wait (see [When a new bridge waits for CPU](#when-a-new-bridge-waits-for-cpu)). [Container: CPU status](README.md#cpu-status) lists the statuses, and [Container Shows Degraded](container-shows-degraded.md#cpu) says what to do for each.

## How UMH Core judges CPU health

Headroom is the CPU the instance can still use. [Container: CPU thresholds](README.md#cpu-thresholds) explains each signal and gives its values, and [why average CPU utilization is the wrong signal](https://www.theocharis.dev/blog/why-we-should-get-rid-of-average-cpu-utilization/) explains the reasoning. An instance degrades in two cases:

- Its headroom falls below its threshold.
- Throttling, CPU pressure or steal show that its work is delayed.

High usage alone degrades an instance only in a last-resort case: no CPU limit is set, the container cannot read CPU pressure, and the machine's free cores cannot be measured. **Instance usage** in [Container: CPU thresholds](README.md#cpu-thresholds) gives the values.

### With and without a CPU limit

Headroom depends on whether the container has a CPU limit (a Docker `--cpus` or a Kubernetes CPU limit):

- **Without a limit**, UMH Core averages how busy the whole machine is over 60 seconds. It reports degraded when less than about one core of the machine is free. It also shows the container's own usage, so you can see how much of the machine total is UMH Core.
- **With a limit**, UMH Core measures headroom against the limit's cores. It reports degraded once usage passes 90% of the limit, which keeps the last 10% in reserve. It also reports degraded when the kernel throttles the container in more than 5% of scheduling periods. Machine headroom still counts too, so a full machine degrades the instance even below its limit.

## When a new bridge waits for CPU

While CPU health is degraded, UMH Core does not start an additional bridge on the instance, because it would compete for CPU that is already short. Bridges that already run keep running. Real CPU use varies per bridge, so a bridge can wait on CPU health before the instance reaches its bridge limit. [Bridges Do Not Start](../../data-flows/bridges-do-not-start.md) covers every reason a bridge waits and the emergency setting that turns the checks off.
