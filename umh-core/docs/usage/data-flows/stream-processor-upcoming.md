
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

Stream processors integrate with the unified data-modelling system:

```yaml
# Complete configuration example
payloadshapes:
  - name: timeseries
    version: v1
    jsonschema: '{"type":"object","properties":{"value":{"type":["number","string","boolean"]},"timestamp_ms":{"type":"integer"}},"required":["value","timestamp_ms"]}'

datamodels:
  - name: Pump
    version: v1
    structure:
      pressure:
        type: timeseries
        constraints: { unit: kPa, min: 0 }
      motor:
        _model: Motor:v1

datacontracts:
  - name: _pump
    version: v1
    model: Pump:v1
    sinks:
      timescaledb: true

streamprocessors:
  - name: pump41_sp
    contract: _pump:v1
    location:
      level0: corpA
      level1: plant-A
      level2: line-4
      level3: pump41
    sources:
      p: "umh.v1.corpA.plant-A.line-4.pump41._raw.pressure"
    mapping:
      pressure: "p"
```

All components are registered in Redpanda Schema Registry at boot, ensuring the UNS output plugin rejects non-compliant messages.

## Configuration Syntax

### Stream Processor Definition

```yaml
streamprocessors:
  - name: processor_name
    desiredState: active          # active | inactive
    contract: contract_name:v1    # Required: binds to data contract
    location:                     # ISA-95 hierarchy
      level0: enterprise
      level1: site
      level2: area
      level3: work_unit
      level4: work_center         # Optional
    sources:                      # Variable definitions
      var_name: "full.uns.topic.path"
    mapping:                      # Field transformations
      model_field: "javascript_expression"
```

### Location Hierarchy (ISA-95)

Defines the asset position in your enterprise hierarchy:

```yaml
location:
  level0: corpA        # Enterprise/Corporation
  level1: plant-A      # Site/Facility
  level2: line-4       # Area/Production Line
  level3: pump41       # Work Unit/Equipment
  level4: motor1       # Work Center/Component (optional)
```

**Generated UNS Topics:**
```
umh.v1.{level0}.{level1}.{level2}.{level3}[.{level4}].{contract}.{field_path}
```

### Source Variables

Map UNS topics to variables for use in expressions:

```yaml
sources:
  # Simple variables
  temp: "umh.v1.corpA.plant-A.line-4.furnace1._raw.temperature_F"
  press: "umh.v1.corpA.plant-A.line-4.pump41.deviceX._raw.pressure"
  
  # Power measurements
  l1: "umh.v1.corpA.plant-A.line-4.pump41._raw.power_l1"
  l2: "umh.v1.corpA.plant-A.line-4.pump41._raw.power_l2"
  l3: "umh.v1.corpA.plant-A.line-4.pump41._raw.power_l3"
```

**Variable Naming:**
- Use descriptive but concise names
- Avoid conflicts with JavaScript keywords
- Consider using prefixes for related variables

### Field Mapping

Transform source variables into model fields using JavaScript expressions:

```yaml
mapping:
  # Direct pass-through
  pressure: "press"
  
  # Unit conversions
  temperature: "(temp - 32) * 5 / 9"
  
  # Calculations
  total_power: "l1 + l2 + l3"
  power_average: "(l1 + l2 + l3) / 3"
  
  # Conditional logic
  status: "temp > 100 ? 'hot' : 'normal'"
  
  # Sub-model fields (dot notation)
  motor.current: "motor_current_var"
  motor.rpm: "motor_speed_var"
  
  # Folder fields
  diagnostics.vibration: "vibration_var"
  
  # Static constants
  serial_number: "'SN-P41-007'"
  firmware_version: "'v2.1.4'"
```

## Mapping Rules Reference

### Expression Evaluation

- **Language**: JavaScript (Node-RED JS engine)
- **Context**: Latest value of each source variable
- **Return**: Value matching field's payload shape
- **Evaluation**: On every source variable update

### Field Path Rules

| Pattern | Description | Example |
|---------|-------------|---------|
| `field` | Top-level field | `pressure: "press"` |
| `folder.field` | Field inside folder | `diagnostics.vibration: "vib"` |
| `submodel.field` | Field in sub-model | `motor.current: "current"` |

### Static Values

For metadata or constants, use empty sources:

