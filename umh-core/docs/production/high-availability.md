# High Availability

UMH Core handles high availability without custom failover daemons. Inside the container, [S6](https://skarnet.org/software/s6/) supervises all processes and restarts them on failure (see [Container Layout](../reference/container-layout.md) for details). Outside the container, your container manager (Docker, Kubernetes, Docker Swarm, Portainer, or any other) handles container restarts and rescheduling.

**Typical recovery times** (actual times depend on configuration and workload):

| Failure | Recovery |
|---------|----------|
| Process crash inside container | Sub-second (S6 restarts the process) |
| Container/pod crash | 30-60 seconds (depends on restart policy and resource availability) |
| Node failure | 2-5 minutes (depends on failure detection timeout and storage reattachment) |

## How Failover Works

All state lives in the `/data` volume (see [Container Layout](../reference/container-layout.md) for structure). When a container or node fails, your container manager starts a new container that mounts the same volume. Redpanda recovers from its on-disk data and resumes processing.

## Why Not Custom Failover?

Some industrial software implements custom heartbeat-based failover. This approach predates container orchestration. Today, it duplicates what your container manager already does. It also introduces split-brain risk: the heartbeat network can partition while both nodes remain up, causing both to assume primary ownership.

UMH Core relies on your container manager as the single source of truth for scheduling.

## Storage Requirements

### Single-Node Deployments

If you have one node, storage choice does not affect failover. The node itself is the single point of failure. Redundancy comes from your underlying infrastructure. For example, a VM on vSAN survives host failure, and RAID storage survives disk failure.

### Multi-Node Deployments

For multi-node failover, you need storage accessible from multiple nodes.

The default k3s storage (`local-path`) binds the volume to one node. If that node dies, the container manager cannot reschedule the container elsewhere because the data is stuck on the failed node.

**Compatible storage options** (all support mounting volumes on any node):

- Enterprise SAN or vSAN
- Cloud block storage: EBS, Azure Disk, GCE PD
- Software-defined: Longhorn, Rook-Ceph

**Performance note:** Software-defined storage like Longhorn adds latency due to synchronous replication. For most UMH Core workloads, this is acceptable. For high-throughput scenarios, benchmark your specific workload before committing.

### Longhorn on k3s

If you do not have enterprise storage, Longhorn is a good choice for k3s clusters. See the [Longhorn Quick Installation Guide](https://longhorn.io/docs/latest/deploy/install/) and [K3s-specific configuration](https://longhorn.io/docs/latest/advanced-resources/os-distro-specific/csi-on-k3s/).

## If You Need Zero Downtime

The standard approach accepts 30-60 second recovery on container failure. If your process cannot tolerate any interruption, contact UMH to discuss active-active architectures.

## OPC UA Sessions After Failover

OPC UA sessions must be re-established after any failover. UMH Core reconnects to OPC UA servers automatically, but re-establishment takes a few seconds during which no data is collected from OPC UA sources. The OPC UA protocol requires this.
