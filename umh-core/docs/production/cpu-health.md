# CPU Health

{% hint style="info" %}
**Early access.** This reporting needs UMH Core v0.44.37 or later, started with `USE_FSMV2_CPU=true`
and `USE_FSMV2_TRANSPORT=true`. Both are read at startup, so changing them requires a container
restart. Without them, an instance is marked degraded whenever its CPU usage stays above 70% of
the cores it may use, whether or not work is being delayed.
{% endhint %}

UMH Core reports whether an instance has the CPU it needs. The Management Console shows the result as a CPU status on the instance's detail page, and while that status is degraded UMH will not start another bridge there. High usage on its own does not make an instance degraded: it degrades when its headroom is gone, or when its work is measurably delayed, which UMH reads from throttling, CPU pressure and steal. (For the reasoning, see [why average CPU utilization is the wrong signal](https://www.theocharis.dev/blog/why-we-should-get-rid-of-average-cpu-utilization/).)

What UMH measures headroom against depends on whether the container has a CPU limit (a Docker `--cpus` or a Kubernetes CPU limit). With no limit, the machine is the ceiling: UMH averages how busy the machine is over 60 seconds and reports degraded when less than about one core is free. It also shows the container's own usage, so you can see how much of the machine total is UMH. With a limit, the limit is the ceiling: UMH measures headroom against those cores rather than the machine, and reports degraded once its usage passes 90% of the limit, the last 10% being held in reserve, or when the kernel throttles it.

## What each CPU status means

| Status | What it means | What to do |
|--------|---------------|------------|
| **CPU healthy** | The instance has the CPU it needs. Usage is shown for context, for example "1.2 of 4 cores". | Nothing. |
| **CPU healthy, limited visibility** | The instance looks fine, but UMH cannot fully measure CPU health here: no CPU limit is set and the operating system is not reporting CPU-pressure statistics. | For full monitoring, set a CPU limit, or boot the operating system with `psi=1`. |
| **CPU limited** | The instance hit its CPU limit and was paused until the next scheduling cycle, for example in 12% of cycles over the last minute. Work is being delayed. | Raise the CPU limit, or reduce the load on the instance. |
| **CPU contention** | Tasks inside the instance spent time waiting for a free CPU core, for example 23% of the last minute. | Reduce the load, or give the instance more CPU. Workloads sharing the server may be competing for it. |
| **CPU taken by the server** | Other virtual machines on the same physical server took CPU this instance needed. | On your virtualization platform, give this VM more guaranteed CPU, or move the other VMs off the server. |
| **CPU running near full** | There is no room left for the next burst of work, because either the machine is full or the instance is at its CPU limit. The status message says which. | Add CPU capacity, reduce load, or raise the CPU limit. If the host is full and this container is not the cause, reserve or pin cores for UMH, or reduce what else runs on the machine: a CPU limit caps UMH rather than protecting it. |

## Where to see it

The Management Console shows CPU on the instance's detail page: the status, the usage row, and a
Technical Details section listing every signal UMH can measure on that machine.

Both the status and Technical Details carry the thresholds, so you do not have to look them up.
A healthy instance states its remaining headroom, and each Technical Details line gives the
reading next to the mark that would change it:

```text
CPU healthy. This instance is using 0.3 of 2 cores (15% of its limit) and can use 1.5 more before it is marked degraded.

Technical Details:
Instance headroom 1.5 cores = 2 total - 0.3 used - 0.2 reserved (degrades below 0).
Throttling 2% (degrades above 5%).
Pressure 4% (degrades above 20%).
Steal not available (not possible).
```

Which lines appear depends on what the machine can measure: a machine whose core count is readable also gets a Machine headroom line, and an instance with no CPU limit gets neither the instance headroom nor the throttling line.

A signal that has already fired shows what would clear it instead, for example
`Throttling 12% (recovers below 3%)`. A signal this machine cannot measure says so rather than
reading zero.

## Thresholds

Each signal degrades at one value and recovers at a lower one, which is what keeps the status from
flickering. All readings are 60-second figures. A reading has to pass a threshold, not merely reach
it, except where the table says "at".

| Signal | Degrades | Recovers | Measured only when |
|--------|----------|----------|--------------------|
| **Throttling** | above 5% of scheduling periods | below 3% | a CPU limit is set |
| **CPU pressure** | above 20% (PSI `avg60`) | below 12% | the kernel publishes PSI |
| **CPU steal** | above 10% | below 6% | the machine is a virtual machine |
| **Machine headroom** | less than 1 core free | 1.5 cores free | the machine's core count is readable |
| **Limit headroom** | usage past 90% of the limit | below 85% of the limit | a CPU limit is set |
| **Usage of the machine** | at 70% | below 60% | host statistics are unreadable (fallback for machine headroom) |

Steal uses the 95th percentile once 20 samples are in, and the mean before that, so a fresh
instance is judgeable within seconds of starting. Bare metal reports no steal at all, so on a
physical machine that signal reads "not possible" rather than 0%.

## When UMH refuses a new bridge

While CPU is degraded, UMH will not start an additional bridge on the instance, because it would
compete for CPU that is already short. The bridge stays pending, and its status reason names the
resource gate that stopped it, usually "System in degraded state". For the cause and the fix, read
the instance's CPU status. Bridges already running are left alone.

To turn this off, so that a degraded CPU no longer stops a new bridge:

```yaml
agent:
  enableResourceLimitBlocking: false
```

This is separate from the capacity ceiling, the number of bridges a given core count can hold, which the [Sizing Guide](./sizing-guide.md) covers. That number is a ceiling rather than a guarantee: because real CPU use varies per bridge, UMH can refuse a bridge on CPU health before you reach it.

## Known limitation

A container pinned to a subset of the machine's CPUs (`cpuset`) with no CPU limit set can print a
usage pair measured against different core counts, such as "using 6.0 of 2 cores", beside a
healthy verdict. Set a CPU limit on such a container to get a correct reading.

## Glossary

| Term | Meaning |
|------|---------|
| **Throttling** | The kernel caps a container to its CPU limit in short repeating periods, about 100 ms each. A container that needs more within a period is paused until the next one, so a workload whose average looks fine can still be paused during bursts. |
| **CPU pressure** | How much time tasks spent waiting for a free CPU core, from Linux Pressure Stall Information (PSI). High pressure means CPU is the bottleneck. |
| **CPU steal** | Time the hypervisor scheduled this machine's CPU elsewhere. High steal usually means the physical server is oversubscribed. A burstable cloud instance whose CPU credits have run out is capped by a different mechanism, but the guest counts that capped time as steal too. |
| **Host contention** | CPU used by software outside UMH on the same machine. UMH cannot see which processes those are, only that they are using CPU it needs. |
