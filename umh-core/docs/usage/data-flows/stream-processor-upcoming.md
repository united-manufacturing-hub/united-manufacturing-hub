# Stream Processor Implementation

> 🚧 **Roadmap Item** - Stream processors transform messages already inside the Unified Namespace using the unified data-modelling system.

Stream processors implement the runtime execution of [data models and contracts](../data-modeling/README.md), providing real-time data transformation and contextualization within the UNS. This document covers the technical implementation details, configuration syntax, and Management Console interface.

## Overview

Stream processors consume messages from the UNS, apply transformations according to data models, and republish structured data that complies with data contracts. They bridge raw industrial data and business-ready information.

**Key Features:**
- **Real-time data contextualization** - Transform raw sensor data into business metrics
- **Data aggregation** - Combine multiple data streams into unified models
- **Schema enforcement** - Validate against data contracts automatically
- **Stream joins** - Correlate data across different devices/systems
- **JavaScript expressions** - Flexible transformation logic

## Architecture Integration

Stream processors integrate with the unified data-modelling system through templates:

```yaml
# Complete configuration example
payloadshapes:
  timeseries-number:
    fields:
      timestamp_ms:
        _type: number
      value:
        _type: number

datamodels:
  pump:
    description: "Pump with motor sub-model"
    versions:
      v1:
        root:
          pressure:
            _payloadshape: timeseries-number
          motor:
            _refModel:
              name: motor
              version: v1

datacontracts:
  - name: _pump_v1
    model:
      name: pump
      version: v1
    default_bridges:
      - type: timescaledb
        retention_in_days: 365

templates:
  streamProcessors:
    pump_template:
      model:
        name: pump
        version: v1
      sources:
        press: "${{ .location_path }}._raw.${{ .pressure_sensor }}"
        temp: "${{ .location_path }}._raw.tempF"
      mapping:
        dynamic:
          pressure: "press"
          temperature: "(temp-32)*5/9"
        static:
          serial_number: "${{ .sn }}"

streamprocessors:
  - name: pump41_sp
    _templateRef: "pump_template"
    location:
      0: corpA
      1: plant-A
      2: line-4
      3: pump41
    variables:
      pressure_sensor: "pressure"
      sn: "SN-P41-007"
```

All components are registered in Redpanda Schema Registry at boot, ensuring the UNS output plugin rejects non-compliant messages.

## Configuration Syntax

### Template Definition

Templates provide reusable configurations with variable substitution:

```yaml
templates:
  streamProcessors:
    template_name:
      model:
        name: model_name
        version: v1
      sources:
        var_name: "${{ .location_path }}._raw.${{ .variable_name }}"
      mapping:
        dynamic:
          model_field: "javascript_expression"
          folder:
            sub_field: "javascript_expression"
        static:
          metadata_field: "${{ .variable_name }}"
```

### Stream Processor Definition

```yaml
streamprocessors:
  - name: processor_name
    _templateRef: "template_name"     # Required: references template
    location:                        # Hierarchical organization (ISA-95, KKS, or custom)
      0: enterprise                  # Level 0 (mandatory)
      1: site                        # Level 1 (optional)
      2: area                        # Level 2 (optional)
      3: work_unit                   # Level 3 (optional)
      4: work_center                 # Level 4 (optional)
    variables:                       # Template variable substitution
      variable_name: "value"
```

### Location Hierarchy

Stream processors define their position in the hierarchical organization (commonly based on ISA-95 but adaptable to KKS or custom naming standards). For complete hierarchy structure and rules, see [Topic Convention](../unified-namespace/topic-convention.md).

```yaml
location:
  0: corpA        # Enterprise (mandatory)
  1: plant-A      # Site/Region (optional)  
  2: line-4       # Area/Zone (optional)
  3: pump42       # Work Unit (optional)
  4: motor1       # Work Center (optional)
```

This creates UNS topics following the standard convention:
```
umh.v1.{0}.{1}.{2}.{3}[.{4}].{contract}.{field_path}
```

### Template Variables

Templates support variable substitution for flexible, reusable configurations:

```yaml
templates:
  streamProcessors:
    sensor_template:
      model:
        name: temperature
        version: v1
      sources:
        temp: "${{ .location_path }}._raw.${{ .sensor_name }}"
      mapping:
        dynamic:
          temperature_in_c: "(temp - 32) * 5 / 9"
        static:
          sensor_id: "${{ .sensor_id }}"
          location: "${{ .location_description }}"
```

