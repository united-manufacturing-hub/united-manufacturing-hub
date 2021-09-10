
---
title: "factorycube-edge"
linkTitle: "factorycube-edge"
weight: 20
---

## sensorconnect

This tool automatically finds connected ifm gateways (e.g. the AL1350 or AL1352), extracts all relevant data and pushes the data to a MQTT broker. Technical information and usage can be found in the [documentation for sensorconnect](sensorconnect)

## cameraconnect

This tool automatically identifies connected cameras network-wide which support the GenICam standard and makes them utilizable. Each camera requires its own container. The camera acquisition can be triggered via MQTT. The resulting image data gets pushed to the MQTT broker. Technical information and usage can be found in the [documentation for cameraconnect](cameraconnect)

## barcodereader

This tool automatically detected connected USB barcode scanners and send the data to a MQTT broker. Technical information and usage can be found in the [documentation for barcodereader](barcodereader)

## mqtt-bridge

This tool acts as an MQTT bridge to handle bad internet connections. Messages are stored in a persistent queue on disk. This allows using the `factorycube-edge` in remote environments with bad internet connections. It will even survive restarts (e.g. internet failure and then 1h later power failure). We developed it as we've tested multiple MQTT brokers and their bridge functionalities (date of testing: 2021-03-15) and could not find a proper solution:

- emqx causes for internet blackouts longer than 3-4 min a DoS attack on our server (messages are send in endless loop, see https://github.com/emqx/emqx-bridge-mqtt/issues/81)
- VerneMQ only stored 10-20% of the data
- mosquitto was working very unreliable

## nodered

This tool is used to connect PLC and to process data. See also [Getting Started](/docs/getting-started/connecting-machines-creating-dashboards). Or take a look into the [official documentation](https://www.nodered.org/docs)

## emqx-edge

This tool is used as a central MQTT broker. See [emqx-edge documentation](https://docs.emqx.io/en/edge/latest/) for more information.
