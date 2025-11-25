# Optional Addons for UMH Core

This directory contains optional addons that extend UMH Core functionality.

---

## Available Addons

### [Historian](historian/)
**Local time-series storage with TimescaleDB + Grafana**

Adds persistent storage for sensor data and visualization dashboards.

- **Use when**: You need local data persistence and real-time dashboards
- **Includes**: TimescaleDB (time-series database), Grafana (visualization)

**Deploy:** See [main deployment README](../README.md#historian-addon-local-time-series-storage) for installation command.

---

## Adding Multiple Addons

To deploy multiple addons simultaneously:

```bash
# From docker-compose/ directory
docker compose -f docker-compose.yaml \
               -f examples/historian/docker-compose.historian.yaml \
               -f examples/future-addon/docker-compose.future.yaml up -d
```
