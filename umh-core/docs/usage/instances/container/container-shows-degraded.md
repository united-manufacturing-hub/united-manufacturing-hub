# Container Shows Degraded

Follow this page when the Container card on the instance details page shows Degraded. The badge reason lists one line per degraded resource, such as `Memory degraded: Memory utilization critical`. [The Container card](README.md#the-container-card) explains each line. Some steps need access to the Docker host.

While the container is degraded, new bridges wait to start. If a bridge must start before the resource problem is fixed, use the emergency setting. See [Emergency settings: Turn off bridge admission](../../data-flows/bridges-do-not-start.md#emergency-settings-turn-off-bridge-admission).

## Memory

Raise the memory limit of the container, or remove bridges or flows. The [Sizing Guide](../../../production/sizing-guide.md#memory) lists how much memory UMH Core needs.

If the [memory monitor preview](how-memory-health-is-judged.md) names `host-memory-full` as the cause, the machine is out of memory. A higher container limit does not help. Free memory on the machine or add memory to it.

## Disk

Free space on the `/data` volume, or shorten how long Redpanda keeps data. See [Disk](../../../production/sizing-guide.md#disk).

## CPU

[CPU status](README.md#cpu-status) explains each status. [How CPU Health Is Judged](how-cpu-health-is-judged.md) explains the signals behind them.

| Status | What to do |
|---|---|
| **CPU limited** | Raise the CPU limit, or reduce the load on the instance. |
| **CPU contention** | See [CPU Contention While Cores Are Free](cpu-contention-while-cores-are-free.md). |
| **CPU taken by the server** | On your virtualization platform, give this virtual machine more guaranteed CPU, or move the other virtual machines off the server. |
| **CPU running near full** | Add CPU capacity, reduce load, or raise the CPU limit. If the machine is full and this container is not the cause, reserve or pin cores for UMH Core and also set a CPU limit, or reduce what else runs on the machine. A CPU limit only caps UMH Core's usage. It reserves no CPU for UMH Core. |

When **CPU healthy** shows a `Limited visibility:` line, set a CPU limit or [enable CPU pressure stats](enable-cpu-pressure-stats.md) for full monitoring. The line does not degrade the container.

When the status says `CPU not measured` and names a file, check whether the container can read that file:

```bash
docker exec umh-core cat /sys/fs/cgroup/cpu.stat
```

Replace `umh-core` with the name of your container and the path with the file the status names.
