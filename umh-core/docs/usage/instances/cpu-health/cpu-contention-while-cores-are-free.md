# CPU contention while cores are free

The **CPU contention** status means that work in the UMH Core container had to wait for a CPU core. It can appear while the machine has free cores. Work that waits finishes late, for example a bridge reading its PLC or UMH Core applying a configuration change. The share of time spent waiting is called CPU pressure. When it rises above its **Degrades** threshold in [Thresholds](README.md#thresholds), the CPU health is degraded. New bridges then wait (see [When a new bridge waits for CPU](README.md#when-a-new-bridge-waits-for-cpu)). The CPU health recovers once pressure falls below its **Recovers** threshold.

## What to do

To lower the pressure, give the machine more cores, move other software off it, or raise the container's CPU limit if it has one. More cores help even when the machine looks idle, because more of the tasks that start together can then run at once.

## Background

Linux measures CPU pressure per container, as the share of time in which at least one task was ready to run and waited for a core. At 10%, for example, a task was waiting during about 6 of every 60 seconds.

Any program on Linux can wait for a core on a machine whose cores are free on average. A CPU usage graph averages over a second or longer, which hides a wait of a few milliseconds. A program that starts more tasks at one moment than there are cores makes the extra tasks wait. The cores can then be idle for the rest of the second. Tasks also wait when other software on the machine holds the cores. They wait as well when the container has used up its CPU limit (see **Throttling** in the [Glossary](README.md#glossary)).

UMH Core is one such program. Every 100 ms, it starts the checks and updates for all its bridges and data flows at the same moment. This is UMH Core's own internal work, separate from how often a bridge polls its PLC. UMH is moving this work to FSMv2, a new design of UMH Core's internals that spreads it over time.

## Read the CPU pressure value yourself

Run `docker exec <container name> cat /sys/fs/cgroup/cpu.pressure` to print the value. The CPU health check uses the `avg60` value of the `some` line, as defined in the Linux kernel's [Pressure Stall Information](https://docs.kernel.org/accounting/psi.html) documentation. The kernel updates it every 2 seconds and weights recent seconds more than older ones. The 6-of-60-seconds figure above is exact only under a steady load. [Why we should get rid of average CPU utilization](https://www.theocharis.dev/blog/why-we-should-get-rid-of-average-cpu-utilization/) shows with examples how an average CPU graph hides these waits.
