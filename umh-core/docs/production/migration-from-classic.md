# Migrating from UMH Classic to UMH Core

This guide walks through running Core side-by-side with Classic, exporting existing Benthos configs, and retiring Classic components. For a quick FAQ see [UMH Core vs UMH Classic](../umh-core-vs-classic-faq.md).

## Migration Strategy

The recommended approach is **side-by-side deployment** to minimize risk:

1. **Deploy Core next to Classic** - Point a Bridge at the Classic UNS topics
2. **Cut over producers/consumers** gradually
3. **Shut down Classic** pods once data is verified

## Exporting Benthos Configurations

Export your existing Benthos configs from Classic:

```bash
kubectl get configmap <benthos-config-name> -o yaml > classic-benthos-config.yaml
```

Extract the Benthos pipeline section and paste it into Core's `config.yaml → dataFlow:` section.

## What to Migrate

| Component | Classic Location | Core Location | Notes |
|-----------|------------------|---------------|--------|
| **Benthos pipelines** | Individual pods | `config.yaml → dataFlow` | Export configs from ConfigMaps |
| **Node-RED flows** | Node-RED pod | External container | Run separately, connect via MQTT/HTTP |
| **Grafana dashboards** | Bundled Grafana | External Grafana | Point at TimescaleDB |
| **TimescaleDB** | TimescaleDB pod | External database | Use Bridge to forward data |

*This guide will be expanded with detailed examples as more migration patterns emerge.* 