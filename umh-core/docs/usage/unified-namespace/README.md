# Unified Namespace

The Unified Namespace (UNS) is the core messaging backbone of UMH Core, providing a standardized, event-driven architecture that eliminates point-to-point integration complexity.

## Key Concepts

Traditional manufacturing systems create "spaghetti diagrams" with point-to-point connections between every system. The UNS flips this model:

- **Event-driven**: Devices publish data as events happen
- **Standard hierarchy**: Flexible location paths (supports ISA-95, KKS, or custom naming)
- **Schema enforcement**: Data contracts ensure consistent payload formats
- **Publish-regardless**: Producers don't need to know about consumers

## Documentation Structure

- **[Overview](overview.md)** - Core UNS concepts and benefits
- **[Topic Convention](topic-convention.md)** - Hierarchical naming structure (ISA-95 compatible)
- **[Payload Formats](payload-formats.md)** - Message structure and data types
- **[Producing Data](producing-data.md)** 🚧 - How to publish data to the UNS
- **[Consuming Data](consuming-data.md)** 🚧 - How to subscribe and process UNS data

## Related Documentation

- **[Data Modeling](../data-modeling/README.md)** - Structure your industrial data with reusable models
- **[Data Flows](../data-flows/README.md)** - Connect external systems via Bridges and Stand-alone Flows
- **[Configuration Reference](../../reference/configuration-reference.md)** - YAML configuration for UNS integration

## Learn More

For deeper understanding of UNS concepts, see our educational content:
- [The Rise of the Unified Namespace](https://learn.umh.app/lesson/chapter-2-the-rise-of-the-unified-namespace/) - Core UNS principles
- [NAMUR Open Architecture versus Unified Namespace](https://learn.umh.app/blog/namur-open-architecture-versus-unified-namespace-two-sides-of-the-same-coin/) - Industrial standards context
- [What is MQTT? Why Most MQTT Explanations Suck](https://learn.umh.app/blog/what-is-mqtt-why-most-mqtt-explanations-suck-and-our-attempt-to-fix-them/) - MQTT vs UNS comparison

