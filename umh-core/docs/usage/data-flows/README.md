# Data Flows

> **Prerequisite:** Complete the [Getting Started guide](../../getting-started/) to understand basic connections and data organization.

Data flows move and transform industrial data in UMH Core. Three types serve different purposes.

Bridges and stream processors are [product features](../../product-features-and-supported-paths.md#product-features). Stand-alone flows are a [supported path](../../product-features-and-supported-paths.md#supported-paths).

## The Data Flows card

The Data Flows card on the instance details page counts the data flows of the instance.

| Field | Shows |
|---|---|
| Status badge | The health of the data flows as a whole. |
| Data Flows | The number of bridges, stand-alone flows and stream processors. It links to the Data Flows list, filtered to this instance. |
| Bridges | The number of bridges. It links to the list of this instance's bridges. |
| Status Breakdown | How many data flows are **Active**, **Degraded**, **Other** (Neutral) and **Unknown** (no health reported). It appears once the instance has a data flow. |

## Flow Types

### [Bridges](bridges.md)
Connect the Unified Namespace to one external system each. The read flow reads from the external system and publishes to the UNS, and the write flow subscribes to UNS topics and writes to the external system. A connection probe checks whether the external system is reachable, and the data is published under the bridge location. Support for 50+ industrial protocols (OPC UA, Modbus, S7) plus IT systems.

### [Stand-alone Flows](stand-alone-flow.md)
Raw Benthos access for custom processing when bridges or stream processors aren't sufficient. Used as fallback for write flows, external integrations, and specialized transformations.

### [Stream Processors](stream-processor.md)
Transform existing UNS data into different structures. Part of the [data modeling system](../data-modeling/stream-processors.md) for aggregating device data into business views.

## Quick Start

Most users start with a bridge that reads from a PLC or another external system:

1. **Data Flows** → **Add Bridge**
2. Select your protocol (OPC UA, Modbus, S7)
3. Enter the external system's address and port, and the bridge's levels
4. Deploy to start data collection

## Learn More

- [Getting Started Guide](../../getting-started/) - Create your first bridge
- [Data Modeling](../data-modeling/) - Structure and validate data
- [Unified Namespace](../unified-namespace/) - Understanding topics and payloads