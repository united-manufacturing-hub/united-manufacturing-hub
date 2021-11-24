---
title: "sensorconnect"
linkTitle: "sensorconnect"
description: >
  This docker container automatically detects ifm gateways in the specified network and reads their sensor values in the highest possible data frequency. The MQTT output is specified in [the MQTT documentation](/docs/concepts/mqtt/)
---

## Getting started

Here is a quick tutorial on how to start up a basic configuration / a basic docker-compose stack, so that you can develop.

1. Specify the environment variables, e.g. in a .env file in the main folder or directly in the docker-compose
2. execute `sudo docker-compose -f ./deployment/sensorconnect/docker-compose.yaml up -d --build`

## Environment variables

This chapter explains all used environment variables.

### TRANSMITTERID

Description: The unique transmitter id. This will be used for the creation of the MQTT topic. ia/raw/TRANSMITTERID/...

Type: string

Possible values: all

Example value: 2021-0156

### BROKER_URL

**Description:** The MQTT broker URL

**Type:** string

**Possible values:** IP, DNS name

**Example value:** ia_mosquitto

**Example value 2:** localhost

### BROKER_PORT

**Description:** The MQTT broker port. Only unencrypted ports are allowed here (default: 1883)

**Type:** integer

**Possible values:** all

**Example value:** 1883

### IP_RANGE

**Description:** The IP range to search for ifm gateways

**Type:** string

**Possible values:** All subnets in [CIDR notation](https://en.wikipedia.org/wiki/Classless_Inter-Domain_Routing)

**Example value:** 172.16.0.0/24

## Underlying Functionality
*We are right now rewriting sensorconnect, the following paragraph might not be fully implemented yet*
Sensorconnect is based on IO-Link. Device manufacturers will provide one IODD file (IO Device Description), for every sensor and actuator they produce.
Those contain information, e.g. necessary to correctly interpret data from the devices. They are in XML-format. Sensorconnect will try to download relevant IODD files automatically after installation from the IODDfinder (https://io-link.com/en/IODDfinder/IODDfinder.php). We will also provide a folder to manually deposit IODD-files, if the automatic download doesn't work.

Sensorconnect will scan for ifm gateways (used to connect the IO-Link devices to) and request the current modes from their REST-API's. 
```.js
{
  "code":"request",
  "cid":-1,
  "adr":"/getdatamulti",
  "data":{
    "datatosend":[
      "/iolinkmaster/port[1]/mode",
      "/iolinkmaster/port<n>/mode" //going through all ports on gateway
      ]
    }
}
```


Based on the VendorIdentifier and DeviceIdentifier (specified in the received data from the ifm-gateway), sensorconnect can look up relevant information from the IODD file to interpret the data.

Now sensorconnect converts the data and sends it (as a JSON) via MQTT to the MQTT broker.


