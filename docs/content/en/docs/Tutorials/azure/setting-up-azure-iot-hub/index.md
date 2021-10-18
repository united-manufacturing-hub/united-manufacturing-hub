---
title: "Setting up Azure IoT Hub"
linkTitle: "Setting up Azure IoT Hub"
description: >
  Azure IoT Hub enabled highly secure and reliable communication between IoT applications and the devices it manages. This article explains how to set it up and links to the official Microsoft Azure documentation. It provides additional information that are required for mechanical engineers working in Industrial IoT.
---

This tutorial is part of a larger series on [how to integrate the United Manufacturing Hub into the Azure stack](/docs/concepts/integration-with-azure/).

## What is Azure IoT Hub?

You can find the [official description on the Microsoft Website](https://docs.microsoft.com/en-us/azure/iot-hub/about-iot-hub). Speaking in IT terms it is basically a message broker (like a MQTT broker) with benefits.

When connecteing with the United Manufacturing Hub we will substitute the VerneMQ broker (see also [architecture](/docs/concepts/)) in the United Manufacturing Hub with Azure IoT Hub.

## Tutorial

### Prerequisites

- Basic knowledge about [IT / OT](/docs/getting-started/understanding-the-technologies/), [Azure](/docs/concepts/integration-with-azure/) and the [difference between symmetric and asymmetric encryption](/docs/tutorials/general/symmetric-asymmetric-encryption/)

#### Step 1: Create the necessary PKI (can be skipped if using symmetric encryption)

We recommend to skip this step if you are new to IT / Public-Key-Infrastructure and just want to see some data in Azure. We strongly advise to not use in production and instead follow the steps mentioned in this step.

There are two options / tutorials to get started:
1. [our tutorial on setting up a PKI infrastructure (recommended, as it can be used in production as well)](/docs/tutorials/general/pki/). Replace the server URL with your Azure IoT Hub URL and ensure that you use the device IDs as `MQTT client id`
2. [use the official Microsoft tutorial (only for testing)](https://docs.microsoft.com/de-de/azure/iot-hub/tutorial-x509-scripts
)

#### Step 2: Create Azure IoT Hub

[Follow the official guide](https://docs.microsoft.com/en-us/azure/iot-hub/iot-hub-create-through-portal). You can use the free plan for the beginning, which will be sufficient to test everything. 

When using asymmetric encrpytion: when creating a new device select X.509 as authentification type.

## What to do now?

We recommend you to [connect the United Manufacturing Hub with Azure IoT Hub](/docs/tutorials/azure/connecting-united-manufacturing-hub-with-azure-iot-hub) as a next step.
