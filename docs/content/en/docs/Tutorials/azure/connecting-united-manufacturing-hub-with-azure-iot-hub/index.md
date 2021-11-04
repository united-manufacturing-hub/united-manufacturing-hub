---
title: "Connecting with Azure IoT Hub"
linkTitle: "Connecting with Azure IoT Hub"
description: >
  Azure IoT Hub enabled highly secure and reliable communication between IoT applications and the devices it manages. In this article it is described how one can connect the United Manufacturing Hub with Azure IoT hub.
---

This tutorial is part of a larger series on [how to integrate the United Manufacturing Hub into the Azure stack](/docs/concepts/integration-with-azure/).

## Tutorial

In this tutorial we are leveraging the fact that Azure IoT Hub is nothing but a MQTT broker with benefits (e.g., device management or REST API). We will therefore create a connection via MQTT.

### Prerequisites

- Basic knowledge about [IT / OT](/docs/getting-started/understanding-the-technologies/), [Azure](/docs/concepts/integration-with-azure/) and the [difference between symmetric and asymmetric encryption](/docs/tutorials/general/symmetric-asymmetric-encryption/)
- You should have one instance of IoT Hub running and one device created with asymmetric encryption. You can follow [our tutorial for setting up Azure IoT Hub](/docs/tutorials/azure/setting-up-azure-iot-hub/). 
- By default Microsoft recommends using symmetric encryption as it is more easier to implement, but they say themselves that asymmetric encryption is more secure. If you are using symmetric encryption (Username / Password authentification, no certificates) there might be some steps that are different for you
- All information for your Azure IoT Hub device ready: 
 - Hostname of the Azure IoT Hub: e.g., contoso-test.azure-devices.net
 - Device ID: e.g., factory3
 - (symmetric) Username: e.g., contoso-test.azure-devices.net/factory3/?api-version=2018-06-30
 - (symmetric) Password / SAS Token: e.g., SharedAccessSignature sr=contoso-test......
 - (asymmetric) Certificate files
