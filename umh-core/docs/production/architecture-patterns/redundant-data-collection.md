# Redundant Data Collection

{% hint style="info" %}
This pattern applies to sites with **redundant PLCs** — two physical controllers publishing the same data. Redundant PLCs are common in process manufacturing (oil and gas, chemicals, power generation, pharmaceuticals) where the process cannot be safely stopped. If you run standard, non-redundant PLCs — as most discrete manufacturing sites do — this page does not apply to you. A single umh-core instance already provides sub-second process recovery and automatic container restart. See [High Availability](../high-availability.md).
{% endhint %}

## The problem

When a site has redundant PLCs, each controller independently reads the same sensors. If the umh-core instance collecting data goes down, data is lost until it recovers — even though a second PLC could have provided the same readings.

## The solution

Run two umh-core instances, each connected to one of the redundant PLCs. Both publish to the same [Unified Namespace](../../usage/unified-namespace/README.md). If one instance fails, the other continues collecting.

![Redundant data collection topology: two umh-core instances, each reading from one PLC, both publishing to the same Unified Namespace](./images/dual-umh-core-topology.png)

No deduplication is needed. Each PLC publishes its own tag set independently. Only one of the two PLCs is active at any moment (leader election is handled by the PLCs themselves). The surviving umh-core instance always has data available in the Unified Namespace.

## Prerequisites

- **Two redundant PLCs.** Two physical or logical OPC UA servers that publish the same data. A single PLC with two IP addresses does not qualify — you need hardware designed for redundancy.
- **High-availability deployment.** Both umh-core instances must be deployed so that a single infrastructure failure does not take both down at the same time. See [High Availability](../high-availability.md) for storage and deployment requirements.

## Recovery behavior

Two terms used in the table below:

- **MTTR** (Mean Time To Recovery): how long until data collection resumes after a failure.
- **RPO** (Recovery Point Objective): how much data is lost during the failure. RPO = 0 means no data is lost.

| Failure | MTTR | RPO | What happens |
|---------|------|-----|--------------|
| Process crash inside container | Sub-second | 0 | S6 restarts the process. The other instance was never affected. |
| Container or pod crash | 5–10 seconds | 0 | Container manager restarts the container. The other instance continues. |
| Full instance failure | ~2 minutes | 0 | Infrastructure reschedules the instance. The other instance continues. Data already in the Unified Namespace is preserved. |

RPO is zero across all failure types because the second instance is always collecting independently. Even during the ~2-minute rescheduling window, one instance is still publishing.

**OPC UA reconnect time is a protocol constraint, not a software one.** OPC UA leader election takes 3–4 seconds at minimum. No vendor can achieve sub-5-second recovery for OPC UA connections after a failover event.

## When this pattern does not apply

- **Your PLC is not redundant.** Connecting two bridges to a single PLC doubles the load on it without improving availability. Redundant collection requires two separate OPC UA endpoints publishing the same data.
- **You only need process-level or container-level recovery.** A standard single umh-core deployment already provides sub-second process recovery and 5–10 second container recovery. No additional topology is needed. See [High Availability](../high-availability.md).
- **You are in discrete manufacturing.** Automotive assembly lines, packaging lines, and machine tools typically use standard PLCs without redundancy. This pattern adds complexity without benefit in those environments.

## Related

- [High Availability](../high-availability.md) — infrastructure-level recovery (container restarts, storage, node rescheduling)
- [Architecture Patterns](README.md) — overview of all deployment patterns
