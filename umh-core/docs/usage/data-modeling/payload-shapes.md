# Payload Shapes

> This article assumes you've completed the [Getting Started guide](../../getting-started/) and understand the [data modeling concepts](README.md).

Payload shapes define the structure and validation rules for message payloads. While built-in shapes handle most time-series data, custom shapes enable relational data modeling for complex business records.

## Overview

In the [component chain](README.md#the-component-chain), payload shapes provide the foundation:

```
Payload Shapes → Data Models → Data Contracts → Data Flows
       ↑
  Value types defined here
```

## UI Capabilities

Payload shapes have no UI component:

| Feature | Available | Notes |
|---------|-----------|-------|
| View shapes | ❌ | Configuration file only |
| Create shapes | ❌ | Edit config.yaml directly |
| Edit shapes | ❌ | Modify config.yaml |
| Built-in shapes | ✅ | Always available, no configuration needed |

**Configuration location**: Select your instance under `Instances` in the menu, press `...`, then `Config File`. Find payload shapes under the `payloadShapes:` section

## Built-in Shapes

UMH provides two built-in shapes that handle 90% of industrial data:

### timeseries-number

For numeric sensor data and measurements.

**Structure:**
```json
{
  "timestamp_ms": 1733904005123,
  "value": 42.5
}
```

**Use cases:** Temperature, pressure, speed, counts, any numeric measurement

### timeseries-string

For text-based status and identifiers.

**Structure:**
```json
{
  "timestamp_ms": 1733904005123,
  "value": "running"
}
```

**Use cases:** Machine states, batch IDs, product codes, operator names

## When to Use Custom Shapes

Custom shapes are specifically for **relational data** - business records with multiple related fields that must stay together.

**Time-series vs Relational:**

| Data Type | Shape | Example |
|-----------|-------|----------|
| Single values over time | Built-in timeseries-* | Temperature readings |
| Complex business records | Custom shape | Work orders, quality inspections |

Learn more: [Payload Formats](../unified-namespace/payload-formats.md)

## Configuration

Access configuration via: Instances → Select instance → `...` → Config File

### Defining Custom Shapes

```yaml
payloadShapes:
  work-order:    # Shape name (referenced in models)
    description: "Work order record"
    fields:
      orderId:
        _type: string
      productId:
        _type: string
      quantity:
        _type: number
      status:
        _type: string    # created/in-progress/completed
      operatorId:
        _type: string
      timestamp:
        _type: string    # ISO 8601 format
```

**Key points:**
- Only two types: `string` and `number`
- All fields are required
- Shape names must be unique

### Using Custom Shapes in Models

Reference custom shapes in your data models to create CRUD-like endpoints:

```yaml
datamodels:
  - name: work-order
    version:
      v1:
        structure:
          create:
            _payloadshape: work-order    # Custom shape for new orders
          update:
            _payloadshape: work-order    # Same shape for updates
          delete:
            _payloadshape: work-order    # Or simplified delete shape
```

This creates topics like:
- `enterprise.site._work_order_v1.create` - New work orders
- `enterprise.site._work_order_v1.update` - Update existing orders
- `enterprise.site._work_order_v1.delete` - Delete orders

## Processing Relational Data

### Example: Processing Work Orders

```javascript
// In nodered_js processor for work order creation
msg.meta.data_contract = "_work_order_v1";
msg.meta.tag_name = "create";  // Maps to the 'create' endpoint

// Build work order record
msg.payload = {
  orderId: "WO-" + Date.now(),
  productId: msg.product,
  quantity: msg.qty,
  status: "created",
  operatorId: msg.operator,
  timestamp: new Date().toISOString()
};

return msg;
```

This creates a message at `enterprise.site._work_order_v1.create` with the complete work order data.


## Relationship to Other Components

Payload shapes are referenced by [data models](data-models.md), which are then enforced by [data contracts](data-contracts.md).

## Next Steps

- **Define structure**: [Data Models](data-models.md) - Create hierarchies using shapes
- **Enforce validation**: [Data Contracts](data-contracts.md) - Make shapes mandatory
- **Build pipelines**: [Data Flows](../data-flows/) - Process data with bridges
