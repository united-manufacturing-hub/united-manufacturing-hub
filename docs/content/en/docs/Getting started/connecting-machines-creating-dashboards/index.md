
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

These are e.g. 

- OPC/UA ([documentation for this node](https://flows.nodered.org/node/node-red-contrib-opcua))
- Siemens S7 ([documentation for this node](https://flows.nodered.org/node/node-red-contrib-s7))
- TCP/IP ([documentation for this node](https://flows.nodered.org/flow/bed6f676d088670d7e1bc298943338b5))
- Rest API  ([documentation for this node](https://cookbook.nodered.org/http/create-an-http-endpoint))
- Modbus  ([documentation for this node](https://flows.nodered.org/node/node-red-contrib-modbus))
- MQTT ([documentation for this node](https://cookbook.nodered.org/mqtt/))

Via Node-RED, these data sources can be read out easily or, in the case of IO-Link, are even read out automatically and made available via MQTT. 

Using these data points and our data model, the data can then be converted into a standardized data model. This is done independently of machine type and manufacturer and can be made usable for various data services, e.g. Grafana.

## 2. Node-RED

TODO: Introduce why Node-RED is the standard (reference to https://docs.umh.app/docs/concepts/node-red-in-industrial-iot/).

- Explain Node-RED on the basis of exemplary cases
1st example: Retrofit with the help of external sensor technology (Sensorconnect incl. adaptation of IP range, MQTT)
2nd example: connection to interface (OPC/UA).
- Integrate "general configuration" into examples
- Just use MQTT outputs

## 3. Create dashboards using factorycube-server

TODO
