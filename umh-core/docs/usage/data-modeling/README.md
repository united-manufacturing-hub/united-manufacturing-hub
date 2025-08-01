# Data Modeling

Unified data-modelling builds on our existing data contract foundation to provide a comprehensive approach to industrial data modeling.

UMH Core's unified data-modelling system provides a structured approach to defining, validating, and processing industrial data. It bridges the gap between raw sensor data and meaningful business information through a clear hierarchy of components.

## Why Data Modeling Matters

Manufacturing companies typically start with **implicit data modeling** - using bridges to contextualize data factory by factory. This bottom-up approach works well for single sites: look at what's available in your PLC/Kepware, add basic metadata, rename cryptic tags like `XYDAG324` to `temperature`, and publish to the UNS.

But as companies scale across **multiple factories**, they hit a wall:

- **Inconsistent schemas**: Each site names the same equipment differently (`motor_speed` vs `rpm` vs `rotational_velocity`)
- **No standardization**: Pump data from Factory A has different fields than identical pumps in Factory B
- **Analytics nightmares**: Cross-site dashboards and analytics require custom mapping for every location
- **Knowledge silos**: Each site's contextualization is trapped in local configurations

**Explicit data modeling** solves this by defining standardized templates that enforce consistency across the entire enterprise. Instead of each factory doing its own contextualization, you define once: "Every Pump has these exact fields: `pressure`, `temperature`, `motor.current`, `motor.rpm`" - then apply that template everywhere.

### From Implicit to Explicit

| Approach | Scope | Benefits | Limitations |
|----------|--------|----------|-------------|
| **Implicit** (Current Bridges) | Per-factory contextualization | Quick setup, site-specific optimization | Inconsistent across sites, no templates |
| **Explicit** (Data Modeling) | Enterprise-wide standardization | Consistent schemas, reusable templates, cross-site analytics | Requires upfront design, more rigid |

UMH's unified data-modelling bridges this gap: keep the flexibility of per-site bridges for raw data collection, but add explicit modeling on top for enterprise standardization.

## Object Hierarchy

The unified data-modelling system uses a four-layer hierarchy:

```
Payload-Shape → Data-Model → Data-Contract → Stream-Processor
```

| Layer | Purpose | Example |
|-------|---------|---------|
| **[Payload-Shape](payload-shapes.md)** | Canonical schema fragment (timeseries default) | `timeseries`, `blob` |
| **[Data-Model](data-models.md)** | Reusable class; tree of fields, folders, sub-models | `Motor`, `Pump`, `Temperature` |
| **[Data-Contract](data-contracts.md)** | Binds model version; decides retention & sinks | `_temperature_v1`, `_pump_v1` |
| **[Stream-Processor](stream-processors.md)** | Runtime pipeline for model instances | `furnaceTemp_sp`, `pump41_sp` |

## Quick Example

Here's how the system transforms raw PLC data into structured, validated information:

### 1. Raw Data Input
```
Topic: umh.v1.corpA.plant-A.line-4.furnace1._raw.temperature_F
Payload: { "value": 1500, "timestamp_ms": 1733904005123 }
```

### 2. Data Model Definition
```yaml
datamodels:
  temperature:
    description: "Temperature sensor model"
    versions:
      v1:
        structure:
          temperatureInC:
            _payloadshape: timeseries-number
```

### 3. Data Contract
```yaml
datacontracts:
  - name: _temperature_v1
    model:
      name: temperature
      version: v1
    default_bridges:
      - type: timescaledb
        retention_in_days: 365
```

### 4. Stream Processor
```yaml
streamprocessors:
  - name: furnaceTemp_sp
    _templateRef: "temperature_template"
    location:
      0: corpA
      1: plant-A
      2: line-4
      3: furnace1
    variables:
      temp_sensor: "temperature_F"
      sn: "SN-F1-001"
```

### 5. Structured Output
```
Topic: umh.v1.corpA.plant-A.line-4.furnace1._temperature.temperatureInC
Payload: { "value": 815.6, "timestamp_ms": 1733904005123 }
Database: Auto-created TimescaleDB hypertable
```

## Key Benefits

- **Unified YAML Dialect**: Single configuration language for all transformations
- **Generic ISA-95 Support**: Built-in hierarchical naming (level0-4)
- **Schema Registry Integration**: All layers pushed to Redpanda Schema Registry
- **Automatic Validation**: UNS output plugin rejects non-compliant messages
- **Sub-Model Reusability**: Define once, use across multiple assets
- **Enterprise Reliability**: Combines MQTT simplicity with data-center-grade features
- **Generic Hierarchical Support**: Built-in hierarchical naming (level0-4+) supports ISA-95, KKS, or custom standards

## Getting Started

1. **[Define Data Models](data-models.md)** - Create reusable data structures
2. **[Create Data Contracts](data-contracts.md)** - Bind models to storage and retention policies  
3. **[Deploy Stream Processors](stream-processors.md)** - Implement real-time data transformation
4. **[Configure in Management Console](../data-flows/stream-processor.md#management-console)** - Use the web interface for deployment

## Architecture Context

This unified approach builds on UMH's hybrid architecture, combining:

- **MQTT** for lightweight edge communication
- **Kafka** for reliable enterprise messaging  
- **Data Contracts** for application-level guarantees
- **Schema Registry** for centralized validation

For deeper technical background on why this hybrid approach is necessary, see our [comprehensive analysis of MQTT limitations and data contract solutions](https://learn.umh.app/blog/what-is-mqtt-why-most-mqtt-explanations-suck-and-our-attempt-to-fix-them/).

## Related Documentation

- [Stream Processors Implementation](../data-flows/stream-processor.md) - Detailed runtime configuration
- [Unified Namespace](../unified-namespace/README.md) - Topic structure and payload formats
- [Data Flows Overview](../data-flows/README.md) - Integration with other flow types 