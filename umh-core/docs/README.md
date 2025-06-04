# Introduction

> **Looking for the old Kubernetes Helm stack?** See the [UMH Core vs UMH Classic FAQ](umh-core-vs-classic-faq.md) to understand which edition fits your project and the current migration path.

UMH-Core is a **single Docker container that turns any PC, VM, or edge gateway into an Industrial Data Hub**.

Inside that one image you'll find:

* **Redpanda** – an embedded, Kafka-compatible broker that buffers every message.
* **Benthos-UMH** – a stream-processor engine with 50 + industrial connectors.
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
   ├─ Bridges 🚧         # ingest or egest data (ex-"Protocol Converters") - Coming Soon
   │   ├─ Source Flow  # read side
   │   └─ Sink Flow    # write side  
   │   └─ Connection   # monitors the network connection
   ├─ Stream Processors 🚧  # Roadmap - transforms messages inside UNS
   └─ Stand-alone Flows ✅  # Available now
```

* **Bridge** 🚧 – a Data Flow that **connects** an external system to the UNS. *(Coming very soon)*
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
