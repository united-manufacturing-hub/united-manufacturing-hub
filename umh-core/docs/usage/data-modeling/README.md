# Data Modeling

> **Quick Start**: If you just want to connect devices and see data flowing, start with [Bridges](../data-flows/bridges.md) and the `_raw` data contract. Come back here when you need structured, validated data models.

UMH Core's data modeling system helps you transform raw industrial data into structured, validated information. Start simple with device-specific models, then evolve to business-ready analytics.

## Why Data Modeling Matters

Manufacturing companies typically start with **implicit data modeling** - using bridges to contextualize data factory by factory. This bottom-up approach works well for single sites: look at what's available in your PLC/Kepware, add basic metadata, rename cryptic tags like `XYDAG324` to `temperature`, and publish to the UNS.

But as companies scale across **multiple factories**, they hit a wall:

- **Inconsistent schemas**: Each site names the same equipment differently (`motor_speed` vs `rpm` vs `rotational_velocity`)
- **No standardization**: Pump data from Factory A has different fields than identical pumps in Factory B
- **Analytics nightmares**: Cross-site dashboards and analytics require custom mapping for every location
- **Knowledge silos**: Each site's contextualization is trapped in local configurations

**Explicit data modeling** solves this by defining standardized templates that enforce consistency across the entire enterprise. Instead of each factory doing its own contextualization, you define once: "Every Pump has these exact fields: `pressure`, `temperature`, `motor.current`, `motor.rpm`" - then apply that template everywhere.

### From Implicit to Explicit

| Approach | Scope | Benefits | Limitations |
|----------|--------|----------|-------------|
| **Implicit** (Current Bridges) | Per-factory contextualization | Quick setup, site-specific optimization | Inconsistent across sites, no templates |
| **Explicit** (Data Modeling) | Enterprise-wide standardization | Consistent schemas, reusable templates, cross-site analytics | Requires upfront design, more rigid |

UMH's unified data-modelling bridges this gap: keep the flexibility of per-site bridges for raw data collection, but add explicit modeling on top for enterprise standardization.

## Get Started

### Step 1: Start Simple (No Modeling)
Connect your devices with bridges and use the `_raw` data contract. Your data flows immediately:
```
OPC UA → Bridge → umh.v1.plant.line.device._raw.temperature
```
This is already "Silver" data - it has location context.

**Learn more:** [Creating Bridges](../data-flows/bridges.md) | [Producing Data](../unified-namespace/producing-data.md)

### Step 2: Add Device Models
When you need validation and structure, create device-specific models:
```
OPC UA → Bridge with Model → umh.v1.plant.line.device._pump_v1.pressure
```
Most users only need this level - device modeling with validation.

**Learn more:** [Data Models](data-models.md) | [Data Contracts](data-contracts.md) | [Example below](#example-device-modeling-in-bridges)

### Step 3: Business Analytics (Gold)
For cross-device business data, use stream processors to transform Silver → Gold:
```
Multiple Silver sources → Stream Processor → umh.v1.plant._workorder_v1.created
```

**Learn more:** [Stream Processors](stream-processors.md) | [Payload Formats](../unified-namespace/payload-formats.md#relational-data)

## The Silver → Gold Architecture

In industrial data, we distinguish between two types of modeling:

### Silver: Device-Specific Models
- **What**: Individual device data with structure (`_pump_v1`, `_temperature_v1`)
- **Where**: Created in bridges using data models and contracts
- **Format**: Mostly [time-series data](../unified-namespace/payload-formats.md#time-series-data)
- **Example**: Pump pressure, motor RPM, temperature readings

### Gold: Use-Case Specific Models  
- **What**: Business data aggregated across devices (`_workorder_v1`, `_maintenance_v1`)
- **Where**: Created by stream processors (🚧 currently [time-series](../unified-namespace/payload-formats.md#time-series-data) only)
- **Format**: Mostly [relational data](../unified-namespace/payload-formats.md#relational-data)
- **Example**: Work orders, maintenance requests, production batches

> **Important**: You don't have to follow this pattern strictly. Bridges can write directly to Gold-level contracts if needed. This is guidance, not enforcement.

## Building Blocks

When you're ready to create models, understand these components:

| Component | Purpose | When You Need It |
|-----------|---------|------------------|
| **[Payload Shapes](payload-shapes.md)** | Define the structure of message payloads using JSON schema | Custom [relational formats](../unified-namespace/payload-formats.md#relational-data) |
| **[Data Models](data-models.md)** | Define [virtual path hierarchy](../unified-namespace/topic-convention.md#virtual-path) after the data contract | Device templates |
| **[Data Contracts](data-contracts.md)** | Enforce validation in bridges via UNS output plugin | Data quality assurance |
| **[Stream Processors](stream-processors.md)** | Transform between models & create different views of UNS data | Silver → Gold only |

## Example: Device Modeling in Bridges

Most users start with device-specific modeling directly in bridges:

### 1. Create a Simple Pump Model
```yaml
# Define the model structure
datamodels:
  pump:
    description: "Standard pump model"
    versions:
      v1:
        structure:
          pressure:
            _payloadshape: timeseries-number
          temperature:
            _payloadshape: timeseries-number
          running:
            _payloadshape: timeseries-number
```

**Learn how:** [Data Models documentation](data-models.md) | [Model structure guide](data-models.md#structure-definition)

### 2. Create a Data Contract for Enforcement
```yaml
# This enforces validation
datacontracts:
  - name: _pump_v1
    model:
      name: pump
      version: v1
```

**Learn how:** [Data Contracts documentation](data-contracts.md) | [Contract configuration](data-contracts.md#core-properties)

### 3. Configure Bridge to Use the Model
```yaml
protocolConverter:
  - name: pump-bridge
    dataflowcomponent_read:
      data_contract: "_pump_v1"  # Enforce pump model
      benthos:
        input:
          opcua:
            endpoint: "opc.tcp://192.168.1.100:4840"
            nodeIDs: ["ns=2;s=Pressure", "ns=2;s=Temperature", "ns=2;s=Status"]
        pipeline:
          processors:
            - tag_processor:
                defaults: |
                  msg.meta.location_path = "{{ .location_path }}";
                  msg.meta.data_contract = "_pump_v1";  # Use pump contract
                  msg.meta.tag_name = msg.meta.opcua_tag_name;
        output:
          uns: {}  # Validates against pump model
```

**Learn how:** [Bridges documentation](../data-flows/bridges.md) | [Tag processor configuration](../data-flows/bridges.md#tag-processor)

### 4. Result: Validated, Structured Data
```
✅ Valid: umh.v1.plant.line.pump42._pump_v1.pressure
   Payload: { "value": 4.2, "timestamp_ms": 1733904005123 }

❌ Invalid: umh.v1.plant.line.pump42._pump_v1.invalid_field
   Result: Bridge goes degraded, message rejected
```

**No stream processor needed!** The bridge handles device modeling directly.

## When Do You Need Stream Processors?

Stream processors are ONLY for transforming data between different models (Silver → Gold):

| Use Case | Solution | Why |
|----------|----------|-----|
| Rename OPC UA tags to friendly names | Bridge with `tag_processor` | Simple mapping, same model |
| Convert temperature units (°F → °C) | Bridge with expression | Simple transformation, same model |
| Enforce pump data structure | Bridge with data contract | Device modeling, validation |
| **Create work order from alarms** | **Stream Processor** | **Multiple devices, relational output** |
| **Generate maintenance request** | **Stream Processor** | **Business logic, different model** |
| **Batch production records** | **Stream Processor** | **Aggregate cycle data, new format** |

> **Rule of Thumb**: If you're working with a single device and want to structure its data, use a bridge. If you're combining data from multiple sources into business KPIs, use a stream processor.

## Next Steps

- **New to UMH?** Start with [Bridges](../data-flows/bridges.md) and `_raw` data
- **Ready for validation?** Learn about [Data Contracts](data-contracts.md)
- **Need custom formats?** Explore [Payload Shapes](payload-shapes.md)
- **Building device templates?** Read [Data Models](data-models.md)
- **Creating business KPIs?** Check [Stream Processors](stream-processors.md) (currently time-series only)

---

## Formal Definitions

### Data Architecture Levels

#### Bronze Data
- **Definition**: Raw, unprocessed data directly from industrial sources (PLCs, sensors, OPC UA servers)
- **Location**: External to UMH - exists in source systems
- **Format**: Vendor-specific, proprietary protocols
- **Example**: Siemens S7 data blocks, OPC UA node values
- **Note**: Bronze doesn't exist within UMH - as soon as data enters via bridges, it becomes Silver

#### Silver Data
- **Definition**: Contextualized device data with location assignment and optional structure
- **Contract Examples**: `_raw` (unstructured), `_pump_v1` (device model), `_temperature_v1` (sensor model)
- **Format**: Primarily [time-series](../unified-namespace/payload-formats.md#time-series-data), some [relational](../unified-namespace/payload-formats.md#relational-data)
- **Creation**: Via bridges (protocol converters) with optional data models
- **Characteristics**:
  - Has ISA-95 location context
  - Device-specific focus
  - 1:1 mapping from source
  - Optional validation via data contracts

#### Gold Data
- **Definition**: Business-ready data aggregated and transformed from multiple Silver sources
- **Contract Examples**: `_workorder_v1`, `_maintenance_v1`, `_batch_v1`
- **Format**: Primarily [relational](../unified-namespace/payload-formats.md#relational-data), some aggregated [time-series](../unified-namespace/payload-formats.md#time-series-data)
- **Creation**: Via stream processors transforming Silver data
- **Characteristics**:
  - Use-case specific
  - Many:1 relationship with devices
  - Business logic applied
  - Often event-driven or time-windowed

### Component Hierarchy

The complete data modeling hierarchy with formal definitions:

```
Payload Shapes → Data Models → Data Contracts → Runtime Components
```

#### 1. Payload Shapes
- **Definition**: JSON Schema definitions that specify the exact structure of message payloads
- **Scope**: Defines fields, types, constraints for a single message format
- **Built-in**: `timeseries-number`, `timeseries-string` (always available)
- **Custom**: User-defined schemas for [relational data](../unified-namespace/payload-formats.md#relational-data)
- **Configuration**: Defined in `payloadShapes:` section of config.yaml

#### 2. Data Models
- **Definition**: Hierarchical structure defining the [virtual path](../unified-namespace/topic-convention.md#virtual-path) and fields after the data contract in UNS topics
- **Scope**: Creates the topic tree structure (folders, sub-models, fields)
- **Versioning**: Each model has explicit versions (v1, v2, etc.)
- **Reusability**: Same model can be used across multiple locations
- **Configuration**: Defined in `dataModels:` section with `structure:` tree

#### 3. Data Contracts
- **Definition**: Enforcement mechanism that binds a specific model version and enables validation
- **Scope**: Links model + version, specifies retention, storage sinks
- **Enforcement Point**: UNS output plugin in bridges validates against contract
- **Naming**: Contract name becomes the data_contract segment in topics (e.g., `_pump_v1`)
- **Configuration**: Defined in `dataContracts:` section referencing model + version

#### 4. Runtime Components

##### Bridges (Protocol Converters)
- **Definition**: Components that connect external systems to UNS with optional data modeling
- **Scope**: Single data contract per bridge read/write flow
- **Processing**: `tag_processor` for [time-series](../unified-namespace/payload-formats.md#time-series-data), `nodered_js` for [relational](../unified-namespace/payload-formats.md#relational-data)
- **Validation**: Enforced via UNS output plugin when contract specified

##### Stream Processors
- **Definition**: Components that transform data between different models (Silver → Gold)
- **Scope**: Consume from one model, produce to another
- **Current Limitation**: [Time-series](../unified-namespace/payload-formats.md#time-series-data) output only (relational planned)
- **Use Cases**: Aggregation, business logic, creating different data views

### Data Flow Patterns

#### Device Modeling Pattern (Most Common)
```
External System → Bridge (with model) → Silver Contract → UNS
```

#### Business Transformation Pattern (Advanced)
```
Multiple Silver Topics → Stream Processor → Gold Contract → UNS
```

#### Direct Gold Pattern (Exceptional)
```
External System → Bridge → Gold Contract → UNS
```
*Note: Possible but not recommended - bypasses progressive refinement*
