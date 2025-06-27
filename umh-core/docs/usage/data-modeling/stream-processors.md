# Stream Processors

> 🚧 **Roadmap Item** - Stream processors implement data contracts by transforming raw data into structured, validated information according to your data models.

Stream processors are the runtime components that bring data models and contracts to life. They consume raw data from your industrial systems, apply transformations according to your data models, and output structured data that complies with your data contracts.

## Overview

Stream processors bridge the gap between raw industrial data and structured business information:

- **Input**: Raw sensor data, PLC values, device messages
- **Processing**: Transformation, validation, contextualization  
- **Output**: Structured data compliant with data contracts
- **Storage**: Automatic routing to configured sinks

```yaml
streamprocessors:
  - name: processor_name
    contract: _contract_name:v1
    location:
      level0: enterprise
      level1: site
      level2: area
      level3: asset
    sources:
      var1: "umh.v1.path.to.raw.data"
    mapping:
      model_field: "var1 * 2"
```

## Key Concepts

### Contract Binding

Each stream processor implements exactly one data contract:

```yaml
streamprocessors:
  - name: pump41_sp
    contract: _pump:v1  # Binds to this contract
```

This binding ensures:
- Output data matches the contract's data model structure
- Data is routed to the contract's configured sinks
- Validation occurs against the contract's schema
- Retention policies are applied automatically

### Location Hierarchy

Stream processors define their position in the hierarchical organization (commonly based on ISA-95 but adaptable to KKS or custom naming standards). For complete hierarchy structure and rules, see [Topic Convention](../unified-namespace/topic-convention.md).

```yaml
location:
  level0: corpA        # Enterprise (mandatory)
  level1: plant-A      # Site/Region (optional)
  level2: line-4       # Area/Zone (optional)
  level3: pump41       # Work Unit (optional)
  level4: motor        # Work Center (optional)
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
  - name: Temperature
    version: v1
    structure:
      temperature_in_c:
        type: timeseries

# Data contract (from data-contracts.md)  
datacontracts:
  - name: _temperature
    version: v1
    model: Temperature:v1
    sinks:
      timescaledb: true

# Stream processor implementation
streamprocessors:
  - name: furnaceTemp_sp
    contract: _temperature:v1
    location:
      level0: corpA
      level1: plant-A
      level2: line-4
      level3: furnace1
    sources:
      tF: "umh.v1.corpA.plant-A.line-4.furnace1._raw.temperature_F"
    mapping:
      temperature_in_c: "(tF - 32) * 5 / 9"
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
    contract: _pump:v1
    location:
      level0: corpA
      level1: plant-A
      level2: line-4
      level3: pump41
    sources:
      p: "umh.v1.corpA.plant-A.line-4.pump41.deviceX._raw.press"
      tF: "umh.v1.corpA.plant-A.line-4.pump41.deviceX._raw.tempF"
      r: "umh.v1.corpA.plant-A.line-4.pump41.deviceX._raw.run"
      v: "umh.v1.corpA.plant-A.line-4.pump41.deviceX._raw.vib"
      c: "umh.v1.corpA.plant-A.line-4.pump41._raw.motor_current"
      s: "umh.v1.corpA.plant-A.line-4.pump41._raw.motor_speed"
      l1: "umh.v1.corpA.plant-A.line-4.pump41._raw.power_l1"
      l2: "umh.v1.corpA.plant-A.line-4.pump41._raw.power_l2"
    mapping:
      pressure: "p"
      temperature: "(tF - 32)*5/9"
      running: "r"
      diagnostics.vibration: "v"
      motor.current: "c"
      motor.rpm: "s"
      total_power: "l1 + l2"
      serial_number: "'SN-P41-007'"
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
- Output must match the bound data model structure
- Field types validated against payload shapes
- Constraint checking (min/max, allowed values)

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