```yaml
sources: {}  # No live topics
mapping:
  serial_number: "'SN-PUMP-001'"
  manufacturer: "'Acme Corp'"
  installation_date: "'2024-01-15'"
```

### Error Handling

| Scenario | Behavior |
|----------|----------|
| Unknown field in mapping | Processor fails to start |
| Missing source variable | Processor fails to start |
| JavaScript syntax error | Processor fails to start |
| Runtime expression error | Message skipped, logged |
| Expression returns `undefined` | Message skipped |

## Complete Examples

### Minimal Temperature Sensor

```yaml
datamodels:
  - name: Temperature
    version: v1
    structure:
      temperature_in_c:
        type: timeseries

datacontracts:
  - name: _temperature
    version: v1
    model: Temperature:v1
    sinks:
      timescaledb: true

streamprocessors:
  - name: furnaceTemp_sp
    desiredState: active
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
- **UNS Topic**: `umh.v1.corpA.plant-A.line-4.furnace1._temperature.temperature_in_c`
- **Payload**: `{"value": 815.6, "timestamp_ms": 1733904005123}`
- **Database**: Auto-created hypertable `temperature_v1`

### Complex Pump with Motor Sub-Model

```yaml
datamodels:
  - name: Motor
    version: v1
    structure:
      current: { type: timeseries }
      rpm: { type: timeseries }

  - name: Pump
    version: v1
    structure:
      pressure:
        type: timeseries
        constraints: { unit: kPa, min: 0 }
      temperature:
        type: timeseries
        constraints: { unit: "°C" }
      running:
        type: timeseries
        constraints: { allowed: [true, false] }
      diagnostics:
        vibration:
          type: timeseries
          constraints: { unit: "mm/s" }
      motor:
        _model: Motor:v1
      total_power:
        type: timeseries
        constraints: { unit: kW }
      serial_number:
        type: timeseries

datacontracts:
  - name: _pump
    version: v1
    model: Pump:v1
    sinks:
      timescaledb: true

streamprocessors:
  - name: pump41_sp
    desiredState: active
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

  - name: pump42_sp
    desiredState: active
    contract: _pump:v1
    location:
      level0: corpA
      level1: plant-A
      level2: line-4
      level3: pump42
    sources:
      # Same source structure but different paths
      p: "umh.v1.corpA.plant-A.line-4.pump42.deviceX._raw.press"
      # ... other sources for pump42
    mapping:
      # Same mapping except different serial number
      pressure: "p"
      # ... other mappings
      serial_number: "'SN-P42-008'"
```

**Generated Infrastructure:**

| Component | Count | Description |
|-----------|-------|-------------|
| Data Model | 1 (Pump:v1) | Reusable across instances |
| Data Contract | 1 (_pump:v1) | Shared configuration |
| Stream Processors | 2 (pump41_sp, pump42_sp) | Asset-specific instances |
| TimescaleDB Tables | 1 (pump_v1) | Shared storage |
| Protocol Converter Write | 1 | Auto-generated sink handler |

## Management Console

> 🚧 **Roadmap Item** - The Management Console provides a visual interface for creating and managing stream processors.

### Console Workflow

The Management Console follows familiar UMH patterns for configuration:

#### 1. Navigate to Stream Processors
```
Data Flows > Stream Processors > + Add Stream Processor
```

#### 2. Single-Page Configuration Panel

```
Stream Processor (pump42_sp)                    [Deploy]
────────────────────────────────────────────────────────────────
1) General
   Name              pump42_sp
   Desired State     Active
   Contract          _pump:v1 ▼     (model implied: Pump:v1)

2) Location (ISA-95)
   level0: corpA     level1: plant-A
   level2: line-4    level3: pump42    level4: (blank)

3) Sources                      📂 Tag Browser
────────────────────────────────────────────────────────────────
Source    Topics
────────────────────────────────────────────────────────────────
press     umh.v1.corpA…deviceX._raw.press
tF        …deviceX._raw.tempF
r         …deviceX._raw.run
c         …pump42._raw.motor_current
s         …pump42._raw.motor_speed
l1        …pump42._raw.power_l1
l2        …pump42._raw.power_l2
────────────────────────────────────────────────────────────────

