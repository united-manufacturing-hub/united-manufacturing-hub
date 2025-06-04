# Stand-Alone Flow

Stand-alone Flows (shown as `dataFlow:` in YAML configuration) provide point-to-point data movement without the connection monitoring and location hierarchy features of Bridges. They're ideal for custom processing pipelines and situations where you don't need UNS buffering.

## When to Use Stand-alone Flows

**Choose Stand-alone Flows when:**
- Point-to-point data transformation is sufficient
- No connection monitoring required
- Custom processing logic that doesn't fit Bridge patterns
- Migrating existing Benthos configurations
- High-throughput scenarios where UNS overhead isn't needed

**Choose Bridges when:**
- Connecting to field devices (PLCs, sensors, actuators)
- Need connection health monitoring
- Want location-based data organization
- Publishing to the Unified Namespace

## Basic Configuration

```yaml
dataFlow:
  - name: custom-processor
    desiredState: active
    dataFlowComponentConfig:
      benthos:
        input:
          # Any Benthos input
        pipeline:
          processors:
            # Custom processing logic
        output:
          # Any Benthos output
```

## Common Use Cases

### 1. MQTT Bridge

Forward data between MQTT brokers:

```yaml
dataFlow:
  - name: mqtt-bridge
    desiredState: active
    dataFlowComponentConfig:
      benthos:
        input:
          mqtt:
            urls: ["tcp://source-broker:1883"]
            topics: ["factory/sensors/+/+"]
            client_id: "umh-mqtt-bridge"
        pipeline:
          processors:
            - mapping: |
                # Transform topic structure
                root.timestamp = now().ts_unix_milli()
                root.device_id = metadata("mqtt_topic").split("/").2
                root.measurement = metadata("mqtt_topic").split("/").3
                root.value = this
        output:
          mqtt:
            urls: ["tcp://target-broker:1883"]
            topic: 'umh/{{ metadata("mqtt_topic").replace("/", ".") }}'
```

### 2. Database ETL

Extract, transform, and load data between systems:

```yaml
dataFlow:
  - name: erp-to-timescale
    desiredState: active
    dataFlowComponentConfig:
      benthos:
        input:
          sql_select:
            driver: "mssql"
            dsn: "server=erp-db;database=manufacturing"
            table: "production_orders"
            columns: ["order_id", "product_code", "start_time", "quantity"]
            where: "status = 'active'"
        pipeline:
          processors:
            - mapping: |
                root.order_id = this.order_id
                root.product = this.product_code
                root.start_timestamp = this.start_time.ts_parse("2006-01-02 15:04:05").ts_unix_milli()
                root.planned_quantity = this.quantity
        output:
          sql_insert:
            driver: "postgres"
            dsn: "postgres://user:pass@timescale:5432/manufacturing"
            table: "active_orders"
```

### 3. File Processing

Process files from network shares or object storage:

```yaml
dataFlow:
  - name: cnc-file-processor
    desiredState: active
    dataFlowComponentConfig:
      benthos:
        input:
          file:
            paths: ["/data/cnc-programs/*.nc"]
            scanner:
              lines: {}
        pipeline:
          processors:
            - mapping: |
                # Parse CNC program files
                root.filename = metadata("path").filepath_split().1
                root.line_number = metadata("line_number")
                root.gcode = this
                root.timestamp = now().ts_unix_milli()
            - branch:
                request_map: |
                  # Extract program metadata
                  root.program_number = this.gcode.re_find("N[0-9]+").trim_prefix("N")
                result_map: |
                  root.program_metadata = this
        output:
          http_client:
            url: "http://mes-system/api/cnc-programs"
            verb: "POST"
            headers:
              Content-Type: "application/json"
```

### 4. UNS Data Processing

Process data from the Unified Namespace:

```yaml
dataFlow:
  - name: uns-analytics
    desiredState: active
    dataFlowComponentConfig:
      benthos:
        input:
          uns:
            topics: ["umh.v1.+.+.+.+._raw.temperature"]
        pipeline:
          processors:
            - mapping: |
                # Process temperature data from all plants
                root.celsius = this.value
                root.fahrenheit = this.value * 1.8 + 32
                root.location = metadata("umh_topic").split(".").slice(1, 6).join(".")
                root.timestamp = this.timestamp_ms
        output:
          influxdb:
            urls: ["http://influxdb:8086"]
            token: "${INFLUX_TOKEN}"
            org: "manufacturing"
            bucket: "analytics"
```

### 5. Custom Protocol Handler

Handle proprietary or custom protocols:

```yaml
dataFlow:
  - name: custom-protocol
    desiredState: active
    dataFlowComponentConfig:
      benthos:
        input:
          socket:
            network: "tcp"
            address: ":8080"
            scanner:
              lines:
                max_length: 1024
        pipeline:
          processors:
            - mapping: |
                # Parse custom protocol format
                let parts = this.split("|")
                root.device_id = parts.0
                root.timestamp = parts.1.number().ts_unix_milli()
                root.values = parts.slice(2).map_each(part -> part.number())
            - unarchive:
                format: "json_array"
                to_map: true
        output:
          uns: {}  # Publish processed data to UNS
```

## Advanced Processing

### Conditional Routing

Route messages based on content:

