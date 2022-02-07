
---
title: "2. Connecting machines and creating dashboards"
linkTitle: "2. Connecting machines and creating dashboards"
weight: 2
description: >
  This section explains how the United Manufacturing Hub is used practically - from connecting machines to creating OEE dashboards
---

This guide will explain the typical workflow on how an engineer will connect production lines and then create dashboards based on the extracted data. It is split into two parts:

1. Extracting data from the shop floor by connecting with existing PLCs or additional sensors (there will be examples for both)
2. Creating a customized dashboard in [Grafana](https://www.grafana.com) to visualize the data

## Prerequisites

Before continuing with this guide, you should have [understood the technologies](../understanding-the-technologies) and [have installed the United Manufacturing Hub](../setup-development).

## Introduction: extracting data from the shop floor 

In general, there are three types of systems on the shop floor that you can connect to:
1. PLCs like for example via OPC/UA, Siemens S7 protocol or Modbus
2. Retrofitted sensors (e.g., because there was no access to the PLC). The United Manufacturing Hub uses IO-Link to connect IO-Link sensors and by using converters digital/analog sensors as well.
3. Additional systems like ERP, MES or quality management systems using TCP/IP, REST or MQTT.

{{< imgproc logos_doc Fit "800x800" >}}{{< /imgproc >}}

For each protocol and type of system there is a microservice, which handles the connection and extracts all data and pushes them into the central MQTT broker.

From there on, the data is converted into the [standardized data model](../../Concepts/mqtt) and therefore aggregated and contextualised. The conversion process is usually done in Node-RED, but can be done in any programming language as well.

Sometimes Node-RED is used not only to aggregated and contextualise, but also to extract data from the shop floor. More information about the data flow and the programs behind it, can be found [in the architecture](../../Concepts). We've also written [a blog article about the industrial usage of Node-RED](../../Concepts/node-red-in-industrial-iot), which we can highly recommend.

In the following, we will go through two examples of connecting the shop floor: 
1. Retrofitting a cutting machine with the help of external sensors;
2. Using OPC/UA to read a warping machine data directly from the PLC.

## Retrofitting with external sensors

In this example we will determine the output and the machine condition of a cutting machine step-by-step.

These sensors are used for this purpose:

- Light barrier
- Button bar
- Inductive sensor

### Step 1: Physically connect sensors and extract their data

TODO: Picture or illustration of setup

As a first step you should mount the sensors on the production machine in the regions of your interest. The sensors are then connected to an IO-Link gateway, e.g., the [ifm AL1350](https://www.ifm.com/us/en/product/AL1350)

The IO-Link gateway is then connected to the network, so that the installation of the United Manufacturing Hub has access to it. As soon as it is physically connected and recieves an IP address it will be automatically detected by [sensorconnect](https://docs.umh.app/docs/developers/factorycube-edge/sensorconnect/) and the raw data will be pushed into the central MQTT broker.

The IO-Link gateways, to which sensors are connected, are found by the microservice sensorconnect using a network scan. For this purpose, sensorconnect must be configured with the correcy IP-range, so that it can search for the gateway in the correct network and read out the sensors via it.

The IP-range needs to be entered in the [CIDR notation](https://www.ionos.com/digitalguide/server/know-how/cidr-classless-inter-domain-routing/). `192.168.1.0/24` means all IP addresses from 192.168.1.1 to 192.168.1.254. To prevent accidently scanning whole IP-ranges it is set to scan localhost by default.

To do this, there are two options. 

- Option 1: Change the IP-range in the [development.yaml (k3OS cloud-init)](https://www.umh.app/development.yaml) file, that you download. In our example the IP of our Factorycube is `192.168.1.131`. Accordingly we change the IP-range to `192.168.1.0/24.`

- Option 2: Lens. In Lens you need to open your already set up cluster for your edge device. Then, as you can see in the picture, click on "Apps" in the left bar, then on "Releases and open "factorycube-edge" by clicking on it.

{{< imgproc ip_range_lens_1 Fit "1200x1200" >}}{{< /imgproc >}}

Next, click into the code and press the key combination `ctrl+F` to search for "iprange". There you have to change the value of the IP-range as shown. In our example the IP of our Factorycube is `192.168.1.131`. Accordingly we change the IP-range to `192.168.1.0/24`.

{{< imgproc ip_range_lens_2 Fit "1200x1200" >}}{{< /imgproc >}}

Now the microservice can search for the gateway in the correct network to read the sensors.

As the endresult, we have now all sensor data available in the MQTT broker.

To get a quick and easy overview of the available MQTT messages and topics we recommend the [MQTT Explorer](http://mqtt-explorer.com/). If you don’t want to install any extra software you can use the MQTT-In node to subscribe to all available topics by subscribing to `#` and then direct the messages of the MQTT in nodes into a debugging node. You can then display the messages in the nodered debugging window and get information about the topic and available data points.
More information on using Node-RED will come further down.

### Step 2: Processing the data by creating a Node-RED flow

As the next step, we need to process the raw sensor data and add supporting information (e.g., new shifts). 

Theoretically, you can do it in any programming language by connecting to the MQTT broker, processing the data and returning the results according to the [datamodel](/docs/concepts/mqtt). However, we recommend to do that with Node-RED.

With Node-RED, it is possible to quickly develop applications, add new data and convert raw sensor data so that for example it can be shown in a Grafana dashboard. For this purpose, so-called *nodes* are drawn onto a surface and wired together to create sequences. For more information, take a look at the original [Node-RED tutorial](https://nodered.org/docs/tutorials/first-flow)

In the following, the creation of such Node-RED flows is demonstrated:
1. Adding new planned operator shifts 
2. Recording the number of produced pieces with a light barrier, 
3. Changing the machine state using buttons on a button bar 
4. Measuring the distance of an object using an inductive sensor. 

#### Example 1: adding new planned operator shifts

A shift is defined as the planned production time and is therefore important for the OEE calculation. If no shifts have been added, it is automatically assumed that the machine is not planned to run. Therefore, all stops will be automatically considered "No shift" or "Operator break" until a shift is added.

{{< imgproc nodered_flow_addshift Fit "800x800" >}}{{< /imgproc >}}

A shift can be added by sending a `/addShift` MQTT message as specified in the [datamodel](/docs/concepts/mqtt). In Node-RED this results in a flow consisting out of at least three nodes:

1. Inject node
2. Function node (in the picture called "add Shift")
3. MQTT node

The inject node can be used unmodified. 

The function node has the following code, in which the MQTT message payload and topic according to the [datamodel](/docs/concepts/mqtt) are prepared:

```js
msg.payload = {
  "timestamp_ms": 1639522810000,
  "timestamp_ms_end": 1639609200000
}
msg.topic = "ia/factoryinsight/dccaachen/docs/addShift"
return msg;
```

The MQTT node needs to be configured to connect with the MQTT broker (if nothing has been changed from the default United Manufacturing Hub installation, it will be selected automatically).

Now you can deploy the flow and by clicking on the inject node you will generate the MQTT message. A shift will be added for the customer `factoryinsight` (default value), the asset location `dccaachen` and the asset name `docs` (see also topic structure).

The shift will start at 2021-12-14 at 00:00 and will end at 2021-12-15 00:00 (depending on your timezone). You can get that information by translating the UNIX timestamp in millis (e.g., `1639609200000`) to a human readable format using for example https://currentmillis.com/

#### Example 2: recording the number of produced pieces 

Now we will process sensor data instead of sending new data as before.

With the light barrier it is possible, for example, to record the number of pieces produced. Also, with a more complicated logic, machine states can be detected directly with a light barrier. For the sake of simplicity, this is not explored and applied in our example.

The first two nodes in this example will be the same for all other remaining examples as well.

##### First node: MQTT-in

{{< alert title="Note" color="warn">}}
It can happen that you need to configure the username for the MQTT broker first. For this you should select a MQTT node and go into the settings of the MQTT broker. Then add there the username `ia_nodered` without any password.
{{< /alert >}}

{{< imgproc mqtt_in Fit "800x150" >}}{{< /imgproc >}}

The topic structure is (see also the datamodel): `ia/raw/<transmitterID>/<gatewaySerialNumber>/<portNumber>/<IOLinkSensorID>`

An example for an ia/raw/ topic is: `ia/raw/development/000200410332/X02/310-372`. This means that an IO-Link gateway with serial number `000200410332` has connected the sensor `310-372` to the first port `X02`.

Now all messages coming in (around 20 messages per second by default for one sensor) will start a Node-RED message flow.

##### Second node: JSON

{{< imgproc json.png Fit "800x150" >}}{{< /imgproc >}}

The payload of all incoming messages will be in the JSON format, so we need to interpret this by using the JSON node.

##### Third node: function

{{< imgproc function Fit "800x150" >}}{{< /imgproc >}}

The third node "function" formats the incoming data by extracting information from the json (timestamp and relevant data point) and arranging it in a usable way for Node-RED (parsing). The code for this node looks like this:

```js
msg.timestamp=msg.payload.timestamp_ms
msg.payload=msg.payload.Distance;
return msg;
```

##### Fourth node: trigger

{{< imgproc trigger Fit "800x150" >}}{{< /imgproc >}}

The trigger allows us, in this example in the case of the light barrier, to sort out distances that are irrelevant for our evaluation, i.e. greater than 15 cm. To do this, you just need to enter a 15 in the "Threshold" field.

##### Fifth node: trigger

{{< imgproc function_light_barrier Fit "800x150" >}}{{< /imgproc >}}

In our example we want to count the number of produced parts. As a trolley on a rail is responsible for the transport into a container after the production of the part and moves towards the light barrier, the number of produced parts shall be increased by one as soon as the distance between the trolley and the light barrier is smaller than 15. To do this, we need a function with the following code:

```js
msg.payload = {
  "timestamp_ms": msg.timestamp,
  "count": 1
}
msg.topic = "ia/factoryinsight/dccaachen/docs/count"
return msg;
```

##### Sixth node: MQTT-out

{{< imgproc mqtt_out Fit "800x150" >}}{{< /imgproc >}}

To publish messages to a pre-configured topic, the **MQTT-out** node is used.

The complete Node-RED flow then looks like this:

{{< imgproc nodered_flow_light_barrier Fit "1200x1200" >}}{{< /imgproc >}}

#### Example 3: changing the machine state using buttons on a button bar

{{% alert title="Note" color="primary" %}}

The first two nodes can be found in [example 2](#example-2-recording-the-number-of-produced-pieces)

{{< /alert >}}

##### Third node: function

{{< imgproc function Fit "800x150" >}}{{< /imgproc >}}

Same principle as in [example 2](#example-2-recording-the-number-of-produced-pieces), just with changed extracted values.

```js
msg.timestamp=msg.payload.timestamp_ms
msg.payload=msg.payload.value_string;
return msg;
```

##### Fourth node: filter

{{< imgproc filter Fit "800x150" >}}{{< /imgproc >}}

The filter blocks the values of a sensor until the value is changed. A change of the property of the node is not necessary.

##### Fifth node: switch

{{< imgproc switch Fit "800x150" >}}{{< /imgproc >}}

The switch node is used to distinguish between the different inputs for the following flow and only lets through the values that come through the previously defined input. 

With the button bar, these are the individual buttons. You can see which name is assigned to a single button by the number that appears in the debug window when you press a button.  For our example these are the numbers "0108", "0104", 0120" and "0101".

##### Sixth node: function

{{< imgproc function_button_bar Fit "800x150" >}}{{< /imgproc >}}

The switch node is followed by a separate function for each button. In our example different states are transmitted. States can be e.g. active, unknow pause, material change, process etc. and are defined via numbers. The different states can be found [here](/docs/concepts/state/).

For example, the code for the function looks like this:

```js
msg.payload = {
  "timestamp_ms": msg.timestamp,
  "state": 10000
}

msg.topic = "ia/factoryinsight/dccaachen/docs/state"
return msg;
```

To reach further machine states, only the adaptation of the state number is necessary.

##### Seventh node: MQTT-out

{{< imgproc mqtt_out Fit "800x150" >}}{{< /imgproc >}}

To publish messages to a pre-configured topic, the **MQTT-out** node is used.

The complete Node-RED flow then looks like this:

{{< imgproc nodered_flow_button_bar Fit "1200x1200" >}}{{< /imgproc >}}

#### Example 4: Measuring the distance of an object using an inductive sensor

{{% alert title="Note" color="primary" %}}

The first two nodes can be found in [example 2](#example-2-recording-the-number-of-produced-pieces)

{{< /alert >}}

##### Third node: function

{{< imgproc function Fit "800x150" >}}{{< /imgproc >}}

In the third node "function" a timestamp is generated for each capacitive value. The timestamp is stored in the form of a string. This makes it possible, for example, to read out the time at which a item was produced.

The code for this node looks like this:

```js
msg.timestamp=msg.payload.timestamp_ms
msg.payload=msg.payload["Process value"]
return msg;
```

##### Fourth node: function

{{< imgproc function_inductive_sensor Fit "800x150" >}}{{< /imgproc >}}

In our example, the following **function** formats the incoming data from MQTT-IN by extracting information from the json (timestamp and relevant data point) and arranging it in a usable way for Node-RED (parsing). To do this, we need a function with the following code:

```js
msg.payload = {
    "timestamp_ms": msg.timestamp, 
    "process_value": msg.payload
}
msg.topic = "ia/factoryinsight/dccaachen/docs/processValue"
return msg;
```

The Process value name `process_value` can be chosen freely.

##### Fifth node: MQTT-out

{{< imgproc mqtt_out Fit "800x150" >}}{{< /imgproc >}}

To publish messages to a pre-configured topic, the **MQTT-out** node is used.

The complete Node-RED flow then looks like this:

{{< imgproc nodered_flow_inductive_sensor Fit "1200x1200" >}}{{< /imgproc >}}

## Extraction of data points via OPC/UA

This chapter will describe in the future how to connect already existing sensors or integrate an already existing interface (OPC/UA) with our system.

Hint: You can use the OPC UA node in Node-RED

## Create dashboards with Grafana

So the first step is to open Grafana by opening the following URL in your browser: `http://<IP>:8080` (e.g. `http://192.168.1.2:8080`). You can log in with the username `admin` and the password acquired during the installation tutorial.

Next, create a new dashboard by clicking on the **+** on the left side.

{{< imgproc grafana_1 Fit "1200x1200" >}}{{< /imgproc >}}

After clicking on **Add an empty panel** you should see the following settings:

{{< imgproc grafana_2 Fit "1200x1200" >}}{{< /imgproc >}}

Now you can add different panels to your dashboard.

### Example panel 1: number of produced pieces acquired by the light barrier

For the light barrier the *stat* panel is used.

{{< imgproc grafana_light_barrier_1 Fit "800x800" >}}{{< /imgproc >}}

First of all, in "location" and "asset" labels are selected from the topic of the second function in the Node-RED flow. The "Value" needs to be **count**.

To get the total amount of produced parts a deeper insight into the panel settings is required. Go to "Value-Options" on the right side and change the entries of "Calculation" and "Fields" as shown below.

{{< imgproc grafana_light_barrier_2 Fit "1200x1200" >}}{{< /imgproc >}}

### Example panel 2: machine state acquired by the button bar

For the button bar the *Discrete* panel is used.

{{< imgproc grafana_button_bar_1 Fit "800x800" >}}{{< /imgproc >}}

The query parameters from the topic of the second function in the Node-RED flow must be selected in "location" and "asset". The "value" must be **state**.

### Example panel 3: process values

For the capacitive sensor the *Graph (old)* panel is used.

{{< imgproc grafana_capacitive_sensor_1 Fit "1200x1200" >}}{{< /imgproc >}}

The query parameters from the topic of the second function in the Node-RED flow must be selected in "location" and "asset". The "value" must be **process_process_value** as specified in the payload message.

Now the dashboard should look like this. In the upper right corner you can set the time span in which the data should be displayed and how often the dashboard should be refreshed. 

{{< imgproc grafana_3 Fit "1200x1200" >}}{{< /imgproc >}}

## Final notes

The complete Node-RED flow can be downloaded [here](/examples/node_red_flow_final.json)

Now go grab a coffee and relax.

