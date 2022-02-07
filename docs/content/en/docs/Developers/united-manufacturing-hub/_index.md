
---
title: "united-manufacturing-hub"
linkTitle: "united-manufacturing-hub"
weight: 5
description: "The architecture of united-manufacturing-hub"
aliases:
  - /docs/Developers/factorycube-edge
  - /docs/developers/factorycube-edge
  - /docs/Developers/factorycube-server
  - /docs/developers/factorycube-server
---

{{< imgproc server Fit "1200x1200" >}}{{< /imgproc >}}

## factoryinsight

factoryinsight is an open source REST API written in Golang to fetch manufacturing data from a timescaleDB database and calculate various manufacturing KPIs before delivering it to a user visualization, e.g. [Grafana] or [PowerBI].

Features:

- OEE (Overall Equipment Effectiveness), including various options to investigate OEE losses (e.g. analysis over time, microstop analytics, changeover deep-dives, etc.)
- Various options to investigate OEE losses further, for example stop analysis over time, microstop analytics, paretos, changeover deep-dives or stop histograms
- Scalable, microservice oriented approach for Plug-and-Play usage in Kubernetes or behind load balancers (including health checks and monitoring)
- Compatible with important automation standards, e.g. Weihenstephaner Standards 09.01 (for filling), [Omron PackML (for packaging/filling)](https://de.scribd.com/document/339103883/PackML-Unit-Machine-Implementation-Guide-V1-00), [EUROMAP 84.1 (for plastic)](https://www.euromap.org/euromap84), [OPC 30060 (for tobacco machines)](https://reference.opcfoundation.org/v104/TMC/v100/docs/) and [VDMA 40502 (for CNC machines)](http://normung.vdma.org/viewer/-/v2article/render/32921121)

The openapi documentation can be found [here](/docs/developers/united-manufacturing-hub/factoryinsight)

## mqtt-to-postgresql

the tool to store incoming MQTT messages to the postgres / timescaleDB database

Technical information and usage can be found in the [documentation for mqtt-to-postgresql](/docs/developers/united-manufacturing-hub/mqtt-to-postgresql)

## grafana-auth

Proxies request from grafana to various backend services, while authenticating the grafana user.
Technical information and usage can be found in the [documentation for grafana-proxy](/docs/developers/united-manufacturing-hub/grafana-proxy)

## grafana-plugins

Contains our grafana datasource plugin and our input panel


## sensorconnect

This tool automatically finds connected ifm gateways (e.g. the AL1350 or AL1352), extracts all relevant data and pushes the data to a MQTT broker. Technical information and usage can be found in the [documentation for sensorconnect](sensorconnect)

## cameraconnect

This tool automatically identifies connected cameras network-wide which support the GenICam standard and makes them utilizable. Each camera requires its own container. The camera acquisition can be triggered via MQTT. The resulting image data gets pushed to the MQTT broker. Technical information and usage can be found in the [documentation for cameraconnect](cameraconnect)

## barcodereader

This tool automatically detected connected USB barcode scanners and send the data to a MQTT broker. Technical information and usage can be found in the [documentation for barcodereader](barcodereader)

## mqtt-bridge

This tool acts as an MQTT bridge to handle bad internet connections. Messages are stored in a persistent queue on disk. This allows using the `united-manufacturing-hub` in remote environments with bad internet connections. It will even survive restarts (e.g. internet failure and then 1h later power failure). We developed it as we've tested multiple MQTT brokers and their bridge functionalities (date of testing: 2021-03-15) and could not find a proper solution:

- emqx causes for internet blackouts longer than 3-4 min a DoS attack on our server (messages are send in endless loop, see https://github.com/emqx/emqx-bridge-mqtt/issues/81)
- VerneMQ only stored 10-20% of the data
- mosquitto was working very unreliable

## nodered

This tool is used to connect PLC and to process data. See also [Getting Started](/docs/getting-started/connecting-machines-creating-dashboards). Or take a look into the [official documentation](https://www.nodered.org/docs)
