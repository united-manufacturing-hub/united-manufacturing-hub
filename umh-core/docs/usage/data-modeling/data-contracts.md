# Data Contracts

Data contracts are the **enforcement mechanism** that makes data validation actually happen in the UNS. Without a contract, models and shapes are just documentation - contracts make them enforced rules.

## How Data Contracts Work

Data contracts operate at the gateway where data enters the UNS:

```
Device → Bridge → UNS Output Plugin → [Contract Check] → Kafka/Redpanda
                         ↑                    ↑
                  (in benthos-umh)     If contract exists, validate
```

**Key Concepts:**
1. **Contracts enforce validation** - Models and shapes alone do nothing
2. **Gateway enforcement** - The [UNS output plugin](https://docs.umh.app/benthos-umh/output/uns-output) validates all data
3. **No contract = No validation** - Data passes through if no contract exists
4. **Bridge degraded state** - Failed validation puts the bridge in degraded state for easy debugging

## Overview

Data contracts are stored in the `datacontracts:` configuration section:

```yaml
datacontracts:
  - name: _machine-state_v1  # Becomes part of the topic name
    model:
      name: machine-state     # References a data model
      version: v1             # Specific model version
```

## Core Properties

### Name and Versioning

```yaml
datacontracts:
  - name: _temperature_v1    # This exact string appears in topics
    model:
      name: temperature       # Which model to enforce
      version: v1            # Which version of that model
```

**Naming Convention:**
- Contract names start with underscore and include version (`_temperature_v1`, `_pump_v2`)
- The contract name becomes the data_contract segment in topics
- Version suffix (`_v1`, `_v2`) is part of the contract name, not separate

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

## How Validation Works

### The Gateway Pattern

All data enters the UNS through bridges or standalone flows - **never directly to Kafka/MQTT**. This ensures consistent validation:

```yaml
# In every bridge with read flow:
output:
  uns: {}  # Uses UNS output plugin - enforces contracts

# Never this (bypasses validation):
output:
  kafka: {}  # Direct Kafka - no contract enforcement!
```

### Processor Differences

How metadata gets set depends on your processor choice:

**With tag_processor (Time-Series bridges):**
```yaml
processors:
  - tag_processor:
      defaults: |
        msg.meta.location_path = "{{ .location_path }}";
        msg.meta.data_contract = "_raw";
        msg.meta.tag_name = msg.meta.opcua_tag_name;
        # Metadata set automatically, UNS plugin builds topic
```

**With nodered_js (Relational bridges):**
```yaml
processors:
  - nodered_js:
      code: |
        // Must manually set umh_topic!
        msg.meta.umh_topic = "umh.v1.plant._machine-state_v1.update";
        msg.meta.data_contract = "_machine-state_v1";
        // Build your relational payload
        msg.payload = { /* your complex data */ };
```

> **Important**: With `nodered_js`, you must set `umh_topic` manually. The UNS output plugin won't build it from metadata.

### What Happens During Validation

When data arrives at the UNS output plugin:

1. **Extract metadata**:
   - From `tag_processor`: Uses location_path, data_contract, tag_name to build topic
   - From `nodered_js`: Uses the manually set umh_topic

2. **Check contract existence**:
   - Contract exists → Validate payload against model
   - No contract → Data passes through without validation

3. **Validation result**:
   - ✅ Valid → Publish to Kafka topic
   - ❌ Invalid → Reject message, outputs WARN message, bridge goes to degraded state

### Example Validation

With contract `_pump_v1` referencing a pump model:

```yaml
# ✅ Valid - matches model structure
Topic: umh.v1.plant.line1._pump_v1.pressure
Payload: {"value": 42.5, "timestamp_ms": 1733904005123}

# ❌ Invalid - field not in model
Topic: umh.v1.plant.line1._pump_v1.invalid_field
Result: Bridge goes degraded, message rejected

# ✅ Valid - no contract, no validation
Topic: umh.v1.plant.line1._raw.anything
Payload: {"any": "data", "structure": "works"}
```

## Relationship to Stream Processors

**[Stream processors](../data-flows/stream-processor.md) do NOT use data contracts directly.** Instead, stream processors use templates that reference data models directly:

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
