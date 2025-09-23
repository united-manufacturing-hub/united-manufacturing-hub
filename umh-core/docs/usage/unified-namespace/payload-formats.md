# Payload Formats

UMH Core supports two payload formats: **Time-Series** for individual sensor readings and **Relational** for structured business data.

## Quick Reference

| Type | Structure | Use Case | Example |
|------|-----------|----------|---------|
| **Time-Series** | `{"timestamp_ms": int, "value": any}` | Sensor readings, machine states | Temperature, pressure, status |
| **Relational** | `{...any JSON object...}` | Business records, work orders | Batch data, quality reports |

## Time-Series Data

Individual sensor values with timestamps. One value per message.

**Format:**
```json
{
  "timestamp_ms": 1733904005123,
  "value": 42.5
}
```

**Requirements:**
- Must have exactly `timestamp_ms` and `value` keys
- `timestamp_ms`: Integer or float without fraction
- `value`: Number, boolean, or string
- No additional keys allowed
- Max size: 1MiB

**Examples:**
```json
// Temperature reading
{"timestamp_ms": 1733904005123, "value": 23.4}

// Machine state
{"timestamp_ms": 1733904005123, "value": "running"}

// Boolean sensor
{"timestamp_ms": 1733904005123, "value": true}
```

## Relational Data

Structured records with multiple fields. Used for business data and aggregated metrics.

**Format:**
```json
{
  "field1": "value1",
  "field2": 123,
  "nested": {
    "field3": true
  }
}
```

**Requirements:**
- Must be a JSON object (not array or primitive)
- Can have any keys and nested structure
- Max size: 1MiB

**Examples:**
```json
// Work order
{
  "order_id": "WO-2024-001",
  "product": "Widget-A",
  "quantity": 1000,
  "timestamp_ms": 1733904005123
}

// Quality inspection
{
  "batch_id": "B-2024-12-01",
  "measurements": {
    "length_mm": 100.2,
    "width_mm": 50.1,
    "weight_g": 250.5
  },
  "passed": true,
  "inspector": "John Doe"
}
```

## The "One Tag, One Topic" Rule

Each sensor value gets its own topic. This simplifies addressing, consumption, and debugging.

**Instead of:**
```
Topic: umh.v1.acme.plant._raw.weather
Payload: {"temperature": 23.4, "humidity": 42.1}
```

**Use:**
```
Topic: umh.v1.acme.plant._raw.weather.temperature
Payload: {"timestamp_ms": 1733904005123, "value": 23.4}

Topic: umh.v1.acme.plant._raw.weather.humidity
Payload: {"timestamp_ms": 1733904005123, "value": 42.1}
```

### Why This Matters

| Problem with Multi-Value Payloads | Real-World Impact |
|-----------------------------------|-------------------|
| **Timing issues** | Temperature arrives, humidity 100ms later. Do you cache? Wait? How long? |
| **Merge windows** | Two PLC variables change almost simultaneously but arrive separately |
| **Clock skew** | Reading 100 tags takes milliseconds - first may change before last is read |
| **Complex addressing** | Need 3 dimensions: topic × tag name × JSON path |
| **Testing complexity** | Each edge case (missing key, late field, partial failure) multiplies bugs |

**Benefits of One Tag Per Topic:**
- **Zero merge logic** - Each value is complete when published
- **Simple subscriptions** - Subscribe only to tags you need
- **Clear debugging** - One topic = one data point
- **Schema simplicity** - Data contract just specifies time-series format

## Next Steps

- [Bridges](../data-flows/bridges.md) - Send data to UNS
- [Stream Processors](../data-modeling/stream-processors.md) - Transform time-series to relational
- [Data Models](../data-modeling/data-models.md) - Define structure for both formats
