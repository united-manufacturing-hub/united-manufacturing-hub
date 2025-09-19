# Stream Processors

> **Prerequisite:** Understand [Data Flow concepts](README.md) and [Data Modeling](../data-modeling/).

Stream processors are specialized data flows that transform existing UNS data into different structures, creating business-oriented views from device data.

## Overview

Stream processors differ from other data flows:
- **Bridges**: Connect external devices → UNS
- **Stand-alone Flows**: Custom point-to-point processing
- **Stream Processors**: Transform UNS data → different UNS structure

They're part of the [data modeling system](../data-modeling/stream-processors.md) for Silver → Gold transformations.

## When to Use Stream Processors

| Scenario | Solution |
|----------|----------|
| Get data from one device into a model | Bridge with data contract |
| Combine data from multiple devices | Multiple bridges → same model |
| **Create business view from device data** | **Stream Processor** |
| **Aggregate time-series into KPIs** | **Stream Processor** |
| **Generate events from state changes** | **Stream Processor** (future) |

## How They Work as Data Flows

### Data Flow Characteristics

Stream processors are managed data flows with:

1. **Automatic Kafka Consumer Groups**: Each processor gets a unique consumer group (hex-encoded name) for offset tracking
2. **Topic Subscriptions**: Subscribe to specific UNS topics (no wildcards)
3. **Dependency-based Processing**: Only process when required source data arrives
4. **Guaranteed Output Structure**: Publish to model-defined topics

### Processing Pipeline

```
Source Topics → Stream Processor → Output Topics
(Silver data)     (Transform)      (Gold data)
```

Example flow:
```
umh.v1.plant.line._raw.temperature ─┐
                                     ├→ Processor → umh.v1.plant.line._pump_v1.inlet_temp
umh.v1.plant.line._raw.pressure ────┘              └→ umh.v1.plant.line._pump_v1.outlet_temp
```

## Time-Series to Relational Challenges

Stream processors currently handle **time-series data only**. Creating relational records from time-series involves complex decisions:

### The Fundamental Problem

PLCs and sensors provide continuous streams:
```
Time    | machine_state | temperature | count
--------|---------------|-------------|-------
10:00:01| RUNNING       | 45.2        | 100
10:00:02| RUNNING       | 45.3        | 101
10:00:03| STOPPED       | 45.1        | 101
10:00:04| IDLE          | 44.9        | 101
```

But business systems need discrete records:
```json
{
  "batch_id": "BATCH-123",
  "start": "10:00:01",
  "end": "10:00:03",
  "total_produced": 1,
  "avg_temperature": 45.2,
  "result": "COMPLETE"
}
```

### Key Challenges

1. **Record Boundaries**
   - When does a batch start? (state change? operator input? time?)
   - When is it complete? (state change? count reached? timeout?)

2. **Data Completeness**
   - What if temperature arrives late?
   - What if count never updates?
   - How long to wait for all values?

3. **Edge Cases**
   - Machine stops mid-batch
   - Network interruption causes gaps
   - Values arrive out of order
   - Clock synchronization issues

4. **State Management**
   - Track partial records
   - Handle overlapping events
   - Manage timeout conditions

### Current Solutions

Until relational output is supported:

**Option 1: Use Stand-alone Flows**
```yaml
dataFlow:
  - name: batch-detector
    dataFlowComponentConfig:
      benthos:
        input:
          uns:
            topics: ["umh.v1.+.+.+.+._raw.machine_state"]
        pipeline:
          processors:
            - mapping: |
                # Detect state changes and create events
                if this.value == "STOPPED" && meta("previous_state") == "RUNNING" {
                  root.event = "BATCH_END"
                  root.batch_data = meta("accumulated_data")
                }
        output:
          sql_insert:  # Write to relational database
            table: "batch_records"
```

**Option 2: External Processing**
- Use stream processors for aggregation
- External service reads aggregated data
- Business logic creates relational records

See [Payload Formats](../unified-namespace/payload-formats.md#edge-cases-and-considerations) for detailed guidance.

## Configuration as Data Flow

Stream processors are configured through the data modeling system but execute as data flows:

```yaml
# Template defines the transformation
templates:
  streamProcessors:
    pump_monitor:
      model:
        name: pump
        version: v1
      sources:
        temp_in: "{{ .location_path }}._raw.inlet_temp"
        temp_out: "{{ .location_path }}._raw.outlet_temp"
      mapping:
        efficiency: "(temp_out - temp_in) / temp_in * 100"
        status: "temp_out > 80 ? 'WARNING' : 'OK'"

# Instance creates the data flow
streamprocessors:
  - name: pump1_monitor
    _templateRef: "pump_monitor"
    location:
      0: plant
      1: building_a
      2: line_1
      3: pump_1
```

This configuration creates a managed data flow that:
1. Subscribes to `umh.v1.plant.building_a.line_1.pump_1._raw.inlet_temp|outlet_temp`
2. Calculates efficiency and status
3. Publishes to `umh.v1.plant.building_a.line_1.pump_1._pump_v1.efficiency|status`

## Monitoring Stream Processors

Stream processors are visible in the **Data Flows → Stream** tab where you can:

![Stream Processors List](../data-modeling/images/stream-processors-overview.png)

- **View all processors**: Listed with instance and throughput
- **Add new processors**: Click "Add Stream Processor" button
- **Monitor status**: Check real-time throughput metrics
- **Access management**: Right-click for logs, metrics, and configuration

Additional monitoring through:
1. **Kafka Consumer Groups**: Check offset lag and consumption rate
2. **Output Topics**: Verify data is being produced
3. **Logs**: Debug transformation issues

## Comparison with Other Data Flows

| Aspect | Bridges | Stand-alone | Stream Processors |
|--------|---------|-------------|-------------------|
| **Input** | External protocols | Any source | UNS topics only |
| **Output** | UNS topics | Any destination | UNS model topics |
| **Structure** | Tag-based | Free-form | Model-enforced |
| **Processing** | Tag processor | Any Benthos processor | JavaScript expressions |
| **Management** | UI + YAML | YAML only | UI + Templates |
| **Use Case** | Device connectivity | Custom integration | Data transformation |

## Next Steps

- Learn [Stream Processor configuration](../data-modeling/stream-processors.md) in detail
- Understand [Data Models](../data-modeling/data-models.md) that define output structure
- Explore [Bridges](bridges.md) for device connectivity
- See [Stand-alone Flows](stand-alone-flow.md) for custom processing