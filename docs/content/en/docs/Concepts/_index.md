---
title: "Concepts"
linkTitle: "Concepts"
weight: 2
description: >
  The software of the United Manufacturing Hub is designed as a modular system. Our software serves as a basic building block for connecting and using various hardware and software components quickly and easily. This enables flexible use and thus the possibility to create comprehensive solutions for various challenges in the industry.
---

## Architecture

{{< imgproc dataprocessing Fit "1200x1200" >}}{{< /imgproc >}}

### Edge-device / IoT gateway

As a central hardware component we use an edge device which is connected to different data sources and to a server. The edge device is an industrial computer on which our software is installed. The customer can either use the United factorycube offered by us or his own IoT gateway. 

More information about our certified devices can be found on our [website](https://www.united-manufacturing-hub.com)

**Examples:**

- Factorycube
- Cubi

### Data acquisition

The data sources connected to the edge device provide the foundation for automatic data collection.  The data sources can be external sensors (e.g. light barriers, vibration sensors), input devices (e.g. button bars), Auto-ID technologies (e.g. barcode scanners), industrial cameras and other data sources such as machine PLCs. The wide range of data sources allows the connection of all machines, either directly via the machine PLC or via simple and fast retrofitting with external sensors.

More information can be found in the technical documentation of the edge helm chart [`factorycube-edge`](../developers/factorycube-edge)

**Examples:**

- sensorconnect
- barcodereader

### Data processing

The software installed on the edge device receives the data from the individual data sources. Using various data processing services and "node-red", the imported data is preprocessed and forwarded to the connected server via the MQTT broker.

More information can be found in the technical documentation of the edge and server helm chart [`factorycube-edge`](../developers/factorycube-edge) [`factorycube-server`](../developers/factorycube-server)


**Examples:**

- node-red

### Data storage

The data forwarded by the edge device can either be stored on the customer's servers or, in the SaaS version, in the United Cloud hosted by us. Relational data (e.g. data about orders and products) as well as time series data in high resolution (e.g. machine data like temperature) can be stored.

More information can be found in the technical documentation of the server helm chart [`factorycube-server`](../developers/factorycube-server)

**Examples:**

- TimescaleDB 

### Data usage

The stored data is automatically processed and provided to the user via a Grafana dashboard or other computer programs via a Rest interface. For each data request, the user can choose between raw data and various pre-processed data such as OEE, MTBF, etc., so that every user (even without programming knowledge) can quickly and easily compose personally tailored dashboards with the help of modular building blocks.

More information can be found in the technical documentation of the server helm chart [`factorycube-server`](../developers/factorycube-server)

**Examples:**

- Grafana
- factoryinsight


## Practical implications

### Edge devices

Typically you have multiple data sources like `sensorconnect` or `barcodereader`, that are containered in a Docker container. They all send their data to the MQTT broker. You can now process the data in node-red by subscribing to the data sources via MQTT, processing the data, and then writing it back.

### Server

#### Database access

The database on the server side should never be accessed directly by a service except `mqtt-to-postgresql` and `factoryinsight`. Instead, these services should be modified to include the required functionalities.
