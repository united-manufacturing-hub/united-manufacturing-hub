# CPU Health

UMH watches the CPU health of each instance, reports a status, and refuses to start a new bridge
when CPU is constrained. The status tells you whether work is actually being delayed and what to do
about it.

UMH does not treat *high* CPU usage as a problem on its own. As long as it can measure whether work is
being delayed, an instance can run near 100% and stay healthy; it's just busy. UMH raises a status
when work is being **starved**: hitting its CPU limit, waiting for a free core, or losing CPU to
something else on the machine. The one exception is an instance UMH can't measure at all, with no CPU
limit and no operating-system pressure stats; there, sustained high usage is the only signal left, so
UMH reports it as a low-confidence "CPU running near full" (see the table below) rather than staying
silent. (For the reasoning behind this, see our write-up on why average CPU utilization is the wrong
signal: `<!-- TODO: insert published article URL -->`.)

## What each CPU status means

| Status | What it means | What to do |
|--------|---------------|------------|
| **CPU healthy** | The instance has the CPU it needs. Usage is shown for context (for example, "1.2 of 4 cores"). | Nothing. |
| **CPU healthy, limited visibility** | The instance looks fine, but UMH can't fully measure CPU health here: no CPU limit is set and the operating system isn't reporting CPU-pressure statistics. | To turn on full monitoring, set a CPU limit for the instance, or enable Linux pressure stats (boot the OS with `psi=1`). |
| **CPU limited** | The instance hit its CPU limit and was paused until the next scheduling cycle (for example, in 12% of cycles over the last minute). Work is being delayed. | Raise the instance's CPU limit, or reduce the load on it. |
| **CPU contention** | Tasks inside the instance spent time waiting for a free CPU core (for example, 23% of the last minute). | Reduce the load on the instance, or give it more CPU. If other workloads share this server, they may be competing for it. |
| **CPU taken by the server** | Other virtual machines on the same physical server took CPU this instance needed. This is outside UMH's control. | On your virtualization platform, give this VM more guaranteed CPU, or reduce the other VMs sharing the server. |
| **Host CPU taken by other software** | The machine ran near full and most of the CPU went to software outside UMH; this instance has no reserved CPU here, so it competes for what's left. | Check what else runs on this host. Give UMH dedicated CPU (reserve or pin cores), or reduce what else runs here. A CPU *limit* caps UMH; it does not protect it from neighbours. |
| **CPU running near full** | CPU averaged high over the last minute with no limit set and no pressure statistics, so starvation can't be confirmed, but there's little headroom left. | Set a CPU limit (the simplest fix, and it lets UMH measure starvation directly), or enable Linux pressure stats (`psi=1`). Consider adding CPU capacity. |

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

UMH judges CPU health over the **last 60 seconds**, not on instantaneous readings. A brief spike
won't flip the status, and a status won't clear until the condition actually eases, so the status
stays stable instead of flickering.

It looks at whether work is being **delayed** (throttling, waiting for a core, or CPU lost to the
host) rather than at raw CPU usage, so an instance running near 100% can still read healthy: busy is
not the same as starved. The exception is an instance where UMH can't measure delay at all (no CPU
limit and no pressure stats); there it falls back to flagging sustained high usage, because it's the
only signal left.

## Glossary

| Term | Meaning |
|------|---------|
| **Throttling** | The system caps each container to its CPU limit in short, repeating periods (about 100 ms). If the container needs more within a period, it's paused until the next one, so even a workload whose average looks fine can be paused during bursts. Frequent pausing means work waits. |
| **CPU pressure** | How much time tasks spent waiting for a free CPU core (Linux Pressure Stall Information, PSI). High pressure means CPU is the bottleneck. |
| **CPU steal** | Time the hypervisor gave this machine's CPU to other virtual machines (or, on burstable cloud instances, the point where your CPU credits run out). High steal means the server is oversubscribed. |
| **Host contention** | CPU used by software outside UMH on the same machine. UMH can't see what those processes are, only that they're using CPU it needs. |
