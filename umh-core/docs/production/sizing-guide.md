# Sizing Guide

This page tells you what machine to provision for UMH Core, what that machine covers, and when to make it larger. It covers UMH Core on its own. If the Historian runs on the same machine, add its needs from [Historian Sizing](../usage/historian/sizing.md).

## Recommended machine

**4 vCPU · 8 GB RAM · 250 GB of free disk, on top of the operating system.**

On a virtual machine, the 4 vCPUs must be reserved for it. See [Virtual machines](#virtual-machines-and-cpu-reservation).

## Supported architectures

UMH Core runs on 64-bit x86 and 64-bit ARM. It does not run on a Raspberry Pi.

| Architecture | Status | Notes |
|--------------|--------|-------|
| x86_64 | ✅ Fully supported | Intel/AMD 64-bit processors |
| ARM64 | ✅ Fully supported | 64-bit ARM processors (AWS Graviton, Apple Silicon, etc.) |
| Raspberry Pi | ❌ Not supported | Redpanda requires 48-bit virtual address space; Raspberry Pi provides only 38-bit |

Redpanda uses the Seastar framework, which requires a 48-bit virtual address space. Raspberry Pi's ARM processors provide only 38 bits, so Redpanda stops with SIGABRT on startup. This is a hardware limit, documented in [Redpanda GitHub Issue #1542](https://github.com/redpanda-data/redpanda/issues/1542).

## What 4 vCPU covers

UMH Core needs CPU for each flow it runs. Sizing counts **running flows**:

- A bridge counts as 1 flow, or 2 when it also writes to the device: one read flow and one write flow.
- A stand-alone flow counts as 1.
- A stream processor counts as 1.

UMH Core plans for **0.2 cores per running flow, plus 1 core** for Redpanda and UMH Core's own services. On 4 vCPU that leaves 3 cores, enough for **15 running flows**. This figure is deliberately conservative: measured usage per flow has been well below 0.2 cores.

### What the per-flow overhead includes

The 0.2 cores pay for the work UMH Core does to keep each flow running:

- reading and parsing every flow's logs and metrics
- deciding whether the flow is healthy, and restarting it when it is not
- applying rules to what it reads. For example, while you edit a flow, UMH Core detects a configuration error in its log lines and rolls the edit back.

Every log line is scanned on every cycle, so a flow that logs heavily raises this overhead.

### Your workload comes on top

The overhead does not include the work your flows do. Your message rate, your processing and your flows' log volume are your workload, and they need CPU on top of the 15-flow figure. Bridges differ a lot. An OPC UA bridge polling 10,000 tags needs more CPU than an MQTT bridge with 100 tags.

Redpanda's cost per message is small. Up to a few thousand messages per second it is negligible; published benchmarks put 1,000 messages per second at about 1–5% of a core. Above about 10,000 messages per second, plan more cores and [contact us](https://discord.gg/F9mqkZnm8U). Redpanda's cost follows the number of produce requests, so many flows that each send little also add to it.

## Memory

UMH Core's memory use grows with the number of running flows. 8 GB leaves room above the 15-flow figure.

| Component | Rule of thumb |
| --- | --- |
| Redpanda | 2 GB, plus 1.5 GB headroom: 3.5 GB |
| UMH Core agent | ≈ 150 MB |
| Each running flow | ≈ 100 MB |

At 15 running flows this comes to about 5.2 GB. These figures come from arithmetic and a lab measurement, not from instances in the field.

## Disk

Plan 250 GB of free space on top of the operating system. The instance reports disk as degraded at 85% use of the partition that holds `/data`. When `/data` sits on the operating system's partition, as it does with a default Docker volume, the operating system counts toward that 85%.

Most of the disk holds Redpanda's buffer. A new instance keeps seven days of messages, so you can replay them. Seven days cost about **0.06–0.08 GB per message per second**. 250 GB therefore covers roughly 2,600–3,500 messages per second before the disk reaches 85%. Payload size drives the real figure, so measure it on your instance.

To use less disk, shorten retention, cap the topic size, or enlarge the disk:

- Set retention during install with `internal.redpanda.redpandaServiceConfig.topic.defaultTopicRetentionMs`, or later per topic with `rpk`.
- Cap the topic with `defaultTopicRetentionBytes`. A byte cap protects the disk, but under heavy load it can cut the replay window below seven days.

## Other software on the same machine

The numbers on this page cover UMH Core only. Antivirus, compliance agents, other Docker containers and anything else on the machine need their own CPU, memory and disk. Add their requirements to UMH Core's.

## Virtual machines and CPU reservation

On a virtual machine, reserve the vCPUs for UMH Core's VM. Do not share them with other VMs through CPU overcommit.

UMH Core works in short bursts and needs its cores at that moment. When the host gives a shared core to another VM, UMH Core's work waits even though the VM looks idle. [CPU contention while cores are free](../usage/instances/container/cpu-health/cpu-contention-while-cores-are-free.md) explains the effect and how to read it.

## When to resize

Review the machine's size when the instance shows a degraded state in the Management Console. For CPU, [CPU Health](../usage/instances/container/cpu-health/README.md) explains each status and what to do. If you are unsure what to change, [contact us](https://discord.gg/F9mqkZnm8U).

### Easy vertical scaling

UMH Core keeps all its state in the **`/data`** volume. To grow:

1. Stop the container
2. Move or resize the volume, or attach it to a bigger VM
3. Start the same image. No reinstall or reconfiguration is needed.

### High availability

For multi-node failover, mount `/data` on storage that several nodes can reach. See [High Availability](./high-availability.md).

## When UMH Core refuses to start a bridge

Before UMH Core starts a new bridge, it runs bridge admission. A new bridge waits while one of these is true:

- **The instance has reached its bridge limit.** The limit is `(CPU cores − 1) × 5`: 15 bridges on 4 cores.
- **A resource is degraded.** This covers CPU as reported by [CPU Health](../usage/instances/container/cpu-health/README.md), including a container throttled at its CPU limit, memory at 80% or more, and disk at 85% or more.
- **Resource health is not proven yet**, for example right after a restart.

Bridges that are already running are left alone. [Bridges Do Not Start](../usage/data-flows/bridges-do-not-start.md) covers every reason and what to do.

### What the bridge limit measures

The bridge limit is a safety cap. It does not measure your workload: it allows the same number of bridges whether each one reads 10 tags or 10,000. It counts bridges only, including stopped bridges and waiting bridges listed earlier in `config.yaml`. A bridge with writes counts once, and stand-alone flows and stream processors are not counted. An instance can be under its bridge limit and still run more than 15 flows. Size the machine by running flows and your workload, not by the limit.

### Resource limit error messages

The bridge's status reason starts with one of these lines, followed by how to start bridges anyway in an emergency:

- **Bridge limit**: `Cannot create bridge - limit exceeded (15 bridges maximum with 4.0 CPU cores, 1 core reserved for Redpanda)`
- **High CPU**: `CPU degraded: CPU contention`, or another CPU Health headline such as `CPU limited`
- **High Memory**: `Memory degraded: Memory utilization critical`
- **High Disk**: `Disk degraded: Disk utilization critical`
- **Health not proven yet**: `Resource health not proven yet`

### Turn off bridge admission

In an emergency, the setting *Turn off bridge admission* starts bridges anyway. Set it in your `config.yaml`:

```yaml
agent:
  enableResourceLimitBlocking: false  # Start bridges even when a resource is degraded or not proven yet, or the bridge limit is reached
```

`true` is the default, also when `config.yaml` does not contain the key. While it is `false`, the instance shows as degraded. See [Bridges Do Not Start](../usage/data-flows/bridges-do-not-start.md) for the full procedure.

## Redpanda and the `--overprovisioned` flag

UMH Core runs Redpanda with the `--overprovisioned` flag. The flag turns off Seastar's busy-polling, so Redpanda uses close to no CPU while no messages arrive, instead of 100% of its core. The cost is slightly higher latency, from microseconds to low milliseconds, which manufacturing data tolerates. UMH Core needs the flag because it runs in Docker, where CPU pinning does not work reliably, and Redpanda shares the container with other processes.
