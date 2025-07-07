# Data Models

> 🚧 **Roadmap Item** - Data models provide reusable, hierarchical data structures for industrial assets and processes.

Data models define the structure and schema of your industrial data. They act as reusable templates that can be instantiated across multiple assets, ensuring consistency and enabling powerful sub-model composition.

## Overview

Data models are stored in the `datamodels:` configuration section and define the logical structure of your data without specifying where it comes from or where it goes. They focus purely on the "what" - what fields exist, their types, and how they're organized.

```yaml
datamodels:
  - name: ModelName
    version: v1
    structure:
      # Field and folder definitions
```

## Structure Elements

Data models support three types of structural elements:

### Fields

Fields represent actual data points and always contain a `_type:` specification:

```yaml
pressure:
  _type: timeseries-number
```

**Field Properties:**

- `_type`: Payload shape (see [Payload Shapes](#payload-shapes))

### Folders

Folders organize related fields without containing data themselves. They have no `_type:` or `_refModel:` properties:

```yaml
diagnostics:
  vibration:
    _type: timeseries-number
  temperature:
    _type: timeseries-number
```

Folders create hierarchical organization in your UNS topics:

```
umh.v1.corpA.plant-A.line-4.pump41._pump.diagnostics.vibration
umh.v1.corpA.plant-A.line-4.pump41._pump.diagnostics.temperature
```

### Sub-Models

Sub-models enable composition by referencing other data models. They contain a `_refModel:` property:

```yaml
motor:
  _refModel:
    name: Motor
    version: v1
```

This includes all fields from the referenced model:

```yaml
# Motor:v1 definition
datamodels:
  - name: Motor
    version: v1
    structure:
      current:
        _type: timeseries-number
      rpm:
        _type: timeseries-number

# Usage in Pump model
datamodels:
  - name: Pump
    version: v1
    structure:
      pressure:
        _type: timeseries-number
      motor:
        _refModel:
          name: Motor
          version: v1  # Includes current and rpm fields
```

## Payload Shapes

Payload shapes define the JSON schema for field values. UMH Core includes built-in shapes:

### Timeseries (Default)

The default payload shape for sensor data:

```yaml
# Schema definition
payloadshapes:
  - name: timeseries
    version: v1
    jsonschema: |
      {
        "type": "object",
        "properties": {
          "value": {"type": ["number", "string", "boolean"]},
          "timestamp_ms": {"type": "integer"}
        },
        "required": ["value", "timestamp_ms"]
      }
```

**Example payload:**

```json
{
  "value": 42.5,
  "timestamp_ms": 1733904005123
}
```

### Binary Blob

For file data or binary content:

```yaml
payloadshapes:
  - name: blob
    version: v1
    jsonschema: |
      {
        "type": "string",
        "contentEncoding": "base64"
      }
```

### Custom Payload Shapes

You can define custom shapes for complex data:

```yaml
payloadshapes:
  - name: batch_report
    version: v1
    jsonschema: |
      {
        "type": "object",
        "properties": {
          "batch_id": {"type": "string"},
          "start_time": {"type": "integer"},
          "end_time": {"type": "integer"},
          "quality_metrics": {
            "type": "object",
            "properties": {
              "yield": {"type": "number"},
              "defect_rate": {"type": "number"}
            }
          }
        },
        "required": ["batch_id", "start_time"]
      }
```

## Complete Examples

### Simple Model

```yaml
datamodels:
  - name: Temperature
    version: v1
    structure:
      value:
        _type: timeseries-number
```

### Complex Model with Sub-Models

```yaml
datamodels:
  - name: Motor
    version: v1
    structure:
      current:
        _type: timeseries-number
      rpm:
        _type: timeseries-number
      status:
        _type: timeseries-string

  - name: Pump
    version: v1
    structure:
      pressure:
        _type: timeseries-number
      temperature:
        _type: timeseries-number
      running:
        _type: timeseries-boolean
      diagnostics:
        vibration:
          _type: timeseries-number
        wear_level:
          _type: timeseries-number
      motor:
        _refModel:
          name: Motor
          version: v1
      total_power:
        _type: timeseries-number
      serial_number:
        _type: timeseries-string
```

## Best Practices

### Model Organization

- **Keep models focused**: Each model should represent a single logical entity
- **Use sub-models for reusability**: Common components like motors, sensors
- **Version models explicitly**: Always specify version numbers
- **Use descriptive naming**: Clear, self-documenting field names

### Field Naming

- **Use snake_case**: `total_power`, not `totalPower` or `Total-Power`
- **Be specific**: `temperature_inlet` vs. just `temperature`
- **Include units in names when helpful**: `temperature_c`, `pressure_kpa`

### Field Configuration

- **Always specify \_type**: Every field must have a payload shape

### Folder Structure

- **Group related fields**: All diagnostics under `diagnostics/`
- **Avoid deep nesting**: Keep hierarchy manageable (2-3 levels max)
- **Use logical grouping**: By function, not by data source

### Sub-Model Usage

- **Create reusable components**: Common equipment types
- **Version sub-models independently**: Allow evolution of components
- **Document dependencies**: Clear which models depend on others
- **Use object syntax**: `_refModel:` with `name:` and `version:` properties

## Schema Registry Integration

All data models are automatically registered in the Redpanda Schema Registry at boot time, enabling:

- **Runtime validation**: Messages are validated against schemas
- **Schema evolution**: Controlled versioning and compatibility
- **Cross-system consistency**: Shared schema definitions

## Related Documentation

- [Data Contracts](data-contracts.md) - Binding models to storage and processing
- [Stream Processors](stream-processors.md) - Implementing model instances
- [Payload Formats](../unified-namespace/payload-formats.md) - UNS payload structure
