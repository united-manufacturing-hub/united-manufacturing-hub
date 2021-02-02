# Getting Started

## 1. Preparation

The first step to be able to use the system is to install the hardware and put it into operation.

**Use our certified Hardware:**

<img src="../images/factorycube.png" height="250"> <img src="../images/cubi.png" height="150">

To get our hardware up and running you can follow [these instructions](factorycube.md).

**Use you own Hardware:**

If you want to use your own hardware, that is of course no problem. To install the required software on the hardware use this [guide for the core stack](installation-core.md) or this [guide for the advanced stack](installation-advanced.md).

After you have installed the required software on your hardware, you can use [these instructions](sensors/mounting-sensors.md) to install and commission any external sensors that may be required.

## 2. Configure node-red for data pre-processing

To extract and pre-process the data from different data sources we use the open source software node-red. node-red is a low-code programming for event-driven applications.

If you haven't worked with node-red yet, [here](https://nodered.org/docs/user-guide/) is a good documentation directly from node-red!

<img src="images/nodered.png">
**TODO: You can download this standard flow here**

### General Configuration

<img src="images/nodered_general.png">

Basically, 3 pieces of information must be communicated to the system. For more information feel free to check [this article](../general/mqtt.md). These 3 information must be set to the system via the green configuration node-red, so that the data can be assigned exactly to an asset

The customer ID to be assigned to the asset: *customerID*

The location where the asset is located: *location*

The name of the asset: *AssetID*

Furthermore, you will find under the general settings:

- The state logic which determines the machine *state* with the help of the *activity* and *detectedAnomaly* topic. For more information feel free to check [this article.](../general/mqtt.md)
  
### Inputs

### Outputs

[See also DCC Aachen example in our showcase.](../examples/dcc-assembly-analytics.md)

## 3. Configure your Dashboard

TODO
