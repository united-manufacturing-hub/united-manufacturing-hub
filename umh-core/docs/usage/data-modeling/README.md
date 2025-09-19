# Data Modeling

Data modeling in UMH Core enables both local flexibility and enterprise standardization through a dual approach: bottom-up contextualization and top-down modeling.

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
umh.v1.enterprise.site.area.line._contract.virtual.path.tag_name
       └───────── fixed ─────────┘         └──── modeled ────┘
```

- **Fixed**: Location hierarchy comes from bridge configuration
- **Modeled**: Everything after the contract is defined by your data model
  - Virtual path: Organizational folders (e.g., `motor.electrical`)
  - Tag name: The actual data point (e.g., `current`)

See also [Topic Convention](../unified-namespace/topic-convention.md)

## The Dual Approach

Successful implementations use both bottom-up and top-down strategies, which naturally align with Silver and Gold data levels:

### Bottom-Up: Contextualization → Silver Data

Start with machine reality. Keep the structure, add meaning. This naturally creates Silver data.

**Example**: Your S7 PLC uses `DB1.DW20`
- Keep as tag name (preserves local knowledge)
- Add metadata (units, description)
- Output to `_raw` or light device models
- Creates Silver data with local context

**Result**: Site engineers recognize their data in Silver layer, can troubleshoot effectively.

### Top-Down: Standardization → Gold Data

Define enterprise requirements. Create business metrics. This naturally creates Gold data.

**Example**: HQ needs "Maintenance Work Orders"
- Aggregate alarm data from multiple pumps (Silver)
- Apply business rules (3 alarms = create work order)
- Output to `_workorder_v1`, `_maintenance_v1`
- Creates Gold data with relational structure

**Result**: Enterprise gets unified work order tracking across all sites.

### The Meeting Point: Structured Silver

Where both approaches converge - device models that serve both needs:

**Example**: Standardized pump model `_pump_v1`
- Structured enough for enterprise analytics (top-down need)
- Detailed enough for site troubleshooting (bottom-up need)
- Still Silver (device-level) but with enforced structure
- Bridges the gap between local and enterprise

### The Alignment

| Data Level | Approach | Owner | Focus | Example |
|------------|----------|-------|-------|---------|
| **Silver** (_raw) | Bottom-up | Site engineers | Flexibility, local knowledge | `DB1.DW20` as-is |
| **Silver** (_device_v1) | Both | Shared ownership | Balance of both needs | Standardized pump model |
| **Gold** (_workorder_v1) | Top-down | Enterprise teams | Business records | Maintenance orders from alarms |

This natural alignment means:
- Site teams naturally work with Silver (their equipment data)
- Enterprise teams naturally work with Gold (business metrics)
- Structured Silver models become the collaboration point

## Data Architecture Levels

### Bronze → Silver → Gold

The industry-standard pattern for data refinement:

| Level | Location | Description | Example | How to Create |
|-------|----------|-------------|---------|---------------|
| **Bronze** | External systems | Raw data before UMH | S7 DB blocks, OPC nodes | N/A - exists outside UMH |
| **Silver** | In UMH | Contextualized device data | `_raw`, `_pump_v1` | Via bridges |
| **Gold** | In UMH | Business records | `_workorder_v1`, `_maintenance_v1` | Via stream processors |

### The UNS Idiomatic Pattern

**Most common**: Bronze → Silver → Gold
```
PLC → Bridge → Silver (_raw or _pump_v1) → Stream Processor → Gold (_workorder_v1)
```

**Also valid**: Direct to Gold
```
MES → Bridge → Gold (_workorder_v1)
```

Use judgment based on your use case.

## Key Concepts

### Tag
A single data point from your industrial equipment.
- **Example**: Temperature sensor reading, motor speed, valve position
- **In UNS**: The final segment of a topic that receives data

### Virtual Path
Organizational folders within your data model.
- **Example**: `motor.electrical`, `diagnostics.vibration`
- **Purpose**: Groups related tags logically

### Data Contract vs Data Model
A common confusion:
- **Data Model**: Defines structure (like a template)
- **Data Contract**: Enforces that structure (makes it mandatory)
- **Example**: When you created `cnc` model in the UI, it auto-created `_cnc_v1` contract

### Time-Series vs Relational Data
- **Time-Series**: Single value with timestamp ([details](../unified-namespace/payload-formats.md#time-series-data))
  - Processing: `tag_processor` in bridges
  - Example: `{"timestamp_ms": 1234567890, "value": 42.5}`
- **Relational**: Multiple related fields ([details](../unified-namespace/payload-formats.md#relational-data))
  - Processing: `nodered_js` in bridges
  - Example: Quality inspection with 10 fields

## Design Patterns

### Pattern 1: Pure Contextualization
```
S7 Bridge → _raw → Keep all tags as-is, add metadata
```
**Use when**: Starting out, exploring data, maintaining local knowledge

### Pattern 2: Full Standardization
```
S7 Bridge → _pump_v1 → Enforce enterprise pump model
```
**Use when**: Mature deployment, cross-site analytics critical

### Pattern 3: Hybrid Approach (Recommended)
```
Most data → _raw (preserve site structure)
Critical equipment → _pump_v1 (standardize key assets)
Equipment alarms → Stream processor → _maintenance_v1
```
**Use when**: Balancing local flexibility with enterprise needs

### Pattern 4: Direct Gold Injection
```
MES/ERP → Bridge → _workorder_v1 (skip Silver)
```
**Use when**: Data is already business-level, no device context needed

## Common Questions

### When should I use data models?

Start with `_raw` for exploration. Add models when:
- You need guaranteed data structure
- Multiple sites need the same format
- Dashboards require stable schemas
- Validation becomes critical

### Why are models immutable?

**The scenario**: A data scientist at HQ builds an analytics dashboard using the `pump_v1` model. It monitors 500 pumps across 10 sites, tracking pressure trends to predict failures.

**What happens without immutability**: A site engineer needs to add a field, so they modify `pump_v1` directly. Suddenly:
- The HQ dashboard breaks - expected fields are renamed
- Historical data queries fail - structure changed
- Other sites' integrations stop working
- The data scientist's weekend is ruined

**This is why models are immutable** - they're a contract between teams. When you use `pump_v1`, you're guaranteed that structure will always exist exactly as defined.

### How do I evolve models then?

Since you can't change v1:
1. Create v2 with your changes
2. Deploy bridges using v2
3. Migrate dashboards to v2
4. Keep v1 running until migration complete
5. Deprecate v1 only when safe

This way, the data scientist's dashboard keeps working while you roll out improvements.

### What's the difference between _raw and no contract?

- `_raw` is a special contract that accepts anything
- No contract means the topic doesn't exist in the system
- Always use `_raw` for unstructured data, not empty contract

## Next Steps

- **Understand structure**: [Data Models](data-models.md) - How hierarchies are created
- **Understand enforcement**: [Data Contracts](data-contracts.md) - How validation works
- **Understand types**: [Payload Shapes](payload-shapes.md) - What values are allowed
- **Advanced aggregation**: [Stream Processors](stream-processors.md) - Creating work orders from device data

---

## Reference: Technical Definitions

### Formal Component Definitions

#### Payload Shapes
**Definition**: JSON Schema definitions that specify the exact structure of message payloads
- **Scope**: Defines fields, types, constraints for a single message format
- **Built-in**: `timeseries-number`, `timeseries-string` (always available)
- **Custom**: User-defined schemas for relational data
- **Configuration**: `payloadShapes:` section of config.yaml (UI not available)

#### Data Models
**Definition**: Hierarchical structure defining the virtual path and fields after the data contract in UNS topics
- **Scope**: Creates the topic tree structure (folders, sub-models, fields)
- **Versioning**: Each model has explicit versions (v1, v2, etc.) that are immutable
- **Reusability**: Same model can be used across multiple locations
- **Configuration**: `dataModels:` section or via UI (auto-creates contracts)

#### Data Contracts
**Definition**: Enforcement mechanism that binds a specific model version and enables validation
- **Scope**: Links model + version, enforces at runtime
- **Enforcement Point**: UNS output plugin in bridges validates against contract
- **Naming**: Contract name becomes the data_contract segment in topics (e.g., `_pump_v1`)
- **Configuration**: `dataContracts:` section or auto-created via UI (view-only in UI)

### Data Flow Patterns

#### Device Modeling Pattern
```
External System → Bridge (with model) → Silver Contract → UNS
```
Most common pattern for equipment data.

#### Business Transformation Pattern
```
Multiple Silver Topics → Stream Processor → Gold Contract → UNS
```
For work orders and maintenance tracking.

#### Direct Gold Pattern
```
Business System → Bridge → Gold Contract → UNS
```
When data is already business-level (MES, ERP).