**Built-in Variables:**
- `${{ .location_path }}`: Auto-generated from location hierarchy (e.g., `umh.v1.corpA.plant-A.line-4.pump41`)

**Custom Variables:**
- Define in `variables:` section of stream processor
- Reference in templates using `${{ .variable_name }}`

### Field Mapping

Transform source variables into model fields using two mapping types:

#### Dynamic Mapping
JavaScript expressions evaluated at runtime:

```yaml
mapping:
  dynamic:
    # Direct pass-through
    pressure: "press"
    
    # Unit conversions
    temperature: "(temp - 32) * 5 / 9"
    
    # Calculations
    total_power: "l1 + l2 + l3"
    
    # Conditional logic
    status: "temp > 100 ? 'hot' : 'normal'"
    
    # Sub-model fields (nested YAML)
    motor:
      current: "motor_current_var"
      rpm: "motor_speed_var"
    
    # Folder fields
    diagnostics:
      vibration: "vibration_var"
```

#### Static Mapping
Template variables resolved at deployment time:

```yaml
mapping:
  static:
    serial_number: "${{ .sn }}"
    firmware_version: "${{ .firmware_ver }}"
    installation_date: "${{ .install_date }}"
```

## Complete Examples

### Minimal Temperature Sensor

```yaml
payloadshapes:
  timeseries-number:
    fields:
      timestamp_ms:
        _type: number
      value:
        _type: number

datamodels:
  temperature:
    description: "Temperature sensor model"
    versions:
      v1:
        root:
          temperature_in_c:
            _payloadshape: timeseries-number

datacontracts:
  - name: _temperature_v1
    model:
      name: temperature
      version: v1
    default_bridges:
      - type: timescaledb
        retention_in_days: 365

templates:
  streamProcessors:
    temperature_template:
      model:
        name: temperature
        version: v1
      sources:
        temp: "${{ .location_path }}._raw.${{ .temp_sensor }}"
      mapping:
        dynamic:
          temperature_in_c: "(temp - 32) * 5 / 9"
        static:
          sensor_id: "${{ .sn }}"

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
- **UNS Topic**: `umh.v1.corpA.plant-A.line-4.furnace1._temperature_v1.temperature_in_c`
- **Payload**: `{"value": 815.6, "timestamp_ms": 1733904005123}`
- **Database**: Auto-created hypertable `temperature_v1`

### Complex Pump with Motor Sub-Model

```yaml
datamodels:
  motor:
    description: "Standard motor model"
    versions:
      v1:
        root:
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
        root:
          pressure:
            _payloadshape: timeseries-number
          temperature:
            _payloadshape: timeseries-number
          running:
            _payloadshape: timeseries-string
          diagnostics:
            vibration:
              _payloadshape: timeseries-number
          motor:
            _refModel:
              name: motor
              version: v1
          total_power:
            _payloadshape: timeseries-number
          serial_number:
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

templates:
  streamProcessors:
    pump_template:
      model:
        name: pump
        version: v1
      sources:
        press: "${{ .location_path }}._raw.${{ .pressure_sensor }}"
        temp: "${{ .location_path }}._raw.tempF"
        run: "${{ .location_path }}._raw.running"
        vib: "${{ .location_path }}._raw.vibration"
        current: "${{ .location_path }}._raw.motor_current"
        rpm: "${{ .location_path }}._raw.motor_speed"
        l1: "${{ .location_path }}._raw.power_l1"
        l2: "${{ .location_path }}._raw.power_l2"
      mapping:
        dynamic:
          pressure: "press"
          temperature: "(temp - 32) * 5 / 9"
          running: "run"
          diagnostics:
            vibration: "vib"
          motor:
            current: "current"
            rpm: "rpm"
          total_power: "l1 + l2"
        static:
          serial_number: "${{ .sn }}"

streamprocessors:
  - name: pump41_sp
    _templateRef: "pump_template"
    location:
      0: corpA
      1: plant-A
      2: line-4
      3: pump41
    variables:
      pressure_sensor: "pressure"
      sn: "SN-P41-007"

  - name: pump42_sp
    _templateRef: "pump_template"
    location:
      0: corpA
      1: plant-A
      2: line-4
      3: pump42
    variables:
      pressure_sensor: "press_sensor"
      sn: "SN-P42-008"
