# CPU Health

CPU health is based on one question: does this container have a CPU limit? (a Docker `--cpus` or Kubernetes CPU limit). The answer decides what UMH measures against.

With no limit, the whole machine is the ceiling. UMH watches how busy the machine is (60-second average) and reports degraded when less than about one core is free, because at that point everything on the box, UMH included, starts waiting for CPU (the same waiting the pressure signal measures). Next to the machine total, the view shows the container's own usage, so you can see roughly what is UMH vs. everything else.

With a limit set, the limit is the ceiling. UMH measures headroom against those cores, not the machine, and reports degraded when the headroom runs out or the kernel throttles the container. A full machine can still slow the container below its limit, so where UMH can see the machine, it warns on that too.

High CPU usage alone is not a problem: an instance can run at high utilization and stay healthy as long as there is spare headroom, meaning a free core on the host or room under the limit. *Busy* is not the same as *starved* or *full*. UMH degrades when the headroom runs out (the host is full, or the limit is exhausted) or when work is actually being delayed; where the system reports them, it shows the three sharp signals: throttling, pressure, and steal. These are kernel-defined facts, not interpretations. The minimum recommendation is 4 vCPU; below that the 1-core headroom reserve leaves too little room for work. (For the reasoning, see our write-up on [why average CPU utilization is the wrong signal](https://www.theocharis.dev/blog/why-we-should-get-rid-of-average-cpu-utilization/).)

## What each CPU status means

| Status | What it means | What to do |
|--------|---------------|------------|
| **CPU healthy** | The instance has the CPU it needs. Usage is shown for context (for example, "1.2 of 4 cores"). | Nothing. |
| **CPU healthy, limited visibility** | The instance looks fine, but UMH can't fully measure CPU health here: no CPU limit is set and the operating system isn't reporting CPU-pressure statistics. | To turn on full monitoring, set a CPU limit for the instance, or enable Linux pressure stats (boot the OS with `psi=1`). |
| **CPU limited** | The instance hit its CPU limit and was paused until the next scheduling cycle (for example, in 12% of cycles over the last minute). Work is being delayed. | Raise the instance's CPU limit, or reduce the load on it. |
| **CPU contention** | Tasks inside the instance spent time waiting for a free CPU core (for example, 23% of the last minute). | Reduce the load on the instance, or give it more CPU. If other workloads share this server, they may be competing for it. |
| **CPU taken by the server** | Other virtual machines on the same physical server took CPU this instance needed. This is outside UMH's control. | On your virtualization platform, give this VM more guaranteed CPU, or reduce the other VMs sharing the server. |
| **CPU running near full** | The host or the container's CPU limit is exhausted: there is no room to absorb the next burst of work. With host stats readable, this fires when less than one core is free. Without host stats (no `/proc/stat`), it fires at a coarser proxy: sustained container usage above 70% of the machine's cores. The Technical Details body names the specific sub-case (host full, limit exhausted, or high usage without host stats). | Add CPU capacity, reduce load, or raise the instance's CPU limit. If the host is full but this container isn't the cause, other software on the host is competing: give UMH dedicated CPU (reserve or pin cores), or reduce what else runs here. A CPU limit caps UMH; it does not protect it from neighbours. If UMH could not confirm starvation directly (no CPU limit, no pressure stats), setting a limit or enabling Linux pressure stats (`psi=1`) adds the richer starvation causes on top. |

{% hint style="info" %}
**No CPU limit and no pressure stats?** UMH says so plainly ("limited visibility") rather than
guessing. Setting a CPU limit on the instance, or booting the operating system with `psi=1`, lets
UMH measure starvation directly and turns on full CPU health monitoring.
{% endhint %}

## When UMH refuses a new bridge

When CPU is in any of the "something's wrong" states above, UMH won't start an additional bridge on
the instance, because a new bridge would only compete for CPU that's already short. The refusal
message names the same cause and fix as the status, for example:

```
Can't add another bridge: this instance is already hitting its CPU limit. Raise the limit or reduce load first.
```

This is separate from the *capacity* ceiling (how many bridges a given number of CPU cores can hold).
For that, see the [Sizing Guide](./sizing-guide.md). Note that the capacity number is a ceiling, not
a guarantee: because real CPU use varies per bridge, UMH can refuse a bridge on CPU health before you
reach it.

## How UMH decides

UMH judges CPU health over the last 60 seconds, not on instantaneous readings. A brief spike
won't flip the status, and a status won't clear until the condition actually eases, so the status
stays stable instead of flickering.

## Glossary

| Term | Meaning |
|------|---------|
| **Throttling** | The system caps each container to its CPU limit in short, repeating periods (about 100 ms). If the container needs more within a period, it's paused until the next one, so even a workload whose average looks fine can be paused during bursts. Frequent pausing means work waits. |
| **CPU pressure** | How much time tasks spent waiting for a free CPU core (Linux Pressure Stall Information, PSI). High pressure means CPU is the bottleneck. |
| **CPU steal** | Time the hypervisor gave this machine's CPU to other virtual machines (or, on burstable cloud instances, the point where your CPU credits run out). High steal means the server is oversubscribed. |
| **Host contention** | CPU used by software outside UMH on the same machine. UMH can't see what those processes are, only that they're using CPU it needs. |
