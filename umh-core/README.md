Below is a revised README that incorporates the new requirement that changing the desired state is done by updating a YAML file. This can happen either by a user through a CI/CD pipeline or by the agent’s internal communication process.

---

# umh-core

**umh-core** is a lightweight edge container from the United Manufacturing Hub (UMH) ecosystem. It is designed to run on resource-constrained edge devices and is built to dynamically adjust its internal data flows based on configuration changes. The container:

- **Bootstraps** with a minimal YAML configuration (including an API key, metadata, and external broker settings).
- Runs a **stateful agent** that reads its desired state from this configuration file. This YAML file may be updated externally (e.g. via a CI/CD pipeline) or internally by the agent’s own communication process.
- Uses **Redpanda** (a Kafka-compatible broker) for internal store-and-forward capabilities.
- Leverages **Benthos** for protocol conversion and data bridging.
- Streams key state information (e.g., Redpanda topics) to the UMH Management Console—where features like the Tag Browser are provided.
- Exposes a Prometheus metrics endpoint for basic monitoring.

> **Key Concept:**  
> Changing the desired state of the system is accomplished by updating a YAML configuration file. Updates may be pushed externally (via CI/CD) or by the agent’s internal communication process. The agent then compares the updated configuration with its current state and makes the necessary transitions.

## Core Components

1. **Agent**  
   - A Go-based control loop that reads its desired state from a YAML configuration file.
   - Manages the lifecycle of Benthos pipelines and Redpanda.
   - Reacts to configuration updates by stopping, updating, or starting pipelines as needed.
   - Communicates with the UMH Management Console (using the provided API key) to both send internal status and receive change requests.

2. **Benthos**  
   - Provides data streaming and transformation capabilities.
   - Runs protocol converters (e.g. OPC UA) that ingest data and publish to Redpanda.
   - Hosts bridging pipelines to forward data (e.g. from Redpanda to an external MQTT broker) when configured.

3. **Redpanda (Kafka-Compatible)**  
   - Operates as the internal message broker for temporary storage and reliable store-and-forward.
   - Data is not directly exposed externally; instead, the agent forwards topic metadata to the Management Console.
   
4. **External MQTT Bridge (Optional)**  
   - When enabled, a Benthos pipeline bridges messages from Redpanda to an external MQTT broker.
   - The configuration for this is specified in the YAML and can be updated as needed.

## High-Level Architecture

```
                ┌───────────────────────────────┐
                │         umh-core          │
                │  (Single Container Instance) │
                │                               │
                │   ┌───────────────────────┐   │
                │   │       Agent         │   │
                │   │ - Reads bootstrapped  │   │
                │   │   YAML config         │   │
                │   │ - Manages state via   │   │
                │   │   a deterministic FSM │   │
                │   │ - Updates desired state│   │
                │   │   on YAML changes     │   │
                │   └───────────────────────┘   │
                │           │                   │
                │   ┌───────────────────────┐   │
                │   │   Benthos Pipelines   │   │
                │   │ - Protocol converters │   │
                │   │ - Data bridges        │   │
                │   └───────────────────────┘   │
                │           │                   │
                │   ┌───────────────────────┐   │
                │   │      Redpanda         │   │
                │   │  (Store-and-Forward)  │   │
                │   └───────────────────────┘   │
                │           │                   │
                │  (Status & Topic Data sent   │
                │   to UMH Management Console) │
                └───────────────────────────────┘
```

## Getting Started

### 1. Prepare the Bootstrap Configuration

On first startup, **umh-core** uses a minimal bootstrapped configuration YAML file. This file is intended to be updated later—either externally via a CI/CD pipeline or by the agent’s communication process—so that the desired state of the container (and its pipelines) can change dynamically.

Create a file named `umh-lite-bootstrap.yaml`:

```yaml
# umh-lite-bootstrap.yaml

apiKey: "YOUR-UMH-BACKEND-API-KEY"

metadata:
  deviceId: "edge-device-001"
  location: "enterprise.siteA.lineB"

externalMQTT:
  enabled: true
  broker: "tcp://broker.example.com:1883"
  username: "mqttUser"
  password: "mqttPass"

logLevel: "info"

# Metrics settings (optional, for Prometheus scraping)
metrics:
  port: 4195
```

