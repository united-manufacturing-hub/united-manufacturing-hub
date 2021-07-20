
---
title: "Node-RED in Industrial IoT: a growing standard"
linkTitle: "Node-RED in Industrial IoT"
weight: 2
description: >
  How an open-source tool is establishing itself in a highly competitive environment against billion dollar companies 
---

{{< imgproc node-red-saw Fit "1000x1000" >}}Using Node-RED and <a href="https://www.unified-automation.com/products/development-tools/uaexpert.html">UaExpert</a> to extract data from the PLC of a Saw{{< /imgproc >}}

Most people know Node-RED from the areas of smart home or programming introductions (those workshops where you connect things with microcontrollers). Yet, very few people realize that it is frequently used in manufacturing as well. 

For those of you that do not know it yet, here is [the official self-description](https://nodered.org/) from the Node-RED website:
> Node-RED is a programming tool for wiring together hardware devices, APIs and online services in new and interesting ways.
>
> It provides a browser-based editor that makes it easy to wire together flows using the wide range of nodes in the palette that can be deployed to its runtime in a single-click.

And the best thing: **it is open-source**

The project started in early 2013 in IBM's research centers. In 2016 it was one of the founding projects of the JS Foundation. Since the [version release 1.0](https://nodered.org/blog/2019/09/30/version-1-0-released) in 2019 it is considered safe for production use.

A self-conducted survey in the same year showed that from 515 respondents, 31.5% use Node-RED in manufacturing, and from 868 respondents, 24% said they have created a PLC application using it [^2]. Also, 24.2 % of 871 respondents said that they use InfluxDB in combination with Node-RED. The reason we think that TimescaleDB is better suited for the Industrial IoT than InfluxDB has been described [in this article](https://docs.umh.app/docs/concepts/timescaledb-vs-influxdb/).

But how widespread is it really in manufacturing? What are these users doing with Node-RED? Let's deep dive into that!

[^2]: https://nodered.org/about/community/survey/2019/

## Usage of Node-RED in Industry

Gathering qualitative data of industry usage of specific solutions can be hard to almost impossible as very few companies are open about the technologies they use. However, we can still gather quantitative data, which strongly indicates a heavy usage in various industries in data extraction and processing.

**First, it is preinstalled on more and more various automation systems like PLCs.** [Wikpedia has a really good overview here (and it checks out!)](https://en.wikipedia.org/wiki/Node-RED). Siemens, in particular, is starting to use it more often, see also [Node-RED with SIMATICIOT2000](https://www.automation.siemens.com/sce-static/learning-training-documents/tia-portal/advanced-communication/sce-094-100-node-red-iot2000-de.pdf) or the [Visual Flow Creator](https://www.dex.siemens.com/mindsphere/applications/Visual-Flow-Creator) 

**Furthermore, various so-called "nodes" are available that can only be used in manufacturing environments**, e.g., to read out data from specific devices. These nodes also have quite impressive download numbers. 

Some examples:

- [node-red-contrib-opcua](https://flows.nodered.org/node/node-red-contrib-opcua) had 874 downloads in the last week (2021-07-02)
- [node-red-contrib-s7](https://flows.nodered.org/node/node-red-contrib-s7) had 902 downloads in the last week (2021-07-02) 
- [node-red-contrib-modbus](https://flows.nodered.org/node/node-red-contrib-modbus) had 4278 downloads in the last week (2021-07-02). However, it should be noted that this node can also be used in other areas

We've talked with the former developer of two of these nodes, [Klaus Landsdorf](https://github.com/biancode) from the German company [Iniationware](https://www.iniationware.com/), which offers companies support in the topics of OPC-UA, Modbus, BACnet and data modeling. 

Klaus confirmed our hypothesis:
> We get many requests from German hardware manufacturers that rely on Node-RED and on these industry-specific nodes like OPC-UA. The OPC-UA project was sponsored by just two small companies with round about 5% of the costs for development in the case of the IIoT OPC-UA contribution package. But in view of using the package and testing it across multiple industrial manufacturing environments to ensure a high stability, we had many and also big companies aboard. In education we have a great response from ILS, because they are using the Iniationware package node-red-contrib-iiot-opcua to teach their students about OPC-UA essentials. Unfortunately, just a few companies understand the idea of a commercial backing for open-source software companies by yearly subscriptions, which could safe a lot of money for each of them. Do it once, stable and share the payment in open-source projects! That would bring a stable community and contribution packages for the specific reley on industrial needs like LTS versions. Simplified: it needs a bit money to make money in a long term as well as to provide stable and up to date Node-RED packages.


**It is also described in the community as being production-ready and used quite frequently.** [In a topic discussing the question of production readiness](https://discourse.nodered.org/t/would-you-use-node-red-in-a-production-environment-for-iot-industry-applications/) a user with the name of `SonoraTechnical` says:

> Although anecdotal, just Friday, I was speaking to an engineer at a major OPC Software Vendor who commented that they see Node-RED frequently deployed by industrial clients and even use it internally for proving out concepts and technology.

Another one with the name of `gemini86` explains the advantages compared with commercial solutions:

> I'm also (very much) late to the party on this, but I work in manufacturing and use AB, Siemens, Codesys, etc. I also use Node-RED for SCADA and database bridging. Our site has well pumps in remote areas where data and commands are sent over 900mhz ethernet radios, and Node-RED handles the MQTT <> modbusRTU processing. Node-RED has been as stable and quick, if not quicker than any Siemens or AB install with comparable network functionality. In fact, I struggled to get my S7-1200 to properly communicate with modbusRTU devices at all. I was completely baffled by their lack of documentation on getting it to work. Their answer? "Use profibus/profinet." So, I myself prefer Node-RED for anything to do with serial or network communications.

**Last but not least, it is very frequently used in scientific environments.** There are over [3.000 research papers](https://scholar.google.de/scholar?hl=de&as_sdt=0%2C5&q=node-red+industrial) available on Google Scholar on the usage of Node-RED in industrial environments!

Therefore, it is safe to say that it is widespread, with growing numbers of users in industry. But what exactly can you do with it? Let us give some examples of how we are using it!

## What you can do with it

The United Manufacturing Hub relies on Node-RED as a tool to

1. Extract data from production machines using various protocols (OPC/UA, Modbus, S7, HTTP, TCP, ...)
2. Processing and unifying data points into our [standardized data model](https://docs.umh.app/docs/concepts/mqtt/) 
3. Customer-specific integrations into existing systems, e.g., MES or ERP systems like SAP or Oracle
4. Combining data from various machines and triggering actions (machine to machine communication or, in short, M2M)
5. Creating small interactive and customer-specific dashboards to trigger actions like specifying stop reasons

Let's explain each one by going through them step-by-step:

### 1. Extract data from production machines using various protocols

One central challenge of Industrial IoT is obtaining data. The shopfloor is usually fitted out with machines from various vendors and of different ages. As there is almost little or no standardization in the protocols or semantics, the data extraction process needs to be customized for each machine.

With Node-RED, various protocols are available as so-called "nodes" - from automation protocols like OPC/UA (see earlier) to various IT protocols like TCP or HTTP. For any other automation protocol, you can use PTC Kepware, which supports over [140 various PLC protocols](https://www.kepware.com/de-de/products/kepserverex/product-search/?productType=c258dc8b-ec42-48f6-9a6c-b6132dcab08d). 

### 2. Processing and unifying data points into our standardized data model

Node-RED was originally developed for

> visualizing and manipulating mappings between MQTT topics [^noderedabout]
and this is what we are still using it for today. All these data points that have been extracted from various production machines now need to be standardized to match our data model. The machine state needs to be calculated, the machines’ output converted from various formats into a simple `/count` message, etc. 

More information about this can be found [in our data model for Industrial IoT](https://docs.umh.app/docs/concepts/mqtt/).

{{< imgproc nodered.png Fit "800x800" >}}Example of working with the United Manufacturing Hub. Everything is flow-based.{{< /imgproc >}}

[^noderedabout]: https://nodered.org/about/

### 3. Customer-specific integrations into existing systems

It is not just good for extracting and processing data. It is also very good for pushing this processed data back into other systems, e.g., MES or ERP systems like Oracle or SAP. These systems usually have REST APIs, e.g., [here is an example for the REST API for the Oracle ERP](https://docs.oracle.com/en/cloud/saas/supply-chain-management/20b/fasrp/rest-endpoints.html).

As the customer implementations of those systems are usually different, the resulting APIs are mostly also different. Therefore, one needs a system that is quick to use to handle those APIs. And Node-RED is perfect for this.

### 4. Machine to machine communication

{{< imgproc 20210702_174941.jpg Fit "600x600" >}}The AGV automatically gets the finished products from one machine and brings them to empty stations, which is a good example for M2M{{< /imgproc >}}

As a result [of our data architecture](https://docs.umh.app/docs/concepts/) machine to machine communication (M2M) is enabled by default. The data from all edge devices is automatically sent to a central MQTT broker and is available to all connected devices (that have been allowed access to that data).

It is easy to gather data from various machines and trigger additional actions, e.g., to trigger the Automated Guided Vehicle (AGV) to fetch material from the production machine when one station is empty of material.

And the perfect tool to set those small triggers is, as you might have guessed, Node-RED.

### 5. Creating small interactive and customer-specific dashboards

{{< imgproc tablet.png Fit "600x600" >}}Example of a dashboard using <a href="https://flows.nodered.org/node/node-red-dashboard">node-red-dashboard</a>. It features a multi-level stop reason selection and the visualization of production speed.{{< /imgproc >}}

Sometimes the machine operators need time-sensitive dashboards to retrieve real-time information or to interact with the system. As many companies still do not have a good and reliable internet connection or even network infrastructure, one cannot wait until the website is fully loaded to enter a stop reason. Therefore, sometimes it is crucial to have a dashboard as close to the machine as possible (and not sitting somewhere in the cloud).

For this one, you can use the [node-red-dashboard node](https://flows.nodered.org/node/node-red-dashboard), which allows you to easily create dashboards and interact with the data via MQTT. 

### Bonus: What not to do: process control

However, we strongly recommend NOT using it to intervene in the production process, e.g., process control or ensuring safety mechanisms for two reasons:

1. IT tools and systems like Node-RED are not designed to ensure the safety of machines or people, e.g., guaranteed time-sensitive reactions to a triggered safety alert
2. It would also be almost impossible to get that certified and approved due to 1.
For these aspects, very good and safe tools, like PLCs or NCs, are already out there in the automation world. 

## Summary

The slogan: "The best things in life are free" also applies in manufacturing: 

Node-RED is on the same level as "professional" closed-source and commercial solutions and is used by thousands of researchers and hundreds of daily users in various manufacturing industries. 

It is included and enabled in every installation of the United Manufacturing Hub - in the cloud and on the edge. 

More information on how we use the system can be found in our [Quick Start](https://docs.umh.app/docs/getting-started/connecting-machines-creating-dashboards/).
