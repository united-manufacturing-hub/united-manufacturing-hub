# Stream Processors

> **Prerequisite:** Understand [Data Flow concepts](README.md) and complete the [Getting Started guide](../../getting-started/).

Stream processors transform data already inside the Unified Namespace, aggregating multiple device streams into business KPIs and metrics. They're the third type of data flow, specialized for transforming device models into business models.

## When to Use

Use stream processors for:
- Aggregating data from multiple devices into KPIs
- Creating business metrics from raw sensor data
- Calculating OEE, efficiency, or other derived values
- Reducing data volume for cloud transmission

Use bridges instead for:
- Getting data into the UNS initially
- Single device to model mapping
- Direct protocol connections

## UI Capabilities

| Action | Available | Notes |
|--------|-----------|-------|
| Create processors | ✅ | Visual configuration with topic browser |
| Select source topics | ✅ | Browse and multi-select |
| Map to model fields | ✅ | Expression-based mapping |
| View processors | ✅ | Listed in Stream tab |
| Monitor throughput | ✅ | Real-time message rates |
| Edit processors | ✅ | Modify configuration |
| Delete processors | ✅ | Remove when not needed |
| Code mode | ✅ | Direct YAML editing |

## Creating a Stream Processor

Stream processors are created through the **Data Flows → Stream** tab:

![Stream Processors List](../data-modeling/images/stream-processors.png)

### Step 1: General Configuration

Configure basic settings and location hierarchy:

![Stream Processor General Configuration](../data-modeling/images/stream-processors-add.png)

- **Name**: Unique identifier for your processor
- **Instance**: The UMH Core instance to run on
- **Data Contract**: Select the output model (creates validated structure)
- **Location Levels**: Set where in the hierarchy to output data (Level 0-4)

### Step 2: Topic Selection

Select source topics and they'll automatically get variable names:

![Topic Selection and Variable Names](../data-modeling/images/stream-processors-add-topic-expression.png)

- Browse available topics in your UNS
- Select multiple sources with checkboxes
- Variables auto-generated (e.g., `_10` for first topic, `_10_2` for second)
- Use "Show Selected Only" to filter view
- Copy variable names for use in expressions

### Step 3: Expression Mapping

Map source variables to model fields using expressions:

![Expression Mapping](../data-modeling/images/stream-processors-add-topic-expression.png)

- Each field in your data model appears on the right
- Enter expressions using the source variables
- Simple passthrough: `source_alias`
- Calculations: `_10 + _10_2` or more complex formulas
- The expressions support standard mathematical operations

### Step 4: Code Mode (Advanced)

Switch to Code Mode to see or edit the generated YAML configuration:

![Code Mode Configuration](../data-modeling/images/stream-processors-add-code-mode.png)

The UI automatically generates YAML with:
- **sources**: Your selected topics with location_path templates
- **mapping**: Field-to-expression mappings
- Comments explaining the structure

You can switch between UI and Code Mode anytime to fine-tune configurations.

**For detailed configuration instructions**, see the comprehensive [Stream Processors guide](../data-modeling/stream-processors.md) in Data Modeling.

## How It Works

Stream processors subscribe to multiple UNS topics and output to a single model-based topic:

```mermaid
flowchart LR
    A["Multiple Device Model Topics<br/>(e.g., _pump_v1, _cnc_v1)"] --> B["Stream Processor<br/>(aggregation)"] --> C["Business Model Topic<br/>(e.g., _maintenance_v1)"]
```

Example: Combining temperature and pressure from different sensors into a pump model:
- Input: `enterprise.site._raw.temp_sensor_1`, `enterprise.site._raw.pressure_gauge_2`  
- Output: `enterprise.site._pump_v1` with calculated efficiency

The system handles:
- Consumer group management for offset tracking
- Dependency-based processing (waits for all required inputs)
- Guaranteed output structure via data models

## Configuration (YAML)

While the UI is the primary way to create stream processors, they're stored as YAML:

```yaml
streamProcessor:
  - name: pump_efficiency_calc
    desiredState: active
    streamProcessorServiceConfig:
      config:
        model:
          name: pump
          version: v1
        sources:
          inlet_temp: enterprise.site._raw.inlet_temp
          outlet_temp: enterprise.site._raw.outlet_temp
        mapping:
          efficiency: "(outlet_temp - inlet_temp) / inlet_temp * 100"
```

See the [Stream Processors configuration guide](../data-modeling/stream-processors.md) for detailed YAML structure.

## Key Differences from Other Data Flows

| Data Flow Type | Purpose | Input Source | Output |
|----------------|---------|--------------|--------|
| **Bridges** | Get data INTO the UNS | External devices (PLCs, sensors) | UNS topics |
| **Stream Processors** | Transform data WITHIN the UNS | UNS topics | UNS model topics |
| **Stand-alone Flows** | Custom processing | Any source | Any destination |

## Metrics

[Metrics](../instances/agent/metrics.md) explains how to read umh-core's metrics.

The Management Console shows each stream processor's message counts. Open the stream processor and select **Metrics** on the Monitoring card. The view shows messages received and sent, errors and latency. Port 8080, umh-core's Prometheus endpoint, does not carry message counts.

Port 8080 carries the stream processor's state and error counts. Stream processors run on FSMv1, the older of umh-core's two frameworks. A framework is the part of umh-core that starts and watches components. [Which framework runs a component](../instances/agent/metrics.md#which-framework-runs-a-component) explains both. Stream processors get the series described in [FSMv1 metrics](../instances/agent/metrics.md#fsmv1-metrics). For a stream processor named `oee-rollup`, the series carry these labels:

| Part | `component` | `instance` |
|---|---|---|
| The stream processor | `stream_processor_instance` | `oee-rollup` |
| The internal flow that runs it | `dataflow_component_instance` | `dfc-streamprocessor-oee-rollup` |
| The Benthos process of that flow | `benthos_instance` | `dataflow-dfc-streamprocessor-oee-rollup` |

Benthos is the engine that moves the data. Prometheus can rename the `instance` label to `exported_instance`. [Read the metrics](../instances/agent/metrics.md#read-the-metrics) explains when.

For the stream processor itself, `umh_core_service_current_state` reads 3 when active, 4 when idle and 0 when stopped. -1 means the stream processor is in a sub-state: one of the starting, degraded or stopping steps, such as `starting_dfc` or `degraded_redpanda`. Check the state in the Management Console.

## Learn More

For comprehensive stream processor configuration and examples, see the [Stream Processors guide](../data-modeling/stream-processors.md) in Data Modeling documentation.