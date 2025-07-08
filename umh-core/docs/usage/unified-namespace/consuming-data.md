# Consuming Data

> **Topic Browser** – Real-time exploration of Unified Namespace topics via Management Console UI or GraphQL API.

Consuming data from the Unified Namespace involves subscribing to specific topics or patterns and processing the standardized message formats. UMH Core provides multiple consumption methods for different use cases.

## Consumption Patterns

### 1. Bridge Sink Flows

Create outbound Bridges to send UNS data to external systems:

```yaml
protocolConverter:
  - name: uns-to-mqtt
    desiredState: active
    protocolConverterServiceConfig:
      template:
        dataflowcomponent_write:
          benthos:
            input:
              uns:
                topics: ["umh.v1.acme.plant1.line4.pump01._raw.+"]
            pipeline:
              processors:
                - mapping: |
                    root.timestamp = this.timestamp_ms
                    root.value = this.value
                    root.topic = metadata("umh_topic")
            output:
              mqtt:
                urls: ["tcp://external-broker:1883"]
                topic: 'factory/{{ metadata("umh_topic").replace(".", "/") }}'
```

### 2. Stand-alone Flow Consumers

Process UNS data with custom logic:

```yaml
dataFlow:
  - name: raw-data-consumer
    desiredState: active
    dataFlowComponentConfig:
      benthos:
        input:
          uns:
            topics: ["umh.v1.+.+.+.+._raw.+"]
        pipeline:
          processors:
            - mapping: |
                # Parse topic to extract location hierarchy
                let topic_parts = metadata("umh_topic").split(".")
                root.enterprise = topic_parts.1
                root.site = topic_parts.2  
                root.area = topic_parts.3
                root.line = topic_parts.4
                root.work_cell = topic_parts.5
                root.tag_name = topic_parts.7
                root.timestamp = this.timestamp_ms.ts_unix_milli()
                root.value = this.value
        output:
          sql_insert:
            driver: "postgres"
            dsn: "postgres://user:pass@timescale:5432/manufacturing"
            table: "sensor_data"
```

### 3. Topic Browser Access

**Management Console UI (Recommended):**
- Navigate to Unified Namespace → Topic Browser
- Visual topic tree with live updates
- Stores up to 100 events per topic
- Built-in search and filtering
- Aggregates all UMH instances

For detailed usage instructions, see [Topic Browser](./topic-browser.md).

**GraphQL API (Programmatic):**
```bash
# Basic topic query
curl -X POST http://localhost:8090/graphql \
  -H "Content-Type: application/json" \
  -d '{"query": "{ topics(limit: 5) { topic metadata { key value } } }"}'

# Filter by data contract
curl -X POST http://localhost:8090/graphql \
  -H "Content-Type: application/json" \
  -d '{"query": "{ topics(filter: { meta: [{ key: \"data_contract\", eq: \"_temperature\" }] }) { topic } }"}'
```

**Default Configuration:**
- GraphQL enabled by default (`agent.graphql.enabled: true`) on port 8090 (`agent.graphql.port: 8090`)
- Topic Browser service runs automatically (`internal.topicBrowser.desiredState: active`)
- Each UMH instance has its own Topic Browser
- Management Console combines all instances

## Topic Patterns with UNS Input

UNS input supports powerful regex patterns to subscribe to specific data. For complete topic structure details, see [Topic Convention](topic-convention.md).

| Pattern                                       | Matches                                | Use Case                             |
| --------------------------------------------- | -------------------------------------- | ------------------------------------ |
| `umh.v1.acme.+.+.+.+._raw.temperature`        | All raw temperature sensors            | Temperature monitoring dashboard     |
| `umh.v1.acme.plant1.+.+.+._raw.+`             | All raw data from plant1               | Plant-specific raw data analytics    |
| `umh.v1.+.+.+.+.pump01._pump.+`               | All structured data from pump01 assets | Asset-specific monitoring            |
| `umh.v1.acme.plant1.line4.+.+._temperature.+` | Line 4 structured temperature data     | Production line temperature analysis |

**Regex Support:**

```yaml
input:
  uns:
    topics: ["umh.v1.acme.plant1.line[0-9]+.+._raw.(temperature|pressure)"]
```

