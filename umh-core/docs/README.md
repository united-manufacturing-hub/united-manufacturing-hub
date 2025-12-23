# Introduction

> **Looking for the old Kubernetes Helm stack?** See the [UMH Core vs UMH Classic FAQ](umh-core-vs-classic-faq.md) to understand which edition fits your project and the current migration path.

## What is UMH Core?

UMH Core is a **single Docker container that turns any PC, VM, or edge gateway into an Industrial Data Hub**.

In manufacturing, every device speaks a different language - PLCs use OPC UA, sensors use Modbus, MES systems use REST APIs. Connecting them all creates a tangled mess of point-to-point integrations. Change one thing, break ten others.

UMH Core creates a **Unified Namespace (UNS)** - a central data backbone where all your industrial data lives in one organized, validated place. Instead of 100 devices talking to each other (creating 1000s of connections), they all publish to one place, and consumers subscribe to what they need.

### What's Inside

That one Docker container includes everything you need:

* **Redpanda** – an embedded, Kafka-compatible broker that buffers every message.
* **Benthos-UMH** – a stream-processor engine with 50+ industrial connectors.
* **Agent** – a Go service that reads `config.yaml`, launches pipelines, watches health, and phones home to the Management Console.
* **S6 Supervisor** – keeps every subprocess alive and starts them in the correct order.

### Why teams pick UMH Core

| Benefit         | What it means in practice                                                                                               |
| --------------- | ----------------------------------------------------------------------------------------------------------------------- |
| **Simple**      | PLCs, sensors, ERP/MES, cloud services all talk to **one Unified Namespace (UNS)** instead of point-to-point spaghetti. |
| **Lightweight** | Runs on almost everything                                                                                               |
| **No lock-in**  | 100 % open-source stack: Redpanda, Benthos, S6, and much more                                                           |

### Core Concepts You'll Learn

Through our getting-started guide, you'll understand:

* **Instance** – A running UMH Core container identified by its location path
* **Management Console** – Cloud UI for deploying and managing instances without touching YAML
* **Unified Namespace (UNS)** – The event-driven data backbone that eliminates point-to-point connections
* **Bridge** – The gateway for external data into the UNS (**the ONLY way data enters**)
* **Topic** – How data is addressed: `location.contract.virtual_path.tag_name`
* **Tag** – A time-series data point (like a PLC variable or sensor reading)
* **Virtual Path** – Folder organization within topics for grouping related data
* **Data Model** – Templates that define and validate data structure
* **Data Contract** – Validation rules (`_raw` = no validation, `_modelname_v1` = enforced structure)

Advanced concepts (after getting-started):
* **Stream Processor** – Transforms messages already inside the UNS (e.g., device models → business models)
* **Stand-alone Flow** – Point-to-point when UNS buffering isn't wanted
* **State Machines** – Component lifecycle management (active/idle/degraded states)

### How It Works

```text
Your Factory Floor                    UMH Core                         Your Systems
──────────────────                    ────────                         ────────────
                                                        
PLCs (S7, Modbus)    ─┐                                          ┌─▶ Dashboards
Sensors (OPC UA)     ─┼─[Bridge]─▶ Unified Namespace ─[Bridge]───┼─▶ Cloud/MQTT
MES/ERP (REST)       ─┘              (organized data)            └─▶ Databases
```

1. **Bridges** connect your devices to the UNS (50+ protocols supported) - they're the ONLY entry point
2. **Data flows** into organized topics: `enterprise.site.area.line._contract.virtual_path.tag`
3. **Models** validate critical data (optional but recommended for production)
4. **Consumers** subscribe to the data they need
5. **Configure** via Management Console UI or directly edit YAML files

_Every message is buffered, validated, and organized - no data loss, guaranteed structure._

## Getting Started

Sign up at [management.umh.app](https://management.umh.app) and deploy your first instance in 60 seconds through the UI.

**Prefer step-by-step learning?** Follow our progressive 4-step guide:

1. [**Install UMH Core**](getting-started/README.md) - One Docker command (5 minutes)
2. [**Connect Your First Data**](getting-started/1-connect-data.md) - Create a Bridge and see data flow (10 minutes)
3. [**Organize Your Data**](getting-started/2-organize-data.md) - Scale from 1 to 1000s of tags automatically (15 minutes)
4. [**Validate Your Data**](getting-started/3-validate-data.md) - Add quality control with Data Models (20 minutes)

By the end, you'll have production-ready data pipelines with validation, organization, and monitoring.

## Documentation Structure

* [**Usage Guides**](usage/) - Step-by-step implementation guides
  * [**Instances**](usage/instances/) - Managing and configuring UMH Core deployments
  * [**Unified Namespace**](usage/unified-namespace/) - Core messaging architecture
  * [**Data Flows**](usage/data-flows/) - Connect and process data streams
  * [**Data Modeling**](usage/data-modeling/) - Enterprise data structuring
  * [**Management Console**](usage/management-console/) - Cloud-based control center
* [**Production Deployment**](production/) - Scaling, security, and operations
* [**Reference Documentation**](reference/) - Complete API and configuration reference

## Learn More About UNS

For deeper understanding of the concepts behind UMH Core:

* [**The Unified Namespace Course Series**](https://learn.umh.app/featured/) - 4-chapter comprehensive course
  * [Chapter 1: OT Foundations](https://learn.umh.app/lesson/chapter-1-the-foundations-of-the-unified-namespace-in-operational-technology/) - Automation pyramid challenges
  * [Chapter 2: The Rise of UNS](https://learn.umh.app/lesson/chapter-2-the-rise-of-the-unified-namespace/) - Core architecture principles
  * [Chapter 3: IT Foundations](https://learn.umh.app/lesson/chapter-3-the-foundations-of-the-unified-namespace-in-information-technology/) - Modern IT patterns
* [**Industrial IoT Architecture**](https://learn.umh.app/blog/cloud-native-technologies-on-the-edge-in-manufacturing/) - Edge computing in manufacturing
* [**MQTT vs UNS Comparison**](https://learn.umh.app/blog/what-is-mqtt-why-most-mqtt-explanations-suck-and-our-attempt-to-fix-them/) - Why data contracts matter

## Community & Support

* [**Discord Community**](https://discord.gg/F9mqkZnm8U) - Get help and connect with other users
* [**GitHub Repository**](https://github.com/united-manufacturing-hub/united-manufacturing-hub) - Source code and issue tracking
* [**Management Console**](https://management.umh.app) - Web-based configuration and monitoring
* [**UMH Website**](https://www.umh.app/) - Company and product information
