# Migrating from UMH Classic to UMH Core

This guide walks through migrating from UMH Classic to UMH Core. The key changes involve data contracts, configuration syntax, and architectural improvements.

## Migration Strategy

The recommended approach is **side-by-side deployment** to minimize risk:

1. **Deploy Core next to Classic** - Point a Bridge at the Classic UNS topics
2. **Cut over producers/consumers** gradually  
3. **Shut down Classic** pods once data is verified

## Key Changes from Classic to Core

### Data Contract Migration

#### Core Change: `_historian` → `_raw`

**UMH Classic (Deprecated):**
```yaml
# Bridge configuration
pipeline:
  processors:
    - tag_processor:
        defaults: |
          msg.meta.data_contract = "_historian";

# Consumer patterns  
topics: ["umh.v1.+.+.+.+._historian.+"]
```

**UMH Core (Current):**
```yaml
# Bridge configuration - Start simple
pipeline:
  processors:
    - tag_processor:
        defaults: |
          msg.meta.data_contract = "_raw";

# Consumer patterns
input:
  uns:
    topics: ["umh.v1.+.+.+.+._raw.+"]        # For simple sensor data
    topics: ["umh.v1.+.+.+.+._temperature.+"]  # For structured temperature data
```

### Configuration Changes

#### Bridge Configuration

**UMH Classic:**
```yaml
# Old benthos-style configuration
input:
  opcua: 
    # ... configuration
processors:
  - tag_processor:
      defaults: |
        msg.meta.data_contract = "_historian";
output:
  kafka: {}
```

**UMH Core:**
```yaml
# New protocolConverter (Bridge) configuration
protocolConverter:
  - name: device-bridge
    desiredState: active
    protocolConverterServiceConfig:
      location:
        2: "production-line"
        3: "device-name"
      config:
        dataflowcomponent_read:
          benthos:
            input:
              opcua: 
                # ... configuration
            pipeline:
              processors:
                - tag_processor:
                    defaults: |
                      msg.meta.data_contract = "_raw";
            output:
              uns: {}  # Always use UNS output
```

#### Key Differences

| Aspect | UMH Classic | UMH Core |
|--------|-------------|----------|
| **Data Contracts** | `_historian` only | `_raw` + explicit contracts |
| **Configuration** | Direct Benthos config | Embedded within Bridges/Flows |
| **Output** | Direct Kafka | UNS output (abstracts Kafka) |
| **Location** | Manual topic construction | Automatic hierarchical path construction (supports ISA-95, KKS, or custom naming) |
| **Data Modeling** | Single payload format | Structured models + contracts |
| **Database Integration** | Automatic with `_historian` | Manual with specific contracts |

## Important: No Default Database Integration

**⚠️ Breaking Change**: UMH Core has **no default data contract** that automatically writes to TimescaleDB.

**UMH Classic:**
```yaml
# _historian automatically integrated with TimescaleDB
pipeline:
  processors:
    - tag_processor:
        defaults: |
          msg.meta.data_contract = "_historian";  # Auto-saved to database
```

**UMH Core:**
```yaml
# _raw is for simple data only - NO automatic database integration
pipeline:
  processors:
    - tag_processor:
        defaults: |
          msg.meta.data_contract = "_raw";  # NOT saved to database automatically

# Database integration requires explicit configuration
dataFlow:
  - name: timescale-sink
    dataFlowComponentConfig:
      benthos:
        input:
          uns:
            topics: ["umh.v1.+.+.+.+._raw.+"]
        pipeline:
          processors:
            - mapping: |
                # Manual transformation for database
                root.timestamp = this.timestamp_ms.ts_unix_milli()
                root.value = this.value
                # Extract location from topic
                let parts = metadata("umh_topic").split(".")
                root.enterprise = parts.1
                root.site = parts.2
                root.tag_name = parts.7
        output:
          sql_insert:
            driver: "postgres"
            dsn: "postgres://user:pass@timescale:5432/manufacturing"
            table: "sensor_readings"
```

**Migration Impact:**
- `_raw` data contracts do **NOT** automatically save to TimescaleDB
- You must create explicit database sink flows for any data you want persisted
- This provides more control but requires manual configuration

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

### 3. Verify Migration

