# Data Models

> This article assumes you've completed the [Getting Started guide](../../getting-started/) and understand the [data modeling concepts](README.md).

Data models define the hierarchical structure of your industrial data. They create the virtual paths and fields that organize raw data into meaningful information.

## Overview

In the [component chain](README.md#the-component-chain), models provide the structure:

```
Payload Shapes → Data Models → Data Contracts → Data Flows
                      ↑
                 Structure defined here
```

When you create a data model, you're defining:
- **Virtual paths**: Organizational folders (e.g., `vibration`, `motor.electrical`)
- **Fields**: Data endpoints with specific types (e.g., `temperature`, `pressure`)
- **Relationships**: How components nest and reference each other

## UI Capabilities

The Management Console provides full control over data models:

| Feature | Available | Notes |
|---------|-----------|-------|
| View model list | ✅ | Shows all models with versions |
| Create models | ✅ | Visual editor with YAML preview |
| View model details | ✅ | Inspect structure and configuration |
| Create new versions | ✅ | Models are immutable, edit by versioning |
| Reference sub-models | ✅ | Link to other models via `_refModel` |
| Delete models | ✅ | Remove unused model versions |
| Direct editing | ❌ | Use "New Version" to modify |

![Data Models List](images/2-data-models-list.png)

**What you see in the UI:**
- **Name**: Model identifier (e.g., `cnc`, `pump`, `temperature-sensor`)
- **Instance**: Which UMH instance owns the model
- **Description**: Optional description of the model's purpose
- **Latest**: Current version number (v1, v2, etc.)

### Model Actions

Click the three-dot menu (⋮) on any model to access actions:

![Data Model Actions](images/2-data-model-actions.png)

- **Data Model**: View the model's structure and YAML configuration
- **New Version**: Create a new version with modifications (since models are immutable)
- **Delete**: Remove the model (only if not in use by contracts or bridges)

![Data Model Creation](images/2-data-model-add.png)

## Configuration

### Basic Structure

```yaml
datamodels:
  - name: pump                    # Model name
    description: "Pump monitoring" # Optional description
    version:
      v1:                         # Version identifier
        structure:                # Hierarchical definition
          pressure:
            inlet:
              _payloadshape: timeseries-number
            outlet:
              _payloadshape: timeseries-number
```

### How Structure Becomes Topics

Model structure directly maps to UNS topics:

```yaml
structure:
  vibration:        # Creates: .../_pump_v1.vibration
    x-axis:         # Creates: .../_pump_v1.vibration.x-axis
```

**Complete topic path:**
```
umh.v1.enterprise.site._pump_v1.vibration.x-axis
       └─ fixed ─┘     └contract┘└─from model─┘
```

## The Three Building Blocks

### 1. Fields - Data Endpoints

```yaml
temperature:
  _payloadshape: timeseries-number  # Accepts numeric values
```

**Characteristics:**
- Has `_payloadshape` property
- Creates a topic endpoint that accepts data
- References a [payload shape](payload-shapes.md) for validation
- Cannot have child elements

### 2. Folders - Organizational Structure

```yaml
vibration:           # Folder - no _payloadshape
  x-axis:           # Field inside folder
    _payloadshape: timeseries-number
  y-axis:           # Field inside folder
    _payloadshape: timeseries-number
```

**Characteristics:**
- No `_payloadshape` property
- Groups related fields or other folders
- Creates hierarchy in topic path
- Can nest multiple levels deep

### 3. Sub-Models - Reusable Components

Define once, use everywhere:

```yaml
# Define reusable motor model
datamodels:
  - name: motor
    version:
      v1:
        structure:
          rpm:
            _payloadshape: timeseries-number
          temperature:
            _payloadshape: timeseries-number

# Reference in pump model
datamodels:
  - name: pump
    version:
      v1:
        structure:
          pressure:
            inlet:
              _payloadshape: timeseries-number
          motor:           # Include the motor model
            _refModel:
              name: motor
              version: v1
```

**Result:** Topics created:
- `_pump_v1.pressure.inlet`
- `_pump_v1.motor.rpm`
- `_pump_v1.motor.temperature`

**Benefits:**
- Single source of truth
- Consistent structure across models
- Update once, reflected everywhere

## Version Evolution

Models are immutable once created. To add fields, create a new version:

### Why Immutability?

From the [README](README.md#why-are-models-immutable):
- Models are contracts between teams
- Dashboards depend on stable structure
- Historical data queries must not break

### Evolution Pattern

**Version 1 - Basic:**
```yaml
version:
  v1:
    structure:
      temperature:
        _payloadshape: timeseries-number
```

**Version 2 - Add pressure:**
```yaml
version:
  v2:
    structure:
      temperature:
        _payloadshape: timeseries-number
      pressure:                          # New field
        _payloadshape: timeseries-number
```

**Migration steps:**
1. Create v2 with additions
2. Deploy new bridges using v2
3. Update dashboards to v2
4. Keep v1 running during transition
5. Deprecate v1 when safe

## Relationship to Contracts

Models define structure, but don't enforce it. That's where [data contracts](data-contracts.md) come in:

| Component | Purpose | Example |
|-----------|---------|----------|
| **Model** | Defines structure | `pump` model with fields |
| **Contract** | Enforces structure | `_pump_v1` validates messages |

**Auto-creation:** When you create a model in the UI:
1. Model `pump` version `v1` is created
2. Contract `_pump_v1` is auto-generated
3. Contract becomes available in bridges

Without a contract, a model is just documentation. With a contract, it becomes validation.

## Next Steps

- **Enforce validation**: [Data Contracts](data-contracts.md) - Make models mandatory
- **Define value types**: [Payload Shapes](payload-shapes.md) - Specify data types for fields
- **Create aggregations**: [Stream Processors](stream-processors.md) - Transform Silver to Gold data
