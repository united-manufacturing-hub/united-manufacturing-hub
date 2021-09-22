
---
title: "factorycube-server"
linkTitle: "factorycube-server"
weight: 5 
description: >
 The architecture of factorycube-server 
---

{{< imgproc server Fit "1200x1200" >}}{{< /imgproc >}}

## factoryinsight

factoryinsight is an open source REST API written in Golang to fetch manufacturing data from a timescaleDB database and calculate various manufacturing KPIs before delivering it to a user visualization, e.g. [Grafana] or [PowerBI].

Features:

- OEE (Overall Equipment Effectiveness), including various options to investigate OEE losses (e.g. analysis over time, microstop analytics, changeover deep-dives, etc.)
- Various options to investigate OEE losses further, for example stop analysis over time, microstop analytics, paretos, changeover deep-dives or stop histograms
- Scalable, microservice oriented approach for Plug-and-Play usage in Kubernetes or behind load balancers (including health checks and monitoring)
- Compatible with important automation standards, e.g. Weihenstephaner Standards 09.01 (for filling), [Omron PackML (for packaging/filling)](https://de.scribd.com/document/339103883/PackML-Unit-Machine-Implementation-Guide-V1-00), [EUROMAP 84.1 (for plastic)](https://www.euromap.org/euromap84), [OPC 30060 (for tobacco machines)](https://reference.opcfoundation.org/v104/TMC/v100/docs/) and [VDMA 40502 (for CNC machines)](http://normung.vdma.org/viewer/-/v2article/render/32921121)

The openapi documentation can be found [here](/docs/developers/factorycube-server/factoryinsight)

## mqtt-to-postgresql

the tool to store incoming MQTT messages to the postgres / timescaleDB database

Technical information and usage can be found in the [documentation for mqtt-to-postgresql](/docs/developers/factorycube-server/mqtt-to-postgresql)

## grafana-auth

Proxies request from grafana to various backend services, while authenticating the grafana user.
Technical information and usage can be found in the [documentation for grafana-proxy](/docs/developers/factorycube-server/grafana-proxy)

## grafana-plugins

Contains our grafana datasource plugin and our input panel
