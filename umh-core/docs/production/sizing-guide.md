# Sizing Guide

This page says what machine to set up for UMH Core and how its needs grow.

## Recommended machine

| | UMH Core | UMH Core + [Historian](../usage/historian/sizing.md) on the same machine |
| --- | --- | --- |
| CPU | 4 cores (host CPUs, or vCPUs reserved on a VM) | 8 cores (host CPUs, or vCPUs reserved on a VM) |
| RAM | 8 GB | 16 GB |
| Disk | 250 GB free on top of the OS | 250 GB free on top of the OS, plus the Historian's disk for your retention |
| Architecture | x86_64 or ARM64, [not Raspberry Pi](#supported-architectures) | x86_64 or ARM64, [not Raspberry Pi](#supported-architectures) |

This is a starting point for about 15 running [flows](../usage/data-flows/README.md). Actual usage depends on the applications you build on UMH Core (for example the number of flows, the message rate, the protocol and the number of tags) and on how fast your CPUs are. If the Historian runs on the same machine, use the second column.

## Scaling

| Resource | Base | Grows by |
| --- | --- | --- |
| CPU | 1 core for the [Unified Namespace](../usage/unified-namespace/README.md) (Redpanda) and UMH Core's own services | 0.2 cores per running flow for monitoring overhead, plus the flow's own work |
| Memory | 3.5 GB for the Unified Namespace, 150 MB for the agent | ≈ 100 MB per running flow |
| Disk | None | 0.06–0.08 GB per message per second, see [Disk](#disk) |

A bridge counts as 1 running flow, or 2 when it also writes to the device. A stand-alone flow counts as 1, and so does a stream processor.

A flow's own work depends on what it does. An OPC UA bridge polling 10,000 tags needs more CPU than an MQTT bridge with 100 tags, and a flow that writes many log lines needs more than one that writes few. The Unified Namespace's own cost per message is negligible up to a few thousand messages per second. Above about 10,000 messages per second, plan more cores and [contact us](https://discord.gg/F9mqkZnm8U).

## Disk

Most of the disk holds the [Unified Namespace](../usage/unified-namespace/README.md) buffer. By default it keeps seven days of messages for replay, and you can change that. With the default, 250 GB covers roughly 3,000 messages per second. Payload size drives the real figure, so measure it on your instance.

The Historian's database needs disk on top of this. [Historian Sizing](../usage/historian/sizing.md#disk-per-year) lists how much per year.

The instance reports disk as degraded at 85% use of the partition that holds `/data`. With a default Docker volume, that is the operating system's partition, so the operating system counts toward the 85%.

To use less disk, shorten retention or cap the topic size:

- Set retention during install with `internal.redpanda.redpandaServiceConfig.topic.defaultTopicRetentionMs`, or later per topic with `rpk`.
- Cap the topic with `defaultTopicRetentionBytes`. Under heavy load a cap can cut the replay window below seven days.

## What reduces capacity

The figures above assume UMH Core has the whole machine to itself and runs on a recent CPU. Three things reduce what it can do:

- **Other software on the machine.** Antivirus, compliance agents and other containers use CPU, memory and disk on top of UMH Core.
- **Overcommitted virtual machines.** Reserve the vCPUs for UMH Core's VM. UMH Core works in short bursts, so its work waits whenever the host lends its core to another VM, even though the VM looks idle. [CPU contention while cores are free](../usage/instances/container/cpu-health/cpu-contention-while-cores-are-free.md) shows how to spot this.
- **Older or slower CPUs.** A slower core does less work, so each flow takes a larger share of it.

## Bridge limit

An instance starts at most `(CPU cores − 1) × 5` bridges: 15 on 4 cores. The one core it subtracts stays with Redpanda. More cores raise the limit.

The limit is a safety cap. It counts bridges, not workload: a bridge reading 10 tags counts the same as one reading 10,000. Stopped bridges and waiting bridges listed earlier in `config.yaml` count too. Stand-alone flows and stream processors do not. Size the machine by running flows instead.

A new bridge also waits while a resource is degraded or its health is not proven yet. [Bridges Do Not Start](../usage/data-flows/bridges-do-not-start.md) explains each reason and the emergency setting that starts bridges anyway.

## Resizing

Grow the machine when the instance shows a degraded resource in the Management Console or reaches its bridge limit. For CPU, [CPU Health](../usage/instances/container/cpu-health/README.md) explains each status and what to do.

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

## Redpanda and `--overprovisioned`

UMH Core runs Redpanda with the `--overprovisioned` flag, which turns off busy-polling, so Redpanda uses close to no CPU while no messages arrive. The cost is latency of up to low milliseconds, which manufacturing data tolerates. UMH Core needs the flag because CPU pinning does not work reliably in Docker and Redpanda shares the container with other processes.
