# Sizing Guide

This page says what machine to set up for UMH Core and how its needs grow.

## Recommended machine

| | UMH Core | UMH Core + [Historian](../usage/historian/sizing.md) on the same machine |
| --- | --- | --- |
| CPU | 4 cores (host CPUs, or vCPUs reserved on a VM) | 8 cores (host CPUs, or vCPUs reserved on a VM) |
| RAM | 8 GB | 16 GB |
| Disk | 250 GB free on top of the OS | 250 GB free on top of the OS, plus the Historian's disk for your retention |
| Architecture | x86_64 or ARM64, [not Raspberry Pi](#supported-architectures) | x86_64 or ARM64, [not Raspberry Pi](#supported-architectures) |

This is a starting point for about 15 flows. Actual usage depends on the applications you build on UMH Core: what your flows do (message rate, protocol, number of tags) and how fast your CPUs are. For more flows, add CPU and memory as shown in [Scaling](#scaling). Other software on the machine, such as antivirus, compliance agents and other containers, needs resources on top.

## Scaling

| Resource | Base | Grows by |
| --- | --- | --- |
| CPU | 1 core for Redpanda and UMH Core's own services | 0.2 cores per running flow |
| Memory | 3.5 GB for Redpanda, 150 MB for the agent | ≈ 100 MB per running flow |
| Disk | None | 0.06–0.08 GB per message per second, with Redpanda keeping 7 days |

### CPU

A bridge counts as 1 running flow, or 2 when it also writes to the device. A stand-alone flow counts as 1, and so does a stream processor.

The 0.2 cores pay for UMH Core watching the flow: reading its logs and metrics, checking its health and restarting it. A flow that logs heavily raises this.

Your flows' own work comes on top. An OPC UA bridge polling 10,000 tags needs more CPU than an MQTT bridge with 100 tags.

Redpanda's cost is negligible up to a few thousand messages per second. Above about 10,000 messages per second, plan more cores and [contact us](https://discord.gg/F9mqkZnm8U).

### Memory

At 15 running flows this comes to about 5.2 GB, so 8 GB leaves room.

### Disk

Most of the disk holds Redpanda's buffer, which keeps seven days of messages for replay. With that default, 250 GB covers roughly 3,000 messages per second. Payload size drives the real figure, so measure it on your instance.

The instance reports disk as degraded at 85% use of the partition that holds `/data`. With a default Docker volume, that is the operating system's partition, so the operating system counts toward the 85%.

To use less disk, shorten retention or cap the topic size:

- Set retention during install with `internal.redpanda.redpandaServiceConfig.topic.defaultTopicRetentionMs`, or later per topic with `rpk`.
- Cap the topic with `defaultTopicRetentionBytes`. Under heavy load a cap can cut the replay window below seven days.

## Virtual machines

Reserve the vCPUs for UMH Core's VM and do not overcommit them. UMH Core works in short bursts, so its work waits whenever the host lends its core to another VM, even though the VM looks idle. [CPU contention while cores are free](../usage/instances/container/cpu-health/cpu-contention-while-cores-are-free.md) shows how to spot this.

## Resizing

Check the machine's size when the instance shows a degraded state in the Management Console. For CPU, [CPU Health](../usage/instances/container/cpu-health/README.md) explains each status and what to do.

UMH Core keeps all its state in the `/data` volume. To grow the machine:

1. Stop the container.
2. Move or resize the volume, or attach it to a bigger VM.
3. Start the same image. No reinstall or reconfiguration is needed.

For failover across several nodes, see [High Availability](./high-availability.md).

## Bridge limit

An instance starts at most `(CPU cores − 1) × 5` bridges: 15 on 4 cores. A new bridge also waits while a resource is degraded (CPU, memory at 80% or more, disk at 85% or more) or its health is not proven yet, for example right after a restart. Bridges that are already running are left alone.

The limit is a safety cap. It counts bridges, not workload: a bridge reading 10 tags counts the same as one reading 10,000. Stopped bridges and waiting bridges listed earlier in `config.yaml` count too. Stand-alone flows and stream processors do not. Size the machine by running flows instead.

The bridge's status reason starts with one of these lines, followed by how to start bridges anyway in an emergency:

- **Bridge limit**: `Cannot create bridge - limit exceeded (15 bridges maximum with 4.0 CPU cores, 1 core reserved for Redpanda)`
- **High CPU**: `CPU degraded: CPU contention`, or another CPU Health headline such as `CPU limited`
- **High Memory**: `Memory degraded: Memory utilization critical`
- **High Disk**: `Disk degraded: Disk utilization critical`
- **Health not proven yet**: `Resource health not proven yet`

In an emergency, the setting *Turn off bridge admission* starts bridges anyway. Set it in your `config.yaml`:

```yaml
agent:
  enableResourceLimitBlocking: false  # Start bridges even when a resource is degraded or not proven yet, or the bridge limit is reached
```

`true` is the default, also when the key is missing. While it is `false`, the instance shows as degraded. [Bridges Do Not Start](../usage/data-flows/bridges-do-not-start.md) covers every reason and the full procedure.

## Supported architectures

| Architecture | Status | Notes |
|--------------|--------|-------|
| x86_64 | ✅ Fully supported | Intel/AMD 64-bit processors |
| ARM64 | ✅ Fully supported | 64-bit ARM processors (AWS Graviton, Apple Silicon, etc.) |
| Raspberry Pi | ❌ Not supported | Redpanda requires 48-bit virtual address space; Raspberry Pi provides only 38-bit |

Redpanda's Seastar framework needs a 48-bit virtual address space. Raspberry Pi's ARM processors provide 38 bits, so Redpanda stops with SIGABRT on startup. See [Redpanda GitHub Issue #1542](https://github.com/redpanda-data/redpanda/issues/1542).

## Redpanda and `--overprovisioned`

UMH Core runs Redpanda with the `--overprovisioned` flag, which turns off busy-polling, so Redpanda uses close to no CPU while no messages arrive. The cost is latency of up to low milliseconds, which manufacturing data tolerates. UMH Core needs the flag because CPU pinning does not work reliably in Docker and Redpanda shares the container with other processes.
