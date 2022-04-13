---
title: "How to connect the United Manufacturing Hub to Tulip"
linkTitle: "Connecting UMH with Tulip"
description: >
  Combine all shop floor data with work instructions from Tulip (www.tulip.co) using the UMH approach. This approach does not require opening ports and additionally allows to deeply integrate Tulip into the shopfloor IT. For more information on the shared value proposition and the use-case please check out also [our blog article](https://www.umh.app/post/combine-all-shop-floor-data-with-work-instructions)
---

## Prerequisites
-	An active Tulip subscription.
-	The UMH platform installed and configured.
-	Available data extracted from sensors, industrial controllers or other shopfloor IT systems and available in the MQTT broker or Apache Kafka.

## About Tulip

[Tulip](https://www.tulip.co) is a frontline operations platform, which is deployed in manufacturing and logistic environments, bridging the interaction between people, machines, devices, and systems. 

Tulip’s most widely documented use case (in different and extensive channels such as [“Tulip Libraries”](https://tulip.co/library/), [“Tulip University”](https://tulip.co/university/) and [“Tulip Community”](https://community.tulip.co/)) focusses on their hardware “I/O Gateway” and [“Edge IO”](https://tulip.co/products/edge-io/) developed for the deployment of work instructions running in the Tulip Cloud. These work instructions can be authorized with additional hardware that connects Tulip approved devices via USB port and sensors to GPIO ports. The main function of the gateway is to send the data to the Tulip app.

{{< imgproc factory_kit.png Fit "800x500" >}}Tulip Edge IO with other devices. Source: https://tulip.co/products/edge-io/ {{< /imgproc >}}

Within the Tulip platform, users can create applications using a a no-code interface. The operations are triggered by various sensors and forwarded by the hardware to the Tulip Cloud.

{{< imgproc work-instructions.png Fit "800x500" >}}Work instructions in an assembly cell{{< /imgproc >}}

In addition to apps, users can create “Tulip Analyses” to monitor KPIs and trends and provide insights to the production team. Interactive dashboards can also be created to monitor KPIs of interest.

The United Manufacturing Hub (UMH) provides the necessary IT/OT infrastructure not only to connect real-time manufacturing data to Tulip, but to also share it with other solutions and applications. For example, data coming from sensors, barcode readers, cameras and industrial controllers can now be used for real-time stream or batch processing by other use cases such as predictive maintenance, quality management systems, track & trace / digital shadow, and supervisor dashboards. These use cases can then run different clouds such as [AWS](/docs/getting-started/usage-in-production/#example-deployment-on-aws-eks) or [Azure](/docs/tutorials/azure/).

Additionally, this allows for adding data sources to Tulip  that are yet to be officially supported (e.g., [IO-Link sensors](/docs/examples/flame-cutting/#light-barriers)). To access local data sources, a [Tulip Connector Host](https://support.tulip.co/en/articles/2221539-introduction-to-tulip-connector-hosts) and/or [opening ports](https://support.tulip.co/en/articles/2259747-networking-requirements-for-a-tulip-cloud-deployment) is no longer required, which should align with the IT security department’s requirements.

## Approach

In this tutorial we will choose the "Tulip Machine Attributes API" approach. Other method is using the [Tulip Connector Host](https://support.tulip.co/en/articles/2221539-introduction-to-tulip-connector-hosts). The approach chosen here has the advantage that it does not requiring [opening ports](https://support.tulip.co/en/articles/2259747-networking-requirements-for-a-tulip-cloud-deployment) and is therefore more easily integrated into enterprise IT.

## Step-by-step tutorial

{{% alert title="Info" color="primary" %}}
See also Tulip knowledge base resource article [How to use the Machine Attributes API](https://support.tulip.co/en/articles/5007794-how-to-use-the-machine-attributes-api)
{{% /alert %}}

Perform the following steps as a Tulip administrator. First, in the Tulip platform, go to your profile icon and click Settings. Then select the Bot tab and create one. For this project, only data should be read and displayed in the Tulip environment, therefore the scope must be set to "read-only".
 
{{< imgproc Picture2.png Fit "800x500" >}}
Tulip Bot showing the different scopes.
{{< /imgproc >}}

The API Key, Secret and Auth Header are displayed. Please store this information in a safe place.
 
{{< imgproc Picture3.png Fit "800x500" >}}
API Key, secret and authentication header is shown.
{{< /imgproc >}}

Then you will need to link this Machine API to a Machine.

### Create a Machine Type

Navigate to the tab “Shopfloor”  “Machines” then change to the tab “Machine Types”. Here, click in the button “Create type”. For now, only change the name of the machine type. 
 
{{< imgproc Picture5.png Fit "800x500" >}}
Creating a Machine Type.
{{< /imgproc >}}

### Create a machine

Go to the Machine Library tab and click the Create Machine button. Assign a name for your machine. Then select Tulip API as the data source. The Tulip interface will ask you to assign your machine to a station. If multiple stations use the data, we recommend selecting "None". Finally, select the machine type you just created.
 
{{< imgproc Picture6.png Fit "800x500" >}}
Creating a Machine.
{{< /imgproc >}}

### Add Machine Attribute

Return to the Machine Type tab and navigate to the Machine Type you previously created. Click the edit button and scroll down and head to “Add Machine Attribute”.
 
{{< imgproc Picture7.png Fit "800x500" >}}
Adding a Machine Attribute.
{{< /imgproc >}}

Here you are able to create new machine attributes or assign ones you created and used before.
 
{{< imgproc Picture8.png Fit "800x500" >}}
Assign a name and a type to the Machine Attribute.
{{< /imgproc >}}

Having performed the previous steps, you are now able to to find the field mappings for each Machine Attribute. Go to the tab “Shopfloor/Machines”, select the machine name of interest, and click in the Configuration tab. There you’ll discover field mappings for each Machine Attributes. The “Attibute ID” and “Machine ID” will be used for setting up the connection in Node-RED.
 
{{< imgproc Picture9.png Fit "800x500" >}}
Field mappings
{{< /imgproc >}}

Having performed the previous steps, the Tulip configuration is finalized. The next part of the article describes the configuration in Node-RED that is necessary for connecting the data acquisition layer performed in Node-RED with the Tulip platform. 

### Connecting to the Tulip Machines Attributes API using Node-RED

In the manufacturer’s environment, UMH installed an electrical cabinet (E-rack) with an Industrial PC (IPC). The IPC was connected to the industrial controller of the machine and reads several data tags such as power time, hydraulic pump time and power consumption, with a flow in Node-RED (see also the blog article mentioned at the beginning of the article)
 
{{< imgproc Picture10.png Fit "800x500" >}}
Automatic saw with E-rack.
{{< /imgproc >}}

To connect to the Tulip Machine Attributes API, two nodes were used, namely a function node and a HTTP response. 
 
{{< imgproc Picture11.png Fit "800x500" >}}
Function and HTTP response nodes used.
{{< /imgproc >}}

For the function node, the message must contain the “Attribute ID” and “Machine ID” corresponding to the data tag being acquired from the industrial controller.
 
{{< imgproc Picture12.png Fit "800x500" >}}
Content in the function node.
{{< /imgproc >}}

For the HTTP response node, a configuration with the Tulip platform is required. This information was acquired while creating the bot in the Tulip environment.
 
{{< imgproc Picture13.png Fit "800x500" >}}
HTTP response node configuration.
{{< /imgproc >}}

Having set everything up, the connection between the Node-RED flow and the Tulip platform is now established. On the next and final part of the blog, it will be shown how to use the data for developing it in the Tulip app environment.

### Using the data in Tulip app
Data coming from industrial controllers or sensors can be connected, leveraging the Machine Attribute created as any other Tulip device. For this, please navigate to the “Apps” tab and create a new app or select one you already created.
 In the App editor, select to create a new trigger. 
 
The trigger menu will appear, and by selecting:
When – Machine – Specific Machine
The  Machine that was created in the previous steps will appear. Select it and in the outputs, select the corresponding Machine attributes. Select the one that concerns you and by doing so you can leverage  the data extracted from the machine.

{{< imgproc Picture14.png Fit "800x500" >}}
Digital Work instruction app with Machine Attribute API connectivity.
{{< /imgproc >}}

## Summary

This approach makes it possible to connect additional data sources to Tulip and combine Tulip with a Unified Namespace. It also reduces implementation time by eliminating the need to open ports in the corporate firewall.

Interested? Then check out also the corresponding [blog article](https://www.umh.app/post/combine-all-shop-floor-data-with-work-instructions)
