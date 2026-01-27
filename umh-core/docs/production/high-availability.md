# High Availability

UMH Core uses Kubernetes-native high availability. Your cluster handles restart and rescheduling. No custom failover daemons required.

## How Failover Works

All state lives in the `/data` volume (see [Container Layout](../reference/container-layout.md) for structure). When a pod or node fails, Kubernetes schedules a new pod that mounts the same volume. Redpanda replays its write-ahead log and resumes processing.

**Expected recovery times:**

| Failure | Recovery |
|---------|----------|
| Process crash inside container | Sub-second (S6 supervisor restarts process) |
| Pod crash | 30–60 seconds |
| Node failure | 1–2 minutes |

## Storage Requirement

For multi-node failover, you need storage accessible from multiple nodes.

The default k3s storage (`local-path`) binds the volume to one node. If that node dies, Kubernetes cannot reschedule the pod elsewhere because the data is stuck on the failed node.

Storage that works: vSAN, SAN, EBS, Azure Disk, GCE PD, Longhorn, Rook-Ceph. All of these allow Kubernetes to mount the volume on any node.

**Performance note:** Software-defined storage like Longhorn provides roughly 20–30% of native disk IOPS due to synchronous replication. For most UMH Core workloads, this is acceptable. For high-throughput scenarios, test before committing.

### Single-Node Deployments

If you have one node, storage choice does not affect failover. The node itself is the single point of failure. Redundancy comes from your underlying infrastructure (for example, a VM on vSAN survives host failure).

### Longhorn on k3s

If you do not have enterprise storage, Longhorn is a good choice for k3s clusters. See the [Longhorn Quick Installation Guide](https://longhorn.io/docs/1.10.1/deploy/install/) and [K3s-specific configuration](https://longhorn.io/docs/1.10.1/advanced-resources/os-distro-specific/csi-on-k3s/).

## Why Kubernetes-Native?

Some industrial software implements custom heartbeat-based failover. This made sense before container orchestration existed. Today, it duplicates what your cluster already does and introduces split-brain risk when the heartbeat network partitions but both nodes remain up.

UMH Core trusts Kubernetes as the single source of truth for scheduling.

## Zero-Downtime Requirements

The standard approach accepts 30–60 second recovery on pod failure. If your process cannot tolerate any interruption, contact UMH to discuss active-active architectures.

Note: OPC UA sessions must be re-established after any failover. This is a protocol limitation.
