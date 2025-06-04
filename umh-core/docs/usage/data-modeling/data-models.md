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

Fields represent actual data points and always contain a `type:` specification:

```yaml
pressure:
  type: timeseries
  constraints:
    unit: kPa
    min: 0
    max: 1000
```

**Field Properties:**
- `type`: Payload shape (see [Payload Shapes](#payload-shapes))
- `constraints`: Optional validation rules (unit, min, max, allowed values)

### Folders

Folders organize related fields without containing data themselves. They have no `type:` or `_model:` properties:

```yaml
diagnostics:
  vibration:
    type: timeseries
    constraints:
      unit: "mm/s"
  temperature:
    type: timeseries
    constraints:
      unit: "°C"
```

Folders create hierarchical organization in your UNS topics:
```
umh.v1.corpA.plant-A.line-4.pump41._pump.diagnostics.vibration
umh.v1.corpA.plant-A.line-4.pump41._pump.diagnostics.temperature
```

### Sub-Models

Sub-models enable composition by referencing other data models. They contain a `_model:` property:

```yaml
motor:
  _model: Motor:v1
```

This includes all fields from the referenced model:

```yaml
# Motor:v1 definition
datamodels:
  - name: Motor
    version: v1
    structure:
      current:
        type: timeseries
        constraints:
          unit: A
      rpm:
        type: timeseries
        constraints:
          unit: rpm

# Usage in Pump model
datamodels:
  - name: Pump
    version: v1
    structure:
      pressure:
        type: timeseries
      motor:
        _model: Motor:v1  # Includes current and rpm fields
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
        type: timeseries
        constraints:
          unit: "°C"
          min: -40
          max: 200
```

### Complex Model with Sub-Models

```yaml
datamodels:
  - name: Motor
    version: v1
    structure:
      current:
        type: timeseries
        constraints:
          unit: A
      rpm:
        type: timeseries
        constraints:
          unit: rpm
      status:
        type: timeseries
        constraints:
          allowed: ["running", "stopped", "fault"]

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
        wear_level:
          type: timeseries
          constraints:
            unit: "%"
            min: 0
            max: 100
      motor:
        _model: Motor:v1
      total_power:
        type: timeseries
        constraints:
          unit: kW
      serial_number:
        type: timeseries  # Static metadata
```

## Validation Rules

Data models support various constraint types:

### Numeric Constraints
```yaml
temperature:
  type: timeseries
  constraints:
    unit: "°C"
    min: -40
    max: 200
```

### Enumerated Values
```yaml
status:
  type: timeseries
  constraints:
    allowed: ["running", "stopped", "fault"]
```

### Boolean Constraints
```yaml
running:
  type: timeseries
  constraints:
    allowed: [true, false]
```

### Units and Metadata
```yaml
pressure:
  type: timeseries
  constraints:
    unit: kPa
    description: "System pressure measurement"
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

### Folder Structure
- **Group related fields**: All diagnostics under `diagnostics/`
- **Avoid deep nesting**: Keep hierarchy manageable (2-3 levels max)
- **Use logical grouping**: By function, not by data source

### Sub-Model Usage
- **Create reusable components**: Common equipment types
- **Version sub-models independently**: Allow evolution of components
- **Document dependencies**: Clear which models depend on others

## Schema Registry Integration

All data models are automatically registered in the Redpanda Schema Registry at boot time, enabling:

- **Runtime validation**: Messages are validated against schemas
- **Schema evolution**: Controlled versioning and compatibility
- **Cross-system consistency**: Shared schema definitions

## Related Documentation

- [Data Contracts](data-contracts.md) - Binding models to storage and processing
- [Stream Processors](stream-processors.md) - Implementing model instances
- [Payload Formats](../unified-namespace/payload-formats.md) - UNS payload structure 