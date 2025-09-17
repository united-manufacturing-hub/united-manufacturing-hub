# Payload Shapes

Payload shapes define the JSON schema for data in the UNS. They ensure type safety and consistency across your industrial data infrastructure.

## Overview

UMH uses two fundamental payload formats (see [Payload Formats](../unified-namespace/payload-formats.md) for details):

1. **Time-series format**: For individual sensor values (one tag, one message, one topic)
2. **Relational format**: For complex business records and multi-field data

Payload shapes provide the schema definitions for these formats.

## Built-in Shapes (Always Available)

> **💡 Important**: These shapes are automatically provided by UMH. You don't need to define them in `config.yaml` - they're always available for use in your data models.

### timeseries-number

The default shape for numeric sensor data. Automatically used when processing tags with `tag_processor`.

**Schema:**
```yaml
fields:
  timestamp_ms:
    _type: number  # Integer or float without fraction
  value:
    _type: number  # Any numeric value
```

**Example payload:**
```json
{
  "timestamp_ms": 1733904005123,
  "value": 42.5
}
```

**Validation Example:**

If you have a data model using `timeseries-number`:
```yaml
dataModels:
  - name: temperature-sensor
    version:
      v1:
        structure:
          temperature:
            _payloadshape: timeseries-number
```

✅ **Valid message** - accepted:
```
Topic: umh.v1.plant._temperature-sensor.temperature
Payload: {"timestamp_ms": 1733904005123, "value": 42.5}
```

❌ **Invalid messages** - rejected:
```
Topic: umh.v1.plant._temperature-sensor.temp
Reason: Field 'temp' not defined in model - only 'temperature' exists

Topic: umh.v1.plant._temperature-sensor.temperature  
Payload: {"timestamp": 1733904005123, "value": 42.5}
Reason: Wrong field name - expected 'timestamp_ms', got 'timestamp'

Topic: umh.v1.plant._temperature-sensor.temperature
Payload: {"timestamp_ms": 1733904005123, "value": 42.5, "unit": "celsius"}
Reason: Extra field 'unit' - timeseries allows only timestamp_ms and value
```

### timeseries-string

The default shape for textual data and status values. Automatically used for string tags with `tag_processor`.

**Schema:**
```yaml
fields:
  timestamp_ms:
    _type: number
  value:
    _type: string  # Text, status, or identifier
```

**Example payload:**
```json
{
  "timestamp_ms": 1733904005123,
  "value": "running"
}
```

## Custom Payload Shapes (Relational Data)

For data that doesn't fit the time-series model (machine states, batch records, alarms, orders), define custom shapes in your `config.yaml`.

> **Note**: Custom shapes are currently configured via `config.yaml`. UI support is planned for future releases.

### When to Use Custom Shapes

Use custom shapes when you need:
- Multiple related fields in a single message
- Complex state updates (not just simple values)
- Business records (orders, batches, quality reports)
- Event data with multiple attributes

### How to Define Custom Shapes

Add your shapes to `/data/config.yaml` under the `payloadShapes` section:

```yaml
payloadShapes:
  # Example: Machine state updates
  machine-state-update:
    description: "Machine state change event"
    fields:
      asset_id:
        _type: string
      updated_by:
        _type: string
      machine_state_ts:
        _type: string
      update_state:
        _type: number
      updated_ts:
        _type: string
      schema:
        _type: string
```

### Using Custom Shapes in Data Models

Reference your custom shape in a data model:

```yaml
dataModels:
  - name: machine-state
    description: "Machine state tracking"
    version:
      v1:
        structure:
          update:
            _payloadshape: machine-state-update
```

### Validation with Custom Shapes

When a data contract exists for your custom shape, the UNS enforces validation:

```yaml
dataContracts:
  - name: _machine-state
    model:
      name: machine-state
      version: v1
```

✅ **Valid message** - accepted:
```
Topic: umh.v1.plant.line1._machine-state.update
Payload: {
  "asset_id": "MACHINE_001",
  "updated_by": "operator",
  "machine_state_ts": "2024-01-01T10:00:00Z",
  "update_state": 1,
  "updated_ts": "2024-01-01T10:00:00Z",
  "schema": "v1.0"
}
```

❌ **Invalid messages** - rejected:
```
Topic: umh.v1.plant.line1._machine-state.status
Reason: Field 'status' not defined in model - only 'update' exists

Topic: umh.v1.plant.line1._machine-state.update
Payload: {
  "machine_id": "MACHINE_001",  // Wrong field name
  "updated_by": "operator",
  ...
}
Reason: Field 'machine_id' not in shape - expected 'asset_id'

Topic: umh.v1.plant.line1._machine-state.update
Payload: {
  "asset_id": "MACHINE_001",
  "updated_by": "operator"
  // Missing required fields
}
Reason: Missing required fields: machine_state_ts, update_state, updated_ts, schema
```

### Processing Custom Shapes

Custom shapes require the `nodered_js` processor in Protocol Converters:

```yaml
templates:
  protocolConverter:
    machine-state-generator:
      dataflowcomponent_read:
        benthos:
          pipeline:
            processors:
              - nodered_js:
                  code: |
                    // Set required UNS metadata
                    msg.meta.location_path = "{{ .location_path }}";
                    msg.meta.data_contract = "_machine-state";
                    msg.meta.tag_name = "update";
                    msg.meta.umh_topic = "umh.v1.{{ .location_path }}._machine-state.update";
                    
                    // Build payload matching your custom shape
                    msg.payload = {
                      "asset_id": "MACHINE_001",
                      "updated_by": "system",
                      "machine_state_ts": new Date().toISOString(),
                      "update_state": 1,
                      "updated_ts": new Date().toISOString(),
                      "schema": "v1.0"
                    };
                    
                    return msg;
```

## Type System

Within payload shapes, the `_type` field defines the data type:

### Basic Types
- `number`: Numeric values (integers or floats)
- `string`: Text values
- `boolean`: True/false values (planned)

## Complete Working Example

Here's a full example showing a custom payload shape for quality inspection data:

```yaml
# In /data/config.yaml

payloadShapes:
  quality-inspection:
    description: "Quality inspection result"
    fields:
      product_id:
        _type: string
      inspection_timestamp:
        _type: string
      pass_fail:
        _type: string
      defect_count:
        _type: number
      inspector_id:
        _type: string
      measurements:
        width_mm:
          _type: number
        height_mm:
          _type: number
        weight_g:
          _type: number

dataModels:
  - name: quality-check
    description: "Product quality inspection"
    version:
      v1:
        structure:
          inspection:
            _payloadshape: quality-inspection

dataContracts:
  - name: _quality-check
    model:
      name: quality-check
      version: v1
```

When this contract is active, the UNS will validate:
- Topic must end with `.inspection` (the only field defined in the model)
- Payload must contain all required fields from `quality-inspection` shape
- Field types must match (e.g., `defect_count` must be a number)
- No extra fields are allowed that aren't in the shape definition
