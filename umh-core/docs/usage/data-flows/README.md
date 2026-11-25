# Data Flows

> **Prerequisite:** Complete the [Getting Started guide](../../getting-started/) to understand basic connections and data organization.

Data flows move and transform industrial data in UMH Core. Three types serve different purposes:

## Flow Types

### [Bridges](bridges.md)
Connect the Unified Namespace to one external system each. The [read flow](read-flow.md) reads from the external system and publishes to the UNS, and the [write flow](write-flow.md) subscribes to UNS topics and writes to the external system. A connection probe checks whether the external system is reachable, and the data is published under the bridge location. Support for 50+ industrial protocols (OPC UA, Modbus, S7) plus IT systems.

### [Stand-alone Flows](stand-alone-flow.md)
Raw Benthos access for custom processing when bridges or stream processors aren't sufficient. Used for external integrations and specialized transformations.

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