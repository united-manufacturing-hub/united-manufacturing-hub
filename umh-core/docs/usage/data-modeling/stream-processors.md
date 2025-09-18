# Stream Processors

> **Important**: Stream processors are for **[Silver → Gold](README.md#the-silver--gold-architecture) transformations only**. For device-specific modeling (Bronze → [Silver](README.md#silver-data)), use [bridges with data contracts](../data-flows/bridges.md) instead. Most users don't need stream processors.
>
> **Current Limitation**: Stream processors currently support [time-series output](../unified-namespace/payload-formats.md#time-series-data) only. [Relational output](../unified-namespace/payload-formats.md#relational-data) for [Gold-level](README.md#gold-data) business data is planned.

Stream processors transform [Silver data](README.md#silver-data) into [Gold](README.md#gold-data) by aggregating multiple sources and creating different views of existing UNS data.

## When to Use Stream Processors

| Need | Solution | Why |
|------|----------|-----|
| Structure device data | **Bridge** with data contract | Device modeling happens in bridges |
| Convert units (°F → °C) | **Bridge** with expression | Simple transformation, same model |
| Rename cryptic tags | **Bridge** with tag_processor | Device-level mapping |
| **Aggregate multiple devices** | **Stream Processor** | Cross-device, new model |
| **Create business KPIs** | **Stream Processor** | [Silver → Gold](README.md#the-silver--gold-architecture) transformation |
| **Generate different data views** | **Stream Processor** | Same data, new perspective |

## Overview

Stream processors create business-ready [Gold data](README.md#gold-data) from device-specific [Silver data](README.md#silver-data):

- **Input**: [Silver data](README.md#silver-data) from multiple devices (e.g., `_raw`, `_pump_v1`, `_temperature_v1`)
- **Processing**: Aggregation, calculation, business logic across devices
- **Output**: [Gold-level](README.md#gold-data) business data (e.g., `_workorder_v1`, `_maintenance_v1`)
- **Templates**: Reusable configurations with variable substitution
- **Auto-validation**: If data contracts exist for the model, output is automatically validated and routed to contract bridges

```yaml
# Template definition
templates:
  streamProcessors:
    motor_template:
      model:
        name: pump
        version: v1
      sources:               # alias → raw topic
        press: "${{ .location_path }}._raw.${{ .abc }}"
        tF: "${{ .location_path }}._raw.tempF"
        r: "${{ .location_path }}._raw.run"
      mapping:               # field → JS / constant / alias
        pressure: "press"
        temperature: "(tF-32)*5/9" # JavaScript expressions for data transformation
        running: "r"
        motor:
          rpm: "press"
        serialNumber: "${{ .sn }}"

# Stream processor instances
streamprocessors:
  - name: motor_assembly
    _templateRef: "motor_template"
    location:
      0: corpA
      1: plant-A
    variables:
      abc: "assembly"
      sn: "SN-P42-008"
  - name: motor_qualitycheck
    _templateRef: "motor_template"
    variables:
      abc: "qualitycheck"
      sn: "SN-P42-213"
```

## Key Concepts

### Template Reference

Each stream processor references a reusable template:

```yaml
streamprocessors:
  - name: pump41_sp
    _templateRef: "pump_template"
```

This template reference ensures:
- Output data matches the template's data model structure
- Validation occurs against the model's schema
- Template variables provide instance-specific configuration
- **Automatic integration**: If data contracts exist for the same model, output is automatically validated and routed to contract bridges

### Location Hierarchy

Stream processors define their position in the hierarchical organization (commonly based on ISA-95 but adaptable to KKS or custom naming standards). For complete hierarchy structure and rules, see [Topic Convention](../unified-namespace/topic-convention.md).

```yaml
location:
  0: corpA        # Enterprise (mandatory)
  1: plant-A      # Site/Region (optional)
  2: line-4       # Area/Zone (optional)
  3: pump41       # Work Unit (optional)
  4: motor        # Work Center (optional)
```

This creates UNS topics like:
```
umh.v1.corpA.plant-A.line-4.pump41._pump.pressure
```

### Data Sources

Stream processors subscribe to raw data topics:

```yaml
sources:
  press: "umh.v1.corpA.plant-A.line-4.pump41.deviceX._raw.press"
  temp: "umh.v1.corpA.plant-A.line-4.pump41.deviceX._raw.tempF"
  power1: "umh.v1.corpA.plant-A.line-4.pump41._raw.power_l1"
  power2: "umh.v1.corpA.plant-A.line-4.pump41._raw.power_l2"
```

### Field Mapping

Transform raw values into model fields using JavaScript expressions:

```yaml
mapping:
  pressure: "press"                    # Direct pass-through
  temperature: "(temp - 32) * 5 / 9"  # Fahrenheit to Celsius
  total_power: "power1 + power2"      # Derived calculation
  serialNumber: "'SN-P41-007'"       # Static metadata
```

## Simple Example

### Temperature Sensor

Transform Fahrenheit readings to Celsius:

```yaml
# Data model (from data-models.md)
datamodels:
  temperature:
    description: "Temperature sensor model"
    versions:
      v1:
        structure:
          temperatureInC:
            _payloadshape: timeseries-number

# Data contract (from data-contracts.md)
datacontracts:
  - name: _temperature_v1
    model:
      name: temperature
      version: v1

# Stream processor implementation
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

**Result:**
- Input: `1500°F` from PLC
- Output: `815.6°C` in structured format

## Validation and Error Handling

Stream processors provide built-in validation:

### Schema Validation
- Output must match the referenced data model structure
- Field types validated against payload shapes
- Constraint checking (min/max, allowed values)
- **Additional validation**: If data contracts exist for the model, output is automatically validated against contract requirements

### Runtime Validation
```yaml
# Invalid mapping - caught at startup
mapping:
  invalid_field: "someVar"  # Error: not defined in model

# Runtime error handling
mapping:
  temperature: "temp / 0"   # Expression error: skips message
```
