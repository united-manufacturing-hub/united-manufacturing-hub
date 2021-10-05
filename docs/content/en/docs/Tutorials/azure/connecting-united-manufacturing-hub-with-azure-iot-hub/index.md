---
title: "Connecting with Azure IoT Hub"
linkTitle: "Connecting with Azure IoT Hub"
description: >
  Azure IoT Hub enabled highly secure and reliable communication between IoT applications and the devices it manages. In this article it is described how one can connect the United Manufacturing Hub with Azure IoT hub.
---

This tutorial is part of a larger series on [how to integrate the United Manufacturing Hub into the Azure stack](/docs/concepts/integration-with-azure/).

## Tutorial

### Prerequisites

- Basic knowledge about [IT / OT](/docs/getting-started/understanding-the-technologies/), [Azure](/docs/concepts/integration-with-azure/) and the [difference between symmetric and asymmetric encryption](/docs/tutorials/general/symmetric-asymmetric-encrption/)
- You should have one instance of IoT Hub running and one device created with asymmetric encryption. You can follow [our tutorial for setting up Azure IoT Hub](/docs/tutorials/azure/setting-up-azure-iot-hub/). 
- By default Microsoft recommends using symmetric encryption as it is more easier to implement, but they say themselves that asymmetric encryption is more secure. If you are using symmetric encryption (Username / Password authentification, no certificates) there might be some steps that are different for you
- All information for your Azure IoT Hub device ready: 
- - Hostname of the Azure IoT Hub: e.g., contoso-test.azure-devices.net
- - Device ID: e.g., factory3
- - (symmetric) Username: e.g., contoso-test.azure-devices.net/factory3/?api-version=2018-06-30
- - (symmetric) Password / SAS Token: e.g., SharedAccessSignature sr=contoso-test......
- - (asymmetric) Certificate files
- Furthermore, we recommend having access to Azure IoT Hub and the Device Explorer to verify whether the connection worked.

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

### Option 2: using mqtt-bridge

**Currently only available for asymmetric encryption. See also [GitHub issue #569 for more information](https://github.com/united-manufacturing-hub/united-manufacturing-hub/issues/569)**

Advantages:
- most reliable solution

Disadvantages:
- requires slightly more time than option 1


## What to do now?

Now we send the data to Azure IoT Hub. But what to do now? Here are some ideas:

1. You could install the server component of the United Manufacturing Hub `factorycube-server` on [Azure Kubernetes Service](https://azure.microsoft.com/en-us/services/kubernetes-service/#features). You would then be able to immediatly start creating dashboard while leveraging Azure's scalability
2. You could use Azure's propertiary services to create a full IIoT infrastructure from scratch. However, this is time intensive as you need to build e.g., your own data model, so we only recommend it for large companies with their own IIoT team. A good next step to proceed from here would be [the tutorial *Add an IoT hub event source to your Azure Time Series Insight environment*](https://docs.microsoft.com/en-us/azure/time-series-insights/how-to-ingest-data-iot-hub)

