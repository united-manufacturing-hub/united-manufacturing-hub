# Unified Namespace

> **Prerequisite:** Complete the [Getting Started guide](../../getting-started/) to see the UNS in action with real devices and data.

The Unified Namespace (UNS) is where ALL your industrial data lives in one organized, accessible place. Instead of hunting through 50 different systems to find a temperature reading, everything flows through one central hub.

## The Problem: Spaghetti Diagrams

Traditional manufacturing IT looks like this:
```
100 devices × 100 systems = 10,000 point-to-point connections
```

Every new dashboard means updating PLCs. Every new sensor means modifying databases. Every integration breaks when you upgrade. It's a maintenance nightmare that gets worse with scale.

## The Solution: One Central Hub

The UNS flips this architecture:
```
100 devices → 1 namespace ← 100 systems = 200 total connections
```

All data flows through one place with consistent structure, validation, and access patterns.

## How It Works

### 1. Publish Regardless
Your PLC doesn't care if anyone is listening. It publishes "pump is running" to the UNS and moves on. When someone needs that data next week, it's already there. No reprogramming required.

### 2. Structured Topics
Every piece of data has an address that answers three questions:
- **WHERE**: `enterprise.site.area.line` - the location hierarchy
- **WHAT**: `_pump_v1` - the data contract defining structure
- **WHICH**: `inlet_temperature` - the specific data point

Result: `umh.v1.enterprise.site.area.line._pump_v1.inlet_temperature`

### 3. Bridges Handle All Data Flow
Data enters and exits the UNS exclusively through [bridges](../data-flows/bridges.md). This ensures every message gets proper context, validation, and organization.

### 4. Progressive Power: From Device to Business
Start simple, add complexity as needed:
- **Raw Data (`_raw`)**: Mirror device 1:1 for initial exploration and debugging
- **Device Models (`_pump_v1`)**: Apply business names directly in bridges for production
- **Business Models (`_maintenance_v1`)**: Aggregate device models via stream processors for KPIs

The progression: Use `_raw` temporarily to understand your data, then apply device models directly in bridges for production. Once multiple sites have device models, create business models on top for cross-site metrics.

## Documentation Structure

- **[Topic Convention](topic-convention.md)** - How data is addressed in the namespace
- **[Payload Formats](payload-formats.md)** - Time-series vs relational message structure
- **[Metadata and Tracing](metadata-and-tracing.md)** - Original tags and data lineage
- **[Topic Browser](topic-browser.md)** - Explore your namespace in real-time

## Next Steps

1. **Explore your data**: Open the [Topic Browser](topic-browser.md)
2. **Connect devices**: Create [bridges](../data-flows/bridges.md)
3. **Model your data**: Define [data models](../data-modeling/data-models.md)
4. **Create KPIs**: Build [stream processors](../data-modeling/stream-processors.md)
