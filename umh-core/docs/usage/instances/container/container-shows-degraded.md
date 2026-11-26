# Container Shows Degraded

Follow this page when the Container card on the instance details page shows Degraded. Some steps need access to the Docker host. The badge reason lists one line per degraded resource, such as `Memory degraded: Memory utilization critical`. [Container states](README.md#container-states) explains each line.

While the container is degraded, new bridges wait to start. If a bridge must start before the resource problem is fixed, use the emergency setting. See [Emergency settings: Turn off bridge admission](../../data-flows/bridges-do-not-start.md#emergency-settings-turn-off-bridge-admission).

## Memory

Without the [memory monitor](memory-health.md), the line reads `Memory utilization critical`. Raise the memory limit of the container, or remove bridges or flows. [Sizing Guide: Memory](../../../production/sizing-guide.md#memory) lists how much memory UMH Core needs.

With the memory monitor, the line gives the memory status. [Memory status](README.md#memory-status) explains each status.

| Status | What to do |
|---|---|
| **Memory running near its limit** | Raise the memory limit of the container, or remove bridges or flows. |
| **Machine memory running near full** | Add memory to the machine, or reduce what else runs on it. A higher container limit does not help, because the machine has no memory left to give. |
| **Memory contention** | Reduce the load on the instance, or give it more memory. If other software on the machine uses much of its memory, reduce that software too. |
| **Memory ran out** | Give the instance more memory, or reduce the load on it. The status stays until a whole hour passes without another OOM kill. |

## Disk

Free space on the `/data` volume, or shorten how long Redpanda keeps data. See [Disk](../../../production/sizing-guide.md#disk).

## CPU

[CPU status](README.md#cpu-status) explains each status. [CPU Health](cpu-health.md) explains the signals behind them.

| Status | What to do |
|---|---|
| **CPU limited** | Raise the CPU limit, or reduce the load on the instance. |
| **CPU contention** | See [CPU Contention While Cores Are Free](cpu-contention-while-cores-are-free.md). |
| **CPU taken by the server** | On your virtualization platform, give this virtual machine more guaranteed CPU, or move the other virtual machines off the server. |
| **CPU running near full** | Add CPU capacity, reduce load, or raise the CPU limit. If the machine is full and this container is not the cause, reserve or pin cores for UMH Core and also set a CPU limit, or reduce what else runs on the machine. A CPU limit only caps UMH Core's usage. It reserves no CPU for UMH Core. |

When **CPU healthy** shows a `Limited visibility:` line, set a CPU limit or [enable CPU pressure stats](enable-cpu-pressure-stats.md) for full monitoring. The line does not degrade the container.

When the status says `CPU not measured` and names a file, check whether the container can read that file. Run `cat` with the file the status names inside the container, the way your [deployment](../../../production/deployment/README.md) runs a command in a container. With Docker:

```bash
docker exec <container name> cat /sys/fs/cgroup/cpu.stat
```
