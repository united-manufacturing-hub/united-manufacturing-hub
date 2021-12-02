
---
title: "2. Connecting machines and creating dashboards"
linkTitle: "2. Connecting machines and creating dashboards"
weight: 2
description: >
  This section explains how the United Manufacturing Hub is used practically 
---

## 0. Video: From sensor to dashboard

TODO
#455

## 1. Introduction

UMH Hub has a wide range of ports and connectors. 

{{< imgproc logos_data_sources Fit "800x800" >}}{{< /imgproc >}}

These are:

- OPC/UA ([documentation for this node](https://flows.nodered.org/node/node-red-contrib-opcua))
- Siemens S7 ([documentation for this node](https://flows.nodered.org/node/node-red-contrib-s7))
- TCP/IP ([documentation for this node](https://flows.nodered.org/flow/bed6f676d088670d7e1bc298943338b5))
- Rest API  ([documentation for this node](https://cookbook.nodered.org/http/create-an-http-endpoint))
- Modbus  ([documentation for this node](https://flows.nodered.org/node/node-red-contrib-modbus))
- MQTT ([documentation for this node](https://cookbook.nodered.org/mqtt/))

Via Node-RED, these data sources can be read out easily or, in the case of IO-Link, are even read out automatically and made available via MQTT. 

Using these data points and our data model, the data can then be converted into a standardized data model. This is done independently of machine type and manufacturer and can be made usable for various data services, e.g. Grafana.

## 2. Node-RED

To extract and preprocess the data from different data sources, we use the open source software Node-RED. Node-RED is a low-code programming for event-driven applications. 

Originally, Node-RED comes from the area of smart home and programming implementations, but is also used in manufacturing more frequently. For more information feel free to check [this article](https://docs.umh.app/docs/concepts/node-red-in-industrial-iot/).

In the following, the procedure for creating a Node-RED flow is described in detail using **two examples**. The first example is about a cutting machine which will be retrofitted by external sensor technology. The second example deals with the connection of already existing sensors from with our system in order to visualize the acquired data.

### 1st example: Connecting external sensor technology

You would like to determine the output and machine condition of a cutting machine.

Used Sensors:

- Light Barrier
- Button Bar

With the help of Sensorconnect, sensors can be connected quickly and easily via an IFM gateway. The sensor values are automatically extracted from the software stack and made available via [MQTT](https://docs.umh.app/docs/concepts/mqtt/). 

*1. Connect Sensors*

TODO: Picture or illustration of how to connect sensors

*2. Make the connected sensors visible to our system*

Based on an IP address, which is assigned to each sensor (or gateway?), the sensor can be integrated into our system. Only then is it possible to read out sensor values. The adaptation of the IP range is required.

TODO: ADAPTION OF IP-RANGE

*3. Creating a flow*

In Node-RED basically three pieces of information must be communicated to the system.

- The customer ID to be assigned to the asset: *customerID*

- The location where the asset is located: *location*

- The name of the asset: *AssetID*

*3.1 First node: MQTT-IN*

TODO: PICTURE
TODO: Introduce the first node briefly, then explain it in detail (topic etc.)

In the topic of our **first node (MQTT IN)** (PICTURE) all these three information are bundled to get a MQTT input.

The topic structure is: `ia/raw/<transmitterID>/<gatewaySerialNumber>/<portNumber>/<IOLinkSensorID>`

To get a quick and easy overview of the available MQTT messages and topics we recommend the MQTT Explorer. If you don’t want to install any extra software you can use the MQTT-In node to subscribe to all available topics by subscribing to # and then direct the messages of the MQTT in nodes into a debugging node. You can then display the messages in the nodered debugging window and get information about the topic and available data points.

An example for an ia/raw/ topic is: `ia/raw/2020-0102/0000005898845/X01/210-156`

This means that an IFM gateway with serial number `0000005898845` is connected to a transmitter with serial number `2020-0102`. This gateway has connected the sensor `210-156` to the first port `X01`.

*3.2 Second node: JSON*

TODO: PICTURE

The **second node (JSON)** is a generic container of elements inside a JSON stream is called JSON. It can contain fundamental types (integers, booleans, floating point numbers, strings) and complex types (arrays and objects) and is used to convert between two formats.

TODO: INFORMATION IN JSON

*3.3 Theird node: Function*

TODO: Following nodes...

### 2nd example: Integration of existing sensors

This chapter describes how to connect already existing sensors/ integrate an already existing interface (OPC/UA) with our system.

## 3. Create dashboards with Grafana

TODO
