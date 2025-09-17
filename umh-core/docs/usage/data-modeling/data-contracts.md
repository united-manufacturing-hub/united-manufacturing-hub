# Data Contracts

Data contracts bind data models to storage, retention, and processing policies, ensuring consistent data handling across your industrial systems.

Data contracts define the operational aspects of your data models - where data gets stored, how long it's retained, and what processing rules apply. They bridge the gap between logical data structure (models) and physical data management.

## Overview

Data contracts are stored in the `datacontracts:` configuration section and reference specific versions of data models:

```yaml
datacontracts:
  - name: contract_name
    model:
      name: modelname
      version: v1
```

## Core Properties

### Name and Versioning

```yaml
datacontracts:
  - name: _temperature_v1
    model:
      name: temperature
      version: v1
```

**Naming Convention:**
- Contract names start with underscore (`_temperature`, `_pump`)
- Model references include name and version (`name: temperature, version: v1`)

### Model Binding

Each contract binds to exactly one data model version:

```yaml
datacontracts:
  - name: _pump_v1
    model:
      name: pump
      version: v1  # Specific model version
```

This binding is immutable - to change the model, create a new contract version.

### Data Bridges

Contracts specify where data gets stored and processed:

```yaml
datacontracts:
  - name: _temperature_v1
    model:
      name: temperature
      version: v1
```

## Validation and Enforcement

### Schema Enforcement

All data contracts are registered in Redpanda Schema Registry:

- **Publish-time validation**: Messages are validated before acceptance
- **Consumer protection**: Invalid messages are rejected automatically
- **Evolution safety**: Schema changes must maintain compatibility

### Runtime Validation

The UNS output plugin enforces contract compliance:

```yaml
# Invalid message - rejected
Topic: umh.v1.corpA.plant-A.line-4.pump41._pump_v1.invalid_field
Reason: Field 'invalid_field' not defined in _pump model, version v1

# Valid message - accepted
Topic: umh.v1.corpA.plant-A.line-4.pump41._pump_v1.pressure
Payload: {"value": 42.5, "timestamp_ms": 1733904005123}
```

## Relationship to Stream Processors

**Stream processors do NOT use data contracts directly.** Instead, stream processors use templates that reference data models directly:

```yaml
# Template references model directly, not contract
templates:
  streamProcessors:
    pump_template:
      model:
        name: pump      # Direct model reference
        version: v1
      sources: {...}
      mapping: {...}

# Stream processor uses template
streamProcessors:
  - name: pump41_sp
    _templateRef: "pump_template"
    location: {...}
    variables: {...}
```

**Data contracts are separate** - they define storage bridges and retention policies for data models. Stream processors work with models directly through templates, but **if a data contract exists for the same model**, the stream processor's output will be automatically validated against that contract and routed to the contract's configured bridges.