```

**Generated Infrastructure:**

| Component | Count | Description |
|-----------|-------|-------------|
| Data Model | 1 (pump:v1) | Reusable across instances |
| Data Contract | 1 (_pump_v1) | Shared configuration |
| Template | 1 (pump_template) | Reusable processor configuration |
| Stream Processors | 2 (pump41_sp, pump42_sp) | Asset-specific instances |
| TimescaleDB Tables | 1 (pump_v1) | Shared storage |

## Validation and Error Handling

Stream processors provide built-in validation at multiple levels:

### Template Validation
- Model references must exist
- Variable syntax must be valid
- Mapping expressions validated for syntax

### Deployment Validation
- All template variables must be provided
- Location hierarchy must be valid
- Source topics must be resolvable

### Runtime Validation
```yaml
# Error scenarios and handling
mapping:
  dynamic:
    invalid_field: "someVar"  # Error: not defined in model
    temperature: "temp / 0"   # Runtime error: skips message
```

### Error Scenarios
- **Unknown model fields**: Processor fails to start
- **Missing template variables**: Processor fails to start  
- **Expression errors**: Message skipped, logged
- **Undefined expression results**: Message skipped

## Management Console

> 🚧 **Roadmap Item** - The Management Console provides a visual interface for creating and managing stream processors.

### Console Workflow

The Management Console follows familiar UMH patterns for configuration:

#### 1. Navigate to Stream Processors
```
Data Flows > Stream Processors > + Add Stream Processor
```

#### 2. Template Selection

```
Select Template                                   [Next]
────────────────────────────────────────────────────────────────
Template          Model         Description
────────────────────────────────────────────────────────────────
○ pump_template   pump:v1       Pump with motor sub-model
○ temp_template   temperature:v1 Temperature sensor
○ motor_template  motor:v1      Standard motor
────────────────────────────────────────────────────────────────
```

#### 3. Configuration Panel

```
Stream Processor (pump42_sp)                    [Deploy]
────────────────────────────────────────────────────────────────
1) General
   Name              pump42_sp
   Template          pump_template (pump:v1)

2) Location
   0: corpA     1: plant-A
   2: line-4    3: pump42    4: (blank)

3) Template Variables
────────────────────────────────────────────────────────────────
Variable           Value
────────────────────────────────────────────────────────────────
pressure_sensor    press_sensor
sn                 SN-P42-008
────────────────────────────────────────────────────────────────

4) Preview Generated Sources          📂 Tag Browser
────────────────────────────────────────────────────────────────
Source    Resolved Topics
────────────────────────────────────────────────────────────────
press     umh.v1.corpA.plant-A.line-4.pump42._raw.press_sensor
temp      umh.v1.corpA.plant-A.line-4.pump42._raw.tempF
run       umh.v1.corpA.plant-A.line-4.pump42._raw.running
────────────────────────────────────────────────────────────────

5) Preview Generated Output Topics
────────────────────────────────────────────────────────────────
umh.v1.corpA.plant-A.line-4.pump42._pump_v1.pressure
umh.v1.corpA.plant-A.line-4.pump42._pump_v1.temperature
umh.v1.corpA.plant-A.line-4.pump42._pump_v1.motor.current
────────────────────────────────────────────────────────────────
[YAML Preview ▼]
```

#### 4. Interactive Features

**Template-Driven Interface:**
- Pre-configured field mappings from template
- Required variables clearly indicated
- Live validation of variable values

**Tag Browser Integration:**
- Click 📂 to verify source topics exist
- Auto-complete topic paths
- Real-time topic availability checking

**Live Preview:**
- Show resolved source topics
- Preview generated output topics
- Validate template variable substitution

## Best Practices

### Template Design
- **Reusable logic**: Design templates for equipment classes, not individual assets
- **Meaningful variables**: Use descriptive variable names that make sense across instances
- **Consistent naming**: Follow naming conventions across all templates

### Stream Processor Configuration
- **Descriptive names**: Use clear, asset-specific processor names
- **Complete variables**: Provide all required template variables
- **Test incrementally**: Deploy one processor at a time for validation

### Performance Optimization
- **Efficient expressions**: Keep JavaScript simple and fast
- **Minimize sources**: Only subscribe to needed topics
- **Batch deployments**: Deploy related processors together

## Related Documentation

For complete conceptual understanding:
- [Data Modeling Overview](../data-modeling/README.md) - Architectural concepts
- [Stream Processors](../data-modeling/stream-processors.md) - Template concepts and usage
- [Data Models](../data-modeling/data-models.md) - Structure and schema design
- [Data Contracts](../data-modeling/data-contracts.md) - Storage and retention policies

For integration and context:
- [Unified Namespace](../unified-namespace/README.md) - Topic conventions and payload formats
- [Data Flows Overview](overview.md) - Integration with other flow types

