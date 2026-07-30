# Additional Services

Docker Compose lets you run more than umh-core. Each page in this section explains how to extend `services:` block of your `docker-compose.yaml`, with a complete configuration you can copy.

Most deployments want a database and dashboards, so start with the [Recommended UMH Stack](recommended-umh-stack.md), which combines umh-core, Grafana, PgBouncer, and TimescaleDB in one file.

To add services one at a time, see [TimescaleDB](timescaledb.md), [Grafana](grafana.md), or [nginx](nginx.md).