- Furthermore, we recommend having access to Azure IoT Hub using the [Azure Device Explorer (download required)](https://github.com/Azure/azure-iot-explorer/releases) to verify whether the connection worked.

### Option 1: using Node-RED 

**Advantages:**
- quick
- easy

**Disadvantages:**
- can be unrealiable, e.g., when the user accidentally adds an unstable external plugin, which takes down Node-RED
- does not buffer data in case of internet outages

{{% alert title="Warning" color="warning" %}}
Please do not use the official Azure IoT Hub plugin for Node-RED ([node-red-contrib-azure-iot-hub](https://flows.nodered.org/node/node-red-contrib-azure-iot-hub)). Our customers and we found this plugin to be unrealiable and we therefore recommend for using the MQTT out node.
{{% /alert %}}

We will do this in three steps. You can also [download our example flows.json](/examples/nodered/azure-iot-hub.json), where you just need to change your authentification parameters (certificates or username / password, host, etc.) and you are ready to go.

#### Step 1: Subscribing to all messages

{{< imgproc 3.png Fit "800x800" >}}MQTT-In node in Node-RED{{< /imgproc >}}

As a first step we take all relevant data out of the MQTT broker and thereby starting a Node-RED flow. Add a MQTT-In node. The MQTT broker details should already been set if you have used our recommended installation method (see also [Getting Started](/docs/getting-started/setup-development/)). 

If not, you can connect on factorycube-edge with the URL: `factorycube-edge-emqxedge-service` under the port 1883.

In the Topic section you can specify what messages from which asset you want to send to Azure IoT Hub using wildcards. If you want to send all messages you can use `ia/#` as a topic. If you just want to send process values of all machines in `plant3` of customer `factoryinsight` you can use:

`ia/factoryinsight/plant3/+/processValue`

{{< imgproc 4.png Fit "800x800" >}}MQTT-In node in Node-RED - detailled view{{< /imgproc >}}

You can test the flow by adding a debug node and connecting it the MQTT-In node with it.

#### Step 2: Changing the payload to contain payload and topic

Azure by default does not support the topic structure of the United Manufacturing Hub. As it contains important infortmation, e.g., the asset that the message is about, we will put that into the payload as well.

{{< imgproc 5.png Fit "800x800" >}}json and function node in Node-RED{{< /imgproc >}}

Add a json node AND a function node with the following content and connect it to the MQTT-In node that we previously created:

```javascript
msg.payload = {
    "topic": msg.topic,
    "payload": msg.payload
}
msg.topic = ""

return msg;
```

{{< imgproc 6.png Fit "800x800" >}}Code pasted into Node-RED{{< /imgproc >}}

Verify by connecting it with the debug node.


#### Step 3: Sending the data to Azure IoT Hub

We recommend following the steps in the article *[Connecting Node-Red to Azure IoT Hub using MQTT nodes](https://medium.com/@nikhilkinkar/connecting-node-red-to-azure-iot-hub-using-mqtt-nodes-6e9160549348)* from [Nikhil Kinkar](https://www.linkedin.com/in/nikhilkinkar/). He is using symmetric encryption. For asymmetric encryption edit the TLS configuration and upload there your certificates.

{{< imgproc 7.png Fit "800x800" >}}MQTT-out node for connecting Node-RED with Azure IoT{{< /imgproc >}}

You should have the connection parameters (username, host, port, etc.) already available, if not go to [Prerequisites](#prerequisites) 

If necessary: as root CA you need to use the [Baltimore CyberTrust Root](https://cacerts.digicert.com/BaltimoreCyberTrustRoot.crt.pem)

Do not forget to set the topic as well (see also the tutorial above): 
{{< imgproc 8.png Fit "800x800" >}}MQTT-out node - topic and server{{< /imgproc >}}
{{< imgproc 9.png Fit "800x800" >}}MQTT-out node - MQTT broker (1/2){{< /imgproc >}}
{{< imgproc 10.png Fit "800x800" >}}MQTT-out node - MQTT broker (2/2){{< /imgproc >}}

Connect it with the previously created function node and test it. If everything went well the messages should now appear in the device explorer in Azure IoT Hub. 

{{< imgproc 11.png Fit "800x800" >}}The entire flow{{< /imgproc >}}


#### Step 4: check whether the messages appear in Azure IoT Hub using the device explorer

{{< imgproc 1.png Fit "800x800" >}}Finding the iothubowner SAS token in Azure{{< /imgproc >}}

Open the Azure IoT Explorer and connect using your iothubowner SAS token (see image above). If everything went well you should see messages coming up (assuming that there are messages in the local MQTT broker. If not, try to send some debug messages to test the connection).

{{< imgproc 2.png Fit "800x800" >}}Messages arriving in the Azure IoT Hub Explorer{{< /imgproc >}}

For more information about how to use the Azure IoT Explorer, [take a look into the official documentation](https://docs.microsoft.com/en-us/azure/iot-fundamentals/howto-use-iot-explorer)

### Option 2: using mqtt-bridge

**Currently specifying the username, which is required for Azure IoT Hub, is still in development. See also [GitHub issue #569 for more information](https://github.com/united-manufacturing-hub/united-manufacturing-hub/issues/569)**

Advantages:
- most reliable solution

Disadvantages:
- requires slightly more time than option 1

In the setup script or in the Helm chart adjust the following values with the corresponding certificates:
- mqttBridgeCACert
- mqttBridgeCert
- mqttBridgePrivkey

As mqttBridgeCACert you need to use the [Baltimore CyberTrust Root](https://cacerts.digicert.com/BaltimoreCyberTrustRoot.crt.pem)

Furthermore, please set `username`, `host`, `port`, etc. accordingly

If you do not know where to get the certificates or the connection parameters, please go back to [Prerequisites](#prerequisites) 

## What to do now?

Now we send the data to Azure IoT Hub. But what to do now? Here are some ideas:

1. You could install the server component of the United Manufacturing Hub `factorycube-server` on [Azure Kubernetes Service](https://azure.microsoft.com/en-us/services/kubernetes-service/#features). You would then be able to immediatly start creating dashboard while leveraging Azure's scalability
2. You could use Azure's propertiary services to create a full IIoT infrastructure from scratch. However, this is time intensive as you need to build e.g., your own data model, so we only recommend it for large companies with their own IIoT team. A good next step to proceed from here would be [the tutorial *Add an IoT hub event source to your Azure Time Series Insight environment*](https://docs.microsoft.com/en-us/azure/time-series-insights/how-to-ingest-data-iot-hub)

