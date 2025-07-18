# Data Models

> 🚧 **Roadmap Item** - Data models provide reusable, hierarchical data structures for industrial assets and processes.

Data models define the structure and schema of your industrial data. They act as reusable templates that can be instantiated across multiple assets, ensuring consistency and enabling powerful sub-model composition.

## Overview

Data models are stored in the `datamodels:` configuration section and define the logical structure of your data without specifying where it comes from or where it goes. They focus purely on the "what" - what fields exist, their types, and how they're organized.

```yaml
datamodels:
  pump:
    description: "pump from vendor ABC"
    versions:
      v1:
        structure:
          count:
            _payloadshape: timeseries-number
          serialNumber:
            _payloadshape: timeseries-string
```

## Payload Shapes

Payload shapes define the JSON schema for field values. They provide reusable templates for common data structures in industrial systems. For complete documentation on available payload shapes, their structure, and usage examples, see [Payload Shapes](payload-shapes.md)

Common payload shapes include:
- `timeseries-number`: For numeric sensor data
- `timeseries-string`: For textual data and status values

## Structure Elements

Data models support three types of structural elements:

### Fields

Fields represent actual data points and contain a `_payloadshape:` specification:

```yaml
pressure:
  _payloadshape: timeseries-number
```

> 🚧 **Roadmap Item** - Enhanced field metadata and constraints:

```yaml
pressure:
  _payloadshape: timeseries-number
  _meta: 
    description: "System pressure measurement"
    unit: "kPa"
  _constraints: 
    min: 0
    max: 1000
```

**Field Properties:**
- `_payloadshape`: Reference to a payload shape definition

> 🚧 **Roadmap Item** - Enhanced field metadata and constraints (only on leaf nodes):

- `_meta`: Optional metadata (description, units) - 🚧 **Roadmap Item**
- `_constraints`: Optional validation rules - 🚧 **Roadmap Item**

**Note:** Leaf nodes are fields that contain `_payloadshape` and represent actual data points, not organizational folders or sub-model references.

### Folders

Folders organize related fields without containing data themselves. They have no `_payloadshape:`, `_refModel:`, or `_meta:` properties:

```yaml
diagnostics:
  vibration:
    _payloadshape: timeseries-number
  temperature:
    _payloadshape: timeseries-number
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
    name: motor
    version: v1
```

This includes all fields from the referenced model:

```yaml
# Motor model definition
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

# Usage in Pump model
datamodels:
  pump:
    description: "Pump with motor sub-model"
    versions:
      v1:
        structure:
          pressure:
            _payloadshape: timeseries-number
          motor:
            _refModel:
              name: motor
              version: v1
```

## Complete Examples

### Simple Model

```yaml
datamodels:
  temperature:
    description: "Temperature sensor model"
    versions:
      v1:
        structure:
          value:
            _payloadshape: timeseries-number
```

### Complex Model with Sub-Models

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

  mitarbeiter:
    description: "irgendwas relational"
    versions:
      v1:
        structure:
          meldePerson:
            _payloadshape: relational-meldePerson
          setzeStatus:
            _payloadshape: relational-setzeStatus
          aendereStatus:
            _payloadshape: relational-aendereStatus

  motor-base:
    description: "basic motor component"
    versions:
      v1:
        structure:
          current:
            _payloadshape: timeseries-number
          rpm:
            _payloadshape: timeseries-number

  motor:
    description: "some motor"
    versions:
      v1:
        structure:
          engine:
            _refModel: 
              name: motor-base
              version: v1
```

## Metadata and Constraints

> 🚧 **Roadmap Item** - Enhanced field metadata and validation rules (only on leaf nodes):

**Important:** `_meta` and `_constraints` properties can only be applied to leaf nodes - fields that contain `_payloadshape` and represent actual data points. They cannot be applied to folder nodes or sub-model references.

### Valid Usage (Leaf Nodes)
```yaml
datamodels:
  pump:
    versions:
      v1:
        structure:
          # ✅ Valid - _meta on leaf node
          pressure:
            _payloadshape: timeseries-number
            _meta: 
              description: "System pressure measurement"
              unit: "kPa"
            _constraints: 
              min: 0
              max: 1000
          
          # ✅ Valid - _constraints on leaf node
          status:
            _payloadshape: timeseries-string
            _constraints: 
              allowed: ["running", "stopped", "fault"]
          
          # ✅ Valid - combined example
          z-axis:
            _payloadshape: timeseries-number
            _meta: # 🚧 **Roadmap Item**
              description: "Z-axis position measurement"
              unit: "m/s"
            _constraints: # 🚧 **Roadmap Item**
            max: 100
            min: 0
```

### Invalid Usage (Non-Leaf Nodes)
```yaml
datamodels:
  pump:
    versions:
      v1:
        structure:
          # ❌ Invalid - _meta on folder node
          diagnostics:
            _meta: # This is NOT allowed
              description: "Diagnostic data"
            vibration:
              _payloadshape: timeseries-number
          
          # ❌ Invalid - _meta on sub-model reference
          motor:
            _refModel:
              name: motor
              version: v1
            _meta: # This is NOT allowed
              description: "Motor component"
```

### Constraint Types

#### Numeric Constraints
```yaml
temperature:
  _payloadshape: timeseries-number
  _constraints: 
    min: -40
    max: 200
```

#### Enumerated Values
```yaml
status:
  _payloadshape: timeseries-string
  _constraints: 
    allowed: ["running", "stopped", "fault"]
```

#### Units and Metadata
```yaml
pressure:
  _payloadshape: timeseries-number
  _meta: 
    unit: "kPa"
    description: "System pressure measurement"
```

## Best Practices

### Model Organization
- **Keep models focused**: Each model should represent a single logical entity
- **Use sub-models for reusability**: Common components like motors, sensors
- **Version models explicitly**: Always specify version numbers
- **Use descriptive naming**: Clear, self-documenting field names

### Field Naming
- **Use camelCase**: `totalPower`, `serialNumber`
- **Be specific**: `temperatureInlet` vs. just `temperature`
- **Use clear references**: `_payloadshape` for payload shape

### Folder Structure
- **Group related fields**: All diagnostics under `diagnostics/`
- **Avoid deep nesting**: Keep hierarchy manageable (2-3 levels max)
- **Use logical grouping**: By function, not by data source

### Sub-Model Usage
- **Create reusable components**: Common equipment types
- **Version sub-models independently**: Allow evolution of components
- **Use clear references**: `_refModel` with explicit name and version

## Related Documentation

- [Payload Shapes](payload-shapes.md) - Reusable JSON schemas for field values
- [Data Contracts](data-contracts.md) - Binding models to storage and processing
- [Stream Processors](stream-processors.md) - Implementing model instances
- [Payload Formats](../unified-namespace/payload-formats.md) - UNS payload structure 