**Notes:**
- The `apiKey` is used for secure communication with the UMH Management Console.
- The `metadata` fields uniquely identify this edge instance.
- The bootstrap YAML is a starting point; subsequent configuration changes will be applied by updating this file (either manually, via CI/CD, or via the agent).

### 2. Deploy the Container

Run **umh-core** with the bootstrap config and a persistent volume:

```bash
docker run -d \
  --name umh-core \
  -v $(pwd)/umh-lite-bootstrap.yaml:/data/umh-lite-bootstrap.yaml:ro \
  -v umh-lite-state:/data \
  -p 4195:4195 \
  umh-core:latest
```

**Volume Mounts:**
- `/data/umh-lite-bootstrap.yaml` is the read-only bootstrap configuration.
- `umh-lite-state` is a persistent volume used for stateful data (Redpanda offsets, pipeline state, etc.).

### 3. How Desired State Updates Work

**umh-core** is designed to be dynamic:
- **External Updates via CI/CD:**  
  Users can update the YAML configuration in source control and deploy new versions via a CI/CD pipeline. When the new YAML is mounted, the agent detects the updated desired state and adjusts the pipelines accordingly.
- **Internal Updates via the Agent:**  
  The agent continuously communicates with the UMH Management Console. Change requests (such as new protocol converters or bridge adjustments) are written into an updated YAML file. The agent reads this file and transitions its internal state (e.g., from `StateRunning` to `StateConfigChanged`, then to `StateChangingConfig`, and finally to `StateStarting` before returning to `StateRunning`).

In either case, the updated YAML drives the agent’s finite state machine, ensuring that changes are applied deterministically.

### 4. Metrics

The container exposes a Prometheus metrics endpoint at port `4195`:
- **Metrics Endpoint:** `http://<device-ip>:4195/metrics`  
  This endpoint provides basic internal metrics from the agent, Benthos pipelines, and Redpanda.
- While metrics are supported, the primary focus is on data flow reliability and dynamic configuration, not on extensive local metrics visualization.

### 5. Management Console Integration

The agent sends key status information (including Redpanda topic data) to the UMH Management Console:
- The **Management Console** provides a rich **Tag Browser** that visualizes data topics across the network.
- The console is the central hub for managing configurations, monitoring health, and viewing historical data.
- The agent itself does not host a Tag Browser UI—this functionality is centralized in the console.

### 6. Summary

**umh-core** brings together:
- **Dynamic, YAML-driven configuration**: Bootstrapped with minimal settings and updated at runtime via CI/CD or internal communication.
- **A stateful agent**: Runs a deterministic control loop to manage Benthos pipelines and Redpanda, ensuring that protocol converters and data bridges match the desired state.
- **Reliable data buffering**: Redpanda serves as an internal Kafka-compatible broker, providing store-and-forward capabilities.
- **Seamless integration with the Management Console**: The agent reports state and topic data for centralized tag browsing and overall monitoring.
- **Basic Prometheus Metrics**: For minimal monitoring of internal performance.

## Troubleshooting & FAQs

- **Q: How do I update the desired state?**  
  **A:** Update the `umh-lite-bootstrap.yaml` either by pushing a new version via your CI/CD pipeline or through the Management Console. The agent detects changes and adjusts the pipelines accordingly.

- **Q: Where is data persisted?**  
  **A:** All stateful data (Redpanda offsets, pipeline state) is stored in the mounted volume (`umh-lite-state`). Removing this volume will result in loss of local state.

- **Q: What happens if connectivity is lost?**  
  **A:** Redpanda buffers data locally (store-and-forward) until connectivity is restored. The agent will report status to the Management Console so you can monitor for issues.

- **Q: Does umh-core provide its own Tag Browser?**  
  **A:** No. The agent streams topic data to the Management Console, which provides a centralized Tag Browser view.

## Contributing

Contributions to **umh-core** are welcome! Please refer to our [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines. For questions or discussions, join our [Discord](https://discord.gg/F9mqkZnm9d).

## License

**umh-core** is released under the [Apache 2.0 License](LICENSE).