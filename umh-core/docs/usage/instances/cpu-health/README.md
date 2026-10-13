# CPU Health

{% hint style="info" %}
**Early access.** This reporting needs UMH Core v0.44.37 or later, started with
`-e USE_FSMV2_CPU=true` added to the `docker run` command that starts the container. The flag is
read at startup, so changing it means recreating the container. Since v0.44.42, the CPU health
check also runs on an instance started without `AUTH_TOKEN` and `API_URL`. Without the flag, an
instance is marked degraded whenever its CPU usage stays above 70% of the cores it may use, whether
or not work is being delayed.
{% endhint %}

UMH Core reports whether an instance has the CPU it needs. The Management Console shows the result as a CPU status on the instance's detail page. An instance degrades when its headroom is gone. Headroom is the CPU the instance can still use. An instance also degrades when throttling, CPU pressure or steal show that its work is delayed. High usage alone does not degrade an instance. While the status is degraded, new bridges wait (see [When a new bridge waits for CPU](#when-a-new-bridge-waits-for-cpu)). The [Glossary](#glossary) explains each term, and [why average CPU utilization is the wrong signal](https://www.theocharis.dev/blog/why-we-should-get-rid-of-average-cpu-utilization/) explains the reasoning.

Headroom depends on whether the container has a CPU limit (a Docker `--cpus` or a Kubernetes CPU limit). Without a limit, UMH averages how busy the whole machine is over 60 seconds. It reports degraded when less than about one core of the machine is free. It also shows the container's own usage, so you can see how much of the machine total is UMH. With a limit, UMH measures headroom against the limit's cores. It reports degraded once usage passes 90% of the limit, which keeps the last 10% in reserve. It also reports degraded when the kernel throttles the container in more than 5% of scheduling periods.

## What each CPU status means

| Status | What it means | What to do |
|--------|---------------|------------|
| **CPU healthy** | The instance has the CPU it needs. Usage is shown for context, for example "1.2 of 4 cores". | Nothing. |
| **CPU healthy, limited visibility** | The instance looks fine, but UMH cannot fully measure CPU health here: no CPU limit is set and the operating system is not reporting CPU-pressure statistics. | For full monitoring, set a CPU limit, or [enable CPU pressure stats](enable-cpu-pressure-stats.md). |
| **CPU limited** | The instance hit its CPU limit and was paused until the next scheduling period, for example in 12% of periods over the last minute. Work is being delayed. | Raise the CPU limit, or reduce the load on the instance. |
| **CPU contention** | Tasks inside the instance spent time waiting for a free CPU core, for example 23% of the last minute. | See [CPU contention while cores are free](cpu-contention-while-cores-are-free.md). |
| **CPU taken by the server** | Other virtual machines on the same physical server took CPU this instance needed. | On your virtualization platform, give this virtual machine more guaranteed CPU, or move the other virtual machines off the server. |
| **CPU running near full** | There is no room left for the next burst of work, because either the machine is full or the instance is at its CPU limit. The status message says which. | Add CPU capacity, reduce load, or raise the CPU limit. If the machine is full and this container is not the cause, reserve or pin cores for UMH, or reduce what else runs on the machine. A CPU limit only caps UMH's usage. It reserves no CPU for UMH. |

From v0.44.43, if UMH cannot read a file it needs to measure the CPU, the CPU health is degraded and the message names the file, for example `CPU not measured: open /sys/fs/cgroup/cpu.stat: no such file or directory`. To check whether the container can read that file, run `docker exec <container name> cat <file>`, for example `docker exec umh-core cat /sys/fs/cgroup/cpu.stat`.

## Where to see the CPU health

The Management Console shows CPU on the instance's detail page: the status, the usage row, and a
Technical Details section listing every signal UMH can measure on that machine.

Both the status and Technical Details show the thresholds. A healthy instance states its remaining
headroom. Each Technical Details line gives the reading next to the threshold that would change the
status:

```text
CPU healthy. This instance is using 0.3 of 2 cores (15% of its limit) and can use 1.5 more before it is marked degraded.

Technical Details:
Instance headroom 1.5 cores = 2 total - 0.3 used - 0.2 reserved (degrades below 0).
Instance usage not measured (throttling already shows whether this instance is hitting its CPU limit, so this rough estimate is not needed).
Throttling 2% (degrades above 5%).
Pressure 4% (degrades above 20%).
Steal not measured (this instance is not running in a virtual machine, so no other virtual machine can take its CPU).
```

