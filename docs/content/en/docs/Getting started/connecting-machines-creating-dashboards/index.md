
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

{{< imgproc logos_doc Fit "800x800" >}}{{< /imgproc >}}

Here are a few selected examples of data inputs:

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

### 1st example: Retrofitting with external sensors

In this example we will determine the output and the machine condition of a cutting machine.

These sensors are used for this purpose:

- Light barrier
- Button bar
- Inductive sensor

*1. Connect sensors*

With the help of Sensorconnect, sensors can be connected quickly and easily via an IO-Link gateway. The sensor values are automatically extracted from the software stack and made available via [MQTT](https://docs.umh.app/docs/concepts/mqtt/). 

TODO: Picture or illustration of how to connect sensors

*2. Make the connected sensors visible to our system*

The gateways to which sensors are connected are found by our microservice in the local network. For this purpose, the IP-range must be communicated to the microservice so that it can search for the gateway in the correct network and read out the sensors via it.

To do this, there are two options. 

1. Option: Change the IP-range in the [development.yaml](https://www.umh.app/development.yaml) file, that you download. In our example the IP of our factory-cube is `192.168.1.131`. Accordingly we change the IP-range to `192.168.1.0/24.`

2. Option: Lens. In Lens you need to open your already set up cluster for your edge device. Then, as you can see in the picture, click on "Apps" in the left bar, then on "Releases and open "factorycube-edge" by clicking on it.

{{< imgproc ip_range_lens_1 Fit "1200x1200" >}}{{< /imgproc >}}

Next, click into the code and press the key combination `ctrl+F` to search for "iprange". There you have to change the value of the IP-range as shown. In our example the IP of our factory-cube is `192.168.1.131`. Accordingly we change the IP-range to `192.168.1.0/24.`

{{< imgproc ip_range_lens_2 Fit "1200x1200" >}}{{< /imgproc >}}

Now the microservice can search for the gateway in the correct network to read the sensors.

*3. Creating a Node-RED flow*

With Node-Red, it is possible to quickly develop applications and prepare untreated sensor data so that Grafana, for example, can use it. For this purpose, so-called nodes are drawn onto a surface and wired together to create sequences. 

In the following, the creation of such Node-RED flows is demonstrated using the example sensors light barrier, button bar and inductive sensor. 

**For all connected sensors, the first two nodes are the same. Only after the second node we will distinguish between the different sensors.**

*3.1 First node: MQTT-In*

{{< imgproc mqtt_in Fit "800x150" >}}{{< /imgproc >}}

In order to contextualise the resulting data points with the help of the United Manufacturing Hub, three identifiers are necessary:

- The customer ID to be assigned to the asset: *customerID* (Default value for the development setup "factoryinsight")

- The location where the asset is located: *location* (Can be chosen freely)

- The name of the asset: *AssetID* (Can be chosen freely)

In the topic of our **first node (MQTT IN)** all these three information are bundled to get a MQTT input.

The topic structure is: `ia/raw/<transmitterID>/<gatewaySerialNumber>/<portNumber>/<IOLinkSensorID>`

To get a quick and easy overview of the available MQTT messages and topics we recommend the [MQTT Explorer](http://mqtt-explorer.com/). . If you don’t want to install any extra software you can use the MQTT-In node to subscribe to all available topics by subscribing to # and then direct the messages of the MQTT in nodes into a debugging node. You can then display the messages in the nodered debugging window and get information about the topic and available data points.

An example for an ia/raw/ topic is: `ia/raw/development/000200410332/X02/310-372`. This means that an IO-Link gateway with serial number `000200410332` has connected the sensor `310-372` to the first port `X02`.

*3.2 Second node: JSON*

{{< imgproc json Fit "800x150" >}}{{< /imgproc >}}

The **second node (JSON)** is a generic container of elements inside a JSON stream (or how to describe it briefly) and is called JSON. It can contain fundamental types (integers, booleans, floating point numbers, strings) and complex types (arrays and objects) and is used to convert data between two formats.

**Now we will take a look at the three different sensors individually.**

#### Light barrier

With the light barrier it is possible, for example, to record the number of pieces produced. Also, with a more complicated logic, machine states can be detected directly with a light barrier. For the sake of simplicity, this is not explored and applied in our example.

*3.3 Theird node: Function*

{{< imgproc function Fit "800x150" >}}{{< /imgproc >}}

In the **third node "Function"** a timestamp is generated for each distance value. The timestamp is stored in the form of a string. This makes it possible, for example, to read out the time at which a item was produced.
The code for this node looks like this:

```js
msg.timestamp=msg.payload.timestamp_ms

msg.payload=msg.payload.value_string;

return msg;
```

*3.4 Fourth node: Trigger*

{{< imgproc trigger Fit "800x150" >}}{{< /imgproc >}}

The **trigger** allows us, in this example in the case of the light barrier, to sort out distances that are irrelevant for our evaluation, i.e. greater than 15 cm. To do this, you just need to enter a 15 in the "Threshold" field.

*3.5 Fifth node: Function*

{{< imgproc function_light_barrier Fit "800x150" >}}{{< /imgproc >}}

In our example we want to count the number of produced parts. As a trolley on a rail is responsible for the transport into a container after the production of the part and moves towards the light barrier, the number of produced parts shall be increased by one as soon as the distance between the trolley and the light barrier is smaller than 15. To do this, we need a function with the following code:

```js
msg.payload = {
  "timestamp_ms": Date.now(),
  "count": 1
}
msg.topic = "ia/"+env.get("factoryinsight")+"/"+env.get("dccaachen")+"/"+env.get("docs")+"/count"
return msg;
```

*3.7 Seventh node: MQTT-Out*

{{< imgproc mqtt_out Fit "800x150" >}}{{< /imgproc >}}

To publish messages to a pre-configured topic, the **MQTT-Out** node is used.

The complete Node-RED flow then looks like this:

{{< imgproc nodered_flow_light_barrier Fit "1200x1200" >}}{{< /imgproc >}}

#### Button bar

```js
For full functionality shifts must be added analog to our data model.
```

*3.3 Theird node: Function*

{{< imgproc function Fit "800x150" >}}{{< /imgproc >}}

In the **third node "function"** a timestamp is generated at the time of pressing a button. The timestamp is stored in the form of a string. This makes it possible, for example, to specify machine states and display them in a timeline.

The code for this node looks like this:

```js
msg.timestamp=msg.payload.timestamp_ms

msg.payload=msg.payload.value_string;

return msg;
```

*3.4 Fourth node: Filter*

{{< imgproc filter Fit "800x150" >}}{{< /imgproc >}}

The **filter** is mainly for clarity and blocks the values of a sensor until the value is changed. A change of the property is not necessary.

*3.5 Fifth node: Switch*

{{< imgproc switch Fit "800x150" >}}{{< /imgproc >}}

The **switch** is used to distinguish between the different inputs for the following flow and only lets through the values that come through the previously defined input. 

With the button bar, these are the individual buttons. You can see which name is assigned to a single button by the number that appears in the debug window when you press a button.  For our example these are the numbers "0108", "0104", 0120" and "0101".

*3.6 Sixth node: Function*

{{< imgproc function_button_bar Fit "800x150" >}}{{< /imgproc >}}

The switch node is followed by a separate **function** for each button. In our example different states are transmitted. States can be e.g. active, unknow pause, material change, process etc. and are defined via numbers. The different states can be found [here](https://docs.umh.app/docs/concepts/state/).

For example, the code for the function looks like this:

```js
msg.payload = {
  "timestamp_ms": msg.timestamp,
  "state": 10000
}

msg.topic = "ia/raw/transmitterID/gatewaySerialNumber/portNumber/IOLinkSensorID"
return msg;
```

To reach further machine states, only the adaptation of the state number is necessary

*3.7 Seventh node: MQTT-Out*

{{< imgproc mqtt_out Fit "800x150" >}}{{< /imgproc >}}

To publish messages to a pre-configured topic, the **MQTT-Out** node is used.

The complete Node-RED flow then looks like this:

{{< imgproc nodered_flow_button_bar Fit "1200x1200" >}}{{< /imgproc >}}

#### Inductive sensor

*3.3 Theird node: Function*

{{< imgproc function Fit "800x150" >}}{{< /imgproc >}}

In the **third node "Function"** a timestamp is generated for each inductive value. The timestamp is stored in the form of a string. This makes it possible, for example, to read out the time at which a item was produced.
The code for this node looks like this:

```js
msg.timestamp=msg.payload.timestamp_ms

msg.payload=msg.payload.value_string;

return msg;
```

*3.4 Fourth node: Function*

{{< imgproc function_inductive_sensor Fit "800x150" >}}{{< /imgproc >}}

In our example, the following **function** ensures that when the value of the inductive sensor is changed, the message "Window open" is output. To do this, we need a function with the following code:

```js
msg.payload = {
    "timestamp_ms": msg.timestamp, 
    "Window open": msg.payload
}
msg.topic = "ia/"+env.get("factoryinsight")+"/"+env.get("dccaachen")+"/"+env.get("docs")+"/processValue"
return msg;
```

*3.5 Fifth node: MQTT-Out*

{{< imgproc mqtt_out Fit "800x150" >}}{{< /imgproc >}}

To publish messages to a pre-configured topic, the **MQTT-Out** node is used.

The complete Node-RED flow then looks like this:

{{< imgproc nodered_flow_inductive_sensor Fit "1200x1200" >}}{{< /imgproc >}}

### 2nd example: Extraction of data points via a predefined interface (In our example: OPC-UA)

This chapter describes how to connect already existing sensors or integrate an already existing interface (OPC/UA) with our system.

## 3. Create dashboards with Grafana

**To create a personalized dashboard at Grafana, first make sure that all preparations have been made as described in [Installation](https://docs.umh.app/docs/getting-started/setup-development/).**

So the first step is to open Grafana by opening the following URL in your browser: http://<IP>:8080 (e.g. http://192.168.1.131:8080). You can log in with the username admin and the password from your clipboard.


