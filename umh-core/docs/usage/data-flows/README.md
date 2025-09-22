# Data Flows

> **Prerequisite:** Complete the [Getting Started guide](../../getting-started/) to understand basic connections and data organization.

Data flows move and transform industrial data in UMH Core. Three types serve different purposes:

## Flow Types

### [Bridges](bridges.md)
Move data into and out of the Unified Namespace with connection monitoring and automatic location path. Support for 50+ industrial protocols (OPC UA, Modbus, S7) plus IT systems. Write flows coming soon - use stand-alone flows meanwhile.

### [Stand-alone Flows](stand-alone-flow.md)
Raw Benthos access for custom processing when bridges or stream processors aren't sufficient. Used as fallback for write flows, external integrations, and specialized transformations.

### [Stream Processors](stream-processor.md)
Transform existing UNS data into different structures. Part of the [data modeling system](../data-modeling/stream-processors.md) for aggregating device data into business views.

## Quick Start

Most users start with bridges to connect devices:

1. **Data Flows** → **Add Bridge**
2. Select your protocol (OPC UA, Modbus, S7)
3. Configure connection and location
4. Deploy to start data collection

## Learn More

- [Getting Started Guide](../../getting-started/) - Connect your first device
- [Data Modeling](../data-modeling/) - Structure and validate data
- [Unified Namespace](../unified-namespace/) - Understanding topics and payloads