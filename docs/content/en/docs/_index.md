
---
title: "Documentation"
linkTitle: "Documentation"
weight: 20
description: "United Manufacturing Hub - the open source manufacturing system"
menu:
  main:
    weight: 20
---

<img src="/images/Otto.svg" style="height: 150px !important">

## About The Project

The United Manufacturing System is an open source solution for extracting and analyzing data from manufacturing plants and sensors. The Hub includes both software and hardware components to enable the retrofit of productions plants by plug-and-play as well as to integrate existing machine PLCs and IT systems. The result is an end-to-end solution for various questions in manufacturing such as the optimization of production through OEE analysis, preventive maintenance through condition analysis and quality improvement through stop analysis.

- **Open**. open-source (see `LICENSE`) and open and well-documented standard interfaces (MQTT, REST, etc.)
- **Scalable**. Horizontal scaling incl. fault tolerance through Docker / Kubernetes / Helm. Edge devices can be quickly set up and configured in large numbers.
- **Flexible**. Flexible deployment options, from public cloud (Azure, AWS, etc.) to on-premise server installations to Raspberry Pis, everything is possible. Free choice of programming language and systems to be connected through central message broker (MQTT).
- **Tailor-made for production**. Pre-built apps for manufacturing. Use of established automation standards (OPC/UA, Modbus, etc.). Quick connection of production assets either by retrofit or by connecting to existing interfaces.
- **Community and support**. Enterprise support and community for the whole package. Built exclusively on well-documented software components with a large community.
- **Information Security & Data Protection**. Implementation of the central protection goals of information security. High confidentiality through e.g. end-to-end encryption, flexible provisioning options and principle of least privilege. High integrity through e.g. ACID databases and MQTT QoS 2 with TLS. High availability through e.g. use of Kubernetes and (for SaaS) a CDN. 

![Demo](dashboard.gif)
