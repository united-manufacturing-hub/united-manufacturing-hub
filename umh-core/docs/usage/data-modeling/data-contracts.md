# Data Contracts

Data contracts bind data models to storage, retention, and processing policies, ensuring consistent data handling across your industrial systems.

Data contracts define the operational aspects of your data models - where data gets stored, how long it's retained, and what processing rules apply. They bridge the gap between logical data structure (models) and physical data management.

## Overview

Data contracts are stored in the `datacontracts:` configuration section and reference specific versions of data models:

```yaml
datacontracts:
  - name: contract_name
    model:
      name: modelname
      version: v1
    default_bridges:
      - type: timescaledb
        retention_in_days: 365
```

## Core Properties

### Name and Versioning

```yaml
datacontracts:
  - name: _temperature_v1
    model:
      name: temperature
      version: v1
```

**Naming Convention:**
- Contract names start with underscore (`_temperature`, `_pump`)
- Model references include name and version (`name: temperature, version: v1`)

### Model Binding

Each contract binds to exactly one data model version:

```yaml
datacontracts:
  - name: _pump_v1
    model:
      name: pump
      version: v1  # Specific model version
```

This binding is immutable - to change the model, create a new contract version.

### Data Bridges

Contracts specify where data gets stored and processed:

```yaml
datacontracts:
  - name: _temperature_v1
    model:
      name: temperature
      version: v1
    default_bridges:
      - type: timescaledb
        retention_in_days: 365
      - type: cloud_storage
```

**Available Bridge Types:**
- `timescaledb`: Automatic TimescaleDB hypertable creation
- `umh-api-sync`: Sync to higher-level UNS
- `cloud_storage`: S3-compatible storage
- `analytics_pipeline`: Stream analytics processing

**Bridge Configuration Details:**
```yaml
default_bridges:        # 🚧 **Roadmap Item**
  - type: timescaledb   # create a default bridge that will store it to timescaledb 
    host:
    port:
    credentials:
    retention_in_days: 365
  - type: umh-api-sync
    remote: 10.13.37.50:80   # create a default bridge, that will send it to a higher level UNS on that IP
```

### Retention Policies

Define how long data is kept:

```yaml
datacontracts:
  - name: _historian_v1
    model:
      name: historiandata
      version: v1
    default_bridges:
      - type: timescaledb
        retention_in_days: 2555  # ~7 years
```

## Complete Examples

### Simple Temperature Contract

```yaml
datamodels:
  temperature:
    description: "Temperature sensor model"
    versions:
      v1:
        structure:
          temperatureInC:
            _payloadshape: timeseries-number

datacontracts:
  - name: _temperature_v1
    model:
      name: temperature
      version: v1
    default_bridges:
      - type: timescaledb
        retention_in_days: 365
```

### Complex Pump Contract

```yaml
datamodels:
  motor:
    description: "Standard motor model"
    versions:
      v1:
        structure:
          current:
            _payloadshape: timeseries-number
          rpm:
            _payloadshape: timeseries-number
          temperature:
            _payloadshape: timeseries-number

  pump:
    description: "Pump with motor and diagnostics"
    versions:
      v1:
        structure:
          pressure:
            _payloadshape: timeseries-number
          temperature:
            _payloadshape: timeseries-number
          running:
            _payloadshape: timeseries-string
          vibration:
            x-axis:
              _payloadshape: timeseries-number
            y-axis:
              _payloadshape: timeseries-number
            z-axis:
              _payloadshape: timeseries-number
              _meta: # 🚧 **Roadmap Item**
                description: "Z-axis vibration measurement"
                unit: "m/s"
              _constraints: # 🚧 **Roadmap Item**
                max: 100
                min: 0
          motor:
            _refModel:
              name: motor
              version: v1
          acceleration:
            x:
              _payloadshape: timeseries-number
            y:
              _payloadshape: timeseries-number
          serialNumber:
            _payloadshape: timeseries-string

datacontracts:
  - name: _pump_v1
    model:
      name: pump
      version: v1
    default_bridges:
      - type: timescaledb
        retention_in_days: 1825  # 5 years
      - type: analytics_pipeline
  - name: _historian
    # no model = no enforcement
    default_bridges: # 🚧 **Roadmap Item**
      - type: timescaledb   # create a default bridge that will store it to timescaledb 
        host:
        port:
        credentials:
        retention_in_days: 365
  - name: _raw
    # no model
    # no bridge
```

## Generated Database Schema

