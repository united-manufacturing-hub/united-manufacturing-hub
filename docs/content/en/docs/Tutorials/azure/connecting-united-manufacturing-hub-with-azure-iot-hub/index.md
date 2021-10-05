---
title: "Connecting with Azure IoT Hub"
linkTitle: "Connecting with Azure IoT Hub"
description: >
  Azure IoT Hub enabled highly secure and reliable communication between IoT applications and the devices it manages. In this article it is described how one can connect the United Manufacturing Hub with Azure IoT hub.
---

This tutorial is part of a larger series on [how to integrate the United Manufacturing Hub into the Azure stack](/docs/concepts/integration-with-azure/).

## What is Azure IoT Hub?

--> basically a message broker (like a MQTT broker)

--> In the following chapter we will substitute the VerneMQ broker in the United Manufacturing Hub with Azure IoT Hub.

## Tutorial

### Prerequisites

- Basic knowledge about [IT / OT](/docs/getting-started/understanding-the-technologies/), [Azure](/docs/concepts/integration-with-azure/) and the [difference between symmetric and asymmetric encryption](/docs/tutorials/general/symmetric-asymmetric-encrption/)
- You should have one instance of IoT Hub running and one device created with asymmetric encryption. You can follow [our tutorial for setting up Azure IoT Hub](/docs/tutorials/azure/setting-up-azure-iot-hub/). By default Microsoft recommends using symmetric encrpytion as it is more easier to implement, but they say themselves that asymmetric encrption is 


- having a valid Azure subscribtion
- [Azure IoT Hub with at least one device created](https://docs.microsoft.com/en-us/azure/iot-hub/iot-hub-create-through-portal)
- for that Azure IoT Hub device at least the following information: 
- - Hostname of the Azure IoT Hub: e.g., contoso-test.azure-devices.net
- - Device ID: e.g., factory3
- - Username: e.g.,

Account with Azure
SAS
Token
Access to verify successful data transmission
etc.

### Option 1: using Node-RED 

Advantages: 
- quick
- easy

Disadvantages:
- can be unrealiable, e.g., when the user accidentally adds an unstable external plugin, which takes down Node-RED
- does not buffer data in case of internet outages

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

