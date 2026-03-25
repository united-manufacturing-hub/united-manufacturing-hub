# Architecture Patterns

Architecture patterns describe how to deploy UMH Core for different operational requirements. Each page explains a topology: what components are involved, how they connect, what the pattern guarantees, and when to use it.

These are not step-by-step guides. They describe architectures so you can choose the right one for your site. For deployment procedures, see [Deployment](../deployment/README.md). For infrastructure-level recovery (container restarts, storage, node rescheduling), see [High Availability](../high-availability.md).

## Patterns

- [Redundant Data Collection](redundant-data-collection.md) — two umh-core instances reading from redundant PLCs for zero data loss
- Single Instance — the default deployment *(coming soon)*
- Historian Stack — umh-core with TimescaleDB, PgBouncer, and Grafana *(coming soon)*
- Multi-Instance Hierarchy — ISA-95 line-to-site-to-enterprise topology *(coming soon)*
