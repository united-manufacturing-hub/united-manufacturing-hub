# Introduction

> **Looking for the old Kubernetes Helm stack?** See the [UMH Core vs UMH Classic FAQ](umh-core-vs-classic-faq.md) to understand which edition fits your project and the current migration path.

UMH-Core is a **single Docker container that turns any PC, VM, or edge gateway into an Industrial Data Hub**.

Inside that one image you'll find:

* **Redpanda** – an embedded, Kafka-compatible broker that buffers every message.
* **Benthos-UMH** – a stream-processor engine with 50+ industrial connectors.
* **Agent** – a Go service that reads `config.yaml`, launches pipelines, watches health, and phones home to the Management Console.
* **S6 Supervisor** – keeps every subprocess alive and starts them in the correct order.

### Why teams pick UMH-Core

| Benefit         | What it means in practice                                                                                               |
| --------------- | ----------------------------------------------------------------------------------------------------------------------- |
| **Simple**      | PLCs, sensors, ERP/MES, cloud services all talk to **one Unified Namespace (UNS)** instead of point-to-point spaghetti. |
| **Lightweight** | Runs on almost everything                                                                                               |
| **No lock-in**  | 100 % open-source stack: Redpanda, Benthos, S6, and much more                                                           |

### Key concepts at a glance

```
Instance
└─ Core
   ├─ Bridges 🚧         # ingest or egest data (ex-"Protocol Converters") - Roadmap Item
   │   ├─ Source Flow  # read side
   │   └─ Sink Flow    # write side  
   │   └─ Connection   # monitors the network connection
   ├─ Stream Processors 🚧  # Roadmap - transforms messages inside UNS
   └─ Stand-alone Flows ✅  # Available now
```

* **Bridge** 🚧 – a Data Flow that **connects** an external system to the UNS. *(Roadmap item - basic functionality available)*
* **Stream Processor** 🚧 – transforms messages already inside the UNS. *(Roadmap item)*
* **Stand-alone Flow** ✅ – point-to-point when UNS buffering isn't wanted. *(Available now)*
* **Connection** ✅ - a continuous network check whether the external system is available. *(Available now)*

### Typical architecture

```
┌──────── PLC / Device ───────┐
│  OPC UA / Modbus / S7 / …   │
└────────────┬────────────────┘
             │  Bridge (Source Flow)
             ▼
    ┌───────────────────┐
    │ Unified Namespace │
    └───────────────────┘
             │  Bridge (Sink Flow)
             ▼
   MQTT broker ▸ Cloud ▸ Historian ▸ Dashboards
```

_Every message first lands in the Unified Namespace, giving you replay, buffering, and reliable data processing. MQTT Brokers, Databases, Historians, Dashboards, etc. then consume the data from that._

## Getting Started

1. **[Quick Setup](getting-started.md)** - Get UMH Core running in minutes
2. **[Unified Namespace Guide](usage/unified-namespace/README.md)** - Understand the core messaging architecture
3. **[Connect Your First Device](usage/unified-namespace/producing-data.md)** 🚧 - Bridge industrial protocols to the UNS
4. **[Data Modeling](usage/data-modeling/README.md)** 🚧 - Structure your industrial data for enterprise-scale analytics

## Documentation Structure

- **[Usage Guides](usage/README.md)** - Step-by-step implementation guides
  - **[Unified Namespace](usage/unified-namespace/README.md)** - Core messaging architecture
  - **[Data Modeling](usage/data-modeling/README.md)** 🚧 - Enterprise data structuring
  - **[Data Flows](usage/data-flows/README.md)** - Connect and process data streams
- **[Production Deployment](production/README.md)** - Scaling, security, and operations
- **[Reference Documentation](reference/README.md)** - Complete API and configuration reference

## Learn More About UNS

For deeper understanding of the concepts behind UMH Core:

- **[The Unified Namespace Course Series](https://learn.umh.app/featured/)** - 4-chapter comprehensive course
  - [Chapter 1: OT Foundations](https://learn.umh.app/lesson/chapter-1-the-foundations-of-the-unified-namespace-in-operational-technology/) - Automation pyramid challenges
  - [Chapter 2: The Rise of UNS](https://learn.umh.app/lesson/chapter-2-the-rise-of-the-unified-namespace/) - Core architecture principles  
  - [Chapter 3: IT Foundations](https://learn.umh.app/lesson/chapter-3-the-foundations-of-the-unified-namespace-in-information-technology/) - Modern IT patterns
- **[Industrial IoT Architecture](https://learn.umh.app/blog/cloud-native-technologies-on-the-edge-in-manufacturing/)** - Edge computing in manufacturing
- **[MQTT vs UNS Comparison](https://learn.umh.app/blog/what-is-mqtt-why-most-mqtt-explanations-suck-and-our-attempt-to-fix-them/)** - Why data contracts matter

## Community & Support

- **[Discord Community](https://discord.gg/F9mqkZnm8U)** - Get help and connect with other users
- **[GitHub Repository](https://github.com/united-manufacturing-hub/umh-core)** - Source code and issue tracking
- **[Management Console](https://management.umh.app)** - Web-based configuration and monitoring
- **[UMH Website](https://www.umh.app/)** - Company and product information
