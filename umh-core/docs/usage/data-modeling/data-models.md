# Data Models

Data models define the **virtual topic hierarchy** that appears after the data contract in your UNS topics. They transform flat data into organized, hierarchical structures.

## How Models Create Topic Structure

Data models literally become the topic path segments after your data contract:

```
umh.v1.<location_path>.<data_contract>.[<virtual_path>].<name>
                                         ↑________________↑
                                    This part comes from your model structure
```

For example, a model with this structure:
```yaml
structure:
  motor:           # Creates virtual path segment
    current:       # Becomes a topic accepting data
    rpm:           # Becomes a topic accepting data
  diagnostics:     # Creates virtual path segment
    vibration:     # Becomes a topic accepting data
```

Creates these exact topics (assuming contract `_pump_v1`):
- `umh.v1.plant.line1._pump_v1.motor.current`
- `umh.v1.plant.line1._pump_v1.motor.rpm`
- `umh.v1.plant.line1._pump_v1.diagnostics.vibration`

## How Models Work

Data models are stored in the `datamodels:` configuration section in the [config.yaml](../../reference/configuration-reference.md):

```yaml
datamodels:
  - name: pump
    description: "pump from vendor ABC"
    version:
      v1:
        structure:
          pressure:
            _payloadshape: timeseries-number
          status:
            _payloadshape: timeseries-string
```

### Important Concepts

1. **Models are templates** - They don't enforce anything by themselves
2. **Data contracts enforce models** - Only when a contract references a model does validation occur
3. **Models apply to all locations** - Once defined, the same virtual structure works everywhere
4. **Structure becomes topics** - The hierarchy you define becomes your actual topic paths

See [Topic Convention](../unified-namespace/topic-convention.md) for complete topic structure details.

## Payload Shapes

Payload shapes define the JSON schema for field values. They provide reusable templates for common data structures in industrial systems. For complete documentation on available payload shapes, their structure, and usage examples, see [Payload Shapes](payload-shapes.md)

Common payload shapes include:
- `timeseries-number`: For numeric sensor data
- `timeseries-string`: For textual data and status values

## Three Types of Structure Elements

Data models use three building blocks to create your topic hierarchy:

### 1. Fields - Data Endpoints

Fields are the actual data points that accept messages. They must reference a payload shape:

```yaml
pressure:
  _payloadshape: timeseries-number  # This field accepts time-series data
```

**What fields do:**
- Create the final topic segment that accepts data
- Define what payload format is expected (via `_payloadshape`)
- Become the `tag_name` in your topic path

**Example**: The field `pressure` creates topic ending `...pressure` that accepts `{"timestamp_ms": 123, "value": 42.5}`

### 2. Folders - Virtual Organization

Folders create hierarchy without being data points themselves. They have no special properties:

```yaml
diagnostics:              # This is a folder (no _payloadshape)
  vibration:             # This is a field
    _payloadshape: timeseries-number
  temperature:           # This is a field
    _payloadshape: timeseries-number
```

**What folders do:**
- Create virtual path segments in your topic structure
- Organize related fields together
- Make topics more readable and logical

**Result**: Creates topics with `diagnostics` as a path segment:
- `umh.v1.plant._pump_v1.diagnostics.vibration`
- `umh.v1.plant._pump_v1.diagnostics.temperature`

### 3. Sub-Models - Composition and Reuse

Sub-models let you include another model's entire structure:

```yaml
motor:
  _refModel:
    name: motor
    version: v1
```

**What sub-models do:**
- Include all fields and folders from another model
- Enable reuse across different equipment types
- Maintain consistency for common components

**Example**: Define motor once, use in pump, conveyor, mixer models:

```yaml
# Define motor model once
dataModels:
  - name: motor
    description: "Standard motor"
    version:
      v1:
        structure:
          current:
            _payloadshape: timeseries-number
          rpm:
            _payloadshape: timeseries-number

# Reuse in pump model
dataModels:
  - name: pump
    description: "Pump with motor"
    version:
      v1:
        structure:
          pressure:
            _payloadshape: timeseries-number
          motor:              # Includes all motor fields
            _refModel:
              name: motor
              version: v1
```

**Result**: Creates topics:
- `umh.v1.plant._pump_v1.pressure`
- `umh.v1.plant._pump_v1.motor.current`
- `umh.v1.plant._pump_v1.motor.rpm`

## Working Examples

### Time-Series Model (Works with Stream Processors)

```yaml
dataModels:
  - name: temperature-sensor
    description: "Simple temperature sensor"
    version:
      v1:
        structure:
          temperature:
            _payloadshape: timeseries-number
```

Used with data contract `_temperature-sensor_v1`, creates topic:
- `umh.v1.factory.line._temperature-sensor_v1.temperature`

### Relational Model (Requires nodered_js)

```yaml
dataModels:
  - name: machine-state
    description: "Machine state tracking"
    version:
      v1:
        structure:
          update:
            _payloadshape: machine-state-update  # Custom payload shape
```

Used with data contract `_machine-state_v1`, creates topic:
- `umh.v1.factory.line._machine-state_v1.update`

### Complex Equipment Model

```yaml
dataModels:
  - name: cnc-machine
    description: "CNC with multiple subsystems"
    version:
      v1:
        structure:
          status:                    # Field at root level
            _payloadshape: timeseries-string
          spindle:                   # Folder for organization
            rpm:
              _payloadshape: timeseries-number
            load:
              _payloadshape: timeseries-number
          axes:                      # Another folder
            x_position:
              _payloadshape: timeseries-number
            y_position:
              _payloadshape: timeseries-number
            z_position:
              _payloadshape: timeseries-number
```

Creates this topic structure (with contract `_cnc-machine_v1`):
- `umh.v1.factory.machining._cnc-machine_v1.status`
- `umh.v1.factory.machining._cnc-machine_v1.spindle.rpm`
- `umh.v1.factory.machining._cnc-machine_v1.spindle.load`
- `umh.v1.factory.machining._cnc-machine_v1.axes.x_position`
- `umh.v1.factory.machining._cnc-machine_v1.axes.y_position`
- `umh.v1.factory.machining._cnc-machine_v1.axes.z_position`

## Key Takeaways

1. **Your model structure = Your topic structure** - Design it like you'd design a folder hierarchy
2. **Models need contracts to enforce** - Without a data contract, models are just documentation
3. **One model, many locations** - The same model structure works across all your factories
4. **Time-series vs Relational** - Choose payload shapes based on your processing needs
