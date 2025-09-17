# Data Models

Data models provide reusable, hierarchical data structures for industrial assets and processes.

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

**Field Properties:**
- `_payloadshape`: Reference to a payload shape definition

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
