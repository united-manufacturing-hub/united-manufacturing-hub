# Overview

Traditional ISA-95 "automation-pyramid" integrations move data **upwards** through\
a stack of firewalls and proprietary databases. Every new use-case requires another\
point-to-point path; the result is _spaghetti diagrams_, data loss through\
aggregation, and weeks of lead-time each time someone asks\
"can I please add this tag to the dashboard?"

The **Unified Namespace** flips that flow:

* **Event driven** – every device, PLC or app _publishes_ its own events as\
  they happen.
* **Standard topic hierarchy** – an ISA-95-compatible key encodes enterprise,\
  site, area, … plus a _data-contract_ that describes the payload, data sources and data sinks.
* **Publish-regardless mentality** – producers do not care whether a consumer\
  exists yet; that turns "add a new dashboard" into a _reading_ exercise, not\
  a PLC­-reprogramming project.
* **Stream Processing / Data Contextualization** - Bridges give incoming data already a base contextualization and enforce schemas, Stream Processor allows you then to properly model the data top-down.

More information on our UMH Blog ["The Rise of the Unified Namespace"](https://learn.umh.app/lesson/chapter-2-the-rise-of-the-unified-namespace/)