For complete UNS input syntax, see [Benthos-UMH UNS Input Documentation](https://docs.umh.app/benthos-umh/input/uns-input).

## Message Processing

### Raw Data Payload

Simple sensor data follows the [timeseries payload format](payload-formats.md):

```json
{
  "value": 42.5,
  "timestamp_ms": 1733904005123
}
```

**Processing example:**

```yaml
pipeline:
  processors:
    - mapping: |
        # Convert timestamp to different formats
        root.timestamp_iso = this.timestamp_ms.ts_unix_milli().ts_format("2006-01-02T15:04:05Z07:00")
        root.timestamp_unix = this.timestamp_ms / 1000
        root.measurement = this.value
        
        # Extract location from UNS topic (see Topic Convention for structure)
        let parts = metadata("umh_topic").split(".")
        root.location = {
          "enterprise": parts.1,
          "site": parts.2,
          "area": parts.3,
          "line": parts.4,
          "work_cell": parts.5,
          "tag": parts.7
        }
```

For topic structure details and parsing rules, see [Topic Convention](topic-convention.md).

### Structured Data Contract Payloads

For structured data contracts, use [Data Models](../data-modeling/data-models.md) to understand payload structure:

```yaml
# Consuming structured pump model data
input:
  uns:
    topics: ["umh.v1.+.+.+.+.pump01._pump.+"]
pipeline:
  processors:
    - mapping: |
        # Handle different fields from pump model
        match metadata("umh_topic").split(".").7 {
          "pressure" => root.sensor_type = "pressure"
          "temperature" => root.sensor_type = "temperature"  
          "motor.current" => root.sensor_type = "motor_current"
          "motor.rpm" => root.sensor_type = "motor_rpm"
          "diagnostics.vibration" => root.sensor_type = "vibration"
        }
        root.asset_id = "pump01"
        root.value = this.value
        root.timestamp = this.timestamp_ms
```

## UNS Input Features

UNS input abstracts away Kafka/Redpanda complexity and provides:

* **Topic pattern matching** with wildcards and regex
* **Automatic metadata** - `umh_topic` contains the full UNS topic path
* **Message headers** - All UNS metadata available via `metadata()` function
* **Embedded broker access** - No need to configure Kafka addresses

```yaml
input:
  uns:
    topics: 
      - "umh.v1.acme.plant1.+.+._raw.+"      # Wildcard matching
      - "umh.v1.acme.plant2.line[1-3].+._temperature.+" # Regex support
```

## Error Handling

Handle consumer errors gracefully:

```yaml
pipeline:
  processors:
    - try:
        - mapping: |
            # Your processing logic
            root.processed_value = this.value * 1.8 + 32
        - catch:
            - mapping: |
                root = deleted()
                # Log error and continue
            - log:
                level: "ERROR" 
                message: "Failed to process message: ${! error() }"
```

## Integration Examples

### Database Integration Pattern

UNS data can be consumed into various databases. Here's the basic pattern:

```yaml
pipeline:
  processors:
    - mapping: |
        # Transform UNS message for database storage
        root.timestamp = this.timestamp_ms.ts_unix_milli()
        root.location = metadata("umh_topic").split(".").slice(1, 6).join(".")
        root.tag_name = metadata("umh_topic").split(".").7
        root.value = this.value
output:
  sql_insert:
    driver: "postgres"
    table: "sensor_readings"
    # ... database-specific configuration
```

For complete database integration examples including TimescaleDB, InfluxDB, and other systems, see [Integration Patterns Guide](../../integrations/).

## Data Contract Evolution

### Consuming Raw Data (Simple)

```yaml
input:
  uns:
    topics: ["umh.v1.+.+.+.+._raw.+"]  # All raw sensor data
pipeline:
  processors:
    - mapping: |
        root.sensor_value = this.value
        root.timestamp = this.timestamp_ms
```

### Consuming Structured Data (Advanced) 🚧

```yaml
input:
  uns:
    topics: ["umh.v1.+.+.+.+._temperature.+"]  # Structured temperature data
pipeline:
  processors:
    - mapping: |
        # Structured contracts have specific field names
        root.temperature_celsius = this.temperatureInC
        root.timestamp = this.timestamp_ms
        # Additional metadata from data model constraints
        root.unit = "°C"
```

## Migration from UMH Classic

> **UMH Classic Users:** See [Migration from UMH Classic to UMH Core](../../production/migration-from-classic.md) for complete migration instructions including `_historian` → `_raw` data contract changes.

## Why UNS Input/Output?

UMH Core uses UNS input/output instead of direct Kafka access because:

* **Abstraction**: Hides Kafka/Redpanda complexity from users
* **Embedded Integration**: Works seamlessly with UMH Core's embedded Redpanda
* **Topic Management**: Automatic topic creation and management
* **Metadata Handling**: Proper UNS metadata propagation
* **Pattern Matching**: Advanced regex support for topic patterns

This aligns with UMH Core's philosophy of embedding Redpanda as an internal implementation detail rather than exposing it directly to users.

## Topic Browser

The Topic Browser provides real-time exploration of UNS topics and data:

* **Management Console UI** - Visual topic tree with live updates
* **GraphQL API** - Programmatic access for automation and integration
* **Advanced Filtering** - Search by topic patterns, metadata, and content
* **Multi-Instance Support** - Each UMH instance has its own Topic Browser

For detailed usage instructions, see [Topic Browser Guide](./topic-browser.md).

## Next Steps

* [**Data Modeling**](../data-modeling/) 🚧 - Structure complex data consumption with explicit models
* [**Data Flows Overview**](../data-flows/) - Advanced processing patterns
* [**Configuration Reference**](../../reference/configuration-reference.md) - Complete consumer configuration

## Learn More

* [Historians vs Open-Source databases](https://learn.umh.app/blog/historians-vs-open-source-databases-which-is-better/) - Choose the right storage backend
* [Why we chose TimescaleDB over InfluxDB](https://learn.umh.app/blog/why-we-chose-timescaledb-over-influxdb/) - Database selection rationale
* [Simplifying Tag Browsing in Grafana](https://learn.umh.app/blog/simplifying-tag-browsing-in-grafana/) - Visualization best practices
* [Benthos-UMH UNS Output Documentation](https://docs.umh.app/benthos-umh/output/uns-output) - Complete UNS output reference
