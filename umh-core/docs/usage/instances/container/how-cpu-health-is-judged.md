# How CPU Health Is Judged

{% hint style="info" %}
The CPU health check is on by default from UMH Core v0.44.46. `USE_FSMV2_CPU=false` turns it off. See [Container: Configuration](README.md#configuration).
{% endhint %}

UMH Core checks whether an instance has the CPU it needs. The Management Console shows the result as a CPU status on the Container card. While CPU health is degraded, new bridges wait (see [When a new bridge waits for CPU](#when-a-new-bridge-waits-for-cpu)). [Instance Details Page: CPU status](../instance-details-page.md#cpu-status) lists the statuses, and [Container](README.md#what-to-do-for-each-cpu-status) says what to do for each.

## How UMH Core judges CPU health

Headroom is the CPU the instance can still use. The [Glossary](#glossary) explains this and the other terms below, and [why average CPU utilization is the wrong signal](https://www.theocharis.dev/blog/why-we-should-get-rid-of-average-cpu-utilization/) explains the reasoning. An instance degrades in two cases:

- Its headroom falls below its threshold (see [CPU thresholds](../instance-details-page.md#cpu-thresholds)).
- Throttling, CPU pressure or steal show that its work is delayed.

High usage alone degrades an instance only in a last-resort case: no CPU limit is set, the container cannot read CPU pressure, and the machine's free cores cannot be measured. **Instance usage** in [CPU thresholds](../instance-details-page.md#cpu-thresholds) gives the values.

### With and without a CPU limit

Headroom depends on whether the container has a CPU limit (a Docker `--cpus` or a Kubernetes CPU limit):

- **Without a limit**, UMH Core averages how busy the whole machine is over 60 seconds. It reports degraded when less than about one core of the machine is free. It also shows the container's own usage, so you can see how much of the machine total is UMH Core.
- **With a limit**, UMH Core measures headroom against the limit's cores. It reports degraded once usage passes 90% of the limit, which keeps the last 10% in reserve. It also reports degraded when the kernel throttles the container in more than 5% of scheduling periods. Machine headroom still counts too, so a full machine degrades the instance even below its limit.

## When a new bridge waits for CPU

While CPU health is degraded, UMH Core will not start an additional bridge on the instance, because it would
compete for CPU that is already short. The bridge waits, and its status reason names the
resource that stopped it, here the CPU. For every reason a bridge waits and the full procedure, see
[Bridges Do Not Start](../../data-flows/bridges-do-not-start.md). Bridges already running are left alone.

To start bridges anyway in an emergency, use the emergency setting [Turn off bridge admission](../../data-flows/bridges-do-not-start.md#emergency-settings-turn-off-bridge-admission). It turns off every admission check, including CPU.

Waiting for CPU health is separate from the bridge limit. [Bridge limit](../../data-flows/bridges-do-not-start.md#bridge-limit) covers the number of bridges a core count can hold. Real CPU use varies per bridge, so a bridge can wait on CPU health before the instance reaches its bridge limit.

## Known limitation

A container pinned to a subset of the machine's CPUs (`cpuset`) with no CPU limit set can print a
usage pair measured against different core counts, such as "using 6.0 of 2 cores", beside a
healthy verdict. Set a CPU limit on such a container to get a correct reading.

## Glossary

| Term | Meaning |
|------|---------|
| **Headroom** | The CPU an instance can still use. Without a CPU limit, UMH Core measures it against the machine's cores. With a limit, it measures it against the limit. |
| **Throttling** | The kernel caps a container to its CPU limit in short repeating periods, about 100 ms each by default. A container that needs more within a period is paused until the next one, so a workload whose average looks fine can still be paused during bursts. |
| **CPU pressure** | The share of time in which tasks of the instance waited for a CPU core, from Linux Pressure Stall Information (PSI). [CPU contention while cores are free](../troubleshooting/cpu-contention-while-cores-are-free.md) explains it in detail. |
| **CPU steal** | Time the hypervisor scheduled this machine's CPU elsewhere. High steal usually means the physical server is oversubscribed. A burstable cloud instance whose CPU credits have run out is capped by a different mechanism, but the guest counts that capped time as steal too. |
| **Host contention** | CPU used by software outside UMH Core on the same machine. UMH Core cannot see which processes those are, only that they are using CPU it needs. It is not the **CPU contention** status: that one is measured inside the instance, as tasks of UMH Core's own waiting for a free core. |
