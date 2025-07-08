# Stream Processors

> 🚧 **Roadmap Item** - Stream processors transform raw data into structured, validated information according to your data models using reusable template configurations.

Stream processors are the runtime components that bring data models to life. They consume raw data from your industrial systems, apply transformations according to your data models, and output structured data. Stream processors work directly with data models through templates, not through data contracts.

## Overview

Stream processors bridge the gap between raw industrial data and structured business information:

- **Input**: Raw sensor data, PLC values, device messages
- **Processing**: Transformation, validation, contextualization according to data models
- **Output**: Structured data compliant with data models
- **Templates**: Reusable configurations with variable substitution
- **Auto-validation**: If data contracts exist for the model, output is automatically validated and routed to contract bridges

```yaml
# Template definition
templates:
  streamProcessors:
    pump_template:
      model:
        name: pump
        version: v1
      sources:
        press: "${{ .location_path }}._raw.${{ .abc }}"
        tF: "${{ .location_path }}._raw.tempF"
        r: "${{ .location_path }}._raw.run"
      mapping:
        dynamic:
          pressure: "press"
          temperature: "(tF-32)*5/9"
          running: "r"
        static:
          serialNumber: "${{ .sn }}"

# Stream processor instance
streamprocessors:
  - name: pump_assembly
    _templateRef: "pump_template"
    location:
      0: corpA
      1: plant-A
    variables:
      abc: "assembly"
      sn: "SN-P42-008"
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
  serial_number: "'SN-P41-007'"       # Static metadata
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
        root:
          temperature_in_c:
            _payloadshape: timeseries-number

# Data contract (from data-contracts.md)  
datacontracts:
  - name: _temperature_v1
    model:
      name: temperature
      version: v1
    default_bridges:
      - type: timescaledb
        retention_in_days: 365

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
- Storage: Auto-created TimescaleDB hypertable

## Complex Example

### Pump with Motor Sub-Model

```yaml
# Stream processor for pump with motor sub-model
streamprocessors:
  - name: pump41_sp
    _templateRef: "pump_template"
    location:
      0: corpA
      1: plant-A
      2: line-4
      3: pump41
    variables:
      abc: "deviceX"
      sn: "SN-P41-007"
```

**Generated Topics:**
```
umh.v1.corpA.plant-A.line-4.pump41._pump.pressure
umh.v1.corpA.plant-A.line-4.pump41._pump.temperature
umh.v1.corpA.plant-A.line-4.pump41._pump.running
umh.v1.corpA.plant-A.line-4.pump41._pump.diagnostics.vibration
umh.v1.corpA.plant-A.line-4.pump41._pump.motor.current
umh.v1.corpA.plant-A.line-4.pump41._pump.motor.rpm
umh.v1.corpA.plant-A.line-4.pump41._pump.total_power
umh.v1.corpA.plant-A.line-4.pump41._pump.serial_number
```

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

### Error Scenarios
- **Unknown fields**: Processor fails to start
- **Missing variables**: Processor fails to start  
- **Expression errors**: Message skipped, logged
- **Undefined expression results**: Message skipped

## Deployment and Management

Stream processors are deployed through:

1. **YAML Configuration**: Direct configuration files
2. **Management Console**: Web-based interface (recommended)
3. **API**: Programmatic deployment

### Management Console Workflow

For detailed information on using the Management Console interface, see:

**[Stream Processor Implementation → Management Console](../data-flows/stream-processor-upcoming.md#management-console)**

The console provides:
- Visual data model builder
- Interactive mapping configuration
- Tag browser for source selection
- Live validation and preview
- One-click deployment

## Operational Benefits

### Automatic Infrastructure
- **Database tables**: Auto-created from data models
- **Schema registry**: Models registered automatically
- **Validation pipelines**: Generated from contracts
- **Monitoring**: Built-in performance metrics

### Scalability
- **Multiple instances**: Same model deployed across assets
- **Shared infrastructure**: One contract, many processors
- **Resource efficiency**: Optimized processing pipelines

### Maintainability  
- **Version management**: Controlled model/contract evolution
- **Configuration as code**: YAML-based, version-controlled
- **Centralized validation**: Consistent across all processors

## Best Practices

### Naming
- **Descriptive names**: `pump41_sp`, `furnace_temp_sp`
- **Include location**: Reference the asset/location
- **Consistent suffix**: Use `_sp` for stream processors

### Source Management
- **Meaningful variable names**: `press`, `temp`, not `var1`, `var2`
- **Full topic paths**: Avoid ambiguity with complete UNS paths
- **Document complex sources**: Comment unusual data sources

### Mapping Logic
- **Keep expressions simple**: Complex logic should be in separate steps
- **Use appropriate precision**: Match industrial data accuracy
- **Handle edge cases**: Consider sensor failure scenarios
- **Document calculations**: Comment unit conversions and formulas

## Related Documentation

For complete implementation details, configuration syntax, and Management Console usage:

**[Data Flows → Stream Processor Implementation](../data-flows/stream-processor-upcoming.md)**

Additional references:
- [Data Models](data-models.md) - Defining data structures
- [Data Contracts](data-contracts.md) - Storage and retention policies
- [Unified Namespace](../unified-namespace/README.md) - Topic structure and conventions 