The headroom lines depend on the machine. A machine whose core count is readable also gets a Machine headroom line, which this example leaves out. An instance with no CPU limit gets no Instance headroom line. Every other signal always has a line, even when this machine cannot measure it.

A signal that is already past its threshold shows what would clear it instead, for example
`Throttling 12% (recovers below 3%)`. A signal this machine cannot measure reads `not measured` with
its reason, as the Steal line above does.

## When a new bridge waits for CPU

While CPU is degraded, UMH will not start an additional bridge on the instance, because it would
compete for CPU that is already short. The bridge waits, and its status reason names the
resource that stopped it, here the CPU. For every reason a bridge waits and the full procedure, see
[Bridges Do Not Start](../../data-flows/bridges-do-not-start.md). Bridges already running are left alone.

To start bridges anyway in an emergency, use the emergency setting [Turn off bridge admission](../../data-flows/bridges-do-not-start.md#emergency-settings-turn-off-bridge-admission). It turns off every admission check, including CPU.

Waiting for CPU health is separate from the bridge limit. The [Sizing Guide](../../../production/sizing-guide.md) covers the bridge limit, the number of bridges a core count can hold. Real CPU use varies per bridge, so a bridge can wait on CPU health before the instance reaches its bridge limit.

## Thresholds

Each signal degrades at one value and recovers at another. The two are held apart so the status
does not flicker. Throttling, pressure, steal and usage are worse the higher they read, headroom the
lower. All readings are 60-second figures. A reading must exceed a threshold to count. Where the
table says "at", reaching it is enough.

| Signal | Degrades | Recovers | Measured only when |
|--------|----------|----------|--------------------|
| **Throttling** | above 5% of scheduling periods | below 3% | a CPU limit is set |
| **Pressure** | above 20% (PSI `some avg60`) | below 12% | the container can read `cpu.pressure` (see [Enable CPU pressure stats](enable-cpu-pressure-stats.md)) |
| **Steal** | above 10% | below 6% | the machine is a virtual machine |
| **Machine headroom** | less than 1 core free | more than 1.5 cores free | the machine's core count is readable |
| **Instance headroom** | usage past 90% of the limit | below 85% of the limit | a CPU limit is set |
| **Instance usage** | at 70% | below 60% | no CPU limit is set and the kernel publishes no PSI (last-resort fallback for machine headroom) |

Steal uses the 95th percentile once 20 samples are in, and the mean before that, so a newly started
instance gets a steal reading within seconds. Bare metal reports no steal, so on a physical machine
the Steal line reads `not measured` with its reason.

## Known limitation

A container pinned to a subset of the machine's CPUs (`cpuset`) with no CPU limit set can print a
usage pair measured against different core counts, such as "using 6.0 of 2 cores", beside a
healthy verdict. Set a CPU limit on such a container to get a correct reading.

## Glossary

| Term | Meaning |
|------|---------|
| **Headroom** | The CPU an instance can still use. Without a CPU limit, UMH measures it against the machine's cores. With a limit, it measures it against the limit. |
| **Throttling** | The kernel caps a container to its CPU limit in short repeating periods, about 100 ms each by default. A container that needs more within a period is paused until the next one, so a workload whose average looks fine can still be paused during bursts. |
| **CPU pressure** | The share of time in which tasks of the instance waited for a CPU core, from Linux Pressure Stall Information (PSI). [CPU contention while cores are free](cpu-contention-while-cores-are-free.md) explains it in detail. |
| **CPU steal** | Time the hypervisor scheduled this machine's CPU elsewhere. High steal usually means the physical server is oversubscribed. A burstable cloud instance whose CPU credits have run out is capped by a different mechanism, but the guest counts that capped time as steal too. |
| **Host contention** | CPU used by software outside UMH on the same machine. UMH cannot see which processes those are, only that they are using CPU it needs. It is not the **CPU contention** status above: that one is measured inside the instance, as tasks of UMH's own waiting for a free core. |