```yaml
pipeline:
  processors:
    - switch:
        - check: 'this.device_type == "temperature"'
          processors:
            - mapping: |
                root.celsius = this.value
                root.fahrenheit = this.value * 1.8 + 32
        - check: 'this.device_type == "pressure"'
          processors:
            - mapping: |
                root.pascal = this.value
                root.bar = this.value / 100000
        - processors:
            - mapping: |
                root.raw_value = this.value
```

### Batching and Windowing

Aggregate data over time windows:

```yaml
pipeline:
  processors:
    - group_by_value:
        value: '${! metadata("mqtt_topic") }'
    - window:
        type: "tumbling"
        size: "30s"
    - mapping: |
        root.topic = this.0.metadata("mqtt_topic")
        root.count = this.length()
        root.average = this.map_each(msg -> msg.value).fold(0, item, acc -> acc + item) / this.length()
        root.min = this.map_each(msg -> msg.value).fold(999999, item, acc -> if item < acc { item } else { acc })
        root.max = this.map_each(msg -> msg.value).fold(-999999, item, acc -> if item > acc { item } else { acc })
        root.window_start = this.0.timestamp
        root.window_end = now().ts_unix_milli()
```

## Error Handling

Implement robust error handling:

```yaml
pipeline:
  processors:
    - try:
        - mapping: |
            # Your main processing logic
            root.processed = this.raw_value * 2
        - catch:
            - mapping: |
                root.error = error()
                root.original_message = this
            - log:
                level: "ERROR"
                message: "Processing failed: ${! error() }"
            - bloblang: |
                # Send to dead letter queue
                root = {
                  "failed_at": now().ts_unix_milli(),
                  "error": error(),
                  "original": this
                }
            - uns: {}  # Send errors to UNS for monitoring
```

## UNS Integration Guidelines

### When to Use UNS Input/Output

**Use UNS output when:**
- Publishing processed data to the Unified Namespace
- Need data to be discoverable via Topic Browser
- Want to leverage UNS topic structure and metadata

**Use UNS input when:**
- Consuming data from the Unified Namespace
- Need pattern matching with regex support
- Want to abstract away Kafka/Redpanda details

**Use direct protocols when:**
- Connecting to external systems (MQTT brokers, databases, APIs)
- Point-to-point integration without UNS involvement
- Legacy system integration

### Example: MQTT to UNS Gateway

```yaml
dataFlow:
  - name: mqtt-to-uns-gateway
    desiredState: active
    dataFlowComponentConfig:
      benthos:
        input:
          mqtt:
            urls: ["tcp://factory-mqtt:1883"]
            topics: ["sensors/+/+/+"]
        pipeline:
          processors:
            - tag_processor:
                defaults: |
                  # Transform MQTT topic to UNS structure
                  let parts = metadata("mqtt_topic").split("/")
                  msg.meta.location_path = "acme.plant1." + parts.1 + "." + parts.2;
                  msg.meta.data_contract = "_raw";
                  msg.meta.tag_name = parts.3;
                  return msg;
        output:
          uns: {}  # Publish to UNS
```

## Comparison with Bridges

| Feature | Stand-alone Flow | Bridge |
|---------|------------------|---------|
| **Configuration** | `dataFlow:` | `protocolConverter:` |
| **Connection Monitoring** | ❌ | ✅ Connection health checks |
| **Location Hierarchy** | Manual | ✅ Automatic via `location:` |
| **UNS Integration** | Optional | ✅ Built-in UNS output |
| **Templating** | ❌ | ✅ Variable substitution |
| **State Management** | Basic | ✅ Advanced FSM states |
| **Use Case** | Custom processing | Device connectivity |

## Best Practices

1. **Use descriptive names**: `customer-data-sync` not `dataflow-1`
2. **Handle errors gracefully**: Always include error handling logic
3. **Monitor resource usage**: Stand-alone flows can consume significant resources
4. **Version your configurations**: Use comments to document changes
5. **Test incrementally**: Start with simple mappings, add complexity gradually
6. **Choose the right I/O**: Use UNS input/output for UNS integration, direct protocols for external systems

## Migration from Benthos

Existing Benthos configurations can be migrated easily:

```yaml
# Original Benthos config.yaml
input:
  mqtt:
    urls: ["tcp://broker:1883"]
    topics: ["sensors/+"]

# UMH Core Stand-alone Flow
dataFlow:
  - name: migrated-mqtt-flow
    desiredState: active
    dataFlowComponentConfig:
      benthos:
        input:
          mqtt:
            urls: ["tcp://broker:1883"]  
            topics: ["sensors/+"]
        # Add your existing pipeline and output here
```

## Related Documentation

- **[Bridges](bridges.md)** - Device connectivity with monitoring
- **[Configuration Reference](../../reference/configuration-reference.md)** - Complete YAML syntax
- **[Unified Namespace](../unified-namespace/README.md)** - UNS integration patterns

## Learn More

- [Node-RED meets Benthos!](https://learn.umh.app/blog/node-red-meets-benthos/) - Enhanced processing capabilities
- [Tools & Techniques for scalable data processing in Industrial IoT](https://learn.umh.app/blog/tools-techniques-for-scalable-data-processing-in-industrial-iot/) - Architecture patterns
- [Benthos-UMH UNS Output Documentation](https://docs.umh.app/benthos-umh/output/uns-output) - Complete UNS output reference

