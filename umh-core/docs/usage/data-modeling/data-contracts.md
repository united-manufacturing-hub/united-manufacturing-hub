# Data Contracts

> 🚧 **Roadmap Item** - Data contracts bind data models to storage, retention, and processing policies, ensuring consistent data handling across your industrial systems.

Data contracts define the operational aspects of your data models - where data gets stored, how long it's retained, and what processing rules apply. They bridge the gap between logical data structure (models) and physical data management.

## Overview

Data contracts are stored in the `datacontracts:` configuration section and reference specific versions of data models:

```yaml
datacontracts:
  - name: contract_name
    version: v1
    model: ModelName:v1
    sinks:
      timescaledb: true
    retention_days: 365
```

## Core Properties

### Name and Versioning

```yaml
datacontracts:
  - name: _temperature
    version: v1
    model: Temperature:v1
```

**Naming Convention:**
- Contract names start with underscore (`_temperature`, `_pump`)
- Versions follow semantic versioning (`v1`, `v2`, etc.)
- Model references include version (`Temperature:v1`)

### Model Binding

Each contract binds to exactly one data model version:

```yaml
datacontracts:
  - name: _pump
    version: v1
    model: Pump:v1  # Specific model version
```

This binding is immutable - to change the model, create a new contract version.

### Data Sinks

Contracts specify where data gets stored and processed:

```yaml
datacontracts:
  - name: _temperature
    version: v1
    model: Temperature:v1
    sinks:
      timescaledb: true
      custom_analytics: false
      cloud_storage: true
```

**Available Sinks:**
- `timescaledb`: Automatic TimescaleDB hypertable creation
- `custom_dfc`: Custom data flow configurations
- `cloud_storage`: S3-compatible storage
- `analytics_pipeline`: Stream analytics processing

### Retention Policies

Define how long data is kept:

```yaml
datacontracts:
  - name: _historian
    version: v1
    model: HistorianData:v1
    sinks:
      timescaledb: true
    retention_days: 2555  # ~7 years
```

## Complete Examples

### Simple Temperature Contract

```yaml
datamodels:
  - name: Temperature
    version: v1
    structure:
      temperature_in_c:
        type: timeseries
        constraints:
          unit: "°C"

datacontracts:
  - name: _temperature
    version: v1
    model: Temperature:v1
    sinks:
      timescaledb: true
    retention_days: 365
```

### Complex Pump Contract

```yaml
datamodels:
  - name: Motor
    version: v1
    structure:
      current:
        type: timeseries
      rpm:
        type: timeseries

  - name: Pump
    version: v1
    structure:
      pressure:
        type: timeseries
        constraints:
          unit: kPa
          min: 0
      temperature:
        type: timeseries
        constraints:
          unit: "°C"
      running:
        type: timeseries
        constraints:
          allowed: [true, false]
      diagnostics:
        vibration:
          type: timeseries
          constraints:
            unit: "mm/s"
      motor:
        _model: Motor:v1
      total_power:
        type: timeseries
        constraints:
          unit: kW
      serial_number:
        type: timeseries

datacontracts:
  - name: _pump
    version: v1
    model: Pump:v1
    sinks:
      timescaledb: true
      analytics_pipeline: true
    retention_days: 1825  # 5 years
```

## Generated Database Schema

When a contract with TimescaleDB sink is deployed, UMH automatically creates:

### Hypertable Structure

For the `_pump:v1` contract:

```sql
-- Auto-generated hypertable: pump_v1
CREATE TABLE pump_v1 (
    time TIMESTAMPTZ NOT NULL,
    location JSONB NOT NULL,
    
    -- Top-level fields
    pressure DOUBLE PRECISION,
    temperature DOUBLE PRECISION,
    running BOOLEAN,
    total_power DOUBLE PRECISION,
    serial_number TEXT,
    
    -- Sub-model fields (flattened)
    motor_current DOUBLE PRECISION,
    motor_rpm DOUBLE PRECISION,
    
    -- Folder fields (flattened with path)
    diagnostics_vibration DOUBLE PRECISION
);

-- Hypertable conversion
SELECT create_hypertable('pump_v1', 'time');

-- Indexes for location-based queries
CREATE INDEX idx_pump_v1_location ON pump_v1 USING GIN (location);
```

### Location Structure

The `location` field stores ISA-95 hierarchy:

```json
{
  "level0": "corpA",
  "level1": "plant-A", 
  "level2": "line-4",
  "level3": "pump41"
}
```

## Contract Evolution

### Version Management

Contracts support controlled evolution:

```yaml
# Version 1
datacontracts:
  - name: _pump
    version: v1
    model: Pump:v1
    sinks:
      timescaledb: true

# Version 2 - Extended retention
datacontracts:
  - name: _pump
    version: v2
    model: Pump:v2  # Updated model
    sinks:
      timescaledb: true
      analytics_pipeline: true  # New sink
    retention_days: 2555  # Extended retention
```

### Backward Compatibility

- Multiple contract versions can coexist
- Existing stream processors continue using their bound contract version
- Database schemas adapt automatically for new fields
- No downtime required for contract updates

## Sink Configuration Details

### TimescaleDB Sink

```yaml
sinks:
  timescaledb: true
```

**Behavior:**
- Auto-creates hypertable `{contract_name}_{version}`
- Generates appropriate column types from model constraints
- Creates location indexes for ISA-95 queries
- Handles sub-model field flattening automatically

### Custom Data Flow Sink

```yaml
sinks:
  custom_dfc:
    endpoint: "https://analytics.company.com/api/pump-data"
    auth_token: "${DFC_AUTH_TOKEN}"
    batch_size: 1000
```

**Behavior:**
- Sends data to external systems
- Supports authentication and batching
- Configurable retry policies
- Schema validation before transmission

### Cloud Storage Sink

```yaml
sinks:
  cloud_storage:
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
Topic: umh.v1.corpA.plant-A.line-4.pump41._pump.invalid_field
Reason: Field 'invalid_field' not defined in Pump:v1 model

# Valid message - accepted
Topic: umh.v1.corpA.plant-A.line-4.pump41._pump.pressure
Payload: {"value": 42.5, "timestamp_ms": 1733904005123}
```

## Best Practices

### Contract Design

- **Single responsibility**: One contract per logical entity type
- **Semantic naming**: Use descriptive, underscore-prefixed names
- **Version explicitly**: Always specify model and contract versions
- **Plan for growth**: Consider future sink requirements

### Retention Planning

- **Match business needs**: Align retention with regulatory requirements
- **Consider storage costs**: Balance retention vs. storage expenses
- **Plan for archival**: Design archival strategies for historical data

### Sink Selection

- **TimescaleDB for time-series**: Optimal for sensor data and analytics
- **Cloud storage for archives**: Long-term, cost-effective storage
- **Custom DFC for integration**: External system connectivity

### Schema Evolution

- **Additive changes**: Add fields rather than modifying existing ones
- **Test compatibility**: Validate schema evolution before deployment
- **Document changes**: Maintain clear change logs

## Integration with Stream Processors

Data contracts are consumed by stream processors:

```yaml
streamprocessors:
  - name: pump41_sp
    contract: _pump:v1  # References this contract
    location:
      level0: corpA
      level1: plant-A
      level2: line-4
      level3: pump41
    # ... mapping configuration
```

The stream processor:
1. Validates output against the contract's model schema
2. Routes data to configured sinks
3. Applies retention policies automatically
4. Enforces location hierarchy requirements

## Related Documentation

- [Data Models](data-models.md) - Defining reusable data structures
- [Stream Processors](stream-processors.md) - Implementing contract instances
- [Stream Processor Implementation](../data-flows/stream-processor-upcoming.md) - Detailed runtime configuration 