4) Apply Model
────────────────────────────────────────────────────────────────
Target Field       JS Expression
────────────────────────────────────────────────────────────────
pressure           press
temperature        (tF-32)*5/9
running            r
motor.current      c
motor.rpm          s
total_power        l1+l2
serial_number      'SN-P42-008'
────────────────────────────────────────────────────────────────
[+ Add Row]    [YAML Preview ▼]
```

#### 3. Interactive Features

**Tag Browser Integration:**
- Click 📂 to open tag browser
- Select from live UNS topics
- Auto-suggest variable names
- Full topic path verification

**Live Validation:**
- Expression syntax checking
- Model field validation
- Real-time error highlighting
- Preview generated topics

**YAML Preview:**
- Show generated `streamprocessors:` block
- Copy for version control
- Validate against schema

#### 4. Deployment Result

After clicking **Deploy**:
- FSM validates configuration
- Benthos container starts processing
- Status updates to 🟢 Running
- Generated topics appear in Tag Browser

### Console Integration

**Data Flows Overview:**
```
Data Flows (Stream Processors)
────────────────────────────────────────────────────────────────
Name          Contract     State    TPS    Location
────────────────────────────────────────────────────────────────
pump41_sp     _pump:v1     🟢Run    1.2    corpA.plant-A.line-4.pump41
pump42_sp     _pump:v1     🟢Run    1.1    corpA.plant-A.line-4.pump42
furnaceTemp   _temp:v1     🟢Run    0.8    corpA.plant-A.line-4.furnace1
```

**Tag Browser Integration:**
```
umh.v1
 └ corpA.plant-A.line-4.pump42
    └ _pump
       ├ pressure
       ├ temperature
       ├ running
       ├ diagnostics
       │   └ vibration
       ├ motor
       │   ├ current
       │   └ rpm
       ├ total_power
       └ serial_number
```

**Live Data Verification:**
- Click any field to see live values
- Right sidebar shows: "Stored in TimescaleDB table pump_v1 • Retention: 365d"
- SQL sample queries for data access

## Why This Meets All Requirements

The unified data-modelling approach addresses key manufacturing requirements:

### 1. Generic ISA-95 Support
- Built-in `level0-4` location hierarchy
- Automatic UNS topic generation
- Hierarchical database storage

### 2. Single YAML Dialect
- Unified configuration language
- Pass-through and derived transformations
- No mode-switching complexity

### 3. Folders & Sub-Models Unified
- Consistent `structure` handling
- Dot-notation field access
- Reusable component modeling

### 4. Per-Field Constraints
- Built-in validation (unit, min, max)
- Type enforcement
- Enumerated value support

### 5. Static Constants Support
- Empty source expressions
- Metadata injection
- Configuration-driven constants

### 6. Simplified Event Model
- Default "evaluate on every update"
- No complex event handling
- Predictable processing behavior

### 7. Succinct Contracts
- YAML flow-style compatible
- Clear model binding
- Explicit sink configuration

### 8. Full UNS Topic Paths
- No ambiguous references
- Complete topic specification
- Clear data lineage

## Best Practices

### Configuration Management
- **Version control**: Store YAML configurations in Git
- **Environment-specific**: Use environment variables for deployment-specific values
- **Validation**: Test configurations in development before production

### Performance Optimization
- **Minimize sources**: Only subscribe to needed topics
- **Efficient expressions**: Keep JavaScript simple and fast
- **Batch similar processors**: Group related transformations

### Monitoring and Maintenance
- **Monitor TPS**: Track messages per second for performance
- **Error logging**: Set up alerting for processing errors
- **Schema evolution**: Plan model updates carefully

### Security
- **Topic authorization**: Restrict access to sensitive source topics
- **Expression validation**: Review JavaScript expressions for security
- **Schema validation**: Use schema registry for data integrity

## Related Documentation

For conceptual understanding and model design:
- [Data Modeling Overview](../data-modeling/README.md) - Architectural concepts
- [Data Models](../data-modeling/data-models.md) - Structure and schema design
- [Data Contracts](../data-modeling/data-contracts.md) - Storage and retention policies

For integration and context:
- [Unified Namespace](../unified-namespace/README.md) - Topic conventions and payload formats
- [Data Flows Overview](overview.md) - Integration with other flow types
- [Configuration Reference](../../reference/configuration-reference.md) - Complete syntax reference

