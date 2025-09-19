# Payload Shapes

> **You've already used these!** Remember in [Step 4](../../getting-started/3-validate-data.md) when you defined `_payloadshape: timeseries-number` for your vibration data? That ensured only numbers could be stored. This guide explains all the shapes available to you.

## What Are Payload Shapes?

Payload shapes are **validation rules** for your data. They define:
- What fields are allowed in a message
- What data types each field must be (number, string, etc.)
- Whether fields are required or optional

Think of them as the difference between:
- ❌ **Without shapes**: "temperature" could receive "hello" instead of 42.5
- ✅ **With shapes**: "temperature" only accepts numbers, rejecting invalid data

## The Shapes You've Already Used

### timeseries-number (Used in Step 4)

You used this for your CNC vibration data. It's built into UMH - no configuration needed.

**What it accepts:**
```json
{
  "timestamp_ms": 1733904005123,
  "value": 12.5  // Your x-axis vibration reading
}
```

**What it means:**
- `timestamp_ms`: When the reading was taken (milliseconds since 1970)
- `value`: The actual sensor reading (any number)

This is perfect for 90% of industrial data: temperature, pressure, speed, counts, etc.

**Real Validation from Step 4:**

Remember when your bridge failed because `DB1.DW20` didn't match `vibration.x-axis`? That was payload shape validation in action!

✅ **What worked:**
```javascript
// Your fixed Tag Processor code
msg.meta.data_contract = "_cnc_v1";
msg.meta.virtual_path = "vibration";
msg.meta.tag_name = "x-axis";
msg.payload = parseFloat(msg.payload) * 1.0;  // Ensures it's a number!
```

❌ **What would fail:**
```javascript
msg.payload = "sensor error";  // String instead of number - REJECTED!
msg.payload = {temp: 42.5};    // Object instead of number - REJECTED!
// Bridge goes to degraded state, deployment fails
```

### timeseries-string (For Text Data)

For status messages, product names, batch IDs - anything that's not a number.

**What it accepts:**
```json
{
  "timestamp_ms": 1733904005123,
  "value": "running"  // Or "stopped", "Product-ABC", "Batch-2024-001"
}
```

**Common uses:**
- Machine states: "running", "stopped", "maintenance"
- Product codes from `DB1.S30.10` (remember that from Step 3?)
- Operator names, batch IDs, quality grades

## When You Need More: Custom Shapes

### The Problem with Time-Series

Time-series is great for single values, but what about complex data?

**Example:** A quality inspection has multiple related fields:
- Product ID
- Inspector name  
- Pass/fail result
- Multiple measurements
- Timestamp

You can't split this into separate time-series messages - it's one inspection event.

**Solution:** Create a custom shape that accepts all fields together.

### Creating Your First Custom Shape

Let's extend your CNC example with quality inspection data:

```yaml
# In /data/config.yaml
payloadShapes:
  cnc-quality-check:
    description: "CNC part quality inspection"
    fields:
      part_id:
        _type: string        # "PART-2024-001"
      machine_id:
        _type: string        # Which CNC produced it
      pass_fail:
        _type: string        # "pass" or "fail"
      measurements:
        diameter_mm:
          _type: number      # 25.4
        length_mm:
          _type: number      # 100.2
        surface_finish:
          _type: number      # 0.8 (Ra value)
      inspector:
        _type: string        # "John Smith"
      timestamp:
        _type: string        # "2024-01-15T10:30:00Z"
```

Now one message contains the complete inspection record.

### Using Custom Shapes in Models

Just like you used `timeseries-number` in Step 4:

```yaml
dataModels:
  - name: cnc-complete  # Your enhanced CNC model
    version:
      v3:
        structure:
          vibration:      # Time-series data (existing)
            x-axis:
              _payloadshape: timeseries-number
          quality:        # Complex data (new!)
            inspection:
              _payloadshape: cnc-quality-check
```

Now your CNC model handles both vibration AND quality data.

### How Validation Works

Just like in Step 4, validation happens at the bridge:

✅ **Valid quality inspection:**
```json
{
  "part_id": "PART-2024-001",
  "machine_id": "CNC-01",
  "pass_fail": "pass",
  "measurements": {
    "diameter_mm": 25.4,
    "length_mm": 100.2,
    "surface_finish": 0.8
  },
  "inspector": "John Smith",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

❌ **Invalid - wrong data types:**
```json
{
  "part_id": "PART-2024-001",
  "machine_id": "CNC-01",
  "pass_fail": true,  // Should be string "pass"/"fail"
  "measurements": {
    "diameter_mm": "25.4mm",  // Should be number, not string
    ...
  }
}
// Result: Bridge goes degraded, just like in Step 4!
```

### Processing Complex Data

Unlike simple tags (Step 3's Tag Processor), complex data needs different processing:

**Simple (what you know):**
```javascript
// Tag Processor for time-series
msg.meta.tag_name = msg.meta.s7_address;  // "DB1.DW20"
msg.payload = parseFloat(msg.payload);     // Single number
```

**Complex (custom shapes):**
```javascript
// nodered_js processor for inspection data
msg.meta.data_contract = "_cnc-complete_v3";
msg.meta.tag_name = "inspection";

// Build complete inspection object
msg.payload = {
  "part_id": generatePartId(),
  "machine_id": "CNC-01",
  "pass_fail": checkTolerance() ? "pass" : "fail",
  "measurements": {
    "diameter_mm": parseFloat(msg.diameter),
    "length_mm": parseFloat(msg.length),
    "surface_finish": parseFloat(msg.surface)
  },
  "inspector": getOperatorName(),
  "timestamp": new Date().toISOString()
};
```

## Quick Reference: Data Types

Only two types you need to know:

- `_type: number` - Any numeric value (42, 3.14, -100)
- `_type: string` - Any text ("running", "Product-ABC", "2024-01-15")

That's it! These handle 99% of industrial data.

## Decision Guide: Which Shape Do I Need?

```
Is your data a single value changing over time?
├─ Yes → Use built-in shapes
│   ├─ Numbers? → timeseries-number (temperature, pressure, speed)
│   └─ Text? → timeseries-string (status, batch ID, operator)
│
└─ No → Create custom shape
    ├─ Quality inspection with multiple measurements
    ├─ Work order with multiple fields
    └─ Any business record with related data
```

**Start simple:** Use time-series shapes until you need more.

## Your Next Step

Now that you understand how data validation works:

- **To organize your data better** → Read [Data Models](data-models.md) 
- **To enforce these rules** → Read [Data Contracts](data-contracts.md)
- **To see it all together** → Review the [complete CNC example](README.md#real-example-scaling-your-cnc-model)
