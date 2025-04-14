# United Manufacturing Hub

The **United Manufacturing Hub** (UMH) is an open-source solution for ingesting, contextualizing, and storing factory data. It helps you quickly build a _Unified Namespace_ for your production lines—laying the foundation for advanced analytics, real-time monitoring, and digital transformations.

This repository contains:
- **UMH Core** (in the `umh-core` folder) – our new, lightweight single-container approach.
- **Helm Chart** (in `deployment/united-manufacturing-hub`) – the classic Kubernetes installation method for a full-stack, scalable setup.

---

## UMH Core

**UMH Core** is a standalone Docker-based solution that bundles the essential services—such as Redpanda (Kafka-compatible broker), Benthos (for data flows), and management/orchestration—into a single container. It allows you to:

- **Deploy a Unified Namespace** on any machine running Docker (no Kubernetes required).
- **Orchestrate protocol converters and data flows** via Benthos-based pipelines.
- **Integrate with [management.umh.app](https://management.umh.app) for remote monitoring and management**, including real-time logs and system health.

### Quick Start

```bash
docker run -d \
  --name umh-core \
  -v umh_data:/data \
  ghcr.io/united-manufacturing-hub/umh-core:latest
```
- Connect to the cloud console:  
  1. Go to [management.umh.app](https://management.umh.app)  
  2. Follow the **Add Instance** steps to add a new UMH Core instance

For more details on using UMH Core, see [our website](https://www.umh.app) or sign in to [management.umh.app](https://management.umh.app).

---

## Helm Chart (UMH Classic)

The **Helm chart** in `deployment/united-manufacturing-hub` provides the original, full-stack “UMH Classic” deployment for Kubernetes. It includes:

- **TimescaleDB** for time-series historian functionality  
- **Node-RED** for flow-based data ingestion and quick device connections  
- **Grafana** dashboards, connectors, and other optional services  

> **Use the Helm chart if** you need a comprehensive, Kubernetes-based environment with built-in storage, visualization, and enterprise-scale orchestration.  

To install:
1. Go to [management.umh.app](https://management.umh.app)  
2. Follow the **Add Instance** steps to add a new UMH Classic instance

For more details, refer to the chart’s [README](deployment/united-manufacturing-hub/README.md) or visit our [documentation](https://umh.docs.umh.app/docs/).

---

## Unified Namespace Overview

A **Unified Namespace** centralizes all plant-floor data in one logical location. With UMH, you can:

- Publish real-time telemetry (e.g., from PLCs, sensors) in a standardized structure  
- Subscribe any consumer (dashboards, analytics tools, custom apps) to the same data  
- Easily unify or correlate data across machines and sites

To learn more about the Unified Namespace concept, see the ["The Rise of the Unified Namespace"](https://learn.umh.app/lesson/chapter-2-the-rise-of-the-unified-namespace/) article on our Learning Portal.

---

## Further Resources

- **Website:** [umh.app](https://www.umh.app)  
  Explore product overviews, enterprise offerings, and the UMH roadmap.
- **Management Console:** [management.umh.app](https://management.umh.app)  
  Connect UMH Core instances for cloud-based monitoring, configuration, and upgrades.
- **Docs (Legacy & Advanced Guides):** [umh.docs.umh.app](https://umh.docs.umh.app/docs/)  
  Deeper info on Helm installations, timeseries historian usage, advanced config, etc.
- **Featured Articles & Tutorials:** [learn.umh.app/featured](https://learn.umh.app/featured/)  

---

## Contributing

Pull requests, issues, and community involvement are welcome! See our [CONTRIBUTING.md](./CONTRIBUTING.md) and [CODE_OF_CONDUCT.md](./CODE_OF_CONDUCT.md) for guidelines.

---

## License

This project is licensed under the [Apache 2.0 License](./LICENSE). Please see [CONTRIBUTOR_LICENSE_AGREEMENT_INDIVIDUAL.md](./CONTRIBUTOR_LICENSE_AGREEMENT_INDIVIDUAL.md) and [CONTRIBUTOR_LICENSE_AGREEMENT_ENTITY.md](./CONTRIBUTOR_LICENSE_AGREEMENT_ENTITY.md) for contributor requirements.

---

*© 2025 United Manufacturing Hub. All rights reserved.*