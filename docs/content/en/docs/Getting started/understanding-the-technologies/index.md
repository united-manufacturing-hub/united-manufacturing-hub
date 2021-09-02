---
title: "0. Understanding the technologies"
linkTitle: "0. Understanding the technologies"
weight: 1
description: >
   Strongly recommended. This section gives you an introduction into the used technologies. A rough understanding of these technologies is fundamental for installing and working with the system. Additionally, this article provides further learning materials for certain technologies.
---


## Introduction into IT / OT

The materials presented below are usually teached in a 2-3 h workshop session. You can find the presentation further below. 

## Deep-dive: IT technologies and techniques

### Flashing a operating system onto a USB-stick

### Installing operating systems from a USB-stick

### Connecting with SSH

#### For Windows

We recommend MobaXTerm. **TODO**

#### For Linux

For Linux you can typically use the inbuilt commands to connect with a device via SSH. Connect using the following command:

`ssh <username>@<IP>`, e.g., `ssh rancher@192.168.99.118`.

{{< imgproc SSH_linux_1.png Fit "800x500" >}}Connect via SSH{{< /imgproc >}}

There will be a warning saying that the authenticity of the host can't be established. Enter `yes` to continue with the connection.

{{< imgproc SSH_linux_2.png Fit "800x500" >}}Warning message: The authenticity of host 'xxx' can't be established.{{< /imgproc >}}

Enter the password and press enter. The default password of the auto setup will be `rancher`.

{{< imgproc SSH_linux_3.png Fit "800x500" >}}Successfully logged in via SSH{{< /imgproc >}}

### Development network

{{< imgproc development-network.png Fit "500x300" >}}{{< /imgproc >}}

### Versioning

### Connect to Kubernetes using Lens

### ...

## Deep Dive: OT technologies and techniques

### ...










# LEGACY

## Why your IT loves us

We built the United Manufacturing Hub with the needs of your IT in mind from day one and adhere to the highest standards of flexibility, scalability, security, and data protection.

### Open

It is Open-Source (AGPL). This means you can commercially use, modify and distribute the project as long as you disclose the source code and all modifications. A more detailled overview can be found [here](https://www.tldrlegal.com/l/agpl3). However, legally binding is the full license in `LICENSE`

Furthermore, it is based on well-documented standard interfaces (MQTT, REST, etc.). A more detailled explaination of those can be found further below.

### Scalable

The system is built for horizontal scaling, including fault tolerance through Docker/Kubernetes/Helm. This means the system is built to run on multiple servers and heals itself when a server unexpectetly crashes. More information on the technology can be found further below.

(enterprise only) Edge devices can be set up and configured quickly in large quantities (somehow we must make our money :) )

### Flexible

All components have flexible deployment options, from public cloud (Azure, AWS, etc.) to on-premise server installations to Raspberry Pis, everything is possible.

Additionally, you can freely choose your programming language as systems are connected through a central message broker (MQTT). Almost all programming languages have a very good MQTT library abvailable.

### Tailored for manufacturing

The United Manufacturing Hub includes ready-made manufacturing apps, which deliver immediate business value and give a good oundation to built upon. It is not a generic IT solution and makes heavily use of established automation standards (OPC/UA, Modbus, etc.).

With this we can connect production assets very quickly either by retrofit or connection to existing interfaces.

### Well-documented

Builds exclusively on well-documented software components supported by a large developer community. No more waiting in random telphone hotlines, just google your question and get your solution within seconds. If you need additionally enterprise support you can always contact us and/or the vendors on which the system is built upon (Grafana, etc.).

### Secure

Meets the highest data protection standards. Compliant with the three pillars of information security: 

- Confidentiality (through e.g. end-to-end encryption, flexible deployment options, and principle of least privilege), 
- integrity (through e.g. ACID databases and MQTT QoS 2 with TLS), 
- and availability (through e.g. Use of Kubernetes and (for SaaS) a CDN)

## Technologies

Here you can find an overview over the used technologies. As there are many good tutorials out there, we will not explain everything and only link to some of these tutorials. In doubt, just google it :)

### Interfaces

The United Manufacturing Hub features various interfaces to push and extract data.

#### MQTT

Here is a video tutorial from Rui Santos on YouTube: https://www.youtube.com/watch?v=EIxdz-2rhLs

Out of the [wikipedia article about MQTT](https://en.wikipedia.org/wiki/MQTT):

> MQTT (originally an acronym for MQ Telemetry Transport) is an lightweight, publish-subscribe network protocol that transports messages between devices. The MQTT protocol defines two types of network entities: a message broker and a number of clients. An MQTT broker is a server that receives all messages from the clients and then routes the messages to the appropriate destination clients. An MQTT client is any device (from a micro controller up to a fully-fledged server) that runs an MQTT library and connects to an MQTT broker over a network.

The protocol MQTT specifies not the actual content of the exchanged MQTT messages. In the UMH stack we specify them further with the [UMH datamodel](../mqtt/)

#### REST / HTTP

Out of the [wikipedia article about REST](https://en.wikipedia.org/wiki/Representational_state_transfer):

> Representational state transfer (REST) is a de-facto standard for a software architecture for interactive applications that typically use multiple Web services. In order to be used in a REST-based application, a Web Service needs to meet certain constraints; such a Web Service is called RESTful. A RESTful Web service is required to provide an application access to its Web resources in a textual representation and support reading and modification of them with a stateless protocol and a predefined set of operations. By being RESTfull, Web Services provide interoperability between the computer systems on the internet that provide these services.

factoryinsight provides a REST / HTTP access to all data stored in the database. [The entire API is well-documented](../developers/factorycube-server/factoryinsight).

### Orchestration tools

Docker, Kubernetes and Helm are used for a so called orchestration. You can find further information on these technologies here:

- Docker (https://www.youtube.com/watch?v=Gjnup-PuquQ),
- Kubernetes (https://www.youtube.com/watch?v=PziYflu8cB8),
- Helm (https://www.youtube.com/watch?v=fy8SHvNZGeE),

### Additional tools / tutorials

- nodered (https://www.youtube.com/watch?v=3AR432bguOY)
- SSL (https://www.youtube.com/watch?v=hExRDVZHhig)
- Microservice approach. The UMH is based on the [microservices approach](https://en.wikipedia.org/wiki/Microservices) and arranges an application as a collection of loosely coupled services. Each service is a self-contained piece of business functionality with clear interfaces.

## Next steps

If you are interested more in the architecture of the systems we recommend that you take a look into our [concepts](../../concepts/). If not, you can go to the next step in the tutorial.