> 🚧 **Roadmap Item** - Automatic database schema generation from data contracts and models is under design. This will include:
> - Auto-generated TimescaleDB hypertables based on data models
> - Field type mapping from payload shapes to database columns
> - Automatic indexing for location-based queries
> - Sub-model field flattening strategies
> - Location hierarchy storage format

## Contract Evolution

### Version Management

Contracts support controlled evolution:

```yaml
# Version 1
datacontracts:
  - name: _pump_v1
    model:
      name: pump
      version: v1
    default_bridges:
      - type: timescaledb
        retention_in_days: 365

# Version 2 - Extended retention
datacontracts:
  - name: _pump_v2
    model:
      name: pump
      version: v2  # Updated model
    default_bridges:
      - type: timescaledb
        retention_in_days: 2555  # Extended retention
      - type: analytics_pipeline  # New bridge
```

### Backward Compatibility

- Multiple contract versions can coexist
- Existing stream processors continue using their bound contract version
- Database schemas adapt automatically for new fields
- No downtime required for contract updates

## Bridge Configuration Details

### TimescaleDB Bridge

```yaml
default_bridges:
  - type: timescaledb
    retention_in_days: 365
```

**Behavior:**
- Auto-creates hypertable `{contract_name}_{version}`
- Generates appropriate column types from model constraints
- Creates location indexes for ISA-95 queries
- Handles sub-model field flattening automatically

### UMH API Sync Bridge

```yaml
default_bridges:
  - type: umh-api-sync
    remote: "10.13.37.50:80"
    auth_token: "${API_AUTH_TOKEN}"
    batch_size: 1000
```

**Behavior:**
- Sends data to external systems
- Supports authentication and batching
- Configurable retry policies
- Schema validation before transmission

### Cloud Storage Bridge

```yaml
default_bridges:
  - type: cloud_storage
    bucket: "industrial-data-lake"
    prefix: "pump-data/{year}/{month}/{day}/"
    format: "parquet"
    compression: "gzip"
```

**Behavior:**
- Partitioned storage by time
- Multiple format support (JSON, Parquet, Avro)
- Compression options
- Automated lifecycle management

## Validation and Enforcement

### Schema Enforcement

All data contracts are registered in Redpanda Schema Registry:

- **Publish-time validation**: Messages are validated before acceptance
- **Consumer protection**: Invalid messages are rejected automatically  
- **Evolution safety**: Schema changes must maintain compatibility

### Runtime Validation

The UNS output plugin enforces contract compliance:

```yaml
# Invalid message - rejected
Topic: umh.v1.corpA.plant-A.line-4.pump41._pump_v1.invalid_field
Reason: Field 'invalid_field' not defined in _pump model, version v1

# Valid message - accepted
Topic: umh.v1.corpA.plant-A.line-4.pump41._pump_v1.pressure
Payload: {"value": 42.5, "timestamp_ms": 1733904005123}
```

## Best Practices

### Contract Design

- **Single responsibility**: One contract per logical entity type
- **Semantic naming**: Use descriptive, underscore-prefixed names
- **Version explicitly**: Always specify model and contract versions
- **Plan for growth**: Consider future bridge requirements

### Retention Planning

- **Match business needs**: Align retention with regulatory requirements
- **Consider storage costs**: Balance retention vs. storage expenses
- **Plan for archival**: Design archival strategies for historical data

### Bridge Selection

- **TimescaleDB for time-series**: Optimal for sensor data and analytics
- **Cloud storage for archives**: Long-term, cost-effective storage
- **UMH API Sync for integration**: Higher-level UNS connectivity

### Schema Evolution

- **Additive changes**: Add fields rather than modifying existing ones
- **Test compatibility**: Validate schema evolution before deployment
- **Document changes**: Maintain clear change logs

## Relationship to Stream Processors

**Stream processors do NOT use data contracts directly.** Instead, stream processors use templates that reference data models directly:

```yaml
# Template references model directly, not contract
templates:
  streamProcessors:
    pump_template:
      model:
        name: pump      # Direct model reference
        version: v1
      sources: {...}
      mapping: {...}

# Stream processor uses template
streamProcessors:
  - name: pump41_sp
    _templateRef: "pump_template"
    location: {...}
    variables: {...}
```

**Data contracts are separate** - they define storage bridges and retention policies for data models. Stream processors work with models directly through templates, but **if a data contract exists for the same model**, the stream processor's output will be automatically validated against that contract and routed to the contract's configured bridges.

## Related Documentation

- [Data Models](data-models.md) - Defining reusable data structures
- [Stream Processors](stream-processors.md) - Implementing contract instances
- [Stream Processor Implementation](../data-flows/stream-processor.md) - Detailed runtime configuration 