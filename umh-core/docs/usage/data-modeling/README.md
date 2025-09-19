# Data Modeling

Data modeling in UMH Core transforms device data into business-ready information through structured schemas and validation.

## The Component Chain

```
Payload Shapes → Data Models → Data Contracts → Data Flows
       ↓              ↓              ↓                ↓
  Value types     Structure      Enforcement    Execution
```

Each component builds on the previous:
- **[Payload Shapes](payload-shapes.md)** define what types of values are allowed
- **[Data Models](data-models.md)** use shapes to create hierarchical structure
- **[Data Contracts](data-contracts.md)** enforce models at runtime
- **[Data Flows](../data-flows/)** execute with or without contracts

## What You Can Model

In any UNS topic, data modeling controls specific portions:

```
umh.v1.enterprise.site.area.line._contract.virtual.path.name
       └───────── fixed ─────────┘         └── modeled ──┘
```

- **Fixed**: Location hierarchy comes from bridge configuration
- **Modeled**: Everything after the contract is defined by your data model
  - Virtual path: Organizational folders (e.g., `motor.electrical`)
  - Name: The actual data point (e.g., `current`)

See [Topic Convention](../unified-namespace/topic-convention.md) for complete structure.

## Data Flow Patterns

Data can follow these patterns based on your needs:

### Device Language (_raw)
Start by exploring your equipment data with original naming:

```
Device → Bridge → _raw → Topic Browser
```

**Example**: `umh.v1.enterprise.chicago.line-1.pump._raw.DB1.DW20`
- Exploration, debugging, quick connectivity
- Site engineers who know the PLC addressing
- Original tags preserved (e.g., `s7_address: "DB1.DW20"`)

### Device Models
Apply business naming directly in bridges:

```
Device → Bridge + Model → _pump_v1 → Applications
         (one step)
```

**Example**: `umh.v1.enterprise.chicago.line-1.pump._pump_v1.inlet_temperature`
- Consistent naming across equipment types
- Operations teams, local dashboards
- Most implementations start here

### Business Models
Transform device data into business KPIs:

```
Multiple _pump_v1 → Stream Processor → _maintenance_v1 → Enterprise Apps
```

**Example**: `umh.v1.enterprise.chicago._maintenance_v1.work_orders.create`
- Aggregated metrics, business records
- Enterprise systems, management dashboards
- Required when scaling across sites

## The Two-Layer Architecture

**Sites and HQ both need their view of the same data.** This isn't a choice between approaches - it's about enabling both layers to work together:

### Layer 1: Device Models (Data Structure Within Equipment)
- Define WHAT data points exist in equipment (temperature, vibration)
- NOT the organizational structure (that's location_path)
- Sites maintain control of their data definitions
- Original tags preserved in metadata
- Applied directly in bridges (one step)
- Primarily time-series data
- **Result**: Sites trust the system because they built it

### Layer 2: Business Models (Enterprise Metrics)
Created in TWO ways:
1. **Aggregation**: Stream processors combine device models into KPIs
2. **Direct**: Bridges connect to ERP/MES systems for business data

- Creates consistent metrics across sites
- Doesn't disturb site operations
- Multiple departments can create their own views
- Primarily relational data
- **Result**: Everyone gets the metrics they need

The key: Device models describe equipment internals, business models describe enterprise needs.

### Why Both Layers Matter
**Common failure patterns:**
- **Only device models**: Chaos across multiple sites, no standardization
- **Only business models**: Sites lose control, engineers reject the system

**The success formula:** Sites own their data structure (device models), everyone creates their views (business models). This is why stream processors exist - to bridge these layers without forcing change on sites.

## Key Concepts

### Location Path vs Device Model
- **Location Path**: WHERE equipment sits in your organization
  - Example: `enterprise.chicago.packaging.line-1.pump-01`
  - Defined in bridge configuration
  - This is your factory hierarchy
  
- **Device Model**: WHAT data points exist within that equipment  
  - Example: `_pump_v1.temperature`, `_pump_v1.vibration.x-axis`
  - Defines internal data structure of a single device
  - NOT the organizational structure

### Name vs Tag
- **Name**: The data point identifier in UNS topics
- **Tag**: Industry term for a sensor/data point
- We use "name" for broader applicability (not just time-series)

### Virtual Path
Organizational folders within your data model:
- **Example**: `motor.electrical`, `diagnostics.vibration`
- **Purpose**: Groups related data points logically within a device

### Data Contract vs Data Model
- **Data Model**: Defines structure (template)
- **Data Contract**: Enforces structure (runtime validation)
- **Example**: Creating `pump` model auto-creates `_pump_v1` contract

### Time-Series vs Relational
- **Time-Series**: Single value with timestamp
  - Example: `{"timestamp_ms": 1733904005123, "value": 42.5}`
- **Relational**: Multiple fields in one message
  - Example: Work order with 10 fields

See [Payload Formats](../unified-namespace/payload-formats.md) for details.

### Processing Methods

**In Bridges:**
- `tag_processor`: For time-series data from PLCs/sensors
- `nodered_js`: For relational data from ERP/MES systems

**In Stream Processors:**
- JavaScript expressions: For aggregating and transforming data
- Example: `total: "sensor1 + sensor2 + sensor3"`

## Choosing Your Data Flow

Based on your data source, choose the appropriate path:

| Source | Output | Bridge Processor | Path |
|--------|--------|-----------------|------|
| PLC/Sensor | Device Model | `tag_processor` | Direct to _pump_v1 |
| PLC/Sensor | Raw | `tag_processor` | Direct to _raw (exploration) |
| ERP/MES | Business Model | `nodered_js` | Direct to _workorder_v1 |
| Multiple Device Models | Business Model | - | Via Stream Processor |

### Data Type Alignment
- **Device Models**: 90% time-series data
  - Temperature, pressure, vibration readings
  - Status indicators, counters, running hours
  - Process with: `tag_processor` in bridges
  
- **Business Models**: 90% relational data  
  - Work orders, maintenance schedules
  - Production batches, quality reports
  - Process with: `nodered_js` in bridges OR aggregate via stream processors

## Implementation Patterns

### Pattern 1: Equipment Monitoring
```
PLC tags → Bridge + tag_processor → Device Model (_pump_v1)
Location: enterprise.site.line.pump-01
Model adds: .temperature, .pressure, .status
```
**When**: Connecting industrial equipment with time-series data

### Pattern 2: ERP Integration
```
SAP work orders → Bridge + nodered_js → Business Model (_workorder_v1)
```
**When**: Connecting business systems with relational data

### Pattern 3: Multi-Site Aggregation
```
site1._pump_v1 ─┐
site2._pump_v1 ─├→ Stream Processor → _maintenance_v1
site3._pump_v1 ─┘   (JavaScript expressions)
```
**When**: Creating KPIs from multiple device models

### Pattern 4: Exploration First
```
Unknown PLC → Bridge + tag_processor → _raw → Topic Browser
                                           ↓
                                    Design device model
                                           ↓
                                    Apply in bridge
```
**When**: Understanding new equipment before modeling

## Why Models Are Immutable

**The scenario**: A data scientist builds a dashboard using `_pump_v1` for 500 pumps across 10 sites.

**Without immutability**: Someone modifies `_pump_v1`:
- Dashboard breaks - fields renamed
- Historical queries fail - structure changed
- Other sites stop working
- Weekend ruined

**With immutability**:
1. Create `_pump_v2` with changes
2. Test thoroughly
3. Migrate gradually
4. Deprecate v1 when safe
5. Dashboard keeps working throughout

## Common Questions

### When should I use data models?

Start with `_raw` for exploration. Add models when:
- You need consistent naming across sites
- Multiple applications consume the data
- Validation becomes critical
- You're ready for production

### How do I handle different equipment versions?

Create separate models:
- `_pump_v1` for older pumps
- `_pump_v2` for newer pumps with more sensors
- Stream processors can aggregate both

### What about equipment-specific data?

Use virtual paths to organize:
```
_pump_v1.motor.temperature
_pump_v1.motor.current
_pump_v1.diagnostics.vibration
```

## Next Steps

1. **Define value types**: [Payload Shapes](payload-shapes.md)
2. **Create structure**: [Data Models](data-models.md)
3. **Enforce validation**: [Data Contracts](data-contracts.md)
4. **Transform data**: [Stream Processors](stream-processors.md)

## Learn More

- [Getting Started Guide](../../getting-started/) - See data modeling in action
- [Bridges](../data-flows/bridges.md) - Apply models during ingestion
- [Topic Convention](../unified-namespace/topic-convention.md) - Understand topic structure