1. **Check topic creation**: Ensure new `_raw` topics are being created
2. **Validate consumers**: Confirm all consumers are receiving data from `_raw` topics  
3. **Monitor metrics**: Watch for any data loss or processing errors
4. **Test integrations**: Verify external systems still receive expected data

## Side-by-Side Deployment

Run Core alongside Classic during transition:

```yaml
# Core Bridge reading from Classic UNS
protocolConverter:
  - name: classic-bridge
    desiredState: active
    protocolConverterServiceConfig:
      config:
        dataflowcomponent_read:
          benthos:
            input:
              kafka:
                addresses: ["classic-kafka:9092"]
                topics: ["umh.v1.+.+.+.+._historian.+"]
            pipeline:
              processors:
                - mapping: |
                    # Convert _historian to _raw
                    root = this
                    meta.data_contract = "_raw"
            output:
              uns: {}
```

## Getting Help

- **Migration Issues**: See [Troubleshooting Guide](../production/troubleshooting.md)
- **Configuration Questions**: Check [Configuration Reference](../reference/configuration-reference.md)
- **Community Support**: Visit [UMH Community Forum](https://community.umh.app)

## Next Steps

After completing the basic migration:

1. **[Data Modeling](../usage/data-modeling/README.md)** - Implement structured data contracts
2. **[Stream Processors](../usage/data-flows/stream-processor.md)** - Transform raw data to business models  
3. **[Production Deployment](../production/README.md)** - Scale and secure your UMH Core deployment 

### Tag Groups Migration

**UMH Classic (Tag Groups in Payload):**
```yaml
# Classic bridge configuration
pipeline:
  processors:
    - tag_processor:
        defaults: |
          msg.meta.data_contract = "_historian";
          msg.meta.tag_name = "head";  # Groups appear inside payload

# Classic payload with tag groups
{
  "timestamp_ms": 1670001234567,
  "pos": { 
    "x": 12.5,
    "y": 7.3,
    "z": 3.2
  },  
  "temperature": 50.0,
  "collision": false
}

# Classic database storage
# head$pos$x, head$pos$y, head$pos$z, head$temperature, head$collision
```

**UMH Core (Virtual Paths in Topics):**
```yaml
# Core data model with virtual paths
datamodels:
  - name: CNCHead
    version: v1
    structure:
      pos:
        x: { type: timeseries }
        y: { type: timeseries }
        z: { type: timeseries }
      temperature: { type: timeseries }
      collision: { type: timeseries }

# Core stream processor
streamprocessors:
  - name: cnc_head_sp
    contract: _cnc_head:v1
    sources:
      x_raw: "umh.v1.corp.plant.line.cnc._raw.head_pos_x"
      y_raw: "umh.v1.corp.plant.line.cnc._raw.head_pos_y"
      z_raw: "umh.v1.corp.plant.line.cnc._raw.head_pos_z"
      temp_raw: "umh.v1.corp.plant.line.cnc._raw.head_temperature"
      collision_raw: "umh.v1.corp.plant.line.cnc._raw.head_collision"
    mapping:
      pos.x: "x_raw"
      pos.y: "y_raw"  
      pos.z: "z_raw"
      temperature: "temp_raw"
      collision: "collision_raw"

# Result: Individual topics with virtual paths
# umh.v1.corp.plant.line.cnc._cnc_head.pos.x
# umh.v1.corp.plant.line.cnc._cnc_head.pos.y
# umh.v1.corp.plant.line.cnc._cnc_head.pos.z
# umh.v1.corp.plant.line.cnc._cnc_head.temperature
# umh.v1.corp.plant.line.cnc._cnc_head.collision
```

**Migration Benefits:**
- **Granular subscriptions**: Subscribe to just `pos.*` for positioning data
- **Independent scaling**: Different retention for different sensor groups
- **Type safety**: Each topic has explicit schema validation
- **Cross-site consistency**: Same virtual path structure across all CNCs

**Migration Steps:**
1. **Identify tag groups** in Classic payloads (nested JSON objects)
2. **Create data models** with folder structures matching groups
3. **Define stream processors** to map raw inputs to virtual paths
4. **Update consumers** to subscribe to specific virtual path patterns 