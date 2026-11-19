# Sizing Guide

## Recommended machine

| | UMH Core | UMH Core + [Historian](../usage/historian/sizing.md) on the same machine |
| --- | --- | --- |
| CPU | 4 vCPU | 8 vCPU |
| Memory | 8 GB | 16 GB |
| Disk | 250 GB free on top of the OS | 250 GB free on top of the OS, plus the Historian's disk for your retention |
| Example cloud instance | AWS `c8i.xlarge`, Azure `D4ls_v6` | AWS `c8i.2xlarge`, Azure `D8ls_v6` |
| Architecture | x86_64 or ARM64, [not Raspberry Pi](#supported-architectures) | x86_64 or ARM64, [not Raspberry Pi](#supported-architectures) |

This is a starting point for about 15 running [flows](../usage/data-flows/README.md). Actual usage depends on the applications you build on UMH Core (for example the number of flows, the message rate, the protocol and the number of tags) and on how fast your CPUs are. If the [Historian](../usage/historian/README.md) runs on the same machine, use the second column.

The CPU and memory figures on this page count running flows, not bridges:

- a [bridge](../usage/data-flows/bridges.md) counts as 2 running flows, 1 for its read flow and 1 for its write flow
- a [stand-alone flow](../usage/data-flows/stand-alone-flow.md) counts as 1
- a [stream processor](../usage/data-flows/stream-processor.md) counts as 1

The CPU figures assume vCPUs as fast as those of the example instances, not overcommitted, with no other software on the machine. See [Overcommitting, slow CPUs and other software](#overcommitting-slow-cpus-and-other-software).

## CPU

UMH Core needs 1 vCPU for the [Unified Namespace](../usage/unified-namespace/README.md) (Redpanda) and its own services. Each running flow adds 0.2 vCPU for monitoring overhead, plus the flow's own work.

A flow's own work depends on what it does. An OPC UA bridge polling 10,000 tags needs more CPU than an MQTT bridge with 100 tags. A flow that writes many log lines also adds more overhead than one that writes few, because UMH Core analyses every log line. Above about 10,000 messages per second, plan more vCPU and [contact us](https://discord.gg/F9mqkZnm8U).

Part of the 0.2 vCPU monitoring overhead goes to reading each flow's metrics. By default, a separate monitoring service does this for each flow. Adding `-e USE_FSMV2_BENTHOS_METRICS=true` to the `docker run` command moves this work into the UMH Core process, where it costs less CPU. Remove and recreate the container for the change to apply, because `docker restart` keeps the old environment. Still plan 0.2 vCPU per flow: it is a conservative budget that also covers the log analysis, which this variable does not change.

On the example instances, 1 vCPU is one hyperthread. Where 1 vCPU is a full physical core, as on AWS Graviton, the same vCPU count gives at least as much. UMH Core counts each vCPU as one core.

### Overcommitting, slow CPUs and other software

All CPU figures in these docs assume vCPUs as fast as those of the [example instances](#recommended-machine), not overcommitted, with no other software on the machine. Each of these reduces what UMH Core can do:

- **Overcommitted virtual machines.** Set a CPU reservation for UMH Core's VM. If [CPU Health](../usage/instances/container/how-cpu-health-is-judged.md#thresholds) shows Steal above 10% (status "CPU taken by the server"), the host is giving the VM's cores to other VMs. VMware shows the same on the host as CPU Ready. Even an idle-looking VM can wait for its cores, see [CPU contention while cores are free](../usage/instances/troubleshooting/cpu-contention-while-cores-are-free.md).
- **Older or slower CPUs.** Each flow takes a larger share of an older or slower core.
- **Other software on the machine.** Antivirus, compliance agents and other containers use CPU, memory and disk on top of UMH Core. Exclude the host directory behind the `/data` volume from real-time antivirus scanning.

### Redpanda and `--overprovisioned`

UMH Core runs Redpanda with the `--overprovisioned` flag, which turns off busy-polling. Redpanda then uses close to no CPU while no messages arrive, and adds up to a few milliseconds of latency.

## Memory

UMH Core needs 3.5 GB for the Unified Namespace and 150 MB for the agent. Each running flow adds about 100 MB.

## Disk

Most of the disk holds the [Unified Namespace](../usage/unified-namespace/README.md) buffer. By default it keeps seven days of messages for replay, and you can change that. At seven days' retention, plan about 0.07–0.09 GB per message per second.

```text
disk = (messages per second × bytes per message × retention in seconds + what the partition already holds) ÷ 0.75
```

Seven days are 604,800 seconds. "What the partition already holds" is the operating system and any other files on the same partition. The 0.75 keeps the disk within the [headroom](#headroom-and-resizing). Bytes per message is an estimate of about 100–130 bytes. Example: 3,000 messages per second × 115 bytes × 604,800 seconds ÷ 0.75 ≈ 280 GB, plus what the partition already holds ÷ 0.75.

To get your own figure, run this twice, 24 hours apart, while the buffer is still younger than the retention, for example in the first days after install. Once UMH Core deletes messages older than the retention, the growth no longer shows what it writes. Multiply the growth by the retention in days, add what the partition already holds and divide by 0.75:

```bash
docker exec umh-core du -sh /data/redpanda
```

With a default Docker volume, `/data` sits on the operating system's partition, so the operating system counts toward the disk use too.

If you run the Historian, add its disk on top. See [Historian Sizing](../usage/historian/sizing.md#disk-per-year).

To use less disk:

- Set retention during install with `internal.redpanda.redpandaServiceConfig.topic.defaultTopicRetentionMs`, or later per topic with `rpk`.
- Cap the topic with `defaultTopicRetentionBytes`. Under heavy load a cap can cut the replay window below seven days.
- Drop unchanged values with the [downsampler](https://docs.umh.app/benthos-umh/processing/downsampler) before they reach the Unified Namespace.

## Headroom and resizing

Size the machine so that, under normal load, memory stays below 70% and disk below 75%, as shown on the [Container card](../usage/instances/container/README.md#when-the-container-shows-degraded), and [CPU Health](../usage/instances/container/how-cpu-health-is-judged.md) stays healthy.

Grow the machine when the Container card shows CPU, memory or disk as degraded, or when new bridges wait at the [bridge limit](../usage/data-flows/bridges-do-not-start.md#bridge-limit). For CPU, [CPU Health](../usage/instances/container/how-cpu-health-is-judged.md) explains each status and what to do.

UMH Core keeps all its state in the `/data` volume. To grow the machine:

1. Stop the container.
2. Move or resize the volume, or attach it to a bigger VM.
3. Start the same image. No reinstall or reconfiguration is needed.

For failover across several nodes, see [High Availability](./high-availability.md).

## Supported architectures

| Architecture | Status | Notes |
|--------------|--------|-------|
| x86_64 | ✅ Fully supported | Intel/AMD 64-bit processors |
| ARM64 | ✅ Fully supported | 64-bit ARM processors (AWS Graviton, Apple Silicon, etc.) |
| Raspberry Pi | ❌ Not supported | Redpanda requires 48-bit virtual address space; Raspberry Pi provides only 38-bit |

Redpanda's Seastar framework needs a 48-bit virtual address space. Raspberry Pi's ARM processors provide 38 bits, so Redpanda stops with SIGABRT on startup. See [Redpanda GitHub Issue #1542](https://github.com/redpanda-data/redpanda/issues/1542).
