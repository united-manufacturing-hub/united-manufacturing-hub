# Data Models

> 🚧 **Roadmap Item** - Data models provide reusable, hierarchical data structures for industrial assets and processes.

Data models define the structure and schema of your industrial data. They act as reusable templates that can be instantiated across multiple assets, ensuring consistency and enabling powerful sub-model composition.

## Overview

Data models are stored in the `datamodels:` configuration section and define the logical structure of your data without specifying where it comes from or where it goes. They focus purely on the "what" - what fields exist, their types, and how they're organized.

```yaml
datamodels:
  - name: ModelName
    versions:
      v1:
        description: "Description of the data model"
        structure:
          # Field and folder definitions
```

## Structure Elements

Data models support three types of structural elements:

### Fields (Leaf Nodes)

Fields represent actual data points and always contain a `_type:` specification:

```yaml
pressure:
  _type: timeseries-number
  _unit: kPa
  _description: "System pressure measurement"
```

**Field Properties:**
- `_type`: Data type (e.g., `timeseries-string`, `timeseries-number`, `relational-[payload-shape]`)
- `_unit`: Optional unit specification
- `_description`: Optional field description

### Folders

Folders organize related fields without containing data themselves. They contain subfields but have no `_type:` or `_refModel:` properties:

```yaml
diagnostics:
  vibration:
    _type: timeseries-number
    _unit: "mm/s"
  temperature:
    _type: timeseries-number
    _unit: "°C"
```

Folders create hierarchical organization in your UNS topics:
```
umh.v1.corpA.plant-A.line-4.pump41._pump.diagnostics.vibration
umh.v1.corpA.plant-A.line-4.pump41._pump.diagnostics.temperature
```

### Sub-Models

Sub-models enable composition by referencing other data models. They contain only a `_refModel:` property:

```yaml
motor:
  _refModel: Motor:v1
```

This includes all fields from the referenced model:

```yaml
# Motor:v1 definition
datamodels:
  - name: Motor
    versions:
      v1:
        description: "Electric motor data model"
        structure:
          current:
            _type: timeseries-number
            _unit: A
          rpm:
            _type: timeseries-number
            _unit: rpm

# Usage in Pump model
datamodels:
  - name: Pump
    versions:
      v1:
        description: "Pump with motor sub-model"
        structure:
          pressure:
            _type: timeseries-number
            _unit: kPa
          motor:
            _refModel: Motor:v1  # Includes current and rpm fields
```

## Validation Rules

Data model structures are validated to ensure consistency and prevent common errors. The following rules are enforced:

### Model Reference Format
- `_refModel` must contain exactly one colon (`:`)
- `_refModel` must have a version specified after the colon
- Version must match the pattern `^v\d+$` (e.g., `v1`, `v2`, `v10`)

```yaml
# ✅ Valid
motor:
  _refModel: Motor:v1

# ❌ Invalid - no colon
motor:
  _refModel: Motor-v1

# ❌ Invalid - multiple colons  
motor:
  _refModel: Motor:Sub:v1

# ❌ Invalid - empty version
motor:
  _refModel: Motor:

# ❌ Invalid - wrong version format
motor:
  _refModel: Motor:version1
```

### Node Type Rules

#### Leaf Nodes
- Must contain `_type`
- Must NOT contain `_refModel`
- May contain `_description` and `_unit`

```yaml
# ✅ Valid leaf node
temperature:
  _type: timeseries-number
  _unit: "°C"
  _description: "Temperature measurement"
```

#### Folder Nodes
- Contain subfields (nested structure)
- Must NOT contain `_refModel`
- May contain `_type`, `_description`, and `_unit`

```yaml
# ✅ Valid folder node
diagnostics:
  vibration:
    _type: timeseries-number
    _unit: "mm/s"
  temperature:
    _type: timeseries-number
    _unit: "°C"
```

#### Sub-Model Nodes
- Must contain ONLY `_refModel`
- Must NOT contain `_type`, `_description`, `_unit`, or subfields

```yaml
# ✅ Valid sub-model node
motor:
  _refModel: Motor:v1

# ❌ Invalid - contains additional fields
motor:
  _refModel: Motor:v1
  _description: "This is not allowed"
```

### Invalid Combinations
- Cannot have both `_type` and `_refModel`
- Cannot have both `subfields` and `_refModel`

```yaml
# ❌ Invalid - both _type and _refModel
field:
  _type: timeseries-string
  _refModel: Motor:v1

# ❌ Invalid - both subfields and _refModel  
field:
  _refModel: Motor:v1
  nested_field:
    _type: timeseries-number
```

## Payload Shapes

Payload shapes define the structure and type of data that fields can contain. The `_type` field in data models specifies which payload shape to use:

### Available Types

UMH Core supports the following data types:

#### Timeseries Types
For time-series sensor and measurement data:
- `timeseries-number`: Numeric values (integers or floats)
- `timeseries-string`: Text values

#### Relational Types
For structured data following specific payload shapes:
- `relational-[payload-shape]`: Relational data that follows the specified payload shape

```yaml
# Examples of different data types
pressure:
  _type: timeseries-number
  _unit: kPa

status:
  _type: timeseries-string

batch_report:
  _type: relational-batch_report
```

**Example payloads:**

Timeseries data:
```json
// timeseries-number
{
  "value": 42.5,
  "timestamp_ms": 1733904005123
}

// timeseries-string  
{
  "value": "running",
  "timestamp_ms": 1733904005123
}
```

Relational data (depends on payload shape):
```json
// relational-batch_report example
{
  "batch_id": "BATCH-2024-001",
  "start_time": 1733904005123,
  "end_time": 1733907605123,
  "quality_metrics": {
    "yield": 95.2,
    "defect_rate": 0.8
  }
}
```

### Payload Shape Definitions

Payload shapes are defined separately in the configuration and referenced by data models. For relational data, you can define custom payload shapes with JSON schemas that specify the exact structure of your data.

## Complete Examples

### Simple Model

```yaml
datamodels:
  - name: Temperature
    versions:
      v1:
        description: "Temperature sensor data model"
        structure:
          value:
            _type: timeseries-number
            _unit: "°C"
            _description: "Temperature measurement"
```

### Complex Model with Sub-Models

```yaml
datamodels:
  - name: Motor
    versions:
      v1:
        description: "Electric motor data model"
        structure:
          current:
            _type: timeseries-number
            _unit: A
          rpm:
            _type: timeseries-number
            _unit: rpm
          status:
            _type: timeseries-string
            _description: "Motor status (running, stopped, fault)"

  - name: Pump
    versions:
      v1:
        description: "Pump with diagnostics and motor"
        structure:
          pressure:
            _type: timeseries-number
            _unit: kPa
          temperature:
            _type: timeseries-number
            _unit: "°C"
          running:
            _type: timeseries-string
            _description: "Running status"
          diagnostics:
            vibration:
              _type: timeseries-number
              _unit: "mm/s"
            wear_level:
              _type: timeseries-number
              _unit: "%"
          motor:
            _refModel: Motor:v1
          total_power:
            _type: timeseries-number
            _unit: kW
          serial_number:
            _type: timeseries-string
            _description: "Equipment serial number"
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