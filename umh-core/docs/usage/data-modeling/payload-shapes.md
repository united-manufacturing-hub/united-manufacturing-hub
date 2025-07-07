# Payload Shapes

> 🚧 **Roadmap Item** - Payload shapes define reusable JSON schemas for field values in industrial data models.

Payload shapes define the JSON schema for field values. They provide reusable templates for common data structures in industrial systems, ensuring consistency across your data models.

## Overview

Payload shapes are stored in the `payloadshapes:` configuration section and define the structure of data that flows through your UNS topics. They use `_type:` to define the types of fields within the payload shape structure:

```yaml
payloadshapes:
  timeseries-number:
    fields:
      timestamp_ms:
        _type: number
      value:
        _type: number
  timeseries-string:
    fields:
      timestamp_ms:
        _type: number
      value:
        _type: string
```

## Built-in Payload Shapes

### Timeseries Number

The default payload shape for numeric sensor data:

```yaml
payloadshapes:
  timeseries-number:
    fields:
      timestamp_ms:
        _type: number
      value:
        _type: number
```

**Example payload:**
```json
{
  "value": 42.5,
  "timestamp_ms": 1733904005123
}
```

**Common use cases:**
- Temperature measurements
- Pressure readings
- RPM values
- Current measurements
- Power consumption

### Timeseries String

For textual sensor data and status values:

```yaml
payloadshapes:
  timeseries-string:
    fields:
      timestamp_ms:
        _type: number
      value:
        _type: string
```

**Example payload:**
```json
{
  "value": "running",
  "timestamp_ms": 1733904005123
}
```

**Common use cases:**
- Equipment status ("running", "stopped", "fault")
- Serial numbers
- Product codes
- Error messages
- Operator notes

## Custom Payload Shapes

You can define custom payload shapes for specific use cases:

```yaml
payloadshapes:
  vibration-analysis:
    fields:
      timestamp_ms:
        _type: number
      frequency_hz:
        _type: number
      amplitude:
        _type: number
      phase_degrees:
        _type: number
      equipment_id:
        _type: string
```

## Usage in Data Models

Payload shapes are referenced in data models using the `_payloadshape:` property:

```yaml
datamodels:
  pump:
    description: "Pump with various measurements"
    versions:
      v1:
        root:
          pressure:
            _payloadshape: timeseries-number
          status:
            _payloadshape: timeseries-string
          vibration:
            _payloadshape: vibration-analysis
```

## Type System

The `_type:` field (used within payload shapes) can reference:

### Basic Types
- `number`: Numeric values (integers, floats)
- `string`: Text values
- `boolean`: True/false values
- `array`: Lists of values

### Payload Shape References
- `_type:` can also reference other payload shapes
- Enables composition and reusability

```yaml
payloadshapes:
  composite-measurement:
    fields:
      primary:
        _type: timeseries-number
      secondary:
        _type: timeseries-string
      metadata:
        _type: string
```

## Relational Payload Shapes

> 🚧 **Roadmap Item** - Relational payload shapes for complex relational data:

```yaml
payloadshapes:
  relational-employee:
    fields:
      employee_id:
        _type: string
      first_name:
        _type: string
      last_name:
        _type: string
      department:
        _type: string
      health_metrics:
        pulse:
          value:
            _type: number
          measured_at:
            _type: number
```

## Best Practices

### Naming Convention
- **Use hyphenated names**: `timeseries-number`, `batch-report`
- **Be descriptive**: `vibration-analysis` vs. `measurement`
- **Include data type**: `timeseries-number` vs. just `timeseries`

### Design Principles
- **Keep shapes focused**: Each shape should serve a specific purpose
- **Favor composition**: Use existing shapes as building blocks
- **Plan for evolution**: Consider future field additions
- **Document use cases**: Clear examples of when to use each shape

### Field Organization
- **Group related fields**: Logical field grouping within shapes
- **Use consistent naming**: `timestamp_ms` not `time` or `ts`
- **Include required metadata**: Timestamp fields for time-series data

## Validation and Schema Registry

All payload shapes are automatically registered in the Redpanda Schema Registry, providing:

- **Runtime validation**: Messages validated against payload shape schemas
- **Type safety**: Ensures data consistency across the system
- **Schema evolution**: Controlled versioning for payload shapes
- **Cross-system compatibility**: Shared schema definitions

## Related Documentation

- [Data Models](data-models.md) - Using payload shapes in data models
- [Data Contracts](data-contracts.md) - Storage and retention policies
- [Stream Processors](stream-processors.md) - Processing data with payload shapes
- [Payload Formats](../unified-namespace/payload-formats.md) - UNS payload structure 