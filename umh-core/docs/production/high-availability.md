# High Availability

UMH Core follows best practices for high availability and disaster recovery, making it Kubernetes-native.
This means that it integrates well with any docker container manager, including
- Docker Engine (single-node)
- systemd (single-node)
- Kubernetes (multi-node)
- Docker Swarm (multi-node)

Here, UMH Core takes care or managing and restart processes inside of the container,
whereas the manager/orchestrator is responsible for managing the lifecycle of the container itself.
This ensures a separation of concerns between the container and the container manager/orchestrator.
An overview of typical recovery times is shown in the following table:

**Typical recovery times** (actual times depend on cluster configuration and workload):

| Failure | Recovery |
|---------|----------|
| Process crash inside container | Sub-second (S6 supervisor restarts process) |
| Pod crash | 30–60 seconds (depends on pod restart policy and resource availability) |
| Node failure | 1–2 minutes (depends on node failure detection timeout and storage reattachment) |

## Why Kubernetes-Native?

Some industrial software implements custom heartbeat-based failover. This made sense before container orchestration existed. Today, it duplicates what your cluster already does and introduces split-brain risk when the heartbeat network partitions but both nodes remain up.

UMH Core trusts the manager/orchestrator as the single source of truth for scheduling.
This follows the cloud native approach as well as the unix design philosophy, in which one tool does one thing well.

# Requirements

Under the hood, UMH Core uses [S6](https://skarnet.org/software/s6/) to manage processes inside the container. S6 is a lightweight process supervisor that provides robust process management and restart capabilities.
S6 stores its entire state based on files, which reside in the `/data` volume.
Therefore, the availability of UMH Core is governed by the availability of the `/data` volume.

Ensuring high availability for this volume is crucial but can be handled very well by common infrastructure practices, being it raid storage for single-node deployments or distributed storage for multi-node deployments. Here, infrastructure teams can leverage their preferred storage solutions to ensure high availability.

## Single-Node Deployments

If you have one node, storage choice does not affect failover. The node itself is the single point of failure. Redundancy comes from your underlying infrastructure (for example, a VM on vSAN survives host failure).

## Multi-Node Storage Requirement

For multi-node failover, you need storage accessible from multiple nodes.

The default k3s storage (`local-path`) binds the volume to one node. If that node dies, Kubernetes cannot reschedule the pod elsewhere because the data is stuck on the failed node.

Storage that works: vSAN, SAN, EBS, Azure Disk, GCE PD, Longhorn, Rook-Ceph. All of these allow Kubernetes to mount the volume on any node.

**Performance note:** Software-defined storage like Longhorn adds latency due to synchronous replication. For most UMH Core workloads, this is acceptable. For high-throughput scenarios, benchmark your specific workload before committing.


### Longhorn on k3s

If you do not have enterprise storage, Longhorn is a good choice for k3s clusters. See the [Longhorn Quick Installation Guide](https://longhorn.io/docs/latest/deploy/install/) and [K3s-specific configuration](https://longhorn.io/docs/latest/advanced-resources/os-distro-specific/csi-on-k3s/).


## Zero-Downtime Requirements

The standard approach accepts 30–60 second recovery on pod failure. If your process cannot tolerate any interruption, contact UMH to discuss active-active architectures.

Note: OPC UA sessions must be re-established after any failover. This is a protocol limitation.
