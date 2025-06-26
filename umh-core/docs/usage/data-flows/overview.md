# Overview

Data flows in UMH Core enable seamless integration between industrial devices and the unified namespace through configurable pipelines. They provide flexible mechanisms for data ingestion, transformation, and routing, allowing you to connect various protocols (OPC UA, Modbus, S7comm) to standardized data streams. Understanding the different flow types and their configurations is essential for building effective industrial data architectures.

> **Heads-up for UMH Classic users:** every protocol-conversion pipeline was a separate Benthos pod in Classic. In UMH Core each pipeline is an S6-managed Benthos *process* inside the single container, giving faster restarts and a smaller resource footprint.

