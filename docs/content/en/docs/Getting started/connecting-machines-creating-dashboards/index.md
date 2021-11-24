
---
title: "2. Connecting machines and creating dashboards"
linkTitle: "2. Connecting machines and creating dashboards"
weight: 2
description: >
  This section explains how the United Manufacturing Hub is used practically 
---

## 1. Extract data using factorycube-edge

**The basic approach for data processing on the local hardware is to extract data from various data sources (OPC/UA, MQTT, Rest), extract the important information, and then make it available to the United Manufacturing Hub via a predefined interface (MQTT).**

**To extract and pre-process the data from different data sources we use the open source software Node-RED. Node-RED is a low-code programming for event-driven applications.**

If you haven't worked with Node-RED yet, [here](https://nodered.org/docs/user-guide/) is a good documentation.

{{< imgproc nodered Fit "800x800" >}}{{< /imgproc >}}

[Here you can download the flow](/examples/nodered/standard_flow.json)

### General Configuration

{{< imgproc nodered_general Fit "800x800" >}}{{< /imgproc >}}

Basically, 3 pieces of information need to be sent to the system. For more information feel free to check [this article](/docs/concepts/mqtt/). 

- The customer ID to be assigned to the asset: *customerID*

- The location where the asset is located: *location*

- The name of the asset: *AssetID*

These 3 information must be set to the system via the green configuration Node-RED, so that the data can be assigned exactly to an asset.

Furthermore, under the general settings you will find the state logic that determines the *state* of the machine using the *activity* and *detectedAnomaly* topic. For more information feel free to check [this article.](/docs/concepts/mqtt/)

### Inputs:
{{< imgproc nodered_inputs Fit "800x800" >}}{{< /imgproc >}}

**With the help of the inputs you can tap different data sources. Like for example:**
- OPC/UA ([documentation for this node](https://flows.nodered.org/node/node-red-contrib-opcua))
- Siemens S7 ([documentation for this node](https://flows.nodered.org/node/node-red-contrib-s7))
- TCP/IP ([documentation for this node](https://flows.nodered.org/flow/bed6f676d088670d7e1bc298943338b5))
- Rest API  ([documentation for this node](https://cookbook.nodered.org/http/create-an-http-endpoint))
- Modbus  ([documentation for this node](https://flows.nodered.org/node/node-red-contrib-modbus))
- MQTT ([documentation for this node](https://cookbook.nodered.org/mqtt/))

**Interaction with sensorconnect (Plug and Play connection of IO-Link Senosors):**

With the help of Sensorconnect, various sensors can be connected quickly and easily via an IFM gateway. The sensor values are automatically extracted from the software stack and made available via [MQTT](http://www.steves-internet-guide.com/mqtt-works/).

To get a quick and easy overview of the available MQTT messages and topics, we recommend using the [MQTT Explorer](http://mqtt-explorer.com/). If you don't want to install aadditional software, you can use the MQTT-In node to subscribe to all available topics by subscribing to  `#` and then directing the messages of the MQTT-In nodes into a debugging node. You can then view the messages in the Node-RED debugging window and get information about the topic and the available data points.


Topic structure: `ia/raw/<transmitterID>/<gatewaySerialNumber>/<portNumber>/<IOLinkSensorID>`

#### Example for ia/raw/

Topic: `ia/raw/2020-0102/0000005898845/X01/210-156`

This means that an ifm gateway with serial number `0000005898845` is connected to the transmitter with serial number `2020-0102`. This gateway has connected the sensor `210-156` to the first port `X01`.

```json
{
"timestamp_ms": 1588879689394, 
"distance": 16
}
```


### Extract information and make it available to the **outputs**:
In order for the data to be processed easily and quickly by the United Manufacturing hub, the input data (OPC/UA, Siemens S7) must be prepared and converted into a standardized data format (MQTT Topic). A detailed explanation of our MQTT data model can be found [here](/docs/concepts/mqtt/) and [here](/docs/concepts/state).

{{< imgproc nodered_outputs Fit "800x800" >}}{{< /imgproc >}}

The 4 most important data points:
- Information whether the machine is running or not: `/activity`
- Information about anomalies or concrete reasons for a machine standstill: `/detectedAnomaly`
- The produced quantity: `/count`
- An interface to communicate any process value to the system (e.g. temperature or energy consumption) - `/processvalue`

Using the information from the `/activtiy` and `/detectedAnomaly` topics, the statelogic node calculates the discrete machine state. It first checks if the machine is running or not. If the machine is not running, the machine state is set equal to the last `/detectedAnomaly`, analogous to [state model](/docs/concepts/state). The discrete machine state is then made available again via the `/state` topic.

**Implementation example: You would like to determine the output and machine condition of a filling machine.**

Used Sensors:
- Lightbarrier for counting the bottles 
- A button bar that the machine operator can use to tell the system that he is taking a break, for example

1.  Extract the information of the light barrier via the MQTT in node whether a bottle was produced. If a bottle was produced, send a message to the output/count topic analog to [MQTT datamodel](/docs/concepts/mqtt).
2.  Use the output_to_activity node to determine the information "the machine is running" from the information "a bottle was produced". For example, if a bottle is produced every X seconds, set the activity to true analogous to the [MQTT datamodel](/docs/concepts/mqtt/).
3.  Use the button bar information to tell the system why the machine is not running. For example, whenever button 3 is pressed, send pause to the detectedAnomaly node analogous to [MQTT datamodel](/docs/concepts/mqtt).

Now the machine status is automatically determined and communicated to the united manufacturing hub for further analysis. Like for example the loss of speed.

TODO: #63 add example Flow for data processing
### Testing:

{{< imgproc nodered_testing Fit "800x800" >}}{{< /imgproc >}}
Using the test flows, you can test your entire system or simply simulate some sample data for visualization.

[See also DCC Aachen example in our showcase.](/docs/examples/assembly-analytics)

## 2. Create dashboards using factorycube-server

TODO
