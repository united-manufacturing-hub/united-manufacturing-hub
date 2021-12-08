
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

The United Manufacturing Hub has a wide variety of connectors and therefore offers maximum connectivity to different machines and systems.

{{< imgproc logos_data_sources Fit "800x800" >}}{{< /imgproc >}}

Here are a few selected examples:

- OPC/UA ([documentation for this node](https://flows.nodered.org/node/node-red-contrib-opcua))
- Siemens S7 ([documentation for this node](https://flows.nodered.org/node/node-red-contrib-s7))
- TCP/IP ([documentation for this node](https://flows.nodered.org/flow/bed6f676d088670d7e1bc298943338b5))
- Rest API  ([documentation for this node](https://cookbook.nodered.org/http/create-an-http-endpoint))
- Modbus  ([documentation for this node](https://flows.nodered.org/node/node-red-contrib-modbus))
- MQTT ([documentation for this node](https://cookbook.nodered.org/mqtt/))

Via Node-RED, these data sources can be read out easily or, in the case of IO-Link, are even read out automatically and made available via MQTT. 

Using these data points and our data model, the data can then be converted into a standardized data model. This is done independently of machine type and manufacturer and can be made usable for various data services, e.g. Grafana.

## 2. Extracting and preparing data with the help of Node-RED

To extract and preprocess the data from different data sources, we use the open source software Node-RED. Node-RED is a low-code programming for event-driven applications. 

Originally, Node-RED comes from the area of smart home and programming implementations, but is also used in manufacturing more frequently. For more information feel free to check [this article](https://docs.umh.app/docs/concepts/node-red-in-industrial-iot/).

In the following, the procedure for creating a Node-RED flow is described in detail using **two examples**. 

1. Retrofit a cutting machine with the help of external sensors
2. Using OPC/UA to read a warping machine data directly from the PLC

### 1st example: Connecting external sensor technology

You would like to determine the output and machine condition of a cutting machine.

Used Sensors:

- Light Barrier
- Button Bar

With the help of Sensorconnect, sensors can be connected quickly and easily via an IFM gateway. The sensor values are automatically extracted from the software stack and made available via [MQTT](https://docs.umh.app/docs/concepts/mqtt/). 

*1. Connect Sensors*

TODO: Picture or illustration of how to connect sensors

*2. Make the connected sensors visible to our system*

The gateways to which sensors are connected are found by our microservice in the local network. For this purpose, the IP address range must be communicated to the microservice so that it can search for the gateway in the correct network and read out the sensors via it.

TODO: ADAPTION OF IP-RANGE

*3. Creating a Node-RED flow*

With Node-Red, it is possible to quickly develop applications and prepare untreated sensor data so that Grafana, for example, can use it. For this purpose, so-called nodes are drawn onto a surface and wired together to create sequences. 

In the following, the creation of such Node-RED flows is demonstrated using the example sensors button bar, light barrier and inductive sensor. 

The first two nodes are the same for all connected sensors. Only after that we will distinguish between the different sensors.

*3.1 First node: MQTT-In*

{{< imgproc mqtt_in Fit "800x800" >}}{{< /imgproc >}}

In order to contextualise the resulting data points with the help of the United Manufacturing Hub, 3 identifiers are necessary:

- The customer ID to be assigned to the asset: *customerID* (Default value for the development setup "factoryinsight"

- The location where the asset is located: *location* (Can be chosen freely)

- The name of the asset: *AssetID* (Can be chosen freely)

In the topic of our **first node (MQTT IN)** (PICTURE) all these three information are bundled to get a MQTT input.

The topic structure is: `ia/raw/<transmitterID>/<gatewaySerialNumber>/<portNumber>/<IOLinkSensorID>`

To get a quick and easy overview of the available MQTT messages and topics we recommend the MQTT Explorer. If you don’t want to install any extra software you can use the MQTT-In node to subscribe to all available topics by subscribing to # and then direct the messages of the MQTT in nodes into a debugging node. You can then display the messages in the nodered debugging window and get information about the topic and available data points.

An example for an ia/raw/ topic is: `ia/raw/2020-0102/0000005898845/X01/210-156`. This means that an IFM gateway with serial number `0000005898845` is connected to a transmitter with serial number `2020-0102`. This gateway has connected the sensor `210-156` to the first port `X01`.

*3.2 Second node: JSON*

{{< imgproc json Fit "800x800" >}}{{< /imgproc >}}

The **second node (JSON)** is a generic container of elements inside a JSON stream (or how to describe it briefly) and is called JSON. It can contain fundamental types (integers, booleans, floating point numbers, strings) and complex types (arrays and objects) and is used to convert data between two formats.

Now we will take a look at the three different sensors individually.

#### Button Bar

*3.3 Theird node: Function*

{{< imgproc function Fit "800x800" >}}{{< /imgproc >}}

In the **third node "function"** a timestamp is generated at the time of pressing a button. The timestamp is stored in the form of a string. This makes it possible, for example, to specify machine states and display them in a timeline.

The code for this node looks like this:

msg.timestamp=msg.payload.timestamp_ms

msg.payload=msg.payload.value_string;

return msg; 

*3.4 Fourth node: Filter*

{{< imgproc filter Fit "800x800" >}}{{< /imgproc >}}

The **filter** is mainly for clarity and blocks the values of a sensor until the value is changed. A change of the property is not necessary.

*3.5 Fifth node: Switch*

{{< imgproc switch Fit "100x100" >}}{{< /imgproc >}}

The **switch** is used to distinguish between the different inputs for the following flow and only lets through the values that come through the previously defined input. 

With the button bar, these are the individual buttons. You can see which name is assigned to a single button by the number that appears in the debug window when you press a button.  For our example these are the numbers "0108", "0104", 0120" and "0101".

*3.6 Sixth node: Function*

{{< imgproc function Fit "800x800" >}}{{< /imgproc >}}

The switch node is followed by a separate **function** for each button. In our example different states are transmitted. States can be e.g. pause, maintenance, emptying etc. and are defined via numbers. The different states can be found [here](https://docs.umh.app/docs/concepts/state/).

For example, the code for the function looks like this:

msg.payload=

{

    "timestamp_ms": msg.timestamp,

    "state": 120000

}

msg.topic = "ia/raw/<transmitterID>/<gatewaySerialNumber>/<portNumber>/<IOLinkSensorID>"

return msg;

To reach further machine states, only the adaptation of the state number is necessary

*3.7 Seventh node: MQTT-Out*

{{< imgproc mqtt_out Fit "800x800" >}}{{< /imgproc >}}

To publish messages to a pre-configured topic, the **MQTT-Out** node is used.

#### Light Barrier

TODO

#### Inductive Sensor

TODO

### 2nd example: Integration of existing sensors

This chapter describes how to connect already existing sensors/ integrate an already existing interface (OPC/UA) with our system.

## 3. Create dashboards with Grafana

